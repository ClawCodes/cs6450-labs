#!/bin/bash

# benchmark-clients.sh - Script to benchmark throughput with different client counts
# Tests client counts: 1, 2, 4, 8, 16, 32... up to the specified maximum

set -euo pipefail

# Configuration
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_FILE="${ROOT}/client_scaling_results.csv"
TEST_DURATION=30
WORKLOAD="YCSB-B"
THETA=0.99

# Function to display usage
usage() {
    echo "Usage: $0 [max_clients] [server_count] [test_duration] [workload]"
    echo "  max_clients:    Maximum number of clients to test (will test 1,2,4,8... up to this) [default: 128]"
    echo "  server_count:   Number of server nodes to use [default: 2]"
    echo "  test_duration:  Duration in seconds for each test [default: 30]"
    echo "  workload:       YCSB workload type [default: YCSB-B]"
    echo ""
    echo "Examples:"
    echo "  $0              # Test up to 128 clients, 2 servers, 30s duration, YCSB-B"
    echo "  $0 64           # Test up to 64 clients"
    echo "  $0 32 4         # Test up to 32 clients, 4 servers"
    echo "  $0 16 2 60      # Test up to 16 clients, 2 servers, 60s duration"
    exit 1
}

# Parse arguments
MAX_CLIENTS=${1:-128}
SERVER_COUNT=${2:-2}
TEST_DURATION=${3:-30}
WORKLOAD=${4:-"YCSB-B"}

# Validate arguments
if ! [[ "$MAX_CLIENTS" =~ ^[0-9]+$ ]] || [ "$MAX_CLIENTS" -eq 0 ]; then
    echo "Error: max_clients must be a positive integer"
    exit 1
fi

if ! [[ "$SERVER_COUNT" =~ ^[0-9]+$ ]] || [ "$SERVER_COUNT" -eq 0 ]; then
    echo "Error: server_count must be a positive integer"
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
echo "Max clients: $MAX_CLIENTS"
echo "Server count: $SERVER_COUNT"
echo "Test duration: ${TEST_DURATION}s"
echo "Workload: $WORKLOAD"
echo "Client counts to test: ${CLIENT_COUNTS[*]}"
echo "Results will be saved to: $RESULTS_FILE"
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
    echo "=== Testing with $num_clients clients ==="
    
    # Use run-cluster.sh with custom client args
    CLIENT_ARGS="-secs $TEST_DURATION -workload $WORKLOAD -theta $THETA -numClients $num_clients"
    
    echo "Running: ./run-cluster.sh $SERVER_COUNT 1 \"\" \"$CLIENT_ARGS\""
    
    # Run the cluster (1 client node, but with multiple concurrent clients)
    ./run-cluster.sh "$SERVER_COUNT" 1 "" "$CLIENT_ARGS"
    
    # Extract throughput from the latest client log
    LOG_DIR=$(readlink "$ROOT/logs/latest")
    CLIENT_LOG=$(find "$LOG_DIR" -name "kvsclient-*.log" | head -1)
    
    if [ -n "$CLIENT_LOG" ] && [ -f "$CLIENT_LOG" ]; then
        THROUGHPUT=$(extract_throughput "$CLIENT_LOG")
        echo "Throughput with $num_clients clients: $THROUGHPUT ops/s"
        
        # Save result to CSV
        echo "$num_clients,$THROUGHPUT" >> "$RESULTS_FILE"
    else
        echo "Warning: Could not find client log file"
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