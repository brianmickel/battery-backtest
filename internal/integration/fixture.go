// Package integration provides the local synthetic provider used by release tests.
// No fixture endpoint is installed in the production API.
package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"battery-backtest/internal/api"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

const SentinelKey = "synthetic-release-secret"

type Fixture struct {
	Handler       http.Handler
	Started       chan struct{}
	Canceled      chan struct{}
	calls         atomic.Int64
	cancellations atomic.Int64
	provider      *httptest.Server
}

func NewFixture(root, frontend string) (*Fixture, error) {
	raw, err := os.ReadFile(filepath.Join(root, "testdata/two-price.json"))
	if err != nil {
		return nil, err
	}
	var market model.GridStatusLMPResponse
	if err := json.Unmarshal(raw, &market); err != nil {
		return nil, err
	}
	f := &Fixture{Started: make(chan struct{}, 16), Canceled: make(chan struct{}, 16)}
	f.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Header.Get("X-API-Key") != SentinelKey {
			http.Error(w, "provider rejected credential", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("download") != "true" || r.URL.Query().Get("start_time") != "2024-01-01" || r.URL.Query().Get("end_time") != "2024-01-02" {
			http.Error(w, "unexpected provider query", http.StatusBadRequest)
			return
		}
		location := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		switch location {
		case "CANCEL_NODE":
			f.Started <- struct{}{}
			<-r.Context().Done()
			f.cancellations.Add(1)
			f.Canceled <- struct{}{}
			return
		case "MALFORMED_NODE":
			_, _ = fmt.Fprint(w, `{"data":[`)
			return
		case "FAIL_NODE":
			http.Error(w, "synthetic upstream failure", http.StatusServiceUnavailable)
			return
		}
		rows := append([]model.LMPInterval(nil), market.Data...)
		for i := range rows {
			rows[i].Location = location
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.GridStatusLMPResponse{Data: rows})
	}))
	client, err := data.NewGridStatusClient(f.provider.Client(), f.provider.URL)
	if err != nil {
		f.Close()
		return nil, err
	}
	presets, err := config.LoadPresetCatalog(filepath.Join(root, "examples/batteries"))
	if err != nil {
		f.Close()
		return nil, err
	}
	router, err := api.NewRouter(api.Dependencies{Provider: client, Presets: presets, LocationsPath: filepath.Join(root, "data/locations.json"), FrontendDir: frontend})
	if err != nil {
		f.Close()
		return nil, err
	}
	f.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/stats" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int64{"calls": f.calls.Load(), "cancellations": f.cancellations.Load()})
			return
		}
		router.ServeHTTP(w, r)
	})
	return f, nil
}

func (f *Fixture) Close() { f.provider.Close() }

func (f *Fixture) Calls() int64 { return f.calls.Load() }
