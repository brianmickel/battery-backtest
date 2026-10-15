package data

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

func testInterval(start string, duration time.Duration, price float64) model.LMPInterval {
	local, err := time.ParseInLocation(time.RFC3339, start, time.UTC)
	if err != nil {
		panic(err)
	}
	end := local.Add(duration)
	return model.LMPInterval{
		IntervalStartUTC: local.UTC(), IntervalEndUTC: end.UTC(),
		IntervalStartLocal: local, IntervalEndLocal: end,
		Market: "REAL_TIME_5_MIN", Location: "NODE_A", LMP: price,
	}
}

func requireValidation(t *testing.T, err error, field, message string) {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validation.Field != field || !strings.Contains(validation.Message, message) {
		t.Fatalf("validation = %#v, want field %q and message containing %q", validation, field, message)
	}
}

func TestNormalizeSeriesCopiesSortsAndPreservesGapsAndDurations(t *testing.T) {
	first := testInterval("2026-01-01T00:00:00-07:00", 30*time.Minute, -10)
	second := testInterval("2026-01-01T01:00:00-07:00", 15*time.Minute, 0)
	rows := []model.LMPInterval{second, first}
	original := append([]model.LMPInterval(nil), rows...)
	got, err := NormalizeSeries(rows, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, original) {
		t.Fatal("normalization mutated caller input")
	}
	if !reflect.DeepEqual(got, []model.LMPInterval{first, second}) {
		t.Fatalf("normalized rows changed intervals: %+v", got)
	}
	if got[0].Duration() != 30*time.Minute || got[1].Duration() != 15*time.Minute || got[1].IntervalStartUTC.Sub(got[0].IntervalEndUTC) != 30*time.Minute {
		t.Fatal("normalization changed durations or filled a gap")
	}
	got[0].LMP = 100
	if !reflect.DeepEqual(rows, original) {
		t.Fatal("result aliases caller slice")
	}
}

func TestNormalizeSeriesClockAndAutumnRepeatedHour(t *testing.T) {
	first := testInterval("2026-11-01T01:00:00-06:00", time.Hour, -5)
	first.IntervalEndLocal = testInterval("2026-11-01T01:00:00-07:00", time.Hour, 0).IntervalStartLocal
	second := testInterval("2026-11-01T01:00:00-07:00", time.Hour, 10)
	rows := []model.LMPInterval{second, first}
	market, err := NormalizeSeries(rows, "market")
	if err != nil {
		t.Fatal(err)
	}
	if market[0].IntervalStartLocal.Format(time.RFC3339) != "2026-11-01T01:00:00-06:00" || market[1].IntervalStartLocal.Format(time.RFC3339) != "2026-11-01T01:00:00-07:00" {
		t.Fatalf("repeated civil hour lost: %+v", market)
	}
	for _, row := range market {
		if row.IntervalStartLocal.Format("2006-01-02") != "2026-11-01" || row.Duration() != time.Hour {
			t.Fatal("DST changed civil date or elapsed duration")
		}
	}
	utc, err := NormalizeSeries(rows, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range utc {
		if row.IntervalStartLocal != row.IntervalStartUTC || row.IntervalEndLocal != row.IntervalEndUTC || row.IntervalStartLocal.Location() != time.UTC {
			t.Fatal("UTC clock did not replace local display fields")
		}
	}
	if rows[1].IntervalStartLocal.Format(time.RFC3339) != "2026-11-01T01:00:00-06:00" {
		t.Fatal("UTC normalization changed input's market clock")
	}
}

func TestNormalizeSeriesUTCOnly(t *testing.T) {
	row := testInterval("2026-01-01T23:30:00-07:00", 30*time.Minute, 0)
	row.IntervalStartLocal, row.IntervalEndLocal = time.Time{}, time.Time{}
	got, err := NormalizeSeries([]model.LMPInterval{row}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].IntervalStartLocal.Format(time.RFC3339) != "2026-01-02T06:30:00Z" {
		t.Fatalf("UTC date = %v", got[0].IntervalStartLocal)
	}
	_, err = NormalizeSeries([]model.LMPInterval{row}, "market")
	requireValidation(t, err, "intervals[0].interval_start_local", "required")
}

func TestNormalizeSeriesRejectsInvalidRows(t *testing.T) {
	base := testInterval("2026-01-01T00:00:00-07:00", 30*time.Minute, 0)
	cases := []struct {
		name, field, message string
		change               func(*model.LMPInterval)
	}{
		{"missing start", "interval_start_utc", "required", func(r *model.LMPInterval) { r.IntervalStartUTC = time.Time{} }},
		{"missing end", "interval_end_utc", "required", func(r *model.LMPInterval) { r.IntervalEndUTC = time.Time{} }},
		{"zero duration", "interval_end_utc", "positive", func(r *model.LMPInterval) { r.IntervalEndUTC = r.IntervalStartUTC }},
		{"negative duration", "interval_end_utc", "positive", func(r *model.LMPInterval) { r.IntervalEndUTC = r.IntervalStartUTC.Add(-time.Hour) }},
		{"partial local pair", "interval_start_local", "together", func(r *model.LMPInterval) { r.IntervalEndLocal = time.Time{} }},
		{"conflicting start", "interval_start_local", "same instant", func(r *model.LMPInterval) { r.IntervalStartLocal = r.IntervalStartLocal.Add(time.Minute) }},
		{"conflicting end", "interval_end_local", "same instant", func(r *model.LMPInterval) { r.IntervalEndLocal = r.IntervalEndLocal.Add(time.Minute) }},
		{"missing location", "location", "empty", func(r *model.LMPInterval) { r.Location = " " }},
		{"missing market", "market", "empty", func(r *model.LMPInterval) { r.Market = "" }},
		{"NaN lmp", "lmp", "finite", func(r *model.LMPInterval) { r.LMP = math.NaN() }},
		{"infinite energy", "energy", "finite", func(r *model.LMPInterval) { r.Energy = math.Inf(1) }},
		{"infinite congestion", "congestion", "finite", func(r *model.LMPInterval) { r.Congestion = math.Inf(-1) }},
		{"NaN loss", "loss", "finite", func(r *model.LMPInterval) { r.Loss = math.NaN() }},
		{"infinite ghg", "ghg", "finite", func(r *model.LMPInterval) { r.GHG = math.Inf(1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := base
			tc.change(&row)
			for _, clock := range []string{"market", "UTC"} {
				got, err := NormalizeSeries([]model.LMPInterval{row}, clock)
				if got != nil {
					t.Fatal("invalid input returned partial rows")
				}
				requireValidation(t, err, "intervals[0]."+tc.field, tc.message)
			}
		})
	}
}

func TestNormalizeSeriesRejectsSeriesConflicts(t *testing.T) {
	first := testInterval("2026-01-01T00:00:00-07:00", 30*time.Minute, 1)
	second := testInterval("2026-01-01T00:30:00-07:00", 30*time.Minute, 2)
	for _, tc := range []struct {
		name, field, message string
		change               func(*model.LMPInterval)
	}{
		{"mixed location", "intervals[1].location", "same location", func(r *model.LMPInterval) { r.Location = "NODE_B" }},
		{"mixed market", "intervals[1].market", "same market", func(r *model.LMPInterval) { r.Market = "DAY_AHEAD" }},
		{"duplicate", "intervals", "duplicate", func(r *model.LMPInterval) { *r = first }},
		{"same start different end", "intervals", "duplicate", func(r *model.LMPInterval) { *r = testInterval("2026-01-01T00:00:00-07:00", time.Hour, 5) }},
		{"overlap", "intervals", "overlapping", func(r *model.LMPInterval) { *r = testInterval("2026-01-01T00:15:00-07:00", time.Hour, 5) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := second
			tc.change(&changed)
			rows := []model.LMPInterval{first, changed}
			before := append([]model.LMPInterval(nil), rows...)
			got, err := NormalizeSeries(rows, "market")
			requireValidation(t, err, tc.field, tc.message)
			if got != nil || !reflect.DeepEqual(rows, before) {
				t.Fatal("failed normalization returned or mutated rows")
			}
		})
	}
}

func TestNormalizeSeriesBoundsAndClock(t *testing.T) {
	_, err := NormalizeSeries(nil, "market")
	requireValidation(t, err, "intervals", "at least one")
	for _, clock := range []string{"utc", "Market", "America/Denver", " UTC "} {
		_, err := NormalizeSeries(nil, clock)
		requireValidation(t, err, "timezone", "market or UTC")
	}
	rows := make([]model.LMPInterval, maxSeriesIntervals+1)
	_, err = NormalizeSeries(rows, "market")
	if !errors.Is(err, ErrIntervalLimit) || !strings.Contains(err.Error(), "17280") {
		t.Fatalf("interval budget error = %v", err)
	}
	first := testInterval("2026-01-01T00:00:00-07:00", 5*time.Minute, -1)
	for index := range rows {
		row := first
		delta := time.Duration(index) * 5 * time.Minute
		row.IntervalStartUTC = row.IntervalStartUTC.Add(delta)
		row.IntervalEndUTC = row.IntervalEndUTC.Add(delta)
		row.IntervalStartLocal = row.IntervalStartLocal.Add(delta)
		row.IntervalEndLocal = row.IntervalEndLocal.Add(delta)
		rows[index] = row
	}
	if got, err := NormalizeSeries(rows[:maxSeriesIntervals], "market"); err != nil || len(got) != maxSeriesIntervals {
		t.Fatalf("boundary rejected: rows=%d, err=%v", len(got), err)
	}
}
