#!/usr/bin/env python3

import pandas as pd
import matplotlib.pyplot as plt
import sys
import os

def plot_batch_scaling(csv_file='batch_scaling_results.csv'):
    """
    Plot the batch scaling results showing throughput vs batch size
    """
    if not os.path.exists(csv_file):
        print(f"Error: Results file '{csv_file}' not found.")
        print("Run the benchmark script first: ./benchmark-batch.sh")
        sys.exit(1)
    
    # Read the CSV data
    try:
        df = pd.read_csv(csv_file)
        print(f"Loaded {len(df)} data points from {csv_file}")
    except Exception as e:
        print(f"Error reading CSV file: {e}")
        sys.exit(1)
    
    # Create the plot
    plt.figure(figsize=(10, 6))
    
    # Plot throughput vs batch size
    plt.plot(df['batchSize'], df['throughput_ops_per_sec'], 'go-', linewidth=2, markersize=8)
    plt.xlabel('Batch Size')
    plt.ylabel('Throughput (ops/s)')
    plt.title('KVS Batch Size Scaling: Throughput vs Batch Size')
    plt.grid(True, alpha=0.3)
    plt.xscale('log', base=2)  # Log scale with base 2 since we're doubling
    
    # Add value labels on points
    for i, row in df.iterrows():
        plt.annotate(f'{row["throughput_ops_per_sec"]:.0f}', 
                    (row['batchSize'], row['throughput_ops_per_sec']),
                    textcoords="offset points", xytext=(0,10), ha='center')
    
    plt.tight_layout()
    
    # Save the plot
    output_file = 'batch_scaling_plot.png'
    # plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Plot saved as: {output_file}")
    
    # Display statistics
    print("\n=== Results Summary ===")
    print(df.to_string(index=False))
    
    print(f"\nPeak throughput: {df['throughput_ops_per_sec'].max():.2f} ops/s with batch size {df.loc[df['throughput_ops_per_sec'].idxmax(), 'batchSize']}")
    
    # Show the plot
    plt.show()

def main():
    if len(sys.argv) > 1:
        csv_file = sys.argv[1]
    else:
        csv_file = 'batch_scaling_results.csv'
    
    plot_batch_scaling(csv_file)

if __name__ == '__main__':
    main()