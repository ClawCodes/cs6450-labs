#!/bin/bash

# benchmark-clients.sh - Script to benchmark throughput with different client counts
# Tests client counts: 1, 2, 4, 8, 16, 32... up to the specified maximum

set -euo pipefail

# Configuration
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CSV_DIR="${CSV_DIR:-$ROOT}" # Set CSV_DIR env var to save csvs to alternate location
OUTFILE="${CSV_FILE_NAME:-client_scaling_results.csv}"
RESULTS_FILE="${CSV_DIR}/${OUTFILE}"

echo "RESULTS: $RESULTS_FILE"

TEST_DURATION=30
WORKLOAD="YCSB-B"
THETA=0.99

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
    echo "Usage: $0 [max_clients] [server_count] [client_count] [test_duration] [workload]"
    echo "  max_clients:    Maximum number of clients to test (will test 1,2,4,8... up to this) [default: 128]"
    echo "  server_count:   Number of server nodes to use [default: half of available nodes]"
    echo "  client_count:   Number of client nodes to use [default: remaining nodes]"
    echo "  test_duration:  Duration in seconds for each test [default: 30]"
    echo "  workload:       YCSB workload type [default: YCSB-B]"
    echo ""
    echo "Examples:"
    echo "  $0              # Auto-split: 4 nodes -> 2 servers + 2 clients, 8 nodes -> 4 servers + 4 clients"
    echo "  $0 64           # Test up to 64 clients, auto-split nodes"
    echo "  $0 32 2 2       # Test up to 32 clients, 2 servers, 2 client nodes"
    echo "  $0 32 2 2 20      # Test up to 32 clients, 2 servers, 2 client nodes, batchSize of 20"
    echo "  $0 16 3 1 20 60    # Test up to 16 clients, 3 servers, 1 client node, batchSize of 20, 60s duration"
    exit 1
}

# Get available node count first to determine defaults
AVAILABLE_COUNT=$(cluster_size)

# Parse arguments with defaults based on available nodes
MAX_CLIENTS=${1:-128}

if [ "$#" -ge 2 ]; then
    SERVER_COUNT="$2"
else
    # Auto-split: half servers, half clients
    SERVER_COUNT=$((AVAILABLE_COUNT / 2))
fi

if [ "$#" -ge 3 ]; then
    CLIENT_COUNT="$3"
else
    # Use remaining nodes as clients
    CLIENT_COUNT=$((AVAILABLE_COUNT - SERVER_COUNT))
fi

BATCH_SIZE=${4:-1}
TEST_DURATION=${5:-30}
WORKLOAD=${6:-"YCSB-B"}

# Validate arguments
if ! [[ "$MAX_CLIENTS" =~ ^[0-9]+$ ]] || [ "$MAX_CLIENTS" -eq 0 ]; then
    echo "Error: max_clients must be a positive integer"
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

# Generate client counts: 1, 2, 4, 8, 16... up to MAX_CLIENTS
CLIENT_COUNTS=()
current=1
while [ $current -le $MAX_CLIENTS ]; do
    CLIENT_COUNTS+=($current)
    current=$((current * 2))
done

echo "=== Client Scaling Benchmark ==="
echo "Available nodes: $AVAILABLE_COUNT"
echo "Server count: $SERVER_COUNT"
echo "Client node count: $CLIENT_COUNT"
echo "Batch size: ${BATCH_SIZE}"
echo "Test duration: ${TEST_DURATION}s"
echo "Workload: $WORKLOAD"
echo "Results will be saved to: $RESULTS_FILE"
echo "Client counts: $CLIENT_COUNTS"
echo

# Create CSV header
echo "numClients,throughput_ops_per_sec" > "$RESULTS_FILE"

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

# Test each client count
for num_clients in "${CLIENT_COUNTS[@]}"; do
    echo "=== Testing with $num_clients clients across $CLIENT_COUNT client nodes ==="
    
    # Use run-cluster.sh with custom client args
    CLIENT_ARGS="-secs $TEST_DURATION -workload $WORKLOAD -theta $THETA -numClients $num_clients -batchSize $BATCH_SIZE"
    
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
        
        echo "Total throughput with $num_clients clients: $TOTAL_THROUGHPUT ops/s"
        
        # Save result to CSV
        echo "$num_clients,$TOTAL_THROUGHPUT" >> "$RESULTS_FILE"
    else
        echo "Warning: Could not find any client log files"
        echo "$num_clients,0" >> "$RESULTS_FILE"
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
echo "To plot the results, run: python3 plot_client_scaling.py"