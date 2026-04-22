// Package config loads ~/.rune/config.json (3-section schema, Go v0.4).
// Spec: docs/v04/spec/components/rune-mcp.md §Config.
// Python: agents/common/config.py (365 LoC) — Go reduced from 7 sections to 3.
//
// Dropped sections (per scope SOT — docs/v04/overview/architecture.md):
//   envector / embedding / llm / scribe / retriever — moved to Vault bundle
//   (memory only) or external embedder process.
package config

import "time"

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
//   ~/.rune/               0700
//   ~/.rune/config.json    0600
//   ~/.rune/logs/          0700
//   ~/.rune/keys/          0700
//   ~/.rune/keys/<id>/     0700
//   ~/.rune/keys/.../*.json 0600
//   ~/.rune/capture_log.jsonl 0600
const (
	DirPerm  = 0700
	FilePerm = 0600
)

// DormantParsedSince parses DormantSince to time.Time (zero if empty/invalid).
func DormantParsedSince(c *Config) time.Time {
	if c.DormantSince == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, c.DormantSince)
	return t
}

// Load reads ~/.rune/config.json.
// TODO: implement with os.UserHomeDir + os.Open + json.Decode.
// TODO: EnsureDirectories (0700 force via os.Chmod, Python L358-365).
// TODO: env var override RUNE_STATE (Python has 12+; Go drops 11, keeps this one).
func Load() (*Config, error) {
	// TODO
	return nil, nil
}
