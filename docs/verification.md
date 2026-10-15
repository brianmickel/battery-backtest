# Verification and proof boundaries

Run the checks from the repository root after installing Go 1.27.2, Node 24, npm dependencies, and Chromium:

```sh
bash scripts/verify.sh
go test -race ./internal/integration/...
(cd web && npm run test:e2e -- system.spec.ts)
bash scripts/smoke-container.sh
```

The full verifier includes race tests, vet, build, formatting, frontend unit tests, type checking, lint, production build, and Chromium tests. System tests use the real router, provider HTTP client, configuration resolver, planner, battery simulation, and UI. A local HTTP fixture supplies deterministic prices; it is the only replaced external boundary. CLI and demo consume the same checked-in JSON and their CSV fields are compared with the API ledger.

Set `INTEGRATION_ARTIFACT_DIR` to retain API request/response and CLI/demo CSVs. Playwright retains system traces and a successful screenshot under `web/test-results`; reports are under `web/playwright-report`. Set `SMOKE_ARTIFACT_DIR` to retain container health, metadata, validation responses, bundle, and safe logs. These artifacts may contain the public synthetic test key, never a real account key.

## Audit finding coverage

Each row names positive behavior and the corresponding negative/regression check. Source paths are executable evidence entry points; the saved release run records exact commands and statuses separately.

| Finding | Positive and negative evidence |
|---|---|
| F1 shared setup/defaults | `internal/integration/system_test.go` compares API, CLI, and demo ledgers; `internal/config/config_test.go` checks metadata defaults against the resolver; `internal/experiment/run_test.go` checks repeatability and invalid local preparation |
| F2 presence, overrides, initial SOC | Config tests cover absent/zero/null, every battery override, changed presets, and incompatible strategy fields; experiment tests retain initial inventory and reject invalid merged variations; editor tests round-trip explicit zero |
| F3 oracle state history | `internal/strategy/oracle_test.go` covers midnight/DST carry, initial inventory, projected energy conservation, tiny enumerated optimum, and stable ties; budget tests reject invalid/extreme work |
| F4 shared physics/ranking | Model pure/mutable sequence and failure tests, engine ledger parity, and `internal/analysis/potential_test.go` variable-duration canonical execution; CLI ranking rejects invalid members atomically |
| F5 one editor draft | `web/tests/e2e/editor.spec.ts` covers delayed metadata, form/JSON round trips, incomplete raw text, coupled SOC edits, strategy switches, invalid imports, and discard/recovery |
| F6 units/dates/windows/clocks | Data/summary tests cover gaps, midnight, DST and repeated hours; presentation tests cover directional MW buckets and weighted windows; draft tests reject invalid Gregorian ranges and malformed response timestamps; system browser displays the real result |
| F7 failure semantics | Integration tests exercise auth/malformed responses, invalid later comparison before I/O, ranking failure after a successful location, and no partial result; API tests cover typed errors/panics/startup; updater tests preserve failures |
| F8 numeric/series invariants | Battery validation and conservation tests; JSON decoder required/null/overflow/timestamp tests; normalized series duplicate/overlap/mixed-clock tests; whole-series validation precedes interval limiting |
| F9 cancellation/resources | Provider transport cancellation/size/timeout tests; planner inner-loop cancellation and checked estimates; experiment aggregate output/work budgets; HTTP admission tests; system browser navigation cancels real provider I/O and later work succeeds |
| F10 preset boundary | Config catalog and API boundary tests reject traversal, unknown IDs, malformed/missing catalogs; local config-relative file loading remains supported |
| F11 credentials | Provider redirect/transport tests prove no header forwarding or safe-error leak; API tests inspect access logs; editor/system browser tests assert header-only credentials and safe failures/copied JSON |
| F12 regression protection | `scripts/verify.sh`, CI jobs, package-specific tests, four real-provider-transport browser system flows, CLI parity, and container smoke; no expected-failure exclusions |
| F13 documentation | Offline quickstart executes both commands and ranking; local synthetic server exercises documented HTTP examples and metadata; help/path checks cover the actual flags; live commands are explicitly marked external |
| F14 cache/operations | Provider no-cache/credential-isolation test; process-barrier updater alias/independent-output/SIGKILL tests; atomic write/close/rename failure tests; container context and runtime checks |

## Smaller cleanup coverage

| Cleanup | Evidence |
|---|---|
| Removed unused BacktestInputs and RankedPotential | All Go packages build and CLI/ranking tests execute the replacements |
| Removed known-ID fallback and shell eval | Updater missing/empty seed tests and literal shell-argument test |
| Removed debug/ledger stub routes and unused handler arguments | API removed-route/startup tests; container 404 JSON assertion |
| Responsive layout, labels, radio groups, clipboard, timers | Editor keyboard/mobile screenshot, label association, clipboard-rejection, delayed metadata and raw-edit tests |
| Wrapped missing-file errors | Location catalog tests preserve error identity; API missing catalog succeeds empty while malformed catalog fails |
| Favicon/demo assets and Docker ignore | Built index/assets checked in container; packaging proof excludes nonsecret sentinels and development files |
| Shared result formatters | Presentation unit tests cover units, finite values, explicit clock and offsets |
| Bundle/runtime investigation | Production bundle size and browser resource entries are recorded; no unmeasured lazy-loading change is included |

## External and measurement boundaries

Synthetic tests prove application behavior under controlled provider responses. They do **not** certify live Grid Status account permissions, quotas, current dataset contents, paid usage, or deployment networking. Live validation requires a real account/key and explicit authorization; it is recorded separately when available.

Docker smoke checks an actual built image and local runtime. It makes only health, static, metadata, and locally rejected API requests, so it does not spend provider quota. Browser resource timings are observations from a local run, not an internet latency benchmark. Optimizer benchmarks and a quiet maximum-accepted-payload peak-memory measurement belong in the release evidence; the 64 MiB optimizer estimate is not a total-process RSS guarantee.

The integrated production build measured 770.27 kB JavaScript (228.67 kB gzip) and 3.19 kB CSS (1.21 kB gzip). The browser fetched one JavaScript bundle, one stylesheet, metadata, and the backtest response; it contacted no third-party origin. Vite reports a bundle-size warning. These local observations do not establish that a particular split would improve user latency, so code splitting remains deferred pending a measured loading scenario.

Quiet precompiled probes on an Apple M4 Max (macOS, Go 1.27.2, October 14, 2026) each sent a 1 MiB HTTP request and a 64 MiB provider response containing 17,280 five-minute intervals:

| Scenario | HTTP | Response bytes | Process wall time | Maximum resident memory |
|---|---|---:|---:|---:|
| One oracle, resolution 200/10, full ledger | 200 | 9,111,831 | 11.30 s | 403.55 MiB |
| Six schedule comparisons, retained outcomes | 200 | 83,260 | 0.18 s | 310.12 MiB |

These single-iteration measurements include the real server, local provider fixture, and test client in one process. They are not isolated server RSS, a total-memory cap, or a universal latency guarantee. Seven comparisons of the same full-length fixture return 422 under the retained-output budget. The quiet oracle benchmark medians were 0.181 seconds for one day and 5.472 seconds for 30 days; the shared one-day experiment runner median was 0.179 seconds. Reproduce payload probes with `BenchmarkMaximumAcceptedHTTPPayload` in `internal/integration/payload_test.go`, running each sub-benchmark separately with `-benchtime=1x` under an OS memory profiler.
