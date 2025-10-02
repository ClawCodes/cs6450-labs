#!/usr/bin/env python3

import pandas as pd
import matplotlib
matplotlib.use('Agg')  # Use non-interactive backend
import matplotlib.pyplot as plt
import sys
import glob
import numpy as np

def plot_theta_analysis(csv_file):
    """Generate plots from theta analysis CSV data"""

    # Read the CSV file
    try:
        df = pd.read_csv(csv_file)
    except FileNotFoundError:
        print(f"Error: Could not find file {csv_file}")
        return
    except Exception as e:
        print(f"Error reading CSV file: {e}")
        return

    # Create figure with subplots
    fig, ((ax1, ax2), (ax3, ax4)) = plt.subplots(2, 2, figsize=(15, 12))
    fig.suptitle('Theta Parameter Impact Analysis on Distributed KVS', fontsize=16, fontweight='bold')

    # Colors for different workloads
    colors = {'YCSB-A': '#e74c3c', 'YCSB-B': '#3498db'}
    markers = {'YCSB-A': 'o', 'YCSB-B': 's'}

    # Plot 1: Commit Rate vs Theta
    for workload in df['workload'].unique():
        workload_data = df[df['workload'] == workload].sort_values('theta')
        ax1.plot(workload_data['theta'], workload_data['commits_per_sec'],
                color=colors[workload], marker=markers[workload], linewidth=2,
                markersize=8, label=f'{workload} (50% writes)' if workload == 'YCSB-A' else f'{workload} (5% writes)')

    ax1.set_xlabel('Theta (Zipfian Skew Parameter)', fontweight='bold')
    ax1.set_ylabel('Commits per Second', fontweight='bold')
    ax1.set_title('Transaction Commit Rate vs Contention Level')
    ax1.grid(True, alpha=0.3)
    ax1.legend()
    ax1.set_xlim(-0.05, 1.05)

    # Plot 2: Abort Rate vs Theta
    for workload in df['workload'].unique():
        workload_data = df[df['workload'] == workload].sort_values('theta')
        ax2.plot(workload_data['theta'], workload_data['abort_rate'],
                color=colors[workload], marker=markers[workload], linewidth=2,
                markersize=8, label=workload)

    ax2.set_xlabel('Theta (Zipfian Skew Parameter)', fontweight='bold')
    ax2.set_ylabel('Abort Rate (%)', fontweight='bold')
    ax2.set_title('Transaction Abort Rate vs Contention Level')
    ax2.grid(True, alpha=0.3)
    ax2.legend()
    ax2.set_xlim(-0.05, 1.05)

    # Plot 3: Total Operations vs Theta
    for workload in df['workload'].unique():
        workload_data = df[df['workload'] == workload].sort_values('theta')
        ax3.plot(workload_data['theta'], workload_data['ops_per_sec'],
                color=colors[workload], marker=markers[workload], linewidth=2,
                markersize=8, label=workload)

    ax3.set_xlabel('Theta (Zipfian Skew Parameter)', fontweight='bold')
    ax3.set_ylabel('Operations per Second', fontweight='bold')
    ax3.set_title('Total Operation Rate vs Contention Level')
    ax3.grid(True, alpha=0.3)
    ax3.legend()
    ax3.set_xlim(-0.05, 1.05)

    # Plot 4: Transaction Success Rate vs Theta
    for workload in df['workload'].unique():
        workload_data = df[df['workload'] == workload].sort_values('theta')
        success_rate = (workload_data['commits_per_sec'] / (workload_data['commits_per_sec'] + workload_data['aborts_per_sec'])) * 100
        ax4.plot(workload_data['theta'], success_rate,
                color=colors[workload], marker=markers[workload], linewidth=2,
                markersize=8, label=workload)

    ax4.set_xlabel('Theta (Zipfian Skew Parameter)', fontweight='bold')
    ax4.set_ylabel('Transaction Success Rate (%)', fontweight='bold')
    ax4.set_title('Transaction Success Rate vs Contention Level')
    ax4.grid(True, alpha=0.3)
    ax4.legend()
    ax4.set_xlim(-0.05, 1.05)

    # Adjust layout and save
    plt.tight_layout()

    # Save the plot
    output_file = csv_file.replace('.csv', '_analysis.png')
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Plot saved as: {output_file}")

    # Also save as PDF for better quality
    # pdf_file = csv_file.replace('.csv', '_analysis.pdf')
    # plt.savefig(pdf_file, bbox_inches='tight')
    # print(f"High-quality plot saved as: {pdf_file}")

    # Show the plot
    # plt.show()  # Commented out to avoid display issues in headless environment

def create_summary_table(csv_file):
    """Create a summary table of the results"""
    df = pd.read_csv(csv_file)

    print("\n" + "="*80)
    print("THETA ANALYSIS SUMMARY TABLE")
    print("="*80)

    for workload in sorted(df['workload'].unique()):
        print(f"\n{workload} Workload:")
        print("-" * 50)
        workload_data = df[df['workload'] == workload].sort_values('theta')

        print(f"{'Theta':<8} {'Commits/s':<12} {'Aborts/s':<12} {'Abort Rate':<12} {'Success Rate':<12}")
        print("-" * 60)

        for _, row in workload_data.iterrows():
            success_rate = (row['commits_per_sec'] / (row['commits_per_sec'] + row['aborts_per_sec'])) * 100 if (row['commits_per_sec'] + row['aborts_per_sec']) > 0 else 100
            print(f"{row['theta']:<8.2f} {row['commits_per_sec']:<12.0f} {row['aborts_per_sec']:<12.0f} "
                  f"{row['abort_rate']:<12.1f}% {success_rate:<12.1f}%")

def main():
    """Main function to process CSV and generate plots"""

    # Check for CSV file argument
    if len(sys.argv) > 1:
        csv_file = sys.argv[1]
    else:
        # Look for the most recent theta analysis CSV file
        csv_files = glob.glob("theta_analysis_*.csv")
        if not csv_files:
            print("No theta analysis CSV files found.")
            print("Usage: python3 plot_theta_analysis.py [csv_file]")
            print("   or: run theta_test.py first to generate data")
            return

        # Use the most recent file
        csv_file = max(csv_files, key=lambda x: x.split('_')[-1])
        print(f"Using most recent CSV file: {csv_file}")

    # Generate plots and summary
    plot_theta_analysis(csv_file)
    create_summary_table(csv_file)

if __name__ == "__main__":
    main()