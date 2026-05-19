package bootstrap

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a valid rune-mcp config.json to runeHome/config.json.
// Tests use this to set up the "configure already done" baseline.
func writeConfig(t *testing.T, runeHome string, vaultEndpoint, vaultToken string) {
	t.Helper()
	if err := os.MkdirAll(runeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := runeMCPConfig{
		Vault: &runeVaultBlock{Endpoint: vaultEndpoint, Token: vaultToken},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(runeHome, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeFakeBinary places an executable stub at path with the given
// content. Used to satisfy the "exists + executable" checks.
func writeFakeBinary(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho fake\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// findCheck returns the named check from r, or fails the test if missing.
func findCheck(t *testing.T, r *InstallChecks, name string) InstallCheck {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in result: %+v", name, r)
	return InstallCheck{}
}

func TestInstallChecks_HealthyInstall(t *testing.T) {
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://vault.example:50051", "evt_test")
	writeFakeBinary(t, filepath.Join(runed, "bin", "runed"))
	writeFakeBinary(t, filepath.Join(runed, "bin", "llama-server"))
	// Plausibly-real GGUF: a file with size >= minModelSize.
	if err := os.MkdirAll(filepath.Join(runed, "models"), 0o700); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(runed, "models", "qwen3-embedding-0.6b.q6_k.gguf")
	if err := os.WriteFile(modelPath, make([]byte, minModelSize+1), 0o600); err != nil {
		t.Fatal(err)
	}

	r := RunInstallChecks(context.Background())
	if !r.OK {
		t.Errorf("OK should be true on healthy install; got %+v", r)
	}
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			t.Errorf("unexpected fail: %+v", c)
		}
	}
}

func TestInstallChecks_MissingConfig(t *testing.T) {
	setRealms(t)
	r := RunInstallChecks(context.Background())
	cfg := findCheck(t, r, CheckRuneConfig)
	if cfg.Status != StatusFail {
		t.Errorf("rune_config should fail when missing; got %+v", cfg)
	}
	if r.OK {
		t.Error("OK should be false when config missing")
	}
}

func TestInstallChecks_CorruptConfig(t *testing.T) {
	rune, _ := setRealms(t)
	if err := os.MkdirAll(rune, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rune, "config.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := RunInstallChecks(context.Background())
	cfg := findCheck(t, r, CheckRuneConfig)
	if cfg.Status != StatusFail {
		t.Errorf("rune_config should fail on parse error; got %+v", cfg)
	}
}

func TestInstallChecks_MissingVaultFields(t *testing.T) {
	rune, _ := setRealms(t)
	writeConfig(t, rune, "", "") // both empty
	r := RunInstallChecks(context.Background())
	creds := findCheck(t, r, CheckVaultCreds)
	if creds.Status != StatusFail {
		t.Errorf("vault_creds should fail on empty fields; got %+v", creds)
	}
}

func TestInstallChecks_MissingRunedBinary(t *testing.T) {
	rune, _ := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	// No runed binary written.
	r := RunInstallChecks(context.Background())
	bin := findCheck(t, r, CheckRunedBinary)
	if bin.Status != StatusFail {
		t.Errorf("runed_binary should fail when missing; got %+v", bin)
	}
	if bin.FixHint == "" {
		t.Errorf("missing-binary check should include a fix hint")
	}
}

func TestInstallChecks_NonExecutableBinary(t *testing.T) {
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	binPath := filepath.Join(runed, "bin", "runed")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o700); err != nil {
		t.Fatal(err)
	}
	// File exists but mode 0o644 — readable but not executable.
	if err := os.WriteFile(binPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := RunInstallChecks(context.Background())
	bin := findCheck(t, r, CheckRunedBinary)
	if bin.Status != StatusFail {
		t.Errorf("runed_binary should fail when not executable; got %+v", bin)
	}
}

func TestInstallChecks_PartialModelFile(t *testing.T) {
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	writeFakeBinary(t, filepath.Join(runed, "bin", "runed"))
	writeFakeBinary(t, filepath.Join(runed, "bin", "llama-server"))
	if err := os.MkdirAll(filepath.Join(runed, "models"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Tiny file — should be flagged as a probable partial download.
	if err := os.WriteFile(filepath.Join(runed, "models", "x.gguf"), []byte("tiny"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := RunInstallChecks(context.Background())
	mc := findCheck(t, r, CheckModelFile)
	if mc.Status != StatusFail {
		t.Errorf("model_file should fail on partial file; got %+v", mc)
	}
}

func TestInstallChecks_NoModelYet_Warns(t *testing.T) {
	// Pre-first-activate state: binaries present but no model on disk.
	// This is NOT a failure — runed will populate the model on first
	// startup. Should warn.
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	writeFakeBinary(t, filepath.Join(runed, "bin", "runed"))
	writeFakeBinary(t, filepath.Join(runed, "bin", "llama-server"))
	if err := os.MkdirAll(filepath.Join(runed, "models"), 0o700); err != nil {
		t.Fatal(err)
	}

	r := RunInstallChecks(context.Background())
	mc := findCheck(t, r, CheckModelFile)
	if mc.Status != StatusWarn {
		t.Errorf("empty models dir should warn (not fail); got %+v", mc)
	}
	// Overall OK should still be true (only warn-level issues).
	if !r.OK {
		t.Errorf("OK should be true with only warns; got %+v", r)
	}
}

func TestInstallChecks_DaemonNotRunning_Warns(t *testing.T) {
	// No socket at the default path — expected pre-activate state.
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	writeFakeBinary(t, filepath.Join(runed, "bin", "runed"))
	writeFakeBinary(t, filepath.Join(runed, "bin", "llama-server"))

	r := RunInstallChecks(context.Background())
	sock := findCheck(t, r, CheckSocket)
	if sock.Status != StatusWarn {
		t.Errorf("daemon_socket should warn when not running; got %+v", sock)
	}
}

func TestInstallChecks_DaemonReachable_OK(t *testing.T) {
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	writeFakeBinary(t, filepath.Join(runed, "bin", "runed"))
	writeFakeBinary(t, filepath.Join(runed, "bin", "llama-server"))

	// Stand up a unix-domain listener at the expected socket path.
	if err := os.MkdirAll(runed, 0o700); err != nil {
		t.Fatal(err)
	}
	sockPath := filepath.Join(runed, "embedding.sock")
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	r := RunInstallChecks(context.Background())
	sock := findCheck(t, r, CheckSocket)
	if sock.Status != StatusOK {
		t.Errorf("daemon_socket should be ok when reachable; got %+v", sock)
	}
}

func TestInstallChecks_SpawnLockPresent_Warns(t *testing.T) {
	rune, runed := setRealms(t)
	writeConfig(t, rune, "tcp://x", "y")
	writeFakeBinary(t, filepath.Join(runed, "bin", "runed"))
	writeFakeBinary(t, filepath.Join(runed, "bin", "llama-server"))
	if err := os.MkdirAll(runed, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runed, "spawn.lock"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	r := RunInstallChecks(context.Background())
	lock := findCheck(t, r, CheckSpawnLock)
	if lock.Status != StatusWarn {
		t.Errorf("spawn_lock should warn when present; got %+v", lock)
	}
}
