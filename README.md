# Distributed Transactions with 2PL+2PC

## Results

Our maximum throughput on a cluster of 4 nodes for the YCSB-A ($\theta=0.5$) was 22.6 thousand commits per second and 68.1 thousand operations per second using 2 server nodes, 2 client nodes, and 128 threads per client.

We analyzed our implementation's behaviour with regards to key contention and scaling cluster size and client load. Summarily, increased key contention - increased $\theta$ (Zipfian Skew Parameter) - increased the abort rate from 0% at $\theta=0.75$ up to 6% at $\theta=0.99$. The increase in aborts is reflected in the decreased commit and transaction success rates. [Figure 1](#figure-1) gives a comprehensive view of the changes in performance over the the range of possible $\theta$ values. With regards to scaling analysis, we tested the impact of cluster size and the impact of the number of client threads. [Figure 2a](#figure-2a) illustrates the impact of cluster size on performance metrics. We tested all combinations of the number of client and server nodes totalling 2 to 8 nodes. Using 128 threads per client, the optimal server to client node ratio was 1:1 with ratios higher or lower causes performance losses. A visualization of the impact of the number of client threads can be found in [Figure 2b](#figure-2b). Our tests showed that the relationship between transactions per second and operations per second is non-monotonic, and that increasing the nummber of threads too much causes the total transaction rate to drop and the aborts rate to massively increase.

The performance characteristics of the payments workload was also analyzed, and is visualized in [Figure 3](#figure-3). The total transaction rate for the 2 server and 2 client was reduced by a factor of 10 compared to the YCSB-A, $\theta=0.5$ test, while the total operations rate was reduced by a factor of about 3. Most improtantly, the transaction success rate was much lower than in the YCSB workloads, even at $\theta=0.99$. Furthermore, it appears we have a bug when there are 3 (and potentially also for 4) client nodes as the commits per second drops to double digits, but not entirely zero. Preventing deadlock and getting reasonable commit rates under high contention required special attention.

#### **Figure 1:**
<p align=center> 
<img src="./charts and stats/theta_analysis_1759536862_analysis.png" alt="Theta Parameter Impact Analysis, 2 server and 2 client nodes, YCSB, 10s duration, 128 threads" width="800"/><br>
Contention analysis for 2 server and 2 client nodes, 10 second duration test, with 128 threads per client.
</p>


#### **Figure 2a:**
<p align=center> 
<img alt="Cluster Size Impact Analysis, YCSB-A, theta=0.5, 10s duration, 128 threads" src="./charts%20and%20stats/scale_results_1759533071_plot.png" width="800"/><br>
Cluster size impact analysis for YCSB-A workload with theta=0.5, 10 second duration test, with 128 threads per client.
</p>

#### **Figure 2b:**
<p align=center>
<img src="./charts and stats/transactions-client-thread-scaling.png" width="500"/><img src="./charts and stats/operations-client-thread-scaling.png" width="500"/><br><img src="./charts and stats/aborts-client-thread-scaling.png" width="500"/><br>
Number of client threads impact analysis for YCSB-A workload with theta=0.5 across varying client-server combinations in an 8 node cluster
</p>


#### **Figure 3:**
<p align=center>
<img alt="Cluster Size Impact Analysis, xfer, 10s duration, 128 threads" src="./charts%20and%20stats/scale_results_1759533996_plot.png" width="800"/><br>
Cluster size impact analysis for Bank Transfer workload, 10 second duration test, with 128 threads per client.
</p>


## Design

Our transactions were designed to use two-phase locking (2PL), two-phase commit (2PC), and a no-wait strategy to avoid deadlocks. We chose to use 2PL+2PC as it ensures serializability by preventing conflicts between concurrent transactions. Additionally, to improve the scalability of the KVS, we implemented sharding by key-hashing. In our implementation, we introducued a transaction data structure, and made important changes to the RPC message structure, and client and server code.

We introduced a Txn struct to encapsulate transaction state. Each transaction maintains four key fields: a list of all available servers, a set tracking which servers have been contacted during the transaction, a unique transaction ID generated using a random number generator, and a local write set cache implemented as a map from keys to values.

All RPC messages were extended to include transaction identifiers. The GetRequest and PutRequest messages now carry a Txid field to associate operations with their transactions. The CommitRequest includes an additional Lead field to prevent double-counting in statistics - only the first participant server increments the commit counter. The AbortRequest simply carries the transaction ID to identify which transaction to abort.

In our design, clients are now responsible for the following:
- *Performing retries* if locks cannot be immediately acquired (No-Wait deadlock avoidance)
- *Server selection via sharding* using the FNV-32a hash. The getServer method not only selects the appropriate server but also automatically adds it to the transaction's usedServers set, tracking which servers participate in the transaction for later commit/abort coordination.
- *Write set caching*, which ensures 'Read Your Writes' semantics. When a transaction performs a Put operation, the key-value pair is cached locally. Subsequent Get operations first check this cache before sending RPCs to servers. This is essential because servers buffer writes until commit time - without the cache, a transaction would read stale values for keys it has just written.
- *Get and Put operations* - Get and Put operations check that a transaction has begun, then route requests to the appropriate server via getServer. For Get, the write set is checked first. For Put, successful operations update the write set. Both methods distinguish between retryable lock conflicts and fatal errors - lock conflicts return errors without aborting, while other errors trigger automatic abort.
- *Commits* - Commit iterates through all servers in usedServers, sending commit requests to each. The first server receives Lead=true for accurate statistics tracking. Similarly, Abort contacts all participating servers to release locks and discard buffered writes.
- *Workload Execution* - Each transaction consists of exactly three operations drawn from the workload generator, as required by the assignment. Failed transactions are retried in their entirety before moving to the next transaction, ensuring no operations are skipped or reordered. Transactions may be retried up to 3 times before being dropped entirely.

The responsibilities of the server in our design are the following:
- *Lock Management* - each key maintains a `LockInfo` structure that trakcs either a set of transaction IDs with read locks, or a single transaction with a write lock. Read locks can be upgraded to write locks if the transaction requesting the write lock is the only transaction with a read lock. All locks are released atomically after the commit RPC is received. Using a fine-grained locking mechanism allows for greater parallelism and therefore higher throughput.
- *Deferred Writes* - servers store a key-value implemented as a `sync.Map` and a transaction log mapping transaction IDs to lists of operations. Writes are applied from the operation list to the key-value store during a commit. This is done to eliminate rollback mechanisms; aborted transactions have their logged operations discarded.
- *Transaction State Tracking* - active transactions store an operation log that captures the list of operations with their type, key, and value. This log serves to record what operations occurred for commit-time application, and to track which keys were accessed for lock release.
- *Handling RPC Requests*
  - *Get* - acquires a read lock, records the operation in the transaction log, then returns the current value from the key-value store. If lock acquisition fails, it immediately releases all locks held by that transaction and returns an error.
  - *Put* - acquires a write lock and records the operation. Recall changes are not applies to the key-value mapping until commit time.
  - *Commit* - applies all buffered put operations, updates statistics if the server was the first contacted in the transaction, then releases locks.
  - *Abort* - Abort handler simply releases locks and discards buffered operations.
Additionally, the entire service is protected by a single RWMutex in the KVService struct. Each RPC handler acquires the write lock for its duration. While this limits concurrency compared to finer-grained locking, it greatly simplifies the implementation and ensures correctness. Given the requirement of the assignment, we prioritized correctness over maximum performance. As for statistics tracking, commits, aborts, gets, and puts are tracked via atomic operations. 

### Additions for Payment Workload


## Reproducability



## Reflections

### Learnings

### Successes

### Troubles

### Further Improvement

### Contributions