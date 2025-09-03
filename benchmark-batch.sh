#!/bin/bash

# benchmark-batch.sh - Script to benchmark throughput with different batch sizes
# Tests batch sizes: start_size, start_size*2, start_size*4, ... until >= max_size

set -euo pipefail

# Configuration
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_FILE="${ROOT}/batch_scaling_results.csv"
TEST_DURATION=30
WORKLOAD="YCSB-B"
THETA=0.99
NUM_CLIENTS=256

# Function to get cluster size (same as run-cluster.sh)
function cluster_size() {
    geni-get -a | \
        grep -Po '<interface_ref client_id=\\".*?\"' | \
        sed 's/<interface_ref client_ide=\\"\(.*\)\\"/\1/' | \
        sort | \
        uniq | \
        wc -l
}

# Function to display usage
usage() {
    echo "Usage: $0 start_batch_size max_batch_size [server_count] [client_count] [test_duration] [num_clients]"
    echo "  start_batch_size:  Starting batch size (will test start_size, start_size*2, start_size*4, ... up to max_size)"
    echo "  max_batch_size:    Maximum batch size to test"
    echo "  server_count:      Number of server nodes to use [default: half of available nodes]"
    echo "  client_count:      Number of client nodes to use [default: remaining nodes]"
    echo "  test_duration:     Duration in seconds for each test [default: 30]"
    echo "  num_clients:       Number of concurrent goroutines per client node [default: 256]"
    echo ""
    echo "Examples:"
    echo "  $0 1024 8192                    # Test batch sizes: 1024, 2048, 4096, 8192"
    echo "  $0 512 4096 2 2                # Test with 2 servers, 2 client nodes"
    echo "  $0 1 1024 2 1 60 128           # Test 1,2,4,...,1024 with 128 goroutines for 60s"
    exit 1
}

# Check for required arguments
if [ "$#" -lt 2 ]; then
    echo "Error: start_batch_size and max_batch_size are required"
    usage
fi

# Parse arguments
START_BATCH_SIZE="$1"
MAX_BATCH_SIZE="$2"

# Get available node count first to determine defaults
AVAILABLE_COUNT=$(cluster_size)

if [ "$#" -ge 3 ]; then
    SERVER_COUNT="$3"
else
    # Auto-split: half servers, half clients
    SERVER_COUNT=$((AVAILABLE_COUNT / 2))
fi

if [ "$#" -ge 4 ]; then
    CLIENT_COUNT="$4"
else
    # Use remaining nodes as clients
    CLIENT_COUNT=$((AVAILABLE_COUNT - SERVER_COUNT))
fi

TEST_DURATION=${5:-30}
NUM_CLIENTS=${6:-256}

# Validate arguments
if ! [[ "$START_BATCH_SIZE" =~ ^[0-9]+$ ]] || [ "$START_BATCH_SIZE" -eq 0 ]; then
    echo "Error: start_batch_size must be a positive integer"
    exit 1
fi

if ! [[ "$MAX_BATCH_SIZE" =~ ^[0-9]+$ ]] || [ "$MAX_BATCH_SIZE" -eq 0 ]; then
    echo "Error: max_batch_size must be a positive integer"
    exit 1
fi

if [ "$START_BATCH_SIZE" -gt "$MAX_BATCH_SIZE" ]; then
    echo "Error: start_batch_size ($START_BATCH_SIZE) must be <= max_batch_size ($MAX_BATCH_SIZE)"
    exit 1
fi

if ! [[ "$SERVER_COUNT" =~ ^[0-9]+$ ]] || [ "$SERVER_COUNT" -eq 0 ]; then
    echo "Error: server_count must be a positive integer"
    exit 1
fi

if ! [[ "$CLIENT_COUNT" =~ ^[0-9]+$ ]] || [ "$CLIENT_COUNT" -eq 0 ]; then
    echo "Error: client_count must be a positive integer"
    exit 1
fi

# Check that we have enough available nodes
TOTAL_NEEDED=$((SERVER_COUNT + CLIENT_COUNT))
if [ "$TOTAL_NEEDED" -gt "$AVAILABLE_COUNT" ]; then
    echo "Error: Requested $TOTAL_NEEDED nodes (servers: $SERVER_COUNT, clients: $CLIENT_COUNT)"
    echo "       but only $AVAILABLE_COUNT nodes are available"
    exit 1
fi

# Generate batch sizes: start_size, start_size*2, start_size*4, ... up to max_size
BATCH_SIZES=()
current=$START_BATCH_SIZE
while [ $current -le $MAX_BATCH_SIZE ]; do
    BATCH_SIZES+=($current)
    current=$((current * 2))
done

echo "=== Batch Size Scaling Benchmark ==="
echo "Available nodes: $AVAILABLE_COUNT"
echo "Server count: $SERVER_COUNT"
echo "Client node count: $CLIENT_COUNT"
echo "Concurrent goroutines per client node: $NUM_CLIENTS"
echo "Test duration: ${TEST_DURATION}s"
echo "Workload: $WORKLOAD"
echo "Batch sizes to test: ${BATCH_SIZES[*]}"
echo "Results will be saved to: $RESULTS_FILE"
echo

# Create CSV header
echo "batchSize,throughput_ops_per_sec" > "$RESULTS_FILE"

# Build the project
echo "Building the project..."
make clean && make
echo

# Function to extract throughput from client output
extract_throughput() {
    local log_file="$1"
    if [ -f "$log_file" ]; then
        # Look for "throughput X.XX ops/s" in the log file
        grep "throughput" "$log_file" | tail -1 | sed -n 's/.*throughput \([0-9.]*\) ops\/s.*/\1/p'
    else
        echo "0"
    fi
}

# Test each batch size
for batch_size in "${BATCH_SIZES[@]}"; do
    echo "=== Testing with batch size $batch_size ==="
    
    # Use run-cluster.sh with custom client args
    CLIENT_ARGS="-secs $TEST_DURATION -workload $WORKLOAD -theta $THETA -numClients $NUM_CLIENTS -batchSize $batch_size"
    
    echo "Running: ./run-cluster.sh $SERVER_COUNT $CLIENT_COUNT \"\" \"$CLIENT_ARGS\""
    
    # Run the cluster with specified server and client node counts
    ./run-cluster.sh "$SERVER_COUNT" "$CLIENT_COUNT" "" "$CLIENT_ARGS"
    
    # Extract throughput from all client logs and sum them up
    LOG_DIR=$(readlink "$ROOT/logs/latest")
    TOTAL_THROUGHPUT=0
    CLIENT_LOGS=($(find "$LOG_DIR" -name "kvsclient-*.log"))
    
    if [ ${#CLIENT_LOGS[@]} -gt 0 ]; then
        for CLIENT_LOG in "${CLIENT_LOGS[@]}"; do
            THROUGHPUT=$(extract_throughput "$CLIENT_LOG")
            if [[ "$THROUGHPUT" =~ ^[0-9]+\.?[0-9]*$ ]]; then
                TOTAL_THROUGHPUT=$(echo "$TOTAL_THROUGHPUT + $THROUGHPUT" | bc -l)
            fi
        done
        
        echo "Total throughput with batch size $batch_size: $TOTAL_THROUGHPUT ops/s"
        
        # Save result to CSV
        echo "$batch_size,$TOTAL_THROUGHPUT" >> "$RESULTS_FILE"
    else
        echo "Warning: Could not find any client log files"
        echo "$batch_size,0" >> "$RESULTS_FILE"
    fi
    
    echo "Waiting 5 seconds before next test..."
    sleep 5
    echo
done

echo "=== Benchmark Complete ==="
echo "Results saved to: $RESULTS_FILE"
echo
echo "Results summary:"
cat "$RESULTS_FILE"
echo
echo "To plot the results, run: python3 plot_batch_scaling.py"