package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
	"github.com/gin-gonic/gin"
)

type Provider interface {
	Fetch(context.Context, string, data.Query) ([]model.LMPInterval, error)
}
type Dependencies struct {
	Provider       Provider
	Presets        config.PresetCatalog
	LocationsPath  string
	FrontendDir    string
	AllowedOrigins []string
}

type server struct{ deps Dependencies }

func NewRouter(deps Dependencies) (*gin.Engine, error) {
	if deps.Provider == nil {
		return nil, errors.New("provider is required")
	}
	if len(deps.Presets.List()) == 0 {
		return nil, errors.New("a nonempty preset catalog is required")
	}
	for _, origin := range deps.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, errors.New("CORS origins must be exact HTTP(S) origins")
		}
	}
	if deps.FrontendDir != "" {
		index, err := os.Stat(filepath.Join(deps.FrontendDir, "index.html"))
		if err != nil {
			return nil, fmt.Errorf("frontend index: %w", err)
		}
		if !index.Mode().IsRegular() {
			return nil, errors.New("frontend index must be a regular file")
		}
	}
	deps.AllowedOrigins = append([]string(nil), deps.AllowedOrigins...)
	s := server{deps: deps}
	router := gin.New()
	router.Use(accessAndRecovery(), cors(deps.AllowedOrigins))
	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	routes := router.Group("/api/v1")
	routes.POST("/backtest", s.backtest)
	routes.POST("/backtest/compare", s.compare)
	routes.GET("/rank", s.rank)
	routes.GET("/batteries", s.batteries)
	routes.GET("/strategies", s.strategies)
	routes.GET("/datasets", s.datasets)
	routes.GET("/locations", s.locations)
	router.NoRoute(func(c *gin.Context) {
		if deps.FrontendDir == "" || strings.HasPrefix(c.Request.URL.Path, "/api") || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			writeFailure(c, failure{status: 404, code: "NOT_FOUND", message: "Route not found."})
			return
		}
		relative := strings.TrimPrefix(filepath.Clean("/"+c.Request.URL.Path), "/")
		path := filepath.Join(deps.FrontendDir, relative)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			c.File(path)
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			writeFailure(c, failure{status: 404, code: "NOT_FOUND", message: "Asset not found."})
			return
		}
		c.File(filepath.Join(deps.FrontendDir, "index.html"))
	})
	return router, nil
}
