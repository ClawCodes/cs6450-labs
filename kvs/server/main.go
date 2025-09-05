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
	mp           sync.Map
	stats        Stats
	prevStats    Stats
	lastPrint    time.Time
	clients      map[string]*rpc.Client
	clientCaches sync.Map // (key, sync.Map(clientID, {}) )
}

func NewKVService(clientAddrs []string) *KVService {
	// init kvs
	kvs := &KVService{}
	kvs.clients = make(map[string]*rpc.Client)
	kvs.lastPrint = time.Now()

	return kvs
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	kv.Lock()
	kv.stats.gets++
	kv.Unlock()

	// If key is in the store, return it and register which client has cached it
	if value, found := kv.mp.Load(request.Key); found {
		response.Value = value.(string)

		// Get set of clients that have cached the key, or create a new one
		clientsMap, _ := kv.clientCaches.LoadOrStore(request.Key, &sync.Map{})

		// Check if the requesting client is in the list and add if not
		if _, found := clientsMap.(*sync.Map).Load(request.ClientAddr); !found {
			// if not, check if the client has an rcp connection, add one if needed
			if _, exists := kv.clients[request.ClientAddr]; !exists {
				rpcCache, err := rpc.DialHTTP("tcp", request.ClientAddr)
				if err != nil {
					log.Fatal(err)
				}
				kv.clients[request.ClientAddr] = rpcCache
			}
			clientsMap.(*sync.Map).Store(request.ClientAddr, struct{}{})
		}
	}

	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	kv.Lock()
	kv.stats.puts++
	kv.Unlock()

	kv.mp.Store(request.Key, request.Value)

	// Send updated value to all clients that have cached this key
	if clientsMap, found := kv.clientCaches.Load(request.Key); found {
		updateRequest := kvs.UpdateRequest{
			Key:   request.Key,
			Value: request.Value,
		}
		updateResponse := kvs.UpdateResponse{}
		clientsMap.(*sync.Map).Range(func(key, _ any) bool {
			kv.clients[key.(string)].Call("KVCache.Update", &updateRequest, &updateResponse)
			return true
		})
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

type ClientList []string

func (c *ClientList) String() string {
	return strings.Join(*c, ",")
}

func (c *ClientList) Set(value string) error {
	*c = strings.Split(value, ",")
	return nil
}

func main() {
	clients := ClientList{}

	port := flag.String("port", "8080", "Port to run the server on")
	flag.Var(&clients, "clients", "Comma-separated list of client:ports to connect to")
	flag.Parse()

	kvs := NewKVService(clients)
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
