package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
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
		return errors.New("cannot commit a transaction that has not begun")
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
	cachedVal := txn.writeSet[key]
	if cachedVal != "" {
		return cachedVal, nil
	}

	resp, err := txn.getServer(key).Get(key, *txn.id)
	if err != nil {
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
	retry := 3
	for !done.Load() {
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
