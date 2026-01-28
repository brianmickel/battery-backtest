package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Location represents a location/node from Grid Status
type Location struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`        // e.g., "GNODE", "LNODE"
	Market      string `json:"market"`      // e.g., "CAISO"
	DatasetID   string `json:"dataset_id"`  // Dataset this location belongs to
}

// LocationList represents a collection of locations
type LocationList struct {
	DatasetID string     `json:"dataset_id"`
	UpdatedAt string     `json:"updated_at"` // ISO 8601 timestamp
	Locations []Location `json:"locations"`
}

// LoadLocations loads locations from a JSON file
func LoadLocations(filePath string) (*LocationList, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read locations file: %w", err)
	}

	var list LocationList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("failed to parse locations file: %w", err)
	}

	return &list, nil
}

// SaveLocations saves locations to a JSON file
func SaveLocations(list *LocationList, filePath string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal locations: %w", err)
	}

	if err := os.WriteFile(filePath, raw, 0644); err != nil {
		return fmt.Errorf("failed to write locations file: %w", err)
	}

	return nil
}

// GetDefaultLocationsPath returns the default path for locations file
func GetDefaultLocationsPath() string {
	// Try environment variable first
	if path := os.Getenv("LOCATIONS_FILE"); path != "" {
		return path
	}
	// Default to data/locations.json in project root
	return "./data/locations.json"
}
