# Current Changes Summary

## 1. Added Parallel Clients

```go
// Original: 1 client/node → (5K * 2) ops/s  
// Current: 32 ~ 128 clients/node → can reach (360K * 2) ops/s
// /kvs/client/main.go

numClients := 128
	for i := 0; i < numClients; i++ {
		go func(clientId int) {
			workload := kvs.NewWorkload(*workload, *theta)
			runClient(clientId, host, &done, workload, resultsCh)
		}(i)
	}
	
// ...

opsCompleted := uint64(0)
for i := 0; i < numClients; i++ {
	opsCompleted += <-resultsCh
}

```

## 2. Distribute the work across two servers (Temporarily ignore)

```go

host := hosts[0] // comment out this

numClients := 128
	for i := 0; i < numClients; i++ {
		go func(clientId int) {
			host := hosts[clientId % len(hosts)]  // add this
			// modification in 1
		}(i)
	}

```

## 3. Added Batching Operations

```go
// /kvs/client/main.go - runClient function
const batchSize = 300

for !done.Load() {
	// "Collect" a batch of ops
	readKeys := make([]string, 0, batchSize)
	writeKeys := make([]string, 0, batchSize)
	writeValues := make([]string, 0, batchSize)

	for j := 0; j < batchSize; j++ {
		op := workload.Next()
		key := fmt.Sprintf("%d", op.Key)

		if op.IsRead {
			readKeys = append(readKeys, key)
		} else {
			writeKeys = append(writeKeys, key)
			writeValues = append(writeValues, value)
		}
	}

	if len(readKeys) > 0 {
		client.BatchGet(readKeys) // One RPC deals with multiple read ops
	}

	if len(writeKeys) > 0 {
		client.BatchPut(writeKeys, writeValues) // One RPC deals with multiple write ops
	}

	opsCompleted += uint64(batchSize)
}
```

## 4. Added Cache Implementation

```go
// /kvs/server/main.go - KVService struct
type KVService struct {
	mp         sync.Map
	cache      sync.Map    // Added cache layer
	cacheSize  int64       // Current cache size
	maxCache   int64       // Maximum cache capacity
	cacheHits  int64       // Cache hit counter
	cacheMiss  int64       // Cache miss counter
	// ... other fields
}

// Cache logic in Get/BatchGet operations
if value, ok := kv.cache.Load(key); ok {
	atomic.AddInt64(&kv.cacheHits, 1)
	response.Value = value.(string)
	return nil
}

// Cache update logic in Put operations
if _, exists := kv.cache.Load(key); exists {
	kv.cache.Store(key, value) // Update existing cached value
} else if atomic.LoadInt64(&kv.cacheSize) < kv.maxCache {
	kv.cache.Store(key, value)        // Store new key-value pair
	atomic.AddInt64(&kv.cacheSize, 1) // Increment cacheSize
}
```
