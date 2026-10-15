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

	"battery-backtest/internal/analysis"
	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
	"battery-backtest/internal/model"
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
	if len(args) == 0 {
		return fmt.Errorf("command is required; use --help")
	}
	switch args[0] {
	case "--help", "-h", "help":
		_, err := fmt.Fprintln(stdout, "usage:\n  cli backtest --data sample_data.json --config examples/config.yaml --out results/dispatch.csv [--n 0] [--timezone market]\n  cli rank --data sample_data.json\n\nBacktest writes a grid-side dispatch CSV. Rank reports approximate foresight cash flow per location.")
		return err
	case "backtest":
		return runBacktest(ctx, args[1:], stdout)
	case "rank":
		return runRank(ctx, args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q; use --help", args[0])
	}
}

func parseFlags(fs *flag.FlagSet, args []string, stdout io.Writer) error {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	err := fs.Parse(args)
	if !errors.Is(err, flag.ErrHelp) {
		return err
	}
	var defaults strings.Builder
	fs.SetOutput(&defaults)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
	if _, err := fmt.Fprintf(stdout, "usage: cli %s [options]\n%s", fs.Name(), defaults.String()); err != nil {
		return err
	}
	return flag.ErrHelp
}

func runBacktest(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backtest", flag.ContinueOnError)
	dataPath := fs.String("data", "sample_data.json", "Path to Grid Status JSON response")
	configPath := fs.String("config", "", "Required YAML configuration path")
	outPath := fs.String("out", "results/dispatch.csv", "Output CSV path")
	count := fs.Int("n", 0, "Chronological interval limit (0=all)")
	clock := fs.String("timezone", "market", "Schedule/display clock: market or UTC")
	if err := parseFlags(fs, args, stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional argument %q", fs.Arg(0))
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if *outPath == "" {
		return fmt.Errorf("--out must not be empty")
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
	if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	if err := backtest.WriteLedgerCSV(*outPath, outcome.Ledger); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	summary := outcome.Summary
	_, err = fmt.Fprintf(stdout, "Wrote %d rows to %s\nTotal PnL=$%.2f Final SOC=%.3f\nInitial SOC=%.3f Initial energy=%.6f MWh Final energy=%.6f MWh\nData coverage: %.6f hours; gaps: %.6f hours (%d)\nInitial stored energy has no acquisition charge; terminal energy has zero salvage value.\n", len(outcome.Ledger), *outPath, summary.TotalPNL, summary.FinalSOC, summary.InitialSOC, summary.InitialEnergyMWh, summary.FinalEnergyMWh, summary.CoveredDurationHours, summary.GapDurationHours, summary.GapCount)
	return err
}

func runRank(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("rank", flag.ContinueOnError)
	dataPaths := fs.String("data", "sample_data.json", "Comma-separated JSON paths or directories")
	if err := parseFlags(fs, args, stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional argument %q", fs.Arg(0))
	}
	paths := splitPaths(*dataPaths)
	if len(paths) == 0 {
		return fmt.Errorf("--data requires at least one path")
	}
	byLocation := make(map[string][]model.LMPInterval)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("data: %w", err)
		}
		files := []string{path}
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				return fmt.Errorf("data: %w", err)
			}
			files = nil
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
					files = append(files, filepath.Join(path, entry.Name()))
				}
			}
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			response, err := data.LoadGridStatusJSON(file)
			if err != nil {
				return fmt.Errorf("data: %w", err)
			}
			for location, rows := range data.GroupByLocation(response) {
				byLocation[location] = append(byLocation[location], rows...)
			}
		}
	}
	ranked, err := analysis.RankByOracleProfit(ctx, byLocation)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "%-4s %-18s %-14s %-8s %-10s %-10s %-12s\n", "rank", "location", "market", "count", "p95-p05", "min/max", "oracle$"); err != nil {
		return err
	}
	for i, potential := range ranked {
		if _, err := fmt.Fprintf(stdout, "%-4d %-18s %-14s %-8d %-10.2f %-5.1f/%-5.1f %-12.2f\n", i+1, potential.Location, potential.Market, potential.Count, potential.SpreadP95P05, potential.MinLMP, potential.MaxLMP, potential.OracleProfit); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(stdout, "Approximate foresight scores include 0.5 MWh initial inventory without acquisition cost; terminal energy has zero salvage value.")
	return err
}

func splitPaths(value string) []string {
	var out []string
	for _, path := range strings.Split(value, ",") {
		if path = strings.TrimSpace(path); path != "" {
			out = append(out, path)
		}
	}
	return out
}
