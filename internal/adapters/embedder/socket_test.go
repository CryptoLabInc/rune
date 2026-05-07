package embedder_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/envector/rune-go/internal/adapters/embedder"
)

func TestResolveSocketPath_EnvVarWins(t *testing.T) {
	t.Setenv(embedder.SocketEnvVar, "/tmp/custom.sock")

	got := embedder.ResolveSocketPath("/from/config.sock")
	if got != "/tmp/custom.sock" {
		t.Errorf("env override should win: got %q, want /tmp/custom.sock", got)
	}
}

func TestResolveSocketPath_ConfigUsedWhenEnvUnset(t *testing.T) {
	t.Setenv(embedder.SocketEnvVar, "")

	got := embedder.ResolveSocketPath("/from/config.sock")
	if got != "/from/config.sock" {
		t.Errorf("config path: got %q, want /from/config.sock", got)
	}
}

func TestResolveSocketPath_DefaultWhenNothingProvided(t *testing.T) {
	t.Setenv(embedder.SocketEnvVar, "")
	t.Setenv("HOME", "/test/home")

	got := embedder.ResolveSocketPath("")
	want := filepath.Join("/test/home", embedder.DefaultSocketPath)
	if got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
}

func TestResolveSocketPath_DefaultEndsInRunedDir(t *testing.T) {
	t.Setenv(embedder.SocketEnvVar, "")
	t.Setenv("HOME", "/h")

	got := embedder.ResolveSocketPath("")
	if !strings.HasSuffix(got, ".runed/embedding.sock") {
		t.Errorf("default path should end in .runed/embedding.sock, got %q", got)
	}
}

func TestResolveSocketPath_EmptyConfigStringFalsThrough(t *testing.T) {
	// Explicit empty config string should fall through to env lookup, not be
	// treated as a valid path itself.
	t.Setenv(embedder.SocketEnvVar, "/env/wins.sock")

	got := embedder.ResolveSocketPath("")
	if got != "/env/wins.sock" {
		t.Errorf("env should still win when config is empty: got %q", got)
	}
}
