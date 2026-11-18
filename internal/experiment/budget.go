package experiment

import (
	"math"
	"time"
	"unsafe"

	"battery-backtest/internal/backtest"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

// estimateRequest includes supplied and normalized backing arrays even when a
// prefix is selected, every retained outcome, and the largest sequential planner.
func estimateRequest(prepared []NamedPrepared, rows []model.LMPInterval, fullRows []model.LMPInterval) error {
	var transitions, largestPlan, retained uint64
	if err := addAllocation(&retained, 2*uint64(len(fullRows)), uint64(unsafe.Sizeof(model.LMPInterval{}))); err != nil {
		return err
	}
	for _, row := range fullRows {
		for _, value := range []string{row.Location, row.Market, row.LocationType} {
			if err := addAllocation(&retained, uint64(len(value)), 1); err != nil {
				return err
			}
		}
	}
	output, err := outputBytes(rows)
	if err != nil {
		return err
	}
	for _, named := range prepared {
		for _, value := range []string{named.Name, named.Prepared.resolved.Battery.Name} {
			if err := addAllocation(&retained, uint64(len(value)), 1); err != nil {
				return err
			}
		}
		estimate, err := strategy.EstimateWork(named.Prepared.resolved.Strategy, len(rows))
		if err != nil {
			return variationError(named.Name, err)
		}
		if estimate.Transitions > strategy.MaxTransitions-transitions {
			return &strategy.BudgetError{Dimension: "transitions"}
		}
		transitions += estimate.Transitions
		if estimate.WorkingBytes > largestPlan {
			largestPlan = estimate.WorkingBytes
		}
		if err := addAllocation(&retained, 1, output); err != nil {
			return err
		}
	}
	return addAllocation(&retained, 1, largestPlan)
}

func outputBytes(rows []model.LMPInterval) (uint64, error) {
	// Window slices grow by append. Four times the maximum payload covers live
	// capacity and backing-array growth, even if every interval forms a new window.
	windowBytes := 4 * uint64(unsafe.Sizeof(backtest.ChargeWindow{}))
	out := uint64(unsafe.Sizeof(Outcome{})) + 64<<10
	if err := addAllocation(&out, uint64(len(rows)), uint64(unsafe.Sizeof(backtest.LedgerRow{}))); err != nil {
		return 0, err
	}
	for _, row := range rows {
		start, end := row.IntervalStartLocal, row.IntervalEndLocal
		// Reject very long windows before date arithmetic or summary allocation.
		if math.Abs(float64(end.Year())-float64(start.Year())) > float64(strategy.MaxWorkingBytes/windowBytes/365)+1 {
			return 0, &strategy.BudgetError{Dimension: "working_bytes"}
		}
		startDay, endDay := civilDay(start), civilDay(end)
		days := endDay - startDay
		if days < 0 {
			days = -days
		}
		if err := addAllocation(&out, uint64(days)+1, windowBytes); err != nil {
			return 0, err
		}
	}
	return out, nil
}

func civilDay(value time.Time) int64 {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

func addAllocation(total *uint64, count, size uint64) error {
	if size > 0 && count > strategy.MaxWorkingBytes/size {
		return &strategy.BudgetError{Dimension: "working_bytes"}
	}
	bytes := count * size
	if bytes > strategy.MaxWorkingBytes-*total {
		return &strategy.BudgetError{Dimension: "working_bytes"}
	}
	*total += bytes
	return nil
}
