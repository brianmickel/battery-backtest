package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"
)

// RankByOracleProfit returns all scores atomically, descending by profit and
// then ascending by location ID. Scoring is sequential and shares one work budget.
func RankByOracleProfit(ctx context.Context, byLocation map[string][]model.LMPInterval) ([]ArbitragePotential, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(byLocation) == 0 || len(byLocation) > 16 {
		return nil, fmt.Errorf("ranking requires 1 to 16 locations")
	}
	ids := make([]string, 0, len(byLocation))
	for id := range byLocation {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var transitions uint64
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("ranking location ID must not be empty")
		}
		rows := byLocation[id]
		if len(rows) == 0 || rows[0].Location != id {
			return nil, fmt.Errorf("location %q: series must identify the requested location", id)
		}
		estimate, err := strategy.EstimateWork(CanonicalStrategyConfig(), len(rows))
		if err != nil {
			return nil, fmt.Errorf("location %q: %w", id, err)
		}
		if estimate.Transitions > strategy.MaxTransitions-transitions {
			return nil, &strategy.BudgetError{Dimension: "transitions"}
		}
		transitions += estimate.Transitions
	}
	out := make([]ArbitragePotential, 0, len(ids))
	for _, id := range ids {
		p, err := ComputePotential(ctx, byLocation[id])
		if err != nil {
			return nil, fmt.Errorf("location %q: %w", id, err)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OracleProfit == out[j].OracleProfit {
			return out[i].Location < out[j].Location
		}
		return out[i].OracleProfit > out[j].OracleProfit
	})
	return out, nil
}
