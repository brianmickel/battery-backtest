#!/bin/bash
# Script to update the locations list from Grid Status API
# Usage: ./scripts/update-locations.sh [dataset_id] [output_file]

set -e

# Get dataset ID from argument or use default
DATASET_ID="${1:-caiso_lmp_real_time_5_min}"
OUTPUT_FILE="${2:-}"

# Check for API key
if [ -z "$GRIDSTATUS_API_KEY" ]; then
    echo "Error: GRIDSTATUS_API_KEY environment variable is required"
    exit 1
fi

# Build command
CMD="go run ./cmd/update-locations --dataset-id \"$DATASET_ID\""

if [ -n "$OUTPUT_FILE" ]; then
    CMD="$CMD --output \"$OUTPUT_FILE\""
fi

# Run the Go command to update locations
echo "Updating locations for dataset: $DATASET_ID"
eval $CMD

echo ""
echo "Locations updated successfully!"
