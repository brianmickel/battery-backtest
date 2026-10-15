package backtest

import (
	"fmt"
	"math"
	"time"

	"battery-backtest/internal/model"
)

type TimeWindow struct {
	Start time.Time
	End   time.Time
}

type ChargeWindow struct {
	TimeWindow
	AverageCostPerMWh float64
	EnergyMWh         float64
}

type DischargeWindow struct {
	TimeWindow
	AveragePricePerMWh float64
	EnergyMWh          float64
}

type Summary struct {
	TotalPNL             float64
	InitialSOC           float64
	InitialEnergyMWh     float64
	FinalSOC             float64
	FinalEnergyMWh       float64
	EnergyChargedMWh     float64
	EnergyDischargedMWh  float64
	CoveredDurationHours float64
	GapDurationHours     float64
	TotalIntervals       int
	GapCount             int
	Timezone             string
	BacktestWindow       TimeWindow
	ChargeWindows        []ChargeWindow
	DischargeWindows     []DischargeWindow
}

// Summarize reads the chronological physical ledger without replaying dispatch.
// The selected clock controls presentation and civil-day window boundaries only.
func Summarize(result Result, params model.BatteryParams, clock string) (Summary, error) {
	if clock != "market" && clock != "UTC" {
		return Summary{}, fmt.Errorf("summary clock must be market or UTC")
	}
	if len(result.Ledger) == 0 {
		return Summary{}, fmt.Errorf("summary requires a nonempty ledger")
	}
	if !finiteSummaryValue(params.EnergyCapacityMWh) || params.EnergyCapacityMWh <= 0 {
		return Summary{}, fmt.Errorf("summary energy capacity must be finite and positive")
	}

	summary := Summary{Timezone: clock, TotalIntervals: len(result.Ledger)}
	var active summaryWindow
	var covered, gaps time.Duration
	for i, row := range result.Ledger {
		if err := validateSummaryRow(row, clock); err != nil {
			return Summary{}, fmt.Errorf("summary ledger row %d: %w", i, err)
		}
		if i > 0 {
			gap := row.IntervalStartUTC.Sub(result.Ledger[i-1].IntervalEndUTC)
			if gap < 0 {
				return Summary{}, fmt.Errorf("summary ledger row %d: intervals must be ordered and nonoverlapping", i)
			}
			if gap > 0 {
				summary.GapCount++
				gaps += gap
			}
		}
		duration := row.IntervalEndUTC.Sub(row.IntervalStartUTC)
		covered += duration
		start, end := row.IntervalStartLocal, row.IntervalEndLocal
		if clock == "UTC" {
			start, end = row.IntervalStartUTC.UTC(), row.IntervalEndUTC.UTC()
		}
		if i == 0 {
			summary.InitialSOC = row.SOCStart
			summary.InitialEnergyMWh = row.SOCStart * params.EnergyCapacityMWh
			summary.BacktestWindow.Start = start
		}
		summary.FinalSOC = row.SOCEnd
		summary.FinalEnergyMWh = row.SOCEnd * params.EnergyCapacityMWh
		summary.BacktestWindow.End = end
		summary.TotalPNL += row.PNL
		summary.EnergyChargedMWh += row.EnergyFromGridMWh
		summary.EnergyDischargedMWh += row.EnergyToGridMWh

		action, energy := model.ActionIdle, 0.0
		if row.EnergyFromGridMWh > 0 {
			action, energy = model.ActionCharging, row.EnergyFromGridMWh
		} else if row.EnergyToGridMWh > 0 {
			action, energy = model.ActionDischarging, row.EnergyToGridMWh
		}
		if action == model.ActionIdle {
			active.flush(&summary)
			continue
		}

		remaining := energy
		for start.Before(end) {
			pieceEnd := end
			year, month, day := start.Date()
			midnight := time.Date(year, month, day+1, 0, 0, 0, 0, start.Location())
			if midnight.Before(end) && !sameSummaryDate(start, end) {
				pieceEnd = midnight
			}
			pieceEnergy := remaining
			if pieceEnd.Before(end) {
				pieceEnergy = energy * (float64(pieceEnd.Sub(start)) / float64(duration))
			}
			if active.action != action || !active.End.Equal(start) || !sameSummaryDate(active.Start, start) {
				active.flush(&summary)
				active.action = action
				active.Start = start
			}
			active.End = pieceEnd
			active.energy += pieceEnergy
			active.value += row.LMP * pieceEnergy
			if !finiteSummaryValue(active.energy) || !finiteSummaryValue(active.value) {
				return Summary{}, fmt.Errorf("summary window exceeds finite numeric range")
			}
			remaining -= pieceEnergy
			start = pieceEnd
		}
	}
	active.flush(&summary)
	summary.CoveredDurationHours = covered.Hours()
	summary.GapDurationHours = gaps.Hours()
	if !finiteSummaryValue(summary.TotalPNL) || !finiteSummaryValue(summary.EnergyChargedMWh) || !finiteSummaryValue(summary.EnergyDischargedMWh) {
		return Summary{}, fmt.Errorf("summary totals exceed finite numeric range")
	}
	return summary, nil
}

type summaryWindow struct {
	TimeWindow
	action model.Action
	energy float64
	value  float64
}

func (window *summaryWindow) flush(summary *Summary) {
	if window.energy > 0 {
		average := window.value / window.energy
		if window.action == model.ActionCharging {
			summary.ChargeWindows = append(summary.ChargeWindows, ChargeWindow{window.TimeWindow, average, window.energy})
		} else {
			summary.DischargeWindows = append(summary.DischargeWindows, DischargeWindow{window.TimeWindow, average, window.energy})
		}
	}
	*window = summaryWindow{}
}

func sameSummaryDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func validateSummaryRow(row LedgerRow, clock string) error {
	if row.IntervalStartUTC.IsZero() || row.IntervalEndUTC.IsZero() || !row.IntervalEndUTC.After(row.IntervalStartUTC) {
		return fmt.Errorf("UTC timestamps must define a positive duration")
	}
	if clock == "market" && (!row.IntervalStartLocal.Equal(row.IntervalStartUTC) || !row.IntervalEndLocal.Equal(row.IntervalEndUTC)) {
		return fmt.Errorf("market timestamps must match UTC instants")
	}
	if !finiteSummaryValue(row.LMP) || !finiteSummaryValue(row.PNL) {
		return fmt.Errorf("price and PNL must be finite")
	}
	if !finiteSummaryValue(row.SOCStart) || !finiteSummaryValue(row.SOCEnd) || row.SOCStart < 0 || row.SOCStart > 1 || row.SOCEnd < 0 || row.SOCEnd > 1 {
		return fmt.Errorf("SOC must be finite and in [0,1]")
	}
	if !finiteSummaryValue(row.EnergyFromGridMWh) || !finiteSummaryValue(row.EnergyToGridMWh) || row.EnergyFromGridMWh < 0 || row.EnergyToGridMWh < 0 || (row.EnergyFromGridMWh > 0 && row.EnergyToGridMWh > 0) {
		return fmt.Errorf("grid energy must be finite, nonnegative, and in at most one direction")
	}
	return nil
}

func finiteSummaryValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
