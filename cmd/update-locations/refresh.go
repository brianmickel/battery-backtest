package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

type locationProvider interface {
	Fetch(context.Context, string, data.Query) ([]model.LMPInterval, error)
}

type refreshOptions struct {
	dataset, output, seed string
	days                  int
}

type refreshResult struct {
	status                 string
	total, updated, failed int
}

func refreshCatalog(ctx context.Context, provider locationProvider, apiKey string, options refreshOptions, now time.Time, progress io.Writer) (refreshResult, error) {
	result := refreshResult{status: "failed"}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	path, lock, err := lockDestination(options.output)
	if err != nil {
		return result, err
	}
	defer lock.Close()

	locations, err := loadRefreshLocations(path, options.seed, options.dataset)
	if err != nil {
		return result, err
	}
	result.total = len(locations)
	end := now.UTC()
	query := data.Query{DatasetID: options.dataset, StartDate: end.AddDate(0, 0, -options.days).Format(time.DateOnly), EndDate: end.Format(time.DateOnly), Clock: "market"}
	for index, location := range locations {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		query.LocationID = location.ID
		rows, err := provider.Fetch(ctx, apiKey, query)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil || len(rows) == 0 {
			result.failed++
			if err == nil {
				err = errors.New("provider returned no intervals")
			}
			fmt.Fprintf(progress, "location %q failed: %v\n", location.ID, err)
			continue
		}
		first := rows[0]
		locations[index] = data.Location{ID: location.ID, Name: inferLocationName(location.ID, location.Name), Type: first.LocationType, Market: first.Market, DatasetID: options.dataset}
		result.updated++
	}
	if result.updated == 0 {
		return result, errors.New("all location refreshes failed; catalog unchanged")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := rejectOutputSymlink(path); err != nil {
		return result, err
	}
	list := &data.LocationList{DatasetID: options.dataset, UpdatedAt: now.UTC().Format(time.RFC3339), Locations: locations}
	if err := data.SaveLocations(list, path); err != nil {
		return result, err
	}
	if result.failed != 0 {
		result.status = "partial"
		return result, errors.New("partial location refresh; failed entries were preserved")
	}
	result.status = "complete"
	return result, nil
}

func loadRefreshLocations(destination, seed, dataset string) ([]data.Location, error) {
	base, err := data.LoadLocations(destination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil && seed == "" {
		return nil, errors.New("missing destination requires an explicit seed catalog")
	}
	locations := make(map[string]data.Location)
	merge := func(list *data.LocationList) error {
		if list.DatasetID != dataset {
			return fmt.Errorf("catalog dataset %q does not match requested dataset %q", list.DatasetID, dataset)
		}
		for _, location := range list.Locations {
			if _, exists := locations[location.ID]; !exists {
				locations[location.ID] = location
			}
		}
		return nil
	}
	if base != nil {
		if err := merge(base); err != nil {
			return nil, err
		}
	}
	if seed != "" {
		additional, err := data.LoadLocations(seed)
		if err != nil {
			return nil, fmt.Errorf("seed: %w", err)
		}
		if len(additional.Locations) == 0 {
			return nil, errors.New("seed catalog must contain at least one location")
		}
		if err := merge(additional); err != nil {
			return nil, err
		}
	}
	if len(locations) == 0 {
		return nil, errors.New("empty destination requires an explicit nonempty seed catalog")
	}
	result := make([]data.Location, 0, len(locations))
	for _, location := range locations {
		result = append(result, location)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func inferLocationName(id, previous string) string {
	if previous != "" {
		return previous
	}
	switch id {
	case "MOSSLD_2_PSP1":
		return "Moss Landing"
	case "SV_LNODER6A":
		return "Flagstaff"
	default:
		return id
	}
}
