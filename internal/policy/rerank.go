package policy

import (
	"time"

	"github.com/envector/rune-go/internal/domain"
)

// Rerank constants — Python: agents/retriever/searcher.py:L31-39.
// Formula: (SimilarityWeight × raw + RecencyWeight × decay) × statusMul
// Half-life decay: decay = 0.5 ^ (age_days / HalfLifeDays)
const (
	HalfLifeDays     = 90.0
	SimilarityWeight = 0.7
	RecencyWeight    = 0.3
)

// StatusMultiplier — Python: searcher.py:L36-39. Unknown status → 1.0.
var StatusMultiplier = map[string]float64{
	"accepted":   1.0,
	"proposed":   0.9,
	"superseded": 0.5,
	"reverted":   0.3,
}

// TimeRanges — Python: searcher.py:L532-535.
var TimeRanges = map[domain.TimeScope]time.Duration{
	domain.TimeScopeLastWeek:    7 * 24 * time.Hour,
	domain.TimeScopeLastMonth:   30 * 24 * time.Hour,
	domain.TimeScopeLastQuarter: 90 * 24 * time.Hour,
	domain.TimeScopeLastYear:    365 * 24 * time.Hour,
}

// ApplyRecencyWeighting — Python: searcher.py:L273-300 _apply_recency_weighting.
//
// For each hit, compute adjusted_score = (0.7 × raw + 0.3 × decay) × status_mul
// where decay = 0.5 ^ (age_days / 90) and age_days = math.Floor((now - ts).days).
// Sorts by adjusted_score descending (stable).
//
// BIT-IDENTICAL REQUIREMENT: Python timedelta.days is integer floor.
// Go Hours()/24 is float — must math.Floor to match.
//
// TODO: implement full loop + sort.
func ApplyRecencyWeighting(hits []domain.SearchHit, now time.Time) []domain.SearchHit {
	// TODO: bit-identical to searcher.py:L273-300
	return hits
}

// FilterByTime — Python: searcher.py:L523-559 _filter_by_time.
// TODO: implement. Records with no timestamp are kept (Python: if no ts, keep).
func FilterByTime(hits []domain.SearchHit, scope domain.TimeScope, now time.Time) []domain.SearchHit {
	// TODO: bit-identical to searcher.py:L523-559
	return hits
}
