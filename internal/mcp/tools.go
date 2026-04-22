// Package mcp holds the 8 MCP tool handlers + stdio server wiring.
// Spec:
//   docs/v04/spec/flows/capture.md (7-phase capture)
//   docs/v04/spec/flows/recall.md (7-phase recall)
//   docs/v04/spec/flows/lifecycle.md (6 other tools)
// Python: mcp/server/server.py (2002 LoC).
//
// MCP SDK: github.com/modelcontextprotocol/go-sdk v1.5.0+ (D2).
// Stdio transport, tools/call dispatch. Input schema auto-generated from Go
// structs with jsonschema tags.
package mcp

import (
	"context"

	"github.com/envector/rune-go/internal/domain"
)

// Deps — injected into all handlers. TODO: fill as adapters stabilize.
type Deps struct {
	// Vault     vault.Client
	// Envector  envector.Client
	// Embedder  embedder.Client
	// CaptureLog *logio.CaptureLog
	// State     *lifecycle.Manager
	// Cfg       *config.Config
}

// ─────────────────────────────────────────────────────────────────────────────
// 8 MCP tools — Python bit-identical names/shapes
// ─────────────────────────────────────────────────────────────────────────────

// ToolCapture — rune_capture. Python: server.py:L698-806 + L1208-1407 _capture_single.
// 7-phase flow (spec/flows/capture.md).
func ToolCapture(ctx context.Context, deps *Deps, req *domain.CaptureRequest) (*domain.CaptureResponse, error) {
	// TODO Phase 1: state gate
	// TODO Phase 2: validate + tier2 check + text_to_embed pick
	// TODO Phase 3: embedder.EmbedSingle(text)
	// TODO Phase 4: envector.Score + Vault.DecryptScores → novelty (D11 thresholds)
	// TODO Phase 5: record_builder.BuildPhases + embedder.EmbedBatch + AES Seal
	// TODO Phase 6: envector.Insert (batch)
	// TODO Phase 7: capture_log append + respond
	return nil, nil
}

// ToolRecall — rune_recall. Python: server.py:L910-1034. 7-phase.
func ToolRecall(ctx context.Context, deps *Deps, args *domain.RecallArgs) (*domain.RecallResult, error) {
	// TODO Phase 1: state gate + topk validation (max 10)
	// TODO Phase 2: policy.Parse (English only — D21)
	// TODO Phase 3: embedder.EmbedBatch(expansions[:3]) (D22/D23)
	// TODO Phase 4: sequential 4-RPC per expansion (D25): Score → DecryptScores
	//                → GetMetadata → DecryptMetadata (service layer calls Vault directly)
	// TODO Phase 5: metadata 3-way classify (AES/plain/legacy base64 per D26)
	// TODO Phase 6: phase_chain expansion (D27) + group assemble + filter
	//                + recency weighting (half-life 90d, status mul)
	// TODO Phase 7: build response (synthesized=false per D28)
	return nil, nil
}

// ToolBatchCapture — rune_batch_capture. Python: server.py:L810-896.
// Per-item independent processing (skipped/captured/near_duplicate/error).
func ToolBatchCapture(ctx context.Context, deps *Deps, items []domain.CaptureRequest) (any, error) {
	// TODO: per-item _capture_single call + summary (captured/skipped/errors)
	return nil, nil
}

// ToolCaptureHistory — rune_capture_history. Python: server.py:L1092-1111.
// Read ~/.rune/capture_log.jsonl reverse, filter by domain/since, limit (default 20, max 100).
func ToolCaptureHistory(ctx context.Context, deps *Deps, limit int, domainFilter, since *string) (any, error) {
	// TODO: logio.Tail
	return nil, nil
}

// ToolDeleteCapture — rune_delete_capture. Python: server.py:L1113-1206.
// Soft-delete: search_by_id → set status=reverted → re-embed → re-insert → log.
func ToolDeleteCapture(ctx context.Context, deps *Deps, recordID string) (any, error) {
	// TODO: soft-delete flow
	return nil, nil
}

// ToolVaultStatus — rune_vault_status. Python: server.py:L496-528. Read-only.
func ToolVaultStatus(ctx context.Context, deps *Deps) (any, error) {
	// TODO: vault.HealthCheck + mode/endpoint response
	return nil, nil
}

// ToolDiagnostics — rune_diagnostics. Python: server.py:L540-684.
// 7 sections: environment / state / vault / keys / pipelines / embedding / envector.
func ToolDiagnostics(ctx context.Context, deps *Deps) (any, error) {
	// TODO: collect 7 sections + 5s envector timeout
	return nil, nil
}

// ToolReloadPipelines — rune_reload_pipelines. Python: server.py:L1046-1089.
// Re-init scribe/retriever pipelines + envector GetIndexList warmup (60s timeout).
func ToolReloadPipelines(ctx context.Context, deps *Deps) (any, error) {
	// TODO: AwaitInitDone + ReinitPipelines + 60s warmup
	return nil, nil
}

// Register — called from main() to bind all 8 tools to the MCP SDK server.
// TODO: uses github.com/modelcontextprotocol/go-sdk/mcp.AddTool for each.
func Register(/* srv *mcp.Server, */ deps *Deps) {
	// TODO:
	//   mcp.AddTool(srv, &mcp.Tool{Name: "rune_capture", ...}, ToolCapture adapter)
	//   ... 8 tools
}
