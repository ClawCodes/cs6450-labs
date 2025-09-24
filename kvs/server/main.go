package main

import (
	"errors"
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
	puts    uint64
	gets    uint64
	commits uint64
	aborts  uint64
}

func (s *Stats) Sub(prev *Stats) Stats {
	r := Stats{}
	r.puts = s.puts - prev.puts
	r.gets = s.gets - prev.gets
	r.commits = s.commits - prev.commits
	r.aborts = s.aborts - prev.aborts
	return r
}

type Operation struct {
	OpType string // "GET" or "PUT"
	Key    string
	Value  string // Empty for GET operations
}
type KVService struct {
	sync.Mutex
	mp           sync.Map //map[string]string
	readSet      sync.Map //map[string]map[uint64]*uint64 //maps keys to the set of transactions that hold locks for that key
	transactions sync.Map //map[uint64][]Operation  //maps transactions to their operations.
	stats        Stats
	prevStats    Stats
	lastPrint    time.Time
}

func NewKVService() *KVService {
	kvs := &KVService{}
	kvs.lastPrint = time.Now()
	return kvs
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	//Add to transaction map if it hasn't been added yet
	if _, found := kv.transactions.Load(request.Txid); !found {
		kv.transactions.Store(request.Txid, make([]Operation, 0, 4)) //Transaction should only have up to 4 operation, but it can grow if needed
	}

	//Looks for key holders for the requests key, acquire a shared lock if there are no write locks or no locks at all
	if keyLockHolders, found := kv.readSet.LoadOrStore(request.Key, map[uint64]*uint64{request.Txid: nil}); found {
		if keyLockHolders, ok := keyLockHolders.(map[uint64]*uint64); ok {
			for _, writeLockHolder := range keyLockHolders { //This loop may be slow, could consider separate writeSet
				if writeLockHolder != nil {
					//Abort: Key is write locked
					kv.dropLocks(request.Txid)
					//dropLocks(request.Txid)
					atomic.AddUint64(&kv.stats.aborts, 1)
					return errors.New("Abort: Cannot acquire Read Lock, key is currently write locked")
				}
			}
			if _, found := keyLockHolders[request.Txid]; !found { //if the transaction is found, it already has a read lock. This shouldn't happen in practice due to client side readset
				keyLockHolders[request.Txid] = nil //key has no write locks, so acquire read lock. Pointer to writer is nil
			}
		}
	}
	var ops, _ = kv.transactions.Load(request.Txid)
	if ops, ok := ops.([]Operation); ok {
		ops = append(ops, Operation{
			OpType: "GET",
			Key:    request.Key,
		})
	}

	kv.transactions.Store(request.Txid, ops)

	if value, found := kv.mp.Load(request.Key); found {
		response.Value = value.(string)
		atomic.AddUint64(&kv.stats.gets, 1)
	}
	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	//Add to transaction map if it hasn't been added yet
	if _, found := kv.transactions.Load(request.Txid); !found {
		kv.transactions.Store(request.Txid, make([]Operation, 0, 4))
	}

	//Looks for key holders for the requests key, acquire a write lock if there are no locks at all
	if keyLockHolders, found := kv.readSet.LoadOrStore(request.Key, map[uint64]*uint64{request.Txid: &request.Txid}); found {
		if keyLockHolders, ok := keyLockHolders.(map[uint64]*uint64); ok {
			if _, found := keyLockHolders[request.Txid]; !found && len(keyLockHolders) > 1 { //if there is a lock holder for the key, it must belong to the same transaction or it has to abort
				keyLockHolders[request.Txid] = &request.Txid //key has no read/write locks, so acquire write lock. Pointer to writer is non-nil
			} else {
				//if there are key holders that don't belong to this transaction, a write lock cannot be acquired
				//Abort
				kv.dropLocks(request.Txid)
				//dropLocks(request.Txid)
				atomic.AddUint64(&kv.stats.aborts, 1)
				return errors.New("Abort: Cannot acquire Write Lock, key is currently locked")
			}
		}
	}
	//Buffer the put request, it will be completed in commit phase
	var ops, _ = kv.transactions.Load(request.Txid)
	if ops, ok := ops.([]Operation); ok {
		ops = append(ops, Operation{
			OpType: "PUT",
			Key:    request.Key,
			Value:  request.Value,
		})
	}

	kv.transactions.Store(request.Txid, ops)

	return nil
}

// Installs all put requests from the transaction, then drops related locks and removes the transaction
func (kv *KVService) Commit(request *kvs.CommitRequest, response *kvs.CommitResponse) error {
	if operations, found := kv.transactions.Load(request.Txid); found {
		if operations, ok := operations.([]Operation); ok {
			for _, op := range operations {
				if op.OpType == "PUT" {
					atomic.AddUint64(&kv.stats.puts, 1)
					kv.mp.Store(op.Key, op.Value)
				}
			}
			atomic.AddUint64(&kv.stats.commits, 1)
			//need to release locks after ALL changes applied
			kv.dropLocks(request.Txid)
			//dropLocks(request.Txid)
		}
	}
	return nil
}

// Handler/Wrapper for Aborts from client
func (kv *KVService) Abort(request *kvs.AbortRequest, response *kvs.AbortResponse) error {
	kv.dropLocks(request.Txid)
	atomic.AddUint64(&kv.stats.aborts, 1)
	return nil
}

// Closes a transaction by deleting all locks it holds, then removes the transaction from the map
func (kv *KVService) dropLocks(Txid uint64) {
	if operations, found := kv.transactions.Load(Txid); found {
		if operations, ok := operations.([]Operation); ok {
			for _, op := range operations {
				if keyLockHolders, found := kv.readSet.Load(op.Key); found {
					if keyLockHolders, ok := keyLockHolders.(map[uint64]*uint64); ok {
						if _, found := keyLockHolders[Txid]; found {
							delete(keyLockHolders, Txid)
						}
					}
				}
			}
			kv.transactions.Delete(Txid)
		}
	}
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
