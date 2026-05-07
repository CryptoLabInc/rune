package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/envector/rune-go/internal/adapters/config"
)

// withTempHome redirects $HOME to a temp dir for the duration of the test.
// config.RuneDir reads $HOME, so this isolates side effects.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("RUNE_STATE", "")
	return dir
}

func readConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".rune", "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse config.json: %v", err)
	}
	return m
}

func TestMarkDormant_FreshInstall_CreatesConfig(t *testing.T) {
	home := withTempHome(t)

	if err := config.MarkDormant("not_configured"); err != nil {
		t.Fatalf("MarkDormant: %v", err)
	}

	got := readConfig(t, home)
	if got["state"] != "dormant" {
		t.Errorf("state: got %v, want dormant", got["state"])
	}
	if got["dormant_reason"] != "not_configured" {
		t.Errorf("dormant_reason: got %v", got["dormant_reason"])
	}
	if _, err := time.Parse(time.RFC3339, got["dormant_since"].(string)); err != nil {
		t.Errorf("dormant_since not RFC3339: %v", got["dormant_since"])
	}
}

func TestMarkDormant_ExistingConfig_PreservesVaultFields(t *testing.T) {
	home := withTempHome(t)

	// Pre-write a config with vault creds.
	cfg := &config.Config{
		Vault: config.VaultConfig{
			Endpoint: "tcp://existing:50051",
			Token:    "evt_keep_me",
		},
		State: "active",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("setup: Save: %v", err)
	}

	// Now mark dormant — vault fields must survive.
	if err := config.MarkDormant("vault_unconfigured"); err != nil {
		t.Fatalf("MarkDormant: %v", err)
	}

	got := readConfig(t, home)
	vault, ok := got["vault"].(map[string]any)
	if !ok {
		t.Fatalf("vault section missing")
	}
	if vault["endpoint"] != "tcp://existing:50051" {
		t.Errorf("vault.endpoint clobbered: got %v", vault["endpoint"])
	}
	if vault["token"] != "evt_keep_me" {
		t.Errorf("vault.token clobbered: got %v", vault["token"])
	}
	if got["state"] != "dormant" {
		t.Errorf("state: got %v, want dormant", got["state"])
	}
	if got["dormant_reason"] != "vault_unconfigured" {
		t.Errorf("dormant_reason: got %v", got["dormant_reason"])
	}
}

func TestMarkDormant_IdempotentOnSameReason(t *testing.T) {
	withTempHome(t)

	if err := config.MarkDormant("not_configured"); err != nil {
		t.Fatalf("first MarkDormant: %v", err)
	}
	first, err := config.Load()
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}
	firstSince := first.DormantSince

	// Sleep to ensure timestamp would differ if write happened.
	time.Sleep(1100 * time.Millisecond)

	// Same reason — must NOT rewrite (timestamp preserved).
	if err := config.MarkDormant("not_configured"); err != nil {
		t.Fatalf("second MarkDormant: %v", err)
	}
	second, err := config.Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if second.DormantSince != firstSince {
		t.Errorf("idempotent same-reason write: timestamp changed (%q → %q)", firstSince, second.DormantSince)
	}
}

func TestMarkDormant_NewReasonOverwrites(t *testing.T) {
	withTempHome(t)

	if err := config.MarkDormant("not_configured"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := config.MarkDormant("vault_unconfigured"); err != nil {
		t.Fatalf("second: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DormantReason != "vault_unconfigured" {
		t.Errorf("reason: got %q, want vault_unconfigured", cfg.DormantReason)
	}
}

func TestMarkDormant_FilePerm0600(t *testing.T) {
	home := withTempHome(t)

	if err := config.MarkDormant("not_configured"); err != nil {
		t.Fatalf("MarkDormant: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".rune", "config.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != config.FilePerm {
		t.Errorf("perm: got %#o, want %#o", got, config.FilePerm)
	}
}

func TestMarkDormant_DormantSinceWithinReasonableWindow(t *testing.T) {
	withTempHome(t)

	before := time.Now().UTC()
	if err := config.MarkDormant("not_configured"); err != nil {
		t.Fatalf("MarkDormant: %v", err)
	}
	after := time.Now().UTC()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	since, err := time.Parse(time.RFC3339, cfg.DormantSince)
	if err != nil {
		t.Fatalf("parse since: %v", err)
	}
	// Allow 1s slack on both sides (RFC3339 is second-precision).
	if since.Before(before.Add(-time.Second)) || since.After(after.Add(time.Second)) {
		t.Errorf("DormantSince out of window: got %v, want between %v and %v", since, before, after)
	}
}

func TestSave_WritesValidJSON(t *testing.T) {
	withTempHome(t)

	cfg := &config.Config{
		Vault: config.VaultConfig{Endpoint: "tcp://test:50051", Token: "t"},
		State: "active",
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load roundtrip: %v", err)
	}
	if got.Vault.Endpoint != "tcp://test:50051" || got.State != "active" {
		t.Errorf("roundtrip: got %+v", got)
	}
}

func TestSave_EnsuresDirIfMissing(t *testing.T) {
	home := withTempHome(t)

	// ~/.rune doesn't exist yet.
	if _, err := os.Stat(filepath.Join(home, ".rune")); !os.IsNotExist(err) {
		t.Fatalf("setup: ~/.rune should not exist, err=%v", err)
	}

	cfg := &config.Config{State: "active"}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".rune"))
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("not a directory")
	}
	if got := info.Mode().Perm(); got != config.DirPerm {
		t.Errorf("dir perm: got %#o, want %#o", got, config.DirPerm)
	}
}

// Sanity: error messages from MarkDormant / Save mention the package prefix
// so callers can grep / wrap appropriately.
func TestSave_ErrorContainsPackagePrefix(t *testing.T) {
	// Force a write error: pass an invalid path that resolves outside any
	// writable dir. We can do this by using SaveToPath directly with a
	// directory path (so WriteFile fails).
	dir := t.TempDir()
	err := config.SaveToPath(&config.Config{}, dir)
	if err == nil {
		t.Fatal("expected error writing to a directory path")
	}
	if !strings.HasPrefix(err.Error(), "config: ") {
		t.Errorf("error prefix: got %q, want 'config: ...'", err.Error())
	}
}
