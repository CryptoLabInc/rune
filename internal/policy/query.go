package policy

import "github.com/envector/rune-go/internal/domain"

// Query parsing — Python: agents/retriever/query_processor.py (437 LoC).
// Flow: docs/v04/spec/flows/recall.md Phase 2.
//
// Constants to port (line-by-line):
//   - INTENT_PATTERNS: 31 regex across 7 intents + GENERAL fallback (L70-116)
//   - TIME_PATTERNS: 16 regex across 4 scopes (L119-124)
//   - STOP_WORDS: 81 words (L127-137)
//   - tech patterns: 4 groups (L345-350)
//
// Multilingual path (_parse_multilingual L237-281) — NOT ported (D21).

// IntentRule — ordered list (insertion order matters; Go map is unordered).
type IntentRule struct {
	Intent   domain.QueryIntent
	Patterns []string // raw regex; compiled once at package init
}

// IntentRules — ordered. TODO: port 31 patterns from query_processor.py:L70-116.
var IntentRules = []IntentRule{
	// TODO: {IntentDecisionRationale, []string{...6 patterns...}},
	// TODO: {IntentFeatureHistory, []string{...5 patterns...}},
	// ... 7 intents, 31 patterns total
}

// TimeRules — ordered. TODO: port 16 patterns from query_processor.py:L119-124.
var TimeRules = []IntentRule{}

// StopWords — 81 words. TODO: port from query_processor.py:L127-137.
var StopWords = map[string]struct{}{
	// TODO: 81 entries (the, a, an, is, are, ... and, or, but, if, ...)
}

// TechPatterns — 4 regex groups. TODO: port from query_processor.py:L345-350.
var TechPatterns = []string{
	// TODO: PostgreSQL|MySQL|..., React|Vue|..., AWS|GCP|..., REST|GraphQL|...
}

// Parse — Python: query_processor.py _parse_english path only (D21).
//
// 6-stage pipeline (spec/flows/recall.md Phase 2):
//  1. cleanQuery: lowercase + whitespace collapse + trailing punct strip (? preserved)
//  2. detectIntent: ordered match against IntentRules → first hit (or GENERAL)
//  3. detectTimeScope: ordered match against TimeRules
//  4. extractEntities: 4-stage (quoted, capitalized i>0, tech patterns, dedup [:10])
//  5. extractKeywords: \w+ + stopword filter + len>2 + dedup [:15]
//  6. generateExpansions: intent variants + entity[:3]×2 + lowercase-key dedup [:5]
//
// TODO: implement each stage.
func Parse(q string) domain.ParsedQuery {
	// TODO: bit-identical to _parse_english (query_processor.py)
	return domain.ParsedQuery{
		Original:  q,
		Intent:    domain.QueryIntentGeneral,
		TimeScope: domain.TimeScopeAllTime,
	}
}
