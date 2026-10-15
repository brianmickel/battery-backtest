package config

import "battery-backtest/internal/model"

type BatteryConfig struct {
	Name                  string  `yaml:"name"`
	EnergyCapacityMWh     float64 `yaml:"energy_capacity_mwh"`
	PowerCapacityMW       float64 `yaml:"power_capacity_mw"`
	ChargeEfficiency      float64 `yaml:"charge_efficiency"`
	DischargeEfficiency   float64 `yaml:"discharge_efficiency"`
	MinSOC                float64 `yaml:"min_soc"`
	MaxSOC                float64 `yaml:"max_soc"`
	InitialSOC            float64 `yaml:"initial_soc"`
	DegradationCostPerMWh float64 `yaml:"degradation_cost_per_mwh"`
}

func (b BatteryConfig) ToModelParams() model.BatteryParams {
	return model.BatteryParams{
		EnergyCapacityMWh:     b.EnergyCapacityMWh,
		PowerCapacityMW:       b.PowerCapacityMW,
		ChargeEfficiency:      b.ChargeEfficiency,
		DischargeEfficiency:   b.DischargeEfficiency,
		MinSOC:                b.MinSOC,
		MaxSOC:                b.MaxSOC,
		DegradationCostPerMWh: b.DegradationCostPerMWh,
	}
}
