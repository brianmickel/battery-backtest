package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"battery-backtest/internal/config"
	"battery-backtest/internal/backtest"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

// Demo:
// - Load Grid Status JSON response from sample_data.json
// - Instantiate a battery model
// - Run a strategy for a few intervals to show how models fit together
func main() {
	dataPath := flag.String("data", "sample_data.json", "Path to Grid Status JSON (sample_data.json)")
	cfgPath := flag.String("config", "", "Path to YAML config (optional)")
	n := flag.Int("n", 12, "Number of intervals to simulate")
	outCSV := flag.String("out", "", "Optional path to write ledger CSV (e.g. results/dispatch.csv)")
	flag.Parse()

	raw, err := os.ReadFile(*dataPath)
	if err != nil {
		panic(err)
	}

	var resp model.GridStatusLMPResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		panic(err)
	}
	if len(resp.Data) == 0 {
		panic("no data in JSON")
	}

	// Defaults (can be overridden via --config).
	params := model.BatteryParams{
		EnergyCapacityMWh:     100,
		PowerCapacityMW:       50,
		ChargeEfficiency:      0.95,
		DischargeEfficiency:   0.95,
		MinSOC:                0.10,
		MaxSOC:                0.90,
		DegradationCostPerMWh: 2.0,
	}
	initialSOC := 0.50

	// Strategy defaults.
	var strat strategy.Strategy = &strategy.ScheduleStrategy{Params: strategy.ScheduleParams{
		ChargeStart:      "10:00",
		ChargeEnd:        "17:00",
		DischargeStart:   "17:00",
		DischargeEnd:     "20:00",
		ChargePowerMW:    50.0,
		DischargePowerMW: 50.0,
	}}

	if *cfgPath != "" {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			panic(err)
		}
		params = cfg.Battery.ToModelParams()
		initialSOC = cfg.Battery.InitialSOC

		switch cfg.Strategy.Name {
		case "schedule":
			chargeStart := "10:00"
			chargeEnd := "17:00"
			dischargeStart := "17:00"
			dischargeEnd := "20:00"
			chargePowerMW := 50.0
			dischargePowerMW := 50.0
			if v, ok := cfg.Strategy.Params["charge_start"].(string); ok && v != "" {
				chargeStart = v
			}
			if v, ok := cfg.Strategy.Params["charge_end"].(string); ok && v != "" {
				chargeEnd = v
			}
			if v, ok := cfg.Strategy.Params["discharge_start"].(string); ok && v != "" {
				dischargeStart = v
			}
			if v, ok := cfg.Strategy.Params["discharge_end"].(string); ok && v != "" {
				dischargeEnd = v
			}
			if v, ok := getNumber(cfg.Strategy.Params, "charge_power_mw"); ok {
				chargePowerMW = v
			}
			if v, ok := getNumber(cfg.Strategy.Params, "discharge_power_mw"); ok {
				dischargePowerMW = v
			}
			strat = &strategy.ScheduleStrategy{Params: strategy.ScheduleParams{
				ChargeStart:      chargeStart,
				ChargeEnd:        chargeEnd,
				DischargeStart:   dischargeStart,
				DischargeEnd:     dischargeEnd,
				ChargePowerMW:    chargePowerMW,
				DischargePowerMW: dischargePowerMW,
			}}
		case "oracle":
			// We'll force actual backtest start SOC to MinSOC below; keep oracle consistent with that.
			orc, err := strategy.NewOracleStrategy(resp.Data, params, params.MinSOC, strategy.OracleParams{
				SocSteps:   200,
				PowerSteps: 10,
			})
			if err != nil {
				panic(err)
			}
			strat = orc
		default:
			panic(fmt.Errorf("unsupported strategy in demo: %q", cfg.Strategy.Name))
		}
	}

	batt, err := model.NewBattery(params, initialSOC)
	if err != nil {
		panic(err)
	}
	// Start each backtest at min SOC to avoid "free" starting inventory.
	batt.State.SOC = batt.Params.MinSOC

	intervals := resp.Data
	if *n < len(intervals) {
		intervals = intervals[:*n]
	}

	engine := backtest.New()
	result, err := engine.Run(intervals, batt, strat)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Loaded %d intervals for %s (%s)\n", len(intervals), intervals[0].Location, intervals[0].Market)
	fmt.Printf("Strategy=%s\n", strat.Name())
	fmt.Printf("Starting SOC=%.3f\n\n", result.Ledger[0].SOCStart)

	for i := 0; i < min(12, len(result.Ledger)); i++ {
		r := result.Ledger[i]
		fmt.Printf(
			"%s lmp=%7.3f  action=%-11s  req=%7.2f  p=%7.2f  soc=%.3f→%.3f  pnl=%8.2f  cum=%8.2f\n",
			r.IntervalStartLocal.Format("2006-01-02 15:04"),
			r.LMP,
			string(r.Action),
			r.RequestedPowerMW,
			r.PowerMW,
			r.SOCStart,
			r.SOCEnd,
			r.PNL,
			r.CumPNL,
		)
	}

	if *outCSV != "" {
		if err := backtest.WriteLedgerCSV(*outCSV, result.Ledger); err != nil {
			panic(err)
		}
		fmt.Printf("\nWrote CSV: %s\n", *outCSV)
	}

	fmt.Printf("\nDone. Final SOC=%.3f  Total PnL=$%.2f\n", result.FinalSOC, result.TotalPNL)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getNumber(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

