package model

// Action is a human-friendly operating mode for a timestep.
// Keep these values stable; they are intended for CSV output.
type Action string

const (
	ActionCharging    Action = "CHARGING"
	ActionIdle        Action = "IDLE"
	ActionDischarging Action = "DISCHARGING"
)

func ActionFromPowerMW(powerMW float64) Action {
	switch {
	case powerMW < 0:
		return ActionCharging
	case powerMW > 0:
		return ActionDischarging
	default:
		return ActionIdle
	}
}

