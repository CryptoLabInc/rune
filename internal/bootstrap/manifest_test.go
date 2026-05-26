package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fullManifestJSON(extraFields string) string {
	return fmt.Sprintf(`{
		"version": 1,
		"rune_mcp_version": "v0.1.0",
		"runed_version": "v0.1.0-alpha.1",
		"platforms": {
			"%[1]s": {
				"runed":    {"url": "https://example/runed-%[1]s",    "sha256": "aaa", "size": 8123456},
				"rune_mcp": {"url": "https://example/rune-mcp-%[1]s", "sha256": "ccc", "size": 16234567}
			}
		}%[2]s
	}`, PlatformTuple(), extraFields)
}

func TestFetchManifest_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fullManifestJSON("")))
	}))
	defer srv.Close()

	m, err := FetchManifest(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if m.RuneMCPVersion != "v0.1.0" {
		t.Errorf("RuneMCPVersion = %q", m.RuneMCPVersion)
	}
	if m.RunedVersion != "v0.1.0-alpha.1" {
		t.Errorf("RunedVersion = %q", m.RunedVersion)
	}
	arts, err := m.ArtifactsForCurrentPlatform()
	if err != nil {
		t.Fatalf("ArtifactsForCurrentPlatform: %v", err)
	}
	if arts.Runed.SHA256 != "aaa" || arts.RuneMCP.SHA256 != "ccc" {
		t.Errorf("artifact SHA256s mismatch: %+v", arts)
	}
	if !strings.HasPrefix(arts.RuneMCP.URL, "https://example/rune-mcp-") {
		t.Errorf("rune_mcp URL: %q", arts.RuneMCP.URL)
	}
}

func TestFetchManifest_EnvOverride(t *testing.T) {
	served := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fullManifestJSON("")))
	}))
	defer srv.Close()

	t.Setenv(envManifest, srv.URL)
	if _, err := FetchManifest(context.Background(), "https://default/manifest.json"); err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if !served {
		t.Errorf("env override should have routed to httptest server")
	}
}

func TestFetchManifest_RejectsUnsupportedVersion(t *testing.T) {
	body := strings.Replace(fullManifestJSON(""), `"version": 1`, `"version": 99`, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if !errors.Is(err, ErrUnsupportedManifestVersion) {
		t.Fatalf("want ErrUnsupportedManifestVersion, got %v", err)
	}
}

func TestFetchManifest_RejectsEmptyPlatforms(t *testing.T) {
	body := `{"version":1,"rune_mcp_version":"v0.1.0","runed_version":"v0.1.0","platforms":{}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "platforms is empty") {
		t.Errorf("want empty-platforms error, got %v", err)
	}
}

func TestFetchManifest_RejectsUnknownFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fullManifestJSON(`, "extra": "field"`)))
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("want unknown-field rejection, got %v", err)
	}
}

func TestFetchManifest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("want HTTP 404, got %v", err)
	}
}

func TestArtifactsForCurrentPlatform_NotFound(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Platforms: map[string]PlatformArtifacts{
			"linux-mips": {
				Runed:   ArtifactSpec{URL: "u", SHA256: "s"},
				RuneMCP: ArtifactSpec{URL: "u", SHA256: "s"},
			},
		},
	}
	_, err := m.ArtifactsForCurrentPlatform()
	if !errors.Is(err, ErrNoArtifactForPlatform) {
		t.Errorf("want ErrNoArtifactForPlatform, got %v", err)
	}
}

func TestArtifactsForCurrentPlatform_IncompleteArtifact(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Platforms: map[string]PlatformArtifacts{
			PlatformTuple(): {
				Runed:   ArtifactSpec{URL: "u", SHA256: "s"},
				RuneMCP: ArtifactSpec{URL: "u", SHA256: ""}, // missing sha256
			},
		},
	}

	_, err := m.ArtifactsForCurrentPlatform()
	if err == nil || !strings.Contains(err.Error(), "rune_mcp") {
		t.Errorf("want missing-artifact error for rune_mcp, got %v", err)
	}
}
