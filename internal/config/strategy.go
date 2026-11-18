package config

import "math"

const (
	defaultChargeStart    = "10:00"
	defaultDischargeStart = "17:00"
	defaultDischargeEnd   = "23:59"
	defaultSOCSteps       = 200
	defaultPowerSteps     = 10
)

type StrategyDefinition struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Parameters  []ParameterDefinition `json:"parameters"`
}

type ParameterDefinition struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	Default       any    `json:"default,omitempty"`
	DefaultSource string `json:"default_source,omitempty"`
}

// StrategyMetadata returns independent descriptions of the resolver's defaults.
func StrategyMetadata() []StrategyDefinition {
	return []StrategyDefinition{
		{Name: "schedule", Description: "Charges and discharges at scheduled interval-start times.", Parameters: []ParameterDefinition{
			{Name: "charge_start", Type: "string", Description: "Charge start (HH:MM)", Default: defaultChargeStart},
			{Name: "charge_end", Type: "string", Description: "Charge end (HH:MM)", DefaultSource: "discharge_start"},
			{Name: "discharge_start", Type: "string", Description: "Discharge start (HH:MM)", Default: defaultDischargeStart},
			{Name: "discharge_end", Type: "string", Description: "Discharge end (HH:MM)", Default: defaultDischargeEnd},
			{Name: "charge_power_mw", Type: "float", Description: "Nonnegative charge magnitude in MW; zero is idle", DefaultSource: "battery_power_capacity_mw"},
			{Name: "discharge_power_mw", Type: "float", Description: "Nonnegative discharge magnitude in MW; zero is idle", DefaultSource: "battery_power_capacity_mw"},
		}},
		{Name: "oracle", Description: "Approximate full-horizon foresight optimizer.", Parameters: []ParameterDefinition{
			{Name: "soc_steps", Type: "int", Description: "SOC discretization steps (2–1000)", Default: defaultSOCSteps},
			{Name: "power_steps", Type: "int", Description: "Power discretization steps (1–50)", Default: defaultPowerSteps},
		}},
	}
}

func validateStrategyName(name *string) error {
	if name != nil && *name != "schedule" && *name != "oracle" {
		return invalid("strategy.name", "must be schedule or oracle")
	}
	return nil
}

func compatibleParams(s StrategyOverride) error {
	if s.Name == nil {
		return nil
	}
	p := s.Params
	if *s.Name == "schedule" {
		if p.SOCSteps != nil {
			return invalid("strategy.params.soc_steps", "is not supported by schedule")
		}
		if p.PowerSteps != nil {
			return invalid("strategy.params.power_steps", "is not supported by schedule")
		}
	}
	if *s.Name == "oracle" {
		fields := []struct {
			name    string
			present bool
		}{
			{"charge_start", p.ChargeStart != nil}, {"charge_end", p.ChargeEnd != nil},
			{"discharge_start", p.DischargeStart != nil}, {"discharge_end", p.DischargeEnd != nil},
			{"charge_power_mw", p.ChargePowerMW != nil}, {"discharge_power_mw", p.DischargePowerMW != nil},
		}
		for _, field := range fields {
			if field.present {
				return invalid("strategy.params."+field.name, "is not supported by oracle")
			}
		}
	}
	return nil
}

func valueOr[T any](value *T, fallback T) T {
	if value != nil {
		return *value
	}
	return fallback
}

func resolveStrategy(s StrategyOverride, batteryPower float64) (Strategy, error) {
	if s.Name == nil {
		return nil, invalid("strategy.name", "is required")
	}
	if err := validateStrategyName(s.Name); err != nil {
		return nil, err
	}
	if err := compatibleParams(s); err != nil {
		return nil, err
	}
	p := s.Params
	if *s.Name == "oracle" {
		out := OracleConfig{SOCSteps: valueOr(p.SOCSteps, defaultSOCSteps), PowerSteps: valueOr(p.PowerSteps, defaultPowerSteps)}
		if out.SOCSteps < 2 || out.SOCSteps > 1000 {
			return nil, invalid("strategy.params.soc_steps", "must be between 2 and 1000")
		}
		if out.PowerSteps < 1 || out.PowerSteps > 50 {
			return nil, invalid("strategy.params.power_steps", "must be between 1 and 50")
		}
		return out, nil
	}
	var out ScheduleConfig
	dischargeStart := valueOr(p.DischargeStart, defaultDischargeStart)
	times := []struct {
		name, value string
		target      *int
	}{
		{"charge_start", valueOr(p.ChargeStart, defaultChargeStart), &out.ChargeStartMinute},
		{"charge_end", valueOr(p.ChargeEnd, dischargeStart), &out.ChargeEndMinute},
		{"discharge_start", dischargeStart, &out.DischargeStartMinute},
		{"discharge_end", valueOr(p.DischargeEnd, defaultDischargeEnd), &out.DischargeEndMinute},
	}
	for _, field := range times {
		minute, err := parseMinute(field.value, "strategy.params."+field.name)
		if err != nil {
			return nil, err
		}
		*field.target = minute
	}
	out.ChargePowerMW = valueOr(p.ChargePowerMW, batteryPower)
	out.DischargePowerMW = valueOr(p.DischargePowerMW, batteryPower)
	for _, field := range []struct {
		name  string
		value float64
	}{{"charge_power_mw", out.ChargePowerMW}, {"discharge_power_mw", out.DischargePowerMW}} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value < 0 {
			return nil, invalid("strategy.params."+field.name, "must be finite and nonnegative")
		}
	}
	if out.ChargePowerMW > 0 && out.DischargePowerMW > 0 && windowsOverlap(out.ChargeStartMinute, out.ChargeEndMinute, out.DischargeStartMinute, out.DischargeEndMinute) {
		return nil, invalid("strategy.params.charge_end", "active charge and discharge windows overlap")
	}
	return out, nil
}

func parseMinute(value, field string) (int, error) {
	if len(value) != 5 || value[2] != ':' || value[0] < '0' || value[0] > '2' || value[1] < '0' || value[1] > '9' || value[3] < '0' || value[3] > '5' || value[4] < '0' || value[4] > '9' {
		return 0, invalid(field, "must be HH:MM between 00:00 and 23:59")
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	if hour > 23 {
		return 0, invalid(field, "must be HH:MM between 00:00 and 23:59")
	}
	return hour*60 + int(value[3]-'0')*10 + int(value[4]-'0'), nil
}

func windowsOverlap(aStart, aEnd, bStart, bEnd int) bool {
	segments := func(start, end int) [][2]int {
		if start == end {
			return nil
		}
		if start < end {
			return [][2]int{{start, end}}
		}
		return [][2]int{{start, 1440}, {0, end}}
	}
	for _, a := range segments(aStart, aEnd) {
		for _, b := range segments(bStart, bEnd) {
			if a[0] < b[1] && b[0] < a[1] {
				return true
			}
		}
	}
	return false
}
