package model

// BacktestInputs represents a canonical "inputs to the system" object.
//
// Your sample_data.json only includes market data; this struct is what the backtester
// will consume once we combine market data + battery configuration + strategy settings.
type BacktestInputs struct {
	MarketData GridStatusLMPResponse
	Battery    BatteryParams
	// Strategy config comes later (rule params, horizons, etc).
}

