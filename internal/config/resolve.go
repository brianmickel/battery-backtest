package config

import "math"

func Resolve(input Input, presets PresetCatalog) (Resolved, error) {
	patch := input.Battery
	if input.BatteryFile != "" {
		if !validPresetID(input.BatteryFile) {
			return Resolved{}, invalid("battery_file", "must be an exact preset ID")
		}
		preset, found := presets.entries[input.BatteryFile]
		if !found {
			return Resolved{}, invalid("battery_file", "unknown preset ID")
		}
		patch = mergeBatteryOverride(preset.patch, patch)
	}
	battery, err := resolveBattery(patch)
	if err != nil {
		return Resolved{}, err
	}
	settings, err := resolveStrategy(input.Strategy, battery.PowerCapacityMW)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Battery: battery, Strategy: settings}, nil
}

func resolveBattery(p BatteryOverride) (BatteryConfig, error) {
	var b BatteryConfig
	fields := []struct {
		name     string
		source   *float64
		target   *float64
		required bool
	}{
		{"energy_capacity_mwh", p.EnergyCapacityMWh, &b.EnergyCapacityMWh, true},
		{"power_capacity_mw", p.PowerCapacityMW, &b.PowerCapacityMW, true},
		{"charge_efficiency", p.ChargeEfficiency, &b.ChargeEfficiency, true},
		{"discharge_efficiency", p.DischargeEfficiency, &b.DischargeEfficiency, true},
		{"min_soc", p.MinSOC, &b.MinSOC, true},
		{"max_soc", p.MaxSOC, &b.MaxSOC, true},
		{"initial_soc", p.InitialSOC, &b.InitialSOC, false},
		{"degradation_cost_per_mwh", p.DegradationCostPerMWh, &b.DegradationCostPerMWh, false},
	}
	for _, field := range fields {
		if field.source == nil {
			if field.required {
				return b, invalid("battery."+field.name, "is required without a preset value")
			}
			continue
		}
		if math.IsNaN(*field.source) || math.IsInf(*field.source, 0) {
			return b, invalid("battery."+field.name, "must be finite")
		}
		*field.target = *field.source
	}
	if p.Name != nil {
		b.Name = *p.Name
	}
	if p.InitialSOC == nil {
		b.InitialSOC = b.MinSOC
	}
	if b.EnergyCapacityMWh <= 0 {
		return b, invalid("battery.energy_capacity_mwh", "must be greater than zero")
	}
	if b.PowerCapacityMW <= 0 {
		return b, invalid("battery.power_capacity_mw", "must be greater than zero")
	}
	if b.ChargeEfficiency <= 0 || b.ChargeEfficiency > 1 {
		return b, invalid("battery.charge_efficiency", "must be in (0, 1]")
	}
	if b.DischargeEfficiency <= 0 || b.DischargeEfficiency > 1 {
		return b, invalid("battery.discharge_efficiency", "must be in (0, 1]")
	}
	if b.MinSOC < 0 || b.MinSOC > 1 {
		return b, invalid("battery.min_soc", "must be in [0, 1]")
	}
	if b.MaxSOC < 0 || b.MaxSOC > 1 || b.MaxSOC < b.MinSOC {
		return b, invalid("battery.max_soc", "must be in [min_soc, 1]")
	}
	if b.InitialSOC < b.MinSOC || b.InitialSOC > b.MaxSOC {
		return b, invalid("battery.initial_soc", "must be in [min_soc, max_soc]")
	}
	if b.DegradationCostPerMWh < 0 {
		return b, invalid("battery.degradation_cost_per_mwh", "must be nonnegative")
	}
	return b, nil
}
