#!/bin/bash

# scaling.sh
# Runs run-cluster.sh for every (servers,clients) combination where
# servers+clients == total_nodes for total_nodes=2..AVAILABLE_COUNT.
# After each run it parses the output of report-tput.py and appends a
# CSV row to logs/scale_results.csv containing totals and metadata.
# About half GPT-5 mini

set -euo pipefail

# Use relative paths from the current working directory
LOG="./logs"
RUN_SCRIPT="./run-cluster.sh"
REPORT_SCRIPT="./report-tput.py"
OUT_DIR="./charts and stats"

function cluster_size() {
    /usr/local/etc/emulab/tmcc hostnames | wc -l
}

AVAILABLE_COUNT=$(cluster_size)
AVAILABLE_COUNT=$(echo "$AVAILABLE_COUNT" | tr -d '[:space:]')

# Command-line options: --workload <string> and --theta <float>
WORKLOAD="YCSB-A"
THETA="0.5"
SECS="30"

# Thread experiment
CLIENT_THREADS=0 # This is the thread exponent - will be processed as 2^0, 2^thread_step, 2^thread_step*2, etc.
THREAD_STEP=1
IS_THREAD_EXP=0
NUM_SERVERS=0 # if set, only run experiment with fixed server count

while [[ $# -gt 0 ]]; do
    case "$1" in
        --workload)
            WORKLOAD="$2"
            shift 2
            ;;
        --theta)
            THETA="$2"
            shift 2
            ;;
        --secs)
            SECS="$2"
            shift 2
            ;;
        --client-threads)
          CLIENT_THREADS="$2"
          IS_THREAD_EXP=1
          shift 2
          ;;
        --thread-step)
          THREAD_STEP="$2"
          shift 2
          ;;
        --num-servers)
          NUM_SERVERS="$2"
          shift 2
          ;;
        *)
          echo "Unknown argument: $1"
          echo "Usage: $0 [--workload WORKLOAD] [--theta THETA] [--secs SECS]"
          exit 1
          ;;
    esac
done

# Create per-run CSV in 'charts and stats' with timestamp
mkdir -p "$LOG"
mkdir -p "$OUT_DIR"
TS_RUN=$(date +%s)
CSV="$OUT_DIR/scale_results_${TS_RUN}.csv"
# CSV headers compatible with plot_theta_analysis.py plus metadata
# Columns: workload,theta,commits_per_sec,aborts_per_sec,abort_rate,ops_per_sec,servers,clients,ts,log_dir

function addCSVHeader(){
  echo "workload,theta,commits_per_sec,aborts_per_sec,abort_rate,ops_per_sec,servers,clients,thread_count,num_nodes,ts,log_dir" > "$1"
}

addCSVHeader "$CSV"

echo "Running scaling experiments for totals 1 -> $AVAILABLE_COUNT"

# shellcheck disable=SC2120
function writeCSVRow() {
  local FILENAME="${1:-$CSV}"
  local threadCount="${2:-1}" # default thread count is 1
  local total=$((clients + servers)) # note, parent scope is expected to have clients and servers set
  # Get the log directory that run-cluster.sh created (it updates logs/latest)
  LOG_DIR="$(readlink -f "$LOG/latest" 2>/dev/null)"

  # Run the report script to compute totals (it reads logs/latest)
  REPORT=$(python3 "$REPORT_SCRIPT" 2>/dev/null)

  # Parse totals from REPORT. Select the summary "total ..." lines so we
  # capture the aggregated numeric values (not the per-node "median" tokens).
  total_ops=$(echo "$REPORT" | awk '/^total .*op\/s/ {print $2; exit}')
  total_commits=$(echo "$REPORT" | awk '/^total .*commit\/s/ {print $2; exit}')
  total_aborts=$(echo "$REPORT" | awk '/^total .*abort\/s/ {print $2; exit}')
  abort_rate=$(echo "$REPORT" | awk '/abort rate/ {print $3; exit}' | tr -d '%')

  total_ops=${total_ops:-0}
  total_commits=${total_commits:-0}
  total_aborts=${total_aborts:-0}
  abort_rate=${abort_rate:-0}

  ts_field="$(basename "$LOG_DIR" 2>/dev/null || date +%s)"

  # Provide fields compatible with plot_theta_analysis.py
  workload="$WORKLOAD"
  theta="$THETA"
  commits_per_sec="$total_commits"
  aborts_per_sec="$total_aborts"
  ops_per_sec="$total_ops"
  echo "$workload,$theta,$commits_per_sec,$aborts_per_sec,$abort_rate,$ops_per_sec,$servers,$clients,$threadCount,$total,$ts_field,$LOG_DIR" >> "$FILENAME"
}

function runThreadingExp() {
  clients=$1
  servers=$2
  echo "Running Client thread Experiment - Max num threads: $CLIENT_THREADS, Step: $CLIENT_THREADS"
  maxThreadCount=$((2 ** CLIENT_THREADS))
  local OUTFILE="$OUT_DIR/scale_results_servers_${servers}_clients_${clients}_max_thread_count_${maxThreadCount}_${TS_RUN}.csv"
  addCSVHeader "$OUTFILE" # All thread steps go to single CSV file
  for ((exp=0; exp<=CLIENT_THREADS; exp+=THREAD_STEP)); do
      threadCount=$((2 ** exp))
      echo "running experiment with $threadCount threads"
      RUN_CMD="$RUN_SCRIPT $servers $clients \"\" \"-workload $WORKLOAD -theta $THETA -secs $SECS --clientThreads $threadCount\""
      bash -c "$RUN_CMD"
      writeCSVRow "$OUTFILE" "$threadCount"
  done
  echo "Client thread experiment complete."
}

function runNodeScalingExp(){
  clients=$1
  servers=$2
  RUN_CMD="$RUN_SCRIPT $servers $clients \"\" \"-workload $WORKLOAD -theta $THETA -secs $SECS\""

  # Execute the run command and capture all output. Continue even if a run fails.
  OUTFILE=$(mktemp /tmp/scale_run_XXXXXX.out)
  echo "running experiment..."
  bash -c "$RUN_CMD" > "$OUTFILE" 2>&1
  echo "extracting data from logs..."

  writeCSVRow
  echo "Results in $CSV$"
}

if [[ $NUM_SERVERS -gt 0 ]]; then # Run with fixed number of client and servers
  NUM_CLIENTS=$((AVAILABLE_COUNT - NUM_SERVERS))
  if [[ $IS_THREAD_EXP -eq 1 ]]; then
    runThreadingExp "$NUM_CLIENTS" "$NUM_SERVERS"
  else
    runNodeScalingExp "$NUM_CLIENTS" "$NUM_SERVERS"
  fi
else
  # Iterate over node counts
  for ((servers=1; servers<AVAILABLE_COUNT; servers++)); do
      clients=$((AVAILABLE_COUNT - servers))
      echo "=== servers=$servers clients=$clients ==="
      if [[ $IS_THREAD_EXP -eq 1 ]]; then # run scaling exp with thread scaling
        runThreadingExp "$clients" "$servers"
      else
        runNodeScalingExp "$clients" "$servers"
      fi
      # small pause so timestamps/dirs differ and to give cluster a clean window
      sleep 1
  done
fi