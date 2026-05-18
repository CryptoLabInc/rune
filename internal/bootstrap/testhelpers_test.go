package bootstrap

import (
	"path/filepath"
	"testing"
)

func setRealms(t *testing.T) (runeHome, runedHome string) {
	t.Helper()
	dir := t.TempDir()
	runeHome = filepath.Join(dir, "rune")
	runedHome = filepath.Join(dir, "runed")
	t.Setenv(envRuneHome, runeHome)
	t.Setenv(envRunedHome, runedHome)
	return runeHome, runedHome
}
