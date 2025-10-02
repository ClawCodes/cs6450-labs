#!/usr/bin/env python3

# Extracts throughputs and summarizes them from a set of kvs server logs.
# Mostly thrown together with Claude from a draft awk script.

import os
import glob
import statistics

LOG_DIR = "./logs/latest"

total_ops_throughput = 0.0
total_commit_throughput = 0.0
total_abort_throughput = 0.0

# Find all matching log files
log_files = sorted(glob.glob(os.path.join(LOG_DIR, "kvsserver-*.log")))

if not log_files:
    print("No matching log files found.")
    exit(0)

for log_path in log_files:
    node = os.path.basename(log_path).removeprefix("kvsserver-").removesuffix(".log")
    ops_throughputs = []
    commit_throughputs = []
    abort_throughputs = []

    with open(log_path) as f:
        for line in f:
            if "ops/s" in line:
                parts = line.strip().split()
                try:
                    # Throughput value is the 2nd column in original awk ($2)
                    ops_throughputs.append(float(parts[1]))
                except (IndexError, ValueError):
                    pass
            elif "commit/s" in line:
                parts = line.strip().split()
                try:
                    # Throughput value is the 2nd column in original awk ($2)
                    commit_throughputs.append(float(parts[1]))
                except (IndexError, ValueError):
                    pass
            elif "abort/s" in line:
                parts = line.strip().split()
                try:
                    # Throughput value is the 2nd column in original awk ($2)
                    abort_throughputs.append(float(parts[1]))
                except (IndexError, ValueError):
                    pass

    if ops_throughputs:
        median_ops = statistics.median(sorted(ops_throughputs))
        print(f"{node} median {median_ops:.0f} op/s")
        total_ops_throughput += median_ops
    else:
        print(f"{node} no ops/s data found")

    if commit_throughputs:
        median_commits = statistics.median(sorted(commit_throughputs))
        print(f"{node} median {median_commits:.0f} commit/s")
        total_commit_throughput += median_commits
    else:
        print(f"{node} no commit/s data found")

    if abort_throughputs:
        median_aborts = statistics.median(sorted(abort_throughputs))
        print(f"{node} median {median_aborts:.0f} abort/s")
        total_abort_throughput += median_aborts
    else:
        print(f"{node} no abort/s data found")

print()
print(f"total {total_ops_throughput:.0f} op/s")
print(f"total {total_commit_throughput:.0f} commit/s")
print(f"total {total_abort_throughput:.0f} abort/s")

# Calculate and display abort rate
if total_commit_throughput + total_abort_throughput > 0:
    abort_rate = total_abort_throughput / (total_commit_throughput + total_abort_throughput) * 100
    print(f"abort rate {abort_rate:.1f}%")
