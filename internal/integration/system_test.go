package integration

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/api/models"
)

func fixtureServer(t *testing.T) (*Fixture, *httptest.Server, string) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFixture(root, "")
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(f.Handler)
	t.Cleanup(func() { s.Close(); f.Close() })
	return f, s, root
}

func post(t *testing.T, client *http.Client, url, key string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GridStatus-API-Key", key)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(SentinelKey)) {
		t.Fatal("credential leaked in response")
	}
	return response.StatusCode, raw
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestHTTPCLIAndDemoParity(t *testing.T) {
	_, server, root := fixtureServer(t)
	request := read(t, filepath.Join(root, "testdata/single-request.json"))
	status, body := post(t, server.Client(), server.URL+"/api/v1/backtest", SentinelKey, request)
	if status != 200 {
		t.Fatalf("HTTP %d: %s", status, body)
	}
	var response models.BacktestResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Summary.TotalPNL != 90 || response.Summary.TotalIntervals != 2 || response.Summary.InitialSOC != 0 || response.Summary.FinalSOC != 0 || response.Summary.EnergyChargedMWh != 1 || response.Summary.EnergyDischargedMWh != 1 || response.Summary.CoveredDurationHours != 2 || len(response.Ledger) != 2 {
		t.Fatalf("unexpected physical result: %+v", response)
	}
	out := t.TempDir()
	if evidence := os.Getenv("INTEGRATION_ARTIFACT_DIR"); evidence != "" {
		if err := os.MkdirAll(evidence, 0o755); err != nil {
			t.Fatal(err)
		}
		out = evidence
		if err := os.WriteFile(filepath.Join(out, "request.json"), request, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "response.json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var previous []byte
	for _, command := range []string{"cli", "demo"} {
		output := filepath.Join(out, command+".csv")
		args := []string{"run", "./cmd/" + command}
		if command == "cli" {
			args = append(args, "backtest")
		}
		args = append(args, "--data", "testdata/two-price.json", "--config", "docs/synthetic-config.yaml", "--n", "0", "--timezone", "UTC", "--out", output)
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		stdout, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", command, err, stdout)
		}
		t.Logf("%s: %s", command, stdout)
		raw := read(t, output)
		if previous != nil && !bytes.Equal(previous, raw) {
			t.Fatal("CLI and demo ledgers differ")
		}
		previous = raw
		records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 3 {
			t.Fatalf("CSV records: %d", len(records))
		}
		indices := map[string]int{}
		for i, column := range records[0] {
			indices[column] = i
		}
		for i, row := range response.Ledger {
			for column, want := range map[string]float64{"power_mw": row.PowerMW, "energy_from_grid_mwh": row.EnergyFromGridMWh, "energy_to_grid_mwh": row.EnergyToGridMWh, "soc_start": row.SOCStart, "soc_end": row.SOCEnd, "pnl": row.PNL, "cum_pnl": row.CumPNL} {
				value, err := strconv.ParseFloat(records[i+1][indices[column]], 64)
				if err != nil || value != want {
					t.Errorf("%s row %d %s = %v, want %v", command, i, column, value, want)
				}
			}
			if records[i+1][indices["action"]] != row.Action || records[i+1][indices["interval_start_utc"]] != row.IntervalStartUTC.Format(time.RFC3339) {
				t.Errorf("%s row %d action/time differs", command, i)
			}
		}
	}
}

func TestHTTPFailuresAndAtomicOperations(t *testing.T) {
	f, server, root := fixtureServer(t)
	single := read(t, filepath.Join(root, "testdata/single-request.json"))
	for _, tc := range []struct {
		name, key, location, code string
		status                    int
	}{
		{"auth", "invalid-synthetic-key", "TEST_NODE", "UPSTREAM_AUTH", 401},
		{"malformed", SentinelKey, "MALFORMED_NODE", "UPSTREAM_UNAVAILABLE", 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := post(t, server.Client(), server.URL+"/api/v1/backtest", tc.key, bytes.ReplaceAll(single, []byte("TEST_NODE"), []byte(tc.location)))
			if status != tc.status || !bytes.Contains(raw, []byte(tc.code)) || bytes.Contains(raw, []byte(`"summary"`)) {
				t.Fatalf("%d %s", status, raw)
			}
		})
	}
	comparison := read(t, filepath.Join(root, "testdata/compare-request.json"))
	status, raw := post(t, server.Client(), server.URL+"/api/v1/backtest/compare", SentinelKey, comparison)
	var compared models.CompareBacktestResponse
	if status != 200 || json.Unmarshal(raw, &compared) != nil || len(compared.Comparison) != 2 || compared.Comparison[0].Summary.TotalPNL != 0 || compared.Comparison[1].Summary.TotalPNL != 90 {
		t.Fatalf("comparison %d %s", status, raw)
	}
	before := f.Calls()
	invalid := bytes.Replace(comparison, []byte(`"config": {}`), []byte(`"config": {"battery":{"power_capacity_mw":0}}`), 1)
	status, raw = post(t, server.Client(), server.URL+"/api/v1/backtest/compare", SentinelKey, invalid)
	if status != 400 || !bytes.Contains(raw, []byte(`"variation":"Oracle"`)) || bytes.Contains(raw, []byte(`"comparison"`)) || f.Calls() != before {
		t.Fatalf("atomic validation %d %s calls %d→%d", status, raw, before, f.Calls())
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/rank?dataset_id=synthetic_lmp&start_date=2024-01-01&end_date=2024-01-02&location_ids=TEST_NODE,FAIL_NODE&limit=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-GridStatus-API-Key", SentinelKey)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 502 || !bytes.Contains(raw, []byte(`"location_id":"FAIL_NODE"`)) || bytes.Contains(raw, []byte(`"rankings"`)) || bytes.Contains(raw, []byte(SentinelKey)) {
		t.Fatalf("atomic ranking %d %s", resp.StatusCode, raw)
	}
}

func TestHTTPDisconnectCancelsProviderAndReleasesAdmission(t *testing.T) {
	f, server, root := fixtureServer(t)
	single := read(t, filepath.Join(root, "testdata/single-request.json"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/backtest", strings.NewReader(strings.ReplaceAll(string(single), "TEST_NODE", "CANCEL_NODE")))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-GridStatus-API-Key", SentinelKey)
	done := make(chan error, 1)
	go func() {
		response, err := server.Client().Do(req)
		if response != nil {
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-f.Started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	status, raw := post(t, server.Client(), server.URL+"/api/v1/backtest", SentinelKey, single)
	if status != 503 || !bytes.Contains(raw, []byte("SERVER_BUSY")) || f.Calls() != 1 {
		t.Fatalf("admission %d %s", status, raw)
	}
	cancel()
	select {
	case <-f.Canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not observe cancellation")
	}
	if err := <-done; err == nil {
		t.Fatal("disconnected client succeeded")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, raw = post(t, server.Client(), server.URL+"/api/v1/backtest", SentinelKey, single)
		if status == 200 {
			break
		}
		if status != 503 || time.Now().After(deadline) {
			t.Fatalf("admission not released: %d %s", status, raw)
		}
		time.Sleep(time.Millisecond)
	}
}
