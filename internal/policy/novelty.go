// Package policy holds pure functions — novelty classification, rerank formula,
// query parsing, record_builder, payload_text rendering, PII redaction.
// No I/O, no external deps. Each file has a Python canonical reference.
package policy

import (
	"math"

	"github.com/envector/rune-go/internal/domain"
)

// NoveltyThresholds — runtime defaults per D11 (Python server.py:L102-104).
// Module constants in embedding.py (0.4/0.7/0.93) are dead defaults — server.py
// passes these values explicitly at call site.
type NoveltyThresholds struct {
	Novel   float64
	Related float64
	NearDup float64
}

// DefaultNoveltyThresholds — runtime values (D11).
var DefaultNoveltyThresholds = NoveltyThresholds{
	Novel:   0.3,
	Related: 0.7,
	NearDup: 0.95,
}

// ClassifyNovelty — Python: agents/common/schemas/embedding.py:L33-56.
//
// Returns (class, score) where score = round(1.0 - maxSimilarity, 4)
// (inverted — higher score means more novel).
//
//   - similarity <  0.3  → novel
//   - 0.3 ≤ sim <  0.7   → evolution
//   - 0.7 ≤ sim <  0.95  → related
//   - sim ≥ 0.95         → near_duplicate (capture blocked)
func ClassifyNovelty(maxSimilarity float64, th NoveltyThresholds) (domain.NoveltyClass, float64) {
	noveltyScore := 1.0 - maxSimilarity
	score := math.Round(noveltyScore*10000) / 10000

	if maxSimilarity >= th.NearDup {
		return domain.NoveltyClassNearDuplicate, score
	} else if maxSimilarity >= th.Related {
		return domain.NoveltyClassRelated, score
	} else if maxSimilarity >= th.Novel {
		return domain.NoveltyClassEvolution, score
	}

	return domain.NoveltyClassNovel, score
}
