package strategy_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

func TestOracleUnrepresentableCandidatesPreserveFeasibleIdle(t *testing.T) {
	params := model.BatteryParams{EnergyCapacityMWh: 1e12, PowerCapacityMW: 1, ChargeEfficiency: 1, DischargeEfficiency: 1, MaxSOC: 1}
	cfg := config.OracleConfig{SOCSteps: 200, PowerSteps: 1}
	for _, initial := range []float64{0, .5} {
		for _, prices := range [][]float64{{0}, {0, -10, 100}} {
			rows := precisionRows(prices)
			battery, err := model.NewBattery(params, initial)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := strategy.Build(context.Background(), cfg, rows, battery)
			if err != nil {
				t.Fatalf("initial=%v prices=%v: %v", initial, prices, err)
			}
			if battery.State.SOC != initial {
				t.Fatal("planning mutated initial inventory")
			}
			result, err := backtest.New().Run(context.Background(), rows, battery, plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Ledger) != len(rows) || result.TotalPNL != 0 || result.FinalSOC != initial {
				t.Fatalf("idle result=%+v", result)
			}
			for i, row := range result.Ledger {
				storedDelta := (row.SOCEnd - row.SOCStart) * params.EnergyCapacityMWh
				transferred := row.EnergyFromGridMWh*params.ChargeEfficiency - row.EnergyToGridMWh/params.DischargeEfficiency
				if storedDelta != transferred || row.RequestedPowerMW != 0 || row.PowerMW != 0 || row.EnergyFromGridMWh != 0 || row.EnergyToGridMWh != 0 || row.ThroughputMWh != 0 || row.SOCStart != initial || row.SOCEnd != initial || row.PNL != 0 || row.CumPNL != 0 {
					t.Fatalf("initial=%v prices=%v row=%d: idle ledger changed cash or energy: %+v", initial, prices, i, row)
				}
			}
		}
	}
}

func TestOracleSkipsOnlyUnrepresentableEnergy(t *testing.T) {
	params := model.BatteryParams{EnergyCapacityMWh: 1e12, PowerCapacityMW: 1, ChargeEfficiency: 1, DischargeEfficiency: 1, MaxSOC: 1}
	state := model.BatteryState{SOC: .5}
	next, result, err := model.SimulateInterval(params, state, 0, model.Dispatch{PowerMW: 1}, 1)
	if !errors.Is(err, model.ErrUnrepresentableEnergy) || next != state || result != (model.IntervalResult{}) {
		t.Fatalf("precision error must retain its identity without a transition: next=%+v result=%+v err=%v", next, result, err)
	}
	params.EnergyCapacityMWh, params.PowerCapacityMW = 2, 2
	battery, err := model.NewBattery(params, 0)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := strategy.Build(context.Background(), config.OracleConfig{SOCSteps: 4, PowerSteps: 1}, precisionRows([]float64{math.MaxFloat64}), battery)
	if err == nil || errors.Is(err, model.ErrUnrepresentableEnergy) || !strings.Contains(err.Error(), "finite numeric range") || plan != nil || battery.State.SOC != 0 {
		t.Fatalf("non-precision candidate failure must abort atomically: plan=%v SOC=%v err=%v", plan, battery.State.SOC, err)
	}
}

func precisionRows(prices []float64) []model.LMPInterval {
	start := time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC)
	rows := make([]model.LMPInterval, len(prices))
	for i, price := range prices {
		end := start.Add(time.Hour)
		rows[i] = model.LMPInterval{IntervalStartUTC: start, IntervalEndUTC: end, IntervalStartLocal: start, IntervalEndLocal: end, Location: "probe", Market: "probe", LMP: price}
		start = end
	}
	return rows
}
