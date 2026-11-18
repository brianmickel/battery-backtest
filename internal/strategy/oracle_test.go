package strategy

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"battery-backtest/internal/config"
	"battery-backtest/internal/model"
)

func intervalsAt(start time.Time, prices []float64, durations ...time.Duration) []model.LMPInterval {
	rows := make([]model.LMPInterval, len(prices))
	for i, price := range prices {
		duration := time.Hour
		if len(durations) > 0 {
			duration = durations[i%len(durations)]
		}
		end := start.Add(duration)
		rows[i] = model.LMPInterval{IntervalStartLocal: start, IntervalEndLocal: end,
			IntervalStartUTC: start.UTC(), IntervalEndUTC: end.UTC(), Location: "node", Market: "market", LMP: price}
		start = end
	}
	return rows
}

func hourlyIntervals(prices []float64) []model.LMPInterval {
	return intervalsAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), prices)
}

func idealParams() model.BatteryParams {
	return model.BatteryParams{EnergyCapacityMWh: 1, PowerCapacityMW: 1, ChargeEfficiency: 1, DischargeEfficiency: 1, MaxSOC: 1}
}

func buildOracle(t *testing.T, rows []model.LMPInterval, params model.BatteryParams, initial float64, cfg config.OracleConfig) (*OracleStrategy, *model.Battery) {
	t.Helper()
	battery, err := model.NewBattery(params, initial)
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(context.Background(), cfg, rows, battery)
	if err != nil {
		t.Fatal(err)
	}
	if battery.State.SOC != initial {
		t.Fatalf("planning mutated initial SOC to %v", battery.State.SOC)
	}
	return built.(*OracleStrategy), battery
}

func replay(t *testing.T, rows []model.LMPInterval, plan Strategy, battery *model.Battery) float64 {
	t.Helper()
	stored := battery.State.SOC * battery.Params.EnergyCapacityMWh
	total := 0.0
	for i, row := range rows {
		dispatch := plan.Decide(Context{Index: i, Interval: row, Battery: battery})
		next, expected, err := model.SimulateInterval(battery.Params, battery.State, row.LMP, dispatch, row.DurationHours())
		if err != nil {
			t.Fatal(err)
		}
		actual, err := battery.ApplyDispatch(row.LMP, dispatch, row.DurationHours())
		if err != nil {
			t.Fatal(err)
		}
		if actual != expected || next != battery.State {
			t.Fatal("pure/mutable replay differs")
		}
		if math.Abs(actual.PowerMW-dispatch.PowerMW) > 1e-9 {
			t.Fatalf("row %d planned infeasible power: %+v, %+v", i, dispatch, actual)
		}
		stored += actual.EnergyFromGridMWh*battery.Params.ChargeEfficiency - actual.EnergyToGridMWh/battery.Params.DischargeEfficiency
		if math.Abs(stored-battery.State.SOC*battery.Params.EnergyCapacityMWh) > 1e-9 {
			t.Fatalf("row %d created or destroyed stored energy", i)
		}
		total += actual.PNL
	}
	return total
}

func TestOracleCrossMidnightAndDSTCarryInventory(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	for name, start := range map[string]time.Time{
		"midnight":       time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC),
		"fall_back":      time.Date(2025, 11, 1, 23, 0, 0, 0, la),
		"spring_forward": time.Date(2025, 3, 8, 23, 0, 0, 0, la),
	} {
		t.Run(name, func(t *testing.T) {
			rows := intervalsAt(start, []float64{-10, 100, 0, 0})
			plan, battery := buildOracle(t, rows, idealParams(), 0, config.OracleConfig{SOCSteps: 20, PowerSteps: 1})
			if got := replay(t, rows, plan, battery); math.Abs(got-110) > 1e-9 {
				t.Fatalf("cash flow = %v, want 110 (daily reset produced 10)", got)
			}
			if battery.State.SOC != 0 {
				t.Fatalf("terminal inventory %v, want empty", battery.State.SOC)
			}
		})
	}
}

func TestOracleDischargesInitialInventoryWithoutAcquisitionCharge(t *testing.T) {
	params := idealParams()
	params.ChargeEfficiency, params.DischargeEfficiency, params.DegradationCostPerMWh = .9, .9, 1
	rows := hourlyIntervals([]float64{50, 50, 50})
	plan, battery := buildOracle(t, rows, params, .371, config.OracleConfig{SOCSteps: 20, PowerSteps: 4})
	for _, dispatch := range plan.plan {
		if dispatch.PowerMW < 0 {
			t.Fatal("flat positive prices should not cause charging")
		}
	}
	if got, want := replay(t, rows, plan, battery), .371*.9*49; math.Abs(got-want) > 1e-9 {
		t.Fatalf("cash flow = %.12f, want %.12f", got, want)
	}
}

func TestOracleDoesNotChargeForRoundedAwayEnergy(t *testing.T) {
	rows := intervalsAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), []float64{-100, -100, 100}, 6*time.Minute)
	plan, battery := buildOracle(t, rows, idealParams(), 0, config.OracleConfig{SOCSteps: 2, PowerSteps: 1})
	if got := replay(t, rows, plan, battery); got != 0 {
		t.Fatalf("unreachable half-MWh grid should idle, got cash flow %v", got)
	}
	for _, d := range plan.plan {
		if d.PowerMW != 0 {
			t.Fatalf("rounded-away energy booked: %+v", plan.plan)
		}
	}
}

func TestOracleProjectedTransitionsConserveEnergy(t *testing.T) {
	params := idealParams()
	params.EnergyCapacityMWh, params.ChargeEfficiency, params.DischargeEfficiency = 2.3, .87, .91
	params.MinSOC, params.MaxSOC, params.DegradationCostPerMWh = .13, .92, 2.5
	rows := intervalsAt(time.Date(2025, 1, 1, 22, 0, 0, 0, time.UTC), []float64{-31, 12, 93, -4, 101, 7}, 17*time.Minute, 43*time.Minute, 91*time.Minute)
	cfg := config.OracleConfig{SOCSteps: 7, PowerSteps: 3}
	grid, initial := socGrid(params, .371, cfg.SOCSteps)
	if grid[initial] != .371 {
		t.Fatalf("initial SOC rounded: %v", grid[initial])
	}
	for _, row := range rows {
		for source := range grid {
			for k := -cfg.PowerSteps; k <= cfg.PowerSteps; k++ {
				target, result, err := projectTransition(grid, source, float64(k)/float64(cfg.PowerSteps)*params.PowerCapacityMW, row, params)
				if err != nil {
					t.Fatal(err)
				}
				next, actual, err := model.SimulateInterval(params, model.BatteryState{SOC: grid[source]}, row.LMP, model.Dispatch{PowerMW: result.PowerMW}, row.DurationHours())
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(next.SOC-grid[target]) > 1e-12 || math.Abs(actual.PNL-result.PNL) > 1e-10 {
					t.Fatalf("booked transition differs from replay: %+v, %+v", result, actual)
				}
			}
		}
	}
	plan, battery := buildOracle(t, rows, params, .371, cfg)
	replay(t, rows, plan, battery)
}

// Enumerate complete feasible paths independently of the DP and its projection.
// Quarter-hour/quarter-SOC steps make every grid edge reachable by the action set.
func TestOracleMatchesTinyEnumeratedGridOptimum(t *testing.T) {
	rows := intervalsAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), []float64{-20, 80, 5, 100}, 30*time.Minute)
	params := idealParams()
	var enumerate func(int, model.BatteryState) float64
	enumerate = func(index int, state model.BatteryState) float64 {
		if index == len(rows) {
			return 0
		}
		best := math.Inf(-1)
		for _, power := range []float64{-1, -.5, 0, .5, 1} {
			next, result, err := model.SimulateInterval(params, state, rows[index].LMP, model.Dispatch{PowerMW: power}, rows[index].DurationHours())
			if err != nil {
				t.Fatal(err)
			}
			best = math.Max(best, result.PNL+enumerate(index+1, next))
		}
		return best
	}
	plan, battery := buildOracle(t, rows, params, .25, config.OracleConfig{SOCSteps: 4, PowerSteps: 2})
	if got, want := replay(t, rows, plan, battery), enumerate(0, model.BatteryState{SOC: .25}); math.Abs(got-want) > 1e-9 {
		t.Fatalf("DP = %v, enumerated optimum = %v", got, want)
	}
}

func TestOracleStableTiesAndLockedSOC(t *testing.T) {
	rows := hourlyIntervals([]float64{0, 0, 0})
	cfg := config.OracleConfig{SOCSteps: 4, PowerSteps: 2}
	first, _ := buildOracle(t, rows, idealParams(), .371, cfg)
	for i := 0; i < 10; i++ {
		plan, battery := buildOracle(t, rows, idealParams(), .371, cfg)
		if !reflect.DeepEqual(plan.plan, first.plan) {
			t.Fatal("nondeterministic equal-value plan")
		}
		replay(t, rows, plan, battery)
	}
	params := idealParams()
	params.MinSOC, params.MaxSOC = .371, .371
	plan, battery := buildOracle(t, rows, params, .371, cfg)
	for _, d := range plan.plan {
		if d.PowerMW != 0 {
			t.Fatal("locked inventory must idle")
		}
	}
	replay(t, rows, plan, battery)
}

// Cancellation on a deterministic Err call proves checks happen inside one
// large timestep, without timing-dependent sleeps or goroutine scheduling.
type countingContext struct {
	context.Context
	calls, cancelAt int
}

func (c *countingContext) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestOracleCancellationWithinCandidates(t *testing.T) {
	rows := hourlyIntervals([]float64{-1, 100, -1, 100})
	battery, _ := model.NewBattery(idealParams(), .5)
	ctx := &countingContext{Context: context.Background(), cancelAt: 5}
	_, err := Build(ctx, config.OracleConfig{SOCSteps: 1000, PowerSteps: 50}, rows, battery)
	if !errors.Is(err, context.Canceled) || ctx.calls != 5 {
		t.Fatalf("cancel error = %v, checks = %d", err, ctx.calls)
	}
	if battery.State.SOC != .5 {
		t.Fatal("canceled plan mutated battery")
	}
}

func BenchmarkOracle(b *testing.B) {
	for _, count := range []int{288, 8640} {
		name := "one_day"
		if count == 8640 {
			name = "thirty_days"
		}
		b.Run(name, func(b *testing.B) {
			prices := make([]float64, count)
			for i := range prices {
				prices[i] = 40 + 60*math.Sin(float64(i%288)*2*math.Pi/288)
			}
			rows := intervalsAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), prices, 5*time.Minute)
			battery, _ := model.NewBattery(idealParams(), .371)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Build(context.Background(), config.OracleConfig{SOCSteps: 200, PowerSteps: 10}, rows, battery); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestOracleEqualValuesPreferSmallerPower(t *testing.T) {
	for _, prices := range [][]float64{{0}, {0, 0}, {0, 0, 0}} {
		plan, battery := buildOracle(t, hourlyIntervals(prices), idealParams(), .371, config.OracleConfig{SOCSteps: 4, PowerSteps: 2})
		for _, dispatch := range plan.plan {
			if dispatch.PowerMW != 0 {
				t.Fatalf("equal-value horizon %v did not prefer idle: %+v", prices, plan.plan)
			}
		}
		replay(t, hourlyIntervals(prices), plan, battery)
		if battery.State.SOC != .371 {
			t.Fatalf("idle horizon changed inventory: %v", battery.State.SOC)
		}
	}
}

func TestOracleRandomizedReplayConservation(t *testing.T) {
	random := rand.New(rand.NewSource(20261014))
	for trial := 0; trial < 40; trial++ {
		params := idealParams()
		params.EnergyCapacityMWh = .5 + random.Float64()*5
		params.ChargeEfficiency = .5 + random.Float64()*.5
		params.DischargeEfficiency = .5 + random.Float64()*.5
		params.MinSOC, params.MaxSOC = .1, .9
		params.DegradationCostPerMWh = random.Float64() * 5
		prices := make([]float64, 12)
		durations := make([]time.Duration, len(prices))
		for i := range prices {
			prices[i] = -50 + random.Float64()*200
			durations[i] = time.Duration(1+random.Intn(120)) * time.Minute
		}
		rows := intervalsAt(time.Date(2025, 1, 1, 23, 0, 0, 0, time.UTC), prices, durations...)
		initial := .1 + random.Float64()*.8
		plan, battery := buildOracle(t, rows, params, initial, config.OracleConfig{SOCSteps: 3 + random.Intn(30), PowerSteps: 1 + random.Intn(8)})
		replay(t, rows, plan, battery)
	}
}
