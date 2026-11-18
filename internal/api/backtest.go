package api

import (
	"net/http"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/experiment"
	"github.com/gin-gonic/gin"
)

func (s server) backtest(c *gin.Context) {
	key, err := requiredKey(c)
	if err != nil {
		writeFailure(c, err)
		return
	}
	var request singleRequest
	if err := decodeBody(c, &request); err != nil {
		writeFailure(c, err)
		return
	}
	query, err := sourceQuery(request.DataSource)
	if err != nil {
		writeFailure(c, err)
		return
	}
	if request.Config == nil {
		writeFailure(c, invalid("config", "Configuration is required."))
		return
	}
	prepared, err := experiment.Prepare(*request.Config, s.deps.Presets)
	if err != nil {
		writeFailure(c, err)
		return
	}
	options, err := runOptions(query, request.Options)
	if err != nil {
		writeFailure(c, err)
		return
	}
	ctx, release, ok := admit(c)
	if !ok {
		return
	}
	defer release()
	rows, err := s.deps.Provider.Fetch(ctx, key, query)
	if err != nil {
		writeFailure(c, err)
		return
	}
	outcome, err := experiment.Run(ctx, prepared, rows, options)
	if err != nil {
		writeFailure(c, err)
		return
	}
	response := models.BacktestResponse{Status: "completed", Summary: wireSummary(outcome.Summary)}
	if request.Options.IncludeLedger {
		response.Ledger = wireLedger(outcome.Ledger)
	}
	if err := ctx.Err(); err != nil {
		writeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (s server) compare(c *gin.Context) {
	key, err := requiredKey(c)
	if err != nil {
		writeFailure(c, err)
		return
	}
	var request comparisonRequest
	if err := decodeBody(c, &request); err != nil {
		writeFailure(c, err)
		return
	}
	query, err := sourceQuery(request.DataSource)
	if err != nil {
		writeFailure(c, err)
		return
	}
	if request.BaseConfig == nil {
		writeFailure(c, invalid("base_config", "Base configuration is required."))
		return
	}
	variations := make([]experiment.Variation, len(request.Variations))
	for i, variation := range request.Variations {
		if variation.Config == nil {
			writeFailure(c, failure{status: 400, code: "INVALID_REQUEST", message: "Variation configuration is required.", field: "config", variation: variation.Name})
			return
		}
		variations[i] = experiment.Variation{Name: variation.Name, Config: *variation.Config}
	}
	prepared, err := experiment.PrepareComparison(*request.BaseConfig, variations, s.deps.Presets)
	if err != nil {
		writeFailure(c, err)
		return
	}
	ctx, release, ok := admit(c)
	if !ok {
		return
	}
	defer release()
	rows, err := s.deps.Provider.Fetch(ctx, key, query)
	if err != nil {
		writeFailure(c, err)
		return
	}
	outcomes, err := experiment.Compare(ctx, prepared, rows, experiment.Options{Clock: query.Clock})
	if err != nil {
		writeFailure(c, err)
		return
	}
	response := models.CompareBacktestResponse{Comparison: make([]models.ComparisonResult, len(outcomes))}
	for i, outcome := range outcomes {
		response.Comparison[i] = models.ComparisonResult{Name: outcome.Name, Summary: wireSummary(outcome.Outcome.Summary)}
	}
	if err := ctx.Err(); err != nil {
		writeFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}
