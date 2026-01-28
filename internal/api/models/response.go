package models

import "time"

// BacktestResponse represents the response from a backtest run
type BacktestResponse struct {
	ID      string           `json:"id,omitempty"`
	Status  string           `json:"status"`
	Summary BacktestSummary  `json:"summary"`
	Ledger  []LedgerRow      `json:"ledger,omitempty"`
}

// BacktestSummary contains aggregated backtest results
type BacktestSummary struct {
	TotalPNL        float64         `json:"total_pnl"`
	FinalSOC        float64         `json:"final_soc"`
	TotalIntervals  int             `json:"total_intervals"`
	BacktestWindow  TimeWindow      `json:"backtest_window"`
	EnergyChargedMWh float64        `json:"energy_charged_mwh"`
	EnergyDischargedMWh float64     `json:"energy_discharged_mwh"`
	ChargeWindows   []ChargeWindow    `json:"charge_windows,omitempty"`    // Per-day charge windows
	DischargeWindows []DischargeWindow    `json:"discharge_windows,omitempty"` // Per-day discharge windows
}

// TimeWindow represents a time range
type TimeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ChargeWindow represents a charge window with average cost
type ChargeWindow struct {
	TimeWindow
	AverageCostPerMWh float64 `json:"average_cost_per_mwh"` // Weighted average LMP during charging
	EnergyMWh          float64 `json:"energy_mwh"`           // Total energy charged in this window
}

// DischargeWindow represents a discharge window with average price
type DischargeWindow struct {
	TimeWindow
	AveragePricePerMWh float64 `json:"average_price_per_mwh"` // Weighted average LMP during discharging
	EnergyMWh          float64 `json:"energy_mwh"`             // Total energy discharged in this window
}

// LedgerRow represents one interval in the backtest ledger
type LedgerRow struct {
	Index              int       `json:"index"`
	IntervalStartLocal time.Time `json:"interval_start_local"`
	IntervalEndLocal   time.Time `json:"interval_end_local"`
	IntervalStartUTC   time.Time `json:"interval_start_utc"`
	IntervalEndUTC     time.Time `json:"interval_end_utc"`
	Location           string    `json:"location"`
	Market             string    `json:"market"`
	LMP                float64   `json:"lmp"`
	Action             string    `json:"action"` // "CHARGING", "DISCHARGING", "IDLE"
	RequestedPowerMW   float64   `json:"requested_power_mw"`
	PowerMW            float64   `json:"power_mw"`
	EnergyFromGridMWh  float64   `json:"energy_from_grid_mwh"`
	EnergyToGridMWh    float64   `json:"energy_to_grid_mwh"`
	ThroughputMWh      float64   `json:"throughput_mwh"`
	SOCStart           float64   `json:"soc_start"`
	SOCEnd             float64   `json:"soc_end"`
	PNL                float64   `json:"pnl"`
	CumPNL             float64   `json:"cum_pnl"`
}

// CompareBacktestResponse represents the response from a comparison
type CompareBacktestResponse struct {
	Comparison []ComparisonResult `json:"comparison"`
}

// ComparisonResult contains results for one variation
type ComparisonResult struct {
	Name    string          `json:"name"`
	Summary BacktestSummary `json:"summary"`
}

// RankResponse represents the response from ranking nodes
type RankResponse struct {
	Rankings []Ranking `json:"rankings"`
}

// Ranking represents one ranked location
type Ranking struct {
	Rank         int     `json:"rank"`
	Location     string  `json:"location"`
	Market       string  `json:"market"`
	Count        int     `json:"count"`
	SpreadP95P05 float64 `json:"spread_p95_p05"`
	MinLMP       float64 `json:"min_lmp"`
	MaxLMP       float64 `json:"max_lmp"`
	OracleProfit float64 `json:"oracle_profit"`
}

// BatteryInfo represents information about a battery preset
type BatteryInfo struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	File string      `json:"file"`
	Specs BatterySpecs `json:"specs"`
}

// BatterySpecs contains battery specifications
type BatterySpecs struct {
	EnergyCapacityMWh float64 `json:"energy_capacity_mwh"`
	PowerCapacityMW   float64 `json:"power_capacity_mw"`
}

// StrategyInfo represents information about a strategy
type StrategyInfo struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  []ParameterInfo  `json:"parameters"`
}

// ParameterInfo describes a strategy parameter
type ParameterInfo struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // "float", "int", "string"
	Description string      `json:"description"`
	Default     interface{} `json:"default,omitempty"`
}

// DatasetInfo represents information about a Grid Status dataset
type DatasetInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Market     string `json:"market"`
	Resolution string `json:"resolution"`
}

// LocationInfo represents information about a location
type LocationInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}
