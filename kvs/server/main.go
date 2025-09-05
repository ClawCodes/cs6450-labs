package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

type Stats struct {
	puts uint64
	gets uint64
}

func (s *Stats) Sub(prev *Stats) Stats { // curr Stats take prev Stats as param, compute difference of ops
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	return r
}

type KVService struct {
	// sync.Mutex -> embedded field -> KVService automatically inherits the methods of sync.Mutex
	mp         sync.Map
	stats      Stats
	prevStats  Stats
	lastPrint  time.Time
	statsMutex sync.Mutex
}

func NewKVService() *KVService { // return a pointer that points to an instance of KVService type
	kvs := &KVService{
		lastPrint: time.Now(),
	}
	return kvs
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	atomic.AddUint64(&kv.stats.gets, 1)

	key := request.Key

	if value, found := kv.mp.Load(key); found {
		response.Value = value.(string)
	}
	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	atomic.AddUint64(&kv.stats.puts, 1)

	key, value := request.Key, request.Value
	kv.mp.Store(key, value)
	return nil
}

func (kv *KVService) BatchOp(request *kvs.BatchOpRequest, response *kvs.BatchOpResponse) error {
	numOps := len(request.Operations)
	response.Results = make([]string, numOps)

	for i, op := range request.Operations {
		if op.OpType == "GET" {
			atomic.AddUint64(&kv.stats.gets, 1)

			if value, found := kv.mp.Load(op.Key); found {
				response.Results[i] = value.(string)
			} else {
				response.Results[i] = ""
			}
		} else if op.OpType == "PUT" {
			atomic.AddUint64(&kv.stats.puts, 1)

			kv.mp.Store(op.Key, op.Value)
			response.Results[i] = ""
		}
	}
	return nil
}

func (kv *KVService) printStats() {
	// kv.Lock()						// Lock ALL kv struct, including updating of stats.gets/puts in Get/Put/BatchGet/BatchPut method
	// stats := kv.stats
	// prevStats := kv.prevStats
	// kv.prevStats = stats
	// now := time.Now()
	// lastPrint := kv.lastPrint
	// kv.lastPrint = now
	// kv.Unlock()
	kv.statsMutex.Lock() // Only lock prevStats / lastPrint

	currentGets := atomic.LoadUint64(&kv.stats.gets)
	currentPuts := atomic.LoadUint64(&kv.stats.puts)

	stats := Stats{gets: currentGets, puts: currentPuts}
	prevStats := kv.prevStats
	kv.prevStats = stats

	now := time.Now()
	lastPrint := kv.lastPrint
	kv.lastPrint = now

	kv.statsMutex.Unlock()

	diff := stats.Sub(&prevStats)
	deltaS := now.Sub(lastPrint).Seconds()

	fmt.Printf("get/s %0.2f\nput/s %0.2f\nops/s %0.2f\n\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS)
}

func main() {
	port := flag.String("port", "8080", "Port to run the server on") // param: (name, value, usage); return the pointer points to value
	flag.Parse()

	kvs := NewKVService()
	rpc.Register(kvs)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", fmt.Sprintf(":%v", *port)) // return (net.Listener, error)
	if e != nil {
		log.Fatal("listen error:", e)
	}

	fmt.Printf("Starting KVS server on :%s\n", *port)

	go func() {
		for {
			kvs.printStats() // Call printStats once per second
			time.Sleep(1 * time.Second)
		}
	}()

	http.Serve(l, nil)
}
