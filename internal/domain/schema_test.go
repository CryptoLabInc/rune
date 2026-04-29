package domain_test

// Tests for DecisionRecord schema helpers — ParseDomain, GenerateRecordID,
// GenerateGroupID, EnsureEvidenceCertaintyConsistency, ValidateEvidenceCertainty,
// EmbeddingTextForRecord, plus enum + constant locks.
//
// Python canonical sources:
//
//	agents/scribe/record_builder.py:L621-655    — _parse_domain (with alias)
//	agents/common/schemas/decision_record.py:L215-242 — validate / ensure
//	agents/common/schemas/decision_record.py:L245-251 — generate_record_id
//	agents/common/schemas/decision_record.py:L254-259 — generate_group_id
//	agents/common/schemas/embedding.py:L21-30  — embedding_text_for_record
//
// Python test baseline: agents/tests/test_record_builder.py:L167-188 only —
// a single weakly-asserting case for ensure_evidence_certainty_consistency.
// Go establishes first-time real coverage for all helpers.

import (
	"slices"
	"strings"
	"testing"
	"time"

	// time/tzdata embeds IANA tzdata so LoadLocation works on minimal
	// containers (alpine, scratch, etc.) without /usr/share/zoneinfo.
	// Test-only import; production binaries opt in independently.
	_ "time/tzdata"

	"github.com/envector/rune-go/internal/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// ParseDomain
// ─────────────────────────────────────────────────────────────────────────────

// every concrete enum value parses to itself when passed verbatim.
// Locks the 19-entry domainList against silent removal/rename.
//
// NOTE: this test is silently tautological for paired constant swaps
// (e.g., if both DomainOps and DomainSecurity wire values were
// reciprocally swapped, the lookup would still self-resolve). The wire
// format is gated by TestDomainEnum_WireValues below, which uses string
// literals; this test gates the lookup mechanics, not the wire bytes.
func TestParseDomain_AllNineteenEnumsRoundTrip(t *testing.T) {
	cases := []struct {
		key  string
		want domain.Domain
	}{
		{"architecture", domain.DomainArchitecture},
		{"security", domain.DomainSecurity},
		{"product", domain.DomainProduct},
		{"exec", domain.DomainExec},
		{"ops", domain.DomainOps},
		{"design", domain.DomainDesign},
		{"data", domain.DomainData},
		{"hr", domain.DomainHR},
		{"marketing", domain.DomainMarketing},
		{"incident", domain.DomainIncident},
		{"debugging", domain.DomainDebugging},
		{"qa", domain.DomainQA},
		{"legal", domain.DomainLegal},
		{"finance", domain.DomainFinance},
		{"sales", domain.DomainSales},
		{"customer_success", domain.DomainCustomerSuccess},
		{"research", domain.DomainResearch},
		{"risk", domain.DomainRisk},
		{"general", domain.DomainGeneral},
	}
	if len(cases) != 19 {
		t.Fatalf("expected 19 enum cases, got %d — list drift", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			if got := domain.ParseDomain(tc.key); got != tc.want {
				t.Errorf("ParseDomain(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// uppercase / mixed-case input lowercases before substring match.
// Python parity: record_builder.py:L626 `domain_lower = domain_str.lower()`.
func TestParseDomain_CaseInsensitive(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Domain
	}{
		{"ARCHITECTURE", domain.DomainArchitecture},
		{"Security", domain.DomainSecurity},
		{"PrOdUcT", domain.DomainProduct},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := domain.ParseDomain(tc.in); got != tc.want {
				t.Errorf("ParseDomain(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// substring containment: "system_architecture", "ops-related" → match.
// Python parity: record_builder.py:L652 `if key in domain_lower`.
func TestParseDomain_SubstringMatch(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Domain
	}{
		{"system_architecture", domain.DomainArchitecture},
		{"the_security_team", domain.DomainSecurity},
		{"product strategy", domain.DomainProduct},
		{"hr_review", domain.DomainHR},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := domain.ParseDomain(tc.in); got != tc.want {
				t.Errorf("ParseDomain(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// empty / unknown / whitespace fall through to General (Python:
// `if not domain_str: return Domain.GENERAL` + post-loop default).
func TestParseDomain_EmptyAndUnknownFallToGeneral(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty_string", ""},
		{"unknown_keyword", "blockchain"},
		{"whitespace_only", "   "},
		{"random_phrase", "lorem ipsum dolor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.ParseDomain(tc.in); got != domain.DomainGeneral {
				t.Errorf("ParseDomain(%q) = %q, want %q", tc.in, got, domain.DomainGeneral)
			}
		})
	}
}

// iteration order: when input contains multiple keys, the FIRST entry in
// domainList wins. domainList order: architecture, security, product, exec,
// ops, design, data, hr, marketing, incident, debugging, qa, legal,
// finance, sales, customer_success, research, risk, general.
//
// "ops_security" contains both "ops" and "security"; security (index 1) is
// before ops (index 4) in the list, so security wins. This locks the
// iteration contract — silently reordering domainList would shift behavior.
func TestParseDomain_FirstEntryInDomainListWins(t *testing.T) {
	cases := []struct {
		in   string
		want domain.Domain
		why  string
	}{
		{"ops_security", domain.DomainSecurity,
			"security (index 1) before ops (index 4)"},
		{"data_design", domain.DomainDesign,
			"design (index 5) before data (index 6)"},
		{"general_architecture", domain.DomainArchitecture,
			"architecture (index 0) before general (index 18)"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := domain.ParseDomain(tc.in); got != tc.want {
				t.Errorf("ParseDomain(%q) = %q, want %q (%s)",
					tc.in, got, tc.want, tc.why)
			}
		})
	}
}

// **Python ↔ Go DIVERGENCE — Phase-A documented gap.**
//
// Python record_builder.py:L646 maps "customer_escalation" → CUSTOMER_SUCCESS
// as an explicit alias. Go's domainList has no such alias entry, so
// ParseDomain("customer_escalation") falls through to DomainGeneral.
//
// Locked at Go semantics here. If a future bit-identity audit insists on
// parity, add an alias entry to domainList in schema.go:
//
//	{"customer_escalation", DomainCustomerSuccess},
//
// placed BEFORE "customer_success" so the more specific match fires first.
// This test would then need updating to `DomainCustomerSuccess`.
//
// TODO(yg): Phase-A debt — match Python alias.
func TestParseDomain_CustomerEscalationDivergesFromPython(t *testing.T) {
	got := domain.ParseDomain("customer_escalation")
	if got != domain.DomainGeneral {
		t.Errorf("ParseDomain(\"customer_escalation\") = %q, want %q "+
			"(Go semantics lock; Python parity would yield CustomerSuccess)",
			got, domain.DomainGeneral)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GenerateRecordID
// ─────────────────────────────────────────────────────────────────────────────

// fixedTS — UTC anchor used across record-id tests so the date prefix is
// deterministic.
var fixedTS = time.Date(2026, 4, 29, 12, 34, 56, 0, time.UTC)

// 3-word lowercase-with-underscore slug. Python parity:
// decision_record.py:L249-250 — `words = title.lower().split()[:3]`
// then `_.join(...)`.
func TestGenerateRecordID_BasicThreeWordSlug(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainArchitecture, "Choose PostgreSQL Database")
	want := "dec_2026-04-29_architecture_choose_postgresql_database"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// more-than-3 words → cap at first 3. Python: `[:3]` on the split list.
func TestGenerateRecordID_CapsAtThreeWords(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainOps,
		"alpha beta gamma delta epsilon")
	want := "dec_2026-04-29_ops_alpha_beta_gamma"
	if got != want {
		t.Errorf("got %q, want %q (only first 3 words should appear)", got, want)
	}
}

// fewer than 3 words → use what's there. No padding.
func TestGenerateRecordID_FewerThanThreeWords(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainOps, "deploy")
	want := "dec_2026-04-29_ops_deploy"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// empty title → empty slug → trailing underscore. Locks the format
// contract; a "skip empty" refactor would rightly change the suffix.
func TestGenerateRecordID_EmptyTitleProducesTrailingUnderscore(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainOps, "")
	want := "dec_2026-04-29_ops_"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Korean title — every rune is `unicode.IsLetter`, so isPyIsalnum returns
// true for the whole word. Python `str.isalnum()` returns True for Korean
// letters too (verified empirically). Both langs produce a slug containing
// the lowercased Korean text.
func TestGenerateRecordID_KoreanTitleSurvives(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainExec, "결정 사항 검토")
	want := "dec_2026-04-29_exec_결정_사항_검토"
	if got != want {
		t.Errorf("got %q, want %q (Korean letters are isalnum=true)", got, want)
	}
}

// punctuation drops the entire WORD (not character). Python:
// `_.join(w for w in words if w.isalnum() or w.replace("_", "").isalnum())`.
//
// Example from onboarding.md: `"Add email@foo.com support"` →
// words = ["add", "email@foo.com", "support"] → "email@foo.com" fails
// both `isalnum()` and `replace("_","").isalnum()` (the `@` and `.`) →
// dropped → slug = "add_support". This is a SUBTLE contract: char-level
// filter would give "add_emailfoocom_support" which is wrong.
func TestGenerateRecordID_PunctuationDropsEntireWord(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainOps,
		"Add email@foo.com support")
	want := "dec_2026-04-29_ops_add_support"
	if got != want {
		t.Errorf("got %q, want %q (word with @ and . must be dropped whole)",
			got, want)
	}
}

// underscore-containing word kept via the second branch:
// `or w.replace("_", "").isalnum()`. Without that branch, "my_var" would
// be dropped because `"my_var".isalnum()` is False (underscore counts).
func TestGenerateRecordID_UnderscoreVariantIsKept(t *testing.T) {
	// "my_var name field" → all 3 words alnum after underscore strip →
	// kept verbatim (underscores preserved in the word, then joined by '_').
	got := domain.GenerateRecordID(fixedTS, domain.DomainData,
		"my_var name field")
	want := "dec_2026-04-29_data_my_var_name_field"
	if got != want {
		t.Errorf("got %q, want %q (underscore-only word kept via "+
			"replace+isalnum branch)", got, want)
	}
}

// all-underscores word — `"___".replace("_","")` = `""`, and
// `"".isalnum()` is False (Python str.isalnum returns False for empty
// strings). Both Go (isPyIsalnum returns false on empty) and Python
// drop the word. Locks the empty-after-strip path that the underscore
// variant branch could otherwise admit.
func TestGenerateRecordID_AllUnderscoresWordIsDropped(t *testing.T) {
	got := domain.GenerateRecordID(fixedTS, domain.DomainOps, "ok ___ done")
	want := "dec_2026-04-29_ops_ok_done"
	if got != want {
		t.Errorf("got %q, want %q (all-underscores word: empty after "+
			"strip → isalnum false → dropped)", got, want)
	}
}

// **Python ↔ Go DIVERGENCE — Phase-A documented gap.**
//
// Python decision_record.py:L247 uses `timestamp.strftime("%Y-%m-%d")`
// which formats LOCAL components — no TZ conversion. Go's
// schema.go:L266 explicitly calls `ts.UTC().Format(...)` first,
// normalizing to UTC.
//
// For TZ-aware timestamps with non-UTC location, the two diverge:
//
//	Python: 2026-04-29 09:00 KST → "2026-04-29" (LOCAL)
//	Go:     2026-04-29 09:00 KST → "2026-04-29" (UTC happens to match)
//	Python: 2026-04-29 00:30 KST → "2026-04-29" (LOCAL)
//	Go:     2026-04-29 00:30 KST → "2026-04-28" (UTC shift)
//
// Locked at Go semantics here. If a future bit-identity audit insists
// on parity, drop the .UTC() call in schema.go — but consider the
// production impact: rune-mcp on a non-UTC machine would otherwise
// generate IDs whose date prefix depends on the operator's TZ, which
// is ambiguous for cross-team recall.
//
// TODO(yg): Phase-A debt — align with Python or commit to Go's UTC
// normalization team-wide and update the python side instead.
func TestGenerateRecordID_NonUTCTimestampNormalizedToUTC_DivergesFromPython(t *testing.T) {
	// 2026-04-29 09:00 KST is 2026-04-29 00:00 UTC (KST = UTC+9).
	// On the boundary, both langs happen to produce the same date —
	// see _UTCBoundaryShiftsDate for the case where they diverge.
	kst, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	ts := time.Date(2026, 4, 29, 9, 0, 0, 0, kst)
	got := domain.GenerateRecordID(ts, domain.DomainOps, "test")
	want := "dec_2026-04-29_ops_test"
	if got != want {
		t.Errorf("got %q, want %q (Go UTC normalization; Python LOCAL would also yield 04-29 here)",
			got, want)
	}
}

// 2026-04-29 00:30 KST = 2026-04-28 15:30 UTC. Date prefix shifts back
// one day in Go (UTC); Python would keep "2026-04-29" (LOCAL).
// See divergence note on TestGenerateRecordID_NonUTCTimestampNormalizedToUTC_DivergesFromPython.
func TestGenerateRecordID_UTCBoundaryShiftsDate_DivergesFromPython(t *testing.T) {
	kst, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	ts := time.Date(2026, 4, 29, 0, 30, 0, 0, kst)
	got := domain.GenerateRecordID(ts, domain.DomainOps, "test")
	want := "dec_2026-04-28_ops_test"
	if got != want {
		t.Errorf("got %q, want %q (Go UTC shift; Python LOCAL would yield dec_2026-04-29_ops_test)",
			got, want)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GenerateGroupID
// ─────────────────────────────────────────────────────────────────────────────

// shares slug logic but uses "grp_" prefix. Python parity:
// decision_record.py:L257-259 — same body as generate_record_id but
// prefix replaced.
func TestGenerateGroupID_SharesSlugWithGrpPrefix(t *testing.T) {
	rec := domain.GenerateRecordID(fixedTS, domain.DomainOps, "deploy v1")
	grp := domain.GenerateGroupID(fixedTS, domain.DomainOps, "deploy v1")

	if !strings.HasPrefix(rec, "dec_") {
		t.Errorf("record id should start with 'dec_', got %q", rec)
	}
	if !strings.HasPrefix(grp, "grp_") {
		t.Errorf("group id should start with 'grp_', got %q", grp)
	}
	// After the prefix, the suffix must be identical.
	if strings.TrimPrefix(rec, "dec_") != strings.TrimPrefix(grp, "grp_") {
		t.Errorf("rec/grp suffixes diverge: rec=%q, grp=%q", rec, grp)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EnsureEvidenceCertaintyConsistency
// ─────────────────────────────────────────────────────────────────────────────

// helper — build a record with mostly default fields, only the dimensions
// we care about (evidence, certainty, status) varied.
func mkRecord(evidence []domain.Evidence, certainty domain.Certainty, status domain.Status) *domain.DecisionRecord {
	return &domain.DecisionRecord{
		Evidence: evidence,
		Why:      domain.Why{Certainty: certainty},
		Status:   status,
	}
}

// no quotes + Supported → downgrade to Unknown + append marker.
// Python: decision_record.py:L233-237.
func TestEnsureEvidenceCertaintyConsistency_NoQuoteSupportedDowngradesToUnknown(t *testing.T) {
	r := mkRecord(
		[]domain.Evidence{{Claim: "we said it", Quote: ""}},
		domain.CertaintySupported,
		domain.StatusProposed,
	)
	domain.EnsureEvidenceCertaintyConsistency(r)

	if r.Why.Certainty != domain.CertaintyUnknown {
		t.Errorf("certainty = %q, want %q (downgrade)",
			r.Why.Certainty, domain.CertaintyUnknown)
	}
	const marker = "No direct quotes found in evidence"
	if !slices.Contains(r.Why.MissingInfo, marker) {
		t.Errorf("MissingInfo missing marker %q, got %v", marker, r.Why.MissingInfo)
	}
}

// marker NOT duplicated when the function runs twice (or when the marker
// already exists). Python: decision_record.py:L236 `if marker not in
// missing_info: append`.
func TestEnsureEvidenceCertaintyConsistency_MarkerNotDuplicated(t *testing.T) {
	const marker = "No direct quotes found in evidence"
	r := mkRecord(
		nil,
		domain.CertaintySupported,
		domain.StatusProposed,
	)
	r.Why.MissingInfo = []string{marker} // already present

	domain.EnsureEvidenceCertaintyConsistency(r)

	count := 0
	for _, m := range r.Why.MissingInfo {
		if m == marker {
			count++
		}
	}
	if count != 1 {
		t.Errorf("marker count = %d, want 1 (no duplication)", count)
	}
}

// no evidence at all + Accepted → status Proposed. Python:
// decision_record.py:L240-242.
func TestEnsureEvidenceCertaintyConsistency_NoEvidenceAcceptedDowngradesToProposed(t *testing.T) {
	r := mkRecord(nil, domain.CertaintyUnknown, domain.StatusAccepted)
	domain.EnsureEvidenceCertaintyConsistency(r)

	if r.Status != domain.StatusProposed {
		t.Errorf("status = %q, want %q", r.Status, domain.StatusProposed)
	}
}

// has-quote evidence + Supported → no change (invariant satisfied).
// Also gates that no spurious marker is appended to MissingInfo on the
// happy path — a regression that flipped the !hasQuotes guard would
// otherwise pass cert/status checks because the upstream `before == after`
// comparison happens before the marker append.
func TestEnsureEvidenceCertaintyConsistency_HasQuotePreservesSupported(t *testing.T) {
	r := mkRecord(
		[]domain.Evidence{{Claim: "X", Quote: "we will pick X"}},
		domain.CertaintySupported,
		domain.StatusAccepted,
	)
	before := r.Why.Certainty
	beforeMissingLen := len(r.Why.MissingInfo)
	domain.EnsureEvidenceCertaintyConsistency(r)
	if r.Why.Certainty != before {
		t.Errorf("certainty changed from %q to %q despite quote present",
			before, r.Why.Certainty)
	}
	if r.Status != domain.StatusAccepted {
		t.Errorf("status changed from accepted; evidence WITH quote should preserve it")
	}
	if len(r.Why.MissingInfo) != beforeMissingLen {
		t.Errorf("MissingInfo grew unexpectedly: before len %d, after %v",
			beforeMissingLen, r.Why.MissingInfo)
	}
}

// PartiallySupported with no quotes → no downgrade. Only Supported is
// downgraded. Python: decision_record.py:L234 `if certainty == SUPPORTED`.
func TestEnsureEvidenceCertaintyConsistency_PartiallySupportedWithoutQuoteUnchanged(t *testing.T) {
	r := mkRecord(
		[]domain.Evidence{{Claim: "X", Quote: ""}},
		domain.CertaintyPartiallySupported,
		domain.StatusProposed,
	)
	domain.EnsureEvidenceCertaintyConsistency(r)

	if r.Why.Certainty != domain.CertaintyPartiallySupported {
		t.Errorf("partially_supported should NOT downgrade; got %q",
			r.Why.Certainty)
	}
	// And no marker should be appended either (marker is only added when
	// the supported-downgrade fires).
	const marker = "No direct quotes found in evidence"
	if slices.Contains(r.Why.MissingInfo, marker) {
		t.Errorf("marker added for non-supported certainty: %v", r.Why.MissingInfo)
	}
}

// both downgrades fire when conditions overlap (no quotes + supported +
// no evidence + accepted).
func TestEnsureEvidenceCertaintyConsistency_BothDowngradesFireIndependently(t *testing.T) {
	r := mkRecord(nil, domain.CertaintySupported, domain.StatusAccepted)
	domain.EnsureEvidenceCertaintyConsistency(r)

	if r.Why.Certainty != domain.CertaintyUnknown {
		t.Errorf("certainty not downgraded: got %q", r.Why.Certainty)
	}
	if r.Status != domain.StatusProposed {
		t.Errorf("status not downgraded: got %q", r.Status)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ValidateEvidenceCertainty (read-only)
// ─────────────────────────────────────────────────────────────────────────────

func TestValidateEvidenceCertainty(t *testing.T) {
	cases := []struct {
		name      string
		evidence  []domain.Evidence
		certainty domain.Certainty
		want      bool
	}{
		{"supported_with_quote_valid",
			[]domain.Evidence{{Quote: "we said it"}}, domain.CertaintySupported, true},
		{"supported_no_quote_invalid",
			[]domain.Evidence{{Claim: "x", Quote: ""}}, domain.CertaintySupported, false},
		{"supported_no_evidence_at_all_invalid",
			nil, domain.CertaintySupported, false},
		{"partially_supported_no_quote_valid",
			[]domain.Evidence{{Claim: "x", Quote: ""}}, domain.CertaintyPartiallySupported, true},
		{"unknown_no_quote_valid",
			nil, domain.CertaintyUnknown, true},
		{"supported_with_one_quoted_one_unquoted_valid",
			[]domain.Evidence{{Claim: "x"}, {Quote: "y"}}, domain.CertaintySupported, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := mkRecord(tc.evidence, tc.certainty, domain.StatusProposed)
			if got := domain.ValidateEvidenceCertainty(r); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ValidateEvidenceCertainty is read-only: must NOT mutate the record.
// Counterpoint: EnsureEvidenceCertaintyConsistency mutates.
//
// Snapshots ALL mutable fields that Ensure could touch (certainty,
// status, missing_info, evidence) so a regression that calls Ensure's
// logic from Validate would surface here.
func TestValidateEvidenceCertainty_DoesNotMutate(t *testing.T) {
	r := mkRecord(
		[]domain.Evidence{{Claim: "x", Quote: ""}},
		domain.CertaintySupported, domain.StatusAccepted,
	)
	// Snapshot every mutable surface.
	beforeCert := r.Why.Certainty
	beforeStatus := r.Status
	beforeMissingLen := len(r.Why.MissingInfo)
	beforeEvidenceLen := len(r.Evidence)

	_ = domain.ValidateEvidenceCertainty(r)

	if r.Why.Certainty != beforeCert {
		t.Errorf("Validate mutated certainty: %q → %q", beforeCert, r.Why.Certainty)
	}
	if r.Status != beforeStatus {
		t.Errorf("Validate mutated status: %q → %q", beforeStatus, r.Status)
	}
	if len(r.Why.MissingInfo) != beforeMissingLen {
		t.Errorf("Validate mutated MissingInfo: len %d → %d",
			beforeMissingLen, len(r.Why.MissingInfo))
	}
	if len(r.Evidence) != beforeEvidenceLen {
		t.Errorf("Validate mutated Evidence: len %d → %d",
			beforeEvidenceLen, len(r.Evidence))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EmbeddingTextForRecord
// ─────────────────────────────────────────────────────────────────────────────

func TestEmbeddingTextForRecord(t *testing.T) {
	cases := []struct {
		name    string
		insight string
		payload string
		want    string
	}{
		{"insight_wins", "the gist", "fallback markdown", "the gist"},
		{"insight_trimmed", "  trimmed  ", "fallback", "trimmed"},
		{"empty_insight_falls_back_to_payload", "", "fallback markdown", "fallback markdown"},
		{"whitespace_only_insight_falls_back", "   \n  \t ", "fallback markdown", "fallback markdown"},
		{"both_empty_returns_empty", "", "", ""},
		// Payload fallback returns the raw text — no TrimSpace applied.
		// Python embedding.py:L30 returns `record.payload.text` directly.
		// A "tidy up" refactor that trimmed both paths would silently
		// change embedding vectors; this case gates it.
		{"payload_whitespace_preserved_on_fallback", "", "  raw payload  ", "  raw payload  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &domain.DecisionRecord{
				ReusableInsight: tc.insight,
				Payload:         domain.Payload{Format: "markdown", Text: tc.payload},
			}
			if got := domain.EmbeddingTextForRecord(r); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Constants + enum wire values
// ─────────────────────────────────────────────────────────────────────────────

// constants lock — D3 / D-spec values. Silent change shifts capture
// behavior across the entire pipeline.
func TestSchemaConstants_LockedToSpecValues(t *testing.T) {
	if got := domain.MaxTitleLen; got != 60 {
		t.Errorf("MaxTitleLen = %d, want 60 (D3)", got)
	}
	if got := domain.MaxPhases; got != 7 {
		t.Errorf("MaxPhases = %d, want 7 (Python llm_extractor.py:L329)", got)
	}
	if got := domain.MaxBundleFacets; got != 5 {
		t.Errorf("MaxBundleFacets = %d, want 5 (Python llm_extractor.py:L388)", got)
	}
}

// Domain enum wire values — these strings appear in DecisionRecord JSON
// and capture_log.jsonl. Locking them as a paired-swap-resistant table.
func TestDomainEnum_WireValues(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"architecture", string(domain.DomainArchitecture), "architecture"},
		{"security", string(domain.DomainSecurity), "security"},
		{"product", string(domain.DomainProduct), "product"},
		{"exec", string(domain.DomainExec), "exec"},
		{"ops", string(domain.DomainOps), "ops"},
		{"design", string(domain.DomainDesign), "design"},
		{"data", string(domain.DomainData), "data"},
		{"hr", string(domain.DomainHR), "hr"},
		{"marketing", string(domain.DomainMarketing), "marketing"},
		{"incident", string(domain.DomainIncident), "incident"},
		{"debugging", string(domain.DomainDebugging), "debugging"},
		{"qa", string(domain.DomainQA), "qa"},
		{"legal", string(domain.DomainLegal), "legal"},
		{"finance", string(domain.DomainFinance), "finance"},
		{"sales", string(domain.DomainSales), "sales"},
		{"customer_success", string(domain.DomainCustomerSuccess), "customer_success"},
		{"research", string(domain.DomainResearch), "research"},
		{"risk", string(domain.DomainRisk), "risk"},
		{"general", string(domain.DomainGeneral), "general"},
	}
	if len(cases) != 19 {
		t.Fatalf("expected 19 enum cases, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("Domain enum %q: got %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// Status enum wire values — used by recall rerank STATUS_MULTIPLIER lookup.
func TestStatusEnum_WireValues(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"proposed", string(domain.StatusProposed), "proposed"},
		{"accepted", string(domain.StatusAccepted), "accepted"},
		{"superseded", string(domain.StatusSuperseded), "superseded"},
		{"reverted", string(domain.StatusReverted), "reverted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("Status enum %q: got %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// Certainty enum wire values — referenced by validate / ensure invariants.
func TestCertaintyEnum_WireValues(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"supported", string(domain.CertaintySupported), "supported"},
		{"partially_supported", string(domain.CertaintyPartiallySupported), "partially_supported"},
		{"unknown", string(domain.CertaintyUnknown), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("Certainty enum %q: got %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}
