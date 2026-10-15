//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCLITerminationSignalCancelsWorkWithNonzeroExit(t *testing.T) {
	configPath, dataPath, _ := commandFixture(t)
	configuration, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(t.TempDir(), "config.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestCLIProcess", "--", "backtest", "--config", fifo, "--data", dataPath, "--out", filepath.Join(t.TempDir(), "canceled.csv"))
	command.Env = append(os.Environ(), "BATTERY_CLI_TEST_PROCESS=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	// Opening the writer completes only when the command has reached LoadFile,
	// after main installed signal cancellation. No sleep selects the signal timing.
	type opened struct {
		file *os.File
		err  error
	}
	ready := make(chan opened, 1)
	go func() { file, err := os.OpenFile(fifo, os.O_WRONLY, 0600); ready <- opened{file, err} }()
	var writer *os.File
	select {
	case result := <-ready:
		if result.err != nil {
			t.Fatal(result.err)
		}
		writer = result.file
	case <-ctx.Done():
		unblock, _ := os.OpenFile(fifo, os.O_RDWR|syscall.O_NONBLOCK, 0600)
		if unblock != nil {
			defer unblock.Close()
		}
		t.Fatal("command did not reach configuration read")
	}
	defer writer.Close()
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(configuration); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf("signal cancellation: stdout=%s stderr=%s error=%v", stdout.String(), stderr.String(), err)
	}
}
