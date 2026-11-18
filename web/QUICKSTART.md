# Interface quickstart

From the repository root, with Go 1.27.2 and Node 24 installed:

```sh
go mod download
(cd web && npm ci && npm run build)
go run ./internal/integration/server --listen 127.0.0.1:8080
```

Open `http://127.0.0.1:8080`. This local fixture server runs the real application with synthetic market data and makes no external provider requests.

1. Enter `synthetic-release-secret` in **Grid Status API Key**.
2. Select **JSON Input** and paste [sample_request.json](../sample_request.json).
3. Select **Apply JSON**, then **Run backtest**.
4. Confirm $90 total PNL, two intervals, and empty final inventory. The charts show 1 MW charging followed by 1 MW discharging.
5. Import [compare-request.json](../testdata/compare-request.json) to compare idle at explicit zero with the oracle. Results retain that order and show $0 and $90.

The form preserves raw edits until the entire candidate is valid. Incomplete numbers, invalid date ranges, or inconsistent SOC bounds keep the text visible and block submission/copy. Correct them together or choose **Discard form edits**. JSON edits require **Apply JSON**; invalid JSON remains editable. Comparison requests remain in JSON mode. Error links focus the associated form field when available.

The result view reports initial/final energy, cash-flow assumptions, coverage and gaps, contiguous charging/discharging windows, and explicit market/UTC time. Chart power is an average over fixed 30-minute buckets. **Copy request JSON** excludes credentials; **Copy response JSON** copies the result.

To use Grid Status, stop the synthetic server and run `go run ./cmd/api`, then use your real key and authorized dataset/location/date range. External provider access is separate from the verified synthetic walkthrough.
