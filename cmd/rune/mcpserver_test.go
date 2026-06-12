package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func TestRunMCPServer_InstallErrorFailFast(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	// Fail-fast error: unsupported manifest version
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": 999}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RUNE_MANIFEST", srv.URL)

	var stderr bytes.Buffer
	start := time.Now()
	code := runMCPServer(context.Background(), nil, &stderr)
	elapsed := time.Since(start)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("took %s; fail fast install errors must not poll the %s self-heal budget", elapsed, mcpSelfhealBudget)
	}
	if !strings.Contains(stderr.String(), "cannot install rune-mcp") {
		t.Errorf("stderr missing fail-fast message: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "another session") {
		t.Errorf("fail-fast errors must not be diagnosed as a concurrent install: %q", stderr.String())
	}
}

//--- Lock tests ---//

func TestRunMCPServer_TryLockDuringInstall(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()
	t.Setenv("RUNE_MANIFEST", "")

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	paths, err := bootstrap.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	f, err := os.OpenFile(paths.InstallLock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open lock: %v", err)
	}
	defer f.Close()

	// Hold lock
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock: %v", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond) // short timeout for test
	defer cancel()

	var stderr bytes.Buffer
	code := runMCPServer(ctx, nil, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "another session") {
		t.Errorf("lock failure should route to the concurrent-install wait path: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "cannot install rune-mcp") {
		t.Errorf("lock failure must be a retriable error: %q", stderr.String())
	}
}

//--- waitForFile tests ---//

func TestWaitForFile_AlreadyPresent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rune-mcp")
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	if !waitForFile(context.Background(), p, time.Second) {
		t.Error("want true when the file already exists")
	}
}

func TestWaitForFile_CreatedAfterCheck(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rune-mcp")
	go func() {
		time.Sleep(100 * time.Millisecond) // file created after initial check
		_ = os.WriteFile(p, []byte("x"), 0o755)
	}()

	if !waitForFile(context.Background(), p, 3*time.Second) {
		t.Error("want true once the file appears after initial check-up")
	}
}

func TestWaitForFile_CtxCancel(t *testing.T) {
	p := filepath.Join(t.TempDir(), "never")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if waitForFile(ctx, p, 5*time.Second) {
		t.Error("want false when ctx is already cancelled")
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ctx cancel should return promptly, not wait out the timeout; took %s", elapsed)
	}
}

func TestWaitForFile_Timeout(t *testing.T) {
	p := filepath.Join(t.TempDir(), "never")
	start := time.Now()
	if waitForFile(context.Background(), p, 200*time.Millisecond) {
		t.Error("want false on timeout when the file never appears")
	}

	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("returned %s before the 200ms timeout", elapsed)
	}
}
