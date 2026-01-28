package backtest

import (
	"time"

	"battery-backtest/internal/model"
)

// LedgerRow is one row of per-interval output.
// This is the primary artifact for "what happened" in a backtest.
type LedgerRow struct {
	Index int

	IntervalStartLocal time.Time
	IntervalEndLocal   time.Time
	IntervalStartUTC   time.Time
	IntervalEndUTC     time.Time

	Location string
	Market   string

	LMP float64

	Action model.Action

	RequestedPowerMW float64
	PowerMW          float64

	EnergyFromGridMWh float64
	EnergyToGridMWh   float64
	ThroughputMWh     float64

	SOCStart float64
	SOCEnd   float64

	PNL    float64
	CumPNL float64
}

type Result struct {
	Ledger []LedgerRow
	TotalPNL float64
	FinalSOC float64
}

