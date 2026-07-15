package main

import (
	"os"
	"runtime"
	"testing"
)

// shortTempDir returns an auto-cleaned temp dir anchored under a short base.
// macOS caps unix-socket paths at 104 bytes, and the default t.TempDir()
// ($TMPDIR = /var/folders/...) already runs ~90 chars, so the runed
// supervisor/embedding sockets created beneath a test home overflow bind(2).
// Anchoring under /tmp on unix keeps those socket paths well within the limit.
func shortTempDir(t *testing.T) string {
	t.Helper()
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = ""
	}
	dir, err := os.MkdirTemp(base, "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
