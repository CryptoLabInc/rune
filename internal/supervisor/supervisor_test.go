//go:build unix

package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const fakeRunedEnv = "RUNE_SUPERVISOR_FAKE_RUNED_BEHAVIOR"
const crashCountFileEnv = "RUNE_SUPERVISOR_FAKE_CRASH_COUNT_FILE"

func TestMain(m *testing.M) {
	if behavior := os.Getenv(fakeRunedEnv); behavior != "" {
		fakeRuned(behavior)
		return
	}
	os.Exit(m.Run())
}

func fakeRuned(behavior string) {
	if path := os.Getenv(crashCountFileEnv); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintln(f, behavior) // append "\n" for counting invocation
			_ = f.Close()
		}
	}

	// Simulate behavior
	switch behavior {
	case "clean":
		os.Exit(0)
	case "crash":
		os.Exit(1)
	case "sleep": // simulate graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		os.Exit(0)
	case "crash_then_clean": // first crash, second exit cleanly
		path := os.Getenv(crashCountFileEnv)
		if path == "" {
			os.Exit(0)
		}

		data, _ := os.ReadFile(path)
		if strings.Count(string(data), "\n") == 1 {
			os.Exit(1)
		}
		os.Exit(0)
	case "ignore_sigterm": // deligate SIGTERM to SIGKILL
		signal.Ignore(syscall.SIGTERM, syscall.SIGINT)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(99)
	}
}

func testWatcherConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		RunedBinary:     os.Args[0],
		BackoffSchedule: []time.Duration{time.Millisecond},
		MaxCrashes:      5,
		MaxCrashWindow:  10 * time.Second,
		ShutdownGrace:   500 * time.Millisecond,
	}
}

func TestWatcher_CleanExitReturnsNil(t *testing.T) {
	t.Setenv(fakeRunedEnv, "clean")
	cfg := testWatcherConfig(t)

	if err := runWatcher(context.Background(), cfg); err != nil {
		t.Errorf("runWatcher: %v, want nil for clean exit", err)
	}
}

func TestWatcher_RestartsAfterCrash(t *testing.T) {
	countFile := t.TempDir() + "/crashes.log"
	t.Setenv(fakeRunedEnv, "crash_then_clean")
	t.Setenv(crashCountFileEnv, countFile)
	cfg := testWatcherConfig(t)

	if err := runWatcher(context.Background(), cfg); err != nil {
		t.Errorf("runWatcher: %v, want nil (first crash -> restart -> clean exit)", err)
	}

	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatalf("read count file: %v", err)
	}

	lines := strings.Count(string(data), "\n")
	if lines != 2 {
		t.Errorf("fake invocations: got %d, want 2 (crash + restart) - log=%s", lines, data)
	}
}

func TestWatcher_GivesUpAfterMaxCrashes(t *testing.T) {
	countFile := t.TempDir() + "/crashes.log"
	t.Setenv(fakeRunedEnv, "crash")
	t.Setenv(crashCountFileEnv, countFile)
	cfg := testWatcherConfig(t)
	cfg.MaxCrashes = 3
	cfg.MaxCrashWindow = 10 * time.Second
	cfg.BackoffSchedule = []time.Duration{time.Millisecond}

	err := runWatcher(context.Background(), cfg)
	if err == nil {
		t.Fatal("runWatcher should return error after MaxCrashes")
	}
	if !strings.Contains(err.Error(), "giving up") {
		t.Errorf("error message: got %q, want substring 'giving up'", err.Error())
	}

	data, _ := os.ReadFile(countFile)
	lines := strings.Count(string(data), "\n")
	if lines != cfg.MaxCrashes {
		t.Errorf("fake invocations: got %d, want %d (one per crash before giving up)", lines, cfg.MaxCrashes)
	}
}

func TestWatcher_ContextCancelTriggersShutdown(t *testing.T) {
	t.Setenv(fakeRunedEnv, "sleep")
	cfg := testWatcherConfig(t)

	ctx, cancel := context.WithCancel(context.Background())

	var watcherErr atomic.Value
	done := make(chan struct{})
	go func() {
		err := runWatcher(ctx, cfg)
		if err != nil {
			watcherErr.Store(err)
		}
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done: // good path
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not exit within 2s of ctx cancellation")
	}

	if v := watcherErr.Load(); v != nil {
		t.Errorf("runWatcher returned %v; want nil on graceful shutdown", v)
	}
}

func TestWatcher_EscalateSIGKILL(t *testing.T) {
	t.Setenv(fakeRunedEnv, "ignore_sigterm")
	cfg := testWatcherConfig(t)
	cfg.ShutdownGrace = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWatcher(ctx, cfg) }()

	time.Sleep(400 * time.Millisecond)
	start := time.Now()
	cancel()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		if err != nil {
			t.Errorf("runWatcher = %v; want nil after SIGKILL escalation", err)
		}

		if elapsed < cfg.ShutdownGrace {
			t.Errorf("returned in %s (< grace %s); SIGTERM should have been ignored", elapsed, cfg.ShutdownGrace)
		}
		if elapsed > cfg.ShutdownGrace+3*time.Second {
			t.Errorf("escalation too slow: %s", elapsed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("runWatcher hung - SIGKILL escalation did not fire")
	}
}
