package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Client struct {
	rpcServers []*rpc.Client
	cache      KVCache
	Address    string // address of this client for receiving update rpc's
	ID         int
}

// structure for server to rpc Update() when other clients write to the KV store
type KVCache struct {
	sync.Map
	sync.Mutex
	hits    int
	misses  int
	updates int
}

type clientCacheLine struct {
	Value string
}

// KV server can request caches to update their values when writes from other clients are received
func (cache *KVCache) Update(request *kvs.UpdateRequest, response *kvs.UpdateResponse) error {
	cacheLine := &clientCacheLine{
		Value: request.Value,
	}
	cache.Lock()
	cache.updates++
	cache.Unlock()
	cache.Store(request.Key, cacheLine)
	return nil
}

func NewClient(clientID int, clientAddr string, serverAddrs []string) *Client {
	// Create new client and connect to KV servers
	client := &Client{}
	for _, addr := range serverAddrs {
		rpcServer, err := rpc.DialHTTP("tcp", addr)
		if err != nil {
			log.Fatal(err)
		}
		client.rpcServers = append(client.rpcServers, rpcServer)
	}
	client.Address = clientAddr
	client.ID = clientID

	return client
}

func (client *Client) Get(key string) string {
	// Look in cache first
	if val, ok := client.cache.Load(key); ok {
		client.cache.Lock()
		client.cache.hits++
		client.cache.Unlock()
		cacheLine := val.(*clientCacheLine)
		return cacheLine.Value
	}

	client.cache.Lock()
	client.cache.misses++
	client.cache.Unlock()
	// Cache miss -> fetch from server and register client is caching this key
	request := kvs.GetRequest{
		Key:        key,
		ClientAddr: client.Address,
	}
	response := kvs.GetResponse{}
	err := client.rpcServers[0].Call("KVService.Get", &request, &response)
	if err != nil {
		log.Fatal(err)
	}

	// Store in cache
	cacheLine := &clientCacheLine{
		Value: response.Value,
	}
	client.cache.Store(key, cacheLine)

	return response.Value
}

func (client *Client) Put(key string, value string) {
	// Update cached entry if it exists
	if _, ok := client.cache.Load(key); ok {
		cacheLine := &clientCacheLine{
			Value: value,
		}
		client.cache.Store(key, cacheLine)
	}

	// Always Put to the server
	request := kvs.PutRequest{
		Key:   key,
		Value: value,
	}
	response := kvs.PutResponse{}
	err := client.rpcServers[0].Call("KVService.Put", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
}

func runClient(client *Client, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
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

	fmt.Printf("Client %d finished operations.\n", client.ID)
	fmt.Printf("Cache: %d hits, %d misses, %d updates", client.cache.hits, client.cache.misses, client.cache.updates)

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

	clientHost, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	port := "8080"
	clientAddr := strings.SplitN(clientHost, ".", 2)[0] + ":" + port
	clientID := 0

	client := NewClient(clientID, clientAddr, hosts)
	rpc.Register(&client.cache)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)

	go func(client *Client) {
		workload := kvs.NewWorkload(*workload, *theta)
		runClient(client, &done, workload, resultsCh)
	}(client)

	time.Sleep(time.Duration(*secs) * time.Second)
	done.Store(true)

	opsCompleted := <-resultsCh

	elapsed := time.Since(start)

	opsPerSec := float64(opsCompleted) / elapsed.Seconds()
	fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
}
