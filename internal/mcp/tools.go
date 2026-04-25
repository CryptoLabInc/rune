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
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/service"
)

// Deps — injected into all 8 MCP handlers.
//
// Phase A: empty struct. Adapter clients · state machine · config will be
// added as Phase 4 (adapters) and Phase 5 (service orchestration) land.
// stubHandler already takes deps as an argument, so Phase 5 will only need
// to swap the closure body, not the signature.
//
// Future fields (commented as a contract sketch — to be activated as the
// owning adapter PR lands):
//
//	Vault      vault.Client
//	Envector   envector.Client
//	Embedder   embedder.Client
//	CaptureLog *logio.CaptureLog
//	State      *lifecycle.Manager
//	Cfg        *config.Config
type Deps struct{}

// emptyArgs — input type for tools that take no arguments.
type emptyArgs struct{}

// Register binds all 8 MCP tools onto the provided SDK server.
//
// Tool names are bit-identical to Python `mcp/server/server.py`. Order in
// this Register call is for readability only; the SDK sorts tools
// alphabetically in `tools/list` output.
//
// AddTool can panic on schema-inference failure (SDK behavior). Register
// recovers so a misconfigured tool surfaces as a startup error instead of
// taking the process down silently after binding.
func Register(srv *sdkmcp.Server, deps *Deps) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("mcp.Register: AddTool panic: %v", r)
		}
	}()

	// Write tools (state gate applies in Phase 5).
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_capture",
		Description: "Capture a decision record (agent-delegated extraction required).",
	}, stubHandler[domain.CaptureRequest, domain.CaptureResponse](deps, "rune_capture"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_batch_capture",
		Description: "Capture a batch of decision records (e.g. session-end sweep).",
	}, stubHandler[service.BatchCaptureArgs, service.BatchCaptureResult](deps, "rune_batch_capture"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_recall",
		Description: "Query organizational memory by natural-language question.",
	}, stubHandler[domain.RecallArgs, domain.RecallResult](deps, "rune_recall"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_delete_capture",
		Description: "Soft-delete a record by ID (sets status=reverted, re-inserts).",
	}, stubHandler[service.DeleteCaptureArgs, service.DeleteCaptureResult](deps, "rune_delete_capture"))

	// Read / diagnostic tools (state gate bypass).
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_capture_history",
		Description: "List recent captures from local capture_log.jsonl (read-only).",
	}, stubHandler[service.CaptureHistoryArgs, service.CaptureHistoryResult](deps, "rune_capture_history"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_vault_status",
		Description: "Probe Vault connectivity and report secure-search mode.",
	}, stubHandler[emptyArgs, service.VaultStatusResult](deps, "rune_vault_status"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_diagnostics",
		Description: "Collect a 7-section health snapshot (env / state / vault / keys / pipelines / embedding / envector).",
	}, stubHandler[emptyArgs, service.DiagnosticsResult](deps, "rune_diagnostics"))

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "rune_reload_pipelines",
		Description: "Re-initialize Vault + envector pipelines (BOOT replay) with envector warmup.",
	}, stubHandler[emptyArgs, service.ReloadPipelinesResult](deps, "rune_reload_pipelines"))

	return nil
}

// stubHandler returns a SDK ToolHandlerFor that always responds with a
// not-yet-implemented isError result. Output type is preserved so tools/list
// can still publish the inferred output schema.
//
// deps is captured but unused in Phase A. Phase 5 will dereference it for
// CheckState / service dispatch — the closure shape stays the same.
func stubHandler[In, Out any](deps *Deps, toolName string) sdkmcp.ToolHandlerFor[In, Out] {
	_ = deps // captured for Phase 5; intentionally unused now
	return func(_ context.Context, _ *sdkmcp.CallToolRequest, _ In) (*sdkmcp.CallToolResult, Out, error) {
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
