//go:build unix

package supervisor

import (
	"path/filepath"
	"testing"
)

func TestAcquireSupervisorLock_FirstCallerWins(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "supervisor.lock")

	// First attempt
	f1, locked1, err := acquireSupervisorLock(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if !locked1 {
		t.Fatal("first acquire should succeed (no contention)")
	}
	t.Cleanup(func() { _ = f1.Close() })

	// Second attempt
	f2, locked2, err := acquireSupervisorLock(lockPath)
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if locked2 {
		t.Error("second acquire should fail (lock held by f1)")
		_ = f2.Close()
	}
	if f2 != nil {
		t.Error("contended acquire should return nil file handle")
	}
}

func TestAcquireSupervisorLock_ReleasedOnClose(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "supervisor.lock")

	// Acquire lock
	f1, locked, err := acquireSupervisorLock(lockPath)
	if err != nil || !locked {
		t.Fatalf("first acquire: locked=%v err=%v", locked, err)
	}
	// Release lock
	if err := f1.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	// Acquire lock
	f2, locked2, err := acquireSupervisorLock(lockPath)
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if !locked2 {
		t.Fatal("re-acquire after close should succeed")
	}
	_ = f2.Close()
}

func TestAcquireSupervisorLock_CreatesParentDir(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "nested", "deeper", "supervisor.lock")

	f, locked, err := acquireSupervisorLock(lockPath)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !locked {
		t.Fatal("acquire should succeed even with missing parent dir")
	}
	_ = f.Close()
}
