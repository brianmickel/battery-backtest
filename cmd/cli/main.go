package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"battery-backtest/internal/analysis"
	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "backtest":
		cmdBacktest(os.Args[2:])
	case "rank":
		cmdRank(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage:")
	fmt.Println("  cli backtest --data sample_data.json --config examples/config.yaml --out results/dispatch.csv")
	fmt.Println("  cli rank --data sample_data.json")
	fmt.Println("")
	fmt.Println("notes:")
	fmt.Println("  - backtest outputs CSV with action=CHARGING/IDLE/DISCHARGING per interval")
	fmt.Println("  - rank computes an 'arbitrage potential' oracle score per node")
}

func cmdBacktest(args []string) {
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	dataPath := fs.String("data", "sample_data.json", "Path to Grid Status JSON response")
	cfgPath := fs.String("config", "", "Path to YAML config")
	outPath := fs.String("out", "results/dispatch.csv", "Output CSV path")
	n := fs.Int("n", 0, "Optional: limit to first N intervals (0=all)")
	_ = fs.Parse(args)

	if *cfgPath == "" {
		fmt.Println("--config is required")
		os.Exit(2)
	}

	resp, err := data.LoadGridStatusJSON(*dataPath)
	if err != nil {
		panic(err)
	}
	intervals := resp.Data
	if *n > 0 && *n < len(intervals) {
		intervals = intervals[:*n]
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}

	batt, err := model.NewBattery(cfg.Battery.ToModelParams(), cfg.Battery.InitialSOC)
	if err != nil {
		panic(err)
	}

	// Start each backtest at min SOC to avoid "free" starting inventory.
	// This makes energy-out explainable purely by energy-in (minus losses) unless you later
	// choose to support an explicit initial_soc override.
	batt.State.SOC = batt.Params.MinSOC

	strat := buildStrategy(cfg, intervals, batt)

	engine := backtest.New()
	res, err := engine.Run(intervals, batt, strat)
	if err != nil {
		panic(err)
	}

	// ensure output dir exists
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		panic(err)
	}
	if err := backtest.WriteLedgerCSV(*outPath, res.Ledger); err != nil {
		panic(err)
	}

	fmt.Printf("Wrote %d rows to %s\n", len(res.Ledger), *outPath)
	fmt.Printf("Total PnL=$%.2f Final SOC=%.3f\n", res.TotalPNL, res.FinalSOC)
}

func cmdRank(args []string) {
	fs := flag.NewFlagSet("rank", flag.ExitOnError)
	dataPaths := fs.String("data", "sample_data.json", "Comma-separated JSON paths or a directory")
	_ = fs.Parse(args)

	paths := splitPaths(*dataPaths)
	byLoc := map[string][]model.LMPInterval{}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			panic(err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				panic(err)
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if !strings.HasSuffix(e.Name(), ".json") {
					continue
				}
				resp, err := data.LoadGridStatusJSON(filepath.Join(p, e.Name()))
				if err != nil {
					panic(err)
				}
				mergeByLoc(byLoc, data.GroupByLocation(resp))
			}
		} else {
			resp, err := data.LoadGridStatusJSON(p)
			if err != nil {
				panic(err)
			}
			mergeByLoc(byLoc, data.GroupByLocation(resp))
		}
	}

	ranked := analysis.RankByOracleProfit(byLoc)
	fmt.Printf("%-4s %-18s %-14s %-8s %-10s %-10s %-12s\n", "rank", "location", "market", "count", "p95-p05", "min/max", "oracle$")
	for i, r := range ranked {
		fmt.Printf(
			"%-4d %-18s %-14s %-8d %-10.2f %-5.1f/%-5.1f %-12.2f\n",
			i+1,
			r.Location,
			r.Market,
			r.Count,
			r.SpreadP95P05,
			r.MinLMP,
			r.MaxLMP,
			r.OracleProfit,
		)
	}
}

func buildStrategy(cfg *config.Config, intervals []model.LMPInterval, batt *model.Battery) strategy.Strategy {
	switch cfg.Strategy.Name {
	case "schedule":
		chargeStart := mustStr(cfg.Strategy.Params, "charge_start", "10:00")
		dischargeStart := mustStr(cfg.Strategy.Params, "discharge_start", "17:00")
		chargeEnd := mustStr(cfg.Strategy.Params, "charge_end", dischargeStart)
		dischargeEnd := mustStr(cfg.Strategy.Params, "discharge_end", dischargeStart) // empty window by default
		chargeMW := mustNum(cfg.Strategy.Params, "charge_power_mw", cfg.Battery.PowerCapacityMW)
		dischargeMW := mustNum(cfg.Strategy.Params, "discharge_power_mw", cfg.Battery.PowerCapacityMW)
		return &strategy.ScheduleStrategy{Params: strategy.ScheduleParams{
			ChargeStart:      chargeStart,
			ChargeEnd:        chargeEnd,
			DischargeStart:   dischargeStart,
			DischargeEnd:     dischargeEnd,
			ChargePowerMW:    chargeMW,
			DischargePowerMW: dischargeMW,
		}}
	case "oracle":
		socSteps := int(mustNum(cfg.Strategy.Params, "soc_steps", 200))
		powerSteps := int(mustNum(cfg.Strategy.Params, "power_steps", 10))
		orc, err := strategy.NewOracleStrategy(intervals, batt.Params, batt.State.SOC, strategy.OracleParams{
			SocSteps:   socSteps,
			PowerSteps: powerSteps,
		})
		if err != nil {
			panic(err)
		}
		return orc
	default:
		panic(fmt.Errorf("unsupported strategy: %q", cfg.Strategy.Name))
	}
}

func splitPaths(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mergeByLoc(dst, src map[string][]model.LMPInterval) {
	for k, v := range src {
		dst[k] = append(dst[k], v...)
	}
}

func mustNum(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key]; ok && v != nil {
		switch x := v.(type) {
		case float64:
			return x
		case int:
			return float64(x)
		}
	}
	return def
}

func mustStr(m map[string]any, key string, def string) string {
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return def
}
