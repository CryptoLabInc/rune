package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const ManifestVersion = 1
const defaultManifestFetchTimeout = 30 * time.Second

// Example JSON:
//
//	{
//	  "version": 1,
//	  "rune_mcp_version": "v0.1.0",
//	  "runed_version":    "v0.1.0-alpha.1",
//	  "platforms": {
//	    "linux-amd64": {
//	      "runed":    {"url": "...", "sha256": "...", "size": 8123456},
//	      "rune_mcp": {"url": "...", "sha256": "...", "size": 16234567}
//	    },
//	    "darwin-arm64": { ... }
//	  }
//	}

type Manifest struct {
	Version        int                          `json:"version"`
	RuneMCPVersion string                       `json:"rune_mcp_version"`
	RunedVersion   string                       `json:"runed_version"`
	Platforms      map[string]PlatformArtifacts `json:"platforms"`
}

type PlatformArtifacts struct {
	Runed   ArtifactSpec `json:"runed"`    // ~/.runed/bin
	RuneMCP ArtifactSpec `json:"rune_mcp"` // ~/.rune/bin
}

type ArtifactSpec struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size,omitempty"` // optional; used for progress UX and sanity check
}

var (
	ErrUnsupportedManifestVersion = errors.New("manifest: unsupported version")
	ErrNoArtifactForPlatform      = errors.New("manifest: no artifacts for this platform")
)

func FetchManifest(ctx context.Context, manifestURL string) (*Manifest, error) {
	if v := os.Getenv(envManifest); v != "" {
		manifestURL = v
	}
	if manifestURL == "" {
		return nil, errors.New("manifest: no URL provided (default missing; set RUNE_MANIFEST?)")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultManifestFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("manifest: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manifest: GET %s: %w", manifestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest: GET %s: HTTP %d", manifestURL, resp.StatusCode)
	}

	const maxBody = 1 << 20 // multi-MB would be misconfiguration
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("manifest: read body: %w", err)
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("manifest: body exceeds %d bytes", maxBody)
	}

	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: parse JSON: %w", err)
	}
	if m.Version != ManifestVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedManifestVersion, m.Version, ManifestVersion)
	}
	if len(m.Platforms) == 0 {
		return nil, errors.New("manifest: platforms is empty")
	}

	return &m, nil
}

func (m *Manifest) ArtifactsForCurrentPlatform() (PlatformArtifacts, error) {
	tuple := PlatformTuple()
	arts, ok := m.Platforms[tuple]
	if !ok {
		return PlatformArtifacts{}, fmt.Errorf("%w: %s", ErrNoArtifactForPlatform, tuple)
	}

	for _, pair := range []struct {
		name string
		spec ArtifactSpec
	}{
		{"runed", arts.Runed},
		{"rune_mcp", arts.RuneMCP},
	} {
		if pair.spec.URL == "" || pair.spec.SHA256 == "" {
			return PlatformArtifacts{}, fmt.Errorf("manifest: %s artifact for %s missing url or sha256", pair.name, tuple)
		}
	}

	return arts, nil
}
