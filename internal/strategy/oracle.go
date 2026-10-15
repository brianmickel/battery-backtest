package strategy

import (
	"context"
	"fmt"
	"math"
	"sort"

	"battery-backtest/internal/config"
	"battery-backtest/internal/model"
)

// OracleStrategy is an approximate foresight optimizer over one continuous
// horizon. The feasible grid restricts profit; it is not an economic upper bound.
type OracleStrategy struct{ plan []model.Dispatch }

func (s *OracleStrategy) Name() string { return "oracle" }
func (s *OracleStrategy) Decide(ctx Context) model.Dispatch {
	if ctx.Index < 0 || ctx.Index >= len(s.plan) {
		return model.Dispatch{}
	}
	return s.plan[ctx.Index]
}

func socGrid(p model.BatteryParams, initial float64, steps int) ([]float64, int) {
	grid := make([]float64, 0, steps+2)
	for i := 0; i <= steps; i++ {
		soc := p.MinSOC + float64(i)/float64(steps)*(p.MaxSOC-p.MinSOC)
		if i == steps {
			soc = p.MaxSOC
		}
		if len(grid) == 0 || soc != grid[len(grid)-1] {
			grid = append(grid, soc)
		}
	}
	index := sort.SearchFloat64s(grid, initial)
	if index == len(grid) || grid[index] != initial {
		grid = append(grid, 0)
		copy(grid[index+1:], grid[index:])
		grid[index] = initial
	}
	return grid, index
}

// projectTransition rounds reachable energy toward its source, then asks the
// shared physical transition to price only that projected energy transfer.
func projectTransition(grid []float64, source int, desiredPower float64, row model.LMPInterval, p model.BatteryParams) (int, model.IntervalResult, error) {
	state := model.BatteryState{SOC: grid[source]}
	reachable, _, err := model.SimulateInterval(p, state, row.LMP, model.Dispatch{PowerMW: desiredPower}, row.DurationHours())
	if err != nil {
		return 0, model.IntervalResult{}, err
	}
	target := sort.SearchFloat64s(grid, reachable.SOC)
	if reachable.SOC > state.SOC && (target == len(grid) || grid[target] > reachable.SOC) {
		target--
	}
	if target == source {
		_, result, err := model.SimulateInterval(p, state, row.LMP, model.Dispatch{}, row.DurationHours())
		return source, result, err
	}
	deltaMWh := (grid[target] - state.SOC) * p.EnergyCapacityMWh
	power := -deltaMWh * p.DischargeEfficiency / row.DurationHours()
	if deltaMWh > 0 {
		power = -deltaMWh / p.ChargeEfficiency / row.DurationHours()
	}
	next, result, err := model.SimulateInterval(p, state, row.LMP, model.Dispatch{PowerMW: power}, row.DurationHours())
	if err != nil {
		return 0, model.IntervalResult{}, err
	}
	if math.Abs(next.SOC-grid[target]) > 1e-12 {
		return 0, model.IntervalResult{}, fmt.Errorf("projected transition did not reach its grid state")
	}
	return target, result, nil
}

func optimizeDP(ctx context.Context, rows []model.LMPInterval, p model.BatteryParams, initial float64, cfg config.OracleConfig) ([]model.Dispatch, error) {
	grid, initialIndex := socGrid(p, initial, cfg.SOCSteps)
	n := len(grid)
	dp, next := make([]float64, n), make([]float64, n)
	targets := make([]uint16, len(rows)*n)
	powers := make([]float64, len(rows)*n)
	var candidates uint64
	// All terminal inventories have zero liquidation value. Working backward
	// breaks profit ties on the current action, including earlier idle choices.
	for t := len(rows) - 1; t >= 0; t-- {
		row := rows[t]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for i := range next {
			next[i] = math.Inf(-1)
		}
		for source := range grid {
			lastTarget := -1
			for k := -cfg.PowerSteps; k <= cfg.PowerSteps; k++ {
				candidates++
				if candidates%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				target, result, err := projectTransition(grid, source, float64(k)/float64(cfg.PowerSteps)*p.PowerCapacityMW, row, p)
				if err != nil {
					return nil, fmt.Errorf("oracle interval %d: %w", t, err)
				}
				if target == lastTarget {
					continue
				}
				lastTarget = target
				candidate := result.PNL + dp[target]
				if math.IsNaN(candidate) || math.IsInf(candidate, 1) {
					return nil, fmt.Errorf("oracle value exceeds finite numeric range")
				}
				index := t*n + source
				if candidate > next[source] || candidate == next[source] && (math.Abs(result.PowerMW) < math.Abs(powers[index]) || math.Abs(result.PowerMW) == math.Abs(powers[index]) && target < int(targets[index])) {
					next[source] = candidate
					targets[index] = uint16(target)
					powers[index] = result.PowerMW
				}
			}
		}
		dp, next = next, dp
	}
	plan := make([]model.Dispatch, len(rows))
	state := initialIndex
	for t := range rows {
		plan[t] = model.Dispatch{PowerMW: powers[t*n+state]}
		state = int(targets[t*n+state])
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return plan, nil
}
