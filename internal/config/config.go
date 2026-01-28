package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"battery-backtest/internal/model"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk configuration shape (YAML).
type Config struct {
	// Optional: load battery parameters from a separate YAML (e.g. examples/batteries/*.yaml).
	// If both BatteryFile and Battery are provided, Battery overrides BatteryFile.
	BatteryFile string         `yaml:"battery_file"`
	Battery     BatteryConfig  `yaml:"battery"`
	Strategy    StrategyConfig `yaml:"strategy"`
}

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

type StrategyConfig struct {
	Name   string         `yaml:"name"`
	Params map[string]any `yaml:"params"`
}

func Load(path string) (*Config, error) {
	c, err := LoadUnchecked(path)
	if err != nil {
		return nil, err
	}
	// If initial_soc is not provided, default it to min_soc.
	// This keeps configs concise and matches our backtest behavior (start at min SOC).
	if c.Battery.InitialSOC == 0 {
		c.Battery.InitialSOC = c.Battery.MinSOC
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// LoadUnchecked loads and merges config, but does not validate it.
// Useful for debugging/printing partial configs.
func LoadUnchecked(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	// If battery_file is set, load it and merge in any explicit overrides from c.Battery.
	if c.BatteryFile != "" {
		batteryPath := c.BatteryFile
		if !filepath.IsAbs(batteryPath) {
			// Prefer interpreting relative paths as relative to the config file directory,
			// but fall back to the provided path (relative to cwd) if that doesn't exist.
			cand := filepath.Join(filepath.Dir(path), batteryPath)
			if _, err := os.Stat(cand); err == nil {
				batteryPath = cand
			}
		}
		loaded, err := loadBatteryFile(batteryPath)
		if err != nil {
			return nil, err
		}
		c.Battery = MergeBattery(loaded, c.Battery)
	}
	return &c, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Strategy.Name == "" {
		return errors.New("strategy.name is required")
	}
	// Validate battery params by constructing a model.Battery.
	params := c.Battery.ToModelParams()
	_, err := model.NewBattery(params, c.Battery.InitialSOC)
	if err != nil {
		return fmt.Errorf("battery config invalid: %w", err)
	}
	return nil
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

type batteryFileWrapper struct {
	Battery BatteryConfig `yaml:"battery"`
}

func loadBatteryFile(path string) (BatteryConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BatteryConfig{}, err
	}
	var w batteryFileWrapper
	if err := yaml.Unmarshal(raw, &w); err != nil {
		return BatteryConfig{}, err
	}
	return w.Battery, nil
}

// MergeBattery overlays non-zero fields from override onto base.
// This is used when loading a battery file and then applying overrides from the request.
func MergeBattery(base, override BatteryConfig) BatteryConfig {
	out := base
	if override.Name != "" {
		out.Name = override.Name
	}
	if override.EnergyCapacityMWh != 0 {
		out.EnergyCapacityMWh = override.EnergyCapacityMWh
	}
	if override.PowerCapacityMW != 0 {
		out.PowerCapacityMW = override.PowerCapacityMW
	}
	if override.ChargeEfficiency != 0 {
		out.ChargeEfficiency = override.ChargeEfficiency
	}
	if override.DischargeEfficiency != 0 {
		out.DischargeEfficiency = override.DischargeEfficiency
	}
	// Note: these are allowed to be 0 in theory, but our configs use non-zero values.
	if override.MinSOC != 0 {
		out.MinSOC = override.MinSOC
	}
	if override.MaxSOC != 0 {
		out.MaxSOC = override.MaxSOC
	}
	if override.InitialSOC != 0 {
		out.InitialSOC = override.InitialSOC
	}
	if override.DegradationCostPerMWh != 0 {
		out.DegradationCostPerMWh = override.DegradationCostPerMWh
	}
	return out
}
