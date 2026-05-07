package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/lifecycle"
	"github.com/envector/rune-go/internal/policy"
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
// TODO: External IO calls (Embedder, Envector, Vault) do not have explicit timeouts.
// We should add context timeouts (context.WithTimeout) for these operations after we determine optimal duration
func (s *RecallService) Handle(ctx context.Context, args *domain.RecallArgs) (*domain.RecallResult, error) {
	// Phase 2: parse query
	parsed := policy.Parse(args.Query)

	expansions := parsed.ExpandedQueries
	if len(expansions) > 3 {
		expansions = expansions[:3]
	}

	// Phase 3: embed expansions
	vectors, err := s.Embedder.EmbedBatch(ctx, expansions)
	if err != nil {
		return nil, fmt.Errorf("embed expansions: %w", err)
	}

	// Phase 4: search with expansions
	topK := args.TopK
	if topK <= 0 {
		topK = 5
	}
	hits, err := s.searchWithExpansions(ctx, args.Query, expansions, vectors, topK)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// Phase 6: group expansion + assemble + filter + rerank
	if len(vectors) > 0 {
		hits = s.expandPhaseChains(ctx, hits, vectors[0])
	}
	hits = s.assembleGroups(hits)

	filters := Filters{Domain: args.Domain, Status: args.Status, Since: args.Since}
	hits = s.applyMetadataFilters(hits, filters)
	hits = policy.FilterByTime(hits, parsed.TimeScope, s.Now())
	hits = policy.ApplyRecencyWeighting(hits, s.Now())

	// Final topK
	if len(hits) > topK {
		hits = hits[:topK]
	}

	// Phase 7: build result
	return s.buildResult(hits), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 4 — search orchestration
// ─────────────────────────────────────────────────────────────────────────────

// searchWithExpansions — Python: searcher.py:L153-176 _search_with_expansions.
func (s *RecallService) searchWithExpansions(
	ctx context.Context,
	original string,
	exps []string,
	vectors [][]float32,
	topk int,
) ([]domain.SearchHit, error) {
	seen := make(map[string]bool)
	var allHits []domain.SearchHit

	for i, vec := range vectors {
		if i >= len(exps) {
			break
		}
		hits, err := s.searchSingle(ctx, vec, topk)
		if err != nil {
			slog.Warn("search expansion failed (best-effort)", "exp", exps[i], "err", err)
			continue
		}
		for _, h := range hits {
			if !seen[h.RecordID] {
				seen[h.RecordID] = true
				allHits = append(allHits, h)
			}
		}
	}

	// Original fallback
	originalInExps := false
	for _, e := range exps {
		if e == original {
			originalInExps = true
			break
		}
	}
	if !originalInExps && s.Embedder != nil {
		vec, err := s.Embedder.EmbedSingle(ctx, original)
		if err == nil {
			hits, err := s.searchSingle(ctx, vec, topk)
			if err == nil {
				for _, h := range hits {
					if !seen[h.RecordID] {
						seen[h.RecordID] = true
						allHits = append(allHits, h)
					}
				}
			}
		}
	}

	// Sort by raw score in descending order
	sort.SliceStable(allHits, func(i, j int) bool {
		return allHits[i].Score > allHits[j].Score
	})

	return allHits, nil
}

// searchSingle — Python: searcher.py:L371-373 + L375-470 _search_via_vault.
func (s *RecallService) searchSingle(ctx context.Context, vec []float32, topk int) ([]domain.SearchHit, error) {
	// Score
	blobs, err := s.Envector.Score(ctx, vec)
	if err != nil {
		return nil, fmt.Errorf("envector score: %w", err)
	}
	if len(blobs) == 0 {
		return nil, nil
	}

	// Vault decrypt scores. The Vault RPC field is `EncryptedBlobB64`
	// (proto3 `string`, valid-UTF-8 only) — envector returns raw cipher
	// bytes, so we must base64-encode before sending. A direct
	// `string(blobs[0])` cast pushes random cipher bytes through the
	// proto3 string-validation path and trips
	// "grpc: error while marshaling: string field contains invalid UTF-8".
	encryptedBlobB64 := base64.StdEncoding.EncodeToString(blobs[0])
	entries, err := s.Vault.DecryptScores(ctx, encryptedBlobB64, topk)
	if err != nil {
		return nil, fmt.Errorf("vault decrypt scores: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}

	// Get metadata
	refs := make([]envector.MetadataRef, len(entries))
	for i, e := range entries {
		refs[i] = envector.MetadataRef{ShardIdx: uint64(e.ShardIdx), RowIdx: uint64(e.RowIdx)}
	}
	metaEntries, err := s.Envector.GetMetadata(ctx, refs, []string{"metadata"})
	if err != nil {
		return nil, fmt.Errorf("envector get_metadata: %w", err)
	}

	// Search hit
	hits, err := s.resolveMetadata(ctx, metaEntries, entries)
	if err != nil {
		return nil, fmt.Errorf("resolve metadata: %w", err)
	}

	return hits, nil
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
func classifyMetadata(data string) (metadataFormat, map[string]any) {
	data = strings.TrimSpace(data)
	if data == "" {
		return fmtUnrecognized, nil
	}

	// Try JSON parse
	var parsed map[string]any
	if err := json.Unmarshal([]byte(data), &parsed); err == nil {
		// Check for AES envelope: {"a": ..., "c": ...}
		if _, hasA := parsed["a"]; hasA {
			if _, hasC := parsed["c"]; hasC {
				return fmtAESEnvelope, nil
			}
		}
		return fmtPlainJSON, parsed
	}

	// Try base64
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err == nil {
		var b64Parsed map[string]any
		if err := json.Unmarshal(decoded, &b64Parsed); err == nil {
			return fmtBase64JSON, b64Parsed
		}
	}

	return fmtUnrecognized, nil
}

// resolveMetadata — Python: searcher.py:L417-464 + _to_search_result.
func (s *RecallService) resolveMetadata(ctx context.Context, entries []envector.MetadataEntry, scores []vault.ScoreEntry) ([]domain.SearchHit, error) {
	type classifiedItem struct {
		idx    int
		fmt    metadataFormat
		parsed map[string]any
		raw    string
		score  float64
	}

	var aesIndices []int
	var aesList []string
	items := make([]classifiedItem, len(entries))

	for i, e := range entries {
		score := 0.0
		if i < len(scores) {
			score = scores[i].Score
		}
		f, parsed := classifyMetadata(e.Data)
		items[i] = classifiedItem{idx: i, fmt: f, parsed: parsed, raw: e.Data, score: score}

		if f == fmtAESEnvelope {
			aesIndices = append(aesIndices, i)
			aesList = append(aesList, e.Data)
		}
	}

	// Batch decrypt AES envelopes
	if len(aesList) > 0 && s.Vault != nil {
		decrypted, err := s.Vault.DecryptMetadata(ctx, aesList)
		if err != nil {
			slog.Warn("batch decrypt failed, trying per-entry", "err", err)
			for j, aesIdx := range aesIndices {
				single, singleErr := s.Vault.DecryptMetadata(ctx, []string{aesList[j]})
				if singleErr == nil && len(single) > 0 {
					var parsed map[string]any
					if json.Unmarshal([]byte(single[0]), &parsed) == nil {
						items[aesIdx].parsed = parsed
						items[aesIdx].fmt = fmtPlainJSON
					}
				}
			}
		} else {
			for j, aesIdx := range aesIndices {
				if j < len(decrypted) {
					var parsed map[string]any
					if json.Unmarshal([]byte(decrypted[j]), &parsed) == nil {
						items[aesIdx].parsed = parsed
						items[aesIdx].fmt = fmtPlainJSON
					}
				}
			}
		}
	}

	// Convert to SearchHits
	var hits []domain.SearchHit
	for _, item := range items {
		if item.parsed == nil {
			continue // unrecognized or  failed
		}
		hits = append(hits, toSearchHit(item.parsed, item.score))
	}

	return hits, nil
}

// toSearchHit — Python: searcher.py:L472-521 _to_search_result.
func toSearchHit(metadata map[string]any, score float64) domain.SearchHit {
	h := domain.SearchHit{
		RecordID:    strFromMap(metadata, "id", "unknown"),
		Title:       strFromMap(metadata, "title", "Untitled"),
		Domain:      strFromMap(metadata, "domain", "general"),
		Status:      strFromMap(metadata, "status", "unknown"),
		Score:       score,
		Metadata:    metadata,
		PayloadText: domain.ExtractPayloadText(metadata),
	}

	if why, ok := metadata["why"].(map[string]any); ok {
		h.Certainty = strFromMap(why, "certainty", "unknown")
	} else {
		h.Certainty = "unknown"
	}

	if ri, ok := metadata["reusable_insight"].(string); ok {
		h.ReusableInsight = ri
	}

	// Optional fileds
	if gid, ok := metadata["group_id"].(string); ok && gid != "" {
		h.GroupID = &gid
	}
	if gt, ok := metadata["group_type"].(string); ok && gt != "" {
		h.GroupType = &gt
	}
	if ps, ok := metadata["phase_seq"].(float64); ok {
		v := int(ps)
		h.PhaseSeq = &v
	}
	if pt, ok := metadata["phase_total"].(float64); ok {
		v := int(pt)
		h.PhaseTotal = &v
	}

	return h
}

func strFromMap(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 6 — group expansion, filters
// ─────────────────────────────────────────────────────────────────────────────

// expandPhaseChains — Python: searcher.py:L306-365 _expand_phase_chains.
func (s *RecallService) expandPhaseChains(ctx context.Context, results []domain.SearchHit, origVec []float32) []domain.SearchHit {
	type groupInfo struct {
		gid       string
		total     int
		present   int
		bestScore float64
	}

	groups := make(map[string]*groupInfo)
	for _, h := range results {
		if h.GroupID == nil {
			continue
		}
		gid := *h.GroupID
		g, ok := groups[gid]
		if !ok {
			total := 0
			if h.PhaseTotal != nil {
				total = *h.PhaseTotal
			}
			g = &groupInfo{gid: gid, total: total}
			groups[gid] = g
		}
		g.present++
		if h.Score > g.bestScore {
			g.bestScore = h.Score
		}
	}

	// Pick max 2 groups with missing siblings
	var incomplete []*groupInfo
	for _, g := range groups {
		if g.total > g.present {
			incomplete = append(incomplete, g)
		}
	}
	sort.Slice(incomplete, func(i, j int) bool {
		return incomplete[i].bestScore > incomplete[j].bestScore
	})
	if len(incomplete) > 2 {
		incomplete = incomplete[:2]
	}

	// Search again for missing siblings
	seen := make(map[string]bool)
	for _, h := range results {
		seen[h.RecordID] = true
	}

	for _, g := range incomplete {
		query := fmt.Sprintf("Group: %s", g.gid)
		vec, err := s.Embedder.EmbedSingle(ctx, query)
		if err != nil {
			continue
		}
		hits, err := s.searchSingle(ctx, vec, g.total)
		if err != nil {
			continue
		}
		for _, h := range hits {
			if !seen[h.RecordID] && h.GroupID != nil && *h.GroupID == g.gid {
				seen[h.RecordID] = true
				results = append(results, h)
			}
		}
	}

	return results
}

// assembleGroups — Python: searcher.py:L178-226 _assemble_groups.
func (s *RecallService) assembleGroups(results []domain.SearchHit) []domain.SearchHit {
	type group struct {
		hits      []domain.SearchHit
		bestScore float64
	}

	groups := make(map[string]*group)
	var standalone []domain.SearchHit

	for _, h := range results {
		if h.GroupID != nil {
			gid := *h.GroupID
			g, ok := groups[gid]
			if !ok {
				g = &group{}
				groups[gid] = g
			}
			g.hits = append(g.hits, h)
			if h.Score > g.bestScore {
				g.bestScore = h.Score
			}
		} else {
			standalone = append(standalone, h)
		}
	}

	type scoredGroup struct {
		gid   string
		g     *group
		score float64
	}

	var sorted []scoredGroup
	for gid, g := range groups {
		// Sort by phase_seq per group
		sort.SliceStable(g.hits, func(i, j int) bool {
			si, sj := 0, 0
			if g.hits[i].PhaseSeq != nil {
				si = *g.hits[i].PhaseSeq
			}
			if g.hits[j].PhaseSeq != nil {
				sj = *g.hits[j].PhaseSeq
			}
			return si < sj
		})

		sorted = append(sorted, scoredGroup{gid: gid, g: g, score: g.bestScore})
	}

	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	sort.SliceStable(standalone, func(i, j int) bool {
		return standalone[i].Score > standalone[j].Score
	})

	var assembled []domain.SearchHit
	gi, si := 0, 0
	for gi < len(sorted) || si < len(standalone) {
		groupScore := -1.0
		standScore := -1.0
		if gi < len(sorted) {
			groupScore = sorted[gi].score
		}
		if si < len(standalone) {
			standScore = standalone[si].Score
		}
		if groupScore >= standScore {
			assembled = append(assembled, sorted[gi].g.hits...)
			gi++
		} else {
			assembled = append(assembled, standalone[si])
			si++
		}
	}

	return assembled
}

// applyMetadataFilters — Python: searcher.py:L228-252 _apply_metadata_filters.
func (s *RecallService) applyMetadataFilters(results []domain.SearchHit, f Filters) []domain.SearchHit {
	var filtered []domain.SearchHit
	for _, h := range results {
		if f.Domain != nil && h.Domain != *f.Domain {
			continue
		}
		if f.Status != nil && h.Status != *f.Status {
			continue
		}
		if f.Since != nil {
			ts := ""
			if m := h.Metadata; m != nil {
				if t, ok := m["timestamp"].(string); ok {
					ts = t
				}
			}

			// Keep record without timestamp (port from Python version)
			if ts != "" && ts < *f.Since {
				continue
			}
		}
		filtered = append(filtered, h)
	}
	return filtered
}

// Filters — user-supplied filter args (subset of RecallArgs).
type Filters struct {
	Domain *string
	Status *string
	Since  *string // ISO date "YYYY-MM-DD"
}

// ─────────────────────────────────────────────────────────────────────────────
// Phase 7 — response build
// ─────────────────────────────────────────────────────────────────────────────

// buildResult — Python: server.py:L950-990 agent-delegated path.
func (s *RecallService) buildResult(results []domain.SearchHit) *domain.RecallResult {
	confidence := calculateConfidence(results)

	entries := make([]domain.RecallEntry, len(results))
	for i, h := range results {
		entry := domain.RecallEntry{
			RecordID:        h.RecordID,
			Title:           h.Title,
			Domain:          h.Domain,
			Certainty:       h.Certainty,
			Status:          h.Status,
			Score:           h.Score,
			AdjustedScore:   h.AdjustedScore,
			ReusableInsight: h.ReusableInsight,
			PayloadText:     h.PayloadText,
			GroupID:         h.GroupID,
			GroupType:       h.GroupType,
			PhaseSeq:        h.PhaseSeq,
			PhaseTotal:      h.PhaseTotal,
		}
		entries[i] = entry
	}

	sourceCount := 5
	if len(results) < sourceCount {
		sourceCount = len(results)
	}

	sources := make([]domain.RecallSource, sourceCount)
	for i := 0; i < sourceCount; i++ {
		sources[i] = domain.RecallSource{
			RecordID: results[i].RecordID,
			Title:    results[i].Title,
		}
	}

	return &domain.RecallResult{
		OK:          true,
		Found:       len(results),
		Results:     entries,
		Confidence:  confidence,
		Sources:     sources,
		Synthesized: false,
	}
}

// calculateConfidence — Python: server.py:L393-412.
// Top-5 weighted sum / 2.0 clamp 1.0 round 2 decimals.
func calculateConfidence(results []domain.SearchHit) float64 {
	if len(results) == 0 {
		return 0
	}

	certaintyWeights := map[string]float64{
		"supported":            1.0,
		"partially_supported":  0.6,
		"unknown":              0.3,
	}

	totalScore := 0.0
	limit := 5
	if len(results) < limit {
		limit = len(results)
	}

	for i := 0; i < limit; i++ {
		r := results[i]
		positionWeight := 1.0 / float64(i+1)
		certWeight := 0.3
		if w, ok := certaintyWeights[r.Certainty]; ok {
			certWeight = w
		}
		weight := positionWeight * certWeight * r.Score
		totalScore += weight
	}

	conf := math.Min(1.0, totalScore/2.0)

	return math.Round(conf*100) / 100
}
