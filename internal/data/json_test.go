package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"battery-backtest/internal/model"
)

func jsonRow(t *testing.T) map[string]any {
	t.Helper()
	row := testInterval("2026-01-01T00:00:00-07:00", 30*time.Minute, 0)
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func payload(t *testing.T, rows ...map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"status_code": 200, "data": rows})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestDecodeGridStatusJSONPreservesPricesAndInputOrder(t *testing.T) {
	first := jsonRow(t)
	second := jsonRow(t)
	first["lmp"] = -45.75
	first["energy"] = -50
	first["interval_start_utc"] = "2026-01-01T08:00:00Z"
	first["interval_end_utc"] = "2026-01-01T08:15:00Z"
	first["interval_start_local"] = "2026-01-01T01:00:00-07:00"
	first["interval_end_local"] = "2026-01-01T01:15:00-07:00"
	first["provider_metadata"] = "allowed"
	rows, err := DecodeGridStatusJSON(strings.NewReader(payload(t, first, second)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].LMP != -45.75 || rows[0].Energy != -50 || rows[1].LMP != 0 || !rows[0].IntervalStartUTC.After(rows[1].IntervalStartUTC) {
		t.Fatalf("decoder changed supplied rows: %+v", rows)
	}
	normalized, err := NormalizeSeries(rows, "market")
	if err != nil || len(normalized) != 2 || normalized[0].LMP != 0 || normalized[1].Duration() != 15*time.Minute {
		t.Fatalf("decode/normalize integration: %+v, %v", normalized, err)
	}
}

func TestDecodeGridStatusJSONRequiredFieldsAndNulls(t *testing.T) {
	for _, field := range []string{"lmp", "interval_start_utc", "interval_end_utc"} {
		for _, missing := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/missing=%t", field, missing), func(t *testing.T) {
				invalid := jsonRow(t)
				if missing {
					delete(invalid, field)
				} else {
					invalid[field] = nil
				}
				rows, err := DecodeGridStatusJSON(strings.NewReader(payload(t, jsonRow(t), invalid)))
				if rows != nil {
					t.Fatal("decoder skipped the invalid second row or returned partial success")
				}
				requireValidation(t, err, "intervals[1]."+field, "")
			})
		}
	}
	for _, field := range []string{"energy", "congestion", "loss", "ghg", "interval_start_local", "interval_end_local"} {
		t.Run(field+" null", func(t *testing.T) {
			row := jsonRow(t)
			row[field] = nil
			_, err := DecodeGridStatusJSON(strings.NewReader(payload(t, row)))
			requireValidation(t, err, "intervals[0]."+field, "null")
		})
	}
}

func TestDecodeGridStatusJSONOptionalFields(t *testing.T) {
	row := jsonRow(t)
	for _, field := range []string{"energy", "congestion", "loss", "ghg", "interval_start_local", "interval_end_local", "location_type"} {
		delete(row, field)
	}
	rows, err := DecodeGridStatusJSON(strings.NewReader(payload(t, row)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeSeries(rows, "UTC"); err != nil {
		t.Fatal(err)
	}
	_, err = NormalizeSeries(rows, "market")
	requireValidation(t, err, "intervals[0].interval_start_local", "required")
}

func TestDecodeGridStatusJSONTimestampErrors(t *testing.T) {
	for _, field := range []string{"interval_start_utc", "interval_end_utc", "interval_start_local", "interval_end_local"} {
		for _, value := range []string{
			"not-a-date", "2026-02-30T00:00:00Z", "2026-01-01T00:00:00", "0001-01-01T00:00:00Z",
			"2026-01-01T1:00:00Z", "2026-01-01T00:00:00,5Z", "2026-01-01T00:00:00+24:00", "2026-01-01T00:00:00+05:60",
		} {
			t.Run(field+"/"+value, func(t *testing.T) {
				row := jsonRow(t)
				row[field] = value
				_, err := DecodeGridStatusJSON(strings.NewReader(payload(t, row)))
				requireValidation(t, err, "intervals[0]."+field, "RFC3339")
			})
		}
	}
}

func TestDecodeGridStatusJSONIgnoresSystemTimezone(t *testing.T) {
	original := time.Local
	time.Local = time.FixedZone("system-local", -7*60*60)
	t.Cleanup(func() { time.Local = original })
	rows, err := DecodeGridStatusJSON(strings.NewReader(payload(t, jsonRow(t))))
	if err != nil {
		t.Fatal(err)
	}
	if zone, offset := rows[0].IntervalStartLocal.Zone(); zone != "" || offset != -7*60*60 {
		t.Fatalf("numeric offset adopted system timezone: zone=%q offset=%d", zone, offset)
	}
}

func TestDecodeGridStatusJSONRejectsOverflowPrices(t *testing.T) {
	for _, field := range []string{"lmp", "energy", "congestion", "loss", "ghg"} {
		for _, value := range []string{"1e309", "-1e309"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				row := jsonRow(t)
				row[field] = "REPLACE_NUMERIC"
				input := strings.Replace(payload(t, row), `"REPLACE_NUMERIC"`, value, 1)
				_, err := DecodeGridStatusJSON(strings.NewReader(input))
				requireValidation(t, err, "intervals[0]."+field, "finite")
			})
		}
	}
}

func TestDecodeGridStatusJSONKeepsSyntaxAndTypeErrors(t *testing.T) {
	_, err := DecodeGridStatusJSON(strings.NewReader(`{"data":[?]}`))
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("syntax error not preserved: %T %v", err, err)
	}
	for _, field := range []string{"lmp", "interval_start_utc", "location"} {
		row := jsonRow(t)
		row[field] = []int{1, 2}
		_, err := DecodeGridStatusJSON(strings.NewReader(payload(t, row)))
		var mismatch *json.UnmarshalTypeError
		if !errors.As(err, &mismatch) || !strings.Contains(err.Error(), "intervals[0]") || !strings.Contains(err.Error(), field) {
			t.Fatalf("type error field/row lost for %s: %T %v", field, err, err)
		}
	}
}

func TestDecodeGridStatusJSONEnvelopeAndTrailingInput(t *testing.T) {
	for _, input := range []string{"null", "{}", `{"data":null}`} {
		_, err := DecodeGridStatusJSON(strings.NewReader(input))
		requireValidation(t, err, "intervals", "missing or null")
	}
	for _, input := range []string{"", `[]`, `{"data":{}}`, `{"data":[42]}`, `{"data":[]} {}`, `{"data":[]} trailing`, `{"data":[NaN]}`} {
		if rows, err := DecodeGridStatusJSON(strings.NewReader(input)); err == nil || rows != nil {
			t.Fatalf("accepted invalid response %s", input)
		}
	}
	_, err := DecodeGridStatusJSON(strings.NewReader(`{"data":[null]}`))
	requireValidation(t, err, "intervals[0]", "object")
	rows, err := DecodeGridStatusJSON(strings.NewReader("{\"data\":[]} \n\t"))
	if err != nil || len(rows) != 0 {
		t.Fatalf("empty valid payload should reach normalization: %+v %v", rows, err)
	}
	_, err = NormalizeSeries(rows, "UTC")
	requireValidation(t, err, "intervals", "at least one")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDecodeGridStatusJSONReaderError(t *testing.T) {
	_, err := DecodeGridStatusJSON(failingReader{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("reader error not preserved: %v", err)
	}
}

func TestLoadGridStatusJSONUsesCompleteDecoder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "series.json")
	input := payload(t, jsonRow(t))
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	response, err := LoadGridStatusJSON(path)
	if err != nil || response.StatusCode != 200 || len(response.Data) != 1 || response.Data[0].LMP != 0 {
		t.Fatalf("file response = %+v, %v", response, err)
	}
	invalid := jsonRow(t)
	delete(invalid, "lmp")
	if err := os.WriteFile(path, []byte(payload(t, jsonRow(t), invalid)), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadGridStatusJSON(path)
	requireValidation(t, err, "intervals[1].lmp", "missing")
	_, err = LoadGridStatusJSON(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error not preserved: %v", err)
	}
}

func TestGroupByLocationRetainsAllRows(t *testing.T) {
	first := testInterval("2026-01-01T00:00:00-07:00", time.Hour, 0)
	second := first
	second.Location = "NODE_B"
	groups := GroupByLocation(&model.GridStatusLMPResponse{Data: []model.LMPInterval{first, second, first}})
	if !reflect.DeepEqual(groups[first.Location], []model.LMPInterval{first, first}) || !reflect.DeepEqual(groups[second.Location], []model.LMPInterval{second}) {
		t.Fatalf("grouping changed rows: %+v", groups)
	}
	if len(GroupByLocation(nil)) != 0 {
		t.Fatal("nil response should produce no groups")
	}
}
