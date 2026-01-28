package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"battery-backtest/internal/analysis"
	"battery-backtest/internal/api/models"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"

	"github.com/gin-gonic/gin"
)

// RankHandler handles ranking-related requests
type RankHandler struct{}

// NewRankHandler creates a new rank handler
func NewRankHandler(gridStatusClient *data.GridStatusClient) *RankHandler {
	_ = gridStatusClient // Not used anymore - API key comes from request
	return &RankHandler{}
}

// RankNodes handles GET /api/v1/rank
func (h *RankHandler) RankNodes(c *gin.Context) {
	var req models.RankRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_REQUEST",
				Message: err.Error(),
			},
		})
		return
	}

	// Validate API key
	if err := validateAPIKeyForRank(req.APIKey); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_API_KEY",
				Message: err.Error(),
			},
		})
		return
	}

	// Create client with API key from request
	client := data.NewGridStatusClient(req.APIKey, "")

	// Parse dates
	startTime, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_DATE",
				Message: "start_date must be in YYYY-MM-DD format",
			},
		})
		return
	}

	endTime, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_DATE",
				Message: "end_date must be in YYYY-MM-DD format",
			},
		})
		return
	}

	// Parse location IDs if provided
	var locationIDs []string
	if req.LocationIDs != "" {
		locationIDs = strings.Split(req.LocationIDs, ",")
		for i := range locationIDs {
			locationIDs[i] = strings.TrimSpace(locationIDs[i])
		}
	}

	// Fetch data for each location
	byLoc := make(map[string][]model.LMPInterval)
	
	if len(locationIDs) > 0 {
		// Fetch specific locations
		for _, locID := range locationIDs {
			resp, err := client.QueryLocation(data.QueryLocationParams{
				DatasetID:  req.DatasetID,
				LocationID: locID,
				StartTime:  startTime,
				EndTime:    endTime,
				Timezone:   "market",
				Download:   true,
			})
			if err != nil {
				// Handle Grid Status API errors
				if gsErr, ok := err.(*data.GridStatusError); ok {
					// For ranking, we might want to continue with other locations
					// but log the error. For now, return error on auth/rate limit issues
					if gsErr.StatusCode == http.StatusForbidden || 
					   gsErr.StatusCode == http.StatusUnauthorized ||
					   gsErr.StatusCode == http.StatusTooManyRequests {
						statusCode := http.StatusBadRequest
						if gsErr.StatusCode == http.StatusForbidden || gsErr.StatusCode == http.StatusUnauthorized {
							statusCode = http.StatusUnauthorized
						} else if gsErr.StatusCode == http.StatusTooManyRequests {
							statusCode = http.StatusTooManyRequests
						}
						c.JSON(statusCode, models.ErrorResponse{
							Error: models.ErrorDetail{
								Code:    gsErr.Code,
								Message: fmt.Sprintf("Error querying location %s: %s", locID, gsErr.Message),
								Details: map[string]interface{}{
									"status_code": gsErr.StatusCode,
									"retry_after": gsErr.RetryAfter,
									"location_id": locID,
								},
							},
						})
						return
					}
				}
				// For other errors, skip this location
				continue
			}
			byLoc[locID] = resp.Data
		}
	} else {
		// TODO: If no locations specified, we'd need to query all locations
		// For now, return error suggesting specific locations
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "LOCATIONS_REQUIRED",
				Message: "Please specify location_ids query parameter (comma-separated)",
			},
		})
		return
	}

	// Rank by oracle profit
	ranked := analysis.RankByOracleProfit(byLoc)

	// Apply limit
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > len(ranked) {
		limit = len(ranked)
	}
	ranked = ranked[:limit]

	// Convert to response format
	rankings := make([]models.Ranking, len(ranked))
	for i, r := range ranked {
		rankings[i] = models.Ranking{
			Rank:         i + 1,
			Location:     r.Location,
			Market:       r.Market,
			Count:        r.Count,
			SpreadP95P05: r.SpreadP95P05,
			MinLMP:       r.MinLMP,
			MaxLMP:       r.MaxLMP,
			OracleProfit: r.OracleProfit,
		}
	}

	c.JSON(http.StatusOK, models.RankResponse{Rankings: rankings})
}

// validateAPIKeyForRank performs basic validation on the API key
func validateAPIKeyForRank(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}
	if len(apiKey) < 10 {
		return fmt.Errorf("API key appears to be invalid (too short)")
	}
	if len(strings.TrimSpace(apiKey)) == 0 {
		return fmt.Errorf("API key cannot be empty or whitespace")
	}
	return nil
}
