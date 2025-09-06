from typing import Dict, Union, Optional

import pandas as pd
import matplotlib.pyplot as plt
from pathlib import Path

ROOT = Path(__file__).parent

def load_csv(filename: str) -> pd.DataFrame:
    df = pd.read_csv(filename)
    print(f"Loaded {len(df)} data points from {filename}")

    return df

def load_csvs(*filenames: Union[str, Path]) -> Dict[str, pd.DataFrame]:
    return {filename: load_csv(filename) for filename in filenames}

def create_plot(x_label: str, y_label: str, title: str) -> plt.Figure:
    plt.figure(figsize=(10, 6))

    plt.xlabel(x_label)
    plt.ylabel(y_label)
    plt.title(title)
    plt.grid(True, alpha=0.3)
    plt.xscale('log', base=2)  # Log scale with base 2 since we're doubling

    plt.tight_layout()

    return plt

def plot_data(plt: plt.Figure, df: pd.DataFrame, x_col: str, y_col: str, label: Optional[str] = None) -> plt.Figure:
    plt.plot(df[x_col], df[y_col], linewidth=2, markersize=8, label=label)

    # Add value labels on points
    for i, row in df.iterrows():
        plt.annotate(f'{row[y_col]:.0f}',
                     (row[x_col], row[y_col]),
                     textcoords="offset points", xytext=(0, 10), ha='center')

    return plt

def plot_node_scaling_exp(*filenames: Union[str, Path], x_col: str, y_col: str, x_label: str, y_label: str, title: str) -> None:
    all_exp = load_csvs(*filenames)
    plt = create_plot(x_label, y_label, title)

    for file, exp in all_exp.items():
        split_file = file.stem.split('_')
        num_servers, num_clients = split_file[2], split_file[4]
        plot_data(plt, all_exp[file], x_col, y_col, label=f"Servers: {num_servers}, client: {num_clients}")

    plt.legend()
    plt.show()

def main():
    exp_4_nodes_dir = ROOT.joinpath('node_scaling_exp_4_count')

    plot_node_scaling_exp(*exp_4_nodes_dir.glob('*.csv'),
                          x_col='numClients',
                          y_col='throughput_ops_per_sec',
                          x_label='Number of Concurrent Goroutines',
                          y_label='Throughput (ops/s)',
                          title='Client side goroutine scaling across varying client-node combinations')

if __name__ == '__main__':
    main()