package policy_test

// Tests for ApplyRecencyWeighting + FilterByTime + rerank constants.
//
// Python parity: agents/retriever/searcher.py:L273-300 (rerank) +
// L523-559 (filter). Python has NO test file for these — Go is
// establishing first-time coverage. Every case below either ports a
// behavioral assertion implicit in the Python code or gates a
// Go-specific contract (math.Floor age, type-switch on metadata
// timestamp, sort.SliceStable).
//
// Black-box style — exercises only the public surface. Internal
// arithmetic is gated through observable AdjustedScore values, computed
// by hand from the documented formula:
//
//	adjusted = (SimilarityWeight × raw + RecencyWeight × decay) × statusMul
//	decay    = 0.5 ^ (ageDays / HalfLifeDays)
//	ageDays  = max(0, floor((now - ts).hours / 24))

import (
	"math"
	"testing"
	"time"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/policy"
)

// fixedNow — a deterministic wall clock for all timestamp arithmetic.
// Chosen to be far past anchored UTC midnight so the only floating-point
// surprise comes from the formula itself, not from clock alignment.
var fixedNow = time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)

// hitWithTS builds a SearchHit whose metadata contains a "timestamp"
// field of the given concrete type. The two metadata types
// ApplyRecencyWeighting handles are:
//
//	string  — RFC3339 (Python parity: datetime.fromisoformat)
//	float64 — Unix seconds (Python parity: datetime.fromtimestamp)
//
// Anything else (int, nil, missing) leaves ageDays = 0 / decay = 1.0.
func hitWithTS(score float64, status string, tsValue any) domain.SearchHit {
	meta := map[string]any{}
	if tsValue != nil {
		meta["timestamp"] = tsValue
	}
	return domain.SearchHit{
		RecordID: "dec_test",
		Score:    score,
		Status:   status,
		Metadata: meta,
	}
}

// almostEqual — float comparison with epsilon tight enough to catch a
// single mis-multiplied weight while loose enough to absorb the natural
// drift of 0.5^(integer/90) compounded with 0.7/0.3 weights.
func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Constants are part of the public contract — silent change shifts every
// reranked recall result. Lock the bytes.
//
// HalfLifeDays / SimilarityWeight / RecencyWeight are defined in
// rerank.go:L14-18. Python: searcher.py:L31-33.
func TestRerankConstants_LockedToPythonValues(t *testing.T) {
	if got := policy.HalfLifeDays; got != 90.0 {
		t.Errorf("HalfLifeDays = %v, want 90.0 (Python searcher.py:L31)", got)
	}
	if got := policy.SimilarityWeight; got != 0.7 {
		t.Errorf("SimilarityWeight = %v, want 0.7 (Python searcher.py:L32)", got)
	}
	if got := policy.RecencyWeight; got != 0.3 {
		t.Errorf("RecencyWeight = %v, want 0.3 (Python searcher.py:L33)", got)
	}
}

// StatusMultiplier — wire-significant: capture/recall behavior depends
// on these exact multipliers. Lock all 4 keys + values. Python:
// searcher.py:L36-39.
func TestStatusMultiplier_AllFourEntriesLocked(t *testing.T) {
	want := map[string]float64{
		"accepted":   1.0,
		"proposed":   0.9,
		"superseded": 0.5,
		"reverted":   0.3,
	}
	if got := len(policy.StatusMultiplier); got != len(want) {
		t.Fatalf("StatusMultiplier has %d entries, want %d (silent addition/removal)",
			got, len(want))
	}
	for status, mult := range want {
		got, ok := policy.StatusMultiplier[status]
		if !ok {
			t.Errorf("StatusMultiplier[%q] missing", status)
			continue
		}
		if got != mult {
			t.Errorf("StatusMultiplier[%q] = %v, want %v", status, got, mult)
		}
	}
}

// TimeRanges — gate every TimeScope key + duration. Python:
// searcher.py:L532-535. AllTime is intentionally absent (filter
// short-circuits via the missing-key path).
func TestTimeRanges_AllFourEntriesLocked(t *testing.T) {
	want := map[domain.TimeScope]time.Duration{
		domain.TimeScopeLastWeek:    7 * 24 * time.Hour,
		domain.TimeScopeLastMonth:   30 * 24 * time.Hour,
		domain.TimeScopeLastQuarter: 90 * 24 * time.Hour,
		domain.TimeScopeLastYear:    365 * 24 * time.Hour,
	}
	if got := len(policy.TimeRanges); got != len(want) {
		t.Fatalf("TimeRanges has %d entries, want %d", got, len(want))
	}
	for scope, dur := range want {
		got, ok := policy.TimeRanges[scope]
		if !ok {
			t.Errorf("TimeRanges[%v] missing", scope)
			continue
		}
		if got != dur {
			t.Errorf("TimeRanges[%v] = %v, want %v", scope, got, dur)
		}
	}
	// AllTime is the implicit default — not in the map, filter falls
	// through to "return all". Lock that contract negatively too.
	if _, ok := policy.TimeRanges[domain.TimeScopeAllTime]; ok {
		t.Errorf("TimeRanges[AllTime] must NOT be present (filter relies on absence)")
	}
}

// ApplyRecencyWeighting — empty input must not panic and must return an
// empty (or nil) slice. Cheap but it's the only test that gates the
// guard against `range hits` on nil.
func TestApplyRecencyWeighting_EmptyInputDoesNotPanic(t *testing.T) {
	got := policy.ApplyRecencyWeighting(nil, fixedNow)
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
	got = policy.ApplyRecencyWeighting([]domain.SearchHit{}, fixedNow)
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// ApplyRecencyWeighting — formula gate. Each subtest pins one moving
// part: the timestamp parser branch (string vs float64 vs missing vs
// invalid), the half-life decay, and the math.Floor age semantics.
//
// Hand-computed expected values are inlined as comments so a reviewer
// can verify the arithmetic without re-running the formula.
func TestApplyRecencyWeighting_FormulaGate(t *testing.T) {
	cases := []struct {
		name     string
		hit      domain.SearchHit
		wantAdj  float64
		wantNote string
	}{
		{
			name: "no_timestamp_treats_age_as_zero",
			hit: domain.SearchHit{
				Score: 1.0, Status: "accepted",
				Metadata: map[string]any{}, // no "timestamp" key
			},
			// decay = 0.5^0 = 1.0
			// adj = (0.7×1.0 + 0.3×1.0) × 1.0 = 1.0
			wantAdj:  1.0,
			wantNote: "missing-ts path: ageDays = 0",
		},
		{
			name: "nil_metadata_treats_age_as_zero",
			hit: domain.SearchHit{
				Score: 1.0, Status: "accepted",
				Metadata: nil,
			},
			// Reading from a nil map in Go yields zero value, ok=false —
			// same path as missing key. This case locks the contract
			// against a future "metadata = make(map) before read"
			// refactor that would silently change nil-input semantics.
			wantAdj:  1.0,
			wantNote: "nil-metadata path: zero-value read, ageDays = 0",
		},
		{
			name: "rfc3339_same_day_decay_one",
			hit: hitWithTS(1.0, "accepted",
				fixedNow.Format(time.RFC3339)),
			// ageDays = floor(0/24) = 0 → decay = 1.0
			// adj = (0.7 + 0.3) × 1.0 = 1.0
			wantAdj:  1.0,
			wantNote: "RFC3339 path",
		},
		{
			name: "rfc3339_exactly_one_half_life_decay_half",
			hit: hitWithTS(1.0, "accepted",
				fixedNow.Add(-90*24*time.Hour).Format(time.RFC3339)),
			// ageDays = 90 → decay = 0.5^(90/90) = 0.5
			// adj = (0.7×1.0 + 0.3×0.5) × 1.0 = 0.85
			wantAdj:  0.85,
			wantNote: "half-life boundary: 90d → decay 0.5",
		},
		{
			name: "rfc3339_two_half_lives_decay_quarter",
			hit: hitWithTS(1.0, "accepted",
				fixedNow.Add(-180*24*time.Hour).Format(time.RFC3339)),
			// ageDays = 180 → decay = 0.5^2 = 0.25
			// adj = (0.7×1.0 + 0.3×0.25) × 1.0 = 0.775
			wantAdj:  0.775,
			wantNote: "two half-lives",
		},
		{
			name: "future_timestamp_clamps_age_to_zero",
			hit: hitWithTS(1.0, "accepted",
				fixedNow.Add(48*time.Hour).Format(time.RFC3339)),
			// (now - ts) = -48h → -2.0 days → floor = -2 → max(0, -2) = 0
			// decay = 1.0; adj = 1.0
			wantAdj:  1.0,
			wantNote: "future ts: math.Max(0, ...) clamps Python parity",
		},
		{
			name: "partial_day_age_floors_down",
			hit: hitWithTS(1.0, "accepted",
				fixedNow.Add(-36*time.Hour).Format(time.RFC3339)),
			// (now - ts).hours/24 = 1.5 → floor = 1.0 (not 1.5)
			// decay = 0.5^(1/90) ≈ 0.9923225751
			// adj = (0.7×1.0 + 0.3×0.9923225751) × 1.0 ≈ 0.9976967725
			//
			// wantAdj uses LITERAL 0.7 / 0.3 / 0.5 / 1.0 / 90.0 — not the
			// exported policy.* constants — so a typo in any constant
			// (e.g., HalfLifeDays → 100) makes impl and want diverge.
			// The formula form is intentional: it documents the spec
			// inline, and binary float drift between this expression and
			// the impl's math.Pow call is identical (same operands), so
			// almostEqual passes only when the spec arithmetic matches.
			// A swap of math.Floor → math.Round would also fail
			// (1.5 rounds to 2, decay base shifts to 0.5^(2/90)).
			wantAdj:  0.7 + 0.3*math.Pow(0.5, 1.0/90.0),
			wantNote: "math.Floor matches Python timedelta.days int truncation",
		},
		{
			name: "float64_unix_timestamp_path",
			hit: hitWithTS(1.0, "accepted",
				float64(fixedNow.Add(-90*24*time.Hour).Unix())),
			// Same as "rfc3339_exactly_one_half_life_decay_half" via
			// the float64 branch.
			wantAdj:  0.85,
			wantNote: "float64 path: datetime.fromtimestamp parity",
		},
		{
			name: "invalid_rfc3339_string_treats_age_as_zero",
			hit:  hitWithTS(1.0, "accepted", "not-a-timestamp"),
			// time.Parse fails → ageDays stays 0 → decay = 1.0
			wantAdj:  1.0,
			wantNote: "Python except (ValueError, TypeError): ageDays unchanged",
		},
		{
			name: "int_timestamp_is_skipped_go_specific_zero_age_masks_divergence",
			hit:  hitWithTS(1.0, "accepted", int(fixedNow.Unix())),
			// Same-day int — both Python (coerce → 0d) and Go (skip → 0d)
			// converge on ageDays=0. Locks the Go contract on this safe path.
			// See "int_timestamp_90d_diverges_from_python" below for the
			// case where the divergence becomes observable.
			wantAdj:  1.0,
			wantNote: "same-day int: both langs reach age=0",
		},
		{
			name: "int_timestamp_90d_diverges_from_python",
			hit:  hitWithTS(1.0, "accepted", int(fixedNow.Add(-90*24*time.Hour).Unix())),
			// **Python ↔ Go DIVERGENCE** (Phase-A documented gap).
			// Python (searcher.py:L286-292): if ts_str is not str, runs
			// `datetime.fromtimestamp(float(ts_str), tz=...)` which coerces
			// int → 90d age → decay 0.5 → adj 0.85.
			// Go (rerank.go:L49-56): type-switch only matches string and
			// float64; int falls through to ageDays=0 → decay=1.0 → adj=1.0.
			// Locked at Go semantics here. In production, JSON-decoded
			// maps yield float64 (encoding/json), so this divergence is
			// unreachable on the wire — but raw Go callers passing int
			// would silently lose recency weighting.
			// TODO(yg): if a future bit-identity audit insists on parity,
			// extend rerank.go's type switch to handle int / int64.
			wantAdj:  1.0,
			wantNote: "Python would yield 0.85 here (90d coerced); Go locks 1.0",
		},
		{
			name: "rfc3339_with_explicit_offset_zero",
			hit: hitWithTS(1.0, "accepted",
				fixedNow.Add(-90*24*time.Hour).Format("2006-01-02T15:04:05-07:00")),
			// Python uses `ts_str.replace("Z", "+00:00")` then fromisoformat;
			// Go's time.RFC3339 layout natively accepts both `Z` and
			// `+00:00`. Same expected adj as the same-90d-`Z` case (0.85).
			wantAdj:  0.85,
			wantNote: "RFC3339 `+00:00` suffix variant parity",
		},
		{
			name: "empty_string_timestamp_treats_age_as_zero",
			hit:  hitWithTS(1.0, "accepted", ""),
			// Python `if ts_str:` short-circuits on empty (falsy); Go's
			// type assertion to string of "" succeeds, time.Parse("")
			// fails → ageDays unchanged = 0. Both paths reach decay=1.0.
			wantAdj:  1.0,
			wantNote: "empty string: same outcome via different code paths",
		},
		{
			name: "wrong_type_metadata_treats_age_as_zero",
			hit:  hitWithTS(1.0, "accepted", []string{"weird"}),
			// Symmetric with the FilterByTime "wrong_type_ts" case.
			// Type-switch matches neither string nor float64 → ageDays=0.
			// Python coerces via `float(["weird"])` → TypeError caught → age=0.
			wantAdj:  1.0,
			wantNote: "wrong type: type-switch fall-through symmetry with FilterByTime",
		},
		{
			name: "score_half_with_180d_decay_gates_raw_score_scaling",
			hit: hitWithTS(0.5, "accepted",
				fixedNow.Add(-180*24*time.Hour).Format(time.RFC3339)),
			// ageDays = 180 → decay = 0.25
			// adj = (0.7×0.5 + 0.3×0.25) × 1.0 = 0.35 + 0.075 = 0.425
			// Without this case, mutations like `SimilarityWeight*r.Score`
			// → `*math.Sqrt(r.Score)` are invisible (1^x == 1 makes
			// score=1.0 cases tautological).
			wantAdj:  0.425,
			wantNote: "score≠1.0 + decay≠1.0: gates raw-score multiplication",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := []domain.SearchHit{tc.hit}
			policy.ApplyRecencyWeighting(hits, fixedNow)
			got := hits[0].AdjustedScore
			if !almostEqual(got, tc.wantAdj) {
				t.Errorf("AdjustedScore = %v, want %v (%s)", got, tc.wantAdj, tc.wantNote)
			}
		})
	}
}

// status multiplier — gate every status (4 known + unknown default + empty).
// Uses Score=0.5 (NOT 1.0) and decay=1.0 so the bracket evaluates to
//
//	(0.7×0.5 + 0.3×1.0) = 0.65
//
// not 1.0 — preventing a swap mutation like
// `(RecencyWeight*raw + SimilarityWeight*decay)` from passing
// coincidentally (which it would if both raw and decay were 1.0).
// Expected adjusted = 0.65 × statusMul.
func TestApplyRecencyWeighting_StatusMultiplierAllValues(t *testing.T) {
	const bracket = 0.65 // 0.7×0.5 + 0.3×1.0
	cases := []struct {
		name   string
		status string
		want   float64
	}{
		{"accepted", "accepted", bracket * 1.0},
		{"proposed", "proposed", bracket * 0.9},
		{"superseded", "superseded", bracket * 0.5},
		{"reverted", "reverted", bracket * 0.3},
		// unknown status defaults to 1.0 (Python: dict.get(s, 1.0))
		{"unknown_value", "weird_unknown_value", bracket * 1.0},
		{"empty_string", "", bracket * 1.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := []domain.SearchHit{
				hitWithTS(0.5, tc.status, fixedNow.Format(time.RFC3339)),
			}
			policy.ApplyRecencyWeighting(hits, fixedNow)
			if !almostEqual(hits[0].AdjustedScore, tc.want) {
				t.Errorf("status %q: AdjustedScore = %v, want %v",
					tc.status, hits[0].AdjustedScore, tc.want)
			}
		})
	}
}

// sort — descending by AdjustedScore. Python: list.sort(key=..., reverse=True).
//
// Critical contract: sort key is AdjustedScore, NOT raw Score. The case
// below picks values so a "sort by raw Score desc" mutation produces a
// DIFFERENT order than the spec — gates the sort key, not just the
// sort direction.
//
//	old_high_raw    Score=0.99, age=10·90d → decay=2^-10≈0.000977
//	                adj = (0.7×0.99 + 0.3×0.000977)×1.0 ≈ 0.69329
//	fresh_med_raw   Score=0.50, age=0      → decay=1.0
//	                adj = (0.7×0.50 + 0.3×1.0)×1.0 = 0.65
//	fresh_low_raw   Score=0.30, age=0      → decay=1.0
//	                adj = (0.7×0.30 + 0.3×1.0)×1.0 = 0.51
//
// Sort by raw Score desc would give [old_high_raw, fresh_med_raw,
// fresh_low_raw]. Sort by adjusted desc gives the SAME order here too.
// So we add a 4th hit whose adjusted is BETWEEN old_high_raw (0.69329)
// and fresh_low_raw (0.51) but whose RAW is greater than old_high_raw —
// impossible because 0.99 is the cap. Use a different lever:
//
//	old_top_raw     Score=0.99, age=10·90d → adj ≈ 0.69329 (rank 2)
//	fresh_top_raw   Score=0.99, age=0      → adj = 0.993    (rank 1)
//
// "sort by raw" sees both as 0.99 → tied. Stable sort would preserve
// input order [old_top_raw first → fresh_top_raw second]. Spec wants
// fresh_top_raw first. This isolates the sort key from the sort
// direction.
//
// All hits have status="accepted" so status_mul=1.0 doesn't move ranks.
func TestApplyRecencyWeighting_SortsDescending(t *testing.T) {
	hits := []domain.SearchHit{
		// (input pos 0) Old, high raw — would be first if sort-by-raw.
		// adj = (0.7×0.99 + 0.3×2^-10)×1.0 ≈ 0.6932930...
		{RecordID: "old_top_raw", Score: 0.99, Status: "accepted",
			Metadata: map[string]any{"timestamp": fixedNow.Add(-900 * 24 * time.Hour).Format(time.RFC3339)}},
		// (input pos 1) Fresh, medium raw.
		// adj = (0.7×0.50 + 0.3×1.0)×1.0 = 0.65
		{RecordID: "fresh_med_raw", Score: 0.50, Status: "accepted",
			Metadata: map[string]any{"timestamp": fixedNow.Format(time.RFC3339)}},
		// (input pos 2) Fresh, identical raw to old_top_raw.
		// Sort-by-raw ties with old_top_raw; spec wants this FIRST.
		// adj = (0.7×0.99 + 0.3×1.0)×1.0 = 0.993
		{RecordID: "fresh_top_raw", Score: 0.99, Status: "accepted",
			Metadata: map[string]any{"timestamp": fixedNow.Format(time.RFC3339)}},
	}
	policy.ApplyRecencyWeighting(hits, fixedNow)

	// Expected by adjusted desc: fresh_top_raw (0.993) > old_top_raw
	// (~0.6933) > fresh_med_raw (0.65). A "sort by raw" mutation would
	// instead tie fresh_top_raw and old_top_raw at 0.99 and (with stable
	// sort) leave them in input order: [old_top_raw, fresh_top_raw, ...].
	wantOrder := []string{"fresh_top_raw", "old_top_raw", "fresh_med_raw"}
	for i, want := range wantOrder {
		if hits[i].RecordID != want {
			t.Errorf("position %d: got %q, want %q (full order: %v)",
				i, hits[i].RecordID, want, recordIDs(hits))
		}
	}
}

// stable sort — when two hits have identical adjusted_score, their
// relative input order must survive. Python's list.sort is stable;
// Go uses sort.SliceStable. Without this test, a future maintainer
// could swap to sort.Slice (unstable) and silently re-order ties.
func TestApplyRecencyWeighting_StableSortPreservesInputOrderOnTies(t *testing.T) {
	mkSame := func(id string) domain.SearchHit {
		return domain.SearchHit{
			RecordID: id, Score: 0.5, Status: "accepted",
			Metadata: map[string]any{"timestamp": fixedNow.Format(time.RFC3339)},
		}
	}
	// All three hits produce identical adjusted_score = 0.65.
	hits := []domain.SearchHit{mkSame("first"), mkSame("second"), mkSame("third")}
	policy.ApplyRecencyWeighting(hits, fixedNow)

	for i, want := range []string{"first", "second", "third"} {
		if hits[i].RecordID != want {
			t.Errorf("stable sort lost input order: pos %d got %q, want %q (order: %v)",
				i, hits[i].RecordID, want, recordIDs(hits))
		}
	}
}

// in-place mutation — ApplyRecencyWeighting mutates the slice it was
// given AND returns it (Python: searcher.py:L299 results.sort(...);
// return results — same semantics). Both behaviors are part of the
// contract; the returned reference must share the backing array.
//
// Uses 2 hits with DIFFERENT scores so a "make + copy + sort + return"
// refactor (which would still produce position-0 alias by accident if
// ranks happened to land that way) is detected by both element address
// AND backing-array identity.
func TestApplyRecencyWeighting_MutatesAndReturnsSameSlice(t *testing.T) {
	hits := []domain.SearchHit{
		hitWithTS(0.5, "accepted", fixedNow.Format(time.RFC3339)),
		hitWithTS(0.9, "accepted", fixedNow.Format(time.RFC3339)),
	}
	// Capture address of the underlying array BEFORE the call. After
	// sort.SliceStable swaps elements in place, &hits[0] still refers
	// to the same array slot, but the SearchHit there may have moved.
	// Backing-array identity is what we want to assert.
	wantBacking := &hits[0]

	got := policy.ApplyRecencyWeighting(hits, fixedNow)

	if &got[0] != wantBacking {
		t.Errorf("returned slice does not share backing array with input " +
			"— caller relying on in-place mutation breaks")
	}
	if len(got) != len(hits) {
		t.Errorf("len(got)=%d, len(hits)=%d — copy semantics suspected", len(got), len(hits))
	}
	// Expected post-sort order: 0.9 first (adj 0.93), 0.5 second (adj 0.65).
	if hits[0].Score != 0.9 || hits[1].Score != 0.5 {
		t.Errorf("input slice not sorted in place: %v", []float64{hits[0].Score, hits[1].Score})
	}
	if !almostEqual(hits[0].AdjustedScore, 0.93) {
		t.Errorf("AdjustedScore at [0] = %v, want 0.93", hits[0].AdjustedScore)
	}
}

// FilterByTime — every scope keeps fresh records and drops old ones at
// the configured cutoff. Boundary precision is asserted in a separate
// test below.
func TestFilterByTime_EachScopeCutoffWorks(t *testing.T) {
	cases := []struct {
		scope    domain.TimeScope
		freshAge time.Duration // age that should be kept
		oldAge   time.Duration // age that should be dropped
	}{
		{domain.TimeScopeLastWeek, 1 * 24 * time.Hour, 8 * 24 * time.Hour},
		{domain.TimeScopeLastMonth, 5 * 24 * time.Hour, 31 * 24 * time.Hour},
		{domain.TimeScopeLastQuarter, 30 * 24 * time.Hour, 91 * 24 * time.Hour},
		{domain.TimeScopeLastYear, 100 * 24 * time.Hour, 366 * 24 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(string(tc.scope), func(t *testing.T) {
			hits := []domain.SearchHit{
				{RecordID: "fresh", Metadata: map[string]any{
					"timestamp": fixedNow.Add(-tc.freshAge).Format(time.RFC3339)}},
				{RecordID: "old", Metadata: map[string]any{
					"timestamp": fixedNow.Add(-tc.oldAge).Format(time.RFC3339)}},
			}
			got := policy.FilterByTime(hits, tc.scope, fixedNow)
			if len(got) != 1 || got[0].RecordID != "fresh" {
				t.Errorf("%v: got %v, want [fresh]", tc.scope, recordIDs(got))
			}
		})
	}
}

// AllTime + unknown scope — both must return input unchanged. AllTime
// is the documented default; unknown is defensive (invalid input
// shouldn't drop records silently).
func TestFilterByTime_AllTimeAndUnknownScopeReturnUnchanged(t *testing.T) {
	hits := []domain.SearchHit{
		{RecordID: "ancient", Metadata: map[string]any{
			"timestamp": fixedNow.Add(-10 * 365 * 24 * time.Hour).Format(time.RFC3339)}},
	}

	gotAllTime := policy.FilterByTime(hits, domain.TimeScopeAllTime, fixedNow)
	if len(gotAllTime) != 1 {
		t.Errorf("AllTime: dropped record, got %d hits", len(gotAllTime))
	}

	gotUnknown := policy.FilterByTime(hits, domain.TimeScope("not_a_scope"), fixedNow)
	if len(gotUnknown) != 1 {
		t.Errorf("unknown scope: dropped record, got %d hits", len(gotUnknown))
	}
}

// FilterByTime keeps records with no timestamp / invalid timestamp.
// Python parity (searcher.py:L546-557): the explicit "else: append"
// branches for both missing-key and parse-failure paths.
//
// Rationale: dropping unparseable records would silently lose data on
// schema drift; keeping them surfaces it via downstream search results.
func TestFilterByTime_KeepsRecordsWithMissingOrInvalidTimestamp(t *testing.T) {
	hits := []domain.SearchHit{
		{RecordID: "no_ts", Metadata: map[string]any{}},
		{RecordID: "invalid_string_ts", Metadata: map[string]any{
			"timestamp": "not-a-timestamp"}},
		{RecordID: "wrong_type_ts", Metadata: map[string]any{
			"timestamp": []string{"weird"}}},
	}
	got := policy.FilterByTime(hits, domain.TimeScopeLastWeek, fixedNow)
	if len(got) != 3 {
		t.Errorf("expected all 3 kept (Python parity), got %d (%v)", len(got), recordIDs(got))
	}
}

// FilterByTime cutoff boundary — `ts.Before(cutoff)` is strict. A
// record with ts == cutoff is kept; one nanosecond older is dropped.
// This is the exact boundary the Python code asserts via `ts >= cutoff`.
//
// Uses time.RFC3339Nano on both sides — plain time.RFC3339 truncates
// sub-second precision (the older record would round-trip as exactly
// 1 second older, and a regression in the boundary handling at
// nanosecond precision would slip through silently).
func TestFilterByTime_CutoffBoundaryIsInclusiveOfCutoffTime(t *testing.T) {
	// LastWeek scope → cutoff = fixedNow - 7d.
	cutoff := fixedNow.Add(-7 * 24 * time.Hour)
	hits := []domain.SearchHit{
		{RecordID: "at_cutoff", Metadata: map[string]any{
			"timestamp": cutoff.Format(time.RFC3339Nano)}},
		{RecordID: "one_ns_older", Metadata: map[string]any{
			"timestamp": cutoff.Add(-time.Nanosecond).Format(time.RFC3339Nano)}},
	}
	got := policy.FilterByTime(hits, domain.TimeScopeLastWeek, fixedNow)

	if len(got) != 1 || got[0].RecordID != "at_cutoff" {
		t.Errorf("boundary contract: ts==cutoff must be kept, ts<cutoff dropped; got %v",
			recordIDs(got))
	}
}

// FilterByTime — future timestamps are kept. The cutoff branch only
// drops when ts.Before(cutoff); a future ts is past cutoff in the
// other direction, so it stays.
//
// Uses 30-day-FUTURE timestamps against LastWeek (7d range). A
// regression that swapped the cutoff comparison to `abs(now-ts) >
// range → drop` would drop these (|30d| > 7d), so the test fails on
// that mutation. A 48h-future against LastWeek would be vacuous since
// |2d| < 7d — kept either way.
func TestFilterByTime_FutureTimestampIsKept(t *testing.T) {
	farFuture := 30 * 24 * time.Hour
	hits := []domain.SearchHit{
		{RecordID: "future_string", Metadata: map[string]any{
			"timestamp": fixedNow.Add(farFuture).Format(time.RFC3339)}},
		{RecordID: "future_float64", Metadata: map[string]any{
			"timestamp": float64(fixedNow.Add(farFuture).Unix())}},
	}
	got := policy.FilterByTime(hits, domain.TimeScopeLastWeek, fixedNow)
	if len(got) != 2 {
		t.Errorf("expected both future records kept (|30d-future| > 7d range "+
			"would still be dropped under abs-distance regression), got %d (%v)",
			len(got), recordIDs(got))
	}
}

// FilterByTime — float64 unix timestamp path also filters correctly.
// Same scenarios as the RFC3339 case above but routed through the
// float64 branch.
func TestFilterByTime_Float64UnixTimestampPath(t *testing.T) {
	hits := []domain.SearchHit{
		{RecordID: "fresh", Metadata: map[string]any{
			"timestamp": float64(fixedNow.Add(-1 * 24 * time.Hour).Unix())}},
		{RecordID: "old", Metadata: map[string]any{
			"timestamp": float64(fixedNow.Add(-8 * 24 * time.Hour).Unix())}},
	}
	got := policy.FilterByTime(hits, domain.TimeScopeLastWeek, fixedNow)
	if len(got) != 1 || got[0].RecordID != "fresh" {
		t.Errorf("float64 ts: got %v, want [fresh]", recordIDs(got))
	}
}

// FilterByTime empty input — no panic, returns empty.
func TestFilterByTime_EmptyInput(t *testing.T) {
	if got := policy.FilterByTime(nil, domain.TimeScopeLastWeek, fixedNow); len(got) != 0 {
		t.Errorf("nil input: got %d hits, want 0", len(got))
	}
	if got := policy.FilterByTime([]domain.SearchHit{}, domain.TimeScopeLastWeek, fixedNow); len(got) != 0 {
		t.Errorf("empty input: got %d hits, want 0", len(got))
	}
}

// recordIDs — small helper for cleaner failure messages.
func recordIDs(hits []domain.SearchHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.RecordID
	}
	return out
}
