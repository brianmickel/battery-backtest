package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

func TestContractDecodeFieldAndVariationPaths(t *testing.T) {
	single := string(fixture(t, "single-request"))
	comparison := string(fixture(t, "compare-request"))
	var calls atomic.Int32
	router := testRouter(t, testDependencies(t, providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
		calls.Add(1)
		return nil, errors.New("must not fetch")
	})))
	for _, tc := range []struct{ name, path, body, field, variation string }{
		{"config scalar", "/api/v1/backtest", strings.Replace(single, `"initial_soc": 0`, `"initial_soc": "sentinel-secret"`, 1), "config.battery.initial_soc", ""},
		{"config null", "/api/v1/backtest", strings.Replace(single, `"initial_soc": 0`, `"initial_soc": null`, 1), "config.battery.initial_soc", ""},
		{"source scalar", "/api/v1/backtest", strings.Replace(single, `"start_date": "2024-01-01"`, `"start_date": 123`, 1), "data_source.start_date", ""},
		{"unknown outer field", "/api/v1/backtest", strings.Replace(single, `"options": {`, `"unexpected":"sentinel-secret","options": {`, 1), "unexpected", ""},
		{"wrong case", "/api/v1/backtest", strings.Replace(single, `"data_source":`, `"DATA_SOURCE":`, 1), "DATA_SOURCE", ""},
		{"duplicate overwrites null", "/api/v1/backtest", strings.Replace(single, `"include_ledger": true`, `"include_ledger": null,"include_ledger": true`, 1), "options.include_ledger", ""},
		{"duplicate overwrites wrong type", "/api/v1/backtest", strings.Replace(single, `"include_ledger": true`, `"include_ledger": "sentinel-secret","include_ledger": true`, 1), "options.include_ledger", ""},
		{"later variation scalar", "/api/v1/backtest/compare", strings.Replace(comparison, `"config": {}`, `"config": {"battery":{"initial_soc":"sentinel-secret"}}`, 1), "config.battery.initial_soc", "Oracle"},
		{"later variation null", "/api/v1/backtest/compare", strings.Replace(comparison, `"config": {}`, `"config": {"battery":{"initial_soc":null}}`, 1), "config.battery.initial_soc", "Oracle"},
		{"later variation unknown param", "/api/v1/backtest/compare", strings.Replace(comparison, `"config": {}`, `"config": {"strategy":{"params":{"mystery":1}}}`, 1), "config.strategy.params.mystery", "Oracle"},
		{"variation name follows config", "/api/v1/backtest/compare", strings.Replace(comparison, "\"name\": \"Oracle\",\n      \"config\": {}", `"config":{"battery":{"initial_soc":"sentinel-secret"}},"name":"Oracle"`, 1), "config.battery.initial_soc", "Oracle"},
		{"base scalar", "/api/v1/backtest/compare", strings.Replace(comparison, `"initial_soc": 0`, `"initial_soc": "sentinel-secret"`, 1), "base_config.battery.initial_soc", ""},
		{"strategy-specific config error", "/api/v1/backtest", strings.Replace(single, `"soc_steps": 200`, `"charge_power_mw": 1`, 1), "config.strategy.params.charge_power_mw", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := request(router, "POST", tc.path, []byte(tc.body))
			if response.Code != 400 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			failure := errorBody(t, response)
			if failure.Code != "INVALID_REQUEST" || failure.Field != tc.field || failure.Variation != tc.variation || strings.Contains(response.Body.String(), "sentinel-secret") {
				t.Fatalf("error=%+v; want field=%q variation=%q", failure, tc.field, tc.variation)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests fetched %d times", calls.Load())
	}
}

func TestContractMalformedQueryRejectedBeforeFetch(t *testing.T) {
	rows := fixtureRows(t)
	var calls atomic.Int32
	router := testRouter(t, testDependencies(t, providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) { calls.Add(1); return rows, nil })))
	for _, tc := range []struct {
		method, path string
		body         []byte
	}{
		{"POST", "/api/v1/backtest?", fixture(t, "single-request")},
		{"POST", "/api/v1/backtest/compare?", fixture(t, "compare-request")},
		{"GET", "/api/v1/rank?dataset_id=test&start_date=2024-01-01&end_date=2024-01-02&location_ids=TEST_NODE&", nil},
	} {
		for _, query := range []string{"api_key=%zz", "api_key=sentinel-secret;ignored=1", "ignored=%zz", "api%zz_key=sentinel-secret", "ignored=%zz&api_key=sentinel-secret"} {
			t.Run(tc.path+query, func(t *testing.T) {
				response := request(router, tc.method, tc.path+query, tc.body)
				if response.Code != 400 || errorBody(t, response).Code != "INVALID_REQUEST" || strings.Contains(response.Body.String(), "sentinel-secret") {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("malformed query fetched %d times", calls.Load())
	}
}

type deadlineObserver struct {
	context.Context
	observed atomic.Bool
}

func (c *deadlineObserver) Deadline() (time.Time, bool) {
	c.observed.Store(true)
	return c.Context.Deadline()
}

type inspectingReader struct {
	io.Reader
	inspect   func()
	inspected bool
}

func (r *inspectingReader) Read(p []byte) (int, error) {
	if !r.inspected {
		r.inspected = true
		r.inspect()
	}
	return r.Reader.Read(p)
}

func TestApplicationDeadlineStartsBeforeBodyRead(t *testing.T) {
	rows := fixtureRows(t)
	observer := &deadlineObserver{Context: context.Background()}
	startedBeforeRead := false
	var providerDeadline time.Time
	router := testRouter(t, testDependencies(t, providerFunc(func(ctx context.Context, _ string, _ data.Query) ([]model.LMPInterval, error) {
		var ok bool
		providerDeadline, ok = ctx.Deadline()
		if !ok {
			t.Error("provider has no deadline")
		}
		return rows, nil
	})))
	reader := &inspectingReader{Reader: bytes.NewReader(fixture(t, "single-request")), inspect: func() { startedBeforeRead = observer.observed.Load() }}
	incoming := httptest.NewRequest("POST", "/api/v1/backtest", reader).WithContext(observer)
	incoming.Header.Set("X-GridStatus-API-Key", "sentinel-secret")
	response := httptest.NewRecorder()
	before := time.Now()
	router.ServeHTTP(response, incoming)
	after := time.Now()
	if !startedBeforeRead {
		t.Error("application deadline was not created before reading the body")
	}
	if response.Code != 200 || providerDeadline.Before(before.Add(60*time.Second)) || providerDeadline.After(after.Add(60*time.Second)) {
		t.Fatalf("status=%d deadline=%s body=%s", response.Code, providerDeadline, response.Body.String())
	}
}

func TestExpiredRequestDoesNotFetchAndReleasesAdmission(t *testing.T) {
	rows := fixtureRows(t)
	calls := 0
	router := testRouter(t, testDependencies(t, providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) { calls++; return rows, nil })))
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	incoming := httptest.NewRequest("POST", "/api/v1/backtest", bytes.NewReader(fixture(t, "single-request"))).WithContext(ctx)
	incoming.Header.Set("X-GridStatus-API-Key", "sentinel-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, incoming)
	if response.Code != 504 || errorBody(t, response).Code != "DEADLINE_EXCEEDED" || calls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	if response = request(router, "POST", "/api/v1/backtest", fixture(t, "single-request")); response.Code != 200 || calls != 1 {
		t.Fatalf("next status=%d calls=%d", response.Code, calls)
	}
}
