package handlers

import (
	"log"
	"net/http"

	"battery-backtest/internal/api/models"

	"github.com/gin-gonic/gin"
)

// StrategyHandler handles strategy-related requests
type StrategyHandler struct{}

// NewStrategyHandler creates a new strategy handler
func NewStrategyHandler() *StrategyHandler {
	return &StrategyHandler{}
}

// ListStrategies handles GET /api/v1/strategies
func (h *StrategyHandler) ListStrategies(c *gin.Context) {
	log.Printf("StrategyHandler: ListStrategies called")
	strategies := []models.StrategyInfo{
		{
			Name:        "schedule",
			Description: "Time-based schedule strategy. Charges and discharges at specific times each day.",
			Parameters: []models.ParameterInfo{
				{
					Name:        "charge_start",
					Type:        "string",
					Description: "Start time for charging (HH:MM format, e.g., '10:00')",
					Default:     "10:00",
				},
				{
					Name:        "charge_end",
					Type:        "string",
					Description: "End time for charging (HH:MM format)",
					Default:     "17:00",
				},
				{
					Name:        "discharge_start",
					Type:        "string",
					Description: "Start time for discharging (HH:MM format, e.g., '17:00')",
					Default:     "17:00",
				},
				{
					Name:        "discharge_end",
					Type:        "string",
					Description: "End time for discharging (HH:MM format)",
					Default:     "23:59",
				},
				{
					Name:        "charge_power_mw",
					Type:        "float",
					Description: "Charge power in MW",
					Default:     0.0,
				},
				{
					Name:        "discharge_power_mw",
					Type:        "float",
					Description: "Discharge power in MW",
					Default:     0.0,
				},
			},
		},
		{
			Name:        "oracle",
			Description: "Perfect foresight optimizer. Uses dynamic programming to find optimal dispatch with full knowledge of future prices.",
			Parameters: []models.ParameterInfo{
				{
					Name:        "soc_steps",
					Type:        "int",
					Description: "Number of SOC discretization steps (higher = more accurate but slower)",
					Default:     200,
				},
				{
					Name:        "power_steps",
					Type:        "int",
					Description: "Number of power discretization steps",
					Default:     10,
				},
			},
		},
	}

	log.Printf("StrategyHandler: Returning %d strategies", len(strategies))
	c.JSON(http.StatusOK, gin.H{"strategies": strategies})
}
