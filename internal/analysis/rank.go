package analysis

import (
	"sort"

	"battery-backtest/internal/model"
)

type RankedPotential struct {
	ArbitragePotential
}

// RankByOracleProfit computes potentials per location and sorts descending by OracleProfit.
func RankByOracleProfit(byLocation map[string][]model.LMPInterval) []RankedPotential {
	out := make([]RankedPotential, 0, len(byLocation))
	for _, intervals := range byLocation {
		p := ComputePotential(intervals)
		out = append(out, RankedPotential{ArbitragePotential: p})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OracleProfit > out[j].OracleProfit
	})
	return out
}

