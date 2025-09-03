#!/usr/bin/env python3

import pandas as pd
import matplotlib.pyplot as plt
import sys
import os

def plot_client_scaling(csv_file='client_scaling_results.csv'):
    """
    Plot the client scaling results showing throughput vs number of clients
    """
    if not os.path.exists(csv_file):
        print(f"Error: Results file '{csv_file}' not found.")
        print("Run the benchmark script first: ./benchmark-clients.sh")
        sys.exit(1)
    
    # Read the CSV data
    try:
        df = pd.read_csv(csv_file)
        print(f"Loaded {len(df)} data points from {csv_file}")
    except Exception as e:
        print(f"Error reading CSV file: {e}")
        sys.exit(1)
    
    # Create the plot
    plt.figure(figsize=(12, 8))
    
    # Plot throughput vs number of clients
    plt.subplot(2, 1, 1)
    plt.plot(df['numClients'], df['throughput_ops_per_sec'], 'bo-', linewidth=2, markersize=8)
    plt.xlabel('Number of Clients')
    plt.ylabel('Throughput (ops/s)')
    plt.title('KVS Client Scaling: Throughput vs Number of Clients')
    plt.grid(True, alpha=0.3)
    plt.xscale('log', base=2)  # Log scale with base 2 since we're doubling
    
    # Add value labels on points
    for i, row in df.iterrows():
        plt.annotate(f'{row["throughput_ops_per_sec"]:.0f}', 
                    (row['numClients'], row['throughput_ops_per_sec']),
                    textcoords="offset points", xytext=(0,10), ha='center')
    
    # Plot efficiency (throughput per client)
    plt.subplot(2, 1, 2)
    df['efficiency'] = df['throughput_ops_per_sec'] / df['numClients']
    plt.plot(df['numClients'], df['efficiency'], 'ro-', linewidth=2, markersize=8)
    plt.xlabel('Number of Clients')
    plt.ylabel('Efficiency (ops/s per client)')
    plt.title('Client Efficiency: Throughput per Client vs Number of Clients')
    plt.grid(True, alpha=0.3)
    plt.xscale('log', base=2)
    
    # Add value labels on points
    for i, row in df.iterrows():
        plt.annotate(f'{row["efficiency"]:.0f}', 
                    (row['numClients'], row['efficiency']),
                    textcoords="offset points", xytext=(0,10), ha='center')
    
    plt.tight_layout()
    
    # Save the plot
    output_file = 'client_scaling_plot.png'
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Plot saved as: {output_file}")
    
    # Display statistics
    print("\n=== Results Summary ===")
    print(df.to_string(index=False))
    
    print(f"\nPeak throughput: {df['throughput_ops_per_sec'].max():.2f} ops/s with {df.loc[df['throughput_ops_per_sec'].idxmax(), 'numClients']} clients")
    print(f"Best efficiency: {df['efficiency'].max():.2f} ops/s per client with {df.loc[df['efficiency'].idxmax(), 'numClients']} clients")
    
    # Show the plot
    plt.show()

def main():
    if len(sys.argv) > 1:
        csv_file = sys.argv[1]
    else:
        csv_file = 'client_scaling_results.csv'
    
    plot_client_scaling(csv_file)

if __name__ == '__main__':
    main()