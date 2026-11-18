package analysis

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

func rowsFor(id string, prices []float64, durations ...time.Duration) []model.LMPInterval {
	rows := make([]model.LMPInterval, len(prices))
	start := time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC)
	for i, price := range prices {
		duration := time.Hour
		if len(durations) > 0 {
			duration = durations[i%len(durations)]
		}
		end := start.Add(duration)
		rows[i] = model.LMPInterval{Location: id, Market: "market", LMP: price,
			IntervalStartUTC: start, IntervalEndUTC: end, IntervalStartLocal: start, IntervalEndLocal: end}
		start = end
	}
	return rows
}

func TestComputePotentialVariableDurationMatchesCanonicalExecution(t *testing.T) {
	rows := rowsFor("node", []float64{-10, 100}, 15*time.Minute, time.Hour)
	potential, err := ComputePotential(context.Background(), rows)
	if err != nil {
		t.Fatal(err)
	}
	battery, err := model.NewBattery(CanonicalBatteryParams(), CanonicalInitialSOC)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := strategy.Build(context.Background(), CanonicalStrategyConfig(), rows, battery)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for i, row := range rows {
		result, err := battery.ApplyDispatch(row.LMP, plan.Decide(strategy.Context{Index: i, Interval: row, Battery: battery}), row.DurationHours())
		if err != nil {
			t.Fatal(err)
		}
		total += result.PNL
	}
	if math.Abs(total-77.5) > 1e-9 || potential.OracleProfit != total {
		t.Fatalf("potential = %v, direct = %v, want 77.5", potential.OracleProfit, total)
	}
	if potential.Count != 2 || potential.MeanLMP != 45 || potential.MinLMP != -10 || potential.MaxLMP != 100 {
		t.Fatalf("price statistics: %+v", potential)
	}
	if potential.StartUTC != rows[0].IntervalStartUTC || potential.EndUTC != rows[1].IntervalEndUTC {
		t.Fatal("coverage timestamps changed")
	}
	rows[0], rows[1] = rows[1], rows[0]
	reordered, err := ComputePotential(context.Background(), rows)
	if err != nil || reordered != potential {
		t.Fatalf("normalization changed score: %+v, %v", reordered, err)
	}
}

func TestRankingStableTiesAndAtomicFailure(t *testing.T) {
	input := map[string][]model.LMPInterval{"b": rowsFor("b", []float64{100}), "a": rowsFor("a", []float64{100}), "c": rowsFor("c", []float64{200})}
	for i := 0; i < 20; i++ {
		ranked, err := RankByOracleProfit(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if ranked[0].Location != "c" || ranked[1].Location != "a" || ranked[2].Location != "b" {
			t.Fatalf("rank order: %+v", ranked)
		}
		if ranked[1].OracleProfit != 50 {
			t.Fatalf("initial inventory score = %v, want 50", ranked[1].OracleProfit)
		}
	}
	input["z"] = rowsFor("z", []float64{10})
	input["z"][0].IntervalEndUTC = input["z"][0].IntervalStartUTC
	ranked, err := RankByOracleProfit(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), `location "z"`) || ranked != nil {
		t.Fatalf("invalid location yielded partial success: %+v, %v", ranked, err)
	}
}

func TestRankingRejectsMalformedSeriesAndLocationSets(t *testing.T) {
	valid := rowsFor("node", []float64{10, 20})
	for name, mutate := range map[string]func([]model.LMPInterval){
		"nonfinite":          func(rows []model.LMPInterval) { rows[0].LMP = math.NaN() },
		"duplicate":          func(rows []model.LMPInterval) { rows[1] = rows[0] },
		"mixed_node":         func(rows []model.LMPInterval) { rows[1].Location = "other" },
		"inconsistent_clock": func(rows []model.LMPInterval) { rows[0].IntervalEndLocal = rows[0].IntervalStartLocal },
	} {
		t.Run(name, func(t *testing.T) {
			rows := append([]model.LMPInterval(nil), valid...)
			mutate(rows)
			if _, err := ComputePotential(context.Background(), rows); err == nil {
				t.Fatal("malformed series received a score")
			}
		})
	}
	for _, input := range []map[string][]model.LMPInterval{nil, {"wrong": valid}, {"node": nil}, {"": valid}} {
		if result, err := RankByOracleProfit(context.Background(), input); err == nil || result != nil {
			t.Fatalf("invalid location set returned %+v, %v", result, err)
		}
	}
}

func TestRankingRequestWideBudgetAndCancellation(t *testing.T) {
	input := make(map[string][]model.LMPInterval)
	// Every individual location fits, but ten maximum-size canonical plans exceed
	// the request-wide transition budget. No interval is valid, proving preflight.
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		rows := make([]model.LMPInterval, strategy.MaxIntervals)
		rows[0].Location = id
		input[id] = rows
	}
	_, err := RankByOracleProfit(context.Background(), input)
	var budget *strategy.BudgetError
	if !errors.As(err, &budget) || budget.Dimension != "transitions" {
		t.Fatalf("request-wide budget = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ComputePotential(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("score canceled = %v", err)
	}
	if _, err := RankByOracleProfit(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("rank canceled = %v", err)
	}
}
