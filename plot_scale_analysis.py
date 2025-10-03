#!/usr/bin/env python3

import pandas as pd
import matplotlib
matplotlib.use('Agg')  # Use non-interactive backend
import matplotlib.pyplot as plt
import sys
import glob
import numpy as np

def plot_scale_analysis(csv_file):
    """Generate plots from scaling analysis CSV data"""

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
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(15, 6))
    fig.suptitle('Scaling Impact Analysis on Distributed KVS', fontsize=16, fontweight='bold')

    # Colors for different total number of machines #1d0c19
    colors = {'2': '#3498db','3': '#518cc2','4': '#6e80a9','5': '#8df286','6': '#abbb1b','7': '#c983ab','8': '#e74c3c'}
    markers = {'2':'o', '3':'s', '4':'v', '5':'D', '6':'X', '7':'*', '8':'P'}

    # Plot 1: Commit Rate vs # Client Nodes
    for num_nodes in df['num_nodes'].unique():
        workload_data = df[df['num_nodes'] == num_nodes]
        ax1.plot(workload_data['clients'], workload_data['commits_per_sec'], color=colors[str(num_nodes)], 
                linewidth=2, linestyle='--', markersize=8, marker=markers[str(num_nodes)], label=f'{num_nodes} nodes')

    ax1.set_xlabel('Number of Client Nodes', fontweight='bold')
    ax1.set_ylabel('Commits per Second', fontweight='bold')
    ax1.set_title('Transaction Commit Rate vs Number of Client Nodes')
    ax1.grid(True, alpha=0.3)
    ax1.legend()

    # Plot 2: Total Ops/s vs # Client Nodes
    for num_nodes in df['num_nodes'].unique():
        workload_data = df[df['num_nodes'] == num_nodes]
        ax2.plot(workload_data['clients'], workload_data['ops_per_sec'], color=colors[str(num_nodes)], 
                linewidth=2, linestyle='--', markersize=8, marker=markers[str(num_nodes)], label=f'{num_nodes} nodes')

    ax2.set_xlabel('Number of Client Nodes', fontweight='bold')
    ax2.set_ylabel('Operations per Second', fontweight='bold')
    ax2.set_title('Total Operation Rate vs Number of Client Nodes')
    ax2.grid(True, alpha=0.3)
    ax2.legend()
    # ax2.set_xlim(-0.05, 1.05)

    # Adjust layout and save
    plt.tight_layout()

    # Save the plot
    output_file = csv_file.replace('.csv', '_plot.png')
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
    print("SCALE ANALYSIS SUMMARY TABLE")
    print("="*80)

    for num_nodes in sorted(df['num_nodes'].unique()):
        print(f"\nCluster size {num_nodes}:")
        print("-" * 80)
        workload_data = df[df['num_nodes'] == num_nodes]

        print(f"{'Clients':<12} {'Servers':<12} {'Commits/s':<12} {'Aborts/s':<12} {'Abort Rate':<12} {'Success Rate':<12} {'Theta':<8} {'Workload'}")
        print("-" * 80)

        for _, row in workload_data.iterrows():
            success_rate = (row['commits_per_sec'] / (row['commits_per_sec'] + row['aborts_per_sec'])) * 100 if (row['commits_per_sec'] + row['aborts_per_sec']) > 0 else 100
            print(f"{row['clients']:<12} {row['servers']:<12} {row['commits_per_sec']:<12.0f} {row['aborts_per_sec']:<12.0f} "
                  f"%{row['abort_rate']:<12.1f} %{success_rate:<12.1f} {row['theta']:<8.2f} {row['workload']}")

def main():
    """Main function to process CSV and generate plots"""

    # Check for CSV file argument
    if len(sys.argv) > 1:
        csv_file = sys.argv[1]
    else:
        # Look for the most recent theta analysis CSV file
        csv_files = glob.glob("scale_results_*.csv")
        if not csv_files:
            print("No scale analysis CSV files found.")
            print("Usage: python3 plot_scale_analysis.py [csv_file]")
            print("   or: run scaling.sh first to generate data")
            return

        # Use the most recent file
        csv_file = max(csv_files, key=lambda x: x.split('_')[-1])
        print(f"Using most recent CSV file: {csv_file}")

    # Generate plots and summary
    plot_scale_analysis(csv_file)
    create_summary_table(csv_file)

if __name__ == "__main__":
    main()