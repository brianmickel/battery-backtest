package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"battery-backtest/internal/data"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	provider, err := data.NewGridStatusClient(nil, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(run(ctx, os.Args[1:], os.Getenv("GRIDSTATUS_API_KEY"), provider, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, apiKey string, provider locationProvider, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("update-locations", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var options refreshOptions
	flags.StringVar(&options.dataset, "dataset-id", "caiso_lmp_real_time_5_min", "Grid Status dataset ID")
	flags.StringVar(&options.output, "output", "", "Output catalog (default: LOCATIONS_FILE or ./data/locations.json)")
	flags.StringVar(&options.seed, "seed", "", "Optional catalog of additional locations")
	flags.IntVar(&options.days, "days", 7, "Lookback in calendar days (1-60)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(options.dataset) == "" || options.days < 1 || options.days > 60 {
		fmt.Fprintln(stderr, "dataset-id is required, days must be 1-60, and positional arguments are not supported")
		return 2
	}
	if strings.TrimSpace(apiKey) == "" {
		fmt.Fprintln(stderr, "GRIDSTATUS_API_KEY environment variable is required")
		return 2
	}
	if options.output == "" {
		options.output = data.GetDefaultLocationsPath()
	}
	result, err := refreshCatalog(ctx, provider, apiKey, options, time.Now().UTC(), stderr)
	fmt.Fprintf(stdout, "%s: updated %d/%d locations; %d failed\n", result.status, result.updated, result.total, result.failed)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
