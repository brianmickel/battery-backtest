package model

import (
	"math"
	"testing"
)

func TestSimulateIntervalPureAndMutableSequences(t *testing.T) {
	for _, efficiency := range []float64{0.55, 0.9, 1} {
		params := testBatteryParams()
		params.ChargeEfficiency, params.DischargeEfficiency = efficiency, efficiency
		battery, err := NewBattery(params, 0.37)
		if err != nil {
			t.Fatal(err)
		}
		pureState := battery.State
		for i, power := range []float64{-100, -4, 0, 2, 100, 4, -1, -100, 100} {
			price, hours := float64(i*15-40), float64(i%3+1)/4
			before := pureState
			next, result, err := SimulateInterval(params, pureState, price, Dispatch{PowerMW: power}, hours)
			if err != nil {
				t.Fatal(err)
			}
			if pureState != before || battery.State != before {
				t.Fatal("pure transition mutated caller state")
			}
			actual, err := battery.ApplyDispatch(price, Dispatch{PowerMW: power}, hours)
			if err != nil {
				t.Fatal(err)
			}
			if actual != result || battery.State != next {
				t.Fatalf("pure and mutable paths disagree: pure=%+v/%+v, mutable=%+v/%+v", next, result, battery.State, actual)
			}
			assertConserved(t, params, before, next, price, Dispatch{PowerMW: power}, hours, result)
			pureState = next
		}
	}
}

func TestSimulateIntervalFailureRetainsInput(t *testing.T) {
	type input struct {
		params BatteryParams
		state  BatteryState
		price  float64
		power  float64
		hours  float64
	}
	valid := input{testBatteryParams(), BatteryState{SOC: 0.5}, 30, -4, 1}
	var invalid []input
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, edit := range []func(*input){
			func(v *input) { v.params.EnergyCapacityMWh = value },
			func(v *input) { v.params.PowerCapacityMW = value },
			func(v *input) { v.params.ChargeEfficiency = value },
			func(v *input) { v.params.DischargeEfficiency = value },
			func(v *input) { v.params.MinSOC = value },
			func(v *input) { v.params.MaxSOC = value },
			func(v *input) { v.params.DegradationCostPerMWh = value },
			func(v *input) { v.state.SOC = value },
			func(v *input) { v.price = value },
			func(v *input) { v.power = value },
			func(v *input) { v.hours = value },
		} {
			v := valid
			edit(&v)
			invalid = append(invalid, v)
		}
	}
	for _, hours := range []float64{0, -1} {
		v := valid
		v.hours = hours
		invalid = append(invalid, v)
	}
	overflow := valid
	overflow.params.EnergyCapacityMWh = math.MaxFloat64
	overflow.params.PowerCapacityMW = math.MaxFloat64
	overflow.params.ChargeEfficiency = 0.01
	overflow.power, overflow.hours = -math.MaxFloat64, 100
	invalid = append(invalid, overflow)
	for i, v := range invalid {
		next, result, err := SimulateInterval(v.params, v.state, v.price, Dispatch{PowerMW: v.power}, v.hours)
		if err == nil || result != (IntervalResult{}) || math.Float64bits(next.SOC) != math.Float64bits(v.state.SOC) {
			t.Errorf("case %d: failure must preserve original state and return no result: next=%+v, result=%+v, err=%v", i, next, result, err)
		}
	}
}

func FuzzSimulateIntervalConservation(f *testing.F) {
	f.Add([]byte{255, 255, 255, 255, 255, 0, 255, 128, 0, 128, 255, 255, 0, 255})
	f.Add([]byte{100, 0, 255, 128, 64, 128, 0, 0, 3, 127, 0, 1, 128, 0})
	f.Add([]byte{0, 0, 255, 0, 0, 20, 255, 255, 255, 128, 255, 255, 128, 0})
	f.Add([]byte{40, 0, 200, 230, 150, 25, 230, 125, 3, 127, 0, 60, 0, 50, 128, 0, 60, 0, 200, 0, 0, 60, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 14 {
			return
		}
		params := BatteryParams{
			EnergyCapacityMWh:     0.01 + float64(uint16(data[0])<<8|uint16(data[1]))/16,
			PowerCapacityMW:       0.01 + float64(data[2])*4,
			ChargeEfficiency:      0.01 + 0.99*float64(data[3])/255,
			DischargeEfficiency:   0.01 + 0.99*float64(data[4])/255,
			MinSOC:                float64(data[5]) / 255,
			DegradationCostPerMWh: float64(data[8]) / 4,
		}
		params.MaxSOC = params.MinSOC + (1-params.MinSOC)*float64(data[6])/255
		state := BatteryState{SOC: params.MinSOC + (params.MaxSOC-params.MinSOC)*float64(data[7])/255}
		battery, err := NewBattery(params, state.SOC)
		if err != nil {
			t.Fatal(err)
		}
		// Bound each fuzz case to 32 intervals while preserving state between them.
		for offset := 9; offset+5 <= len(data) && offset < 9+32*5; offset += 5 {
			power := params.PowerCapacityMW * float64(int8(data[offset])) / 32
			hours := (1 + float64(uint16(data[offset+1])<<8|uint16(data[offset+2]))) / 3600
			price := float64(int16(uint16(data[offset+3])<<8|uint16(data[offset+4]))) / 10
			dispatch := Dispatch{PowerMW: power}
			next, result, err := SimulateInterval(params, state, price, dispatch, hours)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := battery.ApplyDispatch(price, dispatch, hours)
			if err != nil {
				t.Fatal(err)
			}
			if actual != result || battery.State != next {
				t.Fatal("pure and mutable paths disagree")
			}
			assertConserved(t, params, state, next, price, dispatch, hours, result)
			state = next
		}
	})
}
