package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Location struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Market    string `json:"market"`
	DatasetID string `json:"dataset_id"`
}

type LocationList struct {
	DatasetID string     `json:"dataset_id"`
	UpdatedAt string     `json:"updated_at"`
	Locations []Location `json:"locations"`
}

func LoadLocations(path string) (*LocationList, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read locations file: %w", err)
	}
	var list LocationList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parse locations file: %w", err)
	}
	if err := validateLocations(&list); err != nil {
		return nil, err
	}
	return &list, nil
}

func validateLocations(list *LocationList) error {
	if list == nil || strings.TrimSpace(list.DatasetID) == "" {
		return errors.New("locations catalog requires a dataset_id")
	}
	if list.Locations == nil {
		return errors.New("locations catalog requires a locations array")
	}
	seen := make(map[string]bool, len(list.Locations))
	for index := range list.Locations {
		location := &list.Locations[index]
		if strings.TrimSpace(location.ID) == "" || seen[location.ID] {
			return errors.New("locations catalog contains an empty or duplicate location ID")
		}
		seen[location.ID] = true
		if location.DatasetID == "" {
			location.DatasetID = list.DatasetID
		}
		if location.DatasetID != list.DatasetID {
			return fmt.Errorf("location %q has a conflicting dataset_id", location.ID)
		}
	}
	return nil
}

// SaveLocations sorts a copy and replaces the catalog only after writing and
// closing a random, exclusively created temporary file in the same directory.
func SaveLocations(list *LocationList, path string) error {
	if list == nil {
		return errors.New("locations catalog is required")
	}
	copy := *list
	copy.Locations = append([]Location{}, list.Locations...)
	if list.Locations == nil {
		copy.Locations = nil
	}
	if err := validateLocations(&copy); err != nil {
		return err
	}
	sort.Slice(copy.Locations, func(i, j int) bool { return copy.Locations[i].ID < copy.Locations[j].ID })
	raw, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal locations: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create locations directory: %w", err)
	}
	return writeLocationsAtomically(path, append(raw, '\n'), func(dir, pattern string) (locationTempFile, error) { return os.CreateTemp(dir, pattern) }, os.Rename)
}

type locationTempFile interface {
	io.WriteCloser
	Name() string
}

func writeLocationsAtomically(path string, raw []byte, create func(string, string) (locationTempFile, error), rename func(string, string) error) error {
	file, err := create(filepath.Dir(path), ".battery-backtest-*")
	if err != nil {
		return fmt.Errorf("create temporary locations file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if count, err := file.Write(raw); err != nil {
		return fmt.Errorf("write temporary locations file: %w", err)
	} else if count != len(raw) {
		return fmt.Errorf("write temporary locations file: %w", io.ErrShortWrite)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary locations file: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect locations destination: %w", err)
	}
	if info != nil && !info.Mode().IsRegular() {
		return errors.New("locations destination must be a regular file, not a symlink or directory")
	}
	if err := rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace locations file: %w", err)
	}
	return nil
}

func GetDefaultLocationsPath() string {
	if path := os.Getenv("LOCATIONS_FILE"); path != "" {
		return path
	}
	return "./data/locations.json"
}
