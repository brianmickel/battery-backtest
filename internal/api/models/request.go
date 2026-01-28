package models

// BacktestRequest represents the request body for running a backtest
type BacktestRequest struct {
	APIKey     string           `json:"api_key" binding:"required"` // Grid Status API key
	DataSource DataSourceConfig `json:"data_source" binding:"required"`
	Config     BacktestConfig   `json:"config" binding:"required"`
	Options    BacktestOptions  `json:"options,omitempty"`
}

// DataSourceConfig defines how to fetch market data
type DataSourceConfig struct {
	Type       string `json:"type" binding:"required"` // "gridstatus"
	DatasetID  string `json:"dataset_id" binding:"required"`
	LocationID string `json:"location_id" binding:"required"`
	StartDate  string `json:"start_date" binding:"required"` // YYYY-MM-DD
	EndDate    string `json:"end_date" binding:"required"`   // YYYY-MM-DD
	Timezone   string `json:"timezone,omitempty"`             // default: "market"
}

// BacktestConfig contains battery and strategy configuration
type BacktestConfig struct {
	BatteryFile string                 `json:"battery_file,omitempty"`
	Battery     BatteryConfig          `json:"battery,omitempty"`
	Strategy    StrategyConfig         `json:"strategy" binding:"required"`
}

// BatteryConfig defines battery parameters
type BatteryConfig struct {
	Name                 string  `json:"name,omitempty"`
	EnergyCapacityMWh    float64 `json:"energy_capacity_mwh"`
	PowerCapacityMW      float64 `json:"power_capacity_mw"`
	ChargeEfficiency     float64 `json:"charge_efficiency"`
	DischargeEfficiency  float64 `json:"discharge_efficiency"`
	MinSOC               float64 `json:"min_soc"`
	MaxSOC               float64 `json:"max_soc"`
	InitialSOC           float64 `json:"initial_soc,omitempty"`
	DegradationCostPerMWh float64 `json:"degradation_cost_per_mwh,omitempty"`
}

// StrategyConfig defines strategy and its parameters
type StrategyConfig struct {
	Name   string                 `json:"name" binding:"required"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// BacktestOptions contains optional backtest parameters
type BacktestOptions struct {
	LimitIntervals int  `json:"limit_intervals,omitempty"` // 0 = all
	IncludeLedger  bool `json:"include_ledger,omitempty"`  // default: false
}

// CompareBacktestRequest represents a request to compare multiple backtests
type CompareBacktestRequest struct {
	APIKey     string                 `json:"api_key" binding:"required"` // Grid Status API key
	DataSource DataSourceConfig      `json:"data_source" binding:"required"`
	BaseConfig BacktestConfig        `json:"base_config" binding:"required"`
	Variations []BacktestVariation   `json:"variations" binding:"required"`
}

// BacktestVariation defines a variation to test
type BacktestVariation struct {
	Name    string          `json:"name" binding:"required"`
	Config  BacktestConfig  `json:"config" binding:"required"`
}

// RankRequest represents a request to rank nodes
type RankRequest struct {
	APIKey     string  `form:"api_key" binding:"required"` // Grid Status API key
	DatasetID  string  `form:"dataset_id" binding:"required"`
	StartDate  string  `form:"start_date" binding:"required"`
	EndDate    string  `form:"end_date" binding:"required"`
	LocationIDs string  `form:"location_ids,omitempty"` // comma-separated
	Limit      int      `form:"limit,omitempty"`       // default: 10
}
