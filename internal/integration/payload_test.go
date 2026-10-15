package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/api"
	"battery-backtest/internal/api/models"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

// Run each sub-benchmark alone with -benchtime=1x in a precompiled test binary
// under the OS memory profiler. These are payload probes, not latency promises.
func BenchmarkMaximumAcceptedHTTPPayload(b *testing.B) {
	for _, scenario := range []string{"single_oracle", "comparison_7_schedule"} {
		b.Run(scenario, func(b *testing.B) {
			rows := make([]model.LMPInterval, strategy.MaxIntervals)
			start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			for i := range rows {
				from := start.Add(time.Duration(i) * 5 * time.Minute)
				rows[i] = model.LMPInterval{IntervalStartUTC: from, IntervalStartLocal: from, IntervalEndUTC: from.Add(5 * time.Minute), IntervalEndLocal: from.Add(5 * time.Minute), Location: "TEST_NODE", Market: "SYNTHETIC", LocationType: "NODE", LMP: float64(i % 24)}
			}
			body, err := json.Marshal(model.GridStatusLMPResponse{Data: rows})
			if err != nil {
				b.Fatal(err)
			}
			const providerBytes = 64 << 20
			if len(body) > providerBytes {
				b.Fatal("fixture exceeds provider limit")
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") != SentinelKey || r.URL.Query().Get("download") != "true" {
					http.Error(w, "invalid fixture request", 400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Length", fmt.Sprint(providerBytes))
				_, _ = w.Write(body)
				padding := strings.Repeat(" ", 4096)
				for remaining := providerBytes - len(body); remaining > 0; {
					count := min(remaining, len(padding))
					if _, err := io.WriteString(w, padding[:count]); err != nil {
						return
					}
					remaining -= count
				}
			}))
			defer provider.Close()
			client, err := data.NewGridStatusClient(provider.Client(), provider.URL)
			if err != nil {
				b.Fatal(err)
			}
			root, err := filepath.Abs("../..")
			if err != nil {
				b.Fatal(err)
			}
			presets, err := config.LoadPresetCatalog(filepath.Join(root, "examples/batteries"))
			if err != nil {
				b.Fatal(err)
			}
			router, err := api.NewRouter(api.Dependencies{Provider: client, Presets: presets})
			if err != nil {
				b.Fatal(err)
			}
			server := httptest.NewServer(router)
			defer server.Close()
			battery := map[string]any{"energy_capacity_mwh": 1, "power_capacity_mw": 1, "charge_efficiency": 1, "discharge_efficiency": 1, "min_soc": 0, "max_soc": 1, "initial_soc": 0}
			request := map[string]any{"data_source": map[string]any{"type": "gridstatus", "dataset_id": "synthetic_lmp", "location_id": "TEST_NODE", "start_date": "2024-01-01", "end_date": "2024-03-01", "timezone": "UTC"}}
			endpoint := "/api/v1/backtest"
			if scenario == "single_oracle" {
				request["config"] = map[string]any{"battery": battery, "strategy": map[string]any{"name": "oracle", "params": map[string]int{"soc_steps": 200, "power_steps": 10}}}
				request["options"] = map[string]bool{"include_ledger": true}
			} else {
				endpoint += "/compare"
				request["base_config"] = map[string]any{"battery": battery, "strategy": map[string]any{"name": "schedule"}}
				variations := make([]map[string]any, 7)
				for i := range variations {
					variations[i] = map[string]any{"name": fmt.Sprintf("schedule-%d", i), "config": map[string]any{}}
				}
				request["variations"] = variations
			}
			encoded, err := json.Marshal(request)
			if err != nil {
				b.Fatal(err)
			}
			encoded = append(encoded, bytes.Repeat([]byte(" "), (1<<20)-len(encoded))...)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req, err := http.NewRequest(http.MethodPost, server.URL+endpoint, bytes.NewReader(encoded))
				if err != nil {
					b.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-GridStatus-API-Key", SentinelKey)
				resp, err := server.Client().Do(req)
				if err != nil {
					b.Fatal(err)
				}
				raw, err := io.ReadAll(resp.Body)
				closeErr := resp.Body.Close()
				if closeErr != nil {
					b.Fatal(closeErr)
				}
				if err != nil || resp.StatusCode != 200 {
					b.Fatalf("HTTP %d: %v %s", resp.StatusCode, err, raw)
				}
				if scenario == "single_oracle" {
					var result models.BacktestResponse
					if err := json.Unmarshal(raw, &result); err != nil || len(result.Ledger) != strategy.MaxIntervals || result.Summary.TotalIntervals != strategy.MaxIntervals {
						b.Fatalf("invalid single result: %v", err)
					}
				} else {
					var result models.CompareBacktestResponse
					if err := json.Unmarshal(raw, &result); err != nil || len(result.Comparison) != 7 {
						b.Fatalf("invalid comparison: %v", err)
					}
					for _, result := range result.Comparison {
						if result.Summary.TotalIntervals != strategy.MaxIntervals {
							b.Fatal("incomplete comparison result")
						}
					}
				}
				b.Logf("intervals=%d request_bytes=%d provider_bytes=%d response_bytes=%d HTTP=200", len(rows), len(encoded), providerBytes, len(raw))
			}
		})
	}
}
