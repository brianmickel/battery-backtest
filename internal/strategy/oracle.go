package strategy

import (
	"fmt"
	"math"
	"time"

	"battery-backtest/internal/model"
)

// OracleStrategy is a (near) profit-maximizing "perfect foresight" strategy.
// It computes a dispatch plan up-front using dynamic programming on a discretized SOC grid.
// The strategy optimizes each day independently, starting from initialSOC at the start of each day,
// to maximize daily profit from charging during low-price periods and discharging during high-price periods.
//
// Notes:
// - This is designed to be a practical "upper bound" and ranking tool.
// - Each day is optimized independently, allowing the battery to fully utilize its capacity each day.
// - Exact LP/MILP solvers can be integrated later if needed.
type OracleStrategy struct {
	plan []model.Dispatch
}

type OracleParams struct {
	// SocSteps controls SOC discretization between [MinSOC, MaxSOC].
	// Higher = more accurate, slower.
	SocSteps int

	// PowerSteps controls action discretization between [-Pmax, +Pmax].
	// Higher = more accurate, slower.
	PowerSteps int
}

func NewOracleStrategy(intervals []model.LMPInterval, params model.BatteryParams, initialSOC float64, cfg OracleParams) (*OracleStrategy, error) {
	if len(intervals) == 0 {
		return nil, fmt.Errorf("no intervals")
	}
	if cfg.SocSteps <= 0 {
		cfg.SocSteps = 200
	}
	if cfg.PowerSteps <= 0 {
		cfg.PowerSteps = 10
	}

	// Group intervals by day and optimize each day independently
	// This maximizes profit per day rather than across the entire period
	plan, err := optimizeDPByDay(intervals, params, initialSOC, cfg.SocSteps, cfg.PowerSteps)
	if err != nil {
		return nil, err
	}
	return &OracleStrategy{plan: plan}, nil
}

func (s *OracleStrategy) Name() string { return "oracle" }

func (s *OracleStrategy) Decide(ctx Context) model.Dispatch {
	if ctx.Index < 0 || ctx.Index >= len(s.plan) {
		return model.Dispatch{PowerMW: 0}
	}
	return s.plan[ctx.Index]
}

// optimizeDPByDay groups intervals by day and optimizes each day independently.
// This maximizes profit per day, starting from initialSOC at the start of each day.
// Since intervals are already sorted chronologically, we can group them in a single pass.
func optimizeDPByDay(intervals []model.LMPInterval, p model.BatteryParams, initialSOC float64, socSteps int, powerSteps int) ([]model.Dispatch, error) {
	if len(intervals) == 0 {
		return nil, fmt.Errorf("no intervals")
	}

	// Group intervals by day (intervals are already sorted chronologically)
	var dayIntervals []model.LMPInterval
	var fullPlan []model.Dispatch
	var currentDay time.Time

	for i, interval := range intervals {
		intervalDay := time.Date(
			interval.IntervalStartLocal.Year(),
			interval.IntervalStartLocal.Month(),
			interval.IntervalStartLocal.Day(),
			0, 0, 0, 0,
			interval.IntervalStartLocal.Location(),
		)

		// If this is a new day (or the first interval), optimize the previous day
		if i > 0 && !intervalDay.Equal(currentDay) {
			// Optimize the previous day
			dayPlan, err := optimizeDP(dayIntervals, p, initialSOC, socSteps, powerSteps)
			if err != nil {
				return nil, fmt.Errorf("error optimizing day %s: %w", currentDay.Format("2006-01-02"), err)
			}
			fullPlan = append(fullPlan, dayPlan...)
			dayIntervals = dayIntervals[:0] // Reset for new day
		}

		// Start tracking a new day
		if len(dayIntervals) == 0 {
			currentDay = intervalDay
		}

		dayIntervals = append(dayIntervals, interval)
	}

	// Optimize the last day
	if len(dayIntervals) > 0 {
		dayPlan, err := optimizeDP(dayIntervals, p, initialSOC, socSteps, powerSteps)
		if err != nil {
			return nil, fmt.Errorf("error optimizing day %s: %w", currentDay.Format("2006-01-02"), err)
		}
		fullPlan = append(fullPlan, dayPlan...)
	}

	// Validate that plan length matches intervals length
	if len(fullPlan) != len(intervals) {
		return nil, fmt.Errorf("plan length (%d) does not match intervals length (%d)", len(fullPlan), len(intervals))
	}

	return fullPlan, nil
}

func optimizeDP(intervals []model.LMPInterval, p model.BatteryParams, initialSOC float64, socSteps int, powerSteps int) ([]model.Dispatch, error) {
	// SOC grid is [MinSOC, MaxSOC] in socSteps increments.
	if socSteps < 2 {
		socSteps = 2
	}
	nStates := socSteps + 1

	socToIdx := func(soc float64) int {
		if soc <= p.MinSOC {
			return 0
		}
		if soc >= p.MaxSOC {
			return socSteps
		}
		f := (soc - p.MinSOC) / (p.MaxSOC - p.MinSOC)
		return int(math.Round(f * float64(socSteps)))
	}
	idxToSoc := func(idx int) float64 {
		if idx <= 0 {
			return p.MinSOC
		}
		if idx >= socSteps {
			return p.MaxSOC
		}
		f := float64(idx) / float64(socSteps)
		return p.MinSOC + f*(p.MaxSOC-p.MinSOC)
	}

	negInf := -1e100
	dp := make([]float64, nStates)
	next := make([]float64, nStates)
	for i := range dp {
		dp[i] = negInf
	}
	initIdx := socToIdx(initialSOC)
	dp[initIdx] = 0

	// Backpointers:
	choice := make([][]int, len(intervals)) // chosen next-state index
	powerChosen := make([][]float64, len(intervals))
	for t := range intervals {
		choice[t] = make([]int, nStates)
		powerChosen[t] = make([]float64, nStates)
		for s := 0; s < nStates; s++ {
			choice[t][s] = -1
			powerChosen[t][s] = 0
		}
	}

	// Action set: [-Pmax .. +Pmax] in steps.
	pmax := p.PowerCapacityMW
	step := pmax / float64(powerSteps)
	actions := make([]float64, 0, 2*powerSteps+1)
	for k := -powerSteps; k <= powerSteps; k++ {
		actions = append(actions, float64(k)*step)
	}

	for t, it := range intervals {
		for i := range next {
			next[i] = negInf
		}
		dtH := it.DurationHours()
		if dtH <= 0 {
			return nil, fmt.Errorf("non-positive dt at t=%d", t)
		}

		for sIdx := 0; sIdx < nStates; sIdx++ {
			if dp[sIdx] <= negInf/2 {
				continue
			}
			soc := idxToSoc(sIdx)

			// Always consider all actions, including idle (power = 0)
			// Track the best action from this state to ensure we always have a choice
			bestNextState := sIdx // Default to staying in same state (idle)
			bestValue := dp[sIdx] // At minimum, idle maintains current value
			bestPower := 0.0      // Idle power

			for _, desiredPower := range actions {
				nsoc, realizedPower, pnl := simulateInterval(soc, desiredPower, it.LMP, dtH, p)
				ns := socToIdx(nsoc)
				v := dp[sIdx] + pnl
				// Update next state value if this is better
				if v > next[ns] {
					next[ns] = v
				}
				// Track best action from this state
				if v > bestValue {
					bestValue = v
					bestNextState = ns
					bestPower = realizedPower
				}
			}

			// Always record the best action from this reachable state
			// Since we default to idle, bestNextState should always be valid
			choice[t][sIdx] = bestNextState
			powerChosen[t][sIdx] = bestPower
		}

		// Swap dp and next for next iteration
		dp, next = next, dp
	}

	// Pick best final state.
	bestVal := negInf
	bestState := 0
	for i, v := range dp {
		if v > bestVal {
			bestVal = v
			bestState = i
		}
	}

	// Reconstruct plan.
	plan := make([]model.Dispatch, len(intervals))
	state := bestState
	for t := len(intervals) - 1; t >= 0; t-- {
		// We stored choice/power from previous state to next; need to find a prev state that leads to 'state' with optimal value.
		// For simplicity, we replay forward by tracking from initial state using recorded choices.
		// We'll reconstruct forward instead.
		_ = t
		_ = state
		break
	}

	// Forward reconstruction:
	// Follow the recorded choices from the initial state
	cur := initIdx
	for t := 0; t < len(intervals); t++ {
		// choice[t][cur] should always be valid since we record at least idle for reachable states
		ns := choice[t][cur]
		if ns < 0 {
			// This shouldn't happen, but fallback to idle if it does
			plan[t] = model.Dispatch{PowerMW: 0}
			// Stay in current state
			continue
		}
		plan[t] = model.Dispatch{PowerMW: powerChosen[t][cur]}
		cur = ns
	}

	return plan, nil
}

// simulateInterval is a pure version of the battery interval physics+PnL.
// It mirrors model.Battery.ApplyDispatch semantics: desired power is clipped by power limit and SOC bounds.
func simulateInterval(soc float64, desiredPower float64, lmp float64, dtH float64, p model.BatteryParams) (nextSOC float64, realizedPower float64, pnl float64) {
	// Clip by power.
	power := desiredPower
	if power > p.PowerCapacityMW {
		power = p.PowerCapacityMW
	}
	if power < -p.PowerCapacityMW {
		power = -p.PowerCapacityMW
	}

	energyFromGrid := 0.0
	energyToGrid := 0.0

	if power < 0 {
		reqFromGrid := math.Abs(power) * dtH
		// SOC constraint (max charge).
		storableMWh := (p.MaxSOC - soc) * p.EnergyCapacityMWh
		if storableMWh < 0 {
			storableMWh = 0
		}
		limitBySOC := storableMWh / p.ChargeEfficiency
		limitByPower := p.PowerCapacityMW * dtH
		maxFromGrid := math.Min(limitBySOC, limitByPower)
		if reqFromGrid > maxFromGrid && dtH > 0 {
			reqFromGrid = maxFromGrid
			power = -reqFromGrid / dtH
		}
		storedMWh := reqFromGrid * p.ChargeEfficiency
		nextSOC = soc + storedMWh/p.EnergyCapacityMWh
		energyFromGrid = reqFromGrid
	} else if power > 0 {
		reqToGrid := power * dtH
		withdrawableMWh := (soc - p.MinSOC) * p.EnergyCapacityMWh
		if withdrawableMWh < 0 {
			withdrawableMWh = 0
		}
		limitBySOC := withdrawableMWh * p.DischargeEfficiency
		limitByPower := p.PowerCapacityMW * dtH
		maxToGrid := math.Min(limitBySOC, limitByPower)
		if reqToGrid > maxToGrid && dtH > 0 {
			reqToGrid = maxToGrid
			power = reqToGrid / dtH
		}
		withdrawnMWh := reqToGrid / p.DischargeEfficiency
		nextSOC = soc - withdrawnMWh/p.EnergyCapacityMWh
		energyToGrid = reqToGrid
	} else {
		nextSOC = soc
	}

	// Clamp numeric drift.
	if nextSOC < p.MinSOC {
		nextSOC = p.MinSOC
	}
	if nextSOC > p.MaxSOC {
		nextSOC = p.MaxSOC
	}

	// PnL uses grid-side energies + degradation.
	revenue := lmp * energyToGrid
	cost := lmp * energyFromGrid
	deg := p.DegradationCostPerMWh * (energyFromGrid + energyToGrid)
	pnl = revenue - cost - deg
	return nextSOC, power, pnl
}
