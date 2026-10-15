package config

// Input preserves explicit overrides until preset selection and comparison merging finish.
type Input struct {
	BatteryFile string           `json:"battery_file,omitempty" yaml:"battery_file,omitempty"`
	Battery     BatteryOverride  `json:"battery,omitempty" yaml:"battery,omitempty"`
	Strategy    StrategyOverride `json:"strategy,omitempty" yaml:"strategy,omitempty"`
}

type BatteryOverride struct {
	Name                  *string  `json:"name,omitempty" yaml:"name,omitempty"`
	EnergyCapacityMWh     *float64 `json:"energy_capacity_mwh,omitempty" yaml:"energy_capacity_mwh,omitempty"`
	PowerCapacityMW       *float64 `json:"power_capacity_mw,omitempty" yaml:"power_capacity_mw,omitempty"`
	ChargeEfficiency      *float64 `json:"charge_efficiency,omitempty" yaml:"charge_efficiency,omitempty"`
	DischargeEfficiency   *float64 `json:"discharge_efficiency,omitempty" yaml:"discharge_efficiency,omitempty"`
	MinSOC                *float64 `json:"min_soc,omitempty" yaml:"min_soc,omitempty"`
	MaxSOC                *float64 `json:"max_soc,omitempty" yaml:"max_soc,omitempty"`
	InitialSOC            *float64 `json:"initial_soc,omitempty" yaml:"initial_soc,omitempty"`
	DegradationCostPerMWh *float64 `json:"degradation_cost_per_mwh,omitempty" yaml:"degradation_cost_per_mwh,omitempty"`
}

type StrategyOverride struct {
	Name   *string                `json:"name,omitempty" yaml:"name,omitempty"`
	Params StrategyParamsOverride `json:"params,omitempty" yaml:"params,omitempty"`
}

type StrategyParamsOverride struct {
	ChargeStart      *string  `json:"charge_start,omitempty" yaml:"charge_start,omitempty"`
	ChargeEnd        *string  `json:"charge_end,omitempty" yaml:"charge_end,omitempty"`
	DischargeStart   *string  `json:"discharge_start,omitempty" yaml:"discharge_start,omitempty"`
	DischargeEnd     *string  `json:"discharge_end,omitempty" yaml:"discharge_end,omitempty"`
	ChargePowerMW    *float64 `json:"charge_power_mw,omitempty" yaml:"charge_power_mw,omitempty"`
	DischargePowerMW *float64 `json:"discharge_power_mw,omitempty" yaml:"discharge_power_mw,omitempty"`
	SOCSteps         *int     `json:"soc_steps,omitempty" yaml:"soc_steps,omitempty"`
	PowerSteps       *int     `json:"power_steps,omitempty" yaml:"power_steps,omitempty"`
}

type Resolved struct {
	Battery  BatteryConfig
	Strategy Strategy
}

type Strategy interface {
	strategySettings()
}

type ScheduleConfig struct {
	ChargeStartMinute    int
	ChargeEndMinute      int
	DischargeStartMinute int
	DischargeEndMinute   int
	ChargePowerMW        float64
	DischargePowerMW     float64
}

func (ScheduleConfig) strategySettings() {}

type OracleConfig struct {
	SOCSteps   int
	PowerSteps int
}

func (OracleConfig) strategySettings() {}

func copyPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func overlay[T any](base, patch *T) *T {
	if patch != nil {
		return copyPointer(patch)
	}
	return copyPointer(base)
}

func mergeBatteryOverride(base, patch BatteryOverride) BatteryOverride {
	return BatteryOverride{
		Name:                  overlay(base.Name, patch.Name),
		EnergyCapacityMWh:     overlay(base.EnergyCapacityMWh, patch.EnergyCapacityMWh),
		PowerCapacityMW:       overlay(base.PowerCapacityMW, patch.PowerCapacityMW),
		ChargeEfficiency:      overlay(base.ChargeEfficiency, patch.ChargeEfficiency),
		DischargeEfficiency:   overlay(base.DischargeEfficiency, patch.DischargeEfficiency),
		MinSOC:                overlay(base.MinSOC, patch.MinSOC),
		MaxSOC:                overlay(base.MaxSOC, patch.MaxSOC),
		InitialSOC:            overlay(base.InitialSOC, patch.InitialSOC),
		DegradationCostPerMWh: overlay(base.DegradationCostPerMWh, patch.DegradationCostPerMWh),
	}
}

// MergeInput never materializes presets or defaults as explicit overrides.
func MergeInput(base, patch Input) (Input, error) {
	if err := validateStrategyName(base.Strategy.Name); err != nil {
		return Input{}, err
	}
	if err := validateStrategyName(patch.Strategy.Name); err != nil {
		return Input{}, err
	}
	out := Input{BatteryFile: base.BatteryFile, Battery: mergeBatteryOverride(base.Battery, patch.Battery)}
	if patch.BatteryFile != "" {
		out.BatteryFile = patch.BatteryFile
	}
	if out.BatteryFile != "" && !validPresetID(out.BatteryFile) {
		return Input{}, invalid("battery_file", "must be an exact preset ID")
	}
	baseParams := base.Strategy.Params
	if patch.Strategy.Name != nil && (base.Strategy.Name == nil || *patch.Strategy.Name != *base.Strategy.Name) {
		baseParams = StrategyParamsOverride{}
	}
	p := patch.Strategy.Params
	out.Strategy = StrategyOverride{
		Name: overlay(base.Strategy.Name, patch.Strategy.Name),
		Params: StrategyParamsOverride{
			ChargeStart:      overlay(baseParams.ChargeStart, p.ChargeStart),
			ChargeEnd:        overlay(baseParams.ChargeEnd, p.ChargeEnd),
			DischargeStart:   overlay(baseParams.DischargeStart, p.DischargeStart),
			DischargeEnd:     overlay(baseParams.DischargeEnd, p.DischargeEnd),
			ChargePowerMW:    overlay(baseParams.ChargePowerMW, p.ChargePowerMW),
			DischargePowerMW: overlay(baseParams.DischargePowerMW, p.DischargePowerMW),
			SOCSteps:         overlay(baseParams.SOCSteps, p.SOCSteps),
			PowerSteps:       overlay(baseParams.PowerSteps, p.PowerSteps),
		},
	}
	if err := compatibleParams(out.Strategy); err != nil {
		return Input{}, err
	}
	return out, nil
}
