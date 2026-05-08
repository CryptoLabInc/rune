// Package config loads ~/.rune/config.json (3-section schema, Go v0.4).
// Spec: docs/v04/spec/components/rune-mcp.md §Config.
// Python: agents/common/config.py (365 LoC) — Go reduced from 7 sections to 3.
//
// Dropped sections (per scope SOT — docs/v04/overview/architecture.md):
//
//	envector / embedding / llm / scribe / retriever — moved to Vault bundle
//	(memory only) or external embedder process.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config — top-level. Read-only by rune-mcp (write path: /rune:configure CLI).
type Config struct {
	Vault         VaultConfig    `json:"vault"`
	State         string         `json:"state"` // "active" | "dormant"
	DormantReason string         `json:"dormant_reason,omitempty"`
	DormantSince  string         `json:"dormant_since,omitempty"` // RFC3339 UTC
	Metadata      map[string]any `json:"metadata,omitempty"`      // configVersion/lastUpdated/installedFrom
}

// VaultConfig — connection + auth.
type VaultConfig struct {
	Endpoint   string `json:"endpoint"` // tcp://host:port | http(s)://... | host[:port]
	Token      string `json:"token"`
	CACert     string `json:"ca_cert,omitempty"`
	TLSDisable bool   `json:"tls_disable,omitempty"`
}

// FilePerms — per rune-mcp.md §Config:
//
//	~/.rune/               0700
//	~/.rune/config.json    0600
const (
	DirPerm  = 0700
	FilePerm = 0600
)

func DormantParsedSince(c *Config) time.Time {
	if c.DormantSince == "" {
		return time.Time{}
	}

	// DormantSince to time.Time
	t, _ := time.Parse(time.RFC3339, c.DormantSince)
	return t
}

func (c *Config) IsActive() bool {
	return c.State == "active"
}

func RuneDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: UserHomeDir: %w", err)
	}
	return filepath.Join(home, ".rune"), nil // ~/.rune
}

func DefaultConfigPath() (string, error) {
	dir, err := RuneDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil // ~/.rune/config.json
}

func Load() (*Config, error) {
	configPath, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFromPath(configPath)
}

func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if stateOverride := os.Getenv("RUNE_STATE"); stateOverride != "" {
		cfg.State = stateOverride
	}

	return &cfg, nil
}

func EnsureDirectories() error {
	dir, err := RuneDir()
	if err != nil {
		return err
	}

	// Create directory if not exists
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", dir, err)
	}

	// Force permissions
	if err := os.Chmod(dir, DirPerm); err != nil {
		return fmt.Errorf("config: chmod %s: %w", dir, err)
	}

	// Ensure subdirectories
	for _, sub := range []string{"keys", "logs"} {
		subDir := filepath.Join(dir, sub)
		if err := os.MkdirAll(subDir, DirPerm); err != nil {
			return fmt.Errorf("config: mkdir %s: %w", subDir, err)
		}
		if err := os.Chmod(subDir, DirPerm); err != nil {
			return fmt.Errorf("config: chmod %s: %w", subDir, err)
		}
	}

	return nil
}
