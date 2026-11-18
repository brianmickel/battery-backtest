package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
)

func TestCLIAmbiguousMidnightFailsBeforeOutput(t *testing.T) {
	configPath, dataPath, _ := commandFixture(t)
	response, err := data.LoadGridStatusJSON(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	response.Data = response.Data[:1]
	row := &response.Data[0]
	row.IntervalStartLocal = time.Date(2024, 11, 2, 0, 0, 0, 0, time.FixedZone("", -7*3600))
	row.IntervalEndLocal = time.Date(2024, 11, 4, 0, 0, 0, 0, time.FixedZone("", -8*3600))
	row.IntervalStartUTC, row.IntervalEndUTC = row.IntervalStartLocal.UTC(), row.IntervalEndLocal.UTC()
	row.LMP = -10
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	dataPath = filepath.Join(t.TempDir(), "ambiguous.json")
	if err := os.WriteFile(dataPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "result.csv")
	args := []string{"backtest", "--config", configPath, "--data", dataPath, "--out", outPath}
	var stdout bytes.Buffer
	err = run(context.Background(), args, &stdout)
	var invalid *experiment.ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "intervals[0].interval_end_local" || stdout.Len() != 0 {
		t.Fatalf("validation error=%v stdout=%s", err, stdout.String())
	}
	if _, err := os.Stat(outPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid run created output: %v", err)
	}
	processOut, stderr, err := cliProcess(t, t.TempDir(), args...)
	if err == nil || processOut != "" || !strings.Contains(stderr, "intervals[0].interval_end_local") || !strings.Contains(stderr, "split the interval at midnight or select UTC") {
		t.Fatalf("process error=%v stdout=%s stderr=%s", err, processOut, stderr)
	}
	if err := run(context.Background(), append(args, "--timezone", "UTC"), &stdout); err != nil {
		t.Fatalf("UTC run failed: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("UTC run did not write output: %v", err)
	}
}
