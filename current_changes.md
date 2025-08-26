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

## 2. Distribute the work across two servers

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

## 3. Understanding so far

- Each server is an independent KV store (not a distributed system)
- Current strategy is simply horizontal scaling: node0-node2, node1-node3 
- Linearizability: Because it's not a real distributed system now. Each server uses mutex for its own kv store, so it's completely safe