package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLMPIntervalDurationUsesUTCInstantsAcrossRepeatedHour(t *testing.T) {
	start, err := time.Parse(time.RFC3339, "2026-11-01T01:00:00-06:00")
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse(time.RFC3339, "2026-11-01T01:00:00-07:00")
	if err != nil {
		t.Fatal(err)
	}
	row := LMPInterval{IntervalStartUTC: start.UTC(), IntervalEndUTC: end.UTC(), IntervalStartLocal: start, IntervalEndLocal: end}
	if row.Duration() != time.Hour || row.DurationHours() != 1 {
		t.Fatalf("repeated civil hour duration = %s", row.Duration())
	}
	row.IntervalEndLocal = start
	if row.Duration() != time.Hour {
		t.Fatal("local fields overrode UTC duration")
	}
	row.IntervalStartUTC, row.IntervalEndUTC = time.Time{}, time.Time{}
	row.IntervalEndLocal = end
	if row.Duration() != time.Hour {
		t.Fatal("local fallback duration changed")
	}
}

func TestMarketDataJSONShape(t *testing.T) {
	encoded, err := json.Marshal(GridStatusLMPResponse{StatusCode: 200, Data: []LMPInterval{{Market: "RT", Location: "NODE_A", LMP: -1}}})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		StatusCode int                          `json:"status_code"`
		Data       []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	fields := []string{"interval_start_local", "interval_start_utc", "interval_end_local", "interval_end_utc", "market", "location", "location_type", "lmp", "energy", "congestion", "loss", "ghg"}
	if response.StatusCode != 200 || len(response.Data) != 1 || len(response.Data[0]) != len(fields) {
		t.Fatalf("response shape changed: %s", encoded)
	}
	for _, field := range fields {
		if _, present := response.Data[0][field]; !present {
			t.Fatalf("missing JSON field %q", field)
		}
	}
	if string(response.Data[0]["lmp"]) != "-1" || string(response.Data[0]["energy"]) != "0" {
		t.Fatal("numeric prices changed JSON shape")
	}
}
