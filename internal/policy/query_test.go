package policy_test

// Tests for policy.Parse — port of Python's TestQueryProcessor
// (agents/tests/test_retriever.py:11-92) plus thoroughness-extending cases for
// every QueryIntent and TimeScope, regex precedence (rule iteration order),
// clean/cap invariants exact-match, and stop-word filter.
//
// Multilingual tests intentionally omitted: Go ParsedQuery has no Language
// field (D21 — agent pre-translates before invocation), so the regex/LLM
// split that exists in Python QueryProcessor does not exist here.
//
// Black-box style — exercises only the public Parse entry point. Internals
// (cleanQuery, detectIntent, detectTimeScope, extractEntities,
// extractKeywords, generateExpansions) are gated through the resulting
// ParsedQuery fields.

import (
	"slices"
	"strings"
	"testing"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/policy"
)

// Intent classification — covers all 7 explicit intents + GENERAL fallback.
// Python parity (test_retriever.py:19-53):
//
//	test_parse_decision_rationale_query / feature_history / pattern_lookup /
//	technical_context / general_query.
//
// Beyond Python: SECURITY_COMPLIANCE / HISTORICAL_CONTEXT / ATTRIBUTION
// share the regex tables on both sides, so we gate them too.
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

// Intent rule precedence — when a query hits multiple rules, the earliest
// rule in IntentRules iteration order wins. Order is: DecisionRationale (0),
// FeatureHistory (1), PatternLookup (2), TechnicalContext (3),
// SecurityCompliance (4), HistoricalContext (5), Attribution (6).
// Silent reordering would be undetected by TestParse_IntentClassification
// alone — this test gates the iteration contract.
func TestParse_IntentRulePrecedence(t *testing.T) {
	// Hits both DecisionRationale (`why did we decide`) AND
	// SecurityCompliance (`gdpr compliance`). DecisionRationale (rule 0)
	// must win.
	got := policy.Parse("Why did we decide on GDPR compliance?").Intent
	if got != domain.QueryIntentDecisionRationale {
		t.Errorf("rule precedence: DecisionRationale should win over SecurityCompliance, got %q", got)
	}
}

// Time scope detection — all 4 explicit scopes + ALL_TIME default + numeric
// boundaries. Python parity (test_retriever.py:55-62): test_time_scope_detection.
func TestParse_TimeScope(t *testing.T) {
	cases := []struct {
		name  string
		query string
		scope domain.TimeScope
	}{
		{"last_week_phrase", "What decisions did we make last week?", domain.TimeScopeLastWeek},
		{"this_week_phrase", "Any progress this week?", domain.TimeScopeLastWeek},
		{"last_month_phrase", "What did we decide last month?", domain.TimeScopeLastMonth},
		{"this_quarter_phrase", "What is the priority this quarter?", domain.TimeScopeLastQuarter},
		{"last_quarter_q3", "What happened in Q3?", domain.TimeScopeLastQuarter},
		{"past_3_months_phrase", "Decisions from past 3 months?", domain.TimeScopeLastQuarter},
		{"last_year_phrase", "What was decided last year?", domain.TimeScopeLastYear},
		// Numeric boundary: regex is `20\d{2}`, matches 2000-2099 only.
		{"last_year_numeric_2025", "Did we decide in 2025?", domain.TimeScopeLastYear},
		{"past_year_phrase", "Trends in the past year?", domain.TimeScopeLastYear},
		// Negative boundary: 1999 must NOT match `20\d{2}`.
		{"all_time_1999_does_not_match", "What was decided in 1999?", domain.TimeScopeAllTime},
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

// Time rule precedence — when a query hits multiple time patterns, the
// earliest rule in TimeRules iteration order wins. Order: LastWeek (0),
// LastMonth (1), LastQuarter (2), LastYear (3).
func TestParse_TimeRulePrecedence(t *testing.T) {
	// Hits LastWeek (`last week`) AND LastYear (`last year`).
	// LastWeek (rule 0) must win.
	got := policy.Parse("Compare last week to last year").TimeScope
	if got != domain.TimeScopeLastWeek {
		t.Errorf("rule precedence: LastWeek should win over LastYear, got %q", got)
	}
}

// Entity extraction — quoted strings preserved verbatim (incl. case & space).
// Python parity (test_retriever.py:64-67): test_entity_extraction_quoted.
func TestParse_EntitiesQuoted(t *testing.T) {
	parsed := policy.Parse(`Why did we choose "React Native"?`)

	if !slices.Contains(parsed.Entities, "React Native") {
		t.Errorf("entities should contain 'React Native' verbatim, got %v", parsed.Entities)
	}
}

// Entity extraction — capitalized scan + tech regex must each surface their
// targets. Tightened from the looser Python assertion (which used OR): both
// PostgreSQL AND MySQL must appear (case-insensitive after stripping
// trailing punctuation that strings.Fields keeps attached, e.g. "MySQL?").
// Python parity (test_retriever.py:69-74): test_entity_extraction_capitalized.
func TestParse_EntitiesCapitalizedAndTechPatterns(t *testing.T) {
	parsed := policy.Parse("Why did we use PostgreSQL instead of MySQL?")

	want := map[string]bool{"postgresql": false, "mysql": false}
	for _, e := range parsed.Entities {
		key := strings.ToLower(strings.TrimRight(e, "?.,!;:"))
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("entities should contain %q, got %v", k, parsed.Entities)
		}
	}
}

// Decision-rationale query surfaces the entity in entities OR the lowercased
// token in cleaned. Mirrors the disjunction Python asserts in
// test_retriever.py:25 — preserved here as a single test rather than split
// across files.
func TestParse_EntityOrCleanedSurfacesToken(t *testing.T) {
	parsed := policy.Parse("Why did we choose PostgreSQL over MySQL?")

	inEntities := false
	for _, e := range parsed.Entities {
		if e == "PostgreSQL" {
			inEntities = true
			break
		}
	}
	inCleaned := strings.Contains(parsed.Cleaned, "postgresql")
	if !inEntities && !inCleaned {
		t.Errorf("token must surface in entities (verbatim) or cleaned (lowercased); entities=%v cleaned=%q",
			parsed.Entities, parsed.Cleaned)
	}
}

// Keyword extraction — content terms retained, stop words and short words
// filtered, dedup'd post-lowercase. Python parity (test_retriever.py:76-79):
// test_keyword_extraction. Tightened from Python's OR to AND — both content
// terms must surface.
func TestParse_KeywordsRetainsContentTerms(t *testing.T) {
	parsed := policy.Parse("Why did we choose PostgreSQL for the database?")

	for _, w := range []string{"postgresql", "database", "choose"} {
		if !slices.Contains(parsed.Keywords, w) {
			t.Errorf("keywords should contain %q, got %v", w, parsed.Keywords)
		}
	}
}

func TestParse_KeywordsFiltersStopWords(t *testing.T) {
	parsed := policy.Parse("Why did we choose PostgreSQL for the database?")

	// 3+ char stop words present in StopWords map.
	for _, w := range []string{"the", "did", "for", "why"} {
		if slices.Contains(parsed.Keywords, w) {
			t.Errorf("keywords should not contain stop word %q, got %v", w, parsed.Keywords)
		}
	}
	// ≤ 2 chars are filtered by length regardless of stop-word membership.
	for _, w := range []string{"we", "is", "of"} {
		if slices.Contains(parsed.Keywords, w) {
			t.Errorf("keywords should not contain short word %q, got %v", w, parsed.Keywords)
		}
	}
}

// Cleaning happens before keyword extraction, so by the time dedup runs the
// strings are already lowercase. This test gates that property: even when
// the input has multiple casings, the output keyword list contains
// "postgresql" exactly once. Renamed from the earlier "Deduplicated" name —
// it does not test case-insensitive dedup at the extractor level (that
// happens upstream in cleanQuery), only post-lowercase dedup.
func TestParse_KeywordsDedupAfterLowercase(t *testing.T) {
	parsed := policy.Parse("PostgreSQL postgresql PostgreSQL database database")

	count := 0
	for _, k := range parsed.Keywords {
		if k == "postgresql" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("keyword 'postgresql' should appear exactly once, got %d times: %v",
			count, parsed.Keywords)
	}
}

// Query expansion — the cleaned query, intent variants, and entity-derived
// strings all appear. Python parity (test_retriever.py:81-86): test_query_expansion.
// Tightened: also asserts at least one DecisionRationale-prefix variant
// appears (would catch a regression that deletes the intent switch).
//
// Note on query choice: "Why did we choose PostgreSQL?" must match
// DecisionRationale (`why did we (choose|...)`) — a shorter query like
// "Why PostgreSQL?" falls through to GENERAL (no intent prefix matches),
// which exercises a different branch of generateExpansions.
func TestParse_ExpansionContainsCleanedAndIntentVariants(t *testing.T) {
	parsed := policy.Parse("Why did we choose PostgreSQL?")

	if len(parsed.ExpandedQueries) <= 1 {
		t.Fatalf("expanded_queries should have > 1 entry, got %d (%v)",
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

	// DecisionRationale prefixes are: "decision ", "rationale ", "trade-off ".
	// An empty switch (regression) would leave only [cleaned, entity-variants].
	gotIntentVariant := false
	for _, q := range parsed.ExpandedQueries {
		ql := strings.ToLower(q)
		if strings.HasPrefix(ql, "decision ") ||
			strings.HasPrefix(ql, "rationale ") ||
			strings.HasPrefix(ql, "trade-off ") {
			gotIntentVariant = true
			break
		}
	}
	if !gotIntentVariant {
		t.Errorf("expanded_queries should include a DecisionRationale-prefixed variant, got %v",
			parsed.ExpandedQueries)
	}
}

// GENERAL intent gets no intent-prefix variants — only [cleaned, entity-derived].
// "Why PostgreSQL?" fails every IntentRule and falls through to GENERAL.
// Documents the fall-through contract so a future change that adds GENERAL
// expansions has to update this test deliberately.
func TestParse_ExpansionGeneralIntentNoPrefixes(t *testing.T) {
	parsed := policy.Parse("Why PostgreSQL?")

	if got := parsed.Intent; got != domain.QueryIntentGeneral {
		t.Fatalf("precondition: query should classify as GENERAL, got %q", got)
	}
	for _, q := range parsed.ExpandedQueries {
		ql := strings.ToLower(q)
		if strings.HasPrefix(ql, "decision ") ||
			strings.HasPrefix(ql, "rationale ") ||
			strings.HasPrefix(ql, "trade-off ") ||
			strings.HasPrefix(ql, "customer request ") ||
			strings.HasPrefix(ql, "feature rejected ") ||
			strings.HasPrefix(ql, "standard approach ") ||
			strings.HasPrefix(ql, "best practice ") ||
			strings.HasPrefix(ql, "architecture ") ||
			strings.HasPrefix(ql, "implementation ") {
			t.Errorf("GENERAL intent should produce no intent-prefix variants, found %q in %v",
				q, parsed.ExpandedQueries)
		}
	}
}

// Expansion cap is exactly 5 when the input would naturally overflow. A
// regression to a smaller cap (e.g., 3) would be missed by checking only
// "≤ 5"; this test gates "= 5".
func TestParse_ExpansionCappedAtExactlyFive(t *testing.T) {
	// DecisionRationale intent yields 1 (cleaned) + 3 (intent variants) +
	// 2*N (entity-derived) candidates, easily > 5 with 4 quoted entities.
	parsed := policy.Parse(`Why did we choose "PostgreSQL" over "MySQL" and "MongoDB" and "Redis"?`)

	if len(parsed.ExpandedQueries) != 5 {
		t.Errorf("expanded_queries should be exactly 5 when input overflows, got %d (%v)",
			len(parsed.ExpandedQueries), parsed.ExpandedQueries)
	}
}

// Cleaning transformations: lowercase, whitespace collapse (incl. tabs and
// newlines), leading/trailing trim, trailing-punctuation strip (? preserved,
// .!,;: stripped including consecutive runs).
func TestParse_Cleaned(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase", "Why PostgreSQL?", "why postgresql?"},
		{"whitespace_space_collapse", "Why  PostgreSQL?", "why postgresql?"},
		{"whitespace_tab_collapse", "Why\tPostgreSQL?", "why postgresql?"},
		{"whitespace_newline_collapse", "Why\nPostgreSQL?", "why postgresql?"},
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

// Original is preserved verbatim — case, punctuation, whitespace.
func TestParse_OriginalPreserved(t *testing.T) {
	in := "Why did we choose PostgreSQL over MySQL?"
	got := policy.Parse(in).Original
	if got != in {
		t.Errorf("Original: got %q, want %q", got, in)
	}
}

// Output caps must clamp to exactly the documented bounds when input
// overflows. Spec: docs/v04/spec/types.md §5.2 ParsedQuery
// (entities ≤ 10, keywords ≤ 15, expansions ≤ 5).
func TestParse_FieldCaps(t *testing.T) {
	// Entity-flood input: 12 quoted strings (overflows cap by 2) plus
	// tech tokens that would also surface, plus enough content words to
	// overflow the keyword cap.
	long := buildOverflowQuery()
	parsed := policy.Parse(long)

	if got := len(parsed.Entities); got != 10 {
		t.Errorf("Entities cap: got %d, want exactly 10 when overflowing", got)
	}
	if got := len(parsed.Keywords); got != 15 {
		t.Errorf("Keywords cap: got %d, want exactly 15 when overflowing", got)
	}
	if got := len(parsed.ExpandedQueries); got != 5 {
		t.Errorf("ExpandedQueries cap: got %d, want exactly 5 when overflowing", got)
	}
}

func buildOverflowQuery() string {
	// 12 quoted entities (overflows entity cap by 2) plus tech tokens that
	// add more entity candidates, plus content words to overflow keyword cap.
	return `Why did we choose "Alpha" "Beta" "Gamma" "Delta" "Epsilon" "Zeta" "Eta" "Theta" "Iota" "Kappa" "Lambda" "Mu" PostgreSQL MySQL MongoDB Redis Elasticsearch Kafka React Vue Angular Node Python Java AWS GCP Azure Kubernetes Docker Terraform REST GraphQL gRPC HTTP HTTPS over alternatives?`
}
