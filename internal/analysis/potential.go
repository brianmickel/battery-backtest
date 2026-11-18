package analysis

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

type ArbitragePotential struct {
	Location     string
	Market       string
	StartUTC     time.Time
	EndUTC       time.Time
	Count        int
	MinLMP       float64
	MaxLMP       float64
	MeanLMP      float64
	P05LMP       float64
	P95LMP       float64
	SpreadP95P05 float64
	// OracleProfit is approximate foresight cash flow for the canonical battery.
	// Its initial 0.5 MWh has no acquisition cost; terminal energy has no salvage value.
	OracleProfit float64
}

const CanonicalInitialSOC = 0.5

func CanonicalBatteryParams() model.BatteryParams {
	return model.BatteryParams{EnergyCapacityMWh: 1, PowerCapacityMW: 1,
		ChargeEfficiency: 1, DischargeEfficiency: 1, MinSOC: 0, MaxSOC: 1}
}

func CanonicalStrategyConfig() config.OracleConfig {
	return config.OracleConfig{SOCSteps: 200, PowerSteps: 1}
}

// ComputePotential validates one series and replays the canonical full-horizon
// plan with each row's duration. Corrected scores can differ from legacy ranking.
func ComputePotential(ctx context.Context, rows []model.LMPInterval) (ArbitragePotential, error) {
	if err := ctx.Err(); err != nil {
		return ArbitragePotential{}, err
	}
	if _, err := strategy.EstimateWork(CanonicalStrategyConfig(), len(rows)); err != nil {
		return ArbitragePotential{}, err
	}
	rows, err := data.NormalizeSeries(rows, "market")
	if err != nil {
		return ArbitragePotential{}, err
	}
	battery, err := model.NewBattery(CanonicalBatteryParams(), CanonicalInitialSOC)
	if err != nil {
		return ArbitragePotential{}, err
	}
	plan, err := strategy.Build(ctx, CanonicalStrategyConfig(), rows, battery)
	if err != nil {
		return ArbitragePotential{}, err
	}
	p := ArbitragePotential{Location: rows[0].Location, Market: rows[0].Market,
		StartUTC: rows[0].IntervalStartUTC, EndUTC: rows[len(rows)-1].IntervalEndUTC,
		Count: len(rows), MinLMP: math.Inf(1), MaxLMP: math.Inf(-1)}
	values := make([]float64, len(rows))
	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return ArbitragePotential{}, err
		}
		result, err := battery.ApplyDispatch(row.LMP, plan.Decide(strategy.Context{Index: i, Interval: row, Battery: battery}), row.DurationHours())
		if err != nil {
			return ArbitragePotential{}, fmt.Errorf("ranking interval %d: %w", i, err)
		}
		p.OracleProfit += result.PNL
		p.MeanLMP += row.LMP / float64(len(rows))
		values[i] = row.LMP
		p.MinLMP = math.Min(p.MinLMP, row.LMP)
		p.MaxLMP = math.Max(p.MaxLMP, row.LMP)
	}
	if math.IsNaN(p.OracleProfit) || math.IsInf(p.OracleProfit, 0) || math.IsInf(p.MeanLMP, 0) {
		return ArbitragePotential{}, fmt.Errorf("ranking values exceed finite numeric range")
	}
	sort.Float64s(values)
	p.P05LMP = percentileSorted(values, 0.05)
	p.P95LMP = percentileSorted(values, 0.95)
	p.SpreadP95P05 = p.P95LMP - p.P05LMP
	if math.IsInf(p.SpreadP95P05, 0) {
		return ArbitragePotential{}, fmt.Errorf("ranking spread exceeds finite numeric range")
	}
	return p, nil
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
