package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
