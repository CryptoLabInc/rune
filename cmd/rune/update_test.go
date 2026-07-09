package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func updateManifestServer(t *testing.T, runeMCPVer, runedVer string) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"version":          1,
			"rune_mcp_version": runeMCPVer,
			"runed_version":    runedVer,
			"platforms": map[string]any{
				bootstrap.PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": "http://example.test/runed", "sha256": "aa", "size": 1},
					"rune_mcp": map[string]any{"url": "http://example.test/rune-mcp", "sha256": "bb", "size": 1},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/manifest.json"
}

func setTestEnv(t *testing.T) *bootstrap.Paths {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	paths, err := bootstrap.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	return paths
}

func writeAudit(t *testing.T, paths *bootstrap.Paths, url, mcpVer, runedVer string) {
	t.Helper()

	rec := &bootstrap.Manifest{Version: 1, RuneMCPVersion: mcpVer, RunedVersion: runedVer}
	artifact := map[string]bootstrap.InstalledArtifact{
		bootstrap.StepRuneMCP: {Path: paths.RuneMCPBinary},
		bootstrap.StepRuned:   {Path: paths.RunedBinary},
	}

	if err := bootstrap.WriteInstalledManifest(paths, url, rec, artifact); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}
}

func TestRunUpdate_NoManifest(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()
	t.Setenv("RUNE_MANIFEST", "")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), nil, &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no manifest URL") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunUpdate_ExtraArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"extra"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unexpected argument") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunUpdate_BadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--bad"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("flag errors must not land on stdout: %q", stdout.String())
	}
}

func TestRunUpdate_CheckReportsOutdated(t *testing.T) {
	paths := setTestEnv(t)
	url := updateManifestServer(t, "v0.2.0", "v0.2.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.1.0", "v0.2.0") // rune-mcp old, runed current

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "rune_mcp") || !strings.Contains(out, "v0.2.0") {
		t.Errorf("expected rune_mcp update reported, got %q", out)
	}
	if strings.Contains(out, "runed:") {
		t.Errorf("runed is current and must not be listed: %q", out)
	}
}

func TestRunUpdate_CheckJSON(t *testing.T) {
	paths := setTestEnv(t)
	url := updateManifestServer(t, "v0.2.0", "v0.2.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.1.0", "v0.2.0")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--check", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}

	var plan bootstrap.UpdateList
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("stdout is not a valid UpdateList JSON: %v\n%s", err, stdout.String())
	}
	if !plan.HasUpdates() {
		t.Errorf("expected an update in the JSON plan: %+v", plan)
	}
}

func TestRunUpdate_CheckUpToDate(t *testing.T) {
	paths := setTestEnv(t)
	url := updateManifestServer(t, "v0.2.0", "v0.2.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.2.0", "v0.2.0")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--check"}, &stdout, &stderr); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "up to date") {
		t.Errorf("expected up-to-date message, got %q", stdout.String())
	}
}

// TODO: runed is not implemented yet
func dummyUpdateServer(t *testing.T, mcpBytes []byte, mcpVer, runedVer string) string {
	t.Helper()

	sum := sha256.Sum256(mcpBytes)
	mcpSHA := hex.EncodeToString(sum[:])

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"version":          1,
			"rune_mcp_version": mcpVer,
			"runed_version":    runedVer,
			"platforms": map[string]any{
				bootstrap.PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": srv.URL + "/runed", "sha256": "dummy-not-downloaded", "size": 1},
					"rune_mcp": map[string]any{"url": srv.URL + "/rune-mcp", "sha256": mcpSHA, "size": len(mcpBytes)},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/rune-mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(mcpBytes)))
		_, _ = w.Write(mcpBytes)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/manifest.json"
}

func TestRunUpdate_ApplyRuneMCP(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("freshly-updated-rune-mcp")
	url := dummyUpdateServer(t, mcp, "v0.2.0", "v0.1.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.1.0", "v0.1.0") // rune-mcp old, runed current

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) != string(mcp) {
		t.Errorf("rune-mcp not swapped on disk: got %q", b)
	}
	if !strings.Contains(stdout.String(), "updated") {
		t.Errorf("expected an applied message, got %q", stdout.String())
	}

	plan, err := bootstrap.CheckUpdate(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("CheckUpdate: %v", err)
	}
	if plan.HasUpdates() {
		t.Errorf("after apply nothing should be outdated: %+v", plan.Artifacts)
	}
}

func TestRunUpdate_ApplyJSON(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("mcp-json-apply")
	url := dummyUpdateServer(t, mcp, "v0.2.0", "v0.2.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.1.0", "v0.2.0") // rune-mcp old, runed current

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}

	var sum struct {
		Applied []struct {
			Step string `json:"step"`
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"applied"`
		Deferred []string `json:"deferred"`
		Error    string   `json:"error"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &sum); err != nil {
		t.Fatalf("stdout is not a valid update summary JSON: %v\n%s", err, stdout.String())
	}
	if len(sum.Applied) != 1 || sum.Applied[0].Step != bootstrap.StepRuneMCP || sum.Applied[0].To != "v0.2.0" {
		t.Errorf("applied summary wrong: %+v", sum.Applied)
	}
	if sum.Error != "" {
		t.Errorf("unexpected error: %q", sum.Error)
	}
}

func runedUpdateServer(t *testing.T, runedBytes []byte, runedVer, mcpVer string) string {
	t.Helper()
	sum := sha256.Sum256(runedBytes)
	runedSHA := hex.EncodeToString(sum[:])

	// Serve runed as a real binary
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"version":          1,
			"rune_mcp_version": mcpVer,
			"runed_version":    runedVer,
			"platforms": map[string]any{
				bootstrap.PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": srv.URL + "/runed", "sha256": runedSHA, "size": len(runedBytes)},
					"rune_mcp": map[string]any{"url": "http://example.invalid/rune-mcp", "sha256": "bb", "size": 1},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/runed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(runedBytes)))
		_, _ = w.Write(runedBytes)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/manifest.json"
}

func fakeSupervisor(t *testing.T, sockPath, respJSON string) {
	t.Helper()

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen supervisor sock: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var req map[string]any
				_ = json.NewDecoder(c).Decode(&req)
				_, _ = c.Write([]byte(respJSON))
			}(conn)
		}
	}()
}

func fakeSupervisorHangup(t *testing.T, sockPath string) {
	t.Helper()

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen supervisor sock: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	// Simluate superviosr which dropped during reload
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				var req map[string]any
				_ = json.NewDecoder(c).Decode(&req)
				_ = c.Close() // hang up without a response
			}(conn)
		}
	}()
}

func TestRunUpdate_RunedReloadRoundTripFails(t *testing.T) {
	paths := setTestEnv(t)
	runed := []byte("staged-runed")
	url := runedUpdateServer(t, runed, "v0.2.0", "v0.2.0")

	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.2.0", "v0.1.0")
	fakeSupervisorHangup(t, paths.SupervisorSock)

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), nil, &stdout, &stderr); code != 1 {
		t.Errorf("exit = %d, want 1 (reload round-trip failed)", code)
	}
	if strings.Contains(stdout.String(), "staged") {
		t.Errorf("supervisor failure after connection must not report 'staged': %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "daemon may be down") {
		t.Errorf("expected a recovery hint on reload failure, got %q", stderr.String())
	}

	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RunedVersion != "v0.1.0" {
		t.Errorf("audit must stay at v0.1.0 when reload failed, got %q", after.RunedVersion)
	}
}

func TestRunUpdate_ApplyRunedReload(t *testing.T) {
	paths := setTestEnv(t)
	runed := []byte("freshly-updated-runed")
	url := runedUpdateServer(t, runed, "v0.2.0", "v0.2.0")

	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.2.0", "v0.1.0") // rune-mcp current, runed old
	fakeSupervisor(t, paths.SupervisorSock, `{"ok":true,"pid":123}`)

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if b, _ := os.ReadFile(paths.RunedBinary); string(b) != string(runed) {
		t.Errorf("runed not swapped on disk: got %q", b)
	}
	if !strings.Contains(stdout.String(), "reloaded") {
		t.Errorf("expected 'daemon reloaded', got %q", stdout.String())
	}

	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RunedVersion != "v0.2.0" {
		t.Errorf("runed audit not bumped after reload: %q", after.RunedVersion)
	}
}

func TestRunUpdate_RunedNoSupervisor(t *testing.T) {
	paths := setTestEnv(t)
	runed := []byte("staged-runed")
	url := runedUpdateServer(t, runed, "v0.2.0", "v0.2.0")

	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.2.0", "v0.1.0")
	// no supervisor listening

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "staged") {
		t.Errorf("expected 'staged; applies on next daemon start', got %q", stdout.String())
	}

	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RunedVersion != "v0.2.0" {
		t.Errorf("runed audit should be bumped after staging: %q", after.RunedVersion)
	}
}

func TestRunUpdate_RunedReloadFail(t *testing.T) {
	paths := setTestEnv(t)
	runed := []byte("bad-runed")
	url := runedUpdateServer(t, runed, "v0.2.0", "v0.2.0")

	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.2.0", "v0.1.0")
	fakeSupervisor(t, paths.SupervisorSock, `{"ok":false,"error":"restart failed to start"}`)

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), nil, &stdout, &stderr); code != 1 {
		t.Errorf("exit = %d, want 1 (reload failed)", code)
	}

	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RunedVersion != "v0.1.0" {
		t.Errorf("runed audit must stay at v0.1.0 when reload failed, got %q", after.RunedVersion)
	}
}

func pluginOutdatedServer(t *testing.T, mcpBytes []byte, mcpVer, runedVer, pluginVer, minVer string) string {
	t.Helper()
	sum := sha256.Sum256(mcpBytes)
	mcpSHA := hex.EncodeToString(sum[:])

	var srv *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"version":            1,
			"rune_mcp_version":   mcpVer,
			"runed_version":      runedVer,
			"plugin_version":     pluginVer,
			"min_plugin_version": minVer,
			"platforms": map[string]any{
				bootstrap.PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": srv.URL + "/runed", "sha256": "dummy-not-downloaded", "size": 1},
					"rune_mcp": map[string]any{"url": srv.URL + "/rune-mcp", "sha256": mcpSHA, "size": len(mcpBytes)},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/rune-mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(mcpBytes)))
		_, _ = w.Write(mcpBytes)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL + "/manifest.json"
}

func writePluginRoot(t *testing.T, version string) string {
	t.Helper()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"name":"rune","version":%q}`, version)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestRunUpdate_RefusesBelowPluginFloor(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("new-mcp-should-not-land")
	url := pluginOutdatedServer(t, mcp, "v0.2.0", "v0.1.0", "0.5.0", "0.5.0")
	t.Setenv("RUNE_MANIFEST", url)

	writeAudit(t, paths, url, "v0.1.0", "v0.1.0") // rune-mcp outdated, runed current
	root := writePluginRoot(t, "0.4.1")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--plugin-root", root}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1 (refused below plugin floor); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing to apply") || !strings.Contains(stderr.String(), "0.5.0") {
		t.Errorf("expected a refusal naming the floor, got %q", stderr.String())
	}
	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) == string(mcp) {
		t.Error("rune-mcp must NOT be swapped when the update is refused")
	}
	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RuneMCPVersion != "v0.1.0" {
		t.Errorf("audit must stay at v0.1.0 when refused, got %q", after.RuneMCPVersion)
	}
}

func TestRunUpdate_AllowOutdatedBypass(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("bypassed-mcp")
	url := pluginOutdatedServer(t, mcp, "v0.2.0", "v0.1.0", "0.5.0", "0.5.0")
	t.Setenv("RUNE_MANIFEST", url)

	writeAudit(t, paths, url, "v0.1.0", "v0.1.0")
	root := writePluginRoot(t, "0.4.1")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--plugin-root", root, "--allow-plugin-outdated"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (bypassed); stderr=%q", code, stderr.String())
	}
	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) != string(mcp) {
		t.Errorf("rune-mcp should be swapped under --allow-plugin-outdated, got %q", b)
	}
	if !strings.Contains(stderr.String(), "allow-plugin-outdated") {
		t.Errorf("expect outdated warning, got %q", stderr.String())
	}
	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RuneMCPVersion != "v0.2.0" {
		t.Errorf("audit should be bumped under bypass, got %q", after.RuneMCPVersion)
	}
}

func TestRunUpdate_PluginBehindAdvisory(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("advisory-mcp")

	url := pluginOutdatedServer(t, mcp, "v0.2.0", "v0.1.0", "0.6.0", "0.4.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.1.0", "v0.1.0")
	root := writePluginRoot(t, "0.4.1")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--plugin-root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (advisory only); stderr=%q", code, stderr.String())
	}
	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) != string(mcp) {
		t.Errorf("rune-mcp should be swapped when only an advisory applies, got %q", b)
	}
	if !strings.Contains(stdout.String(), "plugin package is 0.4.1") || !strings.Contains(stdout.String(), "0.6.0") {
		t.Errorf("expected a plugin advisory note, got %q", stdout.String())
	}
}

func TestRunUpdate_CheckReportPluginOutdated(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("irrelevant-not-applied-in-check")
	url := pluginOutdatedServer(t, mcp, "v0.2.0", "v0.1.0", "0.5.0", "0.5.0")
	t.Setenv("RUNE_MANIFEST", url)

	writeAudit(t, paths, url, "v0.1.0", "v0.1.0")
	root := writePluginRoot(t, "0.4.1")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--check", "--plugin-root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (check is read-only); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "plugin outdated") || !strings.Contains(stdout.String(), "0.5.0") {
		t.Errorf("expected --check to report the plugin outdated, got %q", stdout.String())
	}
	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) == string(mcp) {
		t.Error("--check must not swap the binary")
	}
}

func TestRunUpdate_RefuseBelowFloorJSON(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("json-refuse-mcp")
	url := pluginOutdatedServer(t, mcp, "v0.2.0", "v0.1.0", "0.5.0", "0.5.0")
	t.Setenv("RUNE_MANIFEST", url)

	writeAudit(t, paths, url, "v0.1.0", "v0.1.0")
	root := writePluginRoot(t, "0.4.1")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--json", "--plugin-root", root}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit = %d, want 1 (refused, JSON); stderr=%q", code, stderr.String())
	}

	var sum struct {
		Applied []appliedUpdate `json:"applied"`
		Error   string          `json:"error"`
		Plugin  *struct {
			Installed    string `json:"installed"`
			Minimum      string `json:"minimum"`
			BelowMinimum bool   `json:"below_minimum"`
		} `json:"plugin"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &sum); err != nil {
		t.Fatalf("stdout is not a valid update summary JSON: %v\n%s", err, stdout.String())
	}
	if sum.Error == "" || len(sum.Applied) != 0 {
		t.Errorf("want non-empty error and empty applied, got error=%q applied=%+v", sum.Error, sum.Applied)
	}
	if sum.Plugin == nil || !sum.Plugin.BelowMinimum || sum.Plugin.Installed != "0.4.1" || sum.Plugin.Minimum != "0.5.0" {
		t.Errorf("plugin compat not reported correctly in JSON: %+v", sum.Plugin)
	}
	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) == string(mcp) {
		t.Error("refused update must not swap the binary (JSON mode)")
	}
}

// '--only'
func TestRunUpdate_OnlyRuneMCP(t *testing.T) {
	paths := setTestEnv(t)
	mcp := []byte("only-mcp")
	url := dummyUpdateServer(t, mcp, "v0.2.0", "v0.2.0")
	t.Setenv("RUNE_MANIFEST", url)
	writeAudit(t, paths, url, "v0.1.0", "v0.1.0")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--only", "rune_mcp"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, stderr.String())
	}

	if b, _ := os.ReadFile(paths.RuneMCPBinary); string(b) != string(mcp) {
		t.Errorf("rune-mcp should be swapped, got %q", b)
	}

	if strings.Contains(stdout.String(), "runed") {
		t.Errorf("--only rune_mcp must not touch runed: %q", stdout.String())
	}

	after, _ := bootstrap.ReadInstalledManifest(paths)
	if after.RuneMCPVersion != "v0.2.0" {
		t.Errorf("rune-mcp audit = %q, want v0.2.0", after.RuneMCPVersion)
	}
	if after.RunedVersion != "v0.1.0" {
		t.Errorf("runed audit must stay at v0.1.0 (not selected), got %q", after.RunedVersion)
	}
}

func TestRunUpdate_OnlyUnknown(t *testing.T) {
	t.Setenv("RUNE_MANIFEST", "http://example.invalid/manifest.json")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--only", "bogus"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2 for an unknown --only artifact", code)
	}
	if !strings.Contains(stderr.String(), "unknown artifact") {
		t.Errorf("stderr = %q, want an --only validation error", stderr.String())
	}
}

func TestRunUpdate_OnlyDegenerate(t *testing.T) {
	t.Setenv("RUNE_MANIFEST", "http://example.invalid/manifest.json")

	var stdout, stderr bytes.Buffer
	if code := runUpdate(context.Background(), []string{"--only", " , "}, &stdout, &stderr); code != 2 {
		t.Errorf("exit = %d, want 2 for a degenerate --only value", code)
	}
	if !strings.Contains(stderr.String(), "no valid artifacts") {
		t.Errorf("stderr = %q, want a 'no valid artifacts' error", stderr.String())
	}
}
