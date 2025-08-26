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
	"sync/atomic"

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
	// sync.Mutex
	// mp        map[string]string
	mp		  	sync.Map
	cache		sync.Map
	cacheSize   int64            
	maxCache    int64
	cacheHits   int64  
	cacheMiss   int64 
	stats     	Stats
	prevStats 	Stats
	lastPrint 	time.Time
	statsMutex 	sync.Mutex
}

func NewKVService() *KVService {
	kvs := &KVService{
		maxCache: 200000,
		lastPrint: time.Now(),
	}
	return kvs
}

func (kv *KVService) BatchGet(request *kvs.BatchGetRequest, response *kvs.BatchGetResponse) error {
	numKeys := len(request.Keys)
	response.Values = make([]string, numKeys)
	
	// 批次更新統計
	atomic.AddUint64(&kv.stats.gets, uint64(numKeys))
	
	for i, key := range request.Keys {
		// 檢查快取
		if value, found := kv.cache.Load(key); found {
			atomic.AddInt64(&kv.cacheHits, 1)
			response.Values[i] = value.(string)
			continue
		}
		
		atomic.AddInt64(&kv.cacheMiss, 1)
		
		// 檢查主存儲
		if value, found := kv.mp.Load(key); found {
			valueStr := value.(string)
			response.Values[i] = valueStr
			
			// 嘗試加入快取
			if atomic.LoadInt64(&kv.cacheSize) < kv.maxCache {
				kv.cache.Store(key, valueStr)
				atomic.AddInt64(&kv.cacheSize, 1)
			}
		}
	}
	
	return nil
}

func (kv *KVService) BatchPut(request *kvs.BatchPutRequest, response *kvs.BatchPutResponse) error {
	numKeys := len(request.Keys)
	
	// 批次更新統計
	atomic.AddUint64(&kv.stats.puts, uint64(numKeys))
	
	for i, key := range request.Keys {
		value := request.Values[i]
		
		// 更新主存儲
		kv.mp.Store(key, value)
		
		// 更新快取
		if _, exists := kv.cache.Load(key); exists {
			kv.cache.Store(key, value)
		} else if atomic.LoadInt64(&kv.cacheSize) < kv.maxCache {
			kv.cache.Store(key, value)
			atomic.AddInt64(&kv.cacheSize, 1)
		}
	}
	
	return nil
}

func (kv *KVService) Get(request *kvs.GetRequest, response *kvs.GetResponse) error {
	// kv.Lock()
	// defer kv.Unlock()

	// kv.stats.gets++
	atomic.AddUint64(&kv.stats.gets, 1)

	key := request.Key

	if value, found := kv.cache.Load(key); found {
		atomic.AddInt64(&kv.cacheHits, 1)
		response.Value = value.(string)
		return nil
	}

	atomic.AddInt64(&kv.cacheMiss, 1)

	// if value, found := kv.mp[request.Key]; found {
	// 	response.Value = value
	// }
	if value, found := kv.mp.Load(key); found {
		// response.Value = value.(string)
		valueStr := value.(string)
		response.Value = valueStr
		
		if atomic.LoadInt64(&kv.cacheSize) < kv.maxCache {
			kv.cache.Store(key, valueStr)
			atomic.AddInt64(&kv.cacheSize, 1)
		}

		return nil
	}

	return nil
}

func (kv *KVService) Put(request *kvs.PutRequest, response *kvs.PutResponse) error {
	// kv.Lock()
	// defer kv.Unlock()

	// kv.stats.puts++
	atomic.AddUint64(&kv.stats.puts, 1)

	key := request.Key
	value := request.Value

	// kv.mp[request.Key] = request.Value
	kv.mp.Store(key, value)
	if _, exists := kv.cache.Load(key); exists {
		kv.cache.Store(key, value)
	} else if atomic.LoadInt64(&kv.cacheSize) < kv.maxCache {
		kv.cache.Store(key, value)
		atomic.AddInt64(&kv.cacheSize, 1)
	}

	return nil
}

func (kv *KVService) printStats() {
	// kv.Lock()
	// stats := kv.stats
	// prevStats := kv.prevStats
	// kv.prevStats = stats
	// now := time.Now()
	// lastPrint := kv.lastPrint
	// kv.lastPrint = now
	// kv.Unlock()
	kv.statsMutex.Lock()

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

	// Stat of cache
	cacheSize := atomic.LoadInt64(&kv.cacheSize)

	fmt.Printf("get/s %0.2f\nput/s %0.2f\nops/s %0.2f\n\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS)
	
	// Print stats of cache
	// fmt.Printf("Cache: %d/%d (%.1f%% full)\n\n",
	// 	cacheSize, kv.maxCache,
	// 	float64(cacheSize)/float64(kv.maxCache)*100)
	
	hits := atomic.LoadInt64(&kv.cacheHits)
	miss := atomic.LoadInt64(&kv.cacheMiss)
	total := hits + miss
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	fmt.Printf("get/s %0.2f\nput/s %0.2f\nops/s %0.2f\n",
		float64(diff.gets)/deltaS,
		float64(diff.puts)/deltaS,
		float64(diff.gets+diff.puts)/deltaS)
	
	fmt.Printf("Cache: %d/%d (%.1f%% full), Hit rate: %.1f%%\n\n",
		cacheSize, kv.maxCache,
		float64(cacheSize)/float64(kv.maxCache)*100,
		hitRate)
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
