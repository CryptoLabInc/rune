// Tests for policy.Parse — port of Python's TestQueryProcessor
// (agents/tests/test_retriever.py) plus thoroughness-extending cases for
// every QueryIntent and TimeScope, clean/cap invariants, and stop-word filter.
//
// Multilingual tests intentionally omitted: Go port has no Language field
// (D21 — agent pre-translates before invocation), so the regex/LLM split in
// the Python QueryProcessor does not exist here.
//
// Black-box style (package policy_test) — exercises only the public Parse
// entry point. Internals (cleanQuery, detectIntent, detectTimeScope,
// extractEntities, extractKeywords, generateExpansions) are gated through
// the resulting ParsedQuery fields.

package policy_test

import (
	"strings"
	"testing"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/policy"
)

// ─────────────────────────────────────────────────────────────────────────────
// Intent classification — covers all 7 explicit intents + GENERAL fallback.
// Python parity tests: test_parse_decision_rationale_query / feature_history /
// pattern_lookup / technical_context / general_query. The remaining 3 intents
// (security_compliance, historical_context, attribution) extend beyond the
// Python suite — same regex tables on both sides, so we gate them here too.
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_IntentClassification(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		intent domain.QueryIntent
	}{
		{"decision_rationale_choose", "Why did we choose PostgreSQL over MySQL?", domain.QueryIntentDecisionRationale},
		{"decision_rationale_reasoning", "What was the reasoning behind the migration?", domain.QueryIntentDecisionRationale},
		{"feature_history_customers", "Have customers asked for dark mode?", domain.QueryIntentFeatureHistory},
		{"pattern_lookup_handle", "How do we handle authentication?", domain.QueryIntentPatternLookup},
		{"technical_context_arch", "What's our architecture for the payment system?", domain.QueryIntentTechnicalContext},
		{"security_compliance_gdpr", "What are the GDPR compliance requirements?", domain.QueryIntentSecurityCompliance},
		{"historical_context_when", "When did we decide to migrate?", domain.QueryIntentHistoricalContext},
		{"attribution_who", "Who decided to use Redis?", domain.QueryIntentAttribution},
		{"general_fallback", "Tell me about our database", domain.QueryIntentGeneral},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := policy.Parse(tc.query).Intent
			if got != tc.intent {
				t.Errorf("Parse(%q).Intent = %q, want %q", tc.query, got, tc.intent)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Time scope detection — all 4 explicit scopes + ALL_TIME default.
// Python parity: test_time_scope_detection. Beyond Python: month/year and
// numeric Q[1-4] / 20\d{2} matchers.
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_TimeScope(t *testing.T) {
	cases := []struct {
		name  string
		query string
		scope domain.TimeScope
	}{
		{"last_week_phrase", "What decisions did we make last week?", domain.TimeScopeLastWeek},
		{"last_quarter_q3", "What happened in Q3?", domain.TimeScopeLastQuarter},
		{"last_month_phrase", "What did we decide last month?", domain.TimeScopeLastMonth},
		{"last_year_phrase", "What was decided last year?", domain.TimeScopeLastYear},
		{"last_year_numeric_2025", "Did we decide in 2025?", domain.TimeScopeLastYear},
		{"all_time_default", "Why PostgreSQL?", domain.TimeScopeAllTime},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := policy.Parse(tc.query).TimeScope
			if got != tc.scope {
				t.Errorf("Parse(%q).TimeScope = %q, want %q", tc.query, got, tc.scope)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Entity extraction — quoted strings, capitalized words, tech name patterns.
// Python parity: test_entity_extraction_quoted / _capitalized.
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_EntitiesQuoted(t *testing.T) {
	parsed := policy.Parse(`Why did we choose "React Native"?`)

	if !contains(parsed.Entities, "React Native") {
		t.Errorf("entities should contain 'React Native', got %v", parsed.Entities)
	}
}

func TestParse_EntitiesCapitalizedAndTechPatterns(t *testing.T) {
	parsed := policy.Parse("Why did we use PostgreSQL instead of MySQL?")

	// Either capitalized scan or tech-pattern matcher should surface both.
	// Strip trailing punctuation that strings.Fields keeps attached.
	entitiesLower := make(map[string]bool)
	for _, e := range parsed.Entities {
		entitiesLower[strings.ToLower(strings.TrimRight(e, "?.,!;:"))] = true
	}
	if !entitiesLower["postgresql"] && !entitiesLower["mysql"] {
		t.Errorf("entities should contain PostgreSQL or MySQL, got %v", parsed.Entities)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Keyword extraction — stop words filtered, words ≤ 2 chars filtered, dedup'd.
// Python parity: test_keyword_extraction.
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_KeywordsRetainsContentTerms(t *testing.T) {
	parsed := policy.Parse("Why did we choose PostgreSQL for the database?")

	if !contains(parsed.Keywords, "postgresql") && !contains(parsed.Keywords, "database") {
		t.Errorf("keywords should contain 'postgresql' or 'database', got %v", parsed.Keywords)
	}
}

func TestParse_KeywordsFiltersStopWords(t *testing.T) {
	parsed := policy.Parse("Why did we choose PostgreSQL for the database?")

	// Words that must be filtered: 3+ char stop words present in StopWords map.
	for _, w := range []string{"the", "did", "for", "why"} {
		if contains(parsed.Keywords, w) {
			t.Errorf("keywords should not contain stop word %q, got %v", w, parsed.Keywords)
		}
	}
	// Words that must be filtered by length (≤ 2 chars).
	for _, w := range []string{"we", "is", "of"} {
		if contains(parsed.Keywords, w) {
			t.Errorf("keywords should not contain short word %q, got %v", w, parsed.Keywords)
		}
	}
}

func TestParse_KeywordsDeduplicated(t *testing.T) {
	parsed := policy.Parse("PostgreSQL postgresql PostgreSQL database database")

	count := 0
	for _, k := range parsed.Keywords {
		if k == "postgresql" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("keyword 'postgresql' should appear once, got %d times: %v", count, parsed.Keywords)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Query expansion — original included, intent-based variants present,
// entity-based variants present, capped at 5.
// Python parity: test_query_expansion.
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_ExpansionContainsCleanedAndIntentVariants(t *testing.T) {
	parsed := policy.Parse("Why PostgreSQL?")

	if len(parsed.ExpandedQueries) <= 1 {
		t.Errorf("expanded_queries should have > 1 entry, got %d (%v)",
			len(parsed.ExpandedQueries), parsed.ExpandedQueries)
	}

	foundPostgres := false
	for _, q := range parsed.ExpandedQueries {
		if strings.Contains(strings.ToLower(q), "postgresql") {
			foundPostgres = true
			break
		}
	}
	if !foundPostgres {
		t.Errorf("expanded_queries should reference 'postgresql', got %v", parsed.ExpandedQueries)
	}
}

func TestParse_ExpansionCappedAtFive(t *testing.T) {
	// Long input with multiple entities + decision_rationale intent should
	// produce > 5 raw expansions; output must clamp.
	parsed := policy.Parse(`Why did we choose "PostgreSQL" over "MySQL" and "MongoDB" and "Redis"?`)

	if len(parsed.ExpandedQueries) > 5 {
		t.Errorf("expanded_queries should be capped at 5, got %d (%v)",
			len(parsed.ExpandedQueries), parsed.ExpandedQueries)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Cleaning — lowercase, whitespace collapse, leading/trailing trim,
// trailing-punctuation strip (but keep ?).
// Beyond Python: explicit table for each transformation.
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_Cleaned(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase", "Why PostgreSQL?", "why postgresql?"},
		{"whitespace_collapse", "Why  PostgreSQL?", "why postgresql?"},
		{"trim_leading_trailing", "  Why PostgreSQL?  ", "why postgresql?"},
		{"strip_trailing_period", "Why PostgreSQL.", "why postgresql"},
		{"strip_trailing_exclaim", "Why PostgreSQL!", "why postgresql"},
		{"strip_trailing_comma", "Why PostgreSQL,", "why postgresql"},
		{"strip_trailing_colon", "Why PostgreSQL:", "why postgresql"},
		{"strip_trailing_semicolon", "Why PostgreSQL;", "why postgresql"},
		{"keep_question_mark", "Why PostgreSQL?", "why postgresql?"},
		{"strip_multiple_trailing", "Why PostgreSQL...", "why postgresql"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := policy.Parse(tc.in).Cleaned
			if got != tc.want {
				t.Errorf("Cleaned: got %q, want %q", got, tc.want)
			}
		})
	}
}

// Original is preserved verbatim (case + punctuation + whitespace).
func TestParse_OriginalPreserved(t *testing.T) {
	in := "Why did we choose PostgreSQL over MySQL?"
	got := policy.Parse(in).Original
	if got != in {
		t.Errorf("Original: got %q, want %q", got, in)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Output caps — entities ≤ 10, keywords ≤ 15, expansions ≤ 5.
// Spec: docs/v04/spec/types.md §5.2 ParsedQuery (matches Python defaults).
// ─────────────────────────────────────────────────────────────────────────────

func TestParse_FieldCaps(t *testing.T) {
	// Tech-heavy query that would naturally produce > caps without clamping.
	long := `Why did we choose "Alpha" "Beta" "Gamma" "Delta" "Epsilon" "Zeta" "Eta" "Theta" "Iota" "Kappa" "Lambda" "Mu" PostgreSQL MySQL MongoDB Redis Elasticsearch Kafka React Vue Angular Node Python Java AWS GCP Azure Kubernetes Docker Terraform REST GraphQL gRPC HTTP HTTPS over alternatives?`
	parsed := policy.Parse(long)

	if len(parsed.Entities) > 10 {
		t.Errorf("Entities cap: got %d, want <= 10", len(parsed.Entities))
	}
	if len(parsed.Keywords) > 15 {
		t.Errorf("Keywords cap: got %d, want <= 15", len(parsed.Keywords))
	}
	if len(parsed.ExpandedQueries) > 5 {
		t.Errorf("ExpandedQueries cap: got %d, want <= 5", len(parsed.ExpandedQueries))
	}
}

// helpers

func contains(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
