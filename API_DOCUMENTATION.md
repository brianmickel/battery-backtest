# HTTP API

The server exposes `/api/v1` and `/health`. Start it from the repository root with `go run ./cmd/api`; the default port is 8080. A built `web/dist` is served automatically when present. No database is required.

## Credentials and requests

Send credentials only in `X-GridStatus-API-Key`. Body/query `api_key` fields are rejected with safe migration guidance. Metadata and health need no key. The following request works against the local synthetic server described in [README](README.md):

```sh
curl --fail-with-body -sS http://127.0.0.1:8080/api/v1/backtest \
  -H 'Content-Type: application/json' \
  -H 'X-GridStatus-API-Key: synthetic-release-secret' \
  --data-binary @sample_request.json
```

[sample_request.json](sample_request.json) contains a complete custom battery, an oracle strategy, a data source, and `options.include_ledger: true`. The synthetic result has two ledger rows and `summary.total_pnl: 90`. Against the production provider, replace the key, dataset/location IDs, and dates with values your account can query.

| Route | Meaning |
|---|---|
| `GET /health` | Process health; `{ "status": "ok" }` |
| `POST /api/v1/backtest` | Execute one configuration |
| `POST /api/v1/backtest/compare` | Execute ordered variations over one fetched series |
| `GET /api/v1/rank` | Rank all requested locations, then apply the output limit |
| `GET /api/v1/batteries` | Preset IDs, names, and capacity/power specs |
| `GET /api/v1/strategies` | Supported parameters and resolver defaults |
| `GET /api/v1/datasets` | Supported dataset listing |
| `GET /api/v1/locations?dataset_id=…` | Optional local location catalog |

There is no stored-run ID or ledger-download route. Request `include_ledger` in the original backtest. Unknown API routes return a JSON error and 404.

## Configuration and time

`data_source` requires `type: "gridstatus"`, `dataset_id`, `location_id`, `start_date`, and `end_date`; `timezone` is `market` or `UTC` and defaults to `market`. Dates must be Gregorian `YYYY-MM-DD`. The start is included and end excluded; the range must be positive, at most 60 calendar days, and end no later than today in UTC.

`config` accepts `battery_file` (an advertised preset ID), a `battery` override object, and `strategy`. Without a preset, capacity, power, efficiencies, and min/max SOC are required. Name defaults to empty, degradation cost to zero, and initial SOC to the resolved minimum. Explicit zero values remain explicit. All values must be finite; all fields reject `null`. Filesystem paths and unknown preset IDs are invalid.

| Strategy | Parameters and defaults |
|---|---|
| `oracle` | `soc_steps: 200` (2–1,000); `power_steps: 10` (1–50) |
| `schedule` | `charge_start: "10:00"`; `charge_end` defaults to resolved `discharge_start`; `discharge_start: "17:00"`; `discharge_end: "23:59"`; charge/discharge power each defaults to resolved battery power |

Schedule times are interval-start predicates in the selected clock. Windows are start-inclusive/end-exclusive, may cross midnight, and equal endpoints mean an empty window. Power parameters are nonnegative magnitudes; zero is idle. Unknown or incompatible strategy parameters are rejected. Initial energy has no purchase cost and final energy has no salvage credit.

## Comparison and ranking

POST [testdata/compare-request.json](testdata/compare-request.json) to `/api/v1/backtest/compare` with the header above. It replaces `config` with `base_config` and a nonempty `variations` array of `{name, config}` patches. Names must be unique. Base fields are merged into each variation before resolution; switching strategy removes incompatible inherited parameters. Every variation must validate before fetching. One common series is fetched, each run starts fresh, and response order matches input order. The example yields $0 then $90.

Ranking is a GET request:

```sh
curl --fail-with-body -sS \
  'http://127.0.0.1:8080/api/v1/rank?dataset_id=synthetic_lmp&start_date=2024-01-01&end_date=2024-01-02&location_ids=TEST_NODE&limit=1' \
  -H 'X-GridStatus-API-Key: synthetic-release-secret'
```

Ranking uses the canonical 1 MWh / 1 MW battery, 100% charge/discharge efficiencies, SOC bounds 0–1, initial SOC 0.5, zero degradation cost, and oracle resolution 200/1. Prices and actual interval durations drive the score. Ties sort by location ID. Supply 1–16 unique locations; `limit` is 1–16 and defaults to 10. A failed location fails the entire response even if a smaller output limit was requested.

## Results and errors

Backtest responses contain `status: "completed"`, `summary`, and an optional `ledger`. Summary fields include PNL, initial/final SOC and MWh, grid energy charged/discharged, interval count, clock, covered/gap hours, gap count, full window, and contiguous directional windows. Idle intervals, opposite actions, and data gaps split windows. Average prices are weighted by grid-side MWh.

Ledger timestamps carry UTC and market offsets. API/CSV power uses positive discharge and negative charge. UI charts label charging positive and discharging negative; each 30-minute bucket displays average MW, not summed samples. Missing bucket coverage is idle for the average and is identified separately.

All errors have `{ "error": { "code": "…", "message": "…", "field": "…", "variation": "…", "location_id": "…" } }`; the last three fields are optional. Comparison/ranking failures return no partial success array.

| HTTP | Code | Meaning |
|---|---|---|
| 400 | `INVALID_REQUEST` | Invalid config, series, dates, credentials placement, or query |
| 401 | `UPSTREAM_AUTH` | Provider rejected the key |
| 404 | `NOT_FOUND` | Unknown route/asset |
| 413 | `BODY_TOO_LARGE` | HTTP body exceeds 1 MiB |
| 422 | `WORK_BUDGET_EXCEEDED` | Interval or optimizer budget exceeded |
| 429 | `UPSTREAM_RATE_LIMIT` | Provider throttling; `Retry-After` propagated |
| 500 | `INTERNAL_ERROR` | Safe generic internal failure |
| 502 | `UPSTREAM_UNAVAILABLE` | Provider transport/response failure |
| 503 | `SERVER_BUSY` | Another expensive operation is active; `Retry-After: 1` |
| 504 | `DEADLINE_EXCEEDED` | Provider/application time budget expired |

The process admits one backtest, comparison, or ranking at a time with no queue. Health/metadata/static routes remain available. Limits are 64 MiB provider body, 17,280 intervals, 16 variations/locations, 100 million estimated transitions per request, and 64 MiB estimated optimizer working allocation. Provider calls have 30 seconds; the application request has 60 seconds. Client disconnects cancel work. These caps are admission rules, not guarantees about total process memory or completion speed.
