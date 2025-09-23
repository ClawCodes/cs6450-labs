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

func (client *Client) Get(key string) (string, error) {
	request := kvs.GetRequest{
		Key: key,
	}
	response := kvs.GetResponse{}
	err := client.rpcClient.Call("KVService.Get", &request, &response)
	if err != nil {
		log.Fatal(err)
		return "", err
	}

	return response.Value, nil
}

func (client *Client) Put(key string, value string) error {
	request := kvs.PutRequest{
		Key:   key,
		Value: value,
	}
	response := kvs.PutResponse{}
	err := client.rpcClient.Call("KVService.Put", &request, &response)
	if err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}

type Txn struct {
	availServers []*Client
	usedServers  *Set[*Client]
	id           *uint64
	writeSet     map[string]string // Keep write set cache to avoid unnecessary requests
}

func (txn *Txn) Begin(availableServers []*Client) {
	txn.availServers = availableServers
	id := randGen.Uint64()
	txn.id = &id
	txn.usedServers = NewSet[*Client]()
}

func (txn *Txn) Commit() error {
	if txn.id == nil {
		return errors.New("cannot commit a transaction that has not begun")
	}

	for server := range txn.usedServers.values {
		request := kvs.CommitRequest{
			*txn.id,
		}
		response := kvs.CommitResponse{}
		err := server.rpcClient.Call("KVService.Commit", &request, &response)
		if err != nil {
			log.Fatal(err)
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
			*txn.id,
		}
		response := kvs.AbortResponse{}
		err := server.rpcClient.Call("KVService.Abort", &request, &response)
		if err == nil {
			log.Fatal(err)
			return err
		}
	}
	return nil
}

func (txn *Txn) getServer(key string) *Client {
	server := serverFromKey(&key, txn.availServers)
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

	resp, err := txn.getServer(key).Get(key)
	if err != nil {
		return "", txn.Abort()
	}

	return resp, nil
}

func (txn *Txn) Put(key string, value string) error {
	if txn.id == nil {
		return errors.New("cannot call Put on a transaction that has not begun")
	}
	err := txn.getServer(key).Put(key, value)
	if err != nil {
		return txn.Abort()
	}
	txn.writeSet[key] = value
	return nil
}

func runClient(id int, servers []*Client, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {

	value := strings.Repeat("x", 128)
	const batchSize = 1024

	opsCompleted := uint64(0)

	for !done.Load() {
		for j := 0; j < batchSize; j++ {
			op := workload.Next()
			key := fmt.Sprintf("%d", op.Key)
			server := serverFromKey(&key, servers)
			if op.IsRead {
				server.Get(key)
			} else {
				server.Put(key, value)
			}
			opsCompleted++
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
	rand.Seed(time.Now().UnixNano())
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
