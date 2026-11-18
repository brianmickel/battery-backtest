//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"battery-backtest/internal/data"
	"battery-backtest/internal/model"
)

func TestUpdaterRejectsUnsafeSidecarsBeforeCatalogWork(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path, target := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "target.json")
			refreshFixture(t, path, data.Location{ID: "A"})
			refreshFixture(t, target, data.Location{ID: "TARGET"})
			before, _ := os.ReadFile(path)
			targetBefore, _ := os.ReadFile(target)
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(target, path+".lock")
			case "hardlink":
				err = os.Link(target, path+".lock")
			case "directory":
				err = os.Mkdir(path+".lock", 0700)
			case "fifo":
				err = syscall.Mkfifo(path+".lock", 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			provider := fakeLocations(func(context.Context, string, data.Query) ([]model.LMPInterval, error) { calls++; return nil, nil })
			finished := make(chan error, 1)
			go func() {
				_, err := refreshCatalog(context.Background(), provider, "k", refreshOptions{dataset: "dataset", output: path, days: 1}, refreshNow(), io.Discard)
				finished <- err
			}()
			select {
			case err := <-finished:
				if err == nil || !strings.Contains(err.Error(), "lock sidecar") {
					t.Fatalf("sidecar error=%v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("sidecar open blocked")
			}
			if calls != 0 {
				t.Fatal("unsafe sidecar reached provider")
			}
			after, _ := os.ReadFile(path)
			targetAfter, _ := os.ReadFile(target)
			if !bytes.Equal(before, after) || !bytes.Equal(targetBefore, targetAfter) {
				t.Fatal("unsafe sidecar changed catalog or linked target")
			}
		})
	}
}

func TestUpdaterReservedNamesCannotReplaceHeldLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	refreshFixture(t, path, data.Location{ID: "A"})
	_, held, err := lockDestination(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	before, _ := os.Stat(path + ".lock")
	if before.Mode().Perm() != 0600 {
		t.Errorf("lock mode=%o", before.Mode().Perm())
	}
	flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, held.Fd(), syscall.F_GETFD, 0)
	if errno != 0 || flags&syscall.FD_CLOEXEC == 0 {
		t.Fatalf("lock descriptor is not close-on-exec: %v %d", errno, flags)
	}
	provider := fakeLocations(func(context.Context, string, data.Query) ([]model.LMPInterval, error) {
		t.Error("reserved output reached provider")
		return nil, nil
	})
	for _, name := range []string{"catalog.json.lock", "catalog.json.LOCK", ".battery-backtest-write", ".BATTERY-BACKTEST-write"} {
		_, err := refreshCatalog(context.Background(), provider, "k", refreshOptions{dataset: "dataset", output: filepath.Join(dir, name), days: 1}, refreshNow(), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("reserved name %q accepted: %v", name, err)
		}
	}
	after, _ := os.Stat(path + ".lock")
	if !os.SameFile(before, after) {
		t.Fatal("held sidecar inode changed")
	}
	_, another, err := lockDestination(path)
	if another != nil {
		_ = another.Close()
	}
	if !errors.Is(err, errDestinationBusy) {
		t.Fatalf("original catalog no longer excluded: %v", err)
	}
}

func TestUpdaterRejectsFinalOutputSymlinkBeforeLock(t *testing.T) {
	dir := t.TempDir()
	path, target := filepath.Join(dir, "catalog.json"), filepath.Join(dir, "target.json")
	refreshFixture(t, target, data.Location{ID: "A"})
	before, _ := os.ReadFile(target)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	_, lock, err := lockDestination(path)
	if lock != nil {
		_ = lock.Close()
	}
	if err == nil {
		t.Fatal("output symlink accepted")
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("output symlink created coordination file")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("output symlink changed target")
	}
}

// The test binary is also the isolated updater process; stdin is an explicit
// barrier after lock acquisition and the first provider call.
func TestUpdaterProcess(t *testing.T) {
	if os.Getenv("BATTERY_UPDATER_TEST_CHILD") != "1" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := true
	mode := os.Getenv("BATTERY_UPDATER_TEST_MODE")
	provider := fakeLocations(func(ctx context.Context, key string, q data.Query) ([]model.LMPInterval, error) {
		fmt.Println("FETCH " + q.LocationID)
		if first {
			first = false
			if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
				return nil, err
			}
			if mode == "cancel" {
				cancel()
				return nil, ctx.Err()
			}
		}
		if mode == "failure" {
			return nil, errors.New("injected provider failure")
		}
		return refreshRows(q), nil
	})
	args := []string{"--dataset-id", "dataset", "--output", os.Getenv("BATTERY_UPDATER_TEST_OUTPUT")}
	if seed := os.Getenv("BATTERY_UPDATER_TEST_SEED"); seed != "" {
		args = append(args, "--seed", seed)
	}
	os.Exit(run(ctx, args, "k", provider, os.Stdout, os.Stderr))
}

type updaterProcess struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	lines          chan string
	done           chan struct{}
	err            error
	stdout, stderr bytes.Buffer
}

func startUpdaterProcess(t *testing.T, path, seed, mode string) *updaterProcess {
	t.Helper()
	p := &updaterProcess{lines: make(chan string, 128), done: make(chan struct{})}
	p.cmd = exec.Command(os.Args[0], "-test.run=^TestUpdaterProcess$")
	p.cmd.Env = append(os.Environ(), "BATTERY_UPDATER_TEST_CHILD=1", "BATTERY_UPDATER_TEST_OUTPUT="+path, "BATTERY_UPDATER_TEST_SEED="+seed, "BATTERY_UPDATER_TEST_MODE="+mode)
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	p.cmd.Stderr = &p.stderr
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			p.stdout.WriteString(line + "\n")
			p.lines <- line
		}
	}()
	go func() { p.err = p.cmd.Wait(); <-scanned; close(p.done) }()
	t.Cleanup(func() {
		_ = p.stdin.Close()
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	return p
}

func (p *updaterProcess) awaitFetch(t *testing.T) {
	t.Helper()
	select {
	case line := <-p.lines:
		if !strings.HasPrefix(line, "FETCH ") {
			t.Fatalf("expected fetch barrier, got %q", line)
		}
	case <-p.done:
		t.Fatalf("updater exited before fetch: %v %s %s", p.err, &p.stdout, &p.stderr)
	case <-time.After(5 * time.Second):
		t.Fatal("updater did not reach fetch barrier")
	}
}
func (p *updaterProcess) release(t *testing.T) {
	t.Helper()
	if _, err := io.WriteString(p.stdin, "continue\n"); err != nil {
		t.Fatal(err)
	}
}
func (p *updaterProcess) wait(t *testing.T, wantSuccess bool) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("updater failed to exit promptly")
	}
	if (p.err == nil) != wantSuccess {
		t.Fatalf("updater success=%v error=%v stdout=%s stderr=%s", wantSuccess, p.err, &p.stdout, &p.stderr)
	}
}

func TestUpdaterProcessesSerializeAliasesAndReloadAdditions(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(realDir, alias); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realDir, "catalog.json")
	seedB, seedC := filepath.Join(dir, "seed-b.json"), filepath.Join(dir, "seed-c.json")
	refreshFixture(t, path, data.Location{ID: "A"})
	refreshFixture(t, seedB, data.Location{ID: "B"})
	refreshFixture(t, seedC, data.Location{ID: "C"})
	before, _ := os.ReadFile(path)
	first := startUpdaterProcess(t, path, seedB, "success")
	first.awaitFetch(t)
	blocked := startUpdaterProcess(t, filepath.Join(alias, "catalog.json"), seedC, "success")
	blocked.wait(t, false)
	if !strings.Contains(blocked.stderr.String(), "busy") || strings.Contains(blocked.stdout.String(), "FETCH") {
		t.Fatalf("contender reached work: %s %s", &blocked.stdout, &blocked.stderr)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("contender changed catalog")
	}
	first.release(t)
	first.wait(t, true)
	if !reflect.DeepEqual(locationIDs(t, path), []string{"A", "B"}) {
		t.Fatal("first writer did not save additions")
	}
	retry := startUpdaterProcess(t, filepath.Join(alias, "catalog.json"), seedC, "success")
	retry.awaitFetch(t)
	retry.release(t)
	retry.wait(t, true)
	if !reflect.DeepEqual(locationIDs(t, path), []string{"A", "B", "C"}) {
		t.Fatal("retry did not reload and preserve both writers' additions")
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatal("stable sidecar was removed")
	}
}

func TestUpdaterProcessesDifferentDestinationsProceed(t *testing.T) {
	dir := t.TempDir()
	firstPath, secondPath := filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")
	refreshFixture(t, firstPath, data.Location{ID: "A"})
	refreshFixture(t, secondPath, data.Location{ID: "B"})
	first := startUpdaterProcess(t, firstPath, "", "success")
	first.awaitFetch(t)
	second := startUpdaterProcess(t, secondPath, "", "success")
	second.awaitFetch(t)
	second.release(t)
	second.wait(t, true)
	_, lock, err := lockDestination(firstPath)
	if lock != nil {
		_ = lock.Close()
	}
	if !errors.Is(err, errDestinationBusy) {
		t.Fatalf("first process lost its independent lock: %v", err)
	}
	first.release(t)
	first.wait(t, true)
}

func TestUpdaterProcessesReleaseAfterCancellationFailureAndTermination(t *testing.T) {
	for _, mode := range []string{"cancel", "failure", "kill"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "catalog.json")
			refreshFixture(t, path, data.Location{ID: "A"})
			before, _ := os.ReadFile(path)
			first := startUpdaterProcess(t, path, "", mode)
			first.awaitFetch(t)
			if mode == "kill" {
				if err := first.cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			} else {
				first.release(t)
			}
			first.wait(t, false)
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("unsuccessful process changed catalog")
			}
			retry := startUpdaterProcess(t, path, "", "success")
			retry.awaitFetch(t)
			retry.release(t)
			retry.wait(t, true)
		})
	}
}
