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
	EnergyCapacityMWh      float64
	PowerCapacityMW        float64
	ChargeEfficiency       float64
	DischargeEfficiency    float64
	MinSOC                 float64
	MaxSOC                 float64
	DegradationCostPerMWh  float64
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
		State: BatteryState{SOC: initialSOC},
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Battery) Validate() error {
	p := b.Params
	if p.EnergyCapacityMWh <= 0 {
		return errors.New("EnergyCapacityMWh must be > 0")
	}
	if p.PowerCapacityMW <= 0 {
		return errors.New("PowerCapacityMW must be > 0")
	}
	if p.ChargeEfficiency <= 0 || p.ChargeEfficiency > 1 {
		return errors.New("ChargeEfficiency must be in (0, 1]")
	}
	if p.DischargeEfficiency <= 0 || p.DischargeEfficiency > 1 {
		return errors.New("DischargeEfficiency must be in (0, 1]")
	}
	if p.MinSOC < 0 || p.MinSOC > 1 || p.MaxSOC < 0 || p.MaxSOC > 1 || p.MinSOC > p.MaxSOC {
		return errors.New("MinSOC/MaxSOC must satisfy 0<=MinSOC<=MaxSOC<=1")
	}
	if b.State.SOC < p.MinSOC || b.State.SOC > p.MaxSOC {
		return errors.New("initial SOC must be within [MinSOC, MaxSOC]")
	}
	if p.DegradationCostPerMWh < 0 {
		return errors.New("DegradationCostPerMWh must be >= 0")
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
	PowerMW        float64 // realized power (may be clipped)
	EnergyToGridMWh float64 // discharge energy delivered to grid
	EnergyFromGridMWh float64 // charge energy pulled from grid
	ThroughputMWh  float64 // EnergyFromGridMWh + EnergyToGridMWh
	SOCStart       float64
	SOCEnd         float64
	PNL            float64 // $ for this interval (incl degradation)
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
	if durationHours <= 0 {
		return IntervalResult{}, errors.New("durationHours must be > 0")
	}

	d = b.ClipDispatch(d)
	p := d.PowerMW

	res := IntervalResult{
		SOCStart: b.State.SOC,
	}

	// SOC constraints determine the max feasible charge/discharge for the interval.
	maxChargeMWhGrid := b.maxChargeEnergyFromGridMWh(durationHours)
	maxDischargeMWhGrid := b.maxDischargeEnergyToGridMWh(durationHours)

	// Convert requested power to requested grid-energy (before/after efficiency).
	if p < 0 {
		// Charging: power magnitude is MW from grid.
		reqFromGridMWh := math.Abs(p) * durationHours
		if reqFromGridMWh > maxChargeMWhGrid {
			reqFromGridMWh = maxChargeMWhGrid
			p = -reqFromGridMWh / durationHours
		}
		// SOC increases by stored energy = fromGrid * chargeEff
		storedMWh := reqFromGridMWh * b.Params.ChargeEfficiency
		b.State.SOC = clamp01((b.State.SOC*b.Params.EnergyCapacityMWh + storedMWh) / b.Params.EnergyCapacityMWh)

		res.PowerMW = p
		res.EnergyFromGridMWh = reqFromGridMWh
		res.EnergyToGridMWh = 0
		res.ThroughputMWh = reqFromGridMWh
	} else if p > 0 {
		// Discharging: power is MW delivered to grid.
		reqToGridMWh := p * durationHours
		if reqToGridMWh > maxDischargeMWhGrid {
			reqToGridMWh = maxDischargeMWhGrid
			p = reqToGridMWh / durationHours
		}
		// SOC decreases by withdrawn energy = toGrid / dischargeEff
		withdrawnMWh := reqToGridMWh / b.Params.DischargeEfficiency
		b.State.SOC = clamp01((b.State.SOC*b.Params.EnergyCapacityMWh - withdrawnMWh) / b.Params.EnergyCapacityMWh)

		res.PowerMW = p
		res.EnergyFromGridMWh = 0
		res.EnergyToGridMWh = reqToGridMWh
		res.ThroughputMWh = reqToGridMWh
	} else {
		res.PowerMW = 0
	}

	res.SOCEnd = b.State.SOC
	res.PNL = b.CalculateIntervalPnL(lmp, res.EnergyFromGridMWh, res.EnergyToGridMWh)
	return res, nil
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

func (b *Battery) maxChargeEnergyFromGridMWh(durationHours float64) float64 {
	// Max additional stored energy before hitting MaxSOC.
	storableMWh := (b.Params.MaxSOC - b.State.SOC) * b.Params.EnergyCapacityMWh
	if storableMWh <= 0 {
		return 0
	}
	// Grid energy required = stored / eff.
	limitBySOC := storableMWh / b.Params.ChargeEfficiency
	limitByPower := b.Params.PowerCapacityMW * durationHours
	return math.Max(0, math.Min(limitBySOC, limitByPower))
}

func (b *Battery) maxDischargeEnergyToGridMWh(durationHours float64) float64 {
	// Max withdrawable stored energy before hitting MinSOC.
	withdrawableMWh := (b.State.SOC - b.Params.MinSOC) * b.Params.EnergyCapacityMWh
	if withdrawableMWh <= 0 {
		return 0
	}
	// Grid energy delivered = withdrawn * eff.
	limitBySOC := withdrawableMWh * b.Params.DischargeEfficiency
	limitByPower := b.Params.PowerCapacityMW * durationHours
	return math.Max(0, math.Min(limitBySOC, limitByPower))
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

