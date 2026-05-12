package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/envector/rune-go/internal/adapters/config"
	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/logio"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/lifecycle"
)

// LifecycleService holds the 6 lifecycle/operational tool implementations.
// Spec: docs/v04/spec/flows/lifecycle.md.
type LifecycleService struct {
	Vault     vault.Client
	Envector  envector.Client
	Embedder  embedder.Client
	State     *lifecycle.Manager
	IndexName string
	ConfigDir string // for CaptureHistory reading capture_log.jsonl

	// Key state (for diagnostics)
	EncKeyLoaded bool
	KeyID        string
	AgentDEK     []byte
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
func (s *LifecycleService) VaultStatus(ctx context.Context) (*VaultStatusResult, error) {
	if s.Vault == nil {
		warning := "secret key may be accessible locally. Configure Vault for secure mode."
		return &VaultStatusResult{
			OK:              true,
			VaultConfigured: false,
			Mode:            "standard (no Vault)",
			Warning:         &warning,
		}, nil
	}

	endpoint := s.Vault.Endpoint()
	healthy, err := s.Vault.HealthCheck(ctx)
	if err != nil {
		slog.Warn("vault health check failed", "err", err)
		h := false
		return &VaultStatusResult{
			OK:              true,
			VaultConfigured: true,
			VaultEndpoint:   &endpoint,
			VaultHealthy:    &h,
			Mode:            "secure (Vault-backed)",
		}, nil
	}

	return &VaultStatusResult{
		OK:                    true,
		VaultConfigured:       true,
		VaultEndpoint:         &endpoint,
		SecureSearchAvailable: healthy,
		Mode:                  "secure (Vault-backed)",
		VaultHealthy:          &healthy,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. rune_diagnostics — read-only. server.py:L540-684. Spec §2.
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
	OS      string `json:"os"`
	Runtime string `json:"runtime"`
	CWD     string `json:"cwd"`
	GOArch  string `json:"goarch"`
}

// VaultInfo — subset exposed in diagnostics.
type VaultInfo struct {
	Configured bool   `json:"configured"`
	Healthy    bool   `json:"healthy"`
	Endpoint   string `json:"endpoint,omitempty"`
	Error      string `json:"error,omitempty"`
}

// KeysInfo — memory-resident key state.
type KeysInfo struct {
	EncKeyLoaded   bool   `json:"enc_key_loaded"`
	KeyID          string `json:"key_id,omitempty"`
	AgentDEKLoaded bool   `json:"agent_dek_loaded"`
}

// PipelinesInfo — scribe/retriever init + active provider.
type PipelinesInfo struct {
	ScribeInitialized    bool   `json:"scribe_initialized"`
	RetrieverInitialized bool   `json:"retriever_initialized"`
	ActiveLLMProvider    string `json:"active_llm_provider,omitempty"` // always empty (Go agent-delegated)
}

// EmbeddingInfo — external embedder info snapshot
type EmbeddingInfo struct {
	Model         string `json:"model"`
	Mode          string `json:"mode"` // "external gRPC"
	VectorDim     int    `json:"vector_dim,omitempty"`
	DaemonVersion string `json:"daemon_version,omitempty"`
}

// EnvectorInfo — reachability probe
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
	r := &DiagnosticsResult{OK: true}

	// Environment
	cwd, _ := os.Getwd()
	r.Environment = EnvInfo{
		OS:      runtime.GOOS,
		Runtime: runtime.Version(),
		CWD:     cwd,
		GOArch:  runtime.GOARCH,
	}

	// Config state
	cfg, err := config.Load()
	if err == nil && cfg != nil {
		state := cfg.State
		r.State = &state
		if cfg.DormantReason != "" {
			r.DormantReason = &cfg.DormantReason
		}
		if cfg.DormantSince != "" {
			r.DormantSince = &cfg.DormantSince
		}
	}

	// Vault
	r.Vault = s.collectVault(ctx, DiagnosticsTimeout)

	// Keys
	r.Keys = KeysInfo{
		EncKeyLoaded:   s.EncKeyLoaded,
		KeyID:          s.KeyID,
		AgentDEKLoaded: len(s.AgentDEK) > 0,
	}

	// Pipelines
	r.Pipelines = PipelinesInfo{
		ScribeInitialized:    s.State != nil && s.State.Current() == lifecycle.StateActive,
		RetrieverInitialized: s.State != nil && s.State.Current() == lifecycle.StateActive,
	}

	// Embedding
	r.Embedding = s.collectEmbedding(ctx, DiagnosticsTimeout)

	// Envector
	r.Envector = s.collectEnvector(ctx, DiagnosticsTimeout)

	if s.Vault != nil && !r.Vault.Healthy {
		r.OK = false
	}
	if !r.Keys.EncKeyLoaded {
		r.OK = false
	}

	return r
}

func (s *LifecycleService) collectVault(ctx context.Context, timeout time.Duration) VaultInfo {
	info := VaultInfo{Configured: s.Vault != nil}
	if s.Vault == nil {
		return info
	}

	info.Endpoint = s.Vault.Endpoint()

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	healthy, err := s.Vault.HealthCheck(ctx2)
	if err != nil {
		info.Error = err.Error()
	}

	info.Healthy = healthy

	return info
}

func (s *LifecycleService) collectEmbedding(ctx context.Context, timeout time.Duration) EmbeddingInfo {
	info := EmbeddingInfo{Mode: "external gRPC"}
	if s.Embedder == nil {
		return info
	}

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	snap, err := s.Embedder.Info(ctx2)
	if err != nil {
		return info
	}

	info.Model = snap.ModelIdentity
	info.VectorDim = snap.VectorDim
	info.DaemonVersion = snap.DaemonVersion

	return info
}

// collectEnvector probes envector, and on any envector adapter error
// re-triggers the boot loop and probes once more before reporting
// This is because the user only configures Vault credentails; the enVector
// endpoint and keys arrive via Vault (recovery is system's job)
func (s *LifecycleService) collectEnvector(ctx context.Context, timeout time.Duration) EnvectorInfo {
	if s.Envector == nil {
		return EnvectorInfo{}
	}

	info, err := s.probeEnvector(ctx, timeout)
	if err == nil {
		return info
	}
	if s.State == nil || !envector.IsAdapterError(err) {
		return info
	}

	slog.Warn("diagnostics: envector probe failed - re-bootstrapping", "err", err)
	s.State.Retrigger()
	if !waitForActiveAfterRetrigger(ctx, s.State, retriggerSettleTimeout) {
		slog.Warn("diagnostics: re-boot did not settle to active")
		return info
	}

	info2, _ := s.probeEnvector(ctx, timeout)
	return info2
}

// Decide whether to attemp recovery or not
func (s *LifecycleService) probeEnvector(ctx context.Context, timeout time.Duration) (EnvectorInfo, error) {
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ch := make(chan error, 1)
	t0 := time.Now()

	go func() {
		_, err := s.Envector.GetIndexList(ctx2)
		ch <- err
	}()

	select {
	case probeErr := <-ch:
		elapsed := time.Since(t0)
		if probeErr == nil {
			return EnvectorInfo{
				Reachable: true,
				LatencyMs: float64(elapsed.Milliseconds()),
			}, nil
		}
		errType, hint := ClassifyEnvectorError(probeErr, elapsed)
		return EnvectorInfo{
			Error:     probeErr.Error(),
			ErrorType: string(errType),
			Hint:      hint,
			ElapsedMs: float64(elapsed.Milliseconds()),
		}, probeErr
	case <-ctx2.Done():
		elapsed := time.Since(t0)
		return EnvectorInfo{
			Error: fmt.Sprintf(
				"Health check timed out after %.0fs (elapsed: %.1fms).",
				timeout.Seconds(), float64(elapsed.Milliseconds()),
			),
			ErrorType: string(EnvErrTimeout),
			ElapsedMs: float64(elapsed.Milliseconds()),
		}, ctx2.Err()
	}
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
func (s *LifecycleService) CaptureHistory(_ context.Context, args CaptureHistoryArgs) (*CaptureHistoryResult, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	logPath := filepath.Join(s.ConfigDir, logio.DefaultFilename)
	entries, err := logio.Tail(logPath, limit, args.Domain, args.Since)
	if err != nil {
		slog.Warn("capture history read failed (degraded)", "err", err)
		return &CaptureHistoryResult{OK: true, Entries: []map[string]any{}}, nil
	}

	// Convert to map[string]any for format flexibility
	mapEntries := make([]map[string]any, len(entries))
	for i, e := range entries {
		data, _ := json.Marshal(e)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		mapEntries[i] = m
	}

	return &CaptureHistoryResult{
		OK:      true,
		Count:   len(mapEntries),
		Entries: mapEntries,
	}, nil
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
//  1. SearchByID(id): find record
//  2. set metadata["status"] = "reverted"
//  3. re-embed + re-insert
//  4. capture_log append with mode="soft-delete", action="deleted"
func (s *LifecycleService) DeleteCapture(ctx context.Context, args DeleteCaptureArgs, capSvc *CaptureService) (*DeleteCaptureResult, error) {
	// Search by ID
	hit, err := SearchByID(ctx, s.Embedder, s.Vault, s.Envector, s.IndexName, args.RecordID)
	if err != nil {
		return nil, fmt.Errorf("delete: search by ID: %w", err)
	}
	if hit == nil {
		return nil, &domain.RuneError{
			Code:    domain.CodeInvalidInput,
			Message: fmt.Sprintf("record %s not found", args.RecordID),
		}
	}

	// Mutate metadata
	metadata := hit.Metadata
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["status"] = "reverted"
	title := hit.Title

	// Re-embed + re-insert
	embedText := hit.ReusableInsight
	if embedText == "" {
		embedText = hit.PayloadText
	}

	vec, err := s.Embedder.EmbedSingle(ctx, embedText)
	if err != nil {
		return nil, fmt.Errorf("delete: re-embed: %w", err)
	}

	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("delete: marshal: %w", err)
	}

	if capSvc == nil || len(capSvc.AgentDEK) == 0 || capSvc.AgentID == "" {
		return nil, fmt.Errorf("delete: missing agent DEK or ID for encryption")
	}

	envelope, err := envector.Seal(capSvc.AgentDEK, capSvc.AgentID, body)
	if err != nil {
		return nil, fmt.Errorf("delete: seal: %w", err)
	}

	insertReq := envector.InsertRequest{
		Vectors:  [][]float32{vec},
		Metadata: []string{envelope},
	}
	_, err = s.Envector.Insert(ctx, insertReq)
	if err != nil {
		return nil, fmt.Errorf("delete: re-insert: %w", err)
	}

	// Capture log
	if capSvc != nil && capSvc.CaptureLog != nil {
		_ = capSvc.CaptureLog.Append(domain.CaptureLogEntry{
			TS:     time.Now().UTC().Format(time.RFC3339),
			Action: "deleted",
			ID:     args.RecordID,
			Title:  title,
			Domain: hit.Domain,
			Mode:   "soft-delete",
		})
	}

	return &DeleteCaptureResult{
		OK:       true,
		Deleted:  true,
		RecordID: args.RecordID,
		Title:    title,
		Method:   "soft-delete (status=reverted)",
	}, nil
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

// ReloadPipelines — re-trigger the boot loop from Dormant + warmup envector.
//
// On a terminal Dormant state (boot loop's goroutine has exited), call
// Manager.Retrigger to spawn a fresh RunBootLoop bound to the same ctx +
// Deps; main.go wires the spawn callback at startup. Manager.Retrigger
// is a silent no-op when state is Starting / WaitingForVault / Active —
// safe to call unconditionally if the trigger surface ever widens.
//
// /rune:activate from a freshly-spawned MCP server (no ~/.rune/config.json
// at boot, then user ran /rune:configure) reaches Active via this path.
// No process restart is required.
func (s *LifecycleService) ReloadPipelines(ctx context.Context) (*ReloadPipelinesResult, error) {
	// Always re-trigger so config changes such as new vault endpoint and rotated token are picked
	// without restarting MCP
	s.State.Retrigger()
	s.waitForBootProgress(ctx, 5*time.Second)

	result := &ReloadPipelinesResult{
		OK:    true,
		State: s.State.Current().String(),
	}

	if s.State.Current() == lifecycle.StateActive {
		result.ScribeInitialized = true
		result.RetrieverInitialized = true
	}

	if s.Envector != nil {
		warmup := s.warmupEnvector(ctx, WarmupTimeout)
		result.EnvectorWarmup = warmup
	}

	return result, nil
}

// waitForBootProgress polls Manager.Current() until either Active or a
// terminal Dormant is reached, or the deadline elapses. The caller has
// already triggered a fresh boot loop; this just gives it room to make
// progress before we snapshot state for the response. WaitingForVault
// (transient retry) is treated as still-in-progress because the loop is
// actively retrying with backoff and may yet reach Active.
//
// Initial 150ms grace period: Retrigger schedules a `go RunBootLoop(...)`
// — there is a brief window between the call returning and the spawned
// goroutine reaching its first `m.SetState(StateStarting)`. Without the
// grace, the very first state read can still see the prior Dormant
// snapshot and exit immediately.
func (s *LifecycleService) waitForBootProgress(ctx context.Context, timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	select {
	case <-ctx.Done():
		return
	case <-time.After(150 * time.Millisecond):
	}

	for time.Now().Before(deadline) {
		switch s.State.Current() {
		case lifecycle.StateActive, lifecycle.StateDormant:
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// warmupEnvector — GetIndexList under 60s timeout.
func (s *LifecycleService) warmupEnvector(ctx context.Context, timeout time.Duration) *WarmupInfo {
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	t0 := time.Now()
	_, err := s.Envector.GetIndexList(ctx2)
	elapsed := float64(time.Since(t0).Milliseconds())

	if err != nil {
		errStr := err.Error()
		return &WarmupInfo{OK: false, LatencyMs: &elapsed, Error: &errStr}
	}

	return &WarmupInfo{OK: true, LatencyMs: &elapsed}
}
