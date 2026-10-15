package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/analysis"
	"battery-backtest/internal/backtest"
	"battery-backtest/internal/config"
	"battery-backtest/internal/data"
	"battery-backtest/internal/experiment"
	"battery-backtest/internal/model"
)

func commandFixture(t *testing.T) (string, string, config.Input) {
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
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	dataPath, err := filepath.Abs("../../testdata/two-price.json")
	if err != nil {
		t.Fatal(err)
	}
	return configPath, dataPath, request.Config
}

func TestCLIBacktestLedgerAndSummaryMatchSharedExecution(t *testing.T) {
	configPath, dataPath, input := commandFixture(t)
	outPath := filepath.Join(t.TempDir(), "nested", "actual.csv")
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"backtest", "--config", configPath, "--data", dataPath, "--out", outPath, "--n", "0", "--timezone", "UTC"}, &stdout); err != nil {
		t.Fatal(err)
	}
	response, err := data.LoadGridStatusJSON(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := experiment.Prepare(input, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := experiment.Run(context.Background(), prepared, response.Data, experiment.Options{Clock: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	ledger := readLedger(t, outPath)
	if !reflect.DeepEqual(ledger, want.Ledger) {
		t.Fatalf("CLI ledger differs from shared execution:\n%+v\n%+v", ledger, want.Ledger)
	}
	resolved, err := config.Resolve(input, config.PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := backtest.Summarize(backtest.Result{Ledger: ledger}, resolved.Battery.ToModelParams(), "UTC")
	if err != nil || !reflect.DeepEqual(summary, want.Summary) {
		t.Fatalf("CLI summary differs: %+v, %+v, %v", summary, want.Summary, err)
	}
	if !strings.Contains(stdout.String(), "Total PnL=$95.00") || !strings.Contains(stdout.String(), "Initial SOC=0.500") || !strings.Contains(stdout.String(), "zero salvage value") {
		t.Fatalf("missing inventory semantics: %s", stdout.String())
	}
}

func readLedger(t *testing.T, path string) []backtest.LedgerRow {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]backtest.LedgerRow, len(records)-1)
	for i, record := range records[1:] {
		row := &rows[i]
		row.Index, err = strconv.Atoi(record[0])
		if err != nil {
			t.Fatal(err)
		}
		for j, field := range []*time.Time{&row.IntervalStartLocal, &row.IntervalEndLocal, &row.IntervalStartUTC, &row.IntervalEndUTC} {
			*field, err = time.Parse(time.RFC3339, record[j+1])
			if err != nil {
				t.Fatal(err)
			}
		}
		row.Location, row.Market, row.Action = record[5], record[6], model.Action(record[8])
		for j, field := range map[int]*float64{7: &row.LMP, 9: &row.RequestedPowerMW, 10: &row.PowerMW, 11: &row.EnergyFromGridMWh, 12: &row.EnergyToGridMWh, 13: &row.ThroughputMWh, 14: &row.SOCStart, 15: &row.SOCEnd, 16: &row.PNL, 17: &row.CumPNL} {
			*field, err = strconv.ParseFloat(record[j], 64)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return rows
}

func TestCLINormalizesBeforeLimitAndRejectsInvalidSuffix(t *testing.T) {
	configPath, dataPath, _ := commandFixture(t)
	response, err := data.LoadGridStatusJSON(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	response.Data[0], response.Data[1] = response.Data[1], response.Data[0]
	path := filepath.Join(t.TempDir(), "unsorted.json")
	writeData := func() {
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeData()
	outPath := filepath.Join(t.TempDir(), "limited.csv")
	args := []string{"backtest", "--config", configPath, "--data", path, "--out", outPath, "--n", "1"}
	if err := run(context.Background(), args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	ledger := readLedger(t, outPath)
	if len(ledger) != 1 || ledger[0].LMP != 10 {
		t.Fatalf("wrong chronological prefix: %+v", ledger)
	}
	response.Data[0] = response.Data[1]
	writeData()
	var stdout bytes.Buffer
	if err := run(context.Background(), args, &stdout); err == nil || stdout.Len() != 0 {
		t.Fatalf("invalid suffix hidden by limit: %v %s", err, stdout.String())
	}
}

func TestCLIRankCanonicalVariableDurationsAndDeterministicTies(t *testing.T) {
	response, err := data.LoadGridStatusJSON("../../testdata/variable-gap.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []model.LMPInterval
	for _, id := range []string{"b", "a"} {
		for i := len(response.Data) - 1; i >= 0; i-- {
			row := response.Data[i]
			row.Location = id
			rows = append(rows, row)
		}
	}
	path := filepath.Join(t.TempDir(), "locations.json")
	raw, _ := json.Marshal(model.GridStatusLMPResponse{Data: rows})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	want, err := analysis.RankByOracleProfit(context.Background(), data.GroupByLocation(&model.GridStatusLMPResponse{Data: rows}))
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run(context.Background(), []string{"rank", "--data", filepath.Dir(path)}, &stdout); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(stdout.String(), "\n")
	for i, potential := range want {
		fields := strings.Fields(lines[i+1])
		if fields[1] != potential.Location {
			t.Fatalf("rank order: %s", stdout.String())
		}
		score, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil || score != potential.OracleProfit {
			t.Fatalf("score %v want %v", score, potential.OracleProfit)
		}
	}
	if want[0].OracleProfit != 42.5 {
		t.Fatalf("variable-duration score = %v", want[0].OracleProfit)
	}
}

func TestCLIProcess(t *testing.T) {
	if os.Getenv("BATTERY_CLI_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"cli"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}
func cliProcess(t *testing.T, cwd string, args ...string) (string, string, error) {
	t.Helper()
	command := exec.Command(os.Args[0], append([]string{"-test.run=TestCLIProcess", "--"}, args...)...)
	command.Env = append(os.Environ(), "BATTERY_CLI_TEST_PROCESS=1")
	command.Dir = cwd
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func TestCLIProcessFailuresAndConfigRelativePaths(t *testing.T) {
	configPath, dataPath, _ := commandFixture(t)
	unrelated := t.TempDir()
	battery := "battery:\n  energy_capacity_mwh: 1\n  power_capacity_mw: 1\n  charge_efficiency: 1\n  discharge_efficiency: 1\n  min_soc: 0\n  max_soc: 1\n  initial_soc: 0.5\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(configPath), "battery.yaml"), []byte(battery), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("battery_file: battery.yaml\nstrategy:\n  name: oracle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "result.csv")
	stdout, stderr, err := cliProcess(t, unrelated, "backtest", "--config", configPath, "--data", dataPath, "--out", outPath)
	if err != nil || stderr != "" || !strings.Contains(stdout, "Total PnL=$95.00") {
		t.Fatalf("config-relative execution: %s %s %v", stdout, stderr, err)
	}
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"data":[`), 0600); err != nil {
		t.Fatal(err)
	}
	invalidConfig := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidConfig, []byte("strategy:\n  name: unsupported\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"invalid_config":   {"backtest", "--config", invalidConfig, "--data", dataPath},
		"missing_config":   {"backtest"},
		"negative_limit":   {"backtest", "--config", configPath, "--data", dataPath, "--n", "-1"},
		"bad_clock":        {"backtest", "--config", configPath, "--timezone", "local"},
		"malformed_data":   {"backtest", "--config", configPath, "--data", malformed},
		"output_directory": {"backtest", "--config", configPath, "--data", dataPath, "--out", unrelated},
		"empty_rank":       {"rank", "--data", unrelated},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := cliProcess(t, unrelated, args...)
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() == 0 || stdout != "" || !strings.Contains(stderr, "error:") || strings.Contains(stderr, "panic:") {
				t.Fatalf("failure not concise/nonzero: stdout=%s stderr=%s err=%v", stdout, stderr, err)
			}
		})
	}
	stdout, stderr, err = cliProcess(t, unrelated, "--help")
	if err != nil || stderr != "" || !strings.Contains(stdout, "usage:") {
		t.Fatalf("help failed: %s %s %v", stdout, stderr, err)
	}
}

func TestCLICanceledWork(t *testing.T) {
	configPath, dataPath, _ := commandFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	if err := run(ctx, []string{"backtest", "--config", configPath, "--data", dataPath}, &stdout); !errors.Is(err, context.Canceled) || stdout.Len() != 0 {
		t.Fatalf("cancel: %v %s", err, stdout.String())
	}
}
