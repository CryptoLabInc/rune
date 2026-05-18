package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_DefaultsUserHome(t *testing.T) {
	t.Setenv(envRuneHome, "")
	t.Setenv(envRunedHome, "")
	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasSuffix(p.RuneHome, "/.rune") {
		t.Errorf("RuneHome = %q, want ending in /.rune", p.RuneHome)
	}
	if !strings.HasSuffix(p.RunedHome, "/.runed") {
		t.Errorf("RunedHome = %q, want ending in /.runed", p.RunedHome)
	}
	if filepath.Dir(p.RunedBinary) != p.RunedBin {
		t.Errorf("RunedBinary not under RunedBin: %q vs %q", p.RunedBinary, p.RunedBin)
	}
}

func TestResolve_RuneHomeEnv(t *testing.T) {
	t.Setenv(envRuneHome, "/tmp/rune-test")
	t.Setenv(envRunedHome, "")
	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.RuneHome != "/tmp/rune-test" {
		t.Errorf("RuneHome = %q", p.RuneHome)
	}
	if p.RuneConfig != "/tmp/rune-test/config.json" {
		t.Errorf("RuneConfig = %q", p.RuneConfig)
	}
	if !strings.HasSuffix(p.RunedHome, "/.runed") {
		t.Errorf("RunedHome should be unaffected by RUNE_HOME: %q", p.RunedHome)
	}
}

func TestResolve_RunedHomeEnv(t *testing.T) {
	t.Setenv(envRuneHome, "")
	t.Setenv(envRunedHome, "/tmp/runed-test")
	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.RunedHome != "/tmp/runed-test" {
		t.Errorf("RunedHome = %q", p.RunedHome)
	}
	if p.RunedBinary != "/tmp/runed-test/bin/runed" {
		t.Errorf("RunedBinary = %q", p.RunedBinary)
	}
	if p.RunedSocket != "/tmp/runed-test/embedding.sock" {
		t.Errorf("RunedSocket = %q", p.RunedSocket)
	}
}

func TestResolve_Independent(t *testing.T) {
	t.Setenv(envRuneHome, "/tmp/rune-x")
	t.Setenv(envRunedHome, "/tmp/runed-y")
	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.RuneHome != "/tmp/rune-x" || p.RunedHome != "/tmp/runed-y" {
		t.Errorf("realms not independent: rune=%q runed=%q", p.RuneHome, p.RunedHome)
	}
	if filepath.Dir(p.RuneConfig) != p.RuneHome {
		t.Errorf("RuneConfig should be under RuneHome: %q vs %q", p.RuneConfig, p.RuneHome)
	}
	if filepath.Dir(p.RunedSocket) != p.RunedHome {
		t.Errorf("RunedSocket should be under RunedHome: %q vs %q", p.RunedSocket, p.RunedHome)
	}
}

func TestEnsureDirs_CreatesRunedOnly(t *testing.T) {
	t.Setenv(envRuneHome, filepath.Join(t.TempDir(), "rune"))
	t.Setenv(envRunedHome, filepath.Join(t.TempDir(), "runed"))
	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if err := p.EnsureDirs(); err != nil { // creates
		t.Fatalf("EnsureDirs first call: %v", err)
	}
	if err := p.EnsureDirs(); err != nil { // no-op
		t.Fatalf("EnsureDirs second call: %v", err)
	}

	for _, expected := range []string{
		p.RunedHome, p.RunedBin, p.RunedModels, p.RunedLogs, p.Cache,
	} {
		info, err := os.Stat(expected)
		if err != nil {
			t.Errorf("expected dir %s: %v", expected, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", expected)
		}
	}

	if _, err := os.Stat(p.RuneHome); !os.IsNotExist(err) {
		t.Errorf("RuneHome %s should not be created by EnsureDirs (rune install must not touch rune-mcp's realm); got err=%v", p.RuneHome, err)
	}
}

func TestPlatformTuple_NonEmpty(t *testing.T) {
	got := PlatformTuple()
	if !strings.Contains(got, "-") {
		t.Errorf("PlatformTuple = %q, want <os>-<arch>", got)
	}
}
