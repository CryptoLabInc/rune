package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type InstalledManifest struct {
	ManifestURL     string `json:"manifest_url"`
	ManifestVersion int    `json:"manifest_version"`

	RuneMCPVersion string `json:"rune_mcp_version,omitempty"`
	RunedVersion   string `json:"runed_version,omitempty"`

	Platform    string                       `json:"platform"`
	InstalledAt string                       `json:"installed_at"` // UTC RFC3339 timestamp
	Artifacts   map[string]InstalledArtifact `json:"artifacts"`
}

type InstalledArtifact struct {
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`                // manifest spec hash (the archive hash for a tar.gz artifact)
	DestSHA256 string `json:"dest_sha256,omitempty"` // installed raw binary sha256 (extracted file's hash for tarball)
	Path       string `json:"path"`
	Size       int64  `json:"size,omitempty"`
}

func WriteInstalledManifest(paths *Paths, manifestURL string, manifest *Manifest, artifacts map[string]InstalledArtifact) error {
	record := InstalledManifest{
		ManifestURL:     manifestURL,
		ManifestVersion: manifest.Version,
		RuneMCPVersion:  manifest.RuneMCPVersion,
		RunedVersion:    manifest.RunedVersion,
		Platform:        PlatformTuple(),
		InstalledAt:     time.Now().UTC().Format(time.RFC3339),
		Artifacts:       artifacts,
	}
	data, err := json.MarshalIndent(&record, "", "  ")
	if err != nil {
		return fmt.Errorf("installed.json: marshal: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(paths.InstalledManifest), 0o700); err != nil {
		return fmt.Errorf("installed.json: mkdir parent: %w", err)
	}

	// Atomic write by writing to tmp then renaming
	tmp, err := os.CreateTemp(filepath.Dir(paths.InstalledManifest), ".installed.json.*")
	if err != nil {
		return fmt.Errorf("installed.json: tmp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("installed.json: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("installed.json: close: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("installed.json: chmod: %w", err)
	}
	if err := os.Rename(tmpName, paths.InstalledManifest); err != nil {
		return fmt.Errorf("installed.json: rename: %w", err)
	}
	return nil
}

func ReadInstalledManifest(paths *Paths) (*InstalledManifest, error) {
	data, err := os.ReadFile(paths.InstalledManifest)
	if err != nil {
		return nil, err
	}

	var record InstalledManifest
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("installed.json: parse: %w", err)
	}

	return &record, nil
}
