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
	"github.com/envector/rune-go/internal/lifecycle"
	"github.com/envector/rune-go/internal/service"
)

// Deps — injected into all 8 MCP handlers.
//
// State + 3 services drive request handling. cmd/rune-mcp/main.go constructs
// Deps after the boot loop has populated adapter clients on the services.
// Until boot completes, write tools fail with PIPELINE_NOT_READY through
// CheckState; read-only tools (vault_status, diagnostics, capture_history)
// can run pre-active for diagnostics.
type Deps struct {
	State     *lifecycle.Manager
	Capture   *service.CaptureService
	Recall    *service.RecallService
	Lifecycle *service.LifecycleService
}

// emptyArgs — input type for tools that take no arguments.
type emptyArgs struct{}

// Register binds all 8 MCP tools onto the provided SDK server.
//
// Tool names are bit-identical to Python `mcp/server/server.py`. SDK sorts
// tools alphabetically in `tools/list` output, so order here is for readability.
//
// Failure modes that Register surfaces as a startup error (via panic +
// recover):
//  1. mustAddTool name validation (SDK's validateToolName has a log-only
//     branch — server.go:238-241 — that we bypass by panicking up-front).
//  2. SDK schema-inference panic (toolForErr).
//  3. SDK schema-shape panic (Server.AddTool).
//
// Result: every registration either succeeds completely or returns an error.
// No silent half-registrations.
func Register(srv *sdkmcp.Server, deps *Deps) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("mcp.Register: %v", r)
		}
	}()

	// Write tools (state gate applies in Phase 5).
	mustAddTool[domain.CaptureRequest, domain.CaptureResponse](srv, deps,
		"rune_capture",
		"Capture a decision record (agent-delegated extraction required).")
	mustAddTool[service.BatchCaptureArgs, service.BatchCaptureResult](srv, deps,
		"rune_batch_capture",
		"Capture a batch of decision records (e.g. session-end sweep).")
	mustAddTool[domain.RecallArgs, domain.RecallResult](srv, deps,
		"rune_recall",
		"Query organizational memory by natural-language question.")
	mustAddTool[service.DeleteCaptureArgs, service.DeleteCaptureResult](srv, deps,
		"rune_delete_capture",
		"Soft-delete a record by ID (sets status=reverted, re-inserts).")

	// Read / diagnostic tools (state gate bypass).
	mustAddTool[service.CaptureHistoryArgs, service.CaptureHistoryResult](srv, deps,
		"rune_capture_history",
		"List recent captures from local capture_log.jsonl (read-only).")
	mustAddTool[emptyArgs, service.VaultStatusResult](srv, deps,
		"rune_vault_status",
		"Probe Vault connectivity and report secure-search mode.")
	mustAddTool[emptyArgs, service.DiagnosticsResult](srv, deps,
		"rune_diagnostics",
		"Collect a 7-section health snapshot (env / state / vault / keys / pipelines / embedding / envector).")
	mustAddTool[emptyArgs, service.ReloadPipelinesResult](srv, deps,
		"rune_reload_pipelines",
		"Re-initialize Vault + envector pipelines (BOOT replay) with envector warmup.")

	return nil
}

// mustAddTool wraps sdkmcp.AddTool with up-front name validation.
//
// The SDK's Server.AddTool only LOGS on invalid tool names
// (go-sdk/mcp/server.go:238-241) — it does not panic, so Register's
// defer recover() would miss it and the bad-named tool would silently
// register. mustAddTool panics on invalid names, unifying the failure
// path so recover() catches everything.
func mustAddTool[In, Out any](srv *sdkmcp.Server, deps *Deps, name, description string) {
	if !isValidToolName(name) {
		panic(fmt.Errorf("mustAddTool: invalid tool name %q (allowed: [A-Za-z0-9_-], 1..128 chars)", name))
	}
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        name,
		Description: description,
	}, stubHandler[In, Out](deps, name))
}

// isValidToolName mirrors the SDK's validateToolName rules
// (go-sdk/mcp/tool.go:109): non-empty, ≤128 chars, only [A-Za-z0-9_-].
// Update this when bumping the SDK if its validation tightens.
func isValidToolName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
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
