// Phase A.5 smoke tests — in-memory MCP server/client to assert that the
// 8-tool catalog and state-gated handlers survive future refactors.
//
// These mirror the bash/jq cookbook in docs/v04/progress/phase-a-mcp-boot.md
// §4.2 (tools/list) and §4.3 (tools/call). Replacing the cookbook with Go
// tests turns the verification into a CI gate.

package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/lifecycle"
	"github.com/envector/rune-go/internal/mcp"
	"github.com/envector/rune-go/internal/service"
)

// expectedTools — alphabetical order matches what the SDK advertises in
// tools/list (Python rune v0.3.x bit-identical names).
var expectedTools = []string{
	"rune_batch_capture",
	"rune_capture",
	"rune_capture_history",
	"rune_delete_capture",
	"rune_diagnostics",
	"rune_recall",
	"rune_reload_pipelines",
	"rune_vault_status",
}

// newSession spins up an in-memory MCP server with all 8 tools registered
// and returns a connected client session ready for tools/list and tools/call.
//
// Deps mirrors a "boot has not progressed past starting" state: the Manager
// is freshly constructed (StateStarting) and services are zero-valued. With
// State == StateStarting, write tools return PIPELINE_NOT_READY through the
// CheckState gate. Read-only tools (vault_status / diagnostics /
// capture_history) bypass the gate but their service nil-checks must hold.
func newSession(t *testing.T) *sdkmcp.ClientSession {
	t.Helper()
	ctx := t.Context()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "rune-mcp-test",
		Version: "0.0.0-test",
	}, nil)
	mgr := lifecycle.NewManager()
	cap := service.NewCaptureService()
	cap.State = mgr
	life := service.NewLifecycleService()
	life.State = mgr
	deps := &mcp.Deps{
		State:     mgr,
		Capture:   cap,
		Recall:    service.NewRecallService(),
		Lifecycle: life,
	}
	if err := mcp.Register(srv, deps); err != nil {
		t.Fatalf("Register: %v", err)
	}

	st, ct := sdkmcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{
		Name:    "rune-mcp-test-client",
		Version: "0.0.0-test",
	}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestRegister_All8ToolsListed(t *testing.T) {
	cs := newSession(t)

	res, err := cs.ListTools(t.Context(), &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := make([]string, len(res.Tools))
	for i, tool := range res.Tools {
		got[i] = tool.Name
	}

	if len(got) != len(expectedTools) {
		t.Fatalf("tool count: got %d, want %d (got=%v)", len(got), len(expectedTools), got)
	}
	// SDK contract: tools/list returns alphabetical order (go-sdk sorts on
	// emit). Compare position-by-position so a regression in the SDK or
	// in registration ordering surfaces here.
	for i, name := range expectedTools {
		if got[i] != name {
			t.Errorf("tool[%d]: got %q, want %q (full list: %v)", i, got[i], name, got)
		}
	}
}

func TestRegister_SchemasInferred(t *testing.T) {
	cs := newSession(t)

	res, err := cs.ListTools(t.Context(), &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// tools.go promises both input AND output schema are preserved
	// (see Register / stubHandler comments). A nil on either side means
	// schema inference broke for that tool.
	for _, tool := range res.Tools {
		if tool.InputSchema == nil {
			t.Errorf("%s: InputSchema is nil — input schema inference regressed", tool.Name)
		}
		if tool.OutputSchema == nil {
			t.Errorf("%s: OutputSchema is nil — output schema inference regressed", tool.Name)
		}
	}
}

// TestRegister_WriteToolsGated — write tools (capture, batch_capture, recall,
// delete_capture) must surface PIPELINE_NOT_READY when Deps.State is in
// StateStarting. Confirms the CheckState gate fires before service dispatch.
//
// reload_pipelines is intentionally NOT gated (it is the dormant→active
// unblocker / `/rune:activate` handler per rune-mcp.md). Smoke tests for it
// live in the diagnostic suite once an envector mock is in place.
func TestRegister_WriteToolsGated(t *testing.T) {
	cs := newSession(t)

	cases := []struct {
		name string
		args map[string]any
	}{
		{"rune_batch_capture", map[string]any{"items": "[]"}},
		{"rune_capture", map[string]any{"text": "hi", "source": "test", "extracted": map[string]any{}}},
		{"rune_delete_capture", map[string]any{"record_id": "test-id"}},
		{"rune_recall", map[string]any{"query": "hello"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &sdkmcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool transport error: %v", err)
			}
			if !res.IsError {
				t.Errorf("IsError: got false, want true (state gate should reject)")
			}
			if len(res.Content) == 0 {
				t.Fatalf("Content: empty")
			}
			tc0, ok := res.Content[0].(*sdkmcp.TextContent)
			if !ok {
				t.Fatalf("Content[0]: got %T, want *TextContent", res.Content[0])
			}
			if !strings.Contains(tc0.Text, "PIPELINE_NOT_READY") {
				t.Errorf("Content[0].Text: %q does not contain PIPELINE_NOT_READY marker", tc0.Text)
			}
		})
	}
}

// TestRegister_ReadOnlyToolsBypassGate — vault_status / diagnostics /
// capture_history must respond successfully (no PIPELINE_NOT_READY) even
// when State == StateStarting. Per rune-mcp.md these tools work
// degraded so the operator can troubleshoot pre-active.
func TestRegister_ReadOnlyToolsBypassGate(t *testing.T) {
	cs := newSession(t)

	cases := []struct {
		name           string
		args           map[string]any
		mustContain    []string // substrings that should appear in TextContent
		mustNotContain []string
	}{
		{
			// nil Vault → "standard mode"
			name:        "rune_vault_status",
			args:        nil,
			mustContain: []string{`"vault_configured":false`, "standard"},
			mustNotContain: []string{
				"PIPELINE_NOT_READY",
			},
		},
		{
			// Diagnostics returns the 7-section snapshot; environment section
			// always populated. We avoid asserting on `state` because it
			// reflects config.json contents (not runtime Manager) and the
			// test environment may have a real config.json present — see
			// `LifecycleService.Diagnostics` for the read path.
			name:        "rune_diagnostics",
			args:        nil,
			mustContain: []string{`"environment"`, `"vault"`, `"keys"`, `"embedding"`},
			mustNotContain: []string{
				"PIPELINE_NOT_READY",
			},
		},
		{
			// CaptureHistory reads ~/.rune/capture_log.jsonl (likely missing in test env);
			// the handler should still respond without error (entries: empty, ok: true).
			name:        "rune_capture_history",
			args:        map[string]any{"limit": 5.0},
			mustContain: []string{`"ok":true`},
			mustNotContain: []string{
				"PIPELINE_NOT_READY",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &sdkmcp.CallToolParams{
				Name:      tc.name,
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool transport error: %v", err)
			}
			if res.IsError {
				body := ""
				if len(res.Content) > 0 {
					if tc0, ok := res.Content[0].(*sdkmcp.TextContent); ok {
						body = tc0.Text
					}
				}
				t.Fatalf("IsError: got true, want false (read-only tools bypass gate). body=%s", body)
			}
			if len(res.Content) == 0 {
				t.Fatalf("Content: empty")
			}
			tc0, ok := res.Content[0].(*sdkmcp.TextContent)
			if !ok {
				t.Fatalf("Content[0]: got %T, want *TextContent", res.Content[0])
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(tc0.Text, want) {
					t.Errorf("Content[0].Text missing %q in: %s", want, tc0.Text)
				}
			}
			for _, deny := range tc.mustNotContain {
				if strings.Contains(tc0.Text, deny) {
					t.Errorf("Content[0].Text contains %q (should not): %s", deny, tc0.Text)
				}
			}
		})
	}
}

// TestRegister_ErrorResultPreservesRuneError — verifies the {ok,error{code,
// message,retryable,recovery_hint}} shape is carried bit-identical in the
// TextContent JSON when handlers fail. Uses the state-gate path which is
// guaranteed to surface PIPELINE_NOT_READY in StateStarting.
func TestRegister_ErrorResultPreservesRuneError(t *testing.T) {
	cs := newSession(t)

	res, err := cs.CallTool(t.Context(), &sdkmcp.CallToolParams{
		Name:      "rune_capture",
		Arguments: map[string]any{"text": "hi", "source": "test", "extracted": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("CallTool transport: %v", err)
	}
	if !res.IsError {
		t.Fatal("IsError: want true for state-gated tool")
	}
	if len(res.Content) == 0 {
		t.Fatal("Content: empty")
	}
	tc0, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("Content[0]: %T not *TextContent", res.Content[0])
	}

	var body struct {
		OK    bool `json:"ok"`
		Error struct {
			Code         string `json:"code"`
			Message      string `json:"message"`
			Retryable    bool   `json:"retryable"`
			RecoveryHint string `json:"recovery_hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(tc0.Text), &body); err != nil {
		t.Fatalf("TextContent not parseable as MakeError JSON: %v\nbody=%s", err, tc0.Text)
	}

	if body.OK {
		t.Errorf("ok: got true, want false")
	}
	if body.Error.Code != "PIPELINE_NOT_READY" {
		t.Errorf("error.code: got %q, want PIPELINE_NOT_READY", body.Error.Code)
	}
	if body.Error.Retryable {
		t.Errorf("error.retryable: got true, want false (PIPELINE_NOT_READY is not retryable)")
	}
	if body.Error.RecoveryHint == "" {
		t.Error("error.recovery_hint: empty (CheckState should populate state-specific hint)")
	}
	if !strings.Contains(body.Error.RecoveryHint, "starting") {
		t.Errorf("error.recovery_hint: %q does not mention starting state", body.Error.RecoveryHint)
	}
}
