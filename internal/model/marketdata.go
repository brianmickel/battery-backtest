package model

import "time"

// GridStatusLMPResponse matches the JSON shape of sample_data.json.
//
// Example:
// {
//   "status_code": 200,
//   "data": [ ... ]
// }
type GridStatusLMPResponse struct {
	StatusCode int           `json:"status_code"`
	Data       []LMPInterval `json:"data"`
}

// LMPInterval represents one interval row from the Grid Status LMP dataset.
// All timestamps are provided in the JSON as RFC3339 strings (with offsets).
type LMPInterval struct {
	IntervalStartLocal time.Time `json:"interval_start_local"`
	IntervalStartUTC   time.Time `json:"interval_start_utc"`
	IntervalEndLocal   time.Time `json:"interval_end_local"`
	IntervalEndUTC     time.Time `json:"interval_end_utc"`

	Market       string `json:"market"`
	Location     string `json:"location"`
	LocationType string `json:"location_type"`

	// Prices in $/MWh.
	LMP        float64 `json:"lmp"`
	Energy     float64 `json:"energy"`
	Congestion float64 `json:"congestion"`
	Loss       float64 `json:"loss"`
	GHG        float64 `json:"ghg"`
}

func (i LMPInterval) Duration() time.Duration {
	// Prefer UTC fields because they're unambiguous and consistent.
	if !i.IntervalEndUTC.IsZero() && !i.IntervalStartUTC.IsZero() {
		return i.IntervalEndUTC.Sub(i.IntervalStartUTC)
	}
	return i.IntervalEndLocal.Sub(i.IntervalStartLocal)
}

func (i LMPInterval) DurationHours() float64 {
	return i.Duration().Hours()
}

