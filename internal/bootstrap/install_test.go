package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type installFixture struct {
	srv               *httptest.Server
	bundleBody        []byte
	bundleSHA         string
	manifestBundleSHA string
	bundleHits        int
}

func newFixture(t *testing.T) *installFixture {
	t.Helper()
	bundle := makeBundleTarball(t, map[string][]byte{
		"bin/runed":        []byte("runed-binary-bytes"),
		"bin/llama-server": []byte("llama-binary-bytes"),
	})
	fx := &installFixture{
		bundleBody: bundle,
		bundleSHA:  sha256Hex(bundle),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		bundleSHA := fx.manifestBundleSHA
		if bundleSHA == "" {
			bundleSHA = fx.bundleSHA
		}
		manifest := map[string]any{
			"version":          1,
			"rune_mcp_version": "v0.4.0-test",
			"runed_bundles": map[string]any{
				PlatformTuple(): map[string]any{
					"url":    fx.srv.URL + "/bundle.tar.gz",
					"sha256": bundleSHA,
					"size":   len(fx.bundleBody),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	})
	mux.HandleFunc("/bundle.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		fx.bundleHits++
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fx.bundleBody)))
		_, _ = w.Write(fx.bundleBody)
	})

	fx.srv = httptest.NewServer(mux)
	t.Cleanup(fx.srv.Close)

	return fx
}

func (fx *installFixture) manifestURL() string { return fx.srv.URL + "/manifest.json" }

func TestInstall_NeverTouchesRune(t *testing.T) {
	rune, _ := setRealms(t)
	fx := newFixture(t)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if _, err := os.Stat(rune); !os.IsNotExist(err) {
		t.Errorf("~/.rune/ should not have been created by rune install; got err=%v", err)
	}
}

func TestInstall_Idempotent_SkipsExistingBundle(t *testing.T) {
	setRealms(t)
	fx := newFixture(t)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if fx.bundleHits != 1 {
		t.Fatalf("first install: bundleHits=%d, want 1", fx.bundleHits)
	}

	r, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if fx.bundleHits != 1 {
		t.Errorf("second install re-downloaded bundle: hits=%d", fx.bundleHits)
	}
	skipMap := map[string]bool{}
	for _, s := range r.Skipped {
		skipMap[s] = true
	}
	if !skipMap[StepBundle] {
		t.Errorf("Skipped missing bundle: %v", r.Skipped)
	}
}

func TestInstall_Force_ReDownloadsBundle(t *testing.T) {
	setRealms(t)
	fx := newFixture(t)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, err := Install(context.Background(), InstallOptions{
		ManifestURL: fx.manifestURL(),
		Force:       true,
	}); err != nil {
		t.Fatalf("force install: %v", err)
	}
	if fx.bundleHits != 2 {
		t.Errorf("force should re-download bundle: hits=%d, want 2", fx.bundleHits)
	}
}

func TestInstall_BundleChecksumMismatch_PartialFailure(t *testing.T) {
	_, runed := setRealms(t)
	fx := newFixture(t)
	fx.manifestBundleSHA = "00" + fx.bundleSHA[2:]

	r, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()})
	if err == nil {
		t.Fatalf("expected bundle checksum error")
	}
	if r.Failed[StepBundle] == "" {
		t.Errorf("Failed missing bundle: %+v", r.Failed)
	}
	if r.Status != "partial" {
		t.Errorf("Status=%q, want partial", r.Status)
	}
	if _, err := os.Stat(filepath.Join(runed, "bin", "runed")); !os.IsNotExist(err) {
		t.Errorf("binary should not exist after checksum failure (err=%v)", err)
	}
}
