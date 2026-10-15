package backtest

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

func TestLedgerCSVGolden(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	row := LedgerRow{Index: 0, IntervalStartLocal: start, IntervalEndLocal: end, IntervalStartUTC: start, IntervalEndUTC: end, Location: "node,one", Market: "market", LMP: 10, Action: model.ActionCharging, RequestedPowerMW: -1, PowerMW: -1, EnergyFromGridMWh: .5, ThroughputMWh: .5, SOCStart: .25, SOCEnd: .75, PNL: -5, CumPNL: -5}
	var buffer bytes.Buffer
	if err := writeLedgerCSV(&buffer, []LedgerRow{row}); err != nil {
		t.Fatal(err)
	}
	want := "index,interval_start_local,interval_end_local,interval_start_utc,interval_end_utc,location,market,lmp,action,requested_power_mw,power_mw,energy_from_grid_mwh,energy_to_grid_mwh,throughput_mwh,soc_start,soc_end,pnl,cum_pnl\n" +
		"0,2025-01-01T00:00:00Z,2025-01-01T00:30:00Z,2025-01-01T00:00:00Z,2025-01-01T00:30:00Z,\"node,one\",market,10.000000,CHARGING,-1.000000,-1.000000,0.500000,0.000000,0.500000,0.250000,0.750000,-5.000000,-5.000000\n"
	if buffer.String() != want {
		t.Fatalf("CSV changed:\n%s\nwant:\n%s", buffer.String(), want)
	}
}

type failingOutput struct {
	writeErr, closeErr error
	closed             bool
	calls              int
}

func (w *failingOutput) Write(p []byte) (int, error) {
	w.calls++
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(p), nil
}
func (w *failingOutput) Close() error { w.closed = true; return w.closeErr }
func TestLedgerCSVPropagatesWriteFlushAndCloseErrors(t *testing.T) {
	writeFailure, closeFailure := errors.New("write failed"), errors.New("close failed")
	for _, tc := range []struct {
		name               string
		rows               []LedgerRow
		writeErr, closeErr error
	}{
		{"flush", nil, writeFailure, nil},
		{"write", []LedgerRow{{Location: strings.Repeat("x", 8192)}}, writeFailure, nil},
		{"close", nil, nil, closeFailure},
		{"both", nil, writeFailure, closeFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &failingOutput{writeErr: tc.writeErr, closeErr: tc.closeErr}
			err := writeLedgerCSVAndClose(writer, tc.rows)
			if !writer.closed || writer.calls == 0 {
				t.Fatal("writer was not flushed and closed")
			}
			if tc.writeErr != nil && !errors.Is(err, tc.writeErr) {
				t.Fatalf("write error hidden: %v", err)
			}
			if tc.closeErr != nil && !errors.Is(err, tc.closeErr) {
				t.Fatalf("close error hidden: %v", err)
			}
		})
	}
}

type shortOutput struct{}

func (shortOutput) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestLedgerCSVShortWriteFails(t *testing.T) {
	if err := writeLedgerCSV(shortOutput{}, nil); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
}
