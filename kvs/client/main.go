package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

var randGen = rand.New(rand.NewSource(time.Now().UnixNano()))

type Client struct {
	rpcClient *rpc.Client
}

func Dial(addr string) *Client {
	rpcClient, err := rpc.DialHTTP("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	return &Client{rpcClient}
}

func (client *Client) Get(key string, txid uint64) (string, error) {
	request := kvs.GetRequest{
		Key:  key,
		Txid: txid,
	}
	response := kvs.GetResponse{}
	err := client.rpcClient.Call("KVService.Get", &request, &response)
	if err != nil {
		log.Printf("Error during Client.Get: %v", err)
		return "", err
	}

	return response.Value, nil
}

func (client *Client) Put(key string, value string, txid uint64) error {
	request := kvs.PutRequest{
		Key:   key,
		Value: value,
		Txid:  txid,
	}
	response := kvs.PutResponse{}
	err := client.rpcClient.Call("KVService.Put", &request, &response)
	if err != nil {
		log.Printf("Error during Client.Put: %v", err)
		return err
	}
	return nil
}

type Txn struct {
	allServers  []*Client
	usedServers *Set[*Client]
	id          *uint64
	writeSet    map[string]string // Keep write set cache to avoid unnecessary requests
}

func (txn *Txn) Begin(availableServers []*Client) {
	txn.allServers = availableServers
	id := randGen.Uint64()
	txn.id = &id
	txn.usedServers = NewSet[*Client]()
	txn.writeSet = make(map[string]string)
}

func (txn *Txn) Commit() error {
	if txn.id == nil {
		return errors.New("cannot commit a transaction that has not begun")
	}

	lead := true // Make first request the lead for server-side logging
	for server := range txn.usedServers.values {
		request := kvs.CommitRequest{
			Txid: *txn.id,
			Lead: lead,
		}
		lead = false
		response := kvs.CommitResponse{}
		err := server.rpcClient.Call("KVService.Commit", &request, &response)
		if err != nil {
			log.Printf("Error during Commit: %v", err)
		}
	}
	return nil
}

func (txn *Txn) Abort() error {
	if txn.id == nil {
		return errors.New("cannot abort a transaction that has not begun")
	}

	for server := range txn.usedServers.values {
		request := kvs.AbortRequest{
			Txid: *txn.id,
		}
		response := kvs.AbortResponse{}
		err := server.rpcClient.Call("KVService.Abort", &request, &response)
		if err != nil {
			log.Printf("Error during Abort: %v", err)
			return err
		}
	}
	return nil
}

func (txn *Txn) getServer(key string) *Client {
	server := serverFromKey(&key, txn.allServers)
	txn.usedServers.Add(server)
	return server
}

func (txn *Txn) Get(key string) (string, error) {
	if txn.id == nil {
		return "", errors.New("cannot call Get on a transaction that has not begun")
	}

	// Check writeSet cache first - use exists check instead of empty string check
	cachedVal, exists := txn.writeSet[key]
	if exists {
		return cachedVal, nil
	}

	resp, err := txn.getServer(key).Get(key, *txn.id)
	if err != nil {
		// Check if this is a lock conflict (retryable) or real error (fatal)
		if strings.Contains(err.Error(), "Cannot acquire") || strings.Contains(err.Error(), "Abort:") {
			// Lock conflict - let caller handle retry
			return "", fmt.Errorf("lock conflict: %w", err)
		}
		// Real error - abort transaction
		_ = txn.Abort()
		return "", fmt.Errorf("server-side error raised: %w", err)
	}

	return resp, nil
}

func (txn *Txn) Put(key string, value string) error {
	if txn.id == nil {
		return errors.New("cannot call Put on a transaction that has not begun")
	}
	err := txn.getServer(key).Put(key, value, *txn.id)
	if err != nil {
		// Check if this is a lock conflict (retryable) or real error (fatal)
		if strings.Contains(err.Error(), "Cannot acquire") || strings.Contains(err.Error(), "Abort:") {
			// Lock conflict - let caller handle retry
			return fmt.Errorf("lock conflict: %w", err)
		}
		// Real error - abort transaction
		_ = txn.Abort()
		return fmt.Errorf("server-side error raised: %w", err)
	}
	txn.writeSet[key] = value
	return nil
}

func executeTxn(txn *Txn, workload *kvs.Workload) (uint64, error) {
	value := strings.Repeat("x", 128)
	opsCompleted := uint64(0)
	for j := 0; j < 3; j++ { // 3 iterations for requirement of Transactions including 3 Ops only
		op := workload.Next()
		key := fmt.Sprintf("%d", op.Key)
		var err error
		if op.IsRead {
			_, err = txn.Get(key)
		} else {
			err = txn.Put(key, value)
		}
		if err != nil {
			return opsCompleted, err
		}
		opsCompleted++
	}
	return opsCompleted, nil
}

func runClient(id int, servers []*Client, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
	opsCompleted := uint64(0)
	var err error
	for !done.Load() {
		retry := 3
		for retry > 0 {
			txn := Txn{}
			txn.Begin(servers)
			opsCompleted, err = executeTxn(&txn, workload)
			if err != nil {
				log.Printf("Error raised during transaction: %v", err)
				retry--
				continue
			}
			err := txn.Commit()
			if err != nil {
				log.Printf("Error raised during commit: %v", err)
			}
			break // Successfully completed transaction Ops
		}
	}

	fmt.Printf("Client %d finished operations.\n", id)
	resultsCh <- opsCompleted
}

func runTransferClient(clientId int, servers []*Client, done *atomic.Bool, resultsCh chan<- uint64) {
	opsCompleted := uint64(0)

	// Initialize accounts if clientId == 0
	if clientId == 0 {
		initAccounts(servers)
		log.Printf("Client %d initialized bank accounts", clientId)

		// Signal that initialization is complete by setting a flag
		for retry := 0; retry < 10; retry++ {
			txn := Txn{}
			txn.Begin(servers)
			err := txn.Put("init_complete", "true")
			if err != nil {
				txn.Abort()
				time.Sleep(100 * time.Millisecond)
				continue
			}
			err = txn.Commit()
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf("Client %d signaled initialization complete", clientId)
	}

	// All clients wait for initialization to complete
	for {
		txn := Txn{}
		txn.Begin(servers)
		initFlag, err := txn.Get("init_complete")
		if err != nil {
			txn.Abort()
		} else {
			err = txn.Commit()
			if err == nil && initFlag == "true" {
				log.Printf("Client %d detected initialization complete, starting transfers", clientId)
				break
			}
		}
		time.Sleep(100 * time.Millisecond) // Wait before checking again
	}

	transferCount := 0
	for !done.Load() {
		// Perform transfer every few iterations
		if transferCount%5 == 0 {
			err := performTransfer(clientId, servers)
			if err != nil {
				log.Printf("Transfer failed: %v", err)
			} else {
				opsCompleted++
			}
		}

		// Check balance integrity
		err := checkTotalBalance(servers)
		if err != nil {
			log.Printf("Balance check failed: %v", err)
		} else {
			opsCompleted++
		}

		transferCount++
		time.Sleep(100 * time.Millisecond) // Slow down to observe behavior
	}

	fmt.Printf("Transfer client %d finished. Completed %d operations.\n", clientId, opsCompleted)
	resultsCh <- opsCompleted
}

func initAccounts(servers []*Client) {
	for i := 0; i < 10; i++ {
		maxRetries := 10
		for retry := 0; retry < maxRetries; retry++ {
			txn := Txn{}
			txn.Begin(servers)

			key := fmt.Sprintf("account_%d", i)
			err := txn.Put(key, "1000") // $1000 initial balance
			if err != nil {
				txn.Abort()
				log.Printf("Error initializing account %d: %v", i, err)
				if retry < maxRetries-1 {
					backoffTime := time.Duration(50*(1<<uint(retry))) * time.Millisecond
					jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
					time.Sleep(backoffTime + jitter)
				}
				continue
			}

			err = txn.Commit()
			if err != nil {
				log.Printf("Error committing account %d initialization: %v", i, err)
				if retry < maxRetries-1 {
					backoffTime := time.Duration(50*(1<<uint(retry))) * time.Millisecond
					jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
					time.Sleep(backoffTime + jitter)
				}
				continue
			}
			break // Successfully initialized account
		}
	}
}

func performTransfer(clientId int, servers []*Client) error {
	src := clientId
	dst := (clientId + 1) % 10

	maxRetries := 15
	baseDelay := 20 // Base delay in milliseconds

	for retry := 0; retry < maxRetries; retry++ {
		txn := Txn{}
		txn.Begin(servers)

		// Lock ordering: Always access accounts in ascending order to prevent deadlock
		accounts := []int{src, dst}
		sort.Ints(accounts)

		// Read all account balances in order
		balances := make(map[int]int)
		readSuccess := true

		for _, account := range accounts {
			key := fmt.Sprintf("account_%d", account)
			balStr, err := txn.Get(key)
			if err != nil {
				txn.Abort()
				if strings.Contains(err.Error(), "lock conflict") {
					readSuccess = false
					break
				}
				return fmt.Errorf("error reading account %d: %w", account, err)
			}

			bal := 0
			if balStr != "" {
				bal, err = strconv.Atoi(balStr)
				if err != nil {
					txn.Abort()
					return fmt.Errorf("error parsing balance for account %d: %w", account, err)
				}
			}
			balances[account] = bal
		}

		if !readSuccess {
			// Apply exponential backoff for lock conflicts
			if retry < maxRetries-1 {
				backoffTime := time.Duration(baseDelay*(1<<uint(retry))) * time.Millisecond
				if backoffTime > 2*time.Second {
					backoffTime = 2 * time.Second // Cap at 2 seconds
				}
				jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
				time.Sleep(backoffTime + jitter)
			}
			continue
		}

		// Check if sufficient funds
		if balances[src] < 100 {
			txn.Abort()
			return fmt.Errorf("insufficient funds in account %d: %d", src, balances[src])
		}

		// Update balances in the same order
		updateSuccess := true
		for _, account := range accounts {
			key := fmt.Sprintf("account_%d", account)
			var newBalance int

			if account == src {
				newBalance = balances[account] - 100
			} else { // account == dst
				newBalance = balances[account] + 100
			}

			err := txn.Put(key, fmt.Sprintf("%d", newBalance))
			if err != nil {
				txn.Abort()
				if strings.Contains(err.Error(), "lock conflict") {
					updateSuccess = false
					break
				}
				return fmt.Errorf("error updating account %d: %w", account, err)
			}
		}

		if !updateSuccess {
			// Apply exponential backoff for lock conflicts
			if retry < maxRetries-1 {
				backoffTime := time.Duration(baseDelay*(1<<uint(retry))) * time.Millisecond
				if backoffTime > 2*time.Second {
					backoffTime = 2 * time.Second // Cap at 2 seconds
				}
				jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
				time.Sleep(backoffTime + jitter)
			}
			continue
		}

		err := txn.Commit()
		if err != nil {
			log.Printf("Error committing transfer: %v", err)
			// Apply exponential backoff
			if retry < maxRetries-1 {
				backoffTime := time.Duration(baseDelay*(1<<uint(retry))) * time.Millisecond
				if backoffTime > 2*time.Second {
					backoffTime = 2 * time.Second // Cap at 2 seconds
				}
				jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
				time.Sleep(backoffTime + jitter)
			}
			continue
		}

		log.Printf("Transfer successful: %d -> %d ($100)", src, dst)
		return nil
	}

	return fmt.Errorf("transfer failed after %d retries", maxRetries)
}

func checkTotalBalance(servers []*Client) error {
	maxRetries := 10
	for retry := 0; retry < maxRetries; retry++ {
		txn := Txn{}
		txn.Begin(servers)

		total := 0
		balances := make([]int, 10)
		readSuccess := true

		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("account_%d", i)
			balStr, err := txn.Get(key)
			if err != nil {
				txn.Abort()
				if strings.Contains(err.Error(), "lock conflict") {
					readSuccess = false
					break
				}
				log.Printf("Error getting balance for account %d: %v", i, err)
				return err
			}

			bal := 0
			if balStr != "" {
				bal, err = strconv.Atoi(balStr)
				if err != nil {
					txn.Abort()
					log.Printf("Error parsing balance for account %d: %v", i, err)
					return err
				}
			}

			balances[i] = bal
			total += bal
		}

		if !readSuccess {
			// Apply exponential backoff
			backoffTime := time.Duration(50*(1<<uint(retry))) * time.Millisecond
			jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
			time.Sleep(backoffTime + jitter)
			continue
		}

		err := txn.Commit()
		if err != nil {
			log.Printf("Error committing balance check: %v", err)
			// Apply exponential backoff
			backoffTime := time.Duration(50*(1<<uint(retry))) * time.Millisecond
			jitter := time.Duration(randGen.Intn(int(backoffTime / 2)))
			time.Sleep(backoffTime + jitter)
			continue
		}

		if total != 10000 {
			log.Printf("INTEGRITY VIOLATION: Total balance is %d, expected 10000", total)
			log.Printf("Account balances: %v", balances)
			return fmt.Errorf("integrity violation: total=%d", total)
		} else {
			log.Printf("Balance check passed: total=%d, balances=%v", total, balances)
		}

		return nil
	}

	return fmt.Errorf("balance check failed after retries")
}

type HostList []string

func (h *HostList) String() string {
	return strings.Join(*h, ",")
}

func (h *HostList) Set(value string) error {
	*h = strings.Split(value, ",")
	return nil
}

func main() {
	hosts := HostList{}

	flag.Var(&hosts, "hosts", "Comma-separated list of host:ports to connect to")
	theta := flag.Float64("theta", 0.99, "Zipfian distribution skew parameter")
	workload := flag.String("workload", "YCSB-B", "Workload type (YCSB-A, YCSB-B, YCSB-C)")
	secs := flag.Int("secs", 30, "Duration in seconds for each client to run")
	flag.Parse()

	if len(hosts) == 0 {
		hosts = append(hosts, "localhost:8080")
	}

	fmt.Printf(
		"hosts %v\n"+
			"theta %.2f\n"+
			"workload %s\n"+
			"secs %d\n",
		hosts, *theta, *workload, *secs,
	)

	start := time.Now()

	done := atomic.Bool{}
	resultsCh := make(chan uint64)

	connections := dialHosts(hosts)

	if *workload == "xfer" {
		// Run transfer workload with multiple clients
		numClients := 10
		for i := 0; i < numClients; i++ {
			go func(clientId int) {
				runTransferClient(clientId, connections, &done, resultsCh)
			}(i)
		}

		time.Sleep(time.Duration(*secs) * time.Second)
		done.Store(true)

		// Collect results from all clients
		totalOps := uint64(0)
		for i := 0; i < numClients; i++ {
			totalOps += <-resultsCh
		}

		elapsed := time.Since(start)
		opsPerSec := float64(totalOps) / elapsed.Seconds()
		fmt.Printf("transfer throughput %.2f ops/s\n", opsPerSec)
	} else {
		// Run normal YCSB workload
		clientId := 0
		go func(clientId int) {
			workload := kvs.NewWorkload(*workload, *theta)
			runClient(clientId, connections, &done, workload, resultsCh)
		}(clientId)

		time.Sleep(time.Duration(*secs) * time.Second)
		done.Store(true)

		opsCompleted := <-resultsCh

		elapsed := time.Since(start)

		opsPerSec := float64(opsCompleted) / elapsed.Seconds()
		fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
	}
}
