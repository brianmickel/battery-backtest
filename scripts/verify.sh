#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"

case "${1:-all}" in
  go)
    go test -race -coverprofile=coverage.out ./cmd/... ./internal/...
    go vet ./cmd/... ./internal/...
    go build ./cmd/...
    unformatted="$(find cmd internal -name '*.go' -type f -exec gofmt -l {} +)"
    if [[ -n "$unformatted" ]]; then
      printf 'Unformatted Go files:\n%s\n' "$unformatted" >&2
      exit 1
    fi
    ;;
  web)
    cd web
    npm run test -- --run
    npm run typecheck
    npm run lint
    npm run build
    ;;
  browser)
    cd web
    npm run test:e2e
    ;;
  all)
    "$0" go
    "$0" web
    "$0" browser
    ;;
  *)
    printf 'Usage: %s [all|go|web|browser]\n' "$0" >&2
    exit 2
    ;;
esac
