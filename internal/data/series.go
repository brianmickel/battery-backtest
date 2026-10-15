package data

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"battery-backtest/internal/model"
)

const maxSeriesIntervals = 17_280

// ErrIntervalLimit identifies series that exceed the bounded work budget.
var ErrIntervalLimit = errors.New("interval count limit exceeded")

// ValidationError describes invalid market data using a stable relative field path.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func withIntervalIndex(index int, err error) error {
	var validation *ValidationError
	if errors.As(err, &validation) {
		field := fmt.Sprintf("intervals[%d]", index)
		if validation.Field != "" {
			field += "." + validation.Field
		}
		return &ValidationError{Field: field, Message: validation.Message}
	}
	return fmt.Errorf("intervals[%d]: %w", index, err)
}

// NormalizeSeries returns a chronological copy of one location's market series.
// Callers must normalize the complete input before applying an interval limit.
// Gaps and variable durations are retained without inventing intervals.
func NormalizeSeries(rows []model.LMPInterval, clock string) ([]model.LMPInterval, error) {
	if clock == "" {
		clock = "market"
	}
	if clock != "market" && clock != "UTC" {
		return nil, &ValidationError{Field: "timezone", Message: "must be market or UTC"}
	}
	if len(rows) == 0 {
		return nil, &ValidationError{Field: "intervals", Message: "at least one interval is required"}
	}
	if len(rows) > maxSeriesIntervals {
		return nil, fmt.Errorf("intervals: %w (maximum %d)", ErrIntervalLimit, maxSeriesIntervals)
	}
	for index, row := range rows {
		if err := validateInterval(row, clock); err != nil {
			return nil, withIntervalIndex(index, err)
		}
		if row.Location != rows[0].Location {
			return nil, withIntervalIndex(index, &ValidationError{Field: "location", Message: "all intervals must have the same location"})
		}
		if row.Market != rows[0].Market {
			return nil, withIntervalIndex(index, &ValidationError{Field: "market", Message: "all intervals must have the same market"})
		}
	}
	result := append([]model.LMPInterval(nil), rows...)
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].IntervalStartUTC.Before(result[j].IntervalStartUTC)
	})
	for index := range result {
		row := &result[index]
		if index > 0 {
			previous := result[index-1]
			if row.IntervalStartUTC.Equal(previous.IntervalStartUTC) {
				return nil, &ValidationError{Field: "intervals", Message: "duplicate interval start at " + row.IntervalStartUTC.Format("2006-01-02T15:04:05Z07:00")}
			}
			if row.IntervalStartUTC.Before(previous.IntervalEndUTC) {
				return nil, &ValidationError{Field: "intervals", Message: "overlapping intervals at " + row.IntervalStartUTC.Format("2006-01-02T15:04:05Z07:00")}
			}
		}
		row.IntervalStartUTC = row.IntervalStartUTC.UTC()
		row.IntervalEndUTC = row.IntervalEndUTC.UTC()
		if clock == "UTC" {
			row.IntervalStartLocal = row.IntervalStartUTC
			row.IntervalEndLocal = row.IntervalEndUTC
		}
	}
	return result, nil
}

func validateInterval(row model.LMPInterval, clock string) error {
	if strings.TrimSpace(row.Location) == "" {
		return &ValidationError{Field: "location", Message: "must not be empty"}
	}
	if strings.TrimSpace(row.Market) == "" {
		return &ValidationError{Field: "market", Message: "must not be empty"}
	}
	if row.IntervalStartUTC.IsZero() {
		return &ValidationError{Field: "interval_start_utc", Message: "is required"}
	}
	if row.IntervalEndUTC.IsZero() {
		return &ValidationError{Field: "interval_end_utc", Message: "is required"}
	}
	if !row.IntervalEndUTC.After(row.IntervalStartUTC) {
		return &ValidationError{Field: "interval_end_utc", Message: "duration must be positive"}
	}
	if row.IntervalStartLocal.IsZero() != row.IntervalEndLocal.IsZero() {
		return &ValidationError{Field: "interval_start_local", Message: "local start and end must be supplied together"}
	}
	if clock == "market" && row.IntervalStartLocal.IsZero() {
		return &ValidationError{Field: "interval_start_local", Message: "local timestamps are required for market clock"}
	}
	if !row.IntervalStartLocal.IsZero() {
		if !row.IntervalStartLocal.Equal(row.IntervalStartUTC) {
			return &ValidationError{Field: "interval_start_local", Message: "must represent the same instant as interval_start_utc"}
		}
		if !row.IntervalEndLocal.Equal(row.IntervalEndUTC) {
			return &ValidationError{Field: "interval_end_local", Message: "must represent the same instant as interval_end_utc"}
		}
	}
	for _, price := range []struct {
		name  string
		value float64
	}{
		{"lmp", row.LMP},
		{"energy", row.Energy},
		{"congestion", row.Congestion},
		{"loss", row.Loss},
		{"ghg", row.GHG},
	} {
		if math.IsNaN(price.value) || math.IsInf(price.value, 0) {
			return &ValidationError{Field: price.name, Message: "must be finite"}
		}
	}
	return nil
}
