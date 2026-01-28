package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"battery-backtest/internal/api/handlers"
	"battery-backtest/internal/api/middleware"

	"github.com/gin-gonic/gin"
)

func main() {
	// Get configuration from environment
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	// Log working directory and important paths for debugging
	wd, err := os.Getwd()
	if err == nil {
		log.Printf("Working directory: %s", wd)
		// Check if examples/batteries exists
		batteryDir := filepath.Join(wd, "examples", "batteries")
		if info, err := os.Stat(batteryDir); err == nil && info.IsDir() {
			log.Printf("Battery directory found: %s", batteryDir)
		} else {
			log.Printf("Battery directory not found at: %s (error: %v)", batteryDir, err)
		}
	}

	// Note: API key is now passed through from client requests
	// We no longer need a server-side API key for Grid Status

	// Set up Gin router
	if os.Getenv("API_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// Apply middleware
	router.Use(middleware.CORS())
	router.Use(middleware.Logger())
	router.Use(middleware.ErrorHandler())

	// Initialize handlers
	backtestHandler := handlers.NewBacktestHandler(nil)
	batteryHandler := handlers.NewBatteryHandler()
	strategyHandler := handlers.NewStrategyHandler()
	rankHandler := handlers.NewRankHandler(nil)

	// Store batteryHandler reference for debug endpoint
	_ = batteryHandler // Keep reference for debug endpoint below

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Diagnostic endpoint to check battery directory
	router.GET("/debug/battery-dir", func(c *gin.Context) {
		wd, _ := os.Getwd()
		batteryDir := batteryHandler.GetBatteryDir()
		info, statErr := os.Stat(batteryDir)
		
		var entries []string
		var entryDetails []map[string]interface{}
		if dirEntries, err := os.ReadDir(batteryDir); err == nil {
			for _, e := range dirEntries {
				entries = append(entries, e.Name())
				entryInfo, _ := e.Info()
				entryDetails = append(entryDetails, map[string]interface{}{
					"name":  e.Name(),
					"is_dir": e.IsDir(),
					"size":   entryInfo.Size(),
				})
			}
		}
		
		c.JSON(200, gin.H{
			"working_directory": wd,
			"battery_dir": batteryDir,
			"battery_dir_exists": statErr == nil,
			"battery_dir_is_dir": info != nil && info.IsDir(),
			"stat_error": func() string {
				if statErr != nil {
					return statErr.Error()
				}
				return ""
			}(),
			"entries": entries,
			"entry_details": entryDetails,
			"entry_count": len(entries),
		})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		api.POST("/backtest", backtestHandler.RunBacktest)
		api.GET("/backtest/:id/ledger", backtestHandler.GetLedger)
		api.POST("/backtest/compare", backtestHandler.CompareBacktests)

		api.GET("/batteries", batteryHandler.ListBatteries)
		api.GET("/strategies", strategyHandler.ListStrategies)

		api.GET("/rank", rankHandler.RankNodes)

		api.GET("/datasets", handlers.ListDatasets)
		api.GET("/locations", handlers.ListLocations)
	}

	// Serve static files from web/dist (if it exists)
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./web/dist"
	}
	
	// Check if static directory exists
	if _, err := os.Stat(staticDir); err == nil {
		// Serve static assets
		router.Static("/assets", staticDir+"/assets")
		router.StaticFile("/favicon.ico", staticDir+"/favicon.ico")
		
		// Serve index.html for all non-API routes (SPA routing)
		router.NoRoute(func(c *gin.Context) {
			// Don't serve index.html for API routes
			path := c.Request.URL.Path
			if len(path) >= 4 && path[:4] == "/api" {
				c.JSON(404, gin.H{"error": "Not found"})
			} else {
				c.File(staticDir + "/index.html")
			}
		})
		log.Printf("Serving static files from %s", staticDir)
	} else {
		log.Printf("Static directory %s not found, skipping static file serving", staticDir)
	}

	// Start server
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting API server on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
