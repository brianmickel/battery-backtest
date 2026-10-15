# Migration and rollback

Deploy the frontend and server together. There is no database migration and no compatibility layer for the removed contracts. Preserve an existing location catalog before operational changes. Roll back by deploying the complete previous frontend/server image together; mixing versions can submit invalid requests or misread errors.

| Previous behavior/input | Current behavior/input | Owning requirement |
|---|---|---|
| An omitted number could become zero or be replaced by a default | Omission inherits/defaults; an explicit `0` remains zero; `null` is invalid | CONFIG-PRESENCE |
| `initial_soc: 0.5` could be silently reset to minimum | The supplied initial SOC is used; omission defaults to resolved minimum | CONFIG-PRESENCE, RUN-EXECUTE |
| Starting energy was hidden by resetting inventory | Initial/final MWh and the no-acquisition/no-salvage cash-flow assumptions are visible | PLAN-HORIZON, SUMMARY-TOTALS |
| Schedule defaults differed between entry points | Charge 10:00 to resolved discharge start; discharge 17:00 to 23:59; power defaults to battery power | CONFIG-STRATEGY |
| `charge_power_mw: 0` could become full power | Explicit zero produces idle dispatch | CONFIG-PRESENCE |
| Switching oracle to schedule could retain oracle fields | A strategy switch discards incompatible inherited parameters; explicitly incompatible fields are errors | CONFIG-PRESENCE |
| HTTP `battery_file: "examples/batteries/1_moss_landing.yaml"` | HTTP `battery_file: "1_moss_landing"`; local YAML paths remain relative to the config file | CONFIG-CATALOG |
| Oracle planning reset at daily boundaries | One full-horizon plan carries inventory across midnight and DST; it remains approximate | PLAN-HORIZON, PLAN-APPROXIMATION |
| Start/end on the same date could mean one day | Use `2024-01-01` to `2024-01-02` for January 1; end is excluded | DRAFT-CALENDAR, HTTP-BOUNDARIES |
| Invalid/overlapping rows or missing prices could pass through | Strict decode and normalized series validation fail before execution; actual durations and gaps are retained | SERIES-DECODE, SERIES-NORMALIZE |
| Comparison/ranking could skip failed entries and return success | Any requested failure returns one error, no partial success array; rank validates all requested locations before top-N output | RUN-COMPARE, HTTP-EXECUTION |
| Body/query `api_key` | Header `X-GridStatus-API-Key`; legacy placement is a safe 400; valid UI imports move the key to separate local credential state | DRAFT-CREDENTIAL, HTTP-BOUNDARIES |
| A result ID implied a ledger download endpoint | Request `options.include_ledger: true`; the old permanent-501 route and unused ID are removed | HTTP-DELETE |
| Development response cache and cache environment variables | No response cache; credentials and requests are isolated | PROVIDER-NOCACHE, HTTP-LIFECYCLE |
| Separate form and JSON defaults could overwrite edits | One committed draft plus visible pending text; apply/correct/discard atomically | EDITOR-ATOMIC, EDITOR-MODES |
| Windows could span idle intervals or missing data | Contiguous actions only; gaps/idle/opposite direction split windows; prices use grid-energy weights | SUMMARY-WINDOWS |
| Chart sample powers were summed | Fixed 30-minute UTC buckets display directional MWh / 0.5 hours in MW | PRESENT-POWER |
| Browser locale implicitly controlled time | Explicit market/UTC formatting preserves market offsets and labels UTC | PRESENT-CLOCK |
| Demo ran without a config; invalid CLI input could panic | Demo requires `--config`; normal errors exit nonzero without a stack; `--n 0` means all for both backtest commands | CLI-PARITY, CLI-FAILURES |
| Catalog refresh could overwrite seeded or concurrent changes | Destination is the base, seeds add IDs, failures retain old entries, and a stable exclusive lock protects atomic replacement | OPS-REFRESH, OPS-FILES |

Before submitting a comparison, inspect its merged configurations rather than assuming the base is valid alone. An incomplete base is permitted when every variation supplies the missing values. All variation names must be nonempty and unique.

Clients should handle the standard error envelope and the resource/admission limits in [API documentation](../API_DOCUMENTATION.md). A 503 with `SERVER_BUSY` means the process already has an expensive operation; `Retry-After: 1` is advice for a later retry. Do not treat it as an empty successful result.
