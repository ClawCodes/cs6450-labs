#!/bin/bash

# scaling.sh
# Runs run-cluster.sh for every (servers,clients) combination where
# servers+clients == total_nodes for total_nodes=2..AVAILABLE_COUNT.
# After each run it parses the output of report-tput.py and appends a
# CSV row to logs/scale_results.csv containing totals and metadata.
# Mostly GPT-5 mini

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
echo "workload,theta,commits_per_sec,aborts_per_sec,abort_rate,ops_per_sec,servers,clients,ts,log_dir" > "$CSV"

echo "Running scaling experiments for totals 2 -> $AVAILABLE_COUNT"

# Iterate over total node counts
for total in $(seq 2 "$AVAILABLE_COUNT"); do
  for servers in $(seq 1 $((total - 1))); do
    clients=$((total - servers))
    echo "=== total=$total servers=$servers clients=$clients ==="

    RUN_CMD="$RUN_SCRIPT $servers $clients \"\" \"-workload $WORKLOAD -theta $THETA -secs $SECS\""

    # Execute the run command and capture all output. Continue even if a run fails.
    OUTFILE=$(mktemp /tmp/scale_run_XXXXXX.out)
    echo "running experiment..."
    bash -c "$RUN_CMD" > "$OUTFILE" 2>&1 || true
    echo "extracting data from logs..."

    # Get the log directory that run-cluster.sh created (it updates logs/latest)
    LOG_DIR="$(readlink -f "$LOG/latest" 2>/dev/null || true)"

    # Run the report script to compute totals (it reads logs/latest)
    REPORT=$(python3 "$REPORT_SCRIPT" 2>/dev/null || true)

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

    echo "$workload,$theta,$commits_per_sec,$aborts_per_sec,$abort_rate,$ops_per_sec,$servers,$clients,$ts_field,$LOG_DIR" >> "$CSV"

    # small pause so timestamps/dirs differ and to give cluster a clean window
    sleep 1
  done
done

echo "Results in $CSV$"