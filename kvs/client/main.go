package main

import (
	"flag"
	"fmt"
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

func (client *Client) BatchGet(keys []string) []string {
	request := kvs.BatchGetRequest{Keys: keys}
	response := kvs.BatchGetResponse{}
	err := client.rpcClient.Call("KVService.BatchGet", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
	return response.Values
}

func (client *Client) BatchPut(keys []string, values []string) {
	request := kvs.BatchPutRequest{Keys: keys, Values: values}
	response := kvs.BatchPutResponse{}
	err := client.rpcClient.Call("KVService.BatchPut", &request, &response)
	if err != nil {
		log.Fatal(err)
	}
}

func runClient(id int, addr string, done *atomic.Bool, workload *kvs.Workload, resultsCh chan<- uint64) {
	client := Dial(addr)

	value := strings.Repeat("x", 128)
	const batchSize = 100

	opsCompleted := uint64(0)

	for !done.Load() {
		readKeys := make([]string, 0, batchSize)
		writeKeys := make([]string, 0, batchSize)
		writeValues := make([]string, 0, batchSize)

		for j := 0; j < batchSize; j++ {
			op := workload.Next()
			key := fmt.Sprintf("%d", op.Key)

			if op.IsRead {
				// client.Get(key)
				readKeys = append(readKeys, key)
			} else {
				// client.Put(key, value)
				writeKeys = append(writeKeys, key)
				writeValues = append(writeValues, value)
			}
			if len(readKeys) > 0 {
				client.BatchGet(readKeys)  // ← 1 次 RPC 處理多個讀取
			}
		
			// 批次執行寫入
			if len(writeKeys) > 0 {
				client.BatchPut(writeKeys, writeValues)  // ← 1 次 RPC 處理多個寫入
			}
			// opsCompleted++
			opsCompleted += uint64(batchSize)
		}
	}

	// fmt.Printf("Client %d finished operations.\n", id)

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

	host := hosts[0]
	numClients := 128
	for i := 0; i < numClients; i++ {
		go func(clientId int) {

			workload := kvs.NewWorkload(*workload, *theta)
			runClient(clientId, host, &done, workload, resultsCh)
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
