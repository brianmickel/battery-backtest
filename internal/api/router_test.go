package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
	"github.com/gin-gonic/gin"
)

type providerFunc func(context.Context, string, data.Query) ([]model.LMPInterval, error)

func (f providerFunc) Fetch(ctx context.Context, key string, query data.Query) ([]model.LMPInterval, error) {
	return f(ctx, key, query)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func fixtureRows(t *testing.T) []model.LMPInterval {
	t.Helper()
	rows, err := data.DecodeGridStatusJSON(bytes.NewReader(fixture(t, "two-price")))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
func testDependencies(t *testing.T, provider Provider) Dependencies {
	t.Helper()
	presets, err := config.LoadPresetCatalog("../../examples/batteries")
	if err != nil {
		t.Fatal(err)
	}
	return Dependencies{Provider: provider, Presets: presets}
}
func testRouter(t *testing.T, deps Dependencies) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router, err := NewRouter(deps)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func request(router http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("X-GridStatus-API-Key", "sentinel-secret")
	r.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, r)
	return response
}
func errorBody(t *testing.T, response *httptest.ResponseRecorder) models.ErrorDetail {
	t.Helper()
	var body models.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err, response.Body.String())
	}
	return body.Error
}

func TestContractSingleComparisonAndMetadata(t *testing.T) {
	rows := fixtureRows(t)
	calls := 0
	provider := providerFunc(func(_ context.Context, key string, query data.Query) ([]model.LMPInterval, error) {
		calls++
		if key != "sentinel-secret" || query.Clock != "UTC" || query.LocationID != "TEST_NODE" {
			t.Errorf("fetch contract: key matched=%v query=%+v", key == "sentinel-secret", query)
		}
		return rows, nil
	})
	router := testRouter(t, testDependencies(t, provider))
	response := request(router, "POST", "/api/v1/backtest", fixture(t, "single-request"))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var single models.BacktestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &single); err != nil {
		t.Fatal(err)
	}
	if single.Summary.TotalPNL != 90 || single.Summary.InitialSOC != 0 || single.Summary.FinalEnergyMWh != 0 || single.Summary.CoveredDurationHours != 2 || single.Summary.GapCount != 0 || len(single.Ledger) != 2 {
		t.Fatalf("single=%+v", single)
	}
	for _, key := range []string{"covered_duration_hours", "gap_duration_hours", "gap_count", "initial_energy_mwh", "final_energy_mwh"} {
		if !bytes.Contains(response.Body.Bytes(), []byte(`"`+key+`"`)) {
			t.Fatal("missing wire field", key)
		}
	}
	response = request(router, "POST", "/api/v1/backtest/compare", fixture(t, "compare-request"))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var comparison models.CompareBacktestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if len(comparison.Comparison) != 2 || comparison.Comparison[0].Name != "Idle at explicit zero" || comparison.Comparison[0].Summary.TotalPNL != 0 || comparison.Comparison[1].Summary.TotalPNL != 90 || calls != 2 {
		t.Fatalf("comparison=%+v fetches=%d", comparison, calls)
	}
	for _, path := range []string{"/health", "/api/v1/batteries", "/api/v1/strategies", "/api/v1/datasets", "/api/v1/locations?dataset_id=test"} {
		response = request(router, "GET", path, nil)
		if response.Code != 200 {
			t.Fatal(path, response.Code, response.Body.String())
		}
	}
	batteries := request(router, "GET", "/api/v1/batteries", nil).Body.String()
	if strings.Contains(batteries, `"file"`) || strings.Contains(batteries, "examples/") {
		t.Fatal("filesystem path exposed", batteries)
	}
	metadata := request(router, "GET", "/api/v1/strategies", nil).Body.String()
	if !strings.Contains(metadata, `"default_source":"battery_power_capacity_mw"`) {
		t.Fatal(metadata)
	}
}

func TestContractValidationBeforeProvider(t *testing.T) {
	var calls atomic.Int32
	router := testRouter(t, testDependencies(t, providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
		calls.Add(1)
		return nil, errors.New("must not fetch")
	})))
	valid := string(fixture(t, "single-request"))
	for _, tc := range []struct {
		name, body, path string
		status           int
	}{
		{"traversal", strings.Replace(valid, `"battery": {`, `"battery_file":"../secret", "battery": {`, 1), "", 400},
		{"unknown preset", strings.Replace(valid, `"battery": {`, `"battery_file":"unknown", "battery": {`, 1), "", 400},
		{"same date", strings.ReplaceAll(valid, "2024-01-02", "2024-01-01"), "", 400},
		{"impossible date", strings.ReplaceAll(valid, "2024-01-01", "2024-02-30"), "", 400},
		{"bad clock", strings.ReplaceAll(valid, `"UTC"`, `"local"`), "", 400},
		{"null", strings.ReplaceAll(valid, `"initial_soc": 0`, `"initial_soc": null`), "", 400},
		{"unknown field", strings.Replace(valid, `"data_source": {`, `"secret_extra":true,"data_source": {`, 1), "", 400},
		{"body key", strings.Replace(valid, `"data_source": {`, `"api_key":"sentinel-secret","data_source": {`, 1), "", 400},
		{"query key", valid, "/api/v1/backtest?api_key=sentinel-secret", 400},
		{"body size", strings.Repeat(" ", maxRequestBytes) + valid, "", 413},
		{"limit budget", strings.Replace(valid, `"include_ledger": true`, `"include_ledger":true,"limit_intervals":17281`, 1), "", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if path == "" {
				path = "/api/v1/backtest"
			}
			response := request(router, "POST", path, []byte(tc.body))
			if response.Code != tc.status {
				t.Fatal(response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "sentinel-secret") {
				t.Fatal("credential echoed")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests fetched %d times", calls.Load())
	}
	presetBody := []byte(`{"data_source":{"type":"gridstatus","dataset_id":"test","location_id":"TEST_NODE","start_date":"2024-01-01","end_date":"2024-01-02"},"config":{"battery_file":"1_moss_landing","battery":{"initial_soc":0.95},"strategy":{"name":"oracle"}}}`)
	response := request(router, "POST", "/api/v1/backtest", presetBody)
	if response.Code != 400 || errorBody(t, response).Field != "config.battery.initial_soc" || calls.Load() != 0 {
		t.Fatal(response.Code, response.Body.String(), calls.Load())
	}
	badCompare := bytes.Replace(fixture(t, "compare-request"), []byte(`"name": "Oracle",`), []byte(`"name":"Oracle",`), 1)
	badCompare = bytes.Replace(badCompare, []byte(`"config": {}`), []byte(`"config":{"battery":{"initial_soc":2}}`), 1)
	response = request(router, "POST", "/api/v1/backtest/compare", badCompare)
	if response.Code != 400 || errorBody(t, response).Variation != "Oracle" || calls.Load() != 0 {
		t.Fatal(response.Code, response.Body.String(), calls.Load())
	}
}

func TestContractErrorMappingAndNoSecrets(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	for _, tc := range []struct {
		name        string
		err         error
		status      int
		code, retry string
	}{
		{"validation", &data.ValidationError{Field: "intervals[0].lmp", Message: "is missing"}, 400, "INVALID_REQUEST", ""},
		{"budget", &strategy.BudgetError{Dimension: "transitions"}, 422, "WORK_BUDGET_EXCEEDED", ""},
		{"auth", &data.GridStatusError{Code: "UPSTREAM_AUTH", StatusCode: 403}, 401, "UPSTREAM_AUTH", ""},
		{"rate", &data.GridStatusError{Code: "UPSTREAM_RATE_LIMIT", RetryAfter: "7"}, 429, "UPSTREAM_RATE_LIMIT", "7"},
		{"upstream", &data.GridStatusError{Code: "UPSTREAM_UNAVAILABLE"}, 502, "UPSTREAM_UNAVAILABLE", ""},
		{"deadline", fmt.Errorf("wrapped: %w", context.DeadlineExceeded), 504, "DEADLINE_EXCEEDED", ""},
		{"internal", errors.New("internal sentinel-secret"), 500, "INTERNAL_ERROR", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := testRouter(t, testDependencies(t, providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) { return nil, tc.err })))
			response := request(router, "POST", "/api/v1/backtest", fixture(t, "single-request"))
			if response.Code != tc.status || errorBody(t, response).Code != tc.code || response.Header().Get("Retry-After") != tc.retry {
				t.Fatal(response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "sentinel-secret") {
				t.Fatal("secret response")
			}
		})
	}
	if strings.Contains(logs.String(), "sentinel-secret") {
		t.Fatal("secret logs")
	}
	if count := strings.Count(logs.String(), "method=POST"); count != 7 {
		t.Fatalf("access log count=%d", count)
	}
}

func TestAdmissionCancellationAndRecovery(t *testing.T) {
	rows := fixtureRows(t)
	started := make(chan struct{})
	var calls atomic.Int32
	provider := providerFunc(func(ctx context.Context, _ string, _ data.Query) ([]model.LMPInterval, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return rows, nil
	})
	deps := testDependencies(t, provider)
	router := testRouter(t, deps)
	otherRouter := testRouter(t, deps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	incoming := httptest.NewRequest("POST", "/api/v1/backtest", bytes.NewReader(fixture(t, "single-request"))).WithContext(ctx)
	incoming.Header.Set("X-GridStatus-API-Key", "sentinel-secret")
	done := make(chan struct{})
	go func() { defer close(done); router.ServeHTTP(httptest.NewRecorder(), incoming) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not started")
	}
	for _, tc := range []struct {
		method, path string
		body         []byte
	}{{"POST", "/api/v1/backtest", fixture(t, "single-request")}, {"POST", "/api/v1/backtest/compare", fixture(t, "compare-request")}, {"GET", "/api/v1/rank?dataset_id=test&start_date=2024-01-01&end_date=2024-01-02&location_ids=TEST_NODE", nil}} {
		response := request(otherRouter, tc.method, tc.path, tc.body)
		if response.Code != 503 || errorBody(t, response).Code != "SERVER_BUSY" || response.Header().Get("Retry-After") != "1" {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	if response := request(otherRouter, "GET", "/health", nil); response.Code != 200 {
		t.Fatal(response.Code)
	}
	if calls.Load() != 1 {
		t.Fatal("busy operation fetched", calls.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled operation did not finish")
	}
	if response := request(router, "POST", "/api/v1/backtest", fixture(t, "single-request")); response.Code != 200 {
		t.Fatal("permit not released", response.Code, response.Body.String())
	}
	for _, panicFirst := range []bool{false, true} {
		first := true
		p := providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
			if first {
				first = false
				if panicFirst {
					panic("sentinel-secret")
				}
				return nil, errors.New("sentinel-secret")
			}
			return rows, nil
		})
		router := testRouter(t, testDependencies(t, p))
		if response := request(router, "POST", "/api/v1/backtest", fixture(t, "single-request")); response.Code != 500 || strings.Contains(response.Body.String(), "sentinel-secret") {
			t.Fatal(response.Code, response.Body.String())
		}
		if response := request(router, "POST", "/api/v1/backtest", fixture(t, "single-request")); response.Code != 200 {
			t.Fatal("failure leaked permit", response.Code, response.Body.String())
		}
	}
}

func TestContractRankAtomicAndDeterministic(t *testing.T) {
	rows := fixtureRows(t)
	var seen []string
	provider := providerFunc(func(_ context.Context, _ string, q data.Query) ([]model.LMPInterval, error) {
		seen = append(seen, q.LocationID)
		if q.LocationID == "FAIL" {
			return nil, &data.GridStatusError{Code: "UPSTREAM_AUTH"}
		}
		copy := append([]model.LMPInterval(nil), rows...)
		for i := range copy {
			copy[i].Location = q.LocationID
		}
		return copy, nil
	})
	router := testRouter(t, testDependencies(t, provider))
	path := "/api/v1/rank?dataset_id=test&start_date=2024-01-01&end_date=2024-01-02&location_ids="
	response := request(router, "GET", path+"Z,A&limit=1", nil)
	if response.Code != 200 || len(seen) != 2 {
		t.Fatal(response.Code, response.Body.String(), seen)
	}
	var ranked models.RankResponse
	if err := json.Unmarshal(response.Body.Bytes(), &ranked); err != nil {
		t.Fatal(err)
	}
	if len(ranked.Rankings) != 1 || ranked.Rankings[0].Location != "A" {
		t.Fatal(ranked)
	}
	response = request(router, "GET", path+"A,FAIL", nil)
	if response.Code != 401 || errorBody(t, response).LocationID != "FAIL" || strings.Contains(response.Body.String(), "rankings") {
		t.Fatal(response.Code, response.Body.String())
	}
	count := len(seen)
	for _, query := range []string{"A,A", "A&limit=0", "A&limit=1.5", url.QueryEscape(strings.Repeat("A,", 16) + "B"), "A&api_key=sentinel-secret"} {
		response = request(router, "GET", path+query, nil)
		if response.Code != 400 {
			t.Fatal(query, response.Code, response.Body.String())
		}
	}
	if len(seen) != count {
		t.Fatal("invalid rank fetched")
	}
}

func TestContractStartupLocationsCORSAndRemovedRoutes(t *testing.T) {
	provider := providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) { return nil, nil })
	deps := testDependencies(t, provider)
	if _, err := NewRouter(Dependencies{Provider: provider}); err == nil {
		t.Fatal("missing catalog accepted")
	}
	bad := deps
	bad.AllowedOrigins = []string{"*"}
	if _, err := NewRouter(bad); err == nil {
		t.Fatal("wildcard CORS accepted")
	}
	deps.LocationsPath = filepath.Join(t.TempDir(), "missing.json")
	deps.AllowedOrigins = []string{"https://allowed.example"}
	router := testRouter(t, deps)
	if response := request(router, "GET", "/api/v1/locations?dataset_id=test", nil); response.Code != 200 || !strings.Contains(response.Body.String(), `"locations":[]`) {
		t.Fatal(response.Code, response.Body.String())
	}
	if err := os.WriteFile(deps.LocationsPath, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if response := request(router, "GET", "/api/v1/locations?dataset_id=test", nil); response.Code != 500 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, origin := range []string{"https://allowed.example", "https://denied.example"} {
		incoming := httptest.NewRequest("GET", "/health", nil)
		incoming.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, incoming)
		want := ""
		if origin == "https://allowed.example" {
			want = origin
		}
		if response.Header().Get("Access-Control-Allow-Origin") != want || response.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatal(response.Header())
		}
	}
	for _, path := range []string{"/debug/battery-dir", "/api/v1/backtest/old/ledger", "/api/v1/unknown"} {
		response := request(router, "GET", path, nil)
		if response.Code != 404 || errorBody(t, response).Code != "NOT_FOUND" {
			t.Fatal(path, response.Code, response.Body.String())
		}
	}
}
