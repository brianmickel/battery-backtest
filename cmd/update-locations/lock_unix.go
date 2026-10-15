//go:build darwin || linux

package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openDestinationLock(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect lock: %w", err)
	}
	if before != nil && !validLockFile(before) {
		return nil, errors.New("lock sidecar must be a regular file with exactly one link")
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock sidecar: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened lock: %w", err)
	}
	if !validLockFile(after) || (before != nil && !os.SameFile(before, after)) {
		_ = file.Close()
		return nil, errors.New("lock sidecar changed or is not a regular file with exactly one link")
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errDestinationBusy
		}
		return nil, fmt.Errorf("lock destination: %w", err)
	}
	return file, nil
}

func validLockFile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return info.Mode().IsRegular() && ok && stat.Nlink == 1
}
