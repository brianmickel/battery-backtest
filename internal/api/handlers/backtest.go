package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"battery-backtest/internal/strategy"

	"github.com/gin-gonic/gin"
)

// BacktestHandler handles backtest-related requests
type BacktestHandler struct{}

// NewBacktestHandler creates a new backtest handler
func NewBacktestHandler(gridStatusClient *data.GridStatusClient) *BacktestHandler {
	_ = gridStatusClient // Not used anymore - API key comes from request
	return &BacktestHandler{}
}

// RunBacktest handles POST /api/v1/backtest
func (h *BacktestHandler) RunBacktest(c *gin.Context) {
	var req models.BacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_REQUEST",
				Message: err.Error(),
			},
		})
		return
	}

	// Validate API key
	if err := validateAPIKey(req.APIKey); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_API_KEY",
				Message: err.Error(),
			},
		})
		return
	}

	// Fetch data from Grid Status
	intervals, err := h.fetchData(req.DataSource, req.APIKey)
	if err != nil {
		// Handle Grid Status API errors
		if gsErr, ok := err.(*data.GridStatusError); ok {
			statusCode := http.StatusBadRequest
			if gsErr.StatusCode == http.StatusForbidden || gsErr.StatusCode == http.StatusUnauthorized {
				statusCode = http.StatusUnauthorized
			} else if gsErr.StatusCode == http.StatusTooManyRequests {
				statusCode = http.StatusTooManyRequests
			}
			c.JSON(statusCode, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    gsErr.Code,
					Message: gsErr.Message,
					Details: map[string]interface{}{
						"status_code": gsErr.StatusCode,
						"retry_after": gsErr.RetryAfter,
					},
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "DATA_FETCH_ERROR",
				Message: err.Error(),
			},
		})
		return
	}

	// Apply interval limit if specified
	if req.Options.LimitIntervals > 0 && req.Options.LimitIntervals < len(intervals) {
		intervals = intervals[:req.Options.LimitIntervals]
	}

	// Build config from request
	cfg, err := h.buildConfig(req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_CONFIG",
				Message: err.Error(),
			},
		})
		return
	}

	// Create battery
	batt, err := model.NewBattery(cfg.Battery.ToModelParams(), cfg.Battery.InitialSOC)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_BATTERY",
				Message: err.Error(),
			},
		})
		return
	}

	// Start at min SOC
	batt.State.SOC = batt.Params.MinSOC

	// Build strategy
	strat := h.buildStrategy(cfg, intervals, batt)

	// Run backtest
	engine := backtest.New()
	result, err := engine.Run(intervals, batt, strat)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "BACKTEST_ERROR",
				Message: err.Error(),
			},
		})
		return
	}

	// Build response
	response := h.buildResponse(result, req.Options.IncludeLedger)
	c.JSON(http.StatusOK, response)
}

// GetLedger handles GET /api/v1/backtest/:id/ledger
// For now, this is a placeholder - we'd need to implement result caching
func (h *BacktestHandler) GetLedger(c *gin.Context) {
	id := c.Param("id")
	c.JSON(http.StatusNotImplemented, models.ErrorResponse{
		Error: models.ErrorDetail{
			Code:    "NOT_IMPLEMENTED",
			Message: "Ledger retrieval not yet implemented. Use include_ledger=true in backtest request.",
		},
	})
	_ = id // TODO: implement result caching
}

// CompareBacktests handles POST /api/v1/backtest/compare
func (h *BacktestHandler) CompareBacktests(c *gin.Context) {
	var req models.CompareBacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_REQUEST",
				Message: err.Error(),
			},
		})
		return
	}

	// Validate API key
	if err := validateAPIKey(req.APIKey); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INVALID_API_KEY",
				Message: err.Error(),
			},
		})
		return
	}

	// Fetch data once
	intervals, err := h.fetchData(req.DataSource, req.APIKey)
	if err != nil {
		// Handle Grid Status API errors
		if gsErr, ok := err.(*data.GridStatusError); ok {
			statusCode := http.StatusBadRequest
			if gsErr.StatusCode == http.StatusForbidden || gsErr.StatusCode == http.StatusUnauthorized {
				statusCode = http.StatusUnauthorized
			} else if gsErr.StatusCode == http.StatusTooManyRequests {
				statusCode = http.StatusTooManyRequests
			}
			c.JSON(statusCode, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    gsErr.Code,
					Message: gsErr.Message,
					Details: map[string]interface{}{
						"status_code": gsErr.StatusCode,
						"retry_after": gsErr.RetryAfter,
					},
				},
			})
			return
		}
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "DATA_FETCH_ERROR",
				Message: err.Error(),
			},
		})
		return
	}

	// Run each variation
	comparison := make([]models.ComparisonResult, 0, len(req.Variations))
	engine := backtest.New()

	for _, variation := range req.Variations {
		// Merge base config with variation
		mergedConfig := h.mergeConfig(req.BaseConfig, variation.Config)

		// Convert to config.Config
		cfg, err := h.buildConfig(mergedConfig)
		if err != nil {
			continue // Skip invalid configs
		}

		// Create battery
		batt, err := model.NewBattery(cfg.Battery.ToModelParams(), cfg.Battery.InitialSOC)
		if err != nil {
			continue // Skip invalid configs
		}
		batt.State.SOC = batt.Params.MinSOC

		// Build strategy
		strat := h.buildStrategy(cfg, intervals, batt)

		// Run backtest
		result, err := engine.Run(intervals, batt, strat)
		if err != nil {
			continue // Skip failed backtests
		}

		comparison = append(comparison, models.ComparisonResult{
			Name:    variation.Name,
			Summary: h.buildSummary(result),
		})
	}

	c.JSON(http.StatusOK, models.CompareBacktestResponse{
		Comparison: comparison,
	})
}

// Helper methods

func (h *BacktestHandler) fetchData(ds models.DataSourceConfig, apiKey string) ([]model.LMPInterval, error) {
	if ds.Type != "gridstatus" {
		return nil, fmt.Errorf("unsupported data source type: %s", ds.Type)
	}

	// Create a new client with the API key from the request
	client := data.NewGridStatusClient(apiKey, "")

	timezone := ds.Timezone
	if timezone == "" {
		timezone = "market"
	}

	resp, err := client.QueryLocationByString(
		ds.DatasetID,
		ds.LocationID,
		ds.StartDate,
		ds.EndDate,
	)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// validateAPIKey performs basic validation on the API key
func validateAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}
	// Basic validation: reject obviously invalid keys
	if len(apiKey) < 10 {
		return fmt.Errorf("API key appears to be invalid (too short)")
	}
	// Reject keys that are just whitespace
	if len(strings.TrimSpace(apiKey)) == 0 {
		return fmt.Errorf("API key cannot be empty or whitespace")
	}
	return nil
}

func (h *BacktestHandler) buildConfig(req models.BacktestConfig) (*config.Config, error) {
	cfg := &config.Config{
		BatteryFile: req.BatteryFile,
		Battery: config.BatteryConfig{
			Name:                  req.Battery.Name,
			EnergyCapacityMWh:     req.Battery.EnergyCapacityMWh,
			PowerCapacityMW:       req.Battery.PowerCapacityMW,
			ChargeEfficiency:      req.Battery.ChargeEfficiency,
			DischargeEfficiency:   req.Battery.DischargeEfficiency,
			MinSOC:                req.Battery.MinSOC,
			MaxSOC:                req.Battery.MaxSOC,
			InitialSOC:            req.Battery.InitialSOC,
			DegradationCostPerMWh: req.Battery.DegradationCostPerMWh,
		},
		Strategy: config.StrategyConfig{
			Name:   req.Strategy.Name,
			Params: req.Strategy.Params,
		},
	}

	// If battery_file is set, load it and merge request overrides onto it
	if cfg.BatteryFile != "" {
		// battery_file should be just the filename (e.g., "1_moss_landing")
		// Files are always looked up in examples/batteries/ directory
		batteryDir := os.Getenv("BATTERY_DIR")
		if batteryDir == "" {
			// Try to resolve relative to working directory
			wd, err := os.Getwd()
			if err == nil {
				batteryDir = filepath.Join(wd, "examples", "batteries")
			} else {
				batteryDir = "./examples/batteries"
			}
		}
		batteryPath := filepath.Join(batteryDir, cfg.BatteryFile+".yaml")
		
		loaded, err := config.LoadUnchecked(batteryPath)
		if err == nil {
			// Merge: battery file is base, request config is override
			cfg.Battery = config.MergeBattery(loaded.Battery, cfg.Battery)
		} else {
			log.Printf("BacktestHandler: Failed to load battery file %s: %v", batteryPath, err)
		}
	}

	// Apply default InitialSOC if not set (default to MinSOC)
	if cfg.Battery.InitialSOC == 0 {
		cfg.Battery.InitialSOC = cfg.Battery.MinSOC
	}

	return cfg, nil
}

func (h *BacktestHandler) mergeConfig(base, override models.BacktestConfig) models.BacktestConfig {
	merged := base
	if override.BatteryFile != "" {
		merged.BatteryFile = override.BatteryFile
	}
	// Merge battery config (simple merge - override non-zero values)
	if override.Battery.EnergyCapacityMWh != 0 {
		merged.Battery.EnergyCapacityMWh = override.Battery.EnergyCapacityMWh
	}
	if override.Battery.PowerCapacityMW != 0 {
		merged.Battery.PowerCapacityMW = override.Battery.PowerCapacityMW
	}
	// ... add more merge logic as needed
	if override.Strategy.Name != "" {
		merged.Strategy = override.Strategy
	}
	return merged
}

func (h *BacktestHandler) buildStrategy(cfg *config.Config, intervals []model.LMPInterval, batt *model.Battery) strategy.Strategy {
	switch cfg.Strategy.Name {
	case "schedule":
		chargeStart := mustStr(cfg.Strategy.Params, "charge_start", "10:00")
		dischargeStart := mustStr(cfg.Strategy.Params, "discharge_start", "17:00")
		chargeEnd := mustStr(cfg.Strategy.Params, "charge_end", dischargeStart)
		dischargeEnd := mustStr(cfg.Strategy.Params, "discharge_end", "23:59")
		chargeMW := mustNum(cfg.Strategy.Params, "charge_power_mw", cfg.Battery.PowerCapacityMW)
		dischargeMW := mustNum(cfg.Strategy.Params, "discharge_power_mw", cfg.Battery.PowerCapacityMW)
		return &strategy.ScheduleStrategy{Params: strategy.ScheduleParams{
			ChargeStart:      chargeStart,
			ChargeEnd:        chargeEnd,
			DischargeStart:   dischargeStart,
			DischargeEnd:     dischargeEnd,
			ChargePowerMW:    chargeMW,
			DischargePowerMW: dischargeMW,
		}}
	case "oracle":
		socSteps := int(mustNum(cfg.Strategy.Params, "soc_steps", 200))
		powerSteps := int(mustNum(cfg.Strategy.Params, "power_steps", 10))
		orc, err := strategy.NewOracleStrategy(intervals, batt.Params, batt.State.SOC, strategy.OracleParams{
			SocSteps:   socSteps,
			PowerSteps: powerSteps,
		})
		if err != nil {
			panic(err) // Should be handled better
		}
		return orc
	default:
		panic(fmt.Errorf("unsupported strategy: %q", cfg.Strategy.Name))
	}
}

func (h *BacktestHandler) buildResponse(result *backtest.Result, includeLedger bool) models.BacktestResponse {
	response := models.BacktestResponse{
		Status:  "completed",
		Summary: h.buildSummary(result),
	}

	if includeLedger {
		response.Ledger = h.convertLedger(result.Ledger)
	}

	return response
}

func (h *BacktestHandler) buildSummary(result *backtest.Result) models.BacktestSummary {
	if len(result.Ledger) == 0 {
		return models.BacktestSummary{
			TotalPNL:       result.TotalPNL,
			FinalSOC:       result.FinalSOC,
			TotalIntervals: 0,
		}
	}

	start := result.Ledger[0].IntervalStartLocal
	end := result.Ledger[len(result.Ledger)-1].IntervalEndLocal

	var chargeTotal, dischargeTotal float64
	
	// Group intervals by day for per-day windows
	type dayKey struct {
		Year  int
		Month time.Month
		Day   int
	}
	
	// Track windows with cost/price calculations
	type chargeWindowData struct {
		window      models.TimeWindow
		totalCost   float64 // Sum of LMP * Energy for weighted average
		totalEnergy float64
	}
	
	type dischargeWindowData struct {
		window      models.TimeWindow
		totalRevenue float64 // Sum of LMP * Energy for weighted average
		totalEnergy  float64
	}
	
	chargeWindowsByDay := make(map[dayKey]*chargeWindowData)
	dischargeWindowsByDay := make(map[dayKey]*dischargeWindowData)

	for _, row := range result.Ledger {
		// Calculate totals
		if row.EnergyFromGridMWh > 0 {
			chargeTotal += row.EnergyFromGridMWh
			
			// Per-day: track charge windows with cost
			day := dayKey{
				Year:  row.IntervalStartLocal.Year(),
				Month: row.IntervalStartLocal.Month(),
				Day:   row.IntervalStartLocal.Day(),
			}
			if win, exists := chargeWindowsByDay[day]; exists {
				// Extend window to include this interval
				win.window.End = row.IntervalEndLocal
				// Accumulate cost (weighted by energy)
				win.totalCost += row.LMP * row.EnergyFromGridMWh
				win.totalEnergy += row.EnergyFromGridMWh
			} else {
				// Start new window for this day
				chargeWindowsByDay[day] = &chargeWindowData{
					window: models.TimeWindow{
						Start: row.IntervalStartLocal,
						End:   row.IntervalEndLocal,
					},
					totalCost:   row.LMP * row.EnergyFromGridMWh,
					totalEnergy: row.EnergyFromGridMWh,
				}
			}
		}
		
		if row.EnergyToGridMWh > 0 {
			dischargeTotal += row.EnergyToGridMWh
			
			// Per-day: track discharge windows with price
			day := dayKey{
				Year:  row.IntervalStartLocal.Year(),
				Month: row.IntervalStartLocal.Month(),
				Day:   row.IntervalStartLocal.Day(),
			}
			if win, exists := dischargeWindowsByDay[day]; exists {
				// Extend window to include this interval
				win.window.End = row.IntervalEndLocal
				// Accumulate revenue (weighted by energy)
				win.totalRevenue += row.LMP * row.EnergyToGridMWh
				win.totalEnergy += row.EnergyToGridMWh
			} else {
				// Start new window for this day
				dischargeWindowsByDay[day] = &dischargeWindowData{
					window: models.TimeWindow{
						Start: row.IntervalStartLocal,
						End:   row.IntervalEndLocal,
					},
					totalRevenue: row.LMP * row.EnergyToGridMWh,
					totalEnergy:  row.EnergyToGridMWh,
				}
			}
		}
	}

	// Convert maps to sorted arrays (by day) with average costs
	chargeWindows := make([]models.ChargeWindow, 0, len(chargeWindowsByDay))
	dischargeWindows := make([]models.DischargeWindow, 0, len(dischargeWindowsByDay))
	
	// Track which days we've already added to maintain order
	seenChargeDays := make(map[dayKey]bool)
	seenDischargeDays := make(map[dayKey]bool)
	
	// Iterate through ledger in chronological order to build sorted arrays
	for _, row := range result.Ledger {
		day := dayKey{
			Year:  row.IntervalStartLocal.Year(),
			Month: row.IntervalStartLocal.Month(),
			Day:   row.IntervalStartLocal.Day(),
		}
		
		if row.EnergyFromGridMWh > 0 {
			if !seenChargeDays[day] {
				if winData, exists := chargeWindowsByDay[day]; exists {
					avgCost := 0.0
					if winData.totalEnergy > 0 {
						avgCost = winData.totalCost / winData.totalEnergy
					}
					chargeWindows = append(chargeWindows, models.ChargeWindow{
						TimeWindow:         winData.window,
						AverageCostPerMWh:  avgCost,
						EnergyMWh:          winData.totalEnergy,
					})
					seenChargeDays[day] = true
				}
			}
		}
		
		if row.EnergyToGridMWh > 0 {
			if !seenDischargeDays[day] {
				if winData, exists := dischargeWindowsByDay[day]; exists {
					avgPrice := 0.0
					if winData.totalEnergy > 0 {
						avgPrice = winData.totalRevenue / winData.totalEnergy
					}
					dischargeWindows = append(dischargeWindows, models.DischargeWindow{
						TimeWindow:          winData.window,
						AveragePricePerMWh:  avgPrice,
						EnergyMWh:           winData.totalEnergy,
					})
					seenDischargeDays[day] = true
				}
			}
		}
	}

	summary := models.BacktestSummary{
		TotalPNL:            result.TotalPNL,
		FinalSOC:            result.FinalSOC,
		TotalIntervals:      len(result.Ledger),
		BacktestWindow:      models.TimeWindow{Start: start, End: end},
		EnergyChargedMWh:    chargeTotal,
		EnergyDischargedMWh: dischargeTotal,
		ChargeWindows:       chargeWindows,
		DischargeWindows:    dischargeWindows,
	}

	return summary
}

func (h *BacktestHandler) convertLedger(ledger []backtest.LedgerRow) []models.LedgerRow {
	result := make([]models.LedgerRow, len(ledger))
	for i, row := range ledger {
		result[i] = models.LedgerRow{
			Index:              row.Index,
			IntervalStartLocal: row.IntervalStartLocal,
			IntervalEndLocal:   row.IntervalEndLocal,
			IntervalStartUTC:   row.IntervalStartUTC,
			IntervalEndUTC:     row.IntervalEndUTC,
			Location:           row.Location,
			Market:             row.Market,
			LMP:                row.LMP,
			Action:             string(row.Action),
			RequestedPowerMW:   row.RequestedPowerMW,
			PowerMW:            row.PowerMW,
			EnergyFromGridMWh:  row.EnergyFromGridMWh,
			EnergyToGridMWh:    row.EnergyToGridMWh,
			ThroughputMWh:      row.ThroughputMWh,
			SOCStart:           row.SOCStart,
			SOCEnd:             row.SOCEnd,
			PNL:                row.PNL,
			CumPNL:             row.CumPNL,
		}
	}
	return result
}

// Helper functions (similar to CLI)
func mustNum(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key]; ok && v != nil {
		switch x := v.(type) {
		case float64:
			return x
		case int:
			return float64(x)
		}
	}
	return def
}

func mustStr(m map[string]interface{}, key string, def string) string {
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}
