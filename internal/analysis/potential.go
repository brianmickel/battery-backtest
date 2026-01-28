package analysis

import (
	"math"
	"sort"
	"time"

	"battery-backtest/internal/model"
)

// ArbitragePotential is a node-level summary you can use for ranking.
// It intentionally does not depend on a specific battery size; it includes
// both raw price stats and an "oracle" profit for a canonical 1MW/1MWh battery.
type ArbitragePotential struct {
	Location string
	Market   string

	StartUTC time.Time
	EndUTC   time.Time

	Count int

	MinLMP  float64
	MaxLMP  float64
	MeanLMP float64
	P05LMP  float64
	P95LMP  float64

	SpreadP95P05 float64

	// OracleProfit is the profit ($) from a canonical battery:
	// - 1 MW power, 1 MWh energy
	// - 100% efficiency, no degradation
	// - SOC bounds [0,1], initial SOC 0.5
	// - dispatch choices {-1, 0, +1} MW each interval
	OracleProfit float64
}

func ComputePotential(intervals []model.LMPInterval) ArbitragePotential {
	p := ArbitragePotential{}
	if len(intervals) == 0 {
		return p
	}
	p.Location = intervals[0].Location
	p.Market = intervals[0].Market
	p.Count = len(intervals)
	p.StartUTC = intervals[0].IntervalStartUTC
	p.EndUTC = intervals[len(intervals)-1].IntervalEndUTC

	sum := 0.0
	minv := math.Inf(1)
	maxv := math.Inf(-1)
	vals := make([]float64, 0, len(intervals))
	for _, it := range intervals {
		v := it.LMP
		vals = append(vals, v)
		sum += v
		if v < minv {
			minv = v
		}
		if v > maxv {
			maxv = v
		}
	}
	sort.Float64s(vals)
	p.MinLMP = minv
	p.MaxLMP = maxv
	p.MeanLMP = sum / float64(len(vals))
	p.P05LMP = percentileSorted(vals, 0.05)
	p.P95LMP = percentileSorted(vals, 0.95)
	p.SpreadP95P05 = p.P95LMP - p.P05LMP

	p.OracleProfit = oracleProfitCanonical(intervals)
	return p
}

func percentileSorted(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}
	// Linear interpolation between order stats.
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// oracleProfitCanonical computes a best-effort "upper bound" using a simple DP:
// SOC discretized into steps of dt (since P=1MW, E=1MWh).
func oracleProfitCanonical(intervals []model.LMPInterval) float64 {
	if len(intervals) == 0 {
		return 0
	}
	dt := intervals[0].DurationHours()
	if dt <= 0 {
		return 0
	}
	stepSOC := dt // with 1MW, 1MWh => dt MWh per step => dt SOC
	steps := int(math.Round(1.0 / stepSOC))
	if steps < 1 {
		steps = 1
	}
	// SOC grid: 0..steps (inclusive) maps to soc = i/steps.
	nStates := steps + 1
	negInf := -1e100
	dp := make([]float64, nStates)
	next := make([]float64, nStates)
	for i := range dp {
		dp[i] = negInf
	}
	// initial SOC 0.5 snapped to nearest state
	init := int(math.Round(0.5 * float64(steps)))
	if init < 0 {
		init = 0
	}
	if init > steps {
		init = steps
	}
	dp[init] = 0

	for _, it := range intervals {
		for i := range next {
			next[i] = negInf
		}
		price := it.LMP

		for socIdx := 0; socIdx <= steps; socIdx++ {
			if dp[socIdx] <= negInf/2 {
				continue
			}

			// Idle
			if dp[socIdx] > next[socIdx] {
				next[socIdx] = dp[socIdx]
			}

			// Charge: -1MW for dt hours => buy dt MWh, SOC increases by dt.
			if socIdx < steps {
				gain := -(price * dt) // cost
				if dp[socIdx]+gain > next[socIdx+1] {
					next[socIdx+1] = dp[socIdx] + gain
				}
			}

			// Discharge: +1MW for dt hours => sell dt MWh, SOC decreases by dt.
			if socIdx > 0 {
				gain := price * dt
				if dp[socIdx]+gain > next[socIdx-1] {
					next[socIdx-1] = dp[socIdx] + gain
				}
			}
		}
		dp, next = next, dp
	}

	best := negInf
	for _, v := range dp {
		if v > best {
			best = v
		}
	}
	if best <= negInf/2 {
		return 0
	}
	return best
}

