package backtest

import (
	"math"
	"reflect"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

func TestSummaryTotalsFromPhysicalLedger(t *testing.T) {
	for _, tc := range []struct {
		name        string
		efficiency  float64
		degradation float64
	}{
		{name: "unit efficiency", efficiency: 1},
		{name: "loss and degradation", efficiency: .8, degradation: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := model.BatteryParams{
				EnergyCapacityMWh: 8, PowerCapacityMW: 2,
				ChargeEfficiency: tc.efficiency, DischargeEfficiency: tc.efficiency,
				MinSOC: 0, MaxSOC: 1, DegradationCostPerMWh: tc.degradation,
			}
			state := model.BatteryState{SOC: .5}
			result := Result{TotalPNL: 999, FinalSOC: .123}
			for i, dispatch := range []float64{-2, 1} {
				next, physical, err := model.SimulateInterval(params, state, 10+float64(i)*30, model.Dispatch{PowerMW: dispatch}, 1)
				if err != nil {
					t.Fatal(err)
				}
				row := summaryTestRow(t, "2026-01-01T10:00:00-07:00", "2026-01-01T11:00:00-07:00", 10+float64(i)*30, physical.EnergyFromGridMWh, physical.EnergyToGridMWh)
				row.IntervalStartLocal = row.IntervalStartLocal.Add(time.Duration(i) * time.Hour)
				row.IntervalEndLocal = row.IntervalEndLocal.Add(time.Duration(i) * time.Hour)
				row.IntervalStartUTC = row.IntervalStartLocal.UTC()
				row.IntervalEndUTC = row.IntervalEndLocal.UTC()
				row.SOCStart, row.SOCEnd, row.PNL = physical.SOCStart, physical.SOCEnd, physical.PNL
				result.Ledger = append(result.Ledger, row)
				state = next
			}
			original := result
			original.Ledger = append([]LedgerRow(nil), result.Ledger...)
			summary, err := Summarize(result, params, "market")
			if err != nil {
				t.Fatal(err)
			}
			assertSummaryNear(t, summary.InitialSOC, .5)
			assertSummaryNear(t, summary.InitialEnergyMWh, 4)
			assertSummaryNear(t, summary.FinalSOC, state.SOC)
			assertSummaryNear(t, summary.FinalEnergyMWh, state.SOC*8)
			assertSummaryNear(t, summary.TotalPNL, 20-3*tc.degradation)
			assertSummaryNear(t, summary.EnergyChargedMWh, 2)
			assertSummaryNear(t, summary.EnergyDischargedMWh, 1)
			if summary.TotalIntervals != 2 || summary.Timezone != "market" {
				t.Fatalf("unexpected summary metadata: %+v", summary)
			}
			if !reflect.DeepEqual(result, original) {
				t.Fatal("Summarize mutated its ledger")
			}
		})
	}
}

func TestSummaryContiguousWindowsAndWeightedPrices(t *testing.T) {
	rows := []LedgerRow{
		summaryTestRow(t, "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", 10, 1, 0),
		summaryTestRow(t, "2026-01-01T01:00:00Z", "2026-01-01T01:30:00Z", 30, 3, 0),
		summaryTestRow(t, "2026-01-01T01:30:00Z", "2026-01-01T02:00:00Z", 999, 0, 0),
		summaryTestRow(t, "2026-01-01T02:00:00Z", "2026-01-01T03:00:00Z", -20, 2, 0),
		summaryTestRow(t, "2026-01-01T03:00:00Z", "2026-01-01T03:30:00Z", -10, 0, 2),
		summaryTestRow(t, "2026-01-01T03:30:00Z", "2026-01-01T04:00:00Z", 30, 0, 6),
		summaryTestRow(t, "2026-01-01T04:00:00Z", "2026-01-01T05:00:00Z", 5, 1, 0),
		summaryTestRow(t, "2026-01-01T06:00:00Z", "2026-01-01T07:00:00Z", 7, 1, 0),
	}
	// An unfulfilled request has no realized energy and must still end the run.
	rows[2].RequestedPowerMW = -10
	rows[2].Action = model.ActionCharging
	got := mustSummarize(t, rows, "UTC")
	if len(got.ChargeWindows) != 4 || len(got.DischargeWindows) != 1 {
		t.Fatalf("windows merged across idle, opposite action, or gap: %+v", got)
	}
	for i, want := range []struct {
		start, end    string
		energy, price float64
	}{
		{"2026-01-01T00:00:00Z", "2026-01-01T01:30:00Z", 4, 25},
		{"2026-01-01T02:00:00Z", "2026-01-01T03:00:00Z", 2, -20},
		{"2026-01-01T04:00:00Z", "2026-01-01T05:00:00Z", 1, 5},
		{"2026-01-01T06:00:00Z", "2026-01-01T07:00:00Z", 1, 7},
	} {
		window := got.ChargeWindows[i]
		assertSummaryWindow(t, window.TimeWindow, want.start, want.end)
		assertSummaryNear(t, window.EnergyMWh, want.energy)
		assertSummaryNear(t, window.AverageCostPerMWh, want.price)
	}
	assertSummaryNear(t, got.DischargeWindows[0].EnergyMWh, 8)
	assertSummaryNear(t, got.DischargeWindows[0].AveragePricePerMWh, 20)
	assertSummaryNear(t, got.CoveredDurationHours, 6)
	assertSummaryNear(t, got.GapDurationHours, 1)
	if got.GapCount != 1 {
		t.Fatalf("gap count = %d, want 1", got.GapCount)
	}
}

func TestSummaryCoverageDistinguishesEqualOuterWindowAndCount(t *testing.T) {
	contiguous := []LedgerRow{
		summaryTestRow(t, "2026-01-01T00:00:00Z", "2026-01-01T01:15:00Z", 0, 0, 0),
		summaryTestRow(t, "2026-01-01T01:15:00Z", "2026-01-01T03:00:00Z", 0, 0, 0),
	}
	gapped := []LedgerRow{
		summaryTestRow(t, "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", 0, 0, 0),
		summaryTestRow(t, "2026-01-01T02:00:00Z", "2026-01-01T03:00:00Z", 0, 0, 0),
	}
	full, partial := mustSummarize(t, contiguous, "UTC"), mustSummarize(t, gapped, "UTC")
	if full.BacktestWindow != partial.BacktestWindow || full.TotalIntervals != partial.TotalIntervals {
		t.Fatal("fixture must have the same window and row count")
	}
	assertSummaryNear(t, full.CoveredDurationHours, 3)
	assertSummaryNear(t, full.GapDurationHours, 0)
	assertSummaryNear(t, partial.CoveredDurationHours, 2)
	assertSummaryNear(t, partial.GapDurationHours, 1)
	if full.GapCount != 0 || partial.GapCount != 1 {
		t.Fatalf("gap counts = %d, %d; want 0, 1", full.GapCount, partial.GapCount)
	}
	if len(full.ChargeWindows)+len(full.DischargeWindows)+len(partial.ChargeWindows)+len(partial.DischargeWindows) != 0 {
		t.Fatal("zero-energy rows created operating windows")
	}
}

func TestSummaryMidnightSplitUsesSelectedClock(t *testing.T) {
	rows := []LedgerRow{
		summaryTestRow(t, "2026-01-01T23:30:00-07:00", "2026-01-02T01:30:00-07:00", -5, 4, 0),
		summaryTestRow(t, "2026-01-02T01:30:00-07:00", "2026-01-02T02:00:00-07:00", 10, 1, 0),
	}
	market, utc := mustSummarize(t, rows, "market"), mustSummarize(t, rows, "UTC")
	if len(market.ChargeWindows) != 2 || len(utc.ChargeWindows) != 1 {
		t.Fatalf("market/UTC window counts = %d/%d, want 2/1", len(market.ChargeWindows), len(utc.ChargeWindows))
	}
	assertSummaryWindow(t, market.ChargeWindows[0].TimeWindow, "2026-01-01T23:30:00-07:00", "2026-01-02T00:00:00-07:00")
	assertSummaryWindow(t, market.ChargeWindows[1].TimeWindow, "2026-01-02T00:00:00-07:00", "2026-01-02T02:00:00-07:00")
	assertSummaryNear(t, market.ChargeWindows[0].EnergyMWh, 1)
	assertSummaryNear(t, market.ChargeWindows[1].EnergyMWh, 4)
	assertSummaryNear(t, market.ChargeWindows[0].AverageCostPerMWh, -5)
	assertSummaryNear(t, market.ChargeWindows[1].AverageCostPerMWh, -1.25)
	assertSummaryWindow(t, utc.BacktestWindow, "2026-01-02T06:30:00Z", "2026-01-02T09:00:00Z")
	assertSummaryWindow(t, utc.ChargeWindows[0].TimeWindow, "2026-01-02T06:30:00Z", "2026-01-02T09:00:00Z")
	assertSummaryNear(t, market.EnergyChargedMWh, 5)
	assertSummaryNear(t, utc.EnergyChargedMWh, 5)
	assertSummaryNear(t, market.TotalPNL, 10)
	assertSummaryNear(t, utc.TotalPNL, 10)
}

func TestSummaryMultipleMidnightsAndExactBoundary(t *testing.T) {
	rows := []LedgerRow{summaryTestRow(t, "2026-01-01T23:00:00Z", "2026-01-03T01:00:00Z", 12, 0, 52)}
	got := mustSummarize(t, rows, "UTC")
	if len(got.DischargeWindows) != 3 {
		t.Fatalf("discharge windows = %d, want 3", len(got.DischargeWindows))
	}
	for i, want := range []float64{2, 48, 2} {
		assertSummaryNear(t, got.DischargeWindows[i].EnergyMWh, want)
		assertSummaryNear(t, got.DischargeWindows[i].AveragePricePerMWh, 12)
	}
	assertSummaryNear(t, got.EnergyDischargedMWh, 52)
	assertSummaryNear(t, got.TotalPNL, 624)
	assertSummaryNear(t, got.CoveredDurationHours, 26)

	rows = []LedgerRow{
		summaryTestRow(t, "2026-01-01T23:00:00Z", "2026-01-02T00:00:00Z", 10, 1, 0),
		summaryTestRow(t, "2026-01-02T00:00:00Z", "2026-01-02T01:00:00Z", 20, 1, 0),
	}
	got = mustSummarize(t, rows, "UTC")
	if len(got.ChargeWindows) != 2 {
		t.Fatalf("exact midnight should split contiguous rows: %+v", got.ChargeWindows)
	}
}

func TestSummaryDSTUsesInstantsAndCivilDates(t *testing.T) {
	for _, tc := range []struct {
		name, start, end string
		hours            float64
	}{
		{"spring", "2026-03-08T01:30:00-07:00", "2026-03-08T03:30:00-06:00", 1},
		{"fall", "2026-11-01T01:30:00-06:00", "2026-11-01T01:30:00-07:00", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := summaryTestRow(t, tc.start, tc.end, 10, 2, 0)
			next := row
			next.IntervalStartLocal = row.IntervalEndLocal
			next.IntervalEndLocal = row.IntervalEndLocal.Add(30 * time.Minute)
			next.IntervalStartUTC, next.IntervalEndUTC = next.IntervalStartLocal.UTC(), next.IntervalEndLocal.UTC()
			got := mustSummarize(t, []LedgerRow{row, next}, "market")
			if len(got.ChargeWindows) != 1 {
				t.Fatalf("offset change split one civil date: %+v", got.ChargeWindows)
			}
			assertSummaryWindow(t, got.ChargeWindows[0].TimeWindow, tc.start, next.IntervalEndLocal.Format(time.RFC3339))
			assertSummaryNear(t, got.CoveredDurationHours, tc.hours+.5)
			assertSummaryNear(t, got.GapDurationHours, 0)
		})
	}

	rows := []LedgerRow{
		summaryTestRow(t, "2026-10-31T23:30:00-06:00", "2026-11-01T01:30:00-07:00", 10, 6, 0),
	}
	got := mustSummarize(t, rows, "market")
	if len(got.ChargeWindows) != 2 {
		t.Fatalf("DST midnight split: %+v", got.ChargeWindows)
	}
	assertSummaryWindow(t, got.ChargeWindows[0].TimeWindow, "2026-10-31T23:30:00-06:00", "2026-11-01T00:00:00-06:00")
	assertSummaryWindow(t, got.ChargeWindows[1].TimeWindow, "2026-11-01T00:00:00-06:00", "2026-11-01T01:30:00-07:00")
	assertSummaryNear(t, got.ChargeWindows[0].EnergyMWh, 1)
	assertSummaryNear(t, got.ChargeWindows[1].EnergyMWh, 5)
	assertSummaryNear(t, got.CoveredDurationHours, 3)
}

func TestSummaryRepeatedHourWindowsRemainChronological(t *testing.T) {
	rows := []LedgerRow{
		summaryTestRow(t, "2026-11-01T01:30:00-06:00", "2026-11-01T01:45:00-06:00", 10, 1, 0),
		summaryTestRow(t, "2026-11-01T01:00:00-07:00", "2026-11-01T01:15:00-07:00", 20, 1, 0),
	}
	got := mustSummarize(t, rows, "market")
	if len(got.ChargeWindows) != 2 || !got.ChargeWindows[0].Start.Before(got.ChargeWindows[1].Start) {
		t.Fatalf("repeated hour windows are not ordered by instant: %+v", got.ChargeWindows)
	}
	assertSummaryNear(t, got.CoveredDurationHours, .5)
	assertSummaryNear(t, got.GapDurationHours, .25)
}

func TestSummaryRejectsInvalidInput(t *testing.T) {
	base := summaryTestRow(t, "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", 10, 1, 0)
	for _, tc := range []struct {
		name   string
		mutate func(*LedgerRow)
	}{
		{"zero duration", func(r *LedgerRow) { r.IntervalEndUTC = r.IntervalStartUTC }},
		{"missing UTC", func(r *LedgerRow) { r.IntervalStartUTC = time.Time{} }},
		{"mismatched local", func(r *LedgerRow) { r.IntervalEndLocal = r.IntervalEndLocal.Add(time.Hour) }},
		{"nonfinite price", func(r *LedgerRow) { r.LMP = math.NaN() }},
		{"nonfinite cashflow", func(r *LedgerRow) { r.PNL = math.Inf(1) }},
		{"nonfinite SOC", func(r *LedgerRow) { r.SOCStart = math.NaN() }},
		{"negative energy", func(r *LedgerRow) { r.EnergyFromGridMWh = -1 }},
		{"nonfinite energy", func(r *LedgerRow) { r.EnergyFromGridMWh = math.Inf(1) }},
		{"two directions", func(r *LedgerRow) { r.EnergyToGridMWh = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := base
			tc.mutate(&row)
			if _, err := Summarize(Result{Ledger: []LedgerRow{row}}, model.BatteryParams{EnergyCapacityMWh: 10}, "market"); err == nil {
				t.Fatal("invalid ledger accepted")
			}
		})
	}
	for _, tc := range []struct {
		name     string
		rows     []LedgerRow
		capacity float64
		clock    string
	}{
		{"empty", nil, 10, "market"},
		{"unknown clock", []LedgerRow{base}, 10, "America/Denver"},
		{"missing clock", []LedgerRow{base}, 10, ""},
		{"zero capacity", []LedgerRow{base}, 0, "market"},
		{"nonfinite capacity", []LedgerRow{base}, math.Inf(1), "market"},
		{"overlap", []LedgerRow{base, base}, 10, "market"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Summarize(Result{Ledger: tc.rows}, model.BatteryParams{EnergyCapacityMWh: tc.capacity}, tc.clock); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func mustSummarize(t *testing.T, rows []LedgerRow, clock string) Summary {
	t.Helper()
	summary, err := Summarize(Result{Ledger: rows}, model.BatteryParams{EnergyCapacityMWh: 10}, clock)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func summaryTestRow(t *testing.T, start, end string, price, charge, discharge float64) LedgerRow {
	t.Helper()
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatal(err)
	}
	endTime, err := time.Parse(time.RFC3339, end)
	if err != nil {
		t.Fatal(err)
	}
	return LedgerRow{
		IntervalStartLocal: startTime, IntervalEndLocal: endTime,
		IntervalStartUTC: startTime.UTC(), IntervalEndUTC: endTime.UTC(),
		LMP: price, EnergyFromGridMWh: charge, EnergyToGridMWh: discharge,
		Action:   model.ActionFromPowerMW(discharge - charge),
		SOCStart: .5, SOCEnd: .5, PNL: price * (discharge - charge),
	}
}

func assertSummaryNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > 1e-10*math.Max(1, math.Abs(want)) {
		t.Fatalf("got %.12g, want %.12g", got, want)
	}
}

func assertSummaryWindow(t *testing.T, got TimeWindow, start, end string) {
	t.Helper()
	if got.Start.Format(time.RFC3339) != start || got.End.Format(time.RFC3339) != end {
		t.Fatalf("window = %s to %s, want %s to %s", got.Start.Format(time.RFC3339), got.End.Format(time.RFC3339), start, end)
	}
}
