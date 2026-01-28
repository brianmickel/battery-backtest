package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"battery-backtest/internal/data"
)

func main() {
	var (
		datasetID  = flag.String("dataset-id", "caiso_lmp_real_time_5_min", "Grid Status dataset ID")
		outputPath = flag.String("output", "", "Output file path (default: ./data/locations.json)")
		seedFile   = flag.String("seed", "", "Path to existing locations file to use as seed")
		days       = flag.Int("days", 7, "Number of days to look back for location discovery")
	)
	flag.Parse()

	apiKey := os.Getenv("GRIDSTATUS_API_KEY")
	if apiKey == "" {
		log.Fatal("GRIDSTATUS_API_KEY environment variable is required")
	}

	if *outputPath == "" {
		*outputPath = data.GetDefaultLocationsPath()
	}

	client := data.NewGridStatusClient(apiKey, "")

	fmt.Printf("Updating locations for dataset: %s\n", *datasetID)

	// Load existing locations as seed if provided
	var existingLocations []data.Location
	if *seedFile != "" {
		if list, err := data.LoadLocations(*seedFile); err == nil {
			existingLocations = list.Locations
			fmt.Printf("Loaded %d existing locations from seed file\n", len(existingLocations))
		}
	} else {
		// Try to load from default path
		if list, err := data.LoadLocations(data.GetDefaultLocationsPath()); err == nil {
			existingLocations = list.Locations
			fmt.Printf("Loaded %d existing locations from default file\n", len(existingLocations))
		}
	}

	// Query known locations to update their metadata
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -*days)

	fmt.Printf("Querying locations from %s to %s to update metadata...\n",
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	locations, err := updateLocationsFromAPI(client, *datasetID, startDate, endDate, existingLocations)
	if err != nil {
		log.Fatalf("Failed to update locations: %v", err)
	}

	fmt.Printf("Found %d total locations\n", len(locations))

	// Create location list
	list := &data.LocationList{
		DatasetID: *datasetID,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Locations: locations,
	}

	// Save to file
	if err := data.SaveLocations(list, *outputPath); err != nil {
		log.Fatalf("Failed to save locations: %v", err)
	}

	fmt.Printf("Saved %d locations to %s\n", len(locations), *outputPath)
}

// updateLocationsFromAPI updates location metadata by querying known locations
// Since Grid Status API requires a location_id to query, we maintain a seed list
// and update metadata for those locations. New locations can be added manually.
func updateLocationsFromAPI(client *data.GridStatusClient, datasetID string, startDate, endDate time.Time, seedLocations []data.Location) ([]data.Location, error) {
	// Build a map of known location IDs from seed
	knownIDs := make(map[string]bool)
	for _, loc := range seedLocations {
		knownIDs[loc.ID] = true
	}

	// Default seed locations if none provided
	if len(seedLocations) == 0 {
		seedLocations = []data.Location{
			{ID: "MOSSLD_2_PSP1", Name: "Moss Landing", Type: "GNODE", Market: "CAISO"},
			{ID: "SV_LNODER6A", Name: "Flagstaff", Type: "LNODE", Market: "CAISO"},
		}
		for _, loc := range seedLocations {
			knownIDs[loc.ID] = true
		}
	}

	locationMap := make(map[string]data.Location)

	// Start with seed locations
	for _, loc := range seedLocations {
		loc.DatasetID = datasetID // Ensure dataset_id is set
		locationMap[loc.ID] = loc
	}

	// Query each known location to update metadata
	fmt.Printf("Querying %d known locations...\n", len(seedLocations))
	successCount := 0

	for _, loc := range seedLocations {
		resp, err := client.QueryLocation(data.QueryLocationParams{
			DatasetID:  datasetID,
			LocationID: loc.ID,
			StartTime:  startDate,
			EndTime:    endDate,
			Timezone:   "market",
			Download:   false,
		})
		if err != nil {
			fmt.Printf("  ⚠️  Warning: Failed to query location %s: %v\n", loc.ID, err)
			// Keep the existing location data even if query fails
			continue
		}

		if len(resp.Data) > 0 {
			first := resp.Data[0]
			// Update location with fresh metadata
			locationMap[first.Location] = data.Location{
				ID:        first.Location,
				Name:      inferLocationName(first.Location, loc.Name),
				Type:      first.LocationType,
				Market:    first.Market,
				DatasetID: datasetID,
			}
			successCount++
			fmt.Printf("  ✓ Updated: %s (%s)\n", first.Location, locationMap[first.Location].Name)
		} else {
			fmt.Printf("  ⚠️  No data for location %s in date range\n", loc.ID)
		}
	}

	fmt.Printf("Successfully updated %d/%d locations\n", successCount, len(seedLocations))

	// Convert map to slice
	locations := make([]data.Location, 0, len(locationMap))
	for _, loc := range locationMap {
		locations = append(locations, loc)
	}

	return locations, nil
}

// inferLocationName attempts to infer a human-readable name from location ID
// If existingName is provided and not empty, it's used; otherwise we try to infer
func inferLocationName(locID, existingName string) string {
	// Use existing name if available
	if existingName != "" {
		return existingName
	}

	// Common location name mappings
	nameMap := map[string]string{
		"MOSSLD_2_PSP1": "Moss Landing",
		"SV_LNODER6A":   "Flagstaff",
	}

	if mapped, ok := nameMap[locID]; ok {
		return mapped
	}

	// Fallback: return the location ID
	return locID
}
