package testutil

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

func TestSharedSeriesFixtures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		covered time.Duration
		gap     time.Duration
	}{
		{"two-price", 2 * time.Hour, 0},
		{"variable-gap", 45 * time.Minute, 15 * time.Minute},
		{"midnight", 30 * time.Minute, 0},
		{"dst-fall", time.Hour, 0},
		{"half-hour", 30 * time.Minute, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contents, err := os.ReadFile("../../testdata/" + tc.name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var response model.GridStatusLMPResponse
			if err := json.Unmarshal(contents, &response); err != nil {
				t.Fatal(err)
			}
			var covered, gaps time.Duration
			for i, row := range response.Data {
				covered += row.Duration()
				if row.Duration() <= 0 || !row.IntervalStartLocal.Equal(row.IntervalStartUTC) || !row.IntervalEndLocal.Equal(row.IntervalEndUTC) {
					t.Fatalf("invalid fixture row %d: %+v", i, row)
				}
				if i > 0 {
					gaps += row.IntervalStartUTC.Sub(response.Data[i-1].IntervalEndUTC)
				}
			}
			if covered != tc.covered || gaps != tc.gap {
				t.Fatalf("coverage/gap = %v/%v, want %v/%v", covered, gaps, tc.covered, tc.gap)
			}
		})
	}
}
