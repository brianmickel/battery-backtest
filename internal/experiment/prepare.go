package experiment

import (
	"errors"
	"fmt"
	"strings"

	"battery-backtest/internal/config"
	"battery-backtest/internal/strategy"
)

const MaxVariations = 16

// Prepared contains resolved values only. Every execution constructs fresh state.
type Prepared struct{ resolved config.Resolved }

type Variation struct {
	Name   string
	Config config.Input
}
type NamedPrepared struct {
	Name     string
	Prepared Prepared
}

type ValidationError struct{ Field, Variation, Message string }

func (e *ValidationError) Error() string {
	message := e.Message
	if e.Field != "" {
		message = e.Field + ": " + message
	}
	if e.Variation != "" {
		message = fmt.Sprintf("variation %q: %s", e.Variation, message)
	}
	return message
}

type VariationError struct {
	Variation string
	Err       error
}

func (e *VariationError) Error() string { return fmt.Sprintf("variation %q: %v", e.Variation, e.Err) }
func (e *VariationError) Unwrap() error { return e.Err }

func Prepare(input config.Input, presets config.PresetCatalog) (Prepared, error) {
	resolved, err := config.Resolve(input, presets)
	if err != nil {
		return Prepared{}, configurationError(err)
	}
	if _, err := strategy.EstimateWork(resolved.Strategy, 0); err != nil {
		return Prepared{}, err
	}
	return Prepared{resolved: resolved}, nil
}

// PrepareComparison resolves merged patches; the common base need not be a
// complete experiment by itself. Nothing here fetches data or builds a plan.
func PrepareComparison(base config.Input, variations []Variation, presets config.PresetCatalog) ([]NamedPrepared, error) {
	if len(variations) == 0 || len(variations) > MaxVariations {
		return nil, &ValidationError{Field: "variations", Message: "requires 1 to 16 variations"}
	}
	names := make([]string, len(variations))
	for i, variation := range variations {
		names[i] = variation.Name
	}
	if err := validateNames(names); err != nil {
		return nil, err
	}
	out := make([]NamedPrepared, 0, len(variations))
	for _, variation := range variations {
		merged, err := config.MergeInput(base, variation.Config)
		if err != nil {
			return nil, variationError(variation.Name, configurationError(err))
		}
		prepared, err := Prepare(merged, presets)
		if err != nil {
			return nil, variationError(variation.Name, err)
		}
		out = append(out, NamedPrepared{Name: variation.Name, Prepared: prepared})
	}
	return out, nil
}

func configurationError(err error) error {
	var invalid *config.ValidationError
	if errors.As(err, &invalid) {
		field := "config"
		if invalid.Field != "" {
			field += "." + invalid.Field
		}
		return &ValidationError{Field: field, Message: invalid.Message}
	}
	return err
}

func variationError(name string, err error) error {
	if name == "" {
		return err
	}
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		return &ValidationError{Field: invalid.Field, Variation: name, Message: invalid.Message}
	}
	return &VariationError{Variation: name, Err: err}
}

func validateNames(names []string) error {
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return &ValidationError{Field: "variations.name", Message: "must not be empty"}
		}
		if seen[name] {
			return &ValidationError{Field: "variations.name", Variation: name, Message: "must be unique"}
		}
		seen[name] = true
	}
	return nil
}

func validatePrepared(prepared Prepared) error {
	if prepared.resolved.Strategy == nil {
		return &ValidationError{Field: "config", Message: "must be prepared before execution"}
	}
	return nil
}
