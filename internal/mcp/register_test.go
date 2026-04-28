// Phase A.5 smoke tests — in-memory MCP server/client to assert that the
// 8-tool catalog and stub responses survive future refactors.
//
// These mirror the bash/jq cookbook in docs/v04/progress/phase-a-mcp-boot.md
// §4.2 (tools/list) and §4.3 (tools/call). Replacing the cookbook with Go
// tests turns the verification into a CI gate.

package mcp_test

import (
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/mcp"
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
func newSession(t *testing.T) *sdkmcp.ClientSession {
	t.Helper()
	ctx := t.Context()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "rune-mcp-test",
		Version: "0.0.0-test",
	}, nil)
	if err := mcp.Register(srv, &mcp.Deps{}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	st, ct := sdkmcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

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

func TestRegister_InputSchemaInferred(t *testing.T) {
	cs := newSession(t)

	res, err := cs.ListTools(t.Context(), &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range res.Tools {
		if tool.InputSchema == nil {
			t.Errorf("%s: InputSchema is nil — schema inference regressed", tool.Name)
		}
	}
}

func TestRegister_StubReturnsIsError(t *testing.T) {
	cs := newSession(t)

	cases := []struct {
		name string
		args map[string]any
	}{
		{"rune_diagnostics", nil},
		{"rune_vault_status", nil},
		{"rune_recall", map[string]any{"query": "hello"}},
		{"rune_capture", map[string]any{"text": "hi", "source": "test", "extracted": map[string]any{}}},
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
				t.Errorf("IsError: got false, want true (Phase A stub)")
			}
			if len(res.Content) == 0 {
				t.Fatalf("Content: empty")
			}
			tc0, ok := res.Content[0].(*sdkmcp.TextContent)
			if !ok {
				t.Fatalf("Content[0]: got %T, want *TextContent", res.Content[0])
			}
			if !strings.Contains(tc0.Text, "not yet implemented") {
				t.Errorf("Content[0].Text: %q does not contain stub marker", tc0.Text)
			}
			if !strings.Contains(tc0.Text, tc.name) {
				t.Errorf("Content[0].Text: %q does not name the tool", tc0.Text)
			}
		})
	}
}
