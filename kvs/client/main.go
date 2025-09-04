package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Client struct {
	rpcClient *rpc.Client
	rpcCache  sync.Map // map[string]*clientCacheLine
	clientID  string   // address:port of this client for receiving invalidations
	listener  net.Listener
}

type clientCacheLine struct {
	Value        string
	LastAccessed time.Time
	Invalidation chan struct{}
}

// Invalidate handles cache invalidation requests from the server
func (client *Client) Invalidate(request *kvs.InvalidationRequest, response *kvs.InvalidationResponse) error {
	if val, ok := client.rpcCache.Load(request.Key); ok {
		cacheLine := val.(*clientCacheLine)
		// Signal invalidation
		select {
		case cacheLine.Invalidation <- struct{}{}:
		default:
			// Channel already has an invalidation signal
		}
		client.rpcCache.Delete(request.Key)
	}
	return nil
}

func NewClient(serverAddr string) *Client {
	// Start RPC server for receiving invalidations
	client := &Client{}
	rpc.Register(client)
	rpc.HandleHTTP()

	// Listen on any available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
	}
	client.listener = listener
	client.clientID = listener.Addr().String()

	// Start HTTP server for invalidation callbacks
	go http.Serve(listener, nil)

	// Connect to the KV server
	rpcClient, err := rpc.DialHTTP("tcp", serverAddr)
	if err != nil {
		log.Fatal(err)
	}
	client.rpcClient = rpcClient

	return client
}

func (client *Client) Get(key string) string {
	// Look in cache first
	if val, ok := client.rpcCache.Load(key); ok {
		cacheLine := val.(*clientCacheLine)

		// Check if there's any invalidation signal
		select {
		case <-cacheLine.Invalidation:
			// Cache was invalidated, remove it
			client.rpcCache.Delete(key)
		default:
			// Cache is still valid, update last accessed time and return value
			cacheLine.LastAccessed = time.Now()
			return cacheLine.Value
		}
	}

	// Cache miss or invalidated, fetch from server
	request := kvs.GetRequest{
		Key: key,
	}
	response := kvs.GetResponse{}
	err := client.rpcClient.Call("KVService.Get", &request, &response)
	if err != nil {
		log.Fatal(err)
	}

	// Store in cache
	cacheLine := &clientCacheLine{
		Value:        response.Value,
		LastAccessed: time.Now(),
		Invalidation: make(chan struct{}, 1),
	}
	client.rpcCache.Store(key, cacheLine)

	// Register cache entry with server
	registerReq := &kvs.RegisterCacheRequest{
		Key:      key,
		ClientID: client.clientID,
	}
	registerResp := &kvs.RegisterCacheResponse{}
	err = client.rpcClient.Call("KVService.RegisterCache", registerReq, registerResp)
	if err != nil {
		// If registration fails, remove from local cache
		client.rpcCache.Delete(key)
		log.Printf("Failed to register cache entry: %v", err)
	}

	return response.Value
}

func (client *Client) Put(key string, value string) {
	// First invalidate any cached entry
	if val, ok := client.rpcCache.Load(key); ok {
		cacheLine := val.(*clientCacheLine)
		// Signal invalidation
		select {
		case cacheLine.Invalidation <- struct{}{}:
		default:
			// Channel already has an invalidation signal
		}
		client.rpcCache.Delete(key)
	}

	request := kvs.PutRequest{
		Key:   key,
		Value: value,
	}
	response := kvs.PutResponse{}
	err := client.rpcClient.Call("KVService.Put", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
}

func runClient(id int, addr string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
	client := NewClient(addr)
	defer client.listener.Close()

	value := strings.Repeat("x", 128)
	const batchSize = 1024

	opsCompleted := uint64(0)

	for !done.Load() {
		for j := 0; j < batchSize; j++ {
			op := workload.Next()
			key := fmt.Sprintf("%d", op.Key)
			if op.IsRead {
				client.Get(key)
			} else {
				client.Put(key, value)
			}
			opsCompleted++
		}
	}

	fmt.Printf("Client %d finished operations.\n", id)

	resultsCh <- opsCompleted
}

type HostList []string

func (h *HostList) String() string {
	return strings.Join(*h, ",")
}

func (h *HostList) Set(value string) error {
	*h = strings.Split(value, ",")
	return nil
}

func main() {
	hosts := HostList{}

	flag.Var(&hosts, "hosts", "Comma-separated list of host:ports to connect to")
	theta := flag.Float64("theta", 0.99, "Zipfian distribution skew parameter")
	workload := flag.String("workload", "YCSB-B", "Workload type (YCSB-A, YCSB-B, YCSB-C)")
	secs := flag.Int("secs", 30, "Duration in seconds for each client to run")
	flag.Parse()

	if len(hosts) == 0 {
		hosts = append(hosts, "localhost:8080")
	}

	fmt.Printf(
		"hosts %v\n"+
			"theta %.2f\n"+
			"workload %s\n"+
			"secs %d\n",
		hosts, *theta, *workload, *secs,
	)

	start := time.Now()

	done := atomic.Bool{}
	resultsCh := make(chan uint64)

	host := hosts[0]
	clientId := 0
	go func(clientId int) {
		workload := kvs.NewWorkload(*workload, *theta)
		runClient(clientId, host, &done, workload, resultsCh)
	}(clientId)

	time.Sleep(time.Duration(*secs) * time.Second)
	done.Store(true)

	opsCompleted := <-resultsCh

	elapsed := time.Since(start)

	opsPerSec := float64(opsCompleted) / elapsed.Seconds()
	fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
}
