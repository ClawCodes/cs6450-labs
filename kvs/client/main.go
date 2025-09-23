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

func (client *Client) Get(key string) string {
	request := kvs.GetRequest{
		Key: key,
	}
	response := kvs.GetResponse{}
	err := client.rpcClient.Call("KVService.Get", &request, &response)
	if err != nil {
		log.Fatal(err)
	}

	return response.Value
}

func (client *Client) Put(key string, value string) {
	request := kvs.PutRequest{
		Key:   key,
		Value: value,
	}
	response := kvs.PutResponse{}
	err := client.rpcClient.Call("KVService.Put", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
}

type Txn struct {
	servers  []*Client
	id       *uint64
	state    string            // TODO: determine if this is required
	writeSet map[string]string // Keep write set cache to avoid unnecessary requests
}

func (txn *Txn) Begin() {
	id := randGen.Uint64()
	txn.id = &id
}

func (txn *Txn) Commit() error {
	if txn.id == nil {
		return errors.New("cannot commit a transaction that has not begun")
	}

	for _, server := range txn.servers {
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

	for _, server := range txn.servers {
		request := kvs.AbortRequest{
			*txn.id,
		}
		response := kvs.AbortResponse{}
		err := server.rpcClient.Call("KVService.Abort", &request, &response)
		if err != nil {
			log.Fatal(err)
		}
	}
	return nil
}

func (txn *Txn) Get(key string) error {
	// TODO: start here
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
