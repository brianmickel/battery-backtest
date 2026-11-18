package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"battery-backtest/internal/analysis"
	"battery-backtest/internal/api/models"
	"battery-backtest/internal/data"
	"battery-backtest/internal/strategy"
	"github.com/gin-gonic/gin"
)

func (s server) rank(c *gin.Context) {
	key, err := requiredKey(c)
	if err != nil {
		writeFailure(c, err)
		return
	}
	values := c.Request.URL.Query()
	allowed := map[string]bool{"dataset_id": true, "start_date": true, "end_date": true, "location_ids": true, "limit": true}
	for name, items := range values {
		if !allowed[name] || len(items) != 1 {
			writeFailure(c, invalid("", "Unknown or repeated ranking query parameter."))
			return
		}
	}
	locations := strings.Split(values.Get("location_ids"), ",")
	if len(locations) > 16 {
		writeFailure(c, invalid("location_ids", "Choose 1 to 16 distinct location IDs."))
		return
	}
	seen := map[string]bool{}
	for _, location := range locations {
		if strings.TrimSpace(location) == "" || seen[location] {
			writeFailure(c, invalid("location_ids", "Choose 1 to 16 distinct nonempty location IDs."))
			return
		}
		seen[location] = true
	}
	limit := 10
	if raw, present := values["limit"]; present {
		limit, err = strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > 16 {
			writeFailure(c, invalid("limit", "Limit must be an integer from 1 to 16."))
			return
		}
	}
	query := data.Query{DatasetID: values.Get("dataset_id"), LocationID: locations[0], StartDate: values.Get("start_date"), EndDate: values.Get("end_date"), Clock: "market"}
	if err := validateQuery(query); err != nil {
		writeFailure(c, err)
		return
	}
	ctx, release, ok := admit(c)
	if !ok {
		return
	}
	defer release()
	potentials := make([]analysis.ArbitragePotential, 0, len(locations))
	var transitions uint64
	for _, location := range locations {
		query.LocationID = location
		rows, err := s.deps.Provider.Fetch(ctx, key, query)
		if err == nil {
			var work strategy.WorkEstimate
			work, err = strategy.EstimateWork(analysis.CanonicalStrategyConfig(), len(rows))
			if err == nil {
				if work.Transitions > strategy.MaxTransitions-transitions {
					err = &strategy.BudgetError{Dimension: "transitions"}
				} else {
					transitions += work.Transitions
				}
			}
		}
		if err == nil {
			var potential analysis.ArbitragePotential
			potential, err = analysis.ComputePotential(ctx, rows)
			if err == nil {
				potentials = append(potentials, potential)
			}
		}
		if err != nil {
			failure := classify(err)
			failure.location = location
			writeFailure(c, failure)
			return
		}
	}
	sort.SliceStable(potentials, func(i, j int) bool {
		if potentials[i].OracleProfit == potentials[j].OracleProfit {
			return potentials[i].Location < potentials[j].Location
		}
		return potentials[i].OracleProfit > potentials[j].OracleProfit
	})
	if len(potentials) > limit {
		potentials = potentials[:limit]
	}
	response := models.RankResponse{Rankings: make([]models.Ranking, len(potentials))}
	for i, p := range potentials {
		response.Rankings[i] = models.Ranking{Rank: i + 1, Location: p.Location, Market: p.Market, Count: p.Count, SpreadP95P05: p.SpreadP95P05, MinLMP: p.MinLMP, MaxLMP: p.MaxLMP, OracleProfit: p.OracleProfit}
	}
	if err := ctx.Err(); err != nil {
		writeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}
