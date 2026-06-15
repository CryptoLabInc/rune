package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestInstall_RepairCorruptBinary(t *testing.T) {
	rune, _ := setRealms(t)
	fx := newFixture(t)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Simulate partial/stale rune-mcp
	mcpPath := filepath.Join(rune, "bin", "rune-mcp")
	if err := os.WriteFile(mcpPath, []byte("corrupt-truncated"), 0o755); err != nil {
		t.Fatalf("corrupt rune-mcp: %v", err)
	}
	beforeMCP, beforeRuned := fx.hits["/rune-mcp"], fx.hits["/runed"]

	// Non-force install detect sha mismatch and repair it
	r, err := Install(context.Background(), InstallOptions{ManifestURL: fx.manifestURL()})
	if err != nil {
		t.Fatalf("repair install: %v", err)
	}
	if !r.OK {
		t.Errorf("Result.OK = false, want true (r=%+v)", r)
	}

	if fx.hits["/rune-mcp"] != beforeMCP+1 {
		t.Errorf("rune-mcp not re-downloaded: hits=%d, want %d", fx.hits["/rune-mcp"], beforeMCP+1)
	}
	if got, _ := os.ReadFile(mcpPath); string(got) != string(fx.runeMCP) {
		t.Errorf("rune-mcp not repaired: on-disk=%q, want %q", got, fx.runeMCP)
	}

	// Skip healthy runed binary
	if fx.hits["/runed"] != beforeRuned {
		t.Errorf("healthy runed re-downloaded: hits=%d, want %d", fx.hits["/runed"], beforeRuned)
	}

	skipped := map[string]bool{}
	for _, s := range r.Skipped {
		skipped[s] = true
	}
	if !skipped[StepRuned] {
		t.Errorf("Skipped missing %s: %v", StepRuned, r.Skipped)
	}
	if skipped[StepRuneMCP] {
		t.Errorf("corrupt %s should have been repaired, but not skipped: %v", StepRuneMCP, r.Skipped)
	}
}

func TestInstall_SkipVerifiedTarball(t *testing.T) {
	setRealms(t)
	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	runedTar := tarGz(t, filepath.Base(paths.RunedBinary), []byte("RUNED"))
	mcpTar := tarGz(t, filepath.Base(paths.RuneMCPBinary), []byte("RUNE-MCP"))
	url := tarballManifestServer(t, runedTar, mcpTar)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: url}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	r, err := Install(context.Background(), InstallOptions{ManifestURL: url})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if r.Status != "no_op" {
		t.Errorf("Status=%q, want no_op (both tar.gz artifacts verified and skipped)", r.Status)
	}
	if len(r.Installed) != 0 {
		t.Errorf("Installed=%v, want empty (nothing re-extracted)", r.Installed)
	}
}

func TestInstall_RepairCorruptTarball(t *testing.T) {
	setRealms(t)
	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	runedTar := tarGz(t, filepath.Base(paths.RunedBinary), []byte("RUNED"))
	mcpTar := tarGz(t, filepath.Base(paths.RuneMCPBinary), []byte("RUNE-MCP"))
	url := tarballManifestServer(t, runedTar, mcpTar)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: url}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Make extracted runed binary as corrupted
	if err := os.WriteFile(paths.RunedBinary, []byte("corrupt"), 0o755); err != nil {
		t.Fatalf("corrupt runed: %v", err)
	}

	r, err := Install(context.Background(), InstallOptions{ManifestURL: url})
	if err != nil {
		t.Fatalf("repair install: %v", err)
	}
	if !r.OK {
		t.Errorf("Result.OK = false, want true (r=%+v)", r)
	}

	// Corrupted runed should be re-installed
	if got, _ := os.ReadFile(paths.RunedBinary); string(got) != "RUNED" {
		t.Errorf("runed not repaired: on-disk=%q, want %q", got, "RUNED")
	}

	skipped := map[string]bool{}
	for _, s := range r.Skipped {
		skipped[s] = true
	}
	if skipped[StepRuned] {
		t.Errorf("corrupt %s should have been repaired, but skipped: %v", StepRuned, r.Skipped)
	}
	if !skipped[StepRuneMCP] {
		t.Errorf("healthy %s should be skipped: %v", StepRuneMCP, r.Skipped)
	}
}

func TestInstall_TarballNoRecordedHash(t *testing.T) {
	// tar.gz repair is conditional on a recorded dest hash. When installed.json
	// has no hash for the step (a prior audit that couldn't hash the extracted
	// file, or a missing/partial record), the skip path degrades to
	// existence-only: a corrupt binary is reused, not repaired. This pins that
	// documented fallback so it can't silently change.
	setRealms(t)
	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	runedTar := tarGz(t, filepath.Base(paths.RunedBinary), []byte("RUNED"))
	mcpTar := tarGz(t, filepath.Base(paths.RuneMCPBinary), []byte("RUNE-MCP"))
	url := tarballManifestServer(t, runedTar, mcpTar)

	if _, err := Install(context.Background(), InstallOptions{ManifestURL: url}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	rec, err := ReadInstalledManifest(paths)
	if err != nil {
		t.Fatalf("read installed.json: %v", err)
	}

	a := rec.Artifacts[StepRuned]
	a.DestSHA256 = "" // extracted runed binary is not hashed
	rec.Artifacts[StepRuned] = a

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal installed.json: %v", err)
	}
	if err := os.WriteFile(paths.InstalledManifest, data, 0o600); err != nil {
		t.Fatalf("rewrite installed.json: %v", err)
	}

	// Make runed as corrupted
	if err := os.WriteFile(paths.RunedBinary, []byte("corrupt"), 0o755); err != nil {
		t.Fatalf("corrupt runed: %v", err)
	}

	r, err := Install(context.Background(), InstallOptions{ManifestURL: url})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	// 'runed' is not repaired if hash is not recorded
	skipped := map[string]bool{}
	for _, s := range r.Skipped {
		skipped[s] = true
	}
	if !skipped[StepRuned] {
		t.Errorf("with no recorded hash, runed re-install should be skipped; Skipped=%v", r.Skipped)
	}
	if got, _ := os.ReadFile(paths.RunedBinary); string(got) != "corrupt" {
		t.Errorf("repair should be skipped; runed on-disk=%q, want %q", got, "corrupt")
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

func tarGz(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "artifact.tar.gz")

	makeTarball(t, p, map[string]struct {
		body []byte
		mode int64
	}{
		name: {body: body, mode: 0o755},
	})

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read tarball: %v", err)
	}

	return b
}

func tarballManifestServer(t *testing.T, runedTar, mcpTar []byte) string {
	t.Helper()

	var srv *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"version":          1,
			"rune_mcp_version": "v0.1.0-test",
			"runed_version":    "v0.1.0-test",
			"platforms": map[string]any{
				PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": srv.URL + "/runed.tar.gz", "sha256": sha256Hex(runedTar), "size": len(runedTar), "extract": "tar.gz"},
					"rune_mcp": map[string]any{"url": srv.URL + "/rune-mcp.tar.gz", "sha256": sha256Hex(mcpTar), "size": len(mcpTar), "extract": "tar.gz"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/runed.tar.gz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(runedTar) })
	mux.HandleFunc("/rune-mcp.tar.gz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(mcpTar) })

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/manifest.json"
}

func TestInstall_TarballExtract(t *testing.T) {
	setRealms(t)
	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	runedTar := tarGz(t, filepath.Base(paths.RunedBinary), []byte("RUNED"))
	mcpTar := tarGz(t, filepath.Base(paths.RuneMCPBinary), []byte("RUNE-MCP"))

	r, err := Install(context.Background(), InstallOptions{ManifestURL: tarballManifestServer(t, runedTar, mcpTar)})
	if err != nil {
		t.Fatalf("Install (tar.gz): %v", err)
	}
	if !r.OK {
		t.Errorf("Result.OK = false, want true (r=%+v)", r)
	}

	for _, p := range []string{paths.RunedBinary, paths.RuneMCPBinary} {
		info, statErr := os.Stat(p)
		if statErr != nil {
			t.Errorf("extracted binary missing at %s: %v", p, statErr)
			continue
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not executable: mode=%v", p, info.Mode())
		}
	}
}

func TestInstall_TarballMissingExpectedFile(t *testing.T) {
	setRealms(t)
	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	runedTar := tarGz(t, filepath.Base(paths.RunedBinary), []byte("RUNED"))
	mcpTar := tarGz(t, "not-rune-mcp", []byte("WRONG-NAME")) // valid archive but wrong entry

	r, err := Install(context.Background(), InstallOptions{ManifestURL: tarballManifestServer(t, runedTar, mcpTar)})
	if err == nil {
		t.Fatal("expected error when the tarball lacks the expected file")
	}
	if !strings.Contains(err.Error(), "did not have expected file") {
		t.Errorf("err = %v, want 'did not have expected file'", err)
	}
	if r.Status != "partial" {
		t.Errorf("Status=%q, want partial", r.Status)
	}
	if r.Failed[StepRuneMCP] == "" {
		t.Errorf("Failed missing %s: %+v", StepRuneMCP, r.Failed)
	}
	if fileExists(paths.RuneMCPBinary) {
		t.Errorf("%s should not exist when the tarball lacks it", paths.RuneMCPBinary)
	}
}

func TestInstall_CorruptTarball(t *testing.T) {
	setRealms(t)
	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	runedTar := tarGz(t, filepath.Base(paths.RunedBinary), []byte("RUNED"))
	corrupt := []byte("this is not a gzip stream") // SHA and size are matched but broken tarball

	r, err := Install(context.Background(), InstallOptions{ManifestURL: tarballManifestServer(t, runedTar, corrupt)})
	if err == nil {
		t.Fatal("expected extract error for a corrupt gzip body")
	}
	if !strings.Contains(err.Error(), "extract") {
		t.Errorf("err = %v, want an extract failure", err)
	}
	if r.Status != "partial" {
		t.Errorf("Status=%q, want partial", r.Status)
	}
	if r.Failed[StepRuneMCP] == "" {
		t.Errorf("Failed missing %s: %+v", StepRuneMCP, r.Failed)
	}
	if fileExists(paths.RuneMCPBinary) {
		t.Errorf("%s should not exist after a failed extract", paths.RuneMCPBinary)
	}
}
