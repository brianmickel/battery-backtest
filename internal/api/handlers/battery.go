package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"battery-backtest/internal/api/models"
	"battery-backtest/internal/config"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// BatteryHandler handles battery-related requests
type BatteryHandler struct {
	batteryDir string
}

// GetBatteryDir returns the battery directory path (for debugging)
func (h *BatteryHandler) GetBatteryDir() string {
	return h.batteryDir
}

// NewBatteryHandler creates a new battery handler
func NewBatteryHandler() *BatteryHandler {
	dir := os.Getenv("BATTERY_DIR")
	if dir == "" {
		// Try to resolve relative to working directory first
		wd, err := os.Getwd()
		if err == nil {
			dir = filepath.Join(wd, "examples", "batteries")
		} else {
			// Fallback to relative path
			dir = "./examples/batteries"
		}
	}
	
	// Convert to absolute path for reliability
	absDir, err := filepath.Abs(dir)
	if err == nil {
		dir = absDir
	}
	
	log.Printf("BatteryHandler: Using battery directory: %s", dir)
	
	return &BatteryHandler{
		batteryDir: dir,
	}
}

// ListBatteries handles GET /api/v1/batteries
func (h *BatteryHandler) ListBatteries(c *gin.Context) {
	batteries := []models.BatteryInfo{}

	// Log current state for debugging
	log.Printf("BatteryHandler: Attempting to read directory: %s", h.batteryDir)
	
	// Check if directory exists first
	if info, err := os.Stat(h.batteryDir); err != nil {
		log.Printf("BatteryHandler: Directory stat error: %v", err)
		if os.IsNotExist(err) {
			log.Printf("BatteryHandler: Directory does not exist: %s", h.batteryDir)
			// Try to list parent directory
			parentDir := filepath.Dir(h.batteryDir)
			if parentInfo, parentErr := os.Stat(parentDir); parentErr == nil {
				log.Printf("BatteryHandler: Parent directory exists: %s (isDir: %v)", parentDir, parentInfo.IsDir())
				if entries, listErr := os.ReadDir(parentDir); listErr == nil {
					log.Printf("BatteryHandler: Parent directory contents: %v", func() []string {
						names := make([]string, 0, len(entries))
						for _, e := range entries {
							names = append(names, e.Name())
						}
						return names
					}())
				}
			}
		}
		c.JSON(http.StatusOK, gin.H{"batteries": batteries})
		return
	} else {
		log.Printf("BatteryHandler: Directory exists: %s (isDir: %v)", h.batteryDir, info.IsDir())
	}

	// Read battery files from directory
	entries, err := os.ReadDir(h.batteryDir)
	if err != nil {
		log.Printf("BatteryHandler: Failed to read battery directory %s: %v", h.batteryDir, err)
		c.JSON(http.StatusOK, gin.H{"batteries": batteries})
		return
	}

	log.Printf("BatteryHandler: Found %d entries in %s", len(entries), h.batteryDir)
	
	// Log all entries for debugging
	for i, entry := range entries {
		log.Printf("BatteryHandler: Entry[%d]: %s (isDir: %v)", i, entry.Name(), entry.IsDir())
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			log.Printf("BatteryHandler: Skipping entry %s (isDir: %v, hasYaml: %v)", 
				entry.Name(), entry.IsDir(), strings.HasSuffix(entry.Name(), ".yaml"))
			continue
		}

		path := filepath.Join(h.batteryDir, entry.Name())
		log.Printf("BatteryHandler: Loading battery file: %s", path)
		info, err := h.loadBatteryInfo(path, entry.Name())
		if err != nil {
			log.Printf("BatteryHandler: Failed to load battery file %s: %v", path, err)
			continue // Skip invalid files
		}

		log.Printf("BatteryHandler: Successfully loaded battery: %s (ID: %s)", info.Name, info.ID)
		batteries = append(batteries, *info)
	}

	log.Printf("BatteryHandler: Returning %d batteries", len(batteries))

	c.JSON(http.StatusOK, gin.H{"batteries": batteries})
}

func (h *BatteryHandler) loadBatteryInfo(path, filename string) (*models.BatteryInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Battery config.BatteryConfig `yaml:"battery"`
	}
	if err := yaml.Unmarshal(raw, &wrapper); err != nil {
		return nil, err
	}

	// Extract ID from filename (e.g., "1_moss_landing.yaml" -> "1_moss_landing")
	// Keep the full filename without extension as the ID for consistency
	id := strings.TrimSuffix(filename, ".yaml")

	name := wrapper.Battery.Name
	if name == "" {
		name = id
	}

	return &models.BatteryInfo{
		ID:   id,
		Name: name,
		File: path,
		Specs: models.BatterySpecs{
			EnergyCapacityMWh: wrapper.Battery.EnergyCapacityMWh,
			PowerCapacityMW:   wrapper.Battery.PowerCapacityMW,
		},
	}, nil
}
