package backtest

import (
	"encoding/csv"
	"os"
	"strconv"
	"time"
)

func WriteLedgerCSV(path string, ledger []LedgerRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"index",
		"interval_start_local",
		"interval_end_local",
		"interval_start_utc",
		"interval_end_utc",
		"location",
		"market",
		"lmp",
		"action",
		"requested_power_mw",
		"power_mw",
		"energy_from_grid_mwh",
		"energy_to_grid_mwh",
		"throughput_mwh",
		"soc_start",
		"soc_end",
		"pnl",
		"cum_pnl",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, r := range ledger {
		row := []string{
			strconv.Itoa(r.Index),
			fmtTime(r.IntervalStartLocal),
			fmtTime(r.IntervalEndLocal),
			fmtTime(r.IntervalStartUTC),
			fmtTime(r.IntervalEndUTC),
			r.Location,
			r.Market,
			fmtFloat(r.LMP),
			string(r.Action),
			fmtFloat(r.RequestedPowerMW),
			fmtFloat(r.PowerMW),
			fmtFloat(r.EnergyFromGridMWh),
			fmtFloat(r.EnergyToGridMWh),
			fmtFloat(r.ThroughputMWh),
			fmtFloat(r.SOCStart),
			fmtFloat(r.SOCEnd),
			fmtFloat(r.PNL),
			fmtFloat(r.CumPNL),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return w.Error()
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func fmtFloat(x float64) string {
	return strconv.FormatFloat(x, 'f', 6, 64)
}

