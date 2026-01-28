package strategy

import (
	"fmt"
	"math"
	"strings"

	"battery-backtest/internal/model"
)

// ScheduleParams implements a simple daily time-window strategy:
// - Charge during [ChargeStart, ChargeEnd)
// - Discharge during [DischargeStart, DischargeEnd)
// - Otherwise IDLE
//
// All times are interpreted in the dataset's interval_start_local timezone.
type ScheduleParams struct {
	ChargeStart     string  // "HH:MM"
	ChargeEnd       string  // "HH:MM" (optional; default = DischargeStart)
	DischargeStart  string  // "HH:MM"
	DischargeEnd    string  // "HH:MM" (optional; default = DischargeStart => zero-length)
	ChargePowerMW   float64 // magnitude; will be treated as charge (negative)
	DischargePowerMW float64 // magnitude; treated as discharge (positive)
}

type ScheduleStrategy struct {
	Params ScheduleParams

	initialized bool
	csMins      int
	ceMins      int
	dsMins      int
	deMins      int
}

func (s *ScheduleStrategy) Name() string { return "schedule" }

func (s *ScheduleStrategy) Decide(ctx Context) model.Dispatch {
	if !s.initialized {
		cs, err := parseHHMM(s.Params.ChargeStart)
		if err != nil {
			panic(err)
		}
		ds, err := parseHHMM(s.Params.DischargeStart)
		if err != nil {
			panic(err)
		}
		ce := ds
		if strings.TrimSpace(s.Params.ChargeEnd) != "" {
			ce, err = parseHHMM(s.Params.ChargeEnd)
			if err != nil {
				panic(err)
			}
		}
		de := ds
		if strings.TrimSpace(s.Params.DischargeEnd) != "" {
			de, err = parseHHMM(s.Params.DischargeEnd)
			if err != nil {
				panic(err)
			}
		}
		s.csMins = cs
		s.ceMins = ce
		s.dsMins = ds
		s.deMins = de

		s.initialized = true
	}

	mins := ctx.Interval.IntervalStartLocal.Hour()*60 + ctx.Interval.IntervalStartLocal.Minute()

	if inWindow(mins, s.csMins, s.ceMins) {
		return model.Dispatch{PowerMW: -math.Abs(s.Params.ChargePowerMW)}
	}
	if inWindow(mins, s.dsMins, s.deMins) {
		return model.Dispatch{PowerMW: math.Abs(s.Params.DischargePowerMW)}
	}
	return model.Dispatch{PowerMW: 0}
}

func parseHHMM(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time %q, expected HH:MM", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	return h*60 + m, nil
}

// inWindow checks whether tMins is in [start, end) on a 24h clock.
// If start == end, the window is empty (always false).
// If start < end, it's a normal same-day window.
// If start > end, it wraps across midnight.
func inWindow(tMins, start, end int) bool {
	if start == end {
		return false
	}
	if start < end {
		return tMins >= start && tMins < end
	}
	// wrap
	return tMins >= start || tMins < end
}

