package strategy

import (
	"context"
	"fmt"
	"math"
	"unsafe"

	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

const (
	MaxIntervals           = 17_280
	MaxTransitions  uint64 = 100_000_000
	MaxWorkingBytes uint64 = 64 << 20
)

type WorkEstimate struct{ Transitions, WorkingBytes uint64 }

type BudgetError struct{ Dimension string }

func (e *BudgetError) Error() string { return "work budget exceeded: " + e.Dimension }

// EstimateWork bounds allocations including validation's series copy, plan,
// backpointers, grid, and both value arrays before any planner allocation.
func EstimateWork(settings config.Strategy, intervalCount int) (WorkEstimate, error) {
	if intervalCount < 0 {
		return WorkEstimate{}, fmt.Errorf("interval count must be nonnegative")
	}
	if intervalCount > MaxIntervals {
		return WorkEstimate{}, &BudgetError{Dimension: "intervals"}
	}
	if err := validateSettings(settings); err != nil {
		return WorkEstimate{}, err
	}
	n := uint64(intervalCount)
	// Reserve allocator/page rounding and object overhead for the bounded arrays.
	out := WorkEstimate{WorkingBytes: n*uint64(unsafe.Sizeof(model.LMPInterval{})) + 64<<10}
	if cfg, ok := settings.(config.OracleConfig); ok {
		states, actions := uint64(cfg.SOCSteps)+2, 2*uint64(cfg.PowerSteps)+1
		// Checked before multiplication, including zero-interval local estimates.
		if n > MaxTransitions/states/actions {
			return WorkEstimate{}, &BudgetError{Dimension: "transitions"}
		}
		out.Transitions = n * states * actions
		out.WorkingBytes += n*states*10 + n*uint64(unsafe.Sizeof(model.Dispatch{})) + states*24
	}
	if out.WorkingBytes > MaxWorkingBytes {
		return WorkEstimate{}, &BudgetError{Dimension: "working_bytes"}
	}
	return out, nil
}

// Build consumes normalized chronological rows and never mutates the battery.
func Build(ctx context.Context, settings config.Strategy, rows []model.LMPInterval, battery *model.Battery) (Strategy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := EstimateWork(settings, len(rows)); err != nil {
		return nil, err
	}
	if battery == nil {
		return nil, fmt.Errorf("battery is required")
	}
	if err := battery.Validate(); err != nil {
		return nil, fmt.Errorf("battery: %w", err)
	}
	if err := validateRows(rows); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch cfg := settings.(type) {
	case config.ScheduleConfig:
		return &scheduleStrategy{settings: cfg}, nil
	case config.OracleConfig:
		plan, err := optimizeDP(ctx, rows, battery.Params, battery.State.SOC, cfg)
		if err != nil {
			return nil, err
		}
		return &OracleStrategy{plan: plan}, nil
	default:
		return nil, fmt.Errorf("unsupported strategy settings")
	}
}

func validateRows(rows []model.LMPInterval) error {
	normalized, err := data.NormalizeSeries(rows, "market")
	if err != nil {
		return err
	}
	for i := range rows {
		if !rows[i].IntervalStartUTC.Equal(normalized[i].IntervalStartUTC) {
			return fmt.Errorf("intervals must be normalized in chronological order")
		}
	}
	return nil
}

func validateSettings(settings config.Strategy) error {
	switch cfg := settings.(type) {
	case config.OracleConfig:
		if cfg.SOCSteps > 1000 {
			return &BudgetError{Dimension: "soc_steps"}
		}
		if cfg.PowerSteps > 50 {
			return &BudgetError{Dimension: "power_steps"}
		}
		if cfg.SOCSteps < 2 || cfg.PowerSteps < 1 {
			return fmt.Errorf("oracle requires soc_steps >= 2 and power_steps >= 1")
		}
	case config.ScheduleConfig:
		for _, minute := range []int{cfg.ChargeStartMinute, cfg.ChargeEndMinute, cfg.DischargeStartMinute, cfg.DischargeEndMinute} {
			if minute < 0 || minute >= 1440 {
				return fmt.Errorf("schedule minutes must be in [0, 1440)")
			}
		}
		for _, power := range []float64{cfg.ChargePowerMW, cfg.DischargePowerMW} {
			if math.IsNaN(power) || math.IsInf(power, 0) || power < 0 {
				return fmt.Errorf("schedule power must be finite and nonnegative")
			}
		}
		if cfg.ChargePowerMW > 0 && cfg.DischargePowerMW > 0 {
			for minute := 0; minute < 1440; minute++ {
				if inWindow(minute, cfg.ChargeStartMinute, cfg.ChargeEndMinute) && inWindow(minute, cfg.DischargeStartMinute, cfg.DischargeEndMinute) {
					return fmt.Errorf("active schedule windows overlap")
				}
			}
		}
	default:
		return fmt.Errorf("unsupported strategy settings")
	}
	return nil
}
