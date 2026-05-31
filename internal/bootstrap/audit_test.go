package bootstrap

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWriteAndReadInstalledManifest_RoundTrip(t *testing.T) {
	setRealms(t)

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	manifest := &Manifest{
		Version:        1,
		RuneMCPVersion: "v0.1.0",
		RunedVersion:   "v0.1.0-alpha.1",
	}

	artifacts := map[string]InstalledArtifact{
		StepRuned: {
			URL:    "https://example/runed-linux-amd64",
			SHA256: "aaa",
			Path:   paths.RunedBinary,
			Size:   12345,
		},
		StepRuneMCP: {
			URL:    "https://example/rune-mcp-linux-amd64",
			SHA256: "ccc",
			Path:   paths.RuneMCPBinary,
			Size:   67890,
		},
	}

	if err := WriteInstalledManifest(paths, "https://example/manifest.json", manifest, artifacts); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}

	info, err := os.Stat(paths.InstalledManifest)
	if err != nil {
		t.Fatalf("stat installed.json: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %v, want 0o600", perm)
	}

	got, err := ReadInstalledManifest(paths)
	if err != nil {
		t.Fatalf("ReadInstalledManifest: %v", err)
	}
	if got.ManifestURL != "https://example/manifest.json" {
		t.Errorf("ManifestURL = %q", got.ManifestURL)
	}
	if got.ManifestVersion != 1 {
		t.Errorf("ManifestVersion = %d", got.ManifestVersion)
	}
	if got.RuneMCPVersion != "v0.1.0" {
		t.Errorf("RuneMCPVersion = %q", got.RuneMCPVersion)
	}
	if got.RunedVersion != "v0.1.0-alpha.1" {
		t.Errorf("RunedVersion = %q", got.RunedVersion)
	}
	if got.Platform == "" {
		t.Error("Platform empty; want <os>-<arch>")
	}
	if got.InstalledAt == "" {
		t.Error("InstalledAt empty")
	}
	if len(got.Artifacts) != 2 {
		t.Errorf("Artifacts: got %d, want 2", len(got.Artifacts))
	}
	if got.Artifacts[StepRuned].SHA256 != "aaa" {
		t.Errorf("runed SHA256 = %q", got.Artifacts[StepRuned].SHA256)
	}
}

func TestReadInstalledManifest_NotInstalled(t *testing.T) {
	setRealms(t)
	paths, _ := Resolve()

	_, err := ReadInstalledManifest(paths)
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want os.IsNotExist (no install has run)", err)
	}
}

func TestWriteInstalledManifest_OverwritesAtomically(t *testing.T) {
	setRealms(t)
	paths, _ := Resolve()
	_ = paths.EnsureDirs()

	// Initial installation
	manifest := &Manifest{Version: 1}
	first := map[string]InstalledArtifact{StepRuned: {URL: "u1", SHA256: "s1", Path: "/p1"}}
	if err := WriteInstalledManifest(paths, "https://first", manifest, first); err != nil {
		t.Fatal(err)
	}

	// Update (or re-install)
	second := map[string]InstalledArtifact{StepRuned: {URL: "u2", SHA256: "s2", Path: "/p2"}}
	if err := WriteInstalledManifest(paths, "https://second", manifest, second); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(paths.InstalledManifest)
	if err != nil {
		t.Fatal(err)
	}

	var got InstalledManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ManifestURL != "https://second" {
		t.Errorf("ManifestURL = %q, want second write to have won", got.ManifestURL)
	}
	if got.Artifacts[StepRuned].URL != "u2" {
		t.Errorf("artifact URL = %q, want u2", got.Artifacts[StepRuned].URL)
	}
}
