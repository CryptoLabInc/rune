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
	srv      *httptest.Server
	runed    []byte
	runeMCP  []byte
	runedSHA string
	mcpSHA   string

	// Override SHA in the manifest to simulate a checksum mismatch
	mismatchStep string // "" | StepRuned | StepRuneMCP

	hits map[string]int
}

func newFixture(t *testing.T) *installFixture {
	t.Helper()
	fx := &installFixture{
		runed:   []byte("runed-binary-bytes"),
		runeMCP: []byte("rune-mcp-binary-bytes"),
		hits:    map[string]int{},
	}
	fx.runedSHA = sha256Hex(fx.runed)
	fx.mcpSHA = sha256Hex(fx.runeMCP)

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		runedSHA := fx.runedSHA
		mcpSHA := fx.mcpSHA

		switch fx.mismatchStep {
		case StepRuned:
			runedSHA = "00" + runedSHA[2:]
		case StepRuneMCP:
			mcpSHA = "00" + mcpSHA[2:]
		}

		manifest := map[string]any{
			"version":          1,
			"rune_mcp_version": "v0.1.0-test",
			"runed_version":    "v0.1.0-test",
			"platforms": map[string]any{
				PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": fx.srv.URL + "/runed", "sha256": runedSHA, "size": len(fx.runed)},
					"rune_mcp": map[string]any{"url": fx.srv.URL + "/rune-mcp", "sha256": mcpSHA, "size": len(fx.runeMCP)},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	})

	serveBinary := func(name string, body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			fx.hits[name]++
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			_, _ = w.Write(body)
		}
	}

	mux.HandleFunc("/runed", serveBinary("/runed", fx.runed))
	mux.HandleFunc("/rune-mcp", serveBinary("/rune-mcp", fx.runeMCP))

	fx.srv = httptest.NewServer(mux)
	t.Cleanup(fx.srv.Close)

	return fx
}

func (fx *installFixture) manifestURL() string { return fx.srv.URL + "/manifest.json" }

func TestInstall_DoesNotWriteRuneConfig(t *testing.T) {
	rune, _ := setRealms(t)
	fx := newFixture(t)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rune, "config.json")); !os.IsNotExist(err) {
		t.Errorf("~/.rune/config.json should not be written by rune install; got err=%v", err)
	}
}

func TestInstall_HappyPath_PlacesBothBinaries(t *testing.T) {
	rune, runed := setRealms(t)
	fx := newFixture(t)

	r, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !r.OK {
		t.Errorf("Result.OK = false, want true (r=%+v)", r)
	}

	for _, p := range []string{
		filepath.Join(runed, "bin", "runed"),
		filepath.Join(rune, "bin", "rune-mcp"),
	} {
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing %s: %v", p, err)
			continue
		}
		// chmod 0755 was applied
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not executable: mode=%v", p, info.Mode())
		}
	}
	if fx.hits["/runed"] != 1 || fx.hits["/rune-mcp"] != 1 {
		t.Errorf("expected one hit per artifact; got %+v", fx.hits)
	}
	if _, err := os.Stat(filepath.Join(runed, "bin", "llama-server")); !os.IsNotExist(err) {
		t.Errorf("llama-server should NOT be installed by rune install; got err=%v", err)
	}
}

func TestInstall_Idempotent_SkipsExistingBinaries(t *testing.T) {
	setRealms(t)
	fx := newFixture(t)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	beforeHits := map[string]int{}
	for k, v := range fx.hits {
		beforeHits[k] = v
	}

	r, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}

	// Second install must not re-download any binary
	for _, name := range []string{"/runed", "/rune-mcp"} {
		if fx.hits[name] != beforeHits[name] {
			t.Errorf("second install re-downloaded %s: hits=%d, want %d", name, fx.hits[name], beforeHits[name])
		}
	}

	skipMap := map[string]bool{}
	for _, s := range r.Skipped {
		skipMap[s] = true
	}
	for _, step := range []string{StepRuned, StepRuneMCP} {
		if !skipMap[step] {
			t.Errorf("Skipped missing %s: %v", step, r.Skipped)
		}
	}
}

func TestInstall_Force_ReDownloadsAllBinaries(t *testing.T) {
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

	for _, name := range []string{"/runed", "/rune-mcp"} {
		if fx.hits[name] != 2 {
			t.Errorf("force should re-download %s: hits=%d, want 2", name, fx.hits[name])
		}
	}
}

func TestInstall_ChecksumMismatch_PartialFailure(t *testing.T) {
	rune, _ := setRealms(t)
	fx := newFixture(t)
	fx.mismatchStep = StepRuneMCP

	r, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()})
	if err == nil {
		t.Fatalf("expected checksum error")
	}
	if r.Failed[StepRuneMCP] == "" {
		t.Errorf("Failed missing %s: %+v", StepRuneMCP, r.Failed)
	}
	if r.Status != "partial" {
		t.Errorf("Status=%q, want partial", r.Status)
	}
	if _, err := os.Stat(filepath.Join(rune, "bin", "rune-mcp")); !os.IsNotExist(err) {
		t.Errorf("rune-mcp should not exist after checksum failure (err=%v)", err)
	}
}
