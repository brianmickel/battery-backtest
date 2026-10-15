package api

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"github.com/gin-gonic/gin"
)

func (s server) batteries(c *gin.Context) {
	presets := s.deps.Presets.List()
	result := make([]models.BatteryInfo, len(presets))
	for i, preset := range presets {
		name := preset.Battery.Name
		if name == "" {
			name = preset.ID
		}
		result[i] = models.BatteryInfo{ID: preset.ID, Name: name, Specs: models.BatterySpecs{EnergyCapacityMWh: preset.Battery.EnergyCapacityMWh, PowerCapacityMW: preset.Battery.PowerCapacityMW}}
	}
	c.JSON(http.StatusOK, gin.H{"batteries": result})
}
func (s server) strategies(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"strategies": config.StrategyMetadata()})
}
func (s server) datasets(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"datasets": []models.DatasetInfo{{ID: "caiso_lmp_real_time_5_min", Name: "CAISO LMP Real-Time 5-Min", Market: "CAISO", Resolution: "5min"}}})
}
func (s server) locations(c *gin.Context) {
	dataset := c.Query("dataset_id")
	if strings.TrimSpace(dataset) == "" {
		writeFailure(c, invalid("dataset_id", "Dataset ID is required."))
		return
	}
	result := []models.LocationInfo{}
	if s.deps.LocationsPath == "" {
		c.JSON(http.StatusOK, gin.H{"locations": result})
		return
	}
	catalog, err := data.LoadLocations(s.deps.LocationsPath)
	if errors.Is(err, fs.ErrNotExist) {
		c.JSON(http.StatusOK, gin.H{"locations": result})
		return
	}
	if err != nil {
		writeFailure(c, err)
		return
	}
	if catalog.DatasetID == dataset {
		for _, location := range catalog.Locations {
			if location.DatasetID == "" || location.DatasetID == dataset {
				result = append(result, models.LocationInfo{ID: location.ID, Name: location.Name, Type: location.Type})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"locations": result, "updated_at": catalog.UpdatedAt, "count": len(result)})
}
