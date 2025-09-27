# Theta Parameter Analysis

This directory contains scripts to analyze the impact of the theta parameter (Zipfian skew) on distributed transaction performance.

## Scripts

### 1. `theta_test.py` - Automated Testing
Runs comprehensive tests across different theta values and workloads.

**Features:**
- Tests theta values: 0, 0.3, 0.5, 0.7, 0.9, 0.99
- Tests both YCSB-A (50% writes) and YCSB-B (5% writes)
- Uses 1 server + 3 clients for maximum contention
- Saves results to timestamped CSV files

### 2. `plot_theta_analysis.py` - Visualization
Generates comprehensive plots and analysis from CSV data.

**Features:**
- 4 different plots: commit rate, abort rate, ops rate, efficiency
- Comparison between YCSB-A and YCSB-B workloads
- Summary table with key metrics
- Saves high-quality PNG and PDF outputs

## Usage

### Step 1: Install Dependencies
```bash
pip3 install matplotlib pandas numpy
```

### Step 2: Run Tests
```bash
# Make scripts executable
chmod +x theta_test.py plot_theta_analysis.py

# Run the comprehensive test suite (takes ~30 minutes)
python3 theta_test.py
```

### Step 3: Generate Plots
```bash
# Plot the most recent results
python3 plot_theta_analysis.py

# Or specify a specific CSV file
python3 plot_theta_analysis.py theta_analysis_1635123456.csv
```

## Expected Results

### YCSB-B (5% writes, 95% reads)
- **Low abort rates** (1-5%) across all theta values
- **High commit rates** due to read-heavy workload
- **Minimal theta impact** due to concurrent reads

### YCSB-A (50% writes, 50% reads)
- **Higher abort rates** (5-20%) especially at high theta
- **Clear theta impact** with decreasing commit rates
- **Efficiency degradation** at high contention levels

## Interpretation

### Key Metrics
- **Commit/s**: Actual useful work completed
- **Abort Rate**: Percentage of failed transactions
- **Efficiency**: Ratio of commits to total operations
- **Theta Impact**: How skew affects performance

### What to Look For
1. **Inverse relationship**: Higher theta → lower commits/s
2. **Workload differences**: YCSB-A more sensitive to theta
3. **Efficiency trends**: Lower efficiency at higher contention
4. **Sweet spots**: Optimal theta values for your use case

## Output Files

- `theta_analysis_<timestamp>.csv`: Raw test data
- `theta_analysis_<timestamp>_analysis.png`: Visualization
- `theta_analysis_<timestamp>_analysis.pdf`: High-quality version

## Troubleshooting

### Common Issues
1. **"run-cluster.sh not found"**: Run from project root directory
2. **"No CSV files found"**: Run theta_test.py first
3. **Import errors**: Install required Python packages

### Test Duration
- Full test suite: ~30 minutes (6 theta × 2 workloads × 30 seconds each)
- Quick test: Modify THETA_VALUES in theta_test.py for fewer data points

### Configuration
Edit `theta_test.py` to modify:
- `CLUSTER_CONFIG`: Server/client configuration
- `TEST_DURATION`: Length of each test
- `THETA_VALUES`: Which theta values to test