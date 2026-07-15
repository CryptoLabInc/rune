package bootstrap

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// shortTempDir returns an auto-cleaned temp dir anchored under a short base.
// macOS caps unix-socket paths at 104 bytes, and the default t.TempDir()
// ($TMPDIR = /var/folders/...) already runs ~90 chars, so a runed socket
// beneath it overflows bind(2). Anchor under /tmp on unix so the embedding /
// supervisor sockets stay well within the limit.
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

func setRealms(t *testing.T) (runeHome, runedHome string) {
	t.Helper()
	dir := shortTempDir(t)
	runeHome = filepath.Join(dir, "rune")
	runedHome = filepath.Join(dir, "runed")
	t.Setenv(envRuneHome, runeHome)
	t.Setenv(envRunedHome, runedHome)
	return runeHome, runedHome
}
