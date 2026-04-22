package service

import (
	"context"
	"time"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/logio"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/lifecycle"
)

// LifecycleService holds the 6 lifecycle/operational tool implementations.
// Python:
//
//	server.py:L496-528   tool_vault_status
//	server.py:L540-684   tool_diagnostics
//	server.py:L1092-1111 tool_capture_history
//	server.py:L1123-1206 tool_delete_capture
//	server.py:L1046-1089 tool_reload_pipelines
//
// Batch lives in capture.go (shared capture flow).
// Spec: docs/v04/spec/flows/lifecycle.md.
type LifecycleService struct {
	Vault     vault.Client
	Envector  envector.Client
	Embedder  embedder.Client
	State     *lifecycle.Manager
	IndexName string
	ConfigDir string // for CaptureHistory reading capture_log.jsonl
}

// NewLifecycleService constructs.
func NewLifecycleService() *LifecycleService {
	return &LifecycleService{}
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. rune_vault_status — read-only. server.py:L496-528. Spec §1.
// ─────────────────────────────────────────────────────────────────────────────

// VaultStatusResult — lifecycle.md §1.
type VaultStatusResult struct {
	OK                    bool    `json:"ok"`
	VaultConfigured       bool    `json:"vault_configured"`
	VaultEndpoint         *string `json:"vault_endpoint,omitempty"`
	SecureSearchAvailable bool    `json:"secure_search_available"`
	Mode                  string  `json:"mode"` // "secure (Vault-backed)" | "standard (no Vault)"
	VaultHealthy          *bool   `json:"vault_healthy,omitempty"`
	TeamIndexName         *string `json:"team_index_name,omitempty"`
	Warning               *string `json:"warning,omitempty"`
}

// VaultStatus — branches on vault == nil (standard mode) vs configured.
// Python L526-528: health RPC failure → make_error(VaultConnectionError) + vault_configured flag.
func (s *LifecycleService) VaultStatus(ctx context.Context) (*VaultStatusResult, error) {
	// TODO: bit-identical to server.py:L496-528
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. rune_diagnostics — read-only. server.py:L540-684. Spec §2.
// 7 sections + OK=false iff vault unhealthy OR enc_key missing.
// ─────────────────────────────────────────────────────────────────────────────

// DiagnosticsResult — aggregates 7 sub-sections.
type DiagnosticsResult struct {
	OK            bool          `json:"ok"`
	Environment   EnvInfo       `json:"environment"`
	State         *string       `json:"state,omitempty"`
	DormantReason *string       `json:"dormant_reason,omitempty"`
	DormantSince  *string       `json:"dormant_since,omitempty"`
	Vault         VaultInfo     `json:"vault"`
	Keys          KeysInfo      `json:"keys"`
	Pipelines     PipelinesInfo `json:"pipelines"`
	Embedding     EmbeddingInfo `json:"embedding"`
	Envector      EnvectorInfo  `json:"envector"`
}

// EnvInfo — OS, Go runtime version, cwd.
type EnvInfo struct {
	OS        string `json:"os"`
	Runtime   string `json:"runtime"` // "go1.24.4" etc.
	CWD       string `json:"cwd"`
	GOArch    string `json:"goarch"`
	GOVersion string `json:"goversion"`
}

// VaultInfo — subset exposed in diagnostics.
type VaultInfo struct {
	Configured bool   `json:"configured"`
	Healthy    bool   `json:"healthy"`
	Endpoint   string `json:"endpoint,omitempty"`
}

// KeysInfo — memory-resident key state.
type KeysInfo struct {
	EncKeyLoaded   bool   `json:"enc_key_loaded"`
	KeyID          string `json:"key_id,omitempty"`
	AgentDEKLoaded bool   `json:"agent_dek_loaded"`
}

// PipelinesInfo — scribe/retriever init + active provider.
// Go: both always initialized when state=active; no LLM provider.
type PipelinesInfo struct {
	ScribeInitialized    bool   `json:"scribe_initialized"`
	RetrieverInitialized bool   `json:"retriever_initialized"`
	ActiveLLMProvider    string `json:"active_llm_provider,omitempty"` // always empty (Go agent-delegated)
}

// EmbeddingInfo — external embedder info snapshot (Info RPC cached).
type EmbeddingInfo struct {
	Model         string `json:"model"`         // from embedder.Info.model_identity
	Mode          string `json:"mode"`          // "external gRPC"
	VectorDim     int    `json:"vector_dim,omitempty"`
	DaemonVersion string `json:"daemon_version,omitempty"`
}

// EnvectorInfo — reachability probe (5s timeout).
type EnvectorInfo struct {
	Reachable bool    `json:"reachable"`
	LatencyMs float64 `json:"latency_ms,omitempty"`
	Error     string  `json:"error,omitempty"`
	ErrorType string  `json:"error_type,omitempty"` // connection_refused|auth_failure|deadline_exceeded|timeout|unknown
	ElapsedMs float64 `json:"elapsed_ms,omitempty"`
	Hint      string  `json:"hint,omitempty"`
}

// DiagnosticsTimeout — Python ENVECTOR_DIAGNOSIS_TIMEOUT (server.py:L633). 5s.
const DiagnosticsTimeout = 5 * time.Second

// Diagnostics collects all 7 sections + derives top-level OK.
func (s *LifecycleService) Diagnostics(ctx context.Context) *DiagnosticsResult {
	// TODO:
	//  r := &DiagnosticsResult{OK: true}
	//  r.Environment = s.collectEnv()
	//  cfg read → r.State / DormantReason / DormantSince
	//  r.Vault = s.collectVault(ctx)
	//  r.Keys = s.collectKeys()
	//  r.Pipelines = s.collectPipelines()
	//  r.Embedding = s.collectEmbedding(ctx) — uses embedder.Info cache
	//  r.Envector = s.collectEnvector(ctx, DiagnosticsTimeout)
	//  if r.Vault.Configured && !r.Vault.Healthy { r.OK = false }
	//  if !r.Keys.EncKeyLoaded { r.OK = false }
	//  return r
	_ = ctx
	return &DiagnosticsResult{}
}

// collectEnvector wraps GetIndexList under 5s timeout + ClassifyEnvectorError.
// Python server.py:L626-676.
func (s *LifecycleService) collectEnvector(ctx context.Context, timeout time.Duration) EnvectorInfo {
	// TODO:
	//  context.WithTimeout + goroutine + select {resultCh, ctx2.Done()}
	//  on success → {Reachable: true, LatencyMs: ...}
	//  on error → ClassifyEnvectorError(err, latency) → {Reachable: false, ErrorType, Hint, ...}
	//  on timeout → {Reachable: false, ErrorType: "timeout", ElapsedMs: ...}
	return EnvectorInfo{}
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. rune_capture_history — read-only. server.py:L1092-1111. Spec §4.
// ─────────────────────────────────────────────────────────────────────────────

// CaptureHistoryArgs — limit default 20, max 100.
type CaptureHistoryArgs struct {
	Limit  int     `json:"limit,omitempty"`
	Domain *string `json:"domain,omitempty"`
	Since  *string `json:"since,omitempty"` // ISO date lex compare
}

// CaptureHistoryResult — entries preserved as map for format flexibility.
type CaptureHistoryResult struct {
	OK      bool             `json:"ok"`
	Count   int              `json:"count"`
	Entries []map[string]any `json:"entries"`
}

// CaptureHistory — reverse-read capture_log.jsonl, filter, cap at limit.
// Python degrade: on read error return empty list (Python except → []).
func (s *LifecycleService) CaptureHistory(ctx context.Context, args CaptureHistoryArgs) (*CaptureHistoryResult, error) {
	// TODO:
	//  if args.Limit == 0 { args.Limit = 20 }
	//  if args.Limit > 100 { args.Limit = 100 }
	//  entries, err := logio.Tail(path, args.Limit, args.Domain, args.Since)
	//  if err != nil { return empty result, nil } // Python degrade
	_ = args
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. rune_delete_capture — soft-delete. server.py:L1123-1206. Spec §5.
// ─────────────────────────────────────────────────────────────────────────────

// DeleteCaptureArgs — single record ID target.
type DeleteCaptureArgs struct {
	RecordID string `json:"record_id"`
}

// DeleteCaptureResult.
type DeleteCaptureResult struct {
	OK       bool   `json:"ok"`
	Deleted  bool   `json:"deleted"`
	RecordID string `json:"record_id"`
	Title    string `json:"title"`
	Method   string `json:"method"` // "soft-delete (status=reverted)"
}

// DeleteCapture — soft-delete workflow:
//  1. SearchByID(id) — embedder.EmbedSingle("ID: {id}") + searchSingle + filter
//  2. set metadata["status"] = "reverted"
//  3. re-embed + re-insert (reusable_insight > payload_text)
//  4. capture_log append with mode="soft-delete", action="deleted"
//  5. on Vault error → state.SetDormant("vault_unreachable")
//     on envector error → state.SetDormant("envector_unreachable")
//     (Python server.py:L1180-1206)
//
// NOTE: re-encrypt uses capture Phase 5b logic (envector.Seal). Consider
// calling into CaptureService helper or sharing via search.go.
func (s *LifecycleService) DeleteCapture(ctx context.Context, args DeleteCaptureArgs, capSvc *CaptureService) (*DeleteCaptureResult, error) {
	// TODO: implement soft-delete with dormant-on-error side effects
	_ = ctx
	_ = args
	_ = capSvc
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. rune_reload_pipelines — server.py:L1046-1089. Spec §6.
// ─────────────────────────────────────────────────────────────────────────────

// ReloadPipelinesResult.
type ReloadPipelinesResult struct {
	OK                   bool        `json:"ok"`
	State                string      `json:"state"`
	ScribeInitialized    bool        `json:"scribe_initialized"`
	RetrieverInitialized bool        `json:"retriever_initialized"`
	Errors               []string    `json:"errors,omitempty"`
	EnvectorWarmup       *WarmupInfo `json:"envector_warmup,omitempty"`
}

// WarmupInfo — GetIndexList probe (60s timeout).
type WarmupInfo struct {
	OK        bool     `json:"ok"`
	LatencyMs *float64 `json:"latency_ms,omitempty"`
	Error     *string  `json:"error,omitempty"`
}

// WarmupTimeout — Python WARMUP_TIMEOUT (server.py:L1059). 60s.
const WarmupTimeout = 60 * time.Second

// ReloadPipelines — re-init + warmup. Requires AwaitInitDone to avoid races.
// Python server.py:L1046-1089.
func (s *LifecycleService) ReloadPipelines(ctx context.Context) (*ReloadPipelinesResult, error) {
	// TODO:
	//  s.State.AwaitInitDone()       — wait for any in-flight boot
	//  s.State.ClearError()
	//  res := s.State.ReinitPipelines(ctx)
	//  if res.ScribeInit && s.Envector != nil {
	//      warmup := s.warmupEnvector(ctx, WarmupTimeout)
	//  }
	//  return compose result
	_ = ctx
	return nil, nil
}

// warmupEnvector — GetIndexList under 60s timeout.
func (s *LifecycleService) warmupEnvector(ctx context.Context, timeout time.Duration) *WarmupInfo {
	// TODO: context.WithTimeout + GetIndexList probe
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tail wrapper — re-exports logio for handlers that need raw entries.
// ─────────────────────────────────────────────────────────────────────────────
var _ = logio.DefaultFilename // keep import edge; TODO remove when Tail wired
