package api

import (
	"battery-backtest/internal/api/models"
	"battery-backtest/internal/backtest"
)

func wireSummary(s backtest.Summary) models.BacktestSummary {
	result := models.BacktestSummary{
		TotalPNL:             s.TotalPNL,
		InitialSOC:           s.InitialSOC,
		InitialEnergyMWh:     s.InitialEnergyMWh,
		FinalSOC:             s.FinalSOC,
		FinalEnergyMWh:       s.FinalEnergyMWh,
		TotalIntervals:       s.TotalIntervals,
		Timezone:             s.Timezone,
		EnergyChargedMWh:     s.EnergyChargedMWh,
		EnergyDischargedMWh:  s.EnergyDischargedMWh,
		CoveredDurationHours: s.CoveredDurationHours,
		GapDurationHours:     s.GapDurationHours,
		GapCount:             s.GapCount,
		BacktestWindow:       models.TimeWindow{Start: s.BacktestWindow.Start, End: s.BacktestWindow.End},
	}
	for _, w := range s.ChargeWindows {
		result.ChargeWindows = append(result.ChargeWindows, models.ChargeWindow{TimeWindow: models.TimeWindow{Start: w.Start, End: w.End}, AverageCostPerMWh: w.AverageCostPerMWh, EnergyMWh: w.EnergyMWh})
	}
	for _, w := range s.DischargeWindows {
		result.DischargeWindows = append(result.DischargeWindows, models.DischargeWindow{TimeWindow: models.TimeWindow{Start: w.Start, End: w.End}, AveragePricePerMWh: w.AveragePricePerMWh, EnergyMWh: w.EnergyMWh})
	}
	return result
}
func wireLedger(rows []backtest.LedgerRow) []models.LedgerRow {
	result := make([]models.LedgerRow, len(rows))
	for i, row := range rows {
		result[i] = models.LedgerRow{
			Index:              row.Index,
			IntervalStartLocal: row.IntervalStartLocal,
			IntervalEndLocal:   row.IntervalEndLocal,
			IntervalStartUTC:   row.IntervalStartUTC,
			IntervalEndUTC:     row.IntervalEndUTC,
			Location:           row.Location,
			Market:             row.Market,
			LMP:                row.LMP,
			RequestedPowerMW:   row.RequestedPowerMW,
			PowerMW:            row.PowerMW,
			EnergyFromGridMWh:  row.EnergyFromGridMWh,
			EnergyToGridMWh:    row.EnergyToGridMWh,
			ThroughputMWh:      row.ThroughputMWh,
			SOCStart:           row.SOCStart,
			SOCEnd:             row.SOCEnd,
			PNL:                row.PNL,
			CumPNL:             row.CumPNL,
			Action:             string(row.Action),
		}
	}
	return result
}
