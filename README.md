# Battery Backtest Engine

Simulate battery electricity-market arbitrage from Grid Status interval data. The Go API, CLI, demo, and React interface share configuration resolution, physical dispatch, and summary calculations.

The oracle is an **approximate full-horizon foresight optimizer** with discretized SOC and power. It is not an LP solver or a guaranteed profit bound. Results report cash flow over the supplied intervals. Initial stored energy has no acquisition charge, and terminal energy has zero salvage value. These assumptions matter when comparing runs.

## Run an offline example

Use Go 1.27.2 (the toolchain pinned in `go.mod`). From the repository root:

```sh
go mod download
go run ./cmd/cli backtest --data testdata/two-price.json --config docs/synthetic-config.yaml --n 0 --timezone UTC --out results/synthetic.csv
go run ./cmd/demo --data testdata/two-price.json --config docs/synthetic-config.yaml --n 0 --timezone UTC --out results/demo.csv
go run ./cmd/cli rank --data testdata/two-price.json
```

The backtest and demo each charge 1 MWh at $10/MWh, discharge 1 MWh at $100/MWh, and finish empty with $90 cash flow. Their CSVs are identical. Ranking uses its own canonical initial-inventory and optimizer convention; its score is not directly comparable to an arbitrary backtest configuration.

Both backtest commands require `--config`. `--n 0` uses all chronological intervals; demo otherwise defaults to 12. `--timezone market|UTC` controls schedule/display time and defaults to `market`. CLI ranking accepts comma-separated JSON files or a directory, validates every file and series, and fails the whole command on an invalid input. See `go run ./cmd/cli --help` and `go run ./cmd/demo --help`.

## Run the interface without an external account

Use Node 24. From the repository root:

```sh
(cd web && npm ci && npm run build)
go run ./internal/integration/server --listen 127.0.0.1:8080
```

Open `http://127.0.0.1:8080`. Enter the synthetic key `synthetic-release-secret`, choose **JSON Input**, paste [sample_request.json](sample_request.json), apply it, and run the backtest. This development command uses the production router and provider client with a local synthetic provider. It does not contact Grid Status. Stop it with Ctrl-C.

For the production provider, run `go run ./cmd/api` instead. Enter your own Grid Status key in the interface. Requests send it only in `X-GridStatus-API-Key`; the browser stores it locally and excludes it from copied request JSON. Production requests require valid dataset/location IDs and authorized provider access. [API documentation](API_DOCUMENTATION.md) describes the wire contract.

## Configure a run

[docs/synthetic-config.yaml](docs/synthetic-config.yaml) is a complete custom battery. [examples/oracle_config.yaml](examples/oracle_config.yaml) and [examples/schedule_config.yaml](examples/schedule_config.yaml) use battery files relative to the configuration file. In HTTP requests, `battery_file` is a preset ID from `/api/v1/batteries`, never a filesystem path.

Missing fields and explicit zero differ. Preset values are applied first, then explicit overrides. Absent initial SOC defaults to the resolved minimum SOC; a supplied initial SOC is retained. A zero schedule power means idle. `null`, nonfinite values, unknown fields, and parameters for another named strategy are errors.

Date ranges include `start_date` and exclude `end_date`. Equal dates are invalid; ranges cannot exceed 60 calendar days or end after the current UTC date. Data gaps are permitted and reported, but duplicates, overlaps, mixed locations/markets, invalid durations, and inconsistent UTC/local instants fail validation.

## Develop and verify

```sh
(cd web && npm ci && npx playwright install chromium)
bash scripts/verify.sh
go test -race ./internal/integration/...
bash scripts/smoke-container.sh
```

The full verifier runs Go race tests, vet/build/format checks, frontend unit/type/lint/build checks, and Chromium tests. Docker smoke requires Docker, curl, and Python 3; it builds and starts a local nonroot container, checks health/UI/assets/metadata/errors, and removes the container. [Verification](docs/verification.md) distinguishes local synthetic evidence from external-account validation.

For separate development servers, run `go run ./cmd/api` and `(cd web && npm run dev)`. Vite serves port 5173 and proxies `/api` to port 8080. [Frontend quickstart](web/QUICKSTART.md) covers editor behavior. [Operations](docs/operations.md) covers configuration, containers, and catalog refresh. [Migration](docs/migration.md) lists breaking changes and rollback.
