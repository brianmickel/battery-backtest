package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	dataPath := fs.String("data", "sample_data.json", "Path to Grid Status JSON response")
	configPath := fs.String("config", "", "Required YAML configuration path")
	count := fs.Int("n", 12, "Chronological interval limit (0=all)")
	outPath := fs.String("out", "", "Optional output CSV path")
	clock := fs.String("timezone", "market", "Schedule/display clock: market or UTC")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			var defaults strings.Builder
			fs.SetOutput(&defaults)
			fs.PrintDefaults()
			_, err := fmt.Fprintf(stdout, "usage: demo --data sample_data.json --config examples/config.yaml [options]\n%s", defaults.String())
			return err
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional argument %q", fs.Arg(0))
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	options := experiment.Options{LimitIntervals: *count, Clock: *clock}
	if err := experiment.ValidateOptions(options); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := config.LoadFile(*configPath)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	prepared, err := experiment.Prepare(input, config.PresetCatalog{})
	if err != nil {
		return err
	}
	response, err := data.LoadGridStatusJSON(*dataPath)
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	outcome, err := experiment.Run(ctx, prepared, response.Data, options)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if *outPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
			return fmt.Errorf("output: %w", err)
		}
		if err := backtest.WriteLedgerCSV(*outPath, outcome.Ledger); err != nil {
			return fmt.Errorf("output: %w", err)
		}
	}
	summary, first := outcome.Summary, outcome.Ledger[0]
	if _, err := fmt.Fprintf(stdout, "Loaded %d intervals for %s (%s)\nStarting SOC=%.3f\n", len(outcome.Ledger), first.Location, first.Market, summary.InitialSOC); err != nil {
		return err
	}
	for i := 0; i < min(12, len(outcome.Ledger)); i++ {
		row := outcome.Ledger[i]
		if _, err := fmt.Fprintf(stdout, "%s lmp=%7.3f action=%-11s req=%7.2f p=%7.2f soc=%.3f→%.3f pnl=%8.2f cum=%8.2f\n", row.IntervalStartLocal.Format("2006-01-02 15:04:05Z07:00"), row.LMP, row.Action, row.RequestedPowerMW, row.PowerMW, row.SOCStart, row.SOCEnd, row.PNL, row.CumPNL); err != nil {
			return err
		}
	}
	if *outPath != "" {
		if _, err := fmt.Fprintf(stdout, "Wrote CSV: %s\n", *outPath); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "Final SOC=%.3f Total PnL=$%.2f\nInitial energy=%.6f MWh Final energy=%.6f MWh\nData coverage: %.6f hours; gaps: %.6f hours (%d)\nInitial stored energy has no acquisition charge; terminal energy has zero salvage value.\n", summary.FinalSOC, summary.TotalPNL, summary.InitialEnergyMWh, summary.FinalEnergyMWh, summary.CoveredDurationHours, summary.GapDurationHours, summary.GapCount)
	return err
}
