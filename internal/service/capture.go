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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/logio"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/lifecycle"
	"github.com/envector/rune-go/internal/policy"
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
	// Phase 2
	detection, extraction, err := domain.ParseExtractionFromAgent(req.Extracted)
	if err != nil {
		var rej *domain.CaptureRejection
		if errors.As(err, &rej) {
			reason := "no reason"
			if rej.Reason != "" {
				reason = rej.Reason
			}
			return &domain.CaptureResponse{
				OK:       true,
				Captured: false,
				Reason:   fmt.Sprintf("Agent rejected: %s", reason),
			}, nil
		}
		return nil, err
	}
	if extraction == nil {
		return nil, &domain.RuneError{Code: domain.CodeInvalidInput, Message: "extraction is nil after parse"}
	}

	// Phase 5: build policy
	rawEvent := &domain.RawEvent{
		Text:    req.Text,
		Source:  req.Source,
		User:    req.User,
		Channel: req.Channel,
	}
	if rawEvent.User == "" {
		rawEvent.User = "unknown"
	}
	if rawEvent.Channel == "" {
		rawEvent.Channel = "claude_session"
	}

	records, err := policy.BuildPhases(rawEvent, detection, extraction, s.Now())
	if err != nil {
		return nil, fmt.Errorf("build phases: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("build phases returned 0 records")
	}

	// Phase 3, 4
	embeddingText := pickEmbedText(&records[0])
	var noveltyInfo *domain.NoveltyInfo

	noveltyInfo, earlyResp, _ := s.runNoveltyCheck(ctx, embeddingText)
	if earlyResp != nil {
		return earlyResp, nil // near_duplicate
	}
	if noveltyInfo == nil {
		noveltyInfo = &domain.NoveltyInfo{Score: 1.0, Class: "novel"}
	}

	// Phase 5: embed, seal
	texts := make([]string, len(records))
	for i := range records {
		texts[i] = pickEmbedText(&records[i])
	}

	vectors, err := s.Embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed batch: %w", err)
	}

	envelopes, err := s.sealMetadata(records)
	if err != nil {
		return nil, fmt.Errorf("seal metadata: %w", err)
	}

	// Phase 6
	insertReq := envector.InsertRequest{
		Vectors:  vectors,
		Metadata: envelopes,
	}
	insertResult, err := s.Envector.Insert(ctx, insertReq)
	if err != nil {
		return nil, fmt.Errorf("envector insert: %w", err)
	}
	if insertResult != nil && len(insertResult.ItemIDs) != len(vectors) {
		slog.Error("envector insert inconsistency",
			"expected", len(vectors), "got", len(insertResult.ItemIDs))
	}

	// Phase 7
	first := records[0]
	if s.CaptureLog != nil {
		var noveltyScore *float64
		var noveltyClass string
		if noveltyInfo != nil {
			nsc := noveltyInfo.Score
			noveltyScore = &nsc
			noveltyClass = string(noveltyInfo.Class)
		}
		_ = s.CaptureLog.Append(domain.CaptureLogEntry{
			TS:           s.Now().UTC().Format(time.RFC3339),
			Action:       "captured",
			ID:           first.ID,
			Title:        first.Title,
			Domain:       string(first.Domain),
			Mode:         "agent-delegated",
			NoveltyClass: noveltyClass,
			NoveltyScore: noveltyScore,
		})
	}

	resp := &domain.CaptureResponse{
		OK:       true,
		Captured: true,
		RecordID: first.ID,
		Title:    first.Title,
		Domain:   first.Domain,
		Novelty:  noveltyInfo,
	}

	return resp, nil
}

// Batch — rune_batch_capture. Python: server.py:L810-896.
// Per-item independent processing; one item's failure does not abort others.
func (s *CaptureService) Batch(ctx context.Context, args BatchCaptureArgs) (*BatchCaptureResult, error) {
	var rawItems []map[string]any
	if err := json.Unmarshal([]byte(args.Items), &rawItems); err != nil {
		return nil, &domain.RuneError{Code: domain.CodeInvalidInput, Message: "invalid items JSON array"}
	}

	result := &BatchCaptureResult{
		OK:      true,
		Total:   len(rawItems),
		Results: make([]BatchItemResult, 0, len(rawItems)),
	}

	for i, item := range rawItems {
		text := ""
		if ri, ok := item["reusable_insight"].(string); ok && ri != "" {
			text = ri
		} else if t, ok := item["title"].(string); ok && t != "" {
			text = t
		} else {
			text = "[batch_capture]"
		}

		req := &domain.CaptureRequest{
			Text:      text,
			Source:    args.Source,
			Extracted: item,
		}
		if args.User != nil {
			req.User = *args.User
		}
		if args.Channel != nil {
			req.Channel = *args.Channel
		}

		resp, err := s.Handle(ctx, req)
		bir := BatchItemResult{Index: i}

		if err != nil {
			errMsg := err.Error()
			bir.Status = "error"
			bir.Error = &errMsg
			result.Errors++
		} else if resp.Captured {
			bir.Status = "captured"
			bir.Title = resp.Title
			if resp.Novelty != nil {
				bir.Novelty = string(resp.Novelty.Class)
			}
			result.Captured++
		} else {
			bir.Status = "skipped"
			if resp.Novelty != nil && resp.Novelty.Class == domain.NoveltyClassNearDuplicate {
				bir.Status = "near_duplicate"
			}
			result.Skipped++
		}
		result.Results = append(result.Results, bir)
	}

	return result, nil
}

// runNoveltyCheck — Phase 4 helper. Returns novelty info + nil if proceed,
// or a pre-built response if near_duplicate (caller short-circuits).
// Python: server.py:L1335-1369.
func (s *CaptureService) runNoveltyCheck(ctx context.Context, embeddingText string) (*domain.NoveltyInfo, *domain.CaptureResponse, error) {
	if s.Embedder == nil || s.Envector == nil || s.Vault == nil {
		return &domain.NoveltyInfo{Score: 1.0, Class: "novel"}, nil, nil
	}

	vec, err := s.Embedder.EmbedSingle(ctx, embeddingText)
	if err != nil {
		slog.Warn("novelty check: embed failed (non-fatal)", "err", err)
		return &domain.NoveltyInfo{Score: 1.0, Class: "novel"}, nil, nil
	}

	blobs, err := s.Envector.Score(ctx, vec)
	if err != nil || len(blobs) == 0 {
		slog.Warn("novelty check: score failed (non-fatal)", "err", err)
		return &domain.NoveltyInfo{Score: 1.0, Class: "novel"}, nil, nil
	}

	entries, err := s.Vault.DecryptScores(ctx, string(blobs[0]), 3)
	if err != nil || len(entries) == 0 {
		slog.Warn("novelty check: decrypt failed (non-fatal)", "err", err)
		return &domain.NoveltyInfo{Score: 1.0, Class: "novel"}, nil, nil
	}

	maxSim := 0.0
	for _, e := range entries {
		if e.Score > maxSim {
			maxSim = e.Score
		}
	}

	class, score := policy.ClassifyNovelty(maxSim, policy.DefaultNoveltyThresholds)
	noveltyInfo := &domain.NoveltyInfo{
		Score: score,
		Class: class,
	}

	if class == domain.NoveltyClassNearDuplicate {
		return noveltyInfo, &domain.CaptureResponse{
			OK:       true,
			Captured: false,
			Reason:   "Near-duplicate — virtually identical insight already stored",
			Novelty:  noveltyInfo,
		}, nil
	}

	return noveltyInfo, nil, nil
}

// sealMetadata — Phase 5 helper. For each record, json.Marshal → envector.Seal.
// Safety check (Python envector_sdk.py:L250-251): agent_dek present but agent_id missing → skip.
func (s *CaptureService) sealMetadata(records []domain.DecisionRecord) ([]string, error) {
	envelopes := make([]string, len(records))

	for i, rec := range records {
		body, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("marshal record %d: %w", i, err)
		}

		if len(s.AgentDEK) > 0 && s.AgentID != "" {
			sealed, err := envector.Seal(s.AgentDEK, s.AgentID, body)
			if err != nil {
				return nil, fmt.Errorf("seal record %d: %w", i, err)
			}
			envelopes[i] = sealed
		} else {
			envelopes[i] = string(body) // no DEK
		}
	}
	return envelopes, nil
}

func pickEmbedText(r *domain.DecisionRecord) string {
	if r.ReusableInsight != "" {
		return r.ReusableInsight
	}
	return r.Payload.Text // fallback
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
