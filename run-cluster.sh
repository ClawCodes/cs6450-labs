#!/bin/bash

# run-cluster.sh - Script to run a distributed key-value store cluster
# on CloudLab more easily.
# Mostly thrown together with Claude.

set -euo pipefail

# Function to display usage
usage() {
    echo "Usage: $0 [server_count] [client_count] [server_args] [client_args]"
    echo "  server_count: Number of server nodes to use [optional - defaults to half of available nodes]"
    echo "  client_count: Number of client nodes to use [optional - defaults to remaining nodes]"
    echo "  server_args:  Arguments to pass to server processes (quoted string) [optional]"
    echo "  client_args:  Arguments to pass to client processes (quoted string) [optional]"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Auto-split available nodes"
    echo "  $0 2                                  # 2 servers, rest as clients"
    echo "  $0 2 3                               # 2 servers, 3 clients"
    echo "  $0 2 3 \"-port 8080\""
    echo "  $0 2 3 \"-port 8080\" \"-threads 4 -duration 30s\""
    exit 1
}

# Check for help options
for arg in "$@"; do
    case "$arg" in
        help|-h|--help|-help)
            usage
            ;;
    esac
done

# Check command line arguments
if [ "$#" -gt 4 ]; then
    echo "Error: Too many arguments"
    usage
fi

# Configuration
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_ROOT="${ROOT}/logs"

function cluster_size() {
    geni-get -a | \
        grep -Po '<interface_ref client_id=\\".*?\"' | \
        sed 's/<interface_ref client_id=\\"\(.*\)\\"/\1/' | \
        sort | \
        uniq | \
        wc -l
}

# Get available node count first to determine defaults
AVAILABLE_COUNT=$(cluster_size)

# Parse arguments with defaults based on available nodes
if [ "$#" -eq 0 ]; then
    # No arguments - auto-split available nodes
    SERVER_COUNT=$((AVAILABLE_COUNT / 2))
    CLIENT_COUNT=$((AVAILABLE_COUNT - SERVER_COUNT))
    SERVER_ARGS=""
    CLIENT_ARGS=""
elif [ "$#" -eq 1 ]; then
    # Only server count provided
    SERVER_COUNT="$1"
    CLIENT_COUNT=$((AVAILABLE_COUNT - SERVER_COUNT))
    SERVER_ARGS=""
    CLIENT_ARGS=""
elif [ "$#" -eq 2 ]; then
    # Server and client counts provided
    SERVER_COUNT="$1"
    CLIENT_COUNT="$2"
    SERVER_ARGS=""
    CLIENT_ARGS=""
elif [ "$#" -eq 3 ]; then
    # Server count, client count, and server args provided
    SERVER_COUNT="$1"
    CLIENT_COUNT="$2"
    SERVER_ARGS="$3"
    CLIENT_ARGS=""
else
    # All arguments provided
    SERVER_COUNT="$1"
    CLIENT_COUNT="$2"
    SERVER_ARGS="$3"
    CLIENT_ARGS="$4"
fi

# Validate arguments
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
    echo "       but only $AVAILABLE_COUNT nodes are available (node0 to node$((AVAILABLE_COUNT-1)))"
    exit 1
fi

echo "Using $TOTAL_NEEDED of $AVAILABLE_COUNT available nodes (node0 to node$((AVAILABLE_COUNT-1)))"

# Build server and client node arrays dynamically using sequential node names
SERVER_NODES=()
for ((i=0; i<SERVER_COUNT; i++)); do
    SERVER_NODES+=("node$i")
done

CLIENT_NODES=()
for ((i=SERVER_COUNT; i<SERVER_COUNT+CLIENT_COUNT; i++)); do
    CLIENT_NODES+=("node$i")
done

echo "Server nodes: ${SERVER_NODES[*]}"
echo "Client nodes: ${CLIENT_NODES[*]}"
echo "Server args: $SERVER_ARGS"
echo "Client args: $CLIENT_ARGS"
echo

SSH_OPTS="-o StrictHostKeyChecking=no"
SSH="ssh ${SSH_OPTS}"

# Timestamped log directory
TS=$(date +"%Y%m%d-%H%M%S")
LOG_DIR="$LOG_ROOT/$TS"
mkdir -p "$LOG_DIR"
ln -sfn "$LOG_DIR" "$LOG_ROOT/latest"

echo "Logs will be in $LOG_DIR"

# Cleanup function
cleanup() {
    echo "Cleaning up processes on all nodes..."
    for node in "${SERVER_NODES[@]}" "${CLIENT_NODES[@]}"; do
        echo "Cleaning up processes on $node..."
        ${SSH} $node "pkill -f 'kvs(server|client)' || true" 2>/dev/null || true
    done
    echo "Cleanup complete."
    echo
}

# Set trap for cleanup on script exit/interrupt
trap cleanup EXIT INT TERM

# Initial cleanup to ensure clean state
echo "Initial cluster cleanup..."
cleanup

echo "Building the project..."
make
echo

# Start servers
# Build comma-separated list of client hosts with port 8080
for node in "${CLIENT_NODES[@]}"; do
    if [ -n "$CLIENT_HOSTS" ]; then
        CLIENT_HOSTS="$CLIENT_HOSTS,$node:8080"
    else
        CLIENT_HOSTS="$node:8080"
    fi
done

for node in "${SERVER_NODES[@]}"; do
    echo "Starting server on $node..."
    ${SSH} $node "${ROOT}/bin/kvsserver -clients $CLIENT_HOSTS $SERVER_ARGS > \"$LOG_DIR/kvsserver-$node.log\" 2>&1 &"
done

# Give servers time to start
sleep 2

# Start clients with a unique marker for identification
# Build comma-separated list of server hosts with port 8080
SERVER_HOSTS=""
for node in "${SERVER_NODES[@]}"; do
    if [ -n "$SERVER_HOSTS" ]; then
        SERVER_HOSTS="$SERVER_HOSTS,$node:8080"
    else
        SERVER_HOSTS="$node:8080"
    fi
done

CLIENT_PIDS=()
for node in "${CLIENT_NODES[@]}"; do
    echo "Starting client on $node..."
    # Use a marker in the command line to make it easier to identify and wait for
    CLIENT_MARKER="kvsclient-run-$TS-$node"
    ${SSH} $node "exec -a '$CLIENT_MARKER' ${ROOT}/bin/kvsclient -hosts $SERVER_HOSTS $CLIENT_ARGS > \"$LOG_DIR/kvsclient-$node.log\" 2>&1" &
    CLIENT_PIDS+=($!)
done

echo "Waiting for clients to finish..."
# Wait for all client SSH sessions to complete
for pid in "${CLIENT_PIDS[@]}"; do
    wait $pid 2>/dev/null || true
done

echo "All clients finished."

# Final cleanup will be handled by the trap
echo "Run complete. Logs in $LOG_DIR"
echo

# Calculate and display median ops/s from server logs
python3 report-tput.py
echo
