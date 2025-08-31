package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"net/rpc"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rstutsman/cs6450-labs/kvs"
)

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

func (client *Client) BatchOp(operations []kvs.Operation) []string {
	request := kvs.BatchOpRequest{Operations: operations}
	response := kvs.BatchOpResponse{}
	err := client.rpcClient.Call("KVService.BatchOp", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
	return response.Results
}
func serverFromKey(key *string, servers *[]*Client) Client {
	h := fnv.New32a()
	h.Write([]byte(*key))
	idx := int(h.Sum32()) % len(*servers)
	return *(*servers)[idx]
}
func runClient(id int, servers []*Client, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
	value := strings.Repeat("x", 128)
	const batchSize = 1024

	opsCompleted := uint64(0)

	for !done.Load() {
		// Collect operations in order to preserve linearizability
		serverOperations := make(map[Client][]kvs.Operation)
		//operations := make([]kvs.Operation, 0, batchSize)

		for j := 0; j < batchSize; j++ {
			op := workload.Next()
			key := fmt.Sprintf("%d", op.Key)
			server := serverFromKey(&key, &servers)

			if _, ok := serverOperations[server]; !ok { //if server has no operations mapped to it yet
				serverOperations[server] = make([]kvs.Operation, 0, batchSize)
			}

			if op.IsRead {
				serverOperations[server] = append(serverOperations[server], kvs.Operation{
					OpType: "GET",
					Key:    key,
					Value:  "",
				})
			} else {
				serverOperations[server] = append(serverOperations[server], kvs.Operation{
					OpType: "PUT",
					Key:    key,
					Value:  value,
				})
			}
		}

		for server, operations := range serverOperations {
			server.BatchOp(operations)
		}
		opsCompleted += uint64(batchSize)
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

func dialHosts(servers HostList) []*Client {
	var clients []*Client
	for _, addr := range servers {
		clients = append(clients, Dial(addr))
	}
	return clients
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

	//host := hosts[0]
	numClients := 128
	for i := 0; i < numClients; i++ {
		go func(clientId int) {
			connections := dialHosts(hosts)
			workload := kvs.NewWorkload(*workload, *theta)
			runClient(clientId, connections, &done, workload, resultsCh)
		}(i)
	}

	time.Sleep(time.Duration(*secs) * time.Second)
	done.Store(true)

	opsCompleted := uint64(0)
	for i := 0; i < numClients; i++ {
		opsCompleted += <-resultsCh
	}

	elapsed := time.Since(start)

	opsPerSec := float64(opsCompleted) / elapsed.Seconds()
	fmt.Printf("throughput %.2f ops/s\n", opsPerSec)
}
