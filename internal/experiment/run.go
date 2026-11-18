package experiment

import (
	"context"
	"errors"

	"battery-backtest/internal/backtest"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

type Options struct {
	LimitIntervals int
	Clock          string
}
type Outcome struct {
	Ledger  []backtest.LedgerRow
	Summary backtest.Summary
}
type NamedOutcome struct {
	Name    string
	Outcome Outcome
}

func ValidateOptions(options Options) error {
	if options.LimitIntervals < 0 {
		return &ValidationError{Field: "options.limit_intervals", Message: "must be nonnegative; zero means all intervals"}
	}
	if options.LimitIntervals > strategy.MaxIntervals {
		return &strategy.BudgetError{Dimension: "intervals"}
	}
	if options.Clock != "" && options.Clock != "market" && options.Clock != "UTC" {
		return &ValidationError{Field: "data_source.timezone", Message: "must be market or UTC"}
	}
	return nil
}

func Run(ctx context.Context, prepared Prepared, rows []model.LMPInterval, options Options) (Outcome, error) {
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	if err := validatePrepared(prepared); err != nil {
		return Outcome{}, err
	}
	normalized, clock, err := normalize(rows, options)
	if err != nil {
		return Outcome{}, err
	}
	fullRows := normalized
	normalized = limit(normalized, options.LimitIntervals)
	if err := estimateRequest([]NamedPrepared{{Prepared: prepared}}, normalized, fullRows); err != nil {
		return Outcome{}, err
	}
	return execute(ctx, prepared, normalized, clock)
}

func Compare(ctx context.Context, prepared []NamedPrepared, rows []model.LMPInterval, options Options) ([]NamedOutcome, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(prepared) == 0 || len(prepared) > MaxVariations {
		return nil, &ValidationError{Field: "variations", Message: "requires 1 to 16 variations"}
	}
	names := make([]string, len(prepared))
	for i, named := range prepared {
		names[i] = named.Name
		if err := validatePrepared(named.Prepared); err != nil {
			return nil, variationError(named.Name, err)
		}
	}
	if err := validateNames(names); err != nil {
		return nil, err
	}
	normalized, clock, err := normalize(rows, options)
	if err != nil {
		return nil, err
	}
	fullRows := normalized
	normalized = limit(normalized, options.LimitIntervals)
	if err := estimateRequest(prepared, normalized, fullRows); err != nil {
		return nil, err
	}
	out := make([]NamedOutcome, 0, len(prepared))
	for _, named := range prepared {
		outcome, err := execute(ctx, named.Prepared, normalized, clock)
		if err != nil {
			return nil, variationError(named.Name, err)
		}
		out = append(out, NamedOutcome{Name: named.Name, Outcome: outcome})
	}
	return out, nil
}

func normalize(rows []model.LMPInterval, options Options) ([]model.LMPInterval, string, error) {
	if err := ValidateOptions(options); err != nil {
		return nil, "", err
	}
	clock := options.Clock
	if clock == "" {
		clock = "market"
	}
	normalized, err := data.NormalizeSeries(rows, clock)
	if errors.Is(err, data.ErrIntervalLimit) {
		return nil, "", &strategy.BudgetError{Dimension: "intervals"}
	}
	var invalid *data.ValidationError
	if errors.As(err, &invalid) {
		return nil, "", &ValidationError{Field: invalid.Field, Message: invalid.Message}
	}
	return normalized, clock, err
}

func limit(rows []model.LMPInterval, count int) []model.LMPInterval {
	if count > 0 && count < len(rows) {
		return rows[:count]
	}
	return rows
}

func execute(ctx context.Context, prepared Prepared, rows []model.LMPInterval, clock string) (Outcome, error) {
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	cfg := prepared.resolved
	battery, err := model.NewBattery(cfg.Battery.ToModelParams(), cfg.Battery.InitialSOC)
	if err != nil {
		return Outcome{}, err
	}
	plan, err := strategy.Build(ctx, cfg.Strategy, rows, battery)
	if err != nil {
		return Outcome{}, err
	}
	result, err := backtest.New().Run(ctx, rows, battery, plan)
	if err != nil {
		return Outcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	summary, err := backtest.Summarize(*result, battery.Params, clock)
	var invalid *backtest.SummaryValidationError
	if errors.As(err, &invalid) {
		return Outcome{}, &ValidationError{Field: invalid.Field, Message: invalid.Message}
	}
	if err != nil {
		return Outcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	return Outcome{Ledger: result.Ledger, Summary: summary}, nil
}
