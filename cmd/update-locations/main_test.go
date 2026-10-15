package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

type fakeLocations func(context.Context, string, data.Query) ([]model.LMPInterval, error)

func (f fakeLocations) Fetch(ctx context.Context, key string, query data.Query) ([]model.LMPInterval, error) {
	return f(ctx, key, query)
}
func refreshFixture(t *testing.T, path string, locations ...data.Location) {
	t.Helper()
	if locations == nil {
		locations = []data.Location{}
	}
	if err := data.SaveLocations(&data.LocationList{DatasetID: "dataset", UpdatedAt: "2020-01-01T00:00:00Z", Locations: locations}, path); err != nil {
		t.Fatal(err)
	}
}
func locationIDs(t *testing.T, path string) []string {
	t.Helper()
	list, err := data.LoadLocations(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(list.Locations))
	for i, location := range list.Locations {
		ids[i] = location.ID
	}
	return ids
}
func refreshRows(query data.Query) []model.LMPInterval {
	return []model.LMPInterval{{Location: query.LocationID, LocationType: "fresh-type", Market: "fresh-market"}}
}
func refreshNow() time.Time {
	return time.Date(2024, 3, 1, 1, 0, 0, 0, time.FixedZone("west", -7*3600))
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestUpdaterReportsOutputFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	refreshFixture(t, path, data.Location{ID: "A"})
	provider := fakeLocations(func(_ context.Context, _ string, query data.Query) ([]model.LMPInterval, error) {
		return refreshRows(query), nil
	})
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"--output", path}, "k", provider, failedOutput{}, &stderr); code != 1 {
		t.Fatalf("output failure returned exit code %d", code)
	}
	if !strings.Contains(stderr.String(), io.ErrClosedPipe.Error()) {
		t.Fatalf("output failure was not reported: %s", &stderr)
	}
}

func TestUpdaterCustomOutputUsesDestinationAndContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.json")
	refreshFixture(t, path, data.Location{ID: "custom-only", Name: "Preserved name"})
	t.Setenv("LOCATIONS_FILE", filepath.Join(t.TempDir(), "must-not-load.json"))
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "value")
	var queries []data.Query
	provider := fakeLocations(func(received context.Context, key string, q data.Query) ([]model.LMPInterval, error) {
		if received.Value(contextKey{}) != "value" || key != "k" {
			t.Error("request context or credential lost")
		}
		queries = append(queries, q)
		return refreshRows(q), nil
	})
	result, err := refreshCatalog(ctx, provider, "k", refreshOptions{dataset: "dataset", output: path, days: 7}, refreshNow(), io.Discard)
	if err != nil || result.status != "complete" || result.updated != 1 || result.failed != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	want := []data.Query{{DatasetID: "dataset", LocationID: "custom-only", StartDate: "2024-02-23", EndDate: "2024-03-01", Clock: "market"}}
	if !reflect.DeepEqual(queries, want) {
		t.Errorf("queries=%#v", queries)
	}
	list, _ := data.LoadLocations(path)
	if list.Locations[0].Name != "Preserved name" || list.Locations[0].Type != "fresh-type" || list.UpdatedAt != "2024-03-01T08:00:00Z" {
		t.Fatalf("saved=%#v", list)
	}
}

func TestUpdaterAdditiveSeedsPartialAndExitStatus(t *testing.T) {
	dir := t.TempDir()
	path, seed := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "seed.json")
	refreshFixture(t, path, data.Location{ID: "A", Name: "Destination metadata", Type: "old"})
	refreshFixture(t, seed, data.Location{ID: "B", Name: "New seed"}, data.Location{ID: "A", Name: "Must not replace"})
	var queried []string
	provider := fakeLocations(func(ctx context.Context, key string, q data.Query) ([]model.LMPInterval, error) {
		queried = append(queried, q.LocationID)
		if q.LocationID == "A" {
			return nil, errors.New("unavailable")
		}
		return refreshRows(q), nil
	})
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--dataset-id", "dataset", "--output", path, "--seed", seed}, "k", provider, &stdout, &stderr)
	if code == 0 || !strings.Contains(stdout.String(), "partial: updated 1/2 locations; 1 failed") || !strings.Contains(stderr.String(), `location "A" failed`) {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	if !reflect.DeepEqual(queried, []string{"A", "B"}) {
		t.Errorf("queried=%v", queried)
	}
	list, _ := data.LoadLocations(path)
	if len(list.Locations) != 2 || list.Locations[0].Name != "Destination metadata" || list.Locations[0].Type != "old" || list.Locations[1].Type != "fresh-type" {
		t.Fatalf("partial catalog=%#v", list)
	}
}

func TestUpdaterAllFailurePreservesBytesAndMissingBootstrap(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{true: "refresh", false: "bootstrap"}[existing], func(t *testing.T) {
			dir := t.TempDir()
			path, seed := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "seed.json")
			refreshFixture(t, seed, data.Location{ID: "A"})
			var before []byte
			var info os.FileInfo
			if existing {
				refreshFixture(t, path, data.Location{ID: "B"})
				before, _ = os.ReadFile(path)
				info, _ = os.Stat(path)
			}
			provider := fakeLocations(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
				return nil, errors.New("all failed")
			})
			var output bytes.Buffer
			code := run(context.Background(), []string{"--dataset-id", "dataset", "--output", path, "--seed", seed}, "k", provider, &output, io.Discard)
			if code == 0 || !strings.Contains(output.String(), "failed:") {
				t.Fatalf("exit=%d output=%s", code, &output)
			}
			after, err := os.ReadFile(path)
			if existing {
				afterInfo, _ := os.Stat(path)
				if !bytes.Equal(before, after) || !afterInfo.ModTime().Equal(info.ModTime()) || !os.SameFile(info, afterInfo) {
					t.Fatal("all-failure changed catalog")
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("bootstrap created output: %v", err)
			}
		})
	}
}

func TestUpdaterRejectsInvalidInputsBeforeProvider(t *testing.T) {
	for _, test := range []struct {
		name, base, seed string
		missing          bool
	}{
		{"missing destination", "", "", true},
		{"malformed destination", "{", "", false},
		{"dataset mismatch", `{"dataset_id":"other","locations":[{"id":"A"}]}`, "", false},
		{"entry dataset mismatch", `{"dataset_id":"dataset","locations":[{"id":"A","dataset_id":"other"}]}`, "", false},
		{"duplicate", `{"dataset_id":"dataset","locations":[{"id":"A"},{"id":"A"}]}`, "", false},
		{"empty ID", `{"dataset_id":"dataset","locations":[{"id":""}]}`, "", false},
		{"empty destination", `{"dataset_id":"dataset","locations":[]}`, "", false},
		{"missing seed", `{"dataset_id":"dataset","locations":[{"id":"A"}]}`, "missing", false},
		{"malformed seed", `{"dataset_id":"dataset","locations":[{"id":"A"}]}`, "{", false},
		{"empty seed", `{"dataset_id":"dataset","locations":[{"id":"A"}]}`, `{"dataset_id":"dataset","locations":[]}`, false},
		{"seed dataset", `{"dataset_id":"dataset","locations":[{"id":"A"}]}`, `{"dataset_id":"other","locations":[{"id":"B"}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "catalog.json")
			seed := ""
			if !test.missing {
				if err := os.WriteFile(path, []byte(test.base), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if test.seed != "" {
				seed = filepath.Join(dir, "seed.json")
				if test.seed != "missing" {
					if err := os.WriteFile(seed, []byte(test.seed), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			calls := 0
			provider := fakeLocations(func(context.Context, string, data.Query) ([]model.LMPInterval, error) { calls++; return nil, nil })
			_, err := refreshCatalog(context.Background(), provider, "k", refreshOptions{dataset: "dataset", output: path, seed: seed, days: 7}, refreshNow(), io.Discard)
			if err == nil || calls != 0 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			after, readErr := os.ReadFile(path)
			if test.missing {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal("created missing catalog")
				}
			} else if string(after) != test.base {
				t.Fatal("invalid catalog changed")
			}
		})
	}
}

func TestUpdaterEmptyDestinationWithSeedAndBlankEntryDataset(t *testing.T) {
	dir := t.TempDir()
	path, seed := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "seed.json")
	refreshFixture(t, path)
	if err := os.WriteFile(seed, []byte(`{"dataset_id":"dataset","locations":[{"id":"A","dataset_id":""}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	provider := fakeLocations(func(ctx context.Context, key string, q data.Query) ([]model.LMPInterval, error) {
		return refreshRows(q), nil
	})
	result, err := refreshCatalog(context.Background(), provider, "k", refreshOptions{dataset: "dataset", output: path, seed: seed, days: 1}, refreshNow(), io.Discard)
	if err != nil || result.updated != 1 || !reflect.DeepEqual(locationIDs(t, path), []string{"A"}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestUpdaterCancellationAndOutputSymlinkAfterFetch(t *testing.T) {
	for _, change := range []string{"cancel", "symlink"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			path, target := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "target.json")
			refreshFixture(t, path, data.Location{ID: "A"})
			before, _ := os.ReadFile(path)
			if err := os.WriteFile(target, []byte("target unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			provider := fakeLocations(func(ctx context.Context, key string, q data.Query) ([]model.LMPInterval, error) {
				if change == "cancel" {
					cancel()
				} else {
					if err := os.Rename(path, path+".saved"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
				}
				return refreshRows(q), nil
			})
			_, err := refreshCatalog(ctx, provider, "k", refreshOptions{dataset: "dataset", output: path, days: 1}, refreshNow(), io.Discard)
			if err == nil {
				t.Fatal("cancellation/output replacement accepted")
			}
			if change == "cancel" {
				after, _ := os.ReadFile(path)
				if !errors.Is(err, context.Canceled) || !bytes.Equal(before, after) {
					t.Fatalf("cancel changed file: %v", err)
				}
			} else {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				after, _ := os.ReadFile(target)
				if string(after) != "target unchanged" {
					t.Fatal("symlink target changed")
				}
			}
			_, lock, err := lockDestination(path)
			if err != nil {
				t.Fatalf("lock not released: %v", err)
			}
			if err := lock.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdaterFlagsAndShellArguments(t *testing.T) {
	provider := fakeLocations(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
		t.Error("invalid flags reached provider")
		return nil, nil
	})
	for _, args := range [][]string{{"--days", "0"}, {"--days", "61"}, {"--dataset-id", " "}, {"unexpected"}, {"--unknown"}} {
		if run(context.Background(), args, "k", provider, io.Discard, io.Discard) == 0 {
			t.Errorf("flags accepted %v", args)
		}
	}
	if run(context.Background(), nil, "", provider, io.Discard, io.Discard) == 0 {
		t.Fatal("missing credential accepted")
	}
	if run(context.Background(), []string{"--help"}, "", provider, io.Discard, io.Discard) != 0 {
		t.Fatal("help failed")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "arguments.txt")
	marker := filepath.Join(dir, "must-not-exist")
	fakeGo := filepath.Join(dir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_ARGUMENTS\"\nexit \"$TEST_EXIT\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	output := "space dir/\"; touch " + marker + "; #.json"
	dataset := "dataset $(touch " + marker + ")"
	seed := "seed with spaces.json"
	for _, exitCode := range []string{"0", "7"} {
		cmd := exec.Command("bash", "../../scripts/update-locations.sh", dataset, output, seed)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "GRIDSTATUS_API_KEY=k", "TEST_ARGUMENTS="+argsPath, "TEST_EXIT="+exitCode)
		result, err := cmd.CombinedOutput()
		if exitCode == "0" && err != nil {
			t.Fatalf("shell failed: %s %v", result, err)
		}
		if exitCode == "7" {
			var failure *exec.ExitError
			if !errors.As(err, &failure) || failure.ExitCode() != 7 {
				t.Fatalf("shell swallowed failure: %v", err)
			}
		}
		raw, _ := os.ReadFile(argsPath)
		want := strings.Join([]string{"run", "./cmd/update-locations", "--dataset-id", dataset, "--output", output, "--seed", seed}, "\n") + "\n"
		if string(raw) != want {
			t.Fatalf("arguments changed: %q", raw)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("shell metacharacters executed")
		}
	}
}

func TestUpdaterUsesRealProviderForPartialRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	refreshFixture(t, path, data.Location{ID: "A", Type: "old-A"}, data.Location{ID: "B", Type: "old-B"})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.Header.Get("x-api-key") != "dummy-secret" || req.URL.Query().Get("download") != "true" || req.URL.Query().Get("timezone") != "market" {
			t.Error("provider request lost credential or query settings")
		}
		if strings.HasSuffix(req.URL.Path, "/B") {
			http.Error(w, "dummy-secret upstream dump", http.StatusForbidden)
			return
		}
		start, err := time.Parse(time.DateOnly, req.URL.Query().Get("start_time"))
		if err != nil {
			t.Error(err)
			return
		}
		row := model.LMPInterval{Location: "A", Market: "fresh-market", LocationType: "fresh-type", IntervalStartUTC: start, IntervalStartLocal: start, IntervalEndUTC: start.Add(5 * time.Minute), IntervalEndLocal: start.Add(5 * time.Minute), LMP: 10}
		_ = json.NewEncoder(w).Encode(model.GridStatusLMPResponse{Data: []model.LMPInterval{row}})
	}))
	defer server.Close()
	provider, err := data.NewGridStatusClient(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--dataset-id", "dataset", "--output", path}, "dummy-secret", provider, &stdout, &stderr)
	list, loadErr := data.LoadLocations(path)
	if code == 0 || calls.Load() != 2 || loadErr != nil || list.Locations[0].Type != "fresh-type" || list.Locations[1].Type != "old-B" {
		t.Fatalf("exit=%d calls=%d list=%#v err=%v", code, calls.Load(), list, loadErr)
	}
	if strings.Contains(stdout.String()+stderr.String(), "dummy-secret") {
		t.Fatal("provider error leaked secret through updater")
	}
}
