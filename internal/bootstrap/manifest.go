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
//	{
//	  "version": 1,
//	  "rune_mcp_version": "v0.4.0",
//	  "runed_bundles": {
//	    "linux-amd64":  {"url": "...", "sha256": "...", "size": 22846457},
//	    "darwin-arm64": {"url": "...", "sha256": "...", "size": 21998765}
//	  }
//	}
type Manifest struct {
	Version        int                     `json:"version"`
	RuneMCPVersion string                  `json:"rune_mcp_version"`
	RunedBundles   map[string]ArtifactSpec `json:"runed_bundles"`
}

type ArtifactSpec struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

var ErrUnsupportedManifestVersion = errors.New("manifest: unsupported version")
var ErrNoBundleForPlatform = errors.New("manifest: no bundle for this platform")

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

	const maxBody = 1 << 20 // multi-MB might be misconfiguration
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
	if len(m.RunedBundles) == 0 {
		return nil, errors.New("manifest: runed_bundles is empty")
	}
	return &m, nil
}

func (m *Manifest) BundleForCurrentPlatform() (ArtifactSpec, error) {
	tuple := PlatformTuple()
	spec, ok := m.RunedBundles[tuple]
	if !ok {
		return ArtifactSpec{}, fmt.Errorf("%w: %s", ErrNoBundleForPlatform, tuple)
	}
	if spec.URL == "" || spec.SHA256 == "" {
		return ArtifactSpec{}, fmt.Errorf("manifest: bundle for %s missing url or sha256", tuple)
	}
	return spec, nil
}
