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

func TestFetchManifest_HappyPath(t *testing.T) {
	body := fmt.Sprintf(`{
		"version": 1,
		"rune_mcp_version": "v0.4.0",
		"runed_bundles": {
			"%[1]s": {"url": "https://example/runed.tar.gz", "sha256": "abc", "size": 22846457}
		}
	}`, PlatformTuple())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	m, err := FetchManifest(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if m.RuneMCPVersion != "v0.4.0" {
		t.Errorf("RuneMCPVersion = %q", m.RuneMCPVersion)
	}
	spec, err := m.BundleForCurrentPlatform()
	if err != nil {
		t.Fatalf("BundleForCurrentPlatform: %v", err)
	}
	if spec.URL != "https://example/runed.tar.gz" || spec.SHA256 != "abc" {
		t.Errorf("bundle spec mismatch: %+v", spec)
	}
}

func TestFetchManifest_EnvOverride(t *testing.T) {
	served := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"version":1,"rune_mcp_version":"v0.4.0","runed_bundles":{"%s":{"url":"u","sha256":"s","size":1}}}`, PlatformTuple())
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"version":99,"rune_mcp_version":"v9.9.9","runed_bundles":{"%s":{"url":"u","sha256":"s","size":1}}}`, PlatformTuple())
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if !errors.Is(err, ErrUnsupportedManifestVersion) {
		t.Fatalf("want ErrUnsupportedManifestVersion, got %v", err)
	}
}

func TestFetchManifest_RejectsEmptyBundles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"version":1,"rune_mcp_version":"v0.4.0","runed_bundles":{}}`)
	}))
	defer srv.Close()

	_, err := FetchManifest(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "runed_bundles is empty") {
		t.Errorf("want empty-bundles error, got %v", err)
	}
}

func TestFetchManifest_RejectsUnknownFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"version":1,"rune_mcp_version":"v0.4.0","runed_bundles":{"%s":{"url":"u","sha256":"s","size":1}},"extra":"field"}`, PlatformTuple())
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

func TestBundleForCurrentPlatform_NotFound(t *testing.T) {
	m := &Manifest{
		Version:      1,
		RunedBundles: map[string]ArtifactSpec{"linux-mips": {URL: "u", SHA256: "s"}},
	}
	_, err := m.BundleForCurrentPlatform()
	if !errors.Is(err, ErrNoBundleForPlatform) {
		t.Errorf("want ErrNoBundleForPlatform, got %v", err)
	}
}
