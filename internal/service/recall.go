package service

import (
	"context"
	"time"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/lifecycle"
)

// RecallService orchestrates the 7-phase recall flow.
// Python: mcp/server/server.py:L910-1034 tool_recall + agents/retriever/searcher.py.
// Spec: docs/v04/spec/flows/recall.md.
type RecallService struct {
	Vault     vault.Client
	Envector  envector.Client
	Embedder  embedder.Client
	State     *lifecycle.Manager
	IndexName string
	Now       func() time.Time
}

// NewRecallService constructs with default clock.
func NewRecallService() *RecallService {
	return &RecallService{Now: time.Now}
}

// Handle — Python: server.py:L910-1034 tool_recall + searcher.search().
//
// Flow (per spec/flows/recall.md):
//
//	Phase 1 (in handler): state gate + arg validation (empty query D24, topk ≤ 10)
//	Phase 2: policy.Parse — intent/time/entities/keywords/expansions (English only D21)
//	Phase 3: embedder.EmbedBatch(expansions[:3]) — D22 cap, D23 batch
//	Phase 4: searchWithExpansions — sequential per expansion (D25 MVP)
//	         per-expansion 4 RPC: envector.Score → Vault.DecryptScores →
//	                              envector.GetMetadata → (Vault.DecryptMetadata in Phase 5)
//	         dedup by record_id + sort by raw score desc
//	Phase 5: resolveMetadata — classify entries (AES/plain/legacy base64 D26)
//	         batch DecryptMetadata + per-entry fallback on batch failure
//	Phase 6: expandPhaseChains (D27) → assembleGroups → applyMetadataFilters →
//	         policy.FilterByTime → policy.ApplyRecencyWeighting → [:topk]
//	Phase 7: buildResult — synthesized=false fixed (D28 agent-delegated)
func (s *RecallService) Handle(ctx context.Context, args *domain.RecallArgs) (*domain.RecallResult, error) {
	// TODO Phase 2: parsed := policy.Parse(args.Query)
	// TODO Phase 3: vectors, err := s.Embedder.EmbedBatch(ctx, parsed.ExpandedQueries[:3])
	// TODO Phase 4: hits, err := s.searchWithExpansions(ctx, args.Query, parsed.ExpandedQueries, vectors, args.TopK)
	// TODO Phase 5: resolveMetadata already happens inside searchSingle (via Phase 4 flow)
	// TODO Phase 6: hits = s.expandPhaseChains(ctx, hits, vectors[0])
	//               hits = s.assembleGroups(hits)
	//               hits = s.applyMetadataFilters(hits, filtersFromArgs(args))
	//               hits = policy.FilterByTime(hits, parsed.TimeScope, s.Now())
	//               hits = policy.ApplyRecencyWeighting(hits, s.Now())
	//               if len(hits) > args.TopK { hits = hits[:args.TopK] }
	// TODO Phase 7: return s.buildResult(hits), nil
	_ = ctx
	_ = args
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 4 — search orchestration
// ─────────────────────────────────────────────────────────────────────────────

// searchWithExpansions — Python: searcher.py:L153-176 _search_with_expansions.
//   - iterate exps[:3] sequentially (D25)
//   - per-expansion: searchSingle → dedup by record_id
//   - original fallback (L167-173): if query.original not in exps, re-embed + search
//   - sort by raw score desc (stable)
//   - per-expansion failure = log warn + continue (Python L468-470 best-effort)
func (s *RecallService) searchWithExpansions(
	ctx context.Context,
	original string,
	exps []string,
	vectors [][]float32,
	topk int,
) ([]domain.SearchHit, error) {
	// TODO
	return nil, nil
}

// searchSingle — Python: searcher.py:L371-373 + L375-470 _search_via_vault.
// 4-RPC sequence per vector:
//  1. envector.Score(vec) → encrypted blobs
//  2. Vault.DecryptScores(blobs[0], topk) → [{shard, row, score}]
//  3. envector.GetMetadata(refs, ["metadata"]) → encrypted metadata entries
//  4. resolveMetadata → plaintext hits
func (s *RecallService) searchSingle(ctx context.Context, vec []float32, topk int) ([]domain.SearchHit, error) {
	// TODO
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 5 — metadata classification + Vault-delegated decrypt (D26)
// ─────────────────────────────────────────────────────────────────────────────

// metadataFormat — 3-way dispatch for encrypted metadata entries.
type metadataFormat int

const (
	fmtUnrecognized metadataFormat = iota
	fmtAESEnvelope                 // {"a": ..., "c": ...}
	fmtPlainJSON                   // already a JSON dict
	fmtBase64JSON                  // legacy format
)

// classifyMetadata — Python: searcher.py:L417-464 inline logic.
// Returns the format + the parsed value (nil for AES, which is passed to Vault).
func classifyMetadata(data string) (metadataFormat, any) {
	// TODO:
	//  1. Try JSON parse → if has "a" and "c" → AES envelope (return original)
	//     else plain JSON (return parsed)
	//  2. Try base64.StdEncoding.DecodeString → JSON parse → base64 JSON
	//  3. Otherwise → unrecognized
	_ = data
	return fmtUnrecognized, nil
}

// resolveMetadata — Python: searcher.py:L417-464 + _to_search_result.
// For each entry: classify + batch Vault.DecryptMetadata + per-entry fallback.
// Unrecognized / failed entries → empty metadata (logged warn, not fatal).
func (s *RecallService) resolveMetadata(ctx context.Context, entries []envector.MetadataEntry) ([]domain.SearchHit, error) {
	// TODO:
	//  - classify each entry; collect AES envelopes as aesItems
	//  - batch decrypt via s.Vault.DecryptMetadata(aesList)
	//  - on batch failure: per-entry loop fallback
	//  - convert to SearchHit via toSearchHit (domain.ExtractPayloadText)
	_ = ctx
	_ = entries
	return nil, nil
}

// toSearchHit — Python: searcher.py:L472-521 _to_search_result.
// Field paths (bit-identical):
//   - id fallback chain: metadata["id"] → raw["id"] → "unknown"
//   - title: default "Untitled"
//   - domain: default "general"
//   - status: default "unknown"
//   - certainty: NESTED metadata["why"]["certainty"], default "unknown"
//   - payload_text: domain.ExtractPayloadText (strict v2.1, D32)
//   - group fields optional (group_id / group_type / phase_seq / phase_total)
func toSearchHit(entry envector.MetadataEntry, metadata map[string]any, score float64) domain.SearchHit {
	// TODO: bit-identical per searcher.py:L472-521
	return domain.SearchHit{}
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 6 — group expansion, filters (Python-specific logic)
// ─────────────────────────────────────────────────────────────────────────────

// expandPhaseChains — Python: searcher.py:L306-365 _expand_phase_chains.
// Detects phase groups with missing siblings (phase_total > present count),
// picks max 2 groups, re-searches with "Group: {gid}" query, merges in phase order.
//
// Issues 4 additional RPCs per expanded group (embedder + searchSingle).
// MVP keeps this (D27); Post-MVP perf eval.
func (s *RecallService) expandPhaseChains(ctx context.Context, results []domain.SearchHit, origVec []float32) []domain.SearchHit {
	// TODO: per recall.md Phase 6 Step 1
	return results
}

// assembleGroups — Python: searcher.py:L178-226 _assemble_groups.
// Groups hits by group_id (best_score per group) + standalone hits.
// Interleaves sorted by best_score desc; phase_seq asc within group.
func (s *RecallService) assembleGroups(results []domain.SearchHit) []domain.SearchHit {
	// TODO: per recall.md Phase 6 Step 2
	return results
}

// Filters — user-supplied filter args (subset of RecallArgs).
type Filters struct {
	Domain *string
	Status *string
	Since  *string // ISO date "YYYY-MM-DD"
}

// applyMetadataFilters — Python: searcher.py:L228-252 _apply_metadata_filters.
// domain + status equality + since ISO date lex comparison.
// No-timestamp records are kept (Python behavior).
func (s *RecallService) applyMetadataFilters(results []domain.SearchHit, f Filters) []domain.SearchHit {
	// TODO: per recall.md Phase 6 Step 3
	return results
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 7 — response build
// ─────────────────────────────────────────────────────────────────────────────

// buildResult — Python: server.py:L950-990 agent-delegated path.
// Includes calculateConfidence (L393-412) + top-5 sources.
// Synthesized = false fixed (D28).
func (s *RecallService) buildResult(results []domain.SearchHit) *domain.RecallResult {
	// TODO: per recall.md Phase 7
	return nil
}

// calculateConfidence — Python: server.py:L393-412.
// See internal/policy/rerank.go for formula docs (may migrate here if unused elsewhere).
// Top-5 weighted sum / 2.0 clamp 1.0 round 2 decimals.
// "/2.0" is the approximate normalization for the harmonic partial sum of
// (1 + 1/2 + 1/3 + 1/4 + 1/5 ≈ 2.283); see docs/v04/spec/flows/recall.md Phase 7.
func calculateConfidence(results []domain.SearchHit) float64 {
	// TODO
	return 0
}
