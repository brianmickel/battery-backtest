package data

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func locationFixture() *LocationList {
	return &LocationList{DatasetID: "dataset", UpdatedAt: "2024-01-02T00:00:00Z", Locations: []Location{{ID: "B", Name: "Beta"}, {ID: "A", Name: "Alpha", DatasetID: "dataset"}}}
}

func TestLocationsRoundTripSortedWithoutMutatingCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	list := locationFixture()
	if err := SaveLocations(list, path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadLocations(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Locations[0].ID != "A" || got.Locations[1].ID != "B" || got.Locations[1].DatasetID != "dataset" {
		t.Fatalf("catalog = %#v", got)
	}
	if !reflect.DeepEqual(list, locationFixture()) {
		t.Fatal("save mutated caller")
	}
	if err := SaveLocations(list, path); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("repeated write was not deterministic")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".battery-backtest-*"))
	if len(matches) != 0 {
		t.Fatalf("temporary files leaked: %v", matches)
	}
}

func TestLocationsLoadPreservesErrorsAndRejectsInvalidCatalogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := LoadLocations(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error = %v", err)
	}
	for _, input := range []string{`{`, `null`, `{}`, `{"dataset_id":"dataset","locations":null}`, `{"dataset_id":"dataset","locations":[{"id":""}]}`, `{"dataset_id":"dataset","locations":[{"id":"A"},{"id":"A"}]}`, `{"dataset_id":"dataset","locations":[{"id":"A","dataset_id":"other"}]}`} {
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadLocations(path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			t.Errorf("invalid catalog %s error = %v", input, err)
		}
		if input == "{" {
			var syntax *json.SyntaxError
			if !errors.As(err, &syntax) {
				t.Error("JSON error identity lost")
			}
		}
	}
	if os.Geteuid() != 0 {
		if err := os.Chmod(path, 0000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(path, 0600)
		if _, err := LoadLocations(path); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("permission error = %v", err)
		}
	}
}

type faultyLocationTemp struct {
	*os.File
	mode  string
	fault error
}

func (f *faultyLocationTemp) Write(b []byte) (int, error) {
	if f.mode == "write" {
		return 0, f.fault
	}
	if f.mode == "short" {
		return len(b) - 1, nil
	}
	return f.File.Write(b)
}
func (f *faultyLocationTemp) Close() error {
	err := f.File.Close()
	if f.mode == "close" {
		return f.fault
	}
	return err
}

func TestLocationsAtomicFailuresPreserveOriginal(t *testing.T) {
	for _, mode := range []string{"create", "write", "short", "close", "rename"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "catalog.json")
			original := []byte(`{"preserve":"exact bytes"}`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Stat(path)
			fault := errors.New("injected failure")
			var created *os.File
			create := func(directory, pattern string) (locationTempFile, error) {
				if directory != dir || pattern != ".battery-backtest-*" {
					t.Errorf("temporary location = %q/%q", directory, pattern)
				}
				if mode == "create" {
					return nil, fault
				}
				file, err := os.CreateTemp(directory, pattern)
				created = file
				if err != nil {
					return nil, err
				}
				return &faultyLocationTemp{File: file, mode: mode, fault: fault}, nil
			}
			rename := func(from, to string) error {
				if _, err := created.Write([]byte("closed")); !errors.Is(err, os.ErrClosed) {
					t.Error("rename attempted before close")
				}
				if mode == "rename" {
					return fault
				}
				return os.Rename(from, to)
			}
			err := writeLocationsAtomically(path, []byte(`{"new":"data"}`), create, rename)
			if mode == "short" {
				if !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("short-write error = %v", err)
				}
			} else if !errors.Is(err, fault) {
				t.Fatalf("injected error identity lost: %v", err)
			}
			after, _ := os.Stat(path)
			contents, _ := os.ReadFile(path)
			if string(contents) != string(original) || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
				t.Fatal("failed write changed original catalog")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatalf("temporary files survived failure: %v", entries)
			}
		})
	}
}

func TestLocationsAtomicWriteRechecksOutputSymlink(t *testing.T) {
	dir := t.TempDir()
	path, target := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte("target unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	create := func(dir, pattern string) (locationTempFile, error) {
		if err := os.Symlink(target, path); err != nil {
			return nil, err
		}
		return os.CreateTemp(dir, pattern)
	}
	err := writeLocationsAtomically(path, []byte("replacement"), create, os.Rename)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink accepted: %v", err)
	}
	contents, _ := os.ReadFile(target)
	if string(contents) != "target unchanged" {
		t.Fatal("symlink target changed")
	}
}
