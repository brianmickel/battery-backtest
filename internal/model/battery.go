package model

import (
	"errors"
	"math"
)

// BatteryParams defines the physical and economic parameters of the battery.
// Units:
// - EnergyCapacityMWh: MWh
// - PowerCapacityMW: MW
// - Efficiencies: 0..1
// - SOC: fraction 0..1
// - DegradationCostPerMWh: $/MWh throughput (charge + discharge)
type BatteryParams struct {
	EnergyCapacityMWh     float64
	PowerCapacityMW       float64
	ChargeEfficiency      float64
	DischargeEfficiency   float64
	MinSOC                float64
	MaxSOC                float64
	DegradationCostPerMWh float64
}

// BatteryState captures mutable state.
type BatteryState struct {
	// SOC is the state of charge as a fraction [0,1].
	SOC float64
}

// Battery is a convenience wrapper bundling params + state.
type Battery struct {
	Params BatteryParams
	State  BatteryState
}

func NewBattery(params BatteryParams, initialSOC float64) (*Battery, error) {
	b := &Battery{
		Params: params,
		State:  BatteryState{SOC: initialSOC},
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Battery) Validate() error {
	p := b.Params
	if !finiteBatteryValue(p.EnergyCapacityMWh) || p.EnergyCapacityMWh <= 0 {
		return errors.New("EnergyCapacityMWh must be finite and > 0")
	}
	if !finiteBatteryValue(p.PowerCapacityMW) || p.PowerCapacityMW <= 0 {
		return errors.New("PowerCapacityMW must be finite and > 0")
	}
	if !finiteBatteryValue(p.ChargeEfficiency) || p.ChargeEfficiency <= 0 || p.ChargeEfficiency > 1 {
		return errors.New("ChargeEfficiency must be finite and in (0, 1]")
	}
	if !finiteBatteryValue(p.DischargeEfficiency) || p.DischargeEfficiency <= 0 || p.DischargeEfficiency > 1 {
		return errors.New("DischargeEfficiency must be finite and in (0, 1]")
	}
	if !finiteBatteryValue(p.MinSOC) || !finiteBatteryValue(p.MaxSOC) || p.MinSOC < 0 || p.MaxSOC > 1 || p.MinSOC > p.MaxSOC {
		return errors.New("MinSOC/MaxSOC must be finite and satisfy 0<=MinSOC<=MaxSOC<=1")
	}
	if !finiteBatteryValue(b.State.SOC) || b.State.SOC < p.MinSOC || b.State.SOC > p.MaxSOC {
		return errors.New("SOC must be finite and within [MinSOC, MaxSOC]")
	}
	if !finiteBatteryValue(p.DegradationCostPerMWh) || p.DegradationCostPerMWh < 0 {
		return errors.New("DegradationCostPerMWh must be finite and >= 0")
	}
	return nil
}

// Dispatch represents a requested power setpoint for an interval.
// Convention: positive MW = discharge to grid, negative MW = charge from grid.
type Dispatch struct {
	PowerMW float64
}

// IntervalResult captures what happened in one interval.
type IntervalResult struct {
	PowerMW           float64 // realized power (may be clipped)
	EnergyToGridMWh   float64 // discharge energy delivered to grid
	EnergyFromGridMWh float64 // charge energy pulled from grid
	ThroughputMWh     float64 // EnergyFromGridMWh + EnergyToGridMWh
	SOCStart          float64
	SOCEnd            float64
	PNL               float64 // $ for this interval (incl degradation)
}

// ClipDispatch enforces the power limit, without applying SOC constraints.
func (b *Battery) ClipDispatch(d Dispatch) Dispatch {
	p := d.PowerMW
	if p > b.Params.PowerCapacityMW {
		p = b.Params.PowerCapacityMW
	}
	if p < -b.Params.PowerCapacityMW {
		p = -b.Params.PowerCapacityMW
	}
	return Dispatch{PowerMW: p}
}

// ApplyDispatch applies a dispatch for a single interval, enforcing:
// - power capacity
// - SOC bounds (by clipping the requested power)
//
// lmp is $/MWh for the interval.
// durationHours is the interval length in hours.
func (b *Battery) ApplyDispatch(lmp float64, d Dispatch, durationHours float64) (IntervalResult, error) {
	next, result, err := SimulateInterval(b.Params, b.State, lmp, d, durationHours)
	if err != nil {
		return IntervalResult{}, err
	}
	b.State = next
	return result, nil
}

// SimulateInterval evaluates a grid-side dispatch without mutating its inputs.
// On failure it returns the original state and an empty interval result.
func SimulateInterval(params BatteryParams, state BatteryState, lmp float64, dispatch Dispatch, durationHours float64) (BatteryState, IntervalResult, error) {
	battery := Battery{Params: params, State: state}
	if err := battery.Validate(); err != nil {
		return state, IntervalResult{}, err
	}
	if !finiteBatteryValue(lmp) {
		return state, IntervalResult{}, errors.New("lmp must be finite")
	}
	if !finiteBatteryValue(dispatch.PowerMW) {
		return state, IntervalResult{}, errors.New("dispatch PowerMW must be finite")
	}
	if !finiteBatteryValue(durationHours) || durationHours <= 0 {
		return state, IntervalResult{}, errors.New("durationHours must be finite and > 0")
	}

	power := battery.ClipDispatch(dispatch).PowerMW
	result := IntervalResult{SOCStart: state.SOC}
	storedDelta := 0.0
	if power < 0 {
		headroom := (params.MaxSOC - state.SOC) * params.EnergyCapacityMWh
		result.EnergyFromGridMWh = math.Min(-power*durationHours, headroom/params.ChargeEfficiency)
		storedDelta = result.EnergyFromGridMWh * params.ChargeEfficiency
	} else if power > 0 {
		available := (state.SOC - params.MinSOC) * params.EnergyCapacityMWh
		result.EnergyToGridMWh = math.Min(power*durationHours, available*params.DischargeEfficiency)
		storedDelta = -result.EnergyToGridMWh / params.DischargeEfficiency
	}

	next := BatteryState{SOC: math.Max(params.MinSOC, math.Min(params.MaxSOC, state.SOC+storedDelta/params.EnergyCapacityMWh))}
	result.SOCEnd = next.SOC
	result.PowerMW = (result.EnergyToGridMWh - result.EnergyFromGridMWh) / durationHours
	result.ThroughputMWh = result.EnergyFromGridMWh + result.EnergyToGridMWh
	result.PNL = battery.CalculateIntervalPnL(lmp, result.EnergyFromGridMWh, result.EnergyToGridMWh)
	if !finiteBatteryValue(storedDelta) || !finiteBatteryValue(result.ThroughputMWh) || !finiteBatteryValue(result.PowerMW) || !finiteBatteryValue(result.PNL) {
		return state, IntervalResult{}, errors.New("battery interval exceeds finite numeric range")
	}
	actualStoredDelta := (next.SOC - state.SOC) * params.EnergyCapacityMWh
	tolerance := math.Max(1e-9, 1e-9*math.Max(math.Abs(actualStoredDelta), math.Abs(storedDelta)))
	if math.Abs(actualStoredDelta-storedDelta) > tolerance {
		return state, IntervalResult{}, errors.New("battery interval cannot represent conserved energy within numeric tolerance")
	}
	return next, result, nil
}

// CalculateIntervalPnL computes interval PnL given the *grid-side* energies.
// - energyFromGridMWh: MWh purchased to charge (cost)
// - energyToGridMWh: MWh sold when discharging (revenue)
func (b *Battery) CalculateIntervalPnL(lmp float64, energyFromGridMWh float64, energyToGridMWh float64) float64 {
	revenue := lmp * energyToGridMWh
	cost := lmp * energyFromGridMWh
	degradation := b.Params.DegradationCostPerMWh * (energyFromGridMWh + energyToGridMWh)
	return revenue - cost - degradation
}

func finiteBatteryValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
