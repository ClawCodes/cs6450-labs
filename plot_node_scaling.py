from typing import Dict, Union, Optional
import os

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


def plot_data(plt: plt.Figure, df: pd.DataFrame, x_col: str, y_col: str, annotate: bool = True,
              label: Optional[str] = None) -> plt.Figure:
    plt.plot(df[x_col], df[y_col], linewidth=2, markersize=8, label=label)

    # Add value labels on points
    if annotate:
        for i, row in df.iterrows():
            plt.annotate(f'{row[y_col]:.0f}',
                         (row[x_col], row[y_col]),
                         textcoords="offset points", xytext=(0, 10), ha='center')

    return plt


def plot_node_scaling_exp(*filenames: Union[str, Path], x_col: str, y_col: str, x_label: str, y_label: str,
                          title: str, outfile: Optional[Union[str, Path]], annotate: bool = True) -> None:
    all_exp = load_csvs(*filenames)
    plt = create_plot(x_label, y_label, title)

    for file, exp in all_exp.items():
        split_file = file.stem.split('_')
        num_servers, num_clients = split_file[3], split_file[5]
        plot_data(plt, all_exp[file], x_col, y_col, annotate=annotate,
                  label=f"Servers: {num_servers}, clients: {num_clients}")

    plt.legend()

    if outfile:
        plt.savefig(outfile)
    else:
        plt.show()

def plot_single_exp(filename: str, x_col: str, y_col: str, x_label: str, y_label: str,
                    title: str, outfile: Optional[Union[str, Path]], annotate: bool = True) -> None:
    df = load_csv(filename)
    plt = create_plot(x_label, y_label, title)
    plot_data(plt, df, x_col, y_col, annotate=annotate)

    if outfile:
        plt.savefig(outfile)
    else:
        plt.show()


def main():
    exp_8_nodes_dir = ROOT.joinpath('node_scaling_exp_8_nodes')
    save_dir = ROOT.joinpath('charts and stats/')

    os.makedirs(save_dir, exist_ok=True)

    # Throughput
    plot_node_scaling_exp(*exp_8_nodes_dir.glob('*.csv'),
                          x_col='thread_count',
                          y_col='ops_per_sec',
                          x_label='Number of Concurrent Goroutines',
                          y_label='Throughput (ops/s)',
                          title='Throughput under client-side goroutine scaling across varying client-node combinations',
                          outfile=save_dir.joinpath('throughput_thread_scaling_8_nodes.png'),
                          annotate=False)

    # Abort
    plot_node_scaling_exp(*exp_8_nodes_dir.glob('*.csv'),
                          x_col='thread_count',
                          y_col='aborts_per_sec',
                          x_label='Number of Concurrent Goroutines',
                          y_label='Aborts (abort count/s)',
                          title='Transaction Aborts under client-side goroutine scaling across varying client-node combinations',
                          outfile=save_dir.joinpath('aborts_thread_scaling_8_nodes.png'),
                          annotate=False)

    #Commits
    plot_node_scaling_exp(*exp_8_nodes_dir.glob('*.csv'),
                          x_col='thread_count',
                          y_col='commits_per_sec',
                          x_label='Number of Concurrent Goroutines',
                          y_label='Commits (commit count/s)',
                          title='Transaction Commits under client-side goroutine scaling across varying client-node combinations',
                          outfile=save_dir.joinpath('commits_thread_scaling_8_nodes.png'),
                          annotate=False)

if __name__ == '__main__':
    main()