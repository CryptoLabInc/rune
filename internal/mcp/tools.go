// Package mcp wires the 8 MCP tool handlers onto the official Go SDK and
// owns Deps injection + state-aware response shaping.
//
// Spec:
//   docs/v04/spec/components/rune-mcp.md (MCP server 구현)
//   docs/v04/spec/flows/{capture,recall,lifecycle}.md
//
// SDK: github.com/modelcontextprotocol/go-sdk v1.5.0+ (D2). Stdio transport.
// Input schema is auto-inferred from the Go input struct (jsonschema tags
// optional but recommended; will be tightened in Phase 5).
//
// Phase A (current): handshake + tools/list only. Every handler returns a
// stubResult ("not yet implemented") so Claude Code can discover the catalog
// without any adapter being wired. Phase 5 replaces each stub with a
// service-layer call (CheckState → service.X.Handle → response wrap).
package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/service"
)

// Deps — injected into all handlers.
//
// Phase A: empty struct. Adapter clients · state machine · config will be
// added as Phase 4 (adapters) and Phase 5 (service orchestration) land.
// Concrete fields stay commented until each adapter has a real Client type;
// the handlers below close over deps already so no signature change later.
type Deps struct {
	// Vault      vault.Client
	// Envector   envector.Client
	// Embedder   embedder.Client
	// CaptureLog *logio.CaptureLog
	// State      *lifecycle.Manager
	// Cfg        *config.Config
}

// emptyArgs — input type for tools that take no arguments.
type emptyArgs struct{}

// Register binds all 8 MCP tools onto the provided SDK server.
//
// Tool naming + ordering are bit-identical to Python `mcp/server/server.py`.
// Descriptions are intentionally short — Claude reads them in tool selection,
// not the user, so they should be a single concrete capability sentence.
func Register(srv *sdkmcp.Server, deps *Deps) {
	// Write tools (state gate applies in Phase 5).
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_capture",
		Description: "Capture a decision record (agent-delegated extraction required).",
	}, stubHandler[domain.CaptureRequest, domain.CaptureResponse]("rune_capture"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_batch_capture",
		Description: "Capture a batch of decision records (e.g. session-end sweep).",
	}, stubHandler[service.BatchCaptureArgs, service.BatchCaptureResult]("rune_batch_capture"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_recall",
		Description: "Query organizational memory by natural-language question.",
	}, stubHandler[domain.RecallArgs, domain.RecallResult]("rune_recall"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_delete_capture",
		Description: "Soft-delete a record by ID (sets status=reverted, re-inserts).",
	}, stubHandler[service.DeleteCaptureArgs, service.DeleteCaptureResult]("rune_delete_capture"))

	// Read / diagnostic tools (state gate bypass).
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_capture_history",
		Description: "List recent captures from local capture_log.jsonl (read-only).",
	}, stubHandler[service.CaptureHistoryArgs, service.CaptureHistoryResult]("rune_capture_history"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_vault_status",
		Description: "Probe Vault connectivity and report secure-search mode.",
	}, stubHandler[emptyArgs, service.VaultStatusResult]("rune_vault_status"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_diagnostics",
		Description: "Collect a 7-section health snapshot (env / state / vault / keys / pipelines / embedding / envector).",
	}, stubHandler[emptyArgs, service.DiagnosticsResult]("rune_diagnostics"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_reload_pipelines",
		Description: "Re-initialize Vault + envector pipelines (BOOT replay) with envector warmup.",
	}, stubHandler[emptyArgs, service.ReloadPipelinesResult]("rune_reload_pipelines"))

	_ = deps // Phase A unused; closures will capture this in Phase 5.
}

// stubHandler returns a SDK ToolHandlerFor that always responds with a
// not-yet-implemented isError result. Output type is preserved so tools/list
// can still publish the inferred output schema.
func stubHandler[In, Out any](toolName string) sdkmcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, in In) (*sdkmcp.CallToolResult, Out, error) {
		_ = ctx
		_ = req
		_ = in
		var zero Out
		return stubResult(toolName), zero, nil
	}
}

// stubResult composes the Phase-A "not implemented" response.
func stubResult(toolName string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		IsError: true,
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{
				Text: toolName + " is not yet implemented (skeleton phase A — MCP handshake + tools/list only).",
			},
		},
	}
}
