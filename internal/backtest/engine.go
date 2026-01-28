package backtest

import (
	"fmt"

	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

type Engine struct{}

func New() *Engine { return &Engine{} }

// Run executes a backtest over a single-node interval series.
func (e *Engine) Run(intervals []model.LMPInterval, batt *model.Battery, strat strategy.Strategy) (*Result, error) {
	if batt == nil {
		return nil, fmt.Errorf("battery is nil")
	}
	if strat == nil {
		return nil, fmt.Errorf("strategy is nil")
	}
	if len(intervals) == 0 {
		return nil, fmt.Errorf("no intervals")
	}

	ledger := make([]LedgerRow, 0, len(intervals))
	cum := 0.0

	for idx, it := range intervals {
		dtH := it.DurationHours()
		req := strat.Decide(strategy.Context{
			Index:    idx,
			Interval: it,
			Battery:  batt,
		})

		res, err := batt.ApplyDispatch(it.LMP, req, dtH)
		if err != nil {
			return nil, fmt.Errorf("interval %d apply dispatch: %w", idx, err)
		}
		cum += res.PNL

		row := LedgerRow{
			Index: idx,

			IntervalStartLocal: it.IntervalStartLocal,
			IntervalEndLocal:   it.IntervalEndLocal,
			IntervalStartUTC:   it.IntervalStartUTC,
			IntervalEndUTC:     it.IntervalEndUTC,

			Location: it.Location,
			Market:   it.Market,
			LMP:      it.LMP,

			Action: model.ActionFromPowerMW(res.PowerMW),

			RequestedPowerMW: req.PowerMW,
			PowerMW:          res.PowerMW,

			EnergyFromGridMWh: res.EnergyFromGridMWh,
			EnergyToGridMWh:   res.EnergyToGridMWh,
			ThroughputMWh:     res.ThroughputMWh,

			SOCStart: res.SOCStart,
			SOCEnd:   res.SOCEnd,

			PNL:    res.PNL,
			CumPNL: cum,
		}
		ledger = append(ledger, row)
	}

	return &Result{
		Ledger:   ledger,
		TotalPNL: cum,
		FinalSOC: batt.State.SOC,
	}, nil
}

