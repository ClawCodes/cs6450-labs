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
<img src="./charts and stats/commits_thread_scaling_4_nodes.png" width="500"/><img src="./charts and stats/throughput_thread_scaling_4_nodes.png" width="500"/><br><img src="./charts and stats/aborts_thread_scaling_4_nodes.png" width="500"/><br>
Number of client threads impact analysis for ...
</p>


#### **Figure 3:**
<p align=center>
<img alt="Cluster Size Impact Analysis, xfer, 10s duration, 128 threads" src="./charts%20and%20stats/scale_results_1759533996_plot.png" width="800"/><br>
Cluster size impact analysis for Bank Transfer workload, 10 second duration test, with 128 threads per client.
</p>


## Design

Our transactions were designed with a focus on guaranteeing serializability over higher performance. To that end, we chose to use strong strict two-phase locking (SS-2PL), two-phase commit (2PC), and a no-wait strategy to avoid deadlocks. We chose to use SS-2PL+2PC as it ensures serializability by preventing conflicts between concurrent transactions even though it may be out-performed by other transaction models. Additionally, to improve the scalability of the KVS, we implemented sharding by key-hashing. Our implementation relies on a transaction data structure, and important changes to the RPC message structure, and client and server code.

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

The payment workload works as follows:
1. 10 client accounts are initialized to $1000 dollars
2. Clients attempt to transfer $100 to the next client id modulo 10
3. Continue to do step 2 until time limit
4. Check balances add to $10,000 to verify semantics

To accomodate the payment workload, additional responsibilities and changes were made. Firstly, the first client is responsible for initializing all accounts before transactions may begin. Between transfers, clients periodically verify consistency by reading all account balances in a single transaction and ensuring the total amount all accounts is what it started as. Furthermore, we had to modify our deadlock and retry strategies.
- *Deterministic Lock Ordering* (Deadlock prevention) - when accessing multiple accounts, locks are always acquired in ascending account number order. This prevents circula wait conditions that acould cause deadlocks.
- *Exponential Backoff with Jittering* (Retry strategy) - when aborting a transaction, the wait time before retrying is doubled and then summed with a small random number. Wait times start at 20ms and are capped at 2s.

## Reproducability

### Dependencies
Running basic workloads requires `Go` and `python`. If use of the behaviour analysis scripts are desired, the following python packages are required: `matplotlib`, `pandas`, and `numpy`.

### Running Workloads
Workloads can be run using the `run-cluster.sh` script. It's usage is

```
./run-cluster.sh [num_servers] [num_clients] [serve_args] [client_args]

where client_args = [-workload W] [-theta θ] [-secs S] [-numThreads N]
      W is one of {YCSB-A, YCSB-B, xfer}
      θ is in [0,1]
      N is an integer
```
Our implementation currently has no server arguments so the empty string ("") is passed to be able to specify client arguments. `xfer` is used to specify the payments workload.

There are additional scripts that can be run to produce datasets beyond basic performance metrics of a single run. `theta_test.py`, `test_bank_transfer.py`, and `scaling.sh` all run workloads and produce datasets or perform consistency checks.

To analyze the impact of contention on our distributed KVS `theta_test.py` is invoked with `python3 theta_test.py`. The number of servers and number of client threads to use can be modified by editing `theta_test.py`. This script outputs a csv file that can we passed to `plot_theta_analysis.py` to generate figures like [Figure 1](#figure-1). Next, the payment workload can be verified by invoking `python3 test_bank_transfer.py`, which outputs the diagnostics to the standard out stream. Finally, the scaling characteristics can be tested using `scaling.sh`. It takes the longform arguments `--workload`, `--secs`, `--theta`, `--client-threads`, `--thread-step`, and `--num-servers`. Workload, secs, and theta are all forwarded directly to the client arguments, while the threads arguments specify the initial and step size of the base 2 logarithm of the number of client threads. `--num-servers` holds the number of server nodes constant for all the workloads. `plot_node_scaling.py` and `plot_scale_analysis.py` can be invoked similarly to the first plotting function to generate the scaling summary graphs as in [Figure 2](#figure-2a) and [3](#figure-3).

### m510 Hardware

| Attribute	| Value  |
|-----------|--------|
|Architecture|	x86_64
|CPU	| Intel Xeon D-1548 @ 2.00GHz
|CPU Cores & Threads |	8 Cores 16 Threads
|L1 Cache |	512KiB
|L2 Cache |	2048KiB
|L3 Cache |	12MB
|System Memory |	64GB


## Reflections

### Learnings

Much of our learnings came from the issues that the payment workload revealed. Three lessons that we took away are:
- *Lock ordering and retry strategies are critical* - Our initial implementation without deterministic lock acquisition experienced frequent deadlocks under the circular transfer pattern. Implementing ascending-order lock acquisition eliminated deadlocks entirely, while exponential backoff with jitter dramatically improved commit rates under contention.
- *Rigorous testing reveals subtle bugs* - The bank transfer workload's balance invariant check immediately exposed incorrect lock release logic that simpler workloads missed. This reinforced that transactional systems require strong consistency checks beyond basic functionality tests. Debugging distributed transactions across multiple servers proved time-consuming but essential - understanding interleaved operations and lock states demanded careful logging and systematic analysis.
- *Distributed coordination is nuanced* - Extending from single-server to multi-server transactions introduced numerous edge cases not apparent initially. Using the transactional store itself for client synchronization (via the init_complete flag) proved useful and avoided external dependencies.


### Successes

A major success was the design of our protocol and transaction data structure. These parts of our implementation were hardly changed after their initial design which allowed us to efficiently implement SS-2PL+2PC distributed transactions. This additionally lead to having more time for scaling and analysis experiments of our implementation. Writing scripts for the theta, cluster size, and client thread impact experiments massively sped up how quickly we could analyze the impact of small changes to our implementation. The error tracking and consistency checking we included for the payment workload was also essential in coming to a serializable implementation.

### Troubles

Initially, we only used a sync.Map data structure to represent our keys and lock holders. This appeared to work at first as we passed all the provided edge cases. However, concurrency  issues occurred when testing our bank balance workload. To resolve this, we included a RWMutex to protect the key-value service, acquiring a lock for the duration of each of its operations like Get, Put, Commit, and Abort.


### Further Improvement

Our design omits batch requests, which could provide throughput speed up of multiple orders of magnitude. Our design does not include any fault toleranace with regard to our sharding implementation. Although being fault tolerant is not very relevant for our use case, when scaling up to larger cluster sizes or long duration workloads, it would certainly have to be a consideration.

### Contributions

| Member | |
|--------|-|
|Chris   | > Client side transaction interface<br> > Adapted scripts for number of client thread scaling and server-to-client node ratio experiments<br> > Wrote plotting script for thread scaling experiments<br> > Ran client thread scaling experiments <br> > Admin - setup cluster
|Will    | > Created and implemented the first version of our server side SS-2PL+2PC system<br> > Added changes to proto.go to support transactions by introducing transaction identifiers along with Commit and Abort messages
|Kyle    | > Created scaling experiment and plotting scripts<br> > Ran cluster size impact experiments<br> > Created README write up - adapted [design](#design) and [learnings](#learnings) sections from group notes
|JJ      | > Payment workload testing<br> > Server-side modifications, optimizations, and debugging<br>&nbsp;&nbsp;&nbsp;- Introduced dedicated LockInfo struct<br>&nbsp;&nbsp;&nbsp;- Explicit separation of read and write lock tracking<br>&nbsp;&nbsp;&nbsp;- O(1) write lock checking instead of O(n) iteration<br>&nbsp;&nbsp;&nbsp;- Centralized lock acquisition logic in acquireReadLock and acquireWriteLock<br>&nbsp;&nbsp;&nbsp;- Improved releaseLocks implementation that correctly handles all lock types <br>&nbsp;&nbsp;&nbsp;- Fixed lock release timing to ensure atomicity<br>&nbsp;&nbsp;&nbsp;- Introduced lock upgrade conditions