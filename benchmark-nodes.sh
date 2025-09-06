function cluster_size() {
    geni-get -a | \
        grep -Po '<interface_ref client_id=\\".*?\"' | \
        sed 's/<interface_ref client_id=\\"\(.*\)\\"/\1/' | \
        sort | \
        uniq | \
        wc -l
}

AVAILABLE_COUNT=$(cluster_size)

EXPERIMENT=""
DIRECTORY=""

# Parse commandline flags
while getopts "e:d:" opt; do
  case $opt in
    e) EXPERIMENT="$OPTARG" ;;
    d) DIRECTORY="$OPTARG" ;;
    *)
      echo "Usage: $0 -e <batch|client> -d <directory to save data to>"
      exit 1
      ;;
  esac
done

# Check required flags
if [[ -z "$EXPERIMENT" || -z "$DIRECTORY" ]]; then
  echo "Error: both -e and -d are required"
  echo "Usage: $0 -e <batch|client> -d <directory to save data to>"
  exit 1
fi

# Validate experiment type
if [[ "$EXPERIMENT" != "batch" && "$EXPERIMENT" != "client" ]]; then
  echo "Error: -e must be 'batch' or 'client'"
  exit 1
fi

# Expand DIRECTORY to absolute path
DIRECTORY="$(realpath "$DIRECTORY")"

mkdir -p "$DIRECTORY" # Create dir if not exists

echo "Experiment: $EXPERIMENT"
echo "Directory:  $DIRECTORY"

export CSV_DIR=$DIRECTORY # Export CSV_DIR for child benchmark script

NUM_CLIENT_THREADS=128
MIN_BATCH=1024
MAX_BATCH=131072
for ((num_servers=1; num_servers<AVAILABLE_COUNT; num_servers++)); do
  num_clients=$((AVAILABLE_COUNT - num_servers))
  # Set env var for child benchmark script
  export CSV_FILE_NAME="${EXPERIMENT}_scaling_${num_servers}_servers_${num_clients}_clients.csv"

  echo "Running experiment with $num_servers servers and $num_clients clients"
  echo "Results will be saved to $CSV_FILE_NAME"
  if [[ $EXPERIMENT == "client" ]]; then
    ./benchmark-clients.sh $NUM_CLIENT_THREADS $num_servers $num_clients
  else
    ./benchmark-batch.sh $MIN_BATCH $MAX_BATCH $num_servers $num_clients $NUM_CLIENT_THREADS
  fi
done


