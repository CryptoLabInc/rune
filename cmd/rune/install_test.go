package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CryptoLabInc/rune-cli/internal/bootstrap"
)

func TestRunInstall_JSONHappyPath(t *testing.T) {
	saved := manifestURL
	manifestURL = ""
	defer func() { manifestURL = saved }()

	dir := t.TempDir()
	t.Setenv("RUNE_HOME", filepath.Join(dir, "rune"))
	t.Setenv("RUNED_HOME", filepath.Join(dir, "runed"))
	t.Setenv("RUNE_MANIFEST", "")

	runed := []byte("runed-bytes")
	mcp := []byte("rune-mcp-bytes")
	sha := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := map[string]any{
			"version":          1,
			"rune_mcp_version": "v0.1.0-test",
			"runed_version":    "v0.1.0-test",
			"platforms": map[string]any{
				bootstrap.PlatformTuple(): map[string]any{
					"runed":    map[string]any{"url": srv.URL + "/runed", "sha256": sha(runed), "size": len(runed)},
					"rune_mcp": map[string]any{"url": srv.URL + "/rune-mcp", "sha256": sha(mcp), "size": len(mcp)},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	})

	serve := func(b []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
			_, _ = w.Write(b)
		}
	}

	mux.HandleFunc("/runed", serve(runed))
	mux.HandleFunc("/rune-mcp", serve(mcp))
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	code := runInstall(context.Background(), []string{"--json", "--manifest-url", srv.URL + "/manifest.json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	dec := json.NewDecoder(&stdout)
	var sawLog, sawSummary bool
	for dec.More() {
		var ev jsonEvent
		if err := dec.Decode(&ev); err != nil {
			t.Fatalf("stdout is not a clean JSON: %v", err)
		}

		switch ev.Event {
		case "log":
			sawLog = true
		case "summary":
			sawSummary = true
			if ev.Error != "" {
				t.Errorf("success summary should carry no error; got %q", ev.Error)
			}
			if ev.Result == nil || !ev.Result.OK {
				t.Errorf("summary Result should be OK; got %+v", ev.Result)
			}
		}
	}

	if !sawLog {
		t.Error("expected at least one log event in --json output")
	}
	if !sawSummary {
		t.Error("expected a terminal summary event in --json output")
	}
}
