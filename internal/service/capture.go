// Package service holds the orchestration layer — multi-phase flows that
// coordinate adapters + policy. MCP tool handlers (internal/mcp/tools.go)
// delegate to these services; business logic lives here, not in handlers.
//
// Spec:
//   docs/v04/spec/flows/capture.md (7-phase)
//   docs/v04/spec/flows/recall.md (7-phase)
//   docs/v04/spec/flows/lifecycle.md (6 tools)
package service

import (
	"context"
	"time"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/logio"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/lifecycle"
)

// CaptureService orchestrates the 7-phase capture flow.
// Python: mcp/server/server.py:L1208-1407 _capture_single + L810-896 tool_batch_capture.
type CaptureService struct {
	Vault      vault.Client
	Envector   envector.Client
	Embedder   embedder.Client
	CaptureLog *logio.CaptureLog
	State      *lifecycle.Manager

	// Injected from Vault bundle at boot.
	AgentID   string
	AgentDEK  []byte // 32B validated (vault.ValidateAgentDEK)
	IndexName string

	Now func() time.Time // injectable clock (default: time.Now)
}

// NewCaptureService constructs with default clock.
// TODO: wire in from main.go / lifecycle boot.
func NewCaptureService() *CaptureService {
	return &CaptureService{Now: time.Now}
}

// Handle — single capture. Called by internal/mcp/tools.go ToolCapture.
// Python: server.py:L1208-1407 _capture_single.
//
// Flow (per spec/flows/capture.md):
//
//	Phase 1 (in handler): state gate → PIPELINE_NOT_READY if not active
//	Phase 2: validate text + parse extracted (Detection + ExtractionResult split)
//	         tier2.capture=false → early rejection {captured:false, reason}
//	Phase 3: embedder.EmbedSingle(text_to_embed) — reusable_insight > payload.text
//	Phase 4: envector.Score → Vault.DecryptScores(top_k=3) → novelty classify
//	         near_duplicate (≥0.95) → return {captured:false, novelty{class, score, related}}
//	         failures non-fatal (server.py:L1370-1372 logger.warning)
//	Phase 5: policy.BuildPhases → embedder.EmbedBatch(texts) → envector.Seal × N
//	Phase 6: envector.Insert (atomic batch, D17)
//	Phase 7: capture_log append (degrade per D19) → respond
func (s *CaptureService) Handle(ctx context.Context, req *domain.CaptureRequest) (*domain.CaptureResponse, error) {
	// TODO Phase 2: validate + detection, extraction := domain.ParseExtractionFromAgent(req.Extracted)
	// TODO Phase 2 early reject: if tier2.capture=false → return {captured:false, reason}
	// TODO Phase 3: embedText := policy.PickTextToEmbed(req.Extracted)
	//               vec, err := s.Embedder.EmbedSingle(ctx, embedText)
	// TODO Phase 4: runNoveltyCheck → near_duplicate early return
	// TODO Phase 5: records := policy.BuildPhases(rawEvent, detection, extraction, clock)
	//               texts := selectEmbedTextsForRecords(records)
	//               vectors := s.Embedder.EmbedBatch(ctx, texts)
	//               envelopes := sealMetadata(records) via envector.Seal
	// TODO Phase 6: result := s.Envector.Insert(InsertRequest{Vectors, Metadata: envelopes})
	//               len(ItemIDs) != len(Vectors) → ErrEnvectorInconsistent (D17 probe)
	// TODO Phase 7: s.CaptureLog.Append + respond
	_ = ctx
	_ = req
	return nil, nil
}

// Batch — rune_batch_capture. Python: server.py:L810-896.
// Per-item independent processing; one item's failure does not abort others.
// Each item classified: captured / skipped / near_duplicate / error.
//
// Spec: docs/v04/spec/flows/lifecycle.md §3.
func (s *CaptureService) Batch(ctx context.Context, args BatchCaptureArgs) (*BatchCaptureResult, error) {
	// TODO:
	//  1. json.Unmarshal(args.Items) → []json.RawMessage
	//  2. iterate: build CaptureRequest per item (text = reusable_insight||title||"[batch_capture]")
	//  3. call s.Handle(ctx, req); classify status
	//  4. aggregate captured/skipped/errors counts
	_ = ctx
	_ = args
	return nil, nil
}

// runNoveltyCheck — Phase 4 helper. Returns novelty info + nil if proceed,
// or a pre-built response if near_duplicate (caller short-circuits).
// Python: server.py:L1335-1369.
func (s *CaptureService) runNoveltyCheck(ctx context.Context, vec []float32) (*domain.NoveltyInfo, *domain.CaptureResponse, error) {
	// TODO:
	//  blobs := s.Envector.Score(ctx, vec)
	//  entries := s.Vault.DecryptScores(ctx, blobs[0], 3)
	//  if len(entries)==0 → return NoveltyInfo{Novel, 1.0, []}, nil, nil
	//  class, score := policy.ClassifyNovelty(entries[0].Score, DefaultNoveltyThresholds)
	//  related = buildRelatedTop3(entries)  // server.py:L1353-1360
	//  if class == NearDuplicate → return novelty, earlyRejectResponse, nil
	//  return novelty, nil, nil
	return nil, nil, nil
}

// sealMetadata — Phase 5 helper. For each record, json.Marshal → envector.Seal.
// Safety check (Python envector_sdk.py:L250-251): agent_dek present but agent_id missing → skip.
func (s *CaptureService) sealMetadata(records []domain.DecisionRecord) ([]string, error) {
	// TODO: for each record → json.Marshal → envector.Seal(dek, agentID, body)
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Batch types — lifecycle.md §3
// ─────────────────────────────────────────────────────────────────────────────

// BatchCaptureArgs — Python: server.py:L810 tool_batch_capture args.
type BatchCaptureArgs struct {
	Items   string  `json:"items"` // JSON array string (agent-supplied)
	Source  string  `json:"source,omitempty"`
	User    *string `json:"user,omitempty"`
	Channel *string `json:"channel,omitempty"`
}

// BatchCaptureResult — aggregated response.
type BatchCaptureResult struct {
	OK       bool              `json:"ok"`
	Total    int               `json:"total"`
	Results  []BatchItemResult `json:"results"`
	Captured int               `json:"captured"`
	Skipped  int               `json:"skipped"`
	Errors   int               `json:"errors"`
}

// BatchItemResult — per-item outcome.
type BatchItemResult struct {
	Index   int     `json:"index"`
	Title   string  `json:"title"`
	Status  string  `json:"status"` // "captured" | "skipped" | "near_duplicate" | "error"
	Novelty string  `json:"novelty,omitempty"`
	Error   *string `json:"error,omitempty"`
}
