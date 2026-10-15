#!/usr/bin/env bash
# Usage: update-locations.sh [dataset_id] [output_file] [seed_file]
set -euo pipefail

if [[ -z "${GRIDSTATUS_API_KEY:-}" ]]; then
    echo "Error: GRIDSTATUS_API_KEY environment variable is required" >&2
    exit 1
fi

args=(go run ./cmd/update-locations --dataset-id "${1:-caiso_lmp_real_time_5_min}")
if [[ -n "${2:-}" ]]; then
    args+=(--output "$2")
fi
if [[ -n "${3:-}" ]]; then
    args+=(--seed "$3")
fi
if (( $# > 3 )); then
    echo "Usage: update-locations.sh [dataset_id] [output_file] [seed_file]" >&2
    exit 2
fi
exec "${args[@]}"
