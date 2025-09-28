# Theta Parameter Analysis

Tests how theta parameter (Zipfian skew) affects transaction performance.

## Usage

### 1. Install Dependencies
```bash
pip3 install matplotlib pandas numpy
```

### 2. Run Tests
```bash
chmod +x theta_test.py
python3 theta_test.py
```

### 3. Generate Plots
```bash
python3 plot_theta_analysis.py
```

## What It Does

- Tests theta values: 0, 0.3, 0.5, 0.7, 0.9, 0.99
- Tests YCSB-A (50% writes) and YCSB-B (5% writes)
- Runs 30 seconds per test (~5 minutes total)
- Saves results to CSV and generates plots

## Output Files

- `theta_analysis_<timestamp>.csv` - Raw data
- `theta_analysis_<timestamp>_analysis.png` - Charts

## Requirements

- Must run from project root directory
- Requires `run-cluster.sh` and `report-tput.py`