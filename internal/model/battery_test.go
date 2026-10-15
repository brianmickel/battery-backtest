package model

import (
	"math"
	"testing"
)

func testBatteryParams() BatteryParams {
	return BatteryParams{
		EnergyCapacityMWh: 10, PowerCapacityMW: 4,
		ChargeEfficiency: 0.8, DischargeEfficiency: 0.5,
		MinSOC: 0.2, MaxSOC: 0.8, DegradationCostPerMWh: 3,
	}
}

func TestNewBatteryValidation(t *testing.T) {
	fields := []struct {
		name string
		set  func(*BatteryParams, float64)
	}{
		{"energy capacity", func(p *BatteryParams, v float64) { p.EnergyCapacityMWh = v }},
		{"power capacity", func(p *BatteryParams, v float64) { p.PowerCapacityMW = v }},
		{"charge efficiency", func(p *BatteryParams, v float64) { p.ChargeEfficiency = v }},
		{"discharge efficiency", func(p *BatteryParams, v float64) { p.DischargeEfficiency = v }},
		{"min SOC", func(p *BatteryParams, v float64) { p.MinSOC = v }},
		{"max SOC", func(p *BatteryParams, v float64) { p.MaxSOC = v }},
		{"degradation", func(p *BatteryParams, v float64) { p.DegradationCostPerMWh = v }},
	}
	for _, field := range fields {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			t.Run(field.name+"/"+floatName(value), func(t *testing.T) {
				params := testBatteryParams()
				field.set(&params, value)
				if battery, err := NewBattery(params, 0.5); err == nil || battery != nil {
					t.Fatalf("NewBattery accepted nonfinite %s: %v", field.name, value)
				}
			})
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0.1, 0.9} {
		if battery, err := NewBattery(testBatteryParams(), value); err == nil || battery != nil {
			t.Errorf("NewBattery accepted invalid initial SOC %v", value)
		}
	}
	invalid := []struct {
		name string
		edit func(*BatteryParams)
	}{
		{"zero energy", func(p *BatteryParams) { p.EnergyCapacityMWh = 0 }},
		{"negative energy", func(p *BatteryParams) { p.EnergyCapacityMWh = -1 }},
		{"zero power", func(p *BatteryParams) { p.PowerCapacityMW = 0 }},
		{"negative power", func(p *BatteryParams) { p.PowerCapacityMW = -1 }},
		{"zero charge efficiency", func(p *BatteryParams) { p.ChargeEfficiency = 0 }},
		{"excess charge efficiency", func(p *BatteryParams) { p.ChargeEfficiency = 1.1 }},
		{"zero discharge efficiency", func(p *BatteryParams) { p.DischargeEfficiency = 0 }},
		{"excess discharge efficiency", func(p *BatteryParams) { p.DischargeEfficiency = 1.1 }},
		{"negative minimum", func(p *BatteryParams) { p.MinSOC = -0.1 }},
		{"excess maximum", func(p *BatteryParams) { p.MaxSOC = 1.1 }},
		{"reversed bounds", func(p *BatteryParams) { p.MinSOC, p.MaxSOC = 0.8, 0.2 }},
		{"negative degradation", func(p *BatteryParams) { p.DegradationCostPerMWh = -1 }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			params := testBatteryParams()
			tc.edit(&params)
			if _, err := NewBattery(params, 0.5); err == nil {
				t.Fatal("NewBattery accepted invalid physical bounds")
			}
		})
	}
	params := testBatteryParams()
	params.ChargeEfficiency, params.DischargeEfficiency = 1, 1
	params.DegradationCostPerMWh = 0
	for _, soc := range []float64{params.MinSOC, 0.5, params.MaxSOC} {
		battery, err := NewBattery(params, soc)
		if err != nil || battery.State.SOC != soc {
			t.Fatalf("valid battery must retain initial SOC %v: battery=%+v, err=%v", soc, battery, err)
		}
	}
	params.MinSOC, params.MaxSOC = 0.5, 0.5
	if _, err := NewBattery(params, 0.5); err != nil {
		t.Fatalf("equal SOC bounds remain valid: %v", err)
	}
}

func TestApplyDispatchRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Battery, *float64, *Dispatch, *float64)
	}{
		{"zero duration", func(_ *Battery, _ *float64, _ *Dispatch, h *float64) { *h = 0 }},
		{"negative duration", func(_ *Battery, _ *float64, _ *Dispatch, h *float64) { *h = -1 }},
		{"invalid state", func(b *Battery, _ *float64, _ *Dispatch, _ *float64) { b.State.SOC = 0.1 }},
		{"invalid params", func(b *Battery, _ *float64, _ *Dispatch, _ *float64) { b.Params.ChargeEfficiency = 0 }},
		{"overflow PNL", func(_ *Battery, p *float64, _ *Dispatch, _ *float64) { *p = math.MaxFloat64 }},
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		tests = append(tests,
			struct {
				name string
				edit func(*Battery, *float64, *Dispatch, *float64)
			}{"price/" + floatName(value), func(_ *Battery, p *float64, _ *Dispatch, _ *float64) { *p = value }},
			struct {
				name string
				edit func(*Battery, *float64, *Dispatch, *float64)
			}{"dispatch/" + floatName(value), func(_ *Battery, _ *float64, d *Dispatch, _ *float64) { d.PowerMW = value }},
			struct {
				name string
				edit func(*Battery, *float64, *Dispatch, *float64)
			}{"duration/" + floatName(value), func(_ *Battery, _ *float64, _ *Dispatch, h *float64) { *h = value }},
			struct {
				name string
				edit func(*Battery, *float64, *Dispatch, *float64)
			}{"state/" + floatName(value), func(b *Battery, _ *float64, _ *Dispatch, _ *float64) { b.State.SOC = value }},
		)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			battery, err := NewBattery(testBatteryParams(), 0.5)
			if err != nil {
				t.Fatal(err)
			}
			price, dispatch, hours := 30.0, Dispatch{PowerMW: -4}, 1.0
			tc.edit(battery, &price, &dispatch, &hours)
			before := math.Float64bits(battery.State.SOC)
			result, err := battery.ApplyDispatch(price, dispatch, hours)
			if err == nil {
				t.Error("ApplyDispatch accepted invalid input or nonfinite output")
			}
			if math.Float64bits(battery.State.SOC) != before {
				t.Error("failed dispatch mutated battery state")
			}
			if result != (IntervalResult{}) {
				t.Errorf("failed dispatch returned a result: %+v", result)
			}
		})
	}
}

func TestApplyDispatchConservesEnergy(t *testing.T) {
	params := testBatteryParams()
	ideal := params
	ideal.ChargeEfficiency, ideal.DischargeEfficiency, ideal.DegradationCostPerMWh = 1, 1, 0
	fixed := params
	fixed.MinSOC, fixed.MaxSOC = 0.5, 0.5
	highPower := params
	highPower.PowerCapacityMW = math.MaxFloat64
	tests := []struct {
		name                          string
		params                        BatteryParams
		soc, power, hours, price      float64
		endSOC, fromGrid, toGrid, pnl float64
	}{
		{"idle", params, 0.5, 0, 0.25, 30, 0.5, 0, 0, 0},
		{"power limited charge", params, 0.5, -100, 0.25, 30, 0.58, 1, 0, -33},
		{"power limited discharge", params, 0.5, 100, 0.25, 30, 0.3, 0, 1, 27},
		{"SOC limited charge", params, 0.5, -4, 2, 30, 0.8, 3.75, 0, -123.75},
		{"SOC limited discharge", params, 0.5, 4, 2, 30, 0.2, 0, 1.5, 40.5},
		{"full charge", params, 0.8, -4, 0.25, 30, 0.8, 0, 0, 0},
		{"empty discharge", params, 0.2, 4, 0.25, 30, 0.2, 0, 0, 0},
		{"negative price charge", params, 0.5, -2, 0.5, -20, 0.58, 1, 0, 17},
		{"negative price discharge", params, 0.5, 2, 0.5, -20, 0.3, 0, 1, -23},
		{"ideal charge", ideal, 0.5, -2, 0.5, 30, 0.6, 1, 0, -30},
		{"ideal discharge", ideal, 0.5, 2, 0.5, 30, 0.4, 0, 1, 30},
		{"fixed bounds charge", fixed, 0.5, -4, 1, 30, 0.5, 0, 0, 0},
		{"fixed bounds discharge", fixed, 0.5, 4, 1, 30, 0.5, 0, 0, 0},
		{"large request", params, 0.5, math.MaxFloat64, 0.25, 30, 0.3, 0, 1, 27},
		{"overflow request clips", highPower, 0.5, math.MaxFloat64, 2, 30, 0.2, 0, 1.5, 40.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			battery, err := NewBattery(tc.params, tc.soc)
			if err != nil {
				t.Fatal(err)
			}
			result, err := battery.ApplyDispatch(tc.price, Dispatch{PowerMW: tc.power}, tc.hours)
			if err != nil {
				t.Fatal(err)
			}
			assertNear(t, "SOC end", result.SOCEnd, tc.endSOC)
			assertNear(t, "grid charge", result.EnergyFromGridMWh, tc.fromGrid)
			assertNear(t, "grid discharge", result.EnergyToGridMWh, tc.toGrid)
			assertNear(t, "PNL", result.PNL, tc.pnl)
			assertConserved(t, tc.params, BatteryState{SOC: tc.soc}, battery.State, tc.price, Dispatch{PowerMW: tc.power}, tc.hours, result)
		})
	}
}

func assertConserved(t *testing.T, params BatteryParams, start, end BatteryState, price float64, dispatch Dispatch, hours float64, result IntervalResult) {
	t.Helper()
	if end.SOC < params.MinSOC || end.SOC > params.MaxSOC {
		t.Fatalf("SOC %v outside configured bounds [%v, %v]", end.SOC, params.MinSOC, params.MaxSOC)
	}
	assertNear(t, "SOC start", result.SOCStart, start.SOC)
	assertNear(t, "SOC state", result.SOCEnd, end.SOC)
	if result.EnergyFromGridMWh < 0 || result.EnergyToGridMWh < 0 || result.EnergyFromGridMWh*result.EnergyToGridMWh != 0 {
		t.Fatalf("invalid directional grid energies: %+v", result)
	}
	assertNear(t, "throughput", result.ThroughputMWh, result.EnergyFromGridMWh+result.EnergyToGridMWh)
	assertNear(t, "realized power", result.PowerMW*hours, result.EnergyToGridMWh-result.EnergyFromGridMWh)
	if math.Abs(result.PowerMW) > math.Min(math.Abs(dispatch.PowerMW), params.PowerCapacityMW)+1e-9 || result.PowerMW*dispatch.PowerMW < 0 {
		t.Fatalf("realized power %v violates request %v or capacity %v", result.PowerMW, dispatch.PowerMW, params.PowerCapacityMW)
	}
	storedDelta := (end.SOC - start.SOC) * params.EnergyCapacityMWh
	chargeStored := result.EnergyFromGridMWh * params.ChargeEfficiency
	dischargeWithdrawn := result.EnergyToGridMWh / params.DischargeEfficiency
	assertNear(t, "stored energy", storedDelta, chargeStored-dischargeWithdrawn)
	losses := result.EnergyFromGridMWh - chargeStored + dischargeWithdrawn - result.EnergyToGridMWh
	assertNear(t, "energy and losses", result.EnergyFromGridMWh-result.EnergyToGridMWh, storedDelta+losses)
	assertNear(t, "cash flow", result.PNL, price*(result.EnergyToGridMWh-result.EnergyFromGridMWh)-params.DegradationCostPerMWh*result.ThroughputMWh)
}

func assertNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	tolerance := math.Max(1e-9, 1e-9*math.Max(math.Abs(got), math.Abs(want)))
	if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > tolerance {
		t.Errorf("%s: got %.17g, want %.17g (tolerance %g)", label, got, want, tolerance)
	}
}

func floatName(value float64) string {
	if math.IsNaN(value) {
		return "NaN"
	}
	if math.IsInf(value, 1) {
		return "+Inf"
	}
	return "-Inf"
}
