package handlers

import (
	"fmt"
	"net/http"
	"os"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/data"

	"github.com/gin-gonic/gin"
)

// ListDatasets handles GET /api/v1/datasets
func ListDatasets(c *gin.Context) {
	// For now, return a hardcoded list of common datasets
	// In the future, this could query Grid Status API for available datasets
	datasets := []models.DatasetInfo{
		{
			ID:         "caiso_lmp_real_time_5_min",
			Name:       "CAISO LMP Real-Time 5-Min",
			Market:     "CAISO",
			Resolution: "5min",
		},
		// Add more datasets as needed
	}

	c.JSON(http.StatusOK, gin.H{"datasets": datasets})
}

// ListLocations handles GET /api/v1/locations
func ListLocations(c *gin.Context) {
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "MISSING_PARAM",
				Message: "dataset_id query parameter is required",
			},
		})
		return
	}

	// Load locations from static file
	locationList, err := loadLocationsForDataset(datasetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "LOCATIONS_LOAD_ERROR",
				Message: fmt.Sprintf("Failed to load locations: %v", err),
			},
		})
		return
	}

	// Convert to response format
	locations := make([]models.LocationInfo, len(locationList.Locations))
	for i, loc := range locationList.Locations {
		locations[i] = models.LocationInfo{
			ID:   loc.ID,
			Name: loc.Name,
			Type: loc.Type,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"locations": locations,
		"updated_at": locationList.UpdatedAt,
		"count": len(locations),
	})
}

// loadLocationsForDataset loads locations from the static file
func loadLocationsForDataset(datasetID string) (*data.LocationList, error) {
	filePath := data.GetDefaultLocationsPath()
	
	locationList, err := data.LoadLocations(filePath)
	if err != nil {
		// If file doesn't exist, return empty list (not an error)
		if os.IsNotExist(err) {
			return &data.LocationList{
				DatasetID: datasetID,
				Locations: []data.Location{},
			}, nil
		}
		return nil, err
	}

	// Filter by dataset_id if specified
	if datasetID != "" && locationList.DatasetID != datasetID {
		// Filter locations that match the dataset
		filtered := []data.Location{}
		for _, loc := range locationList.Locations {
			if loc.DatasetID == datasetID || loc.DatasetID == "" {
				filtered = append(filtered, loc)
			}
		}
		locationList.Locations = filtered
	}

	return locationList, nil
}
