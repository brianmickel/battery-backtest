package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

func ptr[T any](value T) *T { return &value }
func baseInput() config.Input {
	return config.Input{Battery: config.BatteryOverride{EnergyCapacityMWh: ptr(1.0), PowerCapacityMW: ptr(1.0),
		ChargeEfficiency: ptr(1.0), DischargeEfficiency: ptr(1.0), MinSOC: ptr(0.0), MaxSOC: ptr(1.0), InitialSOC: ptr(0.0)},
		Strategy: config.StrategyOverride{Name: ptr("oracle"), Params: config.StrategyParamsOverride{SOCSteps: ptr(20), PowerSteps: ptr(4)}}}
}
func fixtureRows(t testing.TB, name string) []model.LMPInterval {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := data.DecodeGridStatusJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
func prepare(t testing.TB, input config.Input) Prepared {
	t.Helper()
	prepared, err := Prepare(input, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}
func syntheticRows(count int) []model.LMPInterval {
	rows := make([]model.LMPInterval, count)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range rows {
		end := start.Add(5 * time.Minute)
		rows[i] = model.LMPInterval{Location: "node", Market: "market", IntervalStartUTC: start, IntervalEndUTC: end,
			IntervalStartLocal: start, IntervalEndLocal: end, LMP: float64(i%100) - 20}
		start = end
	}
	return rows
}

func TestRunHTTPAndCLIConfigParityAndRepeatability(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/single-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Config config.Input
		Raw    json.RawMessage
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	var wire struct{ Config json.RawMessage }
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "experiment.yaml")
	if err := os.WriteFile(path, wire.Config, 0600); err != nil {
		t.Fatal(err)
	}
	cliInput, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := fixtureRows(t, "two-price.json")
	before := append([]model.LMPInterval(nil), rows...)
	prepared := prepare(t, request.Config)
	first, err := Run(context.Background(), prepared, rows, Options{Clock: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Summary.TotalPNL != 90 || first.Summary.InitialSOC != 0 || first.Summary.FinalSOC != 0 {
		t.Fatalf("fixture summary: %+v", first.Summary)
	}
	second, err := Run(context.Background(), prepared, rows, Options{Clock: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	cli, err := Run(context.Background(), prepare(t, cliInput), rows, Options{Clock: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, cli) {
		t.Fatal("repeat/adapter input produced different ledger or summary")
	}
	if !reflect.DeepEqual(rows, before) {
		t.Fatal("execution mutated shared input")
	}
	*request.Config.Battery.InitialSOC = .75
	*request.Config.Strategy.Params.SOCSteps = 2
	third, err := Run(context.Background(), prepared, rows, Options{Clock: "UTC"})
	if err != nil || !reflect.DeepEqual(first, third) {
		t.Fatal("prepared execution retained boundary patch pointers")
	}
}

func TestRunConcurrentReuseHasIndependentState(t *testing.T) {
	prepared := prepare(t, baseInput())
	rows := fixtureRows(t, "two-price.json")
	var outputs [2]Outcome
	var failures [2]error
	var group sync.WaitGroup
	for i := range outputs {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			outputs[index], failures[index] = Run(context.Background(), prepared, rows, Options{})
		}(i)
	}
	group.Wait()
	for _, err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(outputs[0], outputs[1]) {
		t.Fatal("concurrent prepared reuse leaked mutable state")
	}
	outputs[0].Ledger[0].PNL = 123456
	if outputs[1].Ledger[0].PNL == 123456 {
		t.Fatal("ledger backing arrays shared")
	}
}

func TestRunNormalizesWholeSeriesBeforeChronologicalLimit(t *testing.T) {
	prepared := prepare(t, baseInput())
	rows := fixtureRows(t, "two-price.json")
	rows[0], rows[1] = rows[1], rows[0]
	outcome, err := Run(context.Background(), prepared, rows, Options{LimitIntervals: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Ledger) != 1 || outcome.Ledger[0].LMP != 10 {
		t.Fatalf("limit did not select chronological prefix: %+v", outcome.Ledger)
	}
	rows[0].LMP = math.NaN()
	_, err = Run(context.Background(), prepared, rows, Options{LimitIntervals: 1})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "intervals[0].lmp" {
		t.Fatalf("hidden invalid suffix: %v", err)
	}
	rows = fixtureRows(t, "two-price.json")
	rows[1] = rows[0]
	if _, err := Run(context.Background(), prepared, rows, Options{LimitIntervals: 1}); err == nil {
		t.Fatal("limit hid duplicate timestamp")
	}
}

func TestRunInitialInventoryDurationsAndGaps(t *testing.T) {
	input := baseInput()
	input.Battery.InitialSOC = ptr(.5)
	outcome, err := Run(context.Background(), prepare(t, input), fixtureRows(t, "variable-gap.json"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := outcome.Summary
	if s.InitialSOC != .5 || s.InitialEnergyMWh != .5 || s.CoveredDurationHours != .75 || s.GapDurationHours != .25 || s.GapCount != 1 {
		t.Fatalf("inventory/coverage: %+v", s)
	}
	if math.Abs(s.TotalPNL-42.5) > 1e-9 || math.Abs(s.FinalEnergyMWh-.25) > 1e-9 {
		t.Fatalf("variable-duration replay: %+v", s)
	}
	if outcome.Ledger[1].SOCStart != outcome.Ledger[0].SOCEnd {
		t.Fatal("gap changed SOC")
	}
}

func TestRunClockControlsScheduleWithoutMutatingSource(t *testing.T) {
	input := baseInput()
	input.Strategy = config.StrategyOverride{Name: ptr("schedule"), Params: config.StrategyParamsOverride{
		ChargeStart: ptr("23:00"), ChargeEnd: ptr("00:00"), DischargePowerMW: ptr(0.0)}}
	rows := fixtureRows(t, "midnight.json")
	market, err := Run(context.Background(), prepare(t, input), rows, Options{Clock: "market"})
	if err != nil {
		t.Fatal(err)
	}
	utc, err := Run(context.Background(), prepare(t, input), rows, Options{Clock: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if market.Summary.EnergyChargedMWh != .5 || len(market.Summary.ChargeWindows) != 2 || utc.Summary.EnergyChargedMWh != 0 {
		t.Fatalf("clock-dependent dispatch: market %+v UTC %+v", market.Summary, utc.Summary)
	}
	if rows[0].IntervalStartLocal.Hour() != 23 || utc.Ledger[0].IntervalStartLocal.Hour() != 7 {
		t.Fatal("clock normalization mutated source or failed UTC copy")
	}
}

func TestCompareOrderIndependenceAndSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/compare-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Base       config.Input `json:"base_config"`
		Variations []Variation  `json:"variations"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareComparison(request.Base, request.Variations, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	rows := fixtureRows(t, "two-price.json")
	before := append([]model.LMPInterval(nil), rows...)
	first, err := Compare(context.Background(), prepared, rows, Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Compare(context.Background(), prepared, rows, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(rows, before) {
		t.Fatal("comparison reused state or changed shared rows")
	}
	if first[0].Name != "Idle at explicit zero" || first[1].Name != "Oracle" || first[0].Outcome.Summary.TotalPNL != 0 || first[1].Outcome.Summary.TotalPNL != 90 {
		t.Fatalf("comparison order/result: %+v", first)
	}
	for i, item := range prepared {
		single, err := Run(context.Background(), item.Prepared, rows, Options{})
		if err != nil || !reflect.DeepEqual(single, first[i].Outcome) {
			t.Fatalf("variation %s differs from single run: %v", item.Name, err)
		}
	}
	first[0].Outcome.Ledger[0].PNL = 123456
	if first[1].Outcome.Ledger[0].PNL == 123456 {
		t.Fatal("variation ledger backing storage aliased")
	}
}

func TestPrepareComparisonIncompleteBaseAndInvalidLaterVariation(t *testing.T) {
	complete := baseInput()
	base := config.Input{Strategy: complete.Strategy}
	variations := []Variation{{Name: "first", Config: config.Input{Battery: complete.Battery}}, {Name: "later", Config: config.Input{Battery: complete.Battery}}}
	if _, err := Prepare(base, config.PresetCatalog{}); err == nil {
		t.Fatal("fixture base unexpectedly complete")
	}
	if _, err := PrepareComparison(base, variations, config.PresetCatalog{}); err != nil {
		t.Fatal(err)
	}
	variations[1].Config.Battery.InitialSOC = ptr(1.1)
	providerCalls := 0
	prepared, err := PrepareComparison(base, variations, config.PresetCatalog{})
	if err == nil {
		providerCalls++
	}
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Variation != "later" || validation.Field != "config.battery.initial_soc" || prepared != nil || providerCalls != 0 {
		t.Fatalf("later failure: prepared %+v error %v fetches %d", prepared, err, providerCalls)
	}
}

func TestCompareMiddleRuntimeFailureIsAtomicAndIdentified(t *testing.T) {
	base := baseInput()
	base.Battery.InitialSOC = ptr(.5)
	base.Strategy = config.StrategyOverride{Name: ptr("schedule"), Params: config.StrategyParamsOverride{
		ChargePowerMW: ptr(0.0), DischargeStart: ptr("00:00"), DischargeEnd: ptr("23:59")}}
	variations := []Variation{{Name: "first"}, {Name: "middle", Config: config.Input{Battery: config.BatteryOverride{
		EnergyCapacityMWh: ptr(math.MaxFloat64), PowerCapacityMW: ptr(math.MaxFloat64)}}}, {Name: "last"}}
	prepared, err := PrepareComparison(base, variations, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	rows := syntheticRows(1)
	rows[0].LMP = math.MaxFloat64
	outcomes, err := Compare(context.Background(), prepared, rows, Options{})
	var variation *VariationError
	if !errors.As(err, &variation) || variation.Variation != "middle" || outcomes != nil {
		t.Fatalf("partial runtime success: %+v, %v", outcomes, err)
	}
}

func TestPrepareComparisonPresetChangePreservesOnlyExplicitOverrides(t *testing.T) {
	directory := t.TempDir()
	for id, values := range map[string][3]float64{"a": {1, 1, .1}, "b": {4, 3, .2}} {
		raw := fmt.Sprintf("battery:\n  energy_capacity_mwh: %v\n  power_capacity_mw: %v\n  charge_efficiency: 1\n  discharge_efficiency: 1\n  min_soc: %v\n  max_soc: 1\n  degradation_cost_per_mwh: 7\n", values[0], values[1], values[2])
		if err := os.WriteFile(filepath.Join(directory, id+".yaml"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	presets, err := config.LoadPresetCatalog(directory)
	if err != nil {
		t.Fatal(err)
	}
	base := config.Input{BatteryFile: "a", Battery: config.BatteryOverride{PowerCapacityMW: ptr(2.0), DegradationCostPerMWh: ptr(0.0)},
		Strategy: config.StrategyOverride{Name: ptr("schedule"), Params: config.StrategyParamsOverride{ChargePowerMW: ptr(0.0), DischargePowerMW: ptr(0.0)}}}
	prepared, err := PrepareComparison(base, []Variation{{Name: "base"}, {Name: "replace", Config: config.Input{BatteryFile: "b"}}}, presets)
	if err != nil {
		t.Fatal(err)
	}
	b := prepared[1].Prepared.resolved.Battery
	if b.EnergyCapacityMWh != 4 || b.PowerCapacityMW != 2 || b.InitialSOC != .2 || b.DegradationCostPerMWh != 0 {
		t.Fatalf("preset changed inherited intent: %+v", b)
	}
	outcomes, err := Compare(context.Background(), prepared, syntheticRows(1), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if outcomes[0].Outcome.Summary.InitialEnergyMWh != .1 || outcomes[1].Outcome.Summary.InitialEnergyMWh != .8 {
		t.Fatal("preset initial inventory was materialized from the base")
	}
}

func TestPrepareAndCompareRejectInvalidLocalValues(t *testing.T) {
	for name, variations := range map[string][]Variation{
		"empty": nil, "blank_name": {{Name: " "}}, "duplicate": {{Name: "same"}, {Name: "same"}}, "too_many": make([]Variation, 17),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := PrepareComparison(baseInput(), variations, config.PresetCatalog{}); err == nil {
				t.Fatal("invalid local comparison accepted")
			}
		})
	}
	input := baseInput()
	input.Strategy.Params.SOCSteps = ptr(int(^uint(0) >> 1))
	_, err := Prepare(input, config.PresetCatalog{})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "config.strategy.params.soc_steps" {
		t.Fatalf("extreme step error: %v", err)
	}
	if _, err := Run(context.Background(), Prepared{}, syntheticRows(1), Options{}); err == nil {
		t.Fatal("unprepared run accepted")
	}
	if _, err := Compare(context.Background(), []NamedPrepared{{Name: "empty"}}, syntheticRows(1), Options{}); err == nil {
		t.Fatal("unprepared variation accepted")
	}
}

func TestRunOptionsAndCancellation(t *testing.T) {
	for _, options := range []Options{{LimitIntervals: -1}, {Clock: "America/Denver"}, {LimitIntervals: strategy.MaxIntervals + 1}} {
		if err := ValidateOptions(options); err == nil {
			t.Fatalf("invalid options accepted: %+v", options)
		}
	}
	prepared := prepare(t, baseInput())
	rows := fixtureRows(t, "two-price.json")
	all, err := Run(context.Background(), prepared, rows, Options{LimitIntervals: 0})
	if err != nil || len(all.Ledger) != 2 {
		t.Fatalf("explicit zero should run all: %+v %v", all, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, prepared, rows, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled run: %v", err)
	}
	if _, err := Compare(ctx, []NamedPrepared{{Name: "one", Prepared: prepared}}, rows, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled compare: %v", err)
	}
	deadline, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if _, err := Run(deadline, prepared, rows, Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
}
