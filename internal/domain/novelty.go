package domain

// Novelty classification.
// Spec: docs/v04/spec/types.md §5.4.
// Python: mcp/server/server.py:L100-108 _classify_novelty
//	+ agents/common/schemas/embedding.py:L33-56 classify_novelty.

// NoveltyClass — 4 values.
type NoveltyClass string

const (
	NoveltyClassNovel         NoveltyClass = "novel"
	NoveltyClassEvolution     NoveltyClass = "evolution"
	NoveltyClassRelated       NoveltyClass = "related"
	NoveltyClassNearDuplicate NoveltyClass = "near_duplicate"
)

// NoveltyInfo — §5.4.
// Score = round(1.0 - max_similarity, 4) — inverted (higher score = more novel).
type NoveltyInfo struct {
	Class   NoveltyClass    `json:"class"`
	Score   float64         `json:"score"`
	Related []RelatedRecord `json:"related"` // top-3 similar records
}

// RelatedRecord — appended by caller (server.py:L1353-1360).
type RelatedRecord struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Similarity float64 `json:"similarity"` // round 3
}

// Initial state (server.py:L1338): {Class: Novel, Score: 1.0, Related: []}.
// Used when no prior records exist (max novelty).
