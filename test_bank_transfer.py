#!/usr/bin/env python3

import subprocess
import time
import csv
import os
import re
from datetime import datetime

# Test configuration
TESTS = [
    {"name": "Basic_Distributed", "servers": 2, "clients": 2, "duration": 30},
    {"name": "High_Contention", "servers": 1, "clients": 3, "duration": 30},
    {"name": "Balanced_Distributed", "servers": 3, "clients": 1, "duration": 30}
]

def run_test(test_config):
    """Run a single bank transfer test and collect results"""
    print(f"\n{'='*60}")
    print(f"Running {test_config['name']} Test")
    print(f"Configuration: {test_config['servers']} servers, {test_config['clients']} clients")
    print(f"Duration: {test_config['duration']} seconds")
    print(f"{'='*60}")

    # Run the test
    cmd = f"./run-cluster.sh {test_config['servers']} {test_config['clients']} \"\" \"-workload xfer -secs {test_config['duration']}\""
    print(f"Command: {cmd}")

    try:
        # Run the test and capture output
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=test_config['duration'] + 60)

        if result.returncode != 0:
            print(f"Error running test: {result.stderr}")
            return None

        # Wait for logs to be written
        time.sleep(5)

        # Parse results from logs and output
        results = parse_test_results(test_config, result.stdout, result.stderr)
        return results

    except subprocess.TimeoutExpired:
        print("Test timed out!")
        return None
    except Exception as e:
        print(f"Error running test: {e}")
        return None

def parse_test_results(test_config, stdout, stderr):
    """Parse test results from output and log files"""
    results = {
        'test_name': test_config['name'],
        'servers': test_config['servers'],
        'clients': test_config['clients'],
        'duration': test_config['duration'],
        'timestamp': datetime.now().isoformat(),
        'total_transfers': 0,
        'successful_transfers': 0,
        'failed_transfers': 0,
        'integrity_violations': 0,
        'balance_checks': 0,
        'final_balances': [],
        'transfer_throughput': 0.0,
        'commits_per_sec': 0.0,
        'aborts_per_sec': 0.0,
        'abort_rate': 0.0
    }

    # Parse client output
    if "transfer throughput" in stdout:
        throughput_match = re.search(r'transfer throughput ([\d.]+) ops/s', stdout)
        if throughput_match:
            results['transfer_throughput'] = float(throughput_match.group(1))

    # Parse server logs for detailed statistics
    log_dir = "./logs/latest"
    if os.path.exists(log_dir):
        for log_file in os.listdir(log_dir):
            if log_file.startswith("kvsserver-") and log_file.endswith(".log"):
                log_path = os.path.join(log_dir, log_file)
                results = parse_server_log(log_path, results)

    # Parse client logs for transfer statistics
    client_log_files = [f for f in os.listdir(log_dir) if f.startswith("kvsclient-")]
    for client_log in client_log_files:
        log_path = os.path.join(log_dir, client_log)
        results = parse_client_log(log_path, results)

    return results

def parse_server_log(log_path, results):
    """Parse server log file for commit/abort statistics"""
    try:
        with open(log_path, 'r') as f:
            lines = f.readlines()

        # Get the last few stats printouts
        commit_rates = []
        abort_rates = []

        for line in lines:
            if "commit/s" in line:
                match = re.search(r'([\d.]+)', line)
                if match:
                    commit_rates.append(float(match.group(1)))
            elif "abort/s" in line:
                match = re.search(r'([\d.]+)', line)
                if match:
                    abort_rates.append(float(match.group(1)))

        # Use median of recent measurements
        if commit_rates:
            results['commits_per_sec'] += sorted(commit_rates)[-3:][len(sorted(commit_rates)[-3:])//2] if len(commit_rates) >= 3 else commit_rates[-1]
        if abort_rates:
            results['aborts_per_sec'] += sorted(abort_rates)[-3:][len(sorted(abort_rates)[-3:])//2] if len(abort_rates) >= 3 else abort_rates[-1]

    except Exception as e:
        print(f"Error parsing server log {log_path}: {e}")

    return results

def parse_client_log(log_path, results):
    """Parse client log file for transfer and balance check statistics"""
    try:
        with open(log_path, 'r') as f:
            content = f.read()

        # Count different types of events
        results['successful_transfers'] += len(re.findall(r'Transfer successful:', content))
        results['failed_transfers'] += len(re.findall(r'Transfer failed:', content))
        results['integrity_violations'] += len(re.findall(r'INTEGRITY VIOLATION:', content))
        results['balance_checks'] += len(re.findall(r'Balance check passed:', content))

        # Extract final balance information
        balance_matches = re.findall(r'Balance check passed: total=(\d+), balances=\[([^\]]+)\]', content)
        if balance_matches:
            # Get the last balance check
            total, balances_str = balance_matches[-1]
            # Handle both comma-separated and space-separated formats
            if ',' in balances_str:
                balances = [int(x.strip()) for x in balances_str.split(',')]
            else:
                balances = [int(x.strip()) for x in balances_str.split()]
            results['final_balances'] = balances

    except Exception as e:
        print(f"Error parsing client log {log_path}: {e}")

    return results

def save_results_to_csv(all_results, filename):
    """Save all test results to CSV file"""
    if not all_results:
        print("No results to save")
        return

    fieldnames = [
        'test_name', 'servers', 'clients', 'duration', 'timestamp',
        'total_transfers', 'successful_transfers', 'failed_transfers',
        'integrity_violations', 'balance_checks', 'final_balances',
        'transfer_throughput', 'commits_per_sec', 'aborts_per_sec', 'abort_rate'
    ]

    with open(filename, 'w', newline='') as csvfile:
        writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
        writer.writeheader()

        for result in all_results:
            # Calculate abort rate
            if result['commits_per_sec'] + result['aborts_per_sec'] > 0:
                result['abort_rate'] = (result['aborts_per_sec'] /
                                      (result['commits_per_sec'] + result['aborts_per_sec'])) * 100

            # Convert final_balances list to string for CSV
            result['final_balances'] = str(result['final_balances'])
            writer.writerow(result)

def generate_summary_report(all_results):
    """Generate a summary report of all tests"""
    print(f"\n{'='*80}")
    print("BANK TRANSFER TEST SUMMARY REPORT")
    print(f"{'='*80}")

    for result in all_results:
        if result is None:
            continue

        print(f"\n{result['test_name']} Test Results:")
        print(f"  Configuration: {result['servers']} servers, {result['clients']} clients")
        print(f"  Transfer Throughput: {result['transfer_throughput']:.2f} ops/s")
        print(f"  Successful Transfers: {result['successful_transfers']}")
        print(f"  Failed Transfers: {result['failed_transfers']}")
        print(f"  Integrity Violations: {result['integrity_violations']}")
        print(f"  Balance Checks: {result['balance_checks']}")
        print(f"  Commits/s: {result['commits_per_sec']:.2f}")
        print(f"  Aborts/s: {result['aborts_per_sec']:.2f}")
        print(f"  Abort Rate: {result['abort_rate']:.2f}%")
        print(f"  Final Account Balances: {result['final_balances']}")

        # Analysis
        if result['integrity_violations'] > 0:
            print(f"  ⚠️  WARNING: {result['integrity_violations']} integrity violations detected!")
        else:
            print(f"  ✅ No integrity violations - money conservation maintained")

        if result['failed_transfers'] > result['successful_transfers'] * 0.1:
            print(f"  ⚠️  High transfer failure rate: {result['failed_transfers']}/{result['successful_transfers'] + result['failed_transfers']}")

        print("-" * 60)

def main():
    """Main function to run all bank transfer tests"""
    print("Starting Bank Transfer Test Suite")
    print(f"Timestamp: {datetime.now()}")

    # Check if we're in the right directory
    if not os.path.exists("./run-cluster.sh"):
        print("Error: run-cluster.sh not found. Please run this script from the project root directory.")
        exit(1)

    # Make sure binaries are built
    print("Building project...")
    subprocess.run("make clean && make", shell=True, check=True)

    all_results = []

    for test_config in TESTS:
        result = run_test(test_config)
        if result:
            all_results.append(result)

        # Brief pause between tests
        print("Waiting 10 seconds before next test...")
        time.sleep(10)

    # Save results
    timestamp = int(time.time())
    csv_filename = f"bank_transfer_results_{timestamp}.csv"
    save_results_to_csv(all_results, csv_filename)
    print(f"\nResults saved to: {csv_filename}")

    # Generate summary report
    generate_summary_report(all_results)

    print(f"\n{'='*80}")
    print("Bank Transfer Test Suite Complete!")
    print(f"Results file: {csv_filename}")
    print(f"{'='*80}")

if __name__ == "__main__":
    main()