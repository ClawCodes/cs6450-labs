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
	commits uint64
	aborts uint64
}

func (s *Stats) Sub(prev *Stats) Stats {
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	return r
}
type Operation struct {
	OpType string // "GET" or "PUT"
	Key    string
	Value  string // Empty for GET operations
}
type Tx struct {
	Txid uint64
	Operations []Operation

}
type KVService struct {
	sync.Mutex
	mp        map[string]string
	readSet  map[string]map[uint64]*uint64
	transactions map[uint64][]Operation
	stats     Stats
	prevStats Stats
	lastPrint time.Time
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.mp = make(map[string]string)
	kvs.readSet = make(map[string]map[uint64]*uint64) //maps keys to the set of transactions that hold locks for that key
	kvs.transactions = make(map[uint64][]Operation) //maps transactions to their operations. 
	kvs.lastPrint = time.Now()
	return kvs
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	kv.stats.gets++
	if value, found := kv.transactions[request.Txid]; !found {
		//transaction := Tx{Txid: request.Txid, Operations: make([]Operation, 0, 4)} 
		kv.transactions[request.Txid] = make([]Operation, 0, 4)
	}
	if keyLockHolders, found := kv.readSet[request.Key]; found { 
		for _, writeLockHolder := range keyLockHolders{ //This loop may be slow, could consider separate writeSet
			if writeLockHolder != nil{
				//Abort! Key is write locked
				response.Error = true
				return nil
			}
		}
		if _, found := keyLockHolders[request.Txid]; !found { //if the transaction is found, it already has a read lock. This shouldn't happen in practice due to client side readset
			kv.readSet[request.Key][request.Txid] = nil //key has no write locks, can aquire read lock
		}	
	}else{
		//Important: I'm worried about a possible race condition here, what if a write lock is acquired in between checking for lock holders and this line?
		kv.readSet[request.Key] = map[uint64]*uint64{request.Txid: nil} //key has no read/write locks, can aquire read lock
	}
	kv.transactions[request.Txid] = append(kv.transactions[request.Txid], Operation{
					OpType: "GET",
					Key:    request.key,
					Value:  "",
				})
	if value, found := kv.mp[request.Key]; found {
		response.Value = value
		response.Error = false
	}

	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	kv.Lock()
	defer kv.Unlock()

	kv.stats.puts++

	kv.mp[request.Key] = request.Value

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
