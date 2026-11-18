package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errDestinationBusy = errors.New("destination is busy: another location updater holds its lock")

func lockDestination(destination string) (string, *os.File, error) {
	name := strings.ToLower(filepath.Base(destination))
	if strings.HasSuffix(name, ".lock") || strings.HasPrefix(name, ".battery-backtest-") {
		return "", nil, errors.New("output basename uses a reserved lock or temporary-file name")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", nil, fmt.Errorf("resolve destination: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", nil, fmt.Errorf("resolve destination directory: %w", err)
	}
	canonical := filepath.Join(parent, filepath.Base(absolute))
	if err := rejectOutputSymlink(canonical); err != nil {
		return "", nil, err
	}
	lock, err := openDestinationLock(canonical + ".lock")
	if err != nil {
		return "", nil, err
	}
	return canonical, lock, nil
}

func rejectOutputSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect destination: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("destination must be a regular file, not a symlink or directory")
	}
	return nil
}
