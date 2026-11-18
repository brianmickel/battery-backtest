package strategy

import (
	"battery-backtest/internal/config"
	"battery-backtest/internal/model"
)

type scheduleStrategy struct{ settings config.ScheduleConfig }

func (s *scheduleStrategy) Name() string { return "schedule" }

func (s *scheduleStrategy) Decide(ctx Context) model.Dispatch {
	minute := ctx.Interval.IntervalStartLocal.Hour()*60 + ctx.Interval.IntervalStartLocal.Minute()
	cfg := s.settings
	if cfg.ChargePowerMW > 0 && inWindow(minute, cfg.ChargeStartMinute, cfg.ChargeEndMinute) {
		return model.Dispatch{PowerMW: -cfg.ChargePowerMW}
	}
	if cfg.DischargePowerMW > 0 && inWindow(minute, cfg.DischargeStartMinute, cfg.DischargeEndMinute) {
		return model.Dispatch{PowerMW: cfg.DischargePowerMW}
	}
	return model.Dispatch{}
}

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
