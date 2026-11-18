package experiment

import (
	"context"
	"errors"
	"testing"
	"time"

	"battery-backtest/internal/config"
	"battery-backtest/internal/strategy"
)

type cancelOnExecution struct {
	context.Context
	calls int
}

func (c *cancelOnExecution) Err() error {
	c.calls++
	if c.calls > 1 {
		return context.Canceled
	}
	return nil
}

func TestComparePreflightsAggregateTransitionsBeforeAnyExecution(t *testing.T) {
	input := baseInput()
	input.Strategy.Params.SOCSteps, input.Strategy.Params.PowerSteps = ptr(200), ptr(10)
	prepared, err := PrepareComparison(input, []Variation{{Name: "one"}, {Name: "two"}, {Name: "three"}}, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelOnExecution{Context: context.Background()}
	out, err := Compare(ctx, prepared, syntheticRows(8640), Options{})
	var budget *strategy.BudgetError
	if !errors.As(err, &budget) || budget.Dimension != "transitions" || out != nil || ctx.calls != 1 {
		t.Fatalf("aggregate preflight = %+v, %v, context checks %d", out, err, ctx.calls)
	}
}

func TestCompareAccountsForRetainedOutputMemory(t *testing.T) {
	input := baseInput()
	input.Strategy = config.StrategyOverride{Name: ptr("schedule"), Params: config.StrategyParamsOverride{ChargePowerMW: ptr(0.0), DischargePowerMW: ptr(0.0)}}
	variations := make([]Variation, MaxVariations)
	for i := range variations {
		variations[i].Name = string(rune('a' + i))
	}
	prepared, err := PrepareComparison(input, variations, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelOnExecution{Context: context.Background()}
	out, err := Compare(ctx, prepared, syntheticRows(strategy.MaxIntervals), Options{})
	var budget *strategy.BudgetError
	if !errors.As(err, &budget) || budget.Dimension != "working_bytes" || out != nil || ctx.calls != 1 {
		t.Fatalf("retained memory preflight: %+v %v checks %d", out, err, ctx.calls)
	}
}

func TestRunCannotHideTooManyRowsBehindLimit(t *testing.T) {
	_, err := Run(context.Background(), prepare(t, baseInput()), syntheticRows(strategy.MaxIntervals+1), Options{LimitIntervals: 1})
	var budget *strategy.BudgetError
	if !errors.As(err, &budget) || budget.Dimension != "intervals" {
		t.Fatalf("series limit hidden: %v", err)
	}
}

func TestBudgetIncludesLongIntervalWindowsAndCheckedArithmetic(t *testing.T) {
	rows := syntheticRows(1)
	rows[0].IntervalEndUTC = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	rows[0].IntervalEndLocal = rows[0].IntervalEndUTC
	_, err := Run(context.Background(), prepare(t, baseInput()), rows, Options{})
	var budget *strategy.BudgetError
	if !errors.As(err, &budget) || budget.Dimension != "working_bytes" {
		t.Fatalf("unbounded summary windows: %v", err)
	}
	for _, values := range [][2]uint64{{^uint64(0), 8}, {8, ^uint64(0)}, {strategy.MaxWorkingBytes, 2}} {
		total := uint64(0)
		if err := addAllocation(&total, values[0], values[1]); err == nil || total != 0 {
			t.Fatalf("unchecked allocation arithmetic: %v, total %v", values, total)
		}
	}
	for _, name := range []string{"variable-gap.json", "midnight.json", "dst-fall.json"} {
		rows := fixtureRows(t, name)
		if _, err := outputBytes(rows); err != nil {
			t.Fatalf("ordinary time fixture rejected: %s %v", name, err)
		}
	}
}

func BenchmarkRun(b *testing.B) {
	input := baseInput()
	input.Strategy.Params.SOCSteps, input.Strategy.Params.PowerSteps = ptr(200), ptr(10)
	prepared := prepare(b, input)
	rows := syntheticRows(288)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Run(context.Background(), prepared, rows, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
