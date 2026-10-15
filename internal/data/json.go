package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"battery-backtest/internal/model"
)

var intervalTimestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$`)

func LoadGridStatusJSON(path string) (*model.GridStatusLMPResponse, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return decodeGridStatusResponse(file)
}

// DecodeGridStatusJSON decodes every supplied row without sorting or filtering.
// LMP and UTC timestamps are required. Local timestamps may be omitted for UTC
// input; NormalizeSeries requires them when the selected clock is market.
func DecodeGridStatusJSON(reader io.Reader) ([]model.LMPInterval, error) {
	resp, err := decodeGridStatusResponse(reader)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func decodeGridStatusResponse(reader io.Reader) (*model.GridStatusLMPResponse, error) {
	var raw struct {
		StatusCode int             `json:"status_code"`
		Data       json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("response: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("response: expected one JSON value")
		}
		return nil, fmt.Errorf("response: %w", err)
	}
	if len(raw.Data) == 0 || bytes.Equal(bytes.TrimSpace(raw.Data), []byte("null")) {
		return nil, &ValidationError{Field: "intervals", Message: "required data array is missing or null"}
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw.Data, &rows); err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}
	resp := &model.GridStatusLMPResponse{
		StatusCode: raw.StatusCode,
		Data:       make([]model.LMPInterval, 0, len(rows)),
	}
	for index, rawRow := range rows {
		row, err := decodeInterval(rawRow)
		if err != nil {
			return nil, withIntervalIndex(index, err)
		}
		resp.Data = append(resp.Data, row)
	}
	return resp, nil
}

func decodeInterval(raw json.RawMessage) (model.LMPInterval, error) {
	var fields map[string]json.RawMessage
	var row model.LMPInterval
	if err := json.Unmarshal(raw, &fields); err != nil {
		return row, fmt.Errorf("row: expected an object: %w", err)
	}
	if fields == nil {
		return row, &ValidationError{Message: "expected an object"}
	}
	for _, field := range []struct {
		name     string
		target   any
		required bool
	}{
		{"interval_start_utc", &row.IntervalStartUTC, true},
		{"interval_end_utc", &row.IntervalEndUTC, true},
		{"interval_start_local", &row.IntervalStartLocal, false},
		{"interval_end_local", &row.IntervalEndLocal, false},
		{"lmp", &row.LMP, true},
		{"energy", &row.Energy, false},
		{"congestion", &row.Congestion, false},
		{"loss", &row.Loss, false},
		{"ghg", &row.GHG, false},
		{"market", &row.Market, false},
		{"location", &row.Location, false},
		{"location_type", &row.LocationType, false},
	} {
		value, present := fields[field.name]
		if !present {
			if field.required {
				return row, &ValidationError{Field: field.name, Message: "required field is missing"}
			}
			continue
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return row, &ValidationError{Field: field.name, Message: "must not be null"}
		}
		if timestamp, ok := field.target.(*time.Time); ok {
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return row, fmt.Errorf("%s: %w", field.name, err)
			}
			parsed, err := time.ParseInLocation(time.RFC3339, text, time.UTC)
			if err != nil || parsed.IsZero() || !intervalTimestampPattern.MatchString(text) {
				return row, &ValidationError{Field: field.name, Message: "must be a nonzero RFC3339 timestamp with an offset"}
			}
			*timestamp = parsed
			continue
		}
		if err := json.Unmarshal(value, field.target); err != nil {
			if _, price := field.target.(*float64); price && (value[0] == '-' || (value[0] >= '0' && value[0] <= '9')) {
				return row, &ValidationError{Field: field.name, Message: "must be finite"}
			}
			return row, fmt.Errorf("%s: %w", field.name, err)
		}
	}
	return row, nil
}

// GroupByLocation splits a response into location-keyed slices.
func GroupByLocation(resp *model.GridStatusLMPResponse) map[string][]model.LMPInterval {
	out := map[string][]model.LMPInterval{}
	if resp == nil {
		return out
	}
	for _, it := range resp.Data {
		out[it.Location] = append(out[it.Location], it)
	}
	return out
}
