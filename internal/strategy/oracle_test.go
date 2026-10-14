package strategy

import (
	"math"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

func hourlyIntervals(prices []float64) []model.LMPInterval {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]model.LMPInterval, len(prices))
	for i, p := range prices {
		s := base.Add(time.Duration(i) * time.Hour)
		e := s.Add(time.Hour)
		out[i] = model.LMPInterval{
			IntervalStartLocal: s, IntervalEndLocal: e,
			IntervalStartUTC: s, IntervalEndUTC: e,
			LMP: p,
		}
	}
	return out
}

func TestOptimizeDPChargesLowDischargesHigh(t *testing.T) {
	bp := model.BatteryParams{
		EnergyCapacityMWh: 2, PowerCapacityMW: 1,
		ChargeEfficiency: 1, DischargeEfficiency: 1,
		MinSOC: 0, MaxSOC: 1,
	}
	prices := []float64{10, 10, 100, 100}
	plan, err := optimizeDP(hourlyIntervals(prices), bp, 0, 20, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{-1, -1, 1, 1}
	pnl := 0.0
	for i, d := range plan {
		if math.Abs(d.PowerMW-want[i]) > 1e-9 {
			t.Fatalf("plan[%d] = %v, want %v (full plan %+v)", i, d.PowerMW, want[i], plan)
		}
		pnl += d.PowerMW * prices[i]
	}
	if math.Abs(pnl-180) > 1e-9 {
		t.Fatalf("pnl = %v, want 180", pnl)
	}
}

func TestOptimizeDPIdlesOnFlatPrices(t *testing.T) {
	bp := model.BatteryParams{
		EnergyCapacityMWh: 2, PowerCapacityMW: 1,
		ChargeEfficiency: 0.9, DischargeEfficiency: 0.9,
		MinSOC: 0, MaxSOC: 1, DegradationCostPerMWh: 1,
	}
	plan, err := optimizeDP(hourlyIntervals([]float64{50, 50, 50}), bp, 0.5, 20, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range plan {
		if d.PowerMW < 0 {
			t.Fatalf("plan[%d] charges (%v) on flat prices: %+v", i, d.PowerMW, plan)
		}
	}
}
