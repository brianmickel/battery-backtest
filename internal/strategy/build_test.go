package strategy

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"battery-backtest/internal/config"
	"battery-backtest/internal/model"
)

func ptr[T any](value T) *T { return &value }

func TestBuildScheduleResolvedDefaultsAndExplicitZero(t *testing.T) {
	input := config.Input{Battery: config.BatteryOverride{
		EnergyCapacityMWh: ptr(2.0), PowerCapacityMW: ptr(1.0), ChargeEfficiency: ptr(1.0),
		DischargeEfficiency: ptr(1.0), MinSOC: ptr(0.0), MaxSOC: ptr(1.0)},
		Strategy: config.StrategyOverride{Name: ptr("schedule")}}
	resolved, err := config.Resolve(input, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	rows := intervalsAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), make([]float64, 24))
	battery, _ := model.NewBattery(resolved.Battery.ToModelParams(), resolved.Battery.InitialSOC)
	plan, err := Build(context.Background(), resolved.Strategy, rows, battery)
	if err != nil {
		t.Fatal(err)
	}
	for hour, row := range rows {
		want := 0.0
		if hour >= 10 && hour < 17 {
			want = -1
		}
		if hour >= 17 {
			want = 1
		}
		if got := plan.Decide(Context{Index: hour, Interval: row}).PowerMW; got != want {
			t.Fatalf("hour %d power = %v, want %v", hour, got, want)
		}
	}
	input.Strategy.Params.ChargePowerMW = ptr(0.0)
	resolved, err = config.Resolve(input, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = Build(context.Background(), resolved.Strategy, rows, battery)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Decide(Context{Interval: rows[10]}).PowerMW; got != 0 {
		t.Fatalf("explicit zero charges at %v MW", got)
	}
}

func TestBuildScheduleWrappingEmptyAndIntervalStart(t *testing.T) {
	cfg := config.ScheduleConfig{ChargeStartMinute: 22 * 60, ChargeEndMinute: 2 * 60, ChargePowerMW: 1,
		DischargeStartMinute: 17 * 60, DischargeEndMinute: 17 * 60, DischargePowerMW: 1}
	rows := intervalsAt(time.Date(2025, 1, 1, 21, 30, 0, 0, time.UTC), make([]float64, 6))
	battery, _ := model.NewBattery(idealParams(), .5)
	plan, err := Build(context.Background(), cfg, rows, battery)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float64{0, -1, -1, -1, -1, 0} {
		if got := plan.Decide(Context{Interval: rows[i]}).PowerMW; got != want {
			t.Fatalf("row %d power = %v, want %v", i, got, want)
		}
	}
	cfg.ChargePowerMW = 0
	cfg.DischargeStartMinute, cfg.DischargeEndMinute = 22*60, 23*60
	plan, err = Build(context.Background(), cfg, rows, battery)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Decide(Context{Interval: rows[1]}).PowerMW; got != 1 {
		t.Fatalf("disabled overlapping charge masked discharge: %v", got)
	}
}

func TestBuildRejectsInvalidTypedSettingsAndSeries(t *testing.T) {
	battery, _ := model.NewBattery(idealParams(), .5)
	rows := hourlyIntervals([]float64{1, 2})
	for name, cfg := range map[string]config.Strategy{
		"missing":         nil,
		"oracle_zero":     config.OracleConfig{},
		"negative_power":  config.ScheduleConfig{ChargePowerMW: -1},
		"nonfinite_power": config.ScheduleConfig{ChargePowerMW: math.Inf(1)},
		"invalid_minute":  config.ScheduleConfig{ChargeStartMinute: 1440},
		"overlap":         config.ScheduleConfig{ChargeStartMinute: 1, ChargeEndMinute: 5, DischargeStartMinute: 2, DischargeEndMinute: 4, ChargePowerMW: 1, DischargePowerMW: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(context.Background(), cfg, rows, battery); err == nil {
				t.Fatal("invalid settings accepted")
			}
		})
	}
	cfg := config.OracleConfig{SOCSteps: 20, PowerSteps: 1}
	for name, invalid := range map[string][]model.LMPInterval{
		"empty":     nil,
		"unsorted":  {rows[1], rows[0]},
		"duplicate": {rows[0], rows[0]},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(context.Background(), cfg, invalid, battery); err == nil {
				t.Fatal("invalid series accepted")
			}
		})
	}
	if _, err := Build(context.Background(), cfg, rows, nil); err == nil {
		t.Fatal("nil battery accepted")
	}
	battery.State.SOC = math.NaN()
	if _, err := Build(context.Background(), cfg, rows, battery); err == nil {
		t.Fatal("invalid battery accepted")
	}
}

func TestEstimateWorkRejectsExtremeRequestsBeforeAllocation(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name      string
		cfg       config.OracleConfig
		count     int
		dimension string
	}{
		{"steps_overflow", config.OracleConfig{SOCSteps: maxInt, PowerSteps: maxInt}, 1, "soc_steps"},
		{"powers_overflow", config.OracleConfig{SOCSteps: 2, PowerSteps: maxInt}, 1, "power_steps"},
		{"intervals_overflow", config.OracleConfig{SOCSteps: 2, PowerSteps: 1}, maxInt, "intervals"},
		{"transitions", config.OracleConfig{SOCSteps: 1000, PowerSteps: 50}, MaxIntervals, "transitions"},
		{"memory", config.OracleConfig{SOCSteps: 1000, PowerSteps: 1}, MaxIntervals, "working_bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EstimateWork(tc.cfg, tc.count)
			var budget *BudgetError
			if !errors.As(err, &budget) || budget.Dimension != tc.dimension {
				t.Fatalf("budget error = %v, want %s", err, tc.dimension)
			}
			if allocations := testing.AllocsPerRun(10, func() { _, _ = EstimateWork(tc.cfg, tc.count) }); allocations > 2 {
				t.Fatalf("budget rejection allocated %v objects", allocations)
			}
		})
	}
	if _, err := EstimateWork(config.OracleConfig{SOCSteps: 2, PowerSteps: 1}, -1); err == nil {
		t.Fatal("negative count accepted")
	}
	if _, err := EstimateWork(config.ScheduleConfig{}, MaxIntervals+1); err == nil {
		t.Fatal("schedule bypassed interval cap")
	}
	if estimate, err := EstimateWork(config.ScheduleConfig{}, 100); err != nil || estimate.Transitions != 0 {
		t.Fatalf("schedule work = %+v, %v", estimate, err)
	}
	cfg := config.OracleConfig{SOCSteps: 200, PowerSteps: 10}
	estimate, err := EstimateWork(cfg, MaxIntervals)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Transitions != uint64(MaxIntervals*202*21) || estimate.WorkingBytes > MaxWorkingBytes {
		t.Fatalf("invalid default estimate: %+v", estimate)
	}
}

func TestBuildCanceledAndDeadlineContexts(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	for _, ctx := range []context.Context{canceled, deadline} {
		if _, err := Build(ctx, config.ScheduleConfig{}, nil, nil); !errors.Is(err, ctx.Err()) {
			t.Fatalf("error = %v, want %v", err, ctx.Err())
		}
	}
}
