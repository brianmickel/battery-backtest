package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

func TestSummaryAmbiguousMidnightIsRequestError(t *testing.T) {
	rows := fixtureRows(t)[:1]
	rows[0].IntervalStartLocal = time.Date(2024, 11, 2, 0, 0, 0, 0, time.FixedZone("", -7*3600))
	rows[0].IntervalEndLocal = time.Date(2024, 11, 4, 0, 0, 0, 0, time.FixedZone("", -8*3600))
	rows[0].IntervalStartUTC, rows[0].IntervalEndUTC = rows[0].IntervalStartLocal.UTC(), rows[0].IntervalEndLocal.UTC()
	rows[0].LMP = -10
	router := testRouter(t, testDependencies(t, providerFunc(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
		return rows, nil
	})))
	for _, tc := range []struct{ name, path, variation string }{
		{"single-request", "/api/v1/backtest", ""},
		{"compare-request", "/api/v1/backtest/compare", "Oracle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fixture(t, tc.name)
			market := strings.Replace(string(body), `"timezone": "UTC"`, `"timezone": "market"`, 1)
			response := request(router, "POST", tc.path, []byte(market))
			failure := errorBody(t, response)
			if response.Code != 400 || failure.Code != "INVALID_REQUEST" || failure.Field != "intervals[0].interval_end_local" || failure.Variation != tc.variation {
				t.Fatalf("status=%d error=%+v", response.Code, failure)
			}
			if !strings.Contains(failure.Message, "split the interval at midnight or select UTC") {
				t.Fatalf("missing remedy: %+v", failure)
			}
			if response = request(router, "POST", tc.path, body); response.Code != 200 {
				t.Fatalf("UTC failed: status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
