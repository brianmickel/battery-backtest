package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"battery-backtest/internal/api"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	batteryDir := os.Getenv("BATTERY_DIR")
	if batteryDir == "" {
		batteryDir = "examples/batteries"
	}
	presets, err := config.LoadPresetCatalog(batteryDir)
	if err != nil {
		return fmt.Errorf("load preset catalog: %w", err)
	}
	provider, err := data.NewGridStatusClient(nil, "")
	if err != nil {
		return err
	}
	frontend := os.Getenv("STATIC_DIR")
	if frontend == "" {
		if _, err := os.Stat(filepath.Join("web", "dist", "index.html")); err == nil {
			frontend = filepath.Join("web", "dist")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("frontend index: %w", err)
		}
	}
	var origins []string
	for _, origin := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	router, err := api.NewRouter(api.Dependencies{Provider: provider, Presets: presets, LocationsPath: data.GetDefaultLocationsPath(), FrontendDir: frontend, AllowedOrigins: origins})
	if err != nil {
		return err
	}
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return serve(ctx, newHTTPServer(router), listener)
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 65 * time.Second, IdleTimeout: 60 * time.Second}
}

func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	server.BaseContext = func(net.Listener) context.Context { return ctx }
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
		err := <-result
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
