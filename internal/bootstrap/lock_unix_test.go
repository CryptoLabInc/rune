//go:build !windows

package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAcquireInstallLock_HappyPath(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "install.lock")
	unlock, err := acquireInstallLock(context.Background(), lockPath, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("acquireInstallLock: %v", err)
	}
	unlock()
}

func TestAcquireInstallLock_TimeOut(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "install.lock")

	unlock, err := acquireInstallLock(context.Background(), lockPath, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer unlock()

	start := time.Now()
	_, err = acquireInstallLock(context.Background(), lockPath, 100*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("second acquire should have failed with timeout")
	}
	if !strings.Contains(err.Error(), "another install in progress") {
		t.Errorf("error should mention 'another install in progress'; got %v", err)
	}

	if elapsed < 80*time.Millisecond {
		t.Errorf("returned too quickly: %s", elapsed)
	}
	if elapsed > 100*time.Millisecond+2*installLockPollInterval {
		t.Errorf("waited too long: %s (timeout was 100ms)", elapsed)
	}
}

func TestAcquireInstallLock_CtxCancel(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "install.lock")

	unlock, err := acquireInstallLock(context.Background(), lockPath, 5*time.Second)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	var (
		wg       sync.WaitGroup
		gotErr   error
		gotStart = time.Now()
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, gotErr = acquireInstallLock(ctx, lockPath, 5*time.Second)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
	elapsed := time.Since(gotStart)

	if gotErr == nil {
		t.Fatal("expected error from cancelled acquire")
	}
	if !errors.Is(gotErr, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", gotErr)
	}
	if elapsed > 1*time.Second {
		t.Errorf("ctx cancel should be honored quickly; took %s", elapsed)
	}
}

func TestAcquireInstallLock_ReleasedLockReacquireable(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "install.lock")

	unlock, err := acquireInstallLock(context.Background(), lockPath, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	unlock()

	start := time.Now()
	unlock2, err := acquireInstallLock(context.Background(), lockPath, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	defer unlock2()

	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("reacquire was slow: %s", elapsed)
	}
}
