package backtest

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

type dispatchStrategy struct {
	decide func(strategy.Context) model.Dispatch
}

func (s dispatchStrategy) Name() string                               { return "fixture" }
func (s dispatchStrategy) Decide(ctx strategy.Context) model.Dispatch { return s.decide(ctx) }

func engineRows() []model.LMPInterval {
	start := time.Date(2025, 1, 1, 23, 30, 0, 0, time.UTC)
	return []model.LMPInterval{
		{Location: "node", Market: "market", LMP: -10, IntervalStartUTC: start, IntervalEndUTC: start.Add(30 * time.Minute), IntervalStartLocal: start, IntervalEndLocal: start.Add(30 * time.Minute)},
		{Location: "node", Market: "market", LMP: 100, IntervalStartUTC: start.Add(time.Hour), IntervalEndUTC: start.Add(2 * time.Hour), IntervalStartLocal: start.Add(time.Hour), IntervalEndLocal: start.Add(2 * time.Hour)},
	}
}
func engineBattery(t *testing.T) *model.Battery {
	t.Helper()
	battery, err := model.NewBattery(model.BatteryParams{EnergyCapacityMWh: 2, PowerCapacityMW: 1, ChargeEfficiency: .8, DischargeEfficiency: .9, MinSOC: .1, MaxSOC: .9, DegradationCostPerMWh: 2}, .3)
	if err != nil {
		t.Fatal(err)
	}
	return battery
}

func TestEngineLedgerMatchesPurePhysics(t *testing.T) {
	rows := engineRows()
	battery := engineBattery(t)
	initial := battery.State
	plan := dispatchStrategy{decide: func(ctx strategy.Context) model.Dispatch {
		if ctx.Index == 0 {
			return model.Dispatch{PowerMW: -1}
		}
		return model.Dispatch{PowerMW: 1}
	}}
	result, err := New().Run(context.Background(), rows, battery, plan)
	if err != nil {
		t.Fatal(err)
	}
	state, total := initial, 0.0
	for i, row := range rows {
		next, expected, err := model.SimulateInterval(battery.Params, state, row.LMP, plan.Decide(strategy.Context{Index: i}), row.DurationHours())
		if err != nil {
			t.Fatal(err)
		}
		ledger := result.Ledger[i]
		total += expected.PNL
		if ledger.SOCStart != state.SOC || ledger.SOCEnd != next.SOC || ledger.PNL != expected.PNL || ledger.PowerMW != expected.PowerMW || ledger.EnergyFromGridMWh != expected.EnergyFromGridMWh || ledger.EnergyToGridMWh != expected.EnergyToGridMWh || ledger.CumPNL != total {
			t.Fatalf("row %d: ledger %+v pure %+v", i, ledger, expected)
		}
		if ledger.IntervalStartUTC != row.IntervalStartUTC || ledger.IntervalEndUTC != row.IntervalEndUTC || ledger.Index != i {
			t.Fatal("engine changed timestamp/index semantics")
		}
		state = next
	}
	if result.TotalPNL != total || result.FinalSOC != state.SOC || battery.State != state {
		t.Fatal("result state differs from physics")
	}
}

func TestEngineCancellationStopsBeforeNextDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	battery := engineBattery(t)
	calls := 0
	plan := dispatchStrategy{decide: func(ctx strategy.Context) model.Dispatch {
		calls++
		if ctx.Index == 1 {
			cancel()
		}
		return model.Dispatch{PowerMW: -1}
	}}
	result, err := New().Run(ctx, engineRows(), battery, plan)
	if !errors.Is(err, context.Canceled) || result != nil || calls != 2 {
		t.Fatalf("cancellation: %+v %v calls %d", result, err, calls)
	}
	if math.Abs(battery.State.SOC-.5) > 1e-9 {
		t.Fatalf("canceled second row changed state: %v", battery.State.SOC)
	}
	before := battery.State
	if _, err := New().Run(ctx, engineRows(), battery, plan); !errors.Is(err, context.Canceled) || battery.State != before || calls != 2 {
		t.Fatal("already-canceled engine dispatched")
	}
}

func TestEnginePhysicalFailureReturnsNoPartialResult(t *testing.T) {
	battery := engineBattery(t)
	initial := battery.State
	plan := dispatchStrategy{decide: func(strategy.Context) model.Dispatch { return model.Dispatch{PowerMW: math.NaN()} }}
	result, err := New().Run(context.Background(), engineRows(), battery, plan)
	if err == nil || result != nil || battery.State != initial {
		t.Fatalf("invalid transition committed: %+v %v state %+v", result, err, battery.State)
	}
	plan.decide = func(strategy.Context) model.Dispatch { return model.Dispatch{} }
	for name, run := range map[string]func() error{
		"nil battery":  func() error { _, err := New().Run(context.Background(), engineRows(), nil, plan); return err },
		"nil strategy": func() error { _, err := New().Run(context.Background(), engineRows(), battery, nil); return err },
		"empty series": func() error { _, err := New().Run(context.Background(), nil, battery, plan); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("invalid engine input accepted")
			}
		})
	}
}
