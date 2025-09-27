#!/usr/bin/env python3

import subprocess
import time
import csv
import os

# Test configuration
THETA_VALUES = [0, 0.3, 0.5, 0.7, 0.9, 0.99]
WORKLOADS = ["YCSB-A", "YCSB-B"]
CLUSTER_CONFIG = "1 3"  # 1 server, 3 clients for maximum contention
TEST_DURATION = 30

def run_test(workload, theta):
    """Run a single test and return results"""
    print(f"Testing {workload} with theta={theta}...")

    # Run cluster test
    cmd = f"./run-cluster.sh {CLUSTER_CONFIG} \"\" \"-workload {workload} -theta {theta} -secs {TEST_DURATION}\""

    try:
        # Run the test
        subprocess.run(cmd, shell=True, check=True, capture_output=False)

        # Wait a moment for logs to be written
        time.sleep(2)

        # Parse results using report-tput.py
        result = subprocess.run("python3 report-tput.py", shell=True, capture_output=True, text=True)

        if result.returncode != 0:
            print(f"Error running report-tput.py: {result.stderr}")
            return None

        # Parse the output
        lines = result.stdout.strip().split('\n')
        ops_total = 0
        commit_total = 0
        abort_total = 0
        abort_rate = 0

        for line in lines:
            if line.startswith("total") and "op/s" in line:
                ops_total = float(line.split()[1])
            elif line.startswith("total") and "commit/s" in line:
                commit_total = float(line.split()[1])
            elif line.startswith("total") and "abort/s" in line:
                abort_total = float(line.split()[1])
            elif line.startswith("abort rate"):
                abort_rate = float(line.split()[2].rstrip('%'))

        return {
            'workload': workload,
            'theta': theta,
            'ops_per_sec': ops_total,
            'commits_per_sec': commit_total,
            'aborts_per_sec': abort_total,
            'abort_rate': abort_rate
        }

    except subprocess.CalledProcessError as e:
        print(f"Error running test: {e}")
        return None

def main():
    """Run all tests and save results"""
    results = []

    print("Starting theta impact analysis...")
    print("=" * 50)

    for workload in WORKLOADS:
        print(f"\nTesting workload: {workload}")
        print("-" * 30)

        for theta in THETA_VALUES:
            result = run_test(workload, theta)
            if result:
                results.append(result)
                print(f"  theta={theta}: {result['commits_per_sec']:.0f} commit/s, {result['abort_rate']:.1f}% abort rate")
            else:
                print(f"  theta={theta}: FAILED")

            # Brief pause between tests
            time.sleep(1)

    # Save results to CSV
    if results:
        csv_filename = f"theta_analysis_{int(time.time())}.csv"
        with open(csv_filename, 'w', newline='') as csvfile:
            fieldnames = ['workload', 'theta', 'ops_per_sec', 'commits_per_sec', 'aborts_per_sec', 'abort_rate']
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)

            writer.writeheader()
            for result in results:
                writer.writerow(result)

        print(f"\nResults saved to: {csv_filename}")
        print("=" * 50)

        # Display summary
        print("\nSUMMARY:")
        for workload in WORKLOADS:
            print(f"\n{workload}:")
            workload_results = [r for r in results if r['workload'] == workload]
            for result in workload_results:
                print(f"  theta={result['theta']:4.2f}: {result['commits_per_sec']:6.0f} commit/s, {result['abort_rate']:5.1f}% aborts")
    else:
        print("No successful tests completed!")

if __name__ == "__main__":
    # Check if we're in the right directory
    if not os.path.exists("./run-cluster.sh"):
        print("Error: run-cluster.sh not found. Please run this script from the project root directory.")
        exit(1)

    if not os.path.exists("report-tput.py"):
        print("Error: report-tput.py not found. Please ensure it's in the current directory.")
        exit(1)

    main()