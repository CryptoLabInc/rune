package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/envector/rune-go/internal/bootstrap"
)

func TestRunVersion_PrintConstants(t *testing.T) {
	saved := manifestURL
	manifestURL = "https://example/manifest.json"
	defer func() { manifestURL = saved }()

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

	var buf bytes.Buffer
	_ = runVersion(&buf)
	// Match the actual "manifest missing" copy in version.go's empty
	// branch — keep the assertion loose so future copy edits don't
	// require keeping two strings in lockstep.
	if !strings.Contains(buf.String(), "manifest missing") {
		t.Errorf("empty manifest should be flagged; got %q", buf.String())
	}
}

func TestRenderDiagnosisText_HappyPath(t *testing.T) {
	r := &bootstrap.DiagnosisResult{
		OK: true,
		Checks: []bootstrap.DiagnosisCheck{
			{Name: "rune_config", Status: bootstrap.StatusOK, Detail: "/path"},
			{Name: "model_file", Status: bootstrap.StatusWarn, Detail: "absent", FixHint: "runed will fetch"},
		},
	}
	var buf bytes.Buffer
	renderDiagnosisText(&buf, r)
	out := buf.String()
	if !strings.Contains(out, "[ok]") || !strings.Contains(out, "[warn]") {
		t.Errorf("symbols missing: %q", out)
	}
	if !strings.Contains(out, "status: OK") {
		t.Errorf("final status missing: %q", out)
	}
}

func TestRenderDiagnosisText_FailFlagsStatus(t *testing.T) {
	r := &bootstrap.DiagnosisResult{
		OK: false,
		Checks: []bootstrap.DiagnosisCheck{
			{Name: "vault_creds", Status: bootstrap.StatusFail, Detail: "missing token", FixHint: "/rune:configure"},
		},
	}
	var buf bytes.Buffer
	renderDiagnosisText(&buf, r)
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

func TestRunDiagnosis_ExitCodeOnFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	// No config file
	var buf bytes.Buffer
	code := runDiagnosis(context.Background(), nil, &buf)
	if code != 1 {
		t.Errorf("exit = %d, want 1 (fail)", code)
	}
	if !strings.Contains(buf.String(), "status: FAIL") {
		t.Errorf("expected FAIL banner: %q", buf.String())
	}
}

func TestRunDiagnosis_JSONValidity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))

	var buf bytes.Buffer
	_ = runDiagnosis(context.Background(), []string{"--json"}, &buf)
	var got bootstrap.DiagnosisResult
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
