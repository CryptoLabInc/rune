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
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/vault"
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
	Vault    vault.Client
	Envector envector.Client
	Embedder embedder.Client
	State    *lifecycle.Manager

	Capture   *service.CaptureService
	Recall    *service.RecallService
	Lifecycle *service.LifecycleService
}

func (d *Deps) InjectVault(client vault.Client) {
	d.Vault = client
	if d.Capture != nil {
		d.Capture.Vault = client
	}
	if d.Recall != nil {
		d.Recall.Vault = client
	}
	if d.Lifecycle != nil {
		d.Lifecycle.Vault = client
	}
}

func (d *Deps) InjectEmbedder(client embedder.Client) {
	d.Embedder = client
	if d.Capture != nil {
		d.Capture.Embedder = client
	}
	if d.Recall != nil {
		d.Recall.Embedder = client
	}
	if d.Lifecycle != nil {
		d.Lifecycle.Embedder = client
	}
}

func (d *Deps) InjectEnvector(client envector.Client) {
	d.Envector = client
	if d.Capture != nil {
		d.Capture.Envector = client
	}
	if d.Recall != nil {
		d.Recall.Envector = client
	}
	if d.Lifecycle != nil {
		d.Lifecycle.Envector = client
	}
}

// ApplyVaultBundle propagates per-bundle metadata (AgentID / AgentDEK /
// IndexName / KeyID) to the three services that need them. Called by the
// boot loop after Vault.GetAgentManifest succeeds.
//
// Without this call, capture's AES envelope sealing fails (empty AgentDEK)
// and recall / lifecycle diagnostics surface zero-value IndexName. Adapter
// client injection (InjectVault/InjectEmbedder/InjectEnvector) handles the
// gRPC connections; this method handles the per-token metadata.
func (d *Deps) ApplyVaultBundle(b *vault.Bundle) {
	if b == nil {
		return
	}
	if d.Capture != nil {
		d.Capture.AgentID = b.AgentID
		d.Capture.AgentDEK = b.AgentDEK
		d.Capture.IndexName = b.IndexName
	}
	if d.Recall != nil {
		d.Recall.IndexName = b.IndexName
	}
	if d.Lifecycle != nil {
		d.Lifecycle.IndexName = b.IndexName
		d.Lifecycle.KeyID = b.KeyID
		d.Lifecycle.AgentDEK = b.AgentDEK
		d.Lifecycle.EncKeyLoaded = len(b.EncKey) > 0
	}
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
//  1. mustAdd name validation (SDK's validateToolName has a log-only branch —
//     server.go:238-241 — that we bypass by panicking up-front).
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

	// Write tools — state-gated.
	mustAdd(srv, "rune_capture",
		"Capture a decision record (agent-delegated extraction required).",
		handleCapture(deps))
	mustAdd(srv, "rune_batch_capture",
		"Capture a batch of decision records (e.g. session-end sweep).",
		handleBatchCapture(deps))
	mustAdd(srv, "rune_recall",
		"Query organizational memory by natural-language question.",
		handleRecall(deps))
	mustAdd(srv, "rune_delete_capture",
		"Soft-delete a record by ID (sets status=reverted, re-inserts).",
		handleDeleteCapture(deps))

	// Read / diagnostic tools — bypass state gate.
	mustAdd(srv, "rune_capture_history",
		"List recent captures from local capture_log.jsonl (read-only).",
		handleCaptureHistory(deps))
	mustAdd(srv, "rune_vault_status",
		"Probe Vault connectivity and report secure-search mode.",
		handleVaultStatus(deps))
	mustAdd(srv, "rune_diagnostics",
		"Collect a 7-section health snapshot (env / state / vault / keys / pipelines / embedding / envector).",
		handleDiagnostics(deps))
	mustAdd(srv, "rune_reload_pipelines",
		"Re-initialize Vault + envector pipelines (BOOT replay) with envector warmup.",
		handleReloadPipelines(deps))

	return nil
}

// mustAdd wraps sdkmcp.AddTool with up-front name validation.
//
// The SDK's Server.AddTool only LOGS on invalid tool names
// (go-sdk/mcp/server.go:238-241) — it does not panic, so Register's
// defer recover() would miss it and the bad-named tool would silently
// register. mustAdd panics on invalid names, unifying the failure
// path so recover() catches everything.
func mustAdd[In, Out any](srv *sdkmcp.Server, name, description string, h sdkmcp.ToolHandlerFor[In, Out]) {
	if !isValidToolName(name) {
		panic(fmt.Errorf("mustAdd: invalid tool name %q (allowed: [A-Za-z0-9_-], 1..128 chars)", name))
	}
	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        name,
		Description: description,
	}, h)
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

