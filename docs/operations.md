# Operations

## Server and container

Run from the repository root:

```sh
go run ./cmd/api
docker build -t battery-backtest:local .
docker run --rm -p 8080:8080 battery-backtest:local
```

The last two commands are an alternative to the first. The image contains the API binary, built UI, preset catalog, location catalog, certificates, and timezone data. It runs as UID 1000. The automated smoke script also proves operation with a read-only filesystem and dropped capabilities:

```sh
bash scripts/smoke-container.sh
```

| Environment variable | Default / meaning |
|---|---|
| `API_PORT` | `8080` |
| `BATTERY_DIR` | `examples/batteries`; required nonempty valid preset catalog |
| `STATIC_DIR` | Built `web/dist` when available; production image sets it explicitly |
| `LOCATIONS_FILE` | `./data/locations.json`; missing optional catalog returns an empty list |
| `CORS_ALLOWED_ORIGINS` | Empty means same-origin; otherwise comma-separated exact HTTP(S) origins |
| `GIN_MODE` | Image uses `release` |

There are no response-cache environment variables. The API takes provider credentials per request; the updater alone reads `GRIDSTATUS_API_KEY`. Logs contain method, path, status, safe error code, and elapsed time, excluding raw query values and keys. The process stops accepting requests on SIGINT/SIGTERM and allows up to ten seconds for shutdown. HTTP header/read/write/idle timeouts are 5/15/65/60 seconds.

## Refresh the location catalog

The updater requires an authorized Grid Status account/key and network access. These live commands are documented but external-account execution is not part of synthetic verification:

```sh
go run ./cmd/update-locations --help
# With GRIDSTATUS_API_KEY supplied securely in the environment:
go run ./cmd/update-locations --dataset-id caiso_lmp_real_time_5_min --output data/locations.json --days 7
bash scripts/update-locations.sh caiso_lmp_real_time_5_min data/locations.json
```

Use `--seed path/to/additional-locations.json` (or the script's third positional argument) to add locations. An existing destination remains the base and its metadata wins for existing IDs. Destination and seed must have the selected dataset. Missing destinations require a valid nonempty seed; there is no hidden fallback catalog.

A complete refresh writes all refreshed entries. A partial refresh writes successes while preserving failed entries and exits nonzero. An entirely failed or canceled refresh does not replace the destination. Output uses a temporary file in the same directory and an atomic rename after write/close succeeds.

A stable `<output>.lock` sidecar serializes refreshes on macOS/Linux. Parent directory symlink aliases resolve to the same lock. Busy refreshes fail before provider calls; different destinations can proceed independently. The lock remains after completion and releases on process exit. Do not delete it to bypass a running update. Final output symlinks, unsafe lock types/hardlinks, reserved `.lock`/temporary output names, and unsupported platforms fail closed. The output directory must already exist.

The local updater suite covers provider failure, write/close/rename failure, cancellation, competing processes, alias paths, SIGKILL release, and shell argument preservation:

```sh
go test -race ./cmd/update-locations/... ./internal/data/...
```
