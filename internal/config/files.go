package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type presetEntry struct {
	battery BatteryConfig
	patch   BatteryOverride
}

// PresetCatalog is immutable after loading. Its zero value is an empty catalog.
type PresetCatalog struct{ entries map[string]presetEntry }

type Preset struct {
	ID      string
	Battery BatteryConfig
}

func (c PresetCatalog) List() []Preset {
	out := make([]Preset, 0, len(c.entries))
	for id, entry := range c.entries {
		out = append(out, Preset{ID: id, Battery: entry.battery})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func validPresetID(id string) bool {
	if id == "" {
		return false
	}
	for _, ch := range id {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return false
		}
	}
	return true
}

func LoadPresetCatalog(directory string) (PresetCatalog, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return PresetCatalog{}, fmt.Errorf("load preset catalog: %w", err)
	}
	catalog := PresetCatalog{entries: make(map[string]presetEntry)}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".yaml")
		if !validPresetID(id) {
			return PresetCatalog{}, fmt.Errorf("preset %q: invalid ID", entry.Name())
		}
		if !entry.Type().IsRegular() {
			return PresetCatalog{}, fmt.Errorf("preset %q: must be a regular file", id)
		}
		patch, err := readBatteryPatch(filepath.Join(directory, entry.Name()))
		if err != nil {
			return PresetCatalog{}, fmt.Errorf("preset %q: %w", id, err)
		}
		battery, err := resolveBattery(patch)
		if err != nil {
			return PresetCatalog{}, fmt.Errorf("preset %q: %w", id, err)
		}
		catalog.entries[id] = presetEntry{battery: battery, patch: patch}
	}
	return catalog, nil
}

// LoadFile resolves a local battery path solely relative to this configuration file.
// Defaults remain absent so callers can merge additional comparison patches first.
func LoadFile(path string) (Input, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Input{}, fmt.Errorf("load configuration: %w", err)
	}
	var wire inputWire
	if err := decodeYAML(raw, &wire); err != nil {
		return Input{}, err
	}
	input := Input(wire)
	if err := validateDecodedInput(input); err != nil {
		return Input{}, err
	}
	if input.BatteryFile != "" {
		batteryPath := input.BatteryFile
		if !filepath.IsAbs(batteryPath) {
			batteryPath = filepath.Join(filepath.Dir(path), batteryPath)
		}
		patch, err := readBatteryPatch(batteryPath)
		if err != nil {
			return Input{}, fmt.Errorf("battery_file: %w", err)
		}
		input.Battery = mergeBatteryOverride(patch, input.Battery)
		input.BatteryFile = ""
	}
	return input, nil
}

func readBatteryPatch(path string) (BatteryOverride, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BatteryOverride{}, err
	}
	var wrapper struct {
		Battery BatteryOverride `yaml:"battery"`
	}
	if err := decodeYAML(raw, &wrapper); err != nil {
		return BatteryOverride{}, err
	}
	return wrapper.Battery, nil
}
