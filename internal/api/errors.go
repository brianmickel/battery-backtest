package api

import (
	"context"
	"errors"
	"strings"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
	"battery-backtest/internal/strategy"
	"github.com/gin-gonic/gin"
)

type failure struct {
	status                                           int
	code, message, field, variation, location, retry string
}

func (f failure) Error() string { return f.message }
func invalid(field, message string) error {
	return failure{status: 400, code: "INVALID_REQUEST", field: field, message: message}
}

func classify(err error) failure {
	var explicit failure
	if errors.As(err, &explicit) {
		return explicit
	}
	result := failure{status: 500, code: "INTERNAL_ERROR", message: "The operation could not be completed."}
	var validation *experiment.ValidationError
	var configError *config.ValidationError
	var seriesError *data.ValidationError
	var budget *strategy.BudgetError
	var upstream *data.GridStatusError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.status, result.code, result.message = 504, "DEADLINE_EXCEEDED", "The operation exceeded its time limit."
	case errors.As(err, &validation):
		result.status, result.code, result.message, result.field, result.variation = 400, "INVALID_REQUEST", validation.Message, validation.Field, validation.Variation
	case errors.As(err, &configError):
		result.status, result.code, result.message, result.field = 400, "INVALID_REQUEST", configError.Message, configError.Field
	case errors.As(err, &seriesError):
		result.status, result.code, result.message, result.field = 400, "INVALID_REQUEST", seriesError.Message, seriesError.Field
	case errors.As(err, &budget), errors.Is(err, data.ErrIntervalLimit):
		result.status, result.code, result.message = 422, "WORK_BUDGET_EXCEEDED", "The request exceeds the work budget; reduce its size or optimizer resolution."
	case errors.As(err, &upstream):
		result.status, result.code, result.message = 502, "UPSTREAM_UNAVAILABLE", "The market-data provider is unavailable."
		switch upstream.Code {
		case "UPSTREAM_AUTH":
			result.status, result.code, result.message = 401, "UPSTREAM_AUTH", "The market-data provider rejected the API key."
		case "UPSTREAM_RATE_LIMIT":
			result.status, result.code, result.message, result.retry = 429, "UPSTREAM_RATE_LIMIT", "The market-data provider rate limit was reached.", upstream.RetryAfter
		case "DEADLINE_EXCEEDED":
			result.status, result.code, result.message = 504, "DEADLINE_EXCEEDED", "The market-data provider exceeded its time limit."
		}
	}
	var variation *experiment.VariationError
	if errors.As(err, &variation) {
		result.variation = variation.Variation
	}
	return result
}

func writeFailure(c *gin.Context, err error) {
	if errors.Is(c.Request.Context().Err(), context.Canceled) {
		return
	}
	result := classify(err)
	if result.retry != "" {
		c.Header("Retry-After", result.retry)
	}
	c.AbortWithStatusJSON(result.status, models.ErrorResponse{Error: models.ErrorDetail{Code: result.code, Message: result.message, Field: result.field, Variation: result.variation, LocationID: result.location}})
}

func requiredKey(c *gin.Context) (string, error) {
	if _, present := c.Request.URL.Query()["api_key"]; present {
		return "", invalid("api_key", "Move the API key from the query to X-GridStatus-API-Key.")
	}
	key := c.GetHeader("X-GridStatus-API-Key")
	if strings.TrimSpace(key) == "" {
		return "", invalid("api_key", "Provide X-GridStatus-API-Key.")
	}
	return key, nil
}
