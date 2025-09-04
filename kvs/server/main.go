package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Stats struct {
	puts uint64
	gets uint64
}

func (s *Stats) Sub(prev *Stats) Stats {
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	return r
}

type KVService struct {
	sync.Mutex
	mp           map[string]string
	stats        Stats
	prevStats    Stats
	lastPrint    time.Time
	clientCaches map[string]map[string]*rpc.Client // key -> set of clientIDs that have it cached
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.mp = make(map[string]string)
	kvs.clientCaches = make(map[string]map[string]*rpc.Client)
	kvs.lastPrint = time.Now()
	return kvs
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	kv.Lock()
	defer kv.Unlock()

	kv.stats.gets++

	if value, found := kv.mp[request.Key]; found {
		response.Value = value
	}

	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	kv.Lock()
	defer kv.Unlock()

	kv.stats.puts++

	kv.mp[request.Key] = request.Value

	// Send invalidations to all clients that have this key cached
	if clients, exists := kv.clientCaches[request.Key]; exists {
		for clientID, rpcClient := range clients {
			invalidateReq := &kvs.InvalidationRequest{Key: request.Key}
			invalidateResp := &kvs.InvalidationResponse{}
			// Do async RPC call to avoid blocking
			go func(client *rpc.Client, cid string) {
				err := client.Call("Client.Invalidate", invalidateReq, invalidateResp)
				if err != nil {
					// Client might be dead, remove it from our cache
					kv.Lock()
					delete(kv.clientCaches[request.Key], cid)
					if len(kv.clientCaches[request.Key]) == 0 {
						delete(kv.clientCaches, request.Key)
					}
					kv.Unlock()
				}
			}(rpcClient, clientID)
		}
	}

	return nil
}

func (kv *KVService) RegisterCache(request *kvs.RegisterCacheRequest, response *kvs.RegisterCacheResponse) error {
	kv.Lock()
	defer kv.Unlock()

	// Create a new RPC client for the registering client if we don't have one
	if _, exists := kv.clientCaches[request.Key]; !exists {
		kv.clientCaches[request.Key] = make(map[string]*rpc.Client)
	}

	// Store the client's RPC connection for future invalidations
	if client, err := rpc.DialHTTP("tcp", request.ClientID); err == nil {
		kv.clientCaches[request.Key][request.ClientID] = client
	} else {
		return err
	}

	return nil
}

func (kv *KVService) printStats() {
	kv.Lock()
	stats := kv.stats
	prevStats := kv.prevStats
	kv.prevStats = stats
	now := time.Now()
	lastPrint := kv.lastPrint
	kv.lastPrint = now
	kv.Unlock()

	diff := stats.Sub(&prevStats)
	deltaS := now.Sub(lastPrint).Seconds()

	fmt.Printf("get/s %0.2f\nput/s %0.2f\nops/s %0.2f\n\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS)
}

func main() {
	port := flag.String("port", "8080", "Port to run the server on")
	flag.Parse()

	kvs := NewKVService()
	rpc.Register(kvs)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", fmt.Sprintf(":%v", *port))
	if e != nil {
		log.Fatal("listen error:", e)
	}

	fmt.Printf("Starting KVS server on :%s\n", *port)

	go func() {
		for {
			kvs.printStats()
			time.Sleep(1 * time.Second)
		}
	}()

	http.Serve(l, nil)
}
