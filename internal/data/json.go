package data

import (
	"encoding/json"
	"os"

	"battery-backtest/internal/model"
)

func LoadGridStatusJSON(path string) (*model.GridStatusLMPResponse, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var resp model.GridStatusLMPResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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

