package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func TestRunVersion_PrintConstants(t *testing.T) {
	saved := manifestURL
	manifestURL = "https://example/manifest.json"
	defer func() { manifestURL = saved }()

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	var buf bytes.Buffer
	if code := runVersion(&buf); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "rune ") {
		t.Errorf("missing version prefix: %q", out)
	}
	if !strings.Contains(out, "https://example/manifest.json") {
		t.Errorf("missing manifest URL: %q", out)
	}
}

func TestRunVersion_EmptyManifest(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()
	t.Setenv("RUNE_MANIFEST", "")

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	var buf bytes.Buffer
	_ = runVersion(&buf)
	// Match the actual "manifest missing" copy in version.go's empty
	// branch — keep the assertion loose so future copy edits don't
	// require keeping two strings in lockstep.
	if !strings.Contains(buf.String(), "manifest missing") {
		t.Errorf("empty manifest should be flagged; got %q", buf.String())
	}
}

func TestRunVersion_ShowsInstalledVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	paths, err := bootstrap.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := paths.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	m := &bootstrap.Manifest{Version: 1, RuneMCPVersion: "v0.1.1", RunedVersion: "v0.1.3"}
	arts := map[string]bootstrap.InstalledArtifact{
		bootstrap.StepRuneMCP: {Path: paths.RuneMCPBinary, SHA256: "aaa"},
		bootstrap.StepRuned:   {Path: paths.RunedBinary, SHA256: "bbb"},
	}

	if err := bootstrap.WriteInstalledManifest(paths, "https://example/manifest.json", m, arts); err != nil {
		t.Fatalf("WriteInstalledManifest: %v", err)
	}

	var buf bytes.Buffer
	if code := runVersion(&buf); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}

	out := buf.String()
	if !strings.Contains(out, "v0.1.1") {
		t.Errorf("missing installed rune-mcp version: %q", out)
	}
	if !strings.Contains(out, "v0.1.3") {
		t.Errorf("missing installed runed version: %q", out)
	}
}

func TestRunVersion_NoInstalledManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	var buf bytes.Buffer
	if code := runVersion(&buf); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}

	out := buf.String()
	if !strings.Contains(out, "rune ") {
		t.Errorf("CLI version line must be printed: %q", out)
	}
	if strings.Contains(out, "rune-mcp:") || strings.Contains(out, "runed:") {
		t.Errorf("should not print installed versions when installed.json is absent: %q", out)
	}
}

func TestRenderVerifyText_HappyPath(t *testing.T) {
	r := &bootstrap.InstallChecks{
		OK: true,
		Checks: []bootstrap.InstallCheck{
			{Name: "rune_config", Status: bootstrap.StatusOK, Detail: "/path"},
			{Name: "model_file", Status: bootstrap.StatusWarn, Detail: "absent", FixHint: "runed will fetch"},
		},
	}
	var buf bytes.Buffer
	renderVerifyText(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "[ok]") || !strings.Contains(out, "[warn]") {
		t.Errorf("symbols missing: %q", out)
	}
	if !strings.Contains(out, "status: OK") {
		t.Errorf("final status missing: %q", out)
	}
}

func TestRenderVerifyText_FailFlagsStatus(t *testing.T) {
	r := &bootstrap.InstallChecks{
		OK: false,
		Checks: []bootstrap.InstallCheck{
			{Name: "vault_creds", Status: bootstrap.StatusFail, Detail: "missing token", FixHint: "/rune:configure"},
		},
	}
	var buf bytes.Buffer
	renderVerifyText(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "[fail]") {
		t.Errorf("[fail] symbol missing: %q", out)
	}
	if !strings.Contains(out, "status: FAIL") {
		t.Errorf("FAIL banner missing: %q", out)
	}
	if !strings.Contains(out, "/rune:configure") {
		t.Errorf("fix hint missing: %q", out)
	}
}

func TestRunVerify_ExitCodeOnFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	// No config file
	var buf, errBuf bytes.Buffer
	code := runVerify(context.Background(), nil, &buf, &errBuf)
	if code != 1 {
		t.Errorf("exit = %d, want 1 (fail)", code)
	}
	if !strings.Contains(buf.String(), "status: FAIL") {
		t.Errorf("expected FAIL banner: %q", buf.String())
	}
}

func TestRunVerify_JSONValidity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	var buf, errBuf bytes.Buffer
	_ = runVerify(context.Background(), []string{"--json"}, &buf, &errBuf)
	var got bootstrap.InstallChecks
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got.Checks) == 0 {
		t.Errorf("expected checks in JSON output: %+v", got)
	}
}

func TestRunInstall_ErrorsWithoutManifest(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()
	t.Setenv("RUNE_MANIFEST", "")

	var stdout, stderr bytes.Buffer
	code := runInstall(context.Background(), nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2 (usage error)", code)
	}
	if !strings.Contains(stderr.String(), "no manifest URL configured") {
		t.Errorf("expected manifest error: %q", stderr.String())
	}
}

func TestRunInstall_UnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runInstall(context.Background(), []string{"--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2 (flag parse error)", code)
	}
}

func TestRunInstall_CheckManifestEnv(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))
	t.Setenv("RUNE_MANIFEST", "https://example.invalid/manifest.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer
	code := runInstall(ctx, nil, &stdout, &stderr)
	if code == 2 {
		t.Errorf("exit = 2 (guard tripped); RUNE_MANIFEST exist but ignored. stderr=%q", stderr.String())
	}
	if strings.Contains(stderr.String(), "no manifest URL configured") {
		t.Errorf("guard message present despite RUNE_MANIFEST set: %q", stderr.String())
	}
}

func TestRunInstall_JSONMissingManifest(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()
	t.Setenv("RUNE_MANIFEST", "")

	var stdout, stderr bytes.Buffer
	code := runInstall(context.Background(), []string{"--json"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}

	// Emit stdout as JSON
	var ev jsonEvent
	if err := json.Unmarshal(stdout.Bytes(), &ev); err != nil {
		t.Fatalf("stdout should be a JSON event; got %q (err %v)", stdout.String(), err)
	}

	if ev.Event != "summary" {
		t.Errorf("event = %q, want \"summary\"", ev.Event)
	}
	if ev.Error == "" {
		t.Errorf("missing-manifest summary must carry an error; got %+v", ev)
	}
}

func TestRunVersion_CheckManifestEnv(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()

	t.Setenv("RUNE_MANIFEST", "https://example.invalid/m.json")
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	var buf bytes.Buffer
	if code := runVersion(&buf); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}

	out := buf.String()
	if !strings.Contains(out, "https://example.invalid/m.json") {
		t.Errorf("RUNE_MANIFEST exist but ignored: %q", out)
	}
	if strings.Contains(out, "manifest missing") {
		t.Errorf("should not report missing when RUNE_MANIFEST is set: %q", out)
	}
}

func TestRunVerify_BadFlagToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runVerify(context.Background(), []string{"--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit = %d, want 2 (flag parse error)", code)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout must clean for --json consumers; got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "not defined") {
		t.Errorf("flag error should stderr; got %q", stderr.String())
	}
}
