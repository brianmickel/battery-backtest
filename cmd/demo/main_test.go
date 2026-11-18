package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
)

func demoFixture(t *testing.T) (string, string) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/single-request.json")
	if err != nil {
		t.Fatal(err)
	}
	var request struct{ Config config.Input }
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	initial := .5
	request.Config.Battery.InitialSOC = &initial
	raw, err = json.Marshal(request.Config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	dataPath, err := filepath.Abs("../../testdata/two-price.json")
	if err != nil {
		t.Fatal(err)
	}
	return path, dataPath
}

func TestDemoSharedRunnerParityAndZeroLimit(t *testing.T) {
	configPath, dataPath := demoFixture(t)
	outPath := filepath.Join(t.TempDir(), "nested", "actual.csv")
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"--config", configPath, "--data", dataPath, "--out", outPath, "--n", "0", "--timezone", "UTC"}, &stdout); err != nil {
		t.Fatal(err)
	}
	input, err := config.LoadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := experiment.Prepare(input, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	response, err := data.LoadGridStatusJSON(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := experiment.Run(context.Background(), prepared, response.Data, experiment.Options{Clock: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	expectedPath := filepath.Join(t.TempDir(), "expected.csv")
	if err := backtest.WriteLedgerCSV(expectedPath, outcome.Ledger); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) || !strings.Contains(stdout.String(), "Loaded 2 intervals") || !strings.Contains(stdout.String(), "Starting SOC=0.500") || !strings.Contains(stdout.String(), "Total PnL=$95.00") || !strings.Contains(stdout.String(), "zero salvage value") {
		t.Fatalf("demo parity failed: %s\nCSV: %s", stdout.String(), actual)
	}
}

func TestDemoProcess(t *testing.T) {
	if os.Getenv("BATTERY_DEMO_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"demo"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}
func demoProcess(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=TestDemoProcess", "--"}, args...)...)
	command.Env = append(os.Environ(), "BATTERY_DEMO_TEST_PROCESS=1")
	command.Dir = t.TempDir()
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
func TestDemoFailuresAndHelp(t *testing.T) {
	configPath, dataPath := demoFixture(t)
	malformed := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(malformed, []byte(`{"data":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"missing_config":   {},
		"negative_limit":   {"--config", configPath, "--n", "-1"},
		"bad_data":         {"--config", configPath, "--data", malformed},
		"output_directory": {"--config", configPath, "--data", dataPath, "--out", t.TempDir()},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := demoProcess(t, args...)
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() == 0 || stdout != "" || !strings.Contains(stderr, "error:") || strings.Contains(stderr, "panic:") {
				t.Fatalf("failure: %s %s %v", stdout, stderr, err)
			}
		})
	}
	stdout, stderr, err := demoProcess(t, "--help")
	if err != nil || stderr != "" || !strings.Contains(stdout, "usage:") {
		t.Fatalf("help: %s %s %v", stdout, stderr, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, []string{"--config", configPath, "--data", dataPath}, &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestDemoLimitAppliesBeforePlanningAndValidatesWholeSeries(t *testing.T) {
	configPath, dataPath := demoFixture(t)
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"--config", configPath, "--data", dataPath, "--n", "1"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Loaded 1 intervals") || !strings.Contains(stdout.String(), "Total PnL=$5.00") {
		t.Fatalf("planned past the selected prefix: %s", stdout.String())
	}
	response, err := data.LoadGridStatusJSON(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	response.Data[1] = response.Data[0]
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := run(context.Background(), []string{"--config", configPath, "--data", path, "--n", "1"}, &stdout); err == nil || stdout.Len() != 0 {
		t.Fatalf("limit hid invalid suffix: %v %s", err, stdout.String())
	}
}
