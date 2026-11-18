//go:build !darwin && !linux

package main

import (
	"errors"
	"os"
)

func openDestinationLock(string) (*os.File, error) {
	return nil, errors.New("location updater destination locking is supported only on Linux and macOS")
}
