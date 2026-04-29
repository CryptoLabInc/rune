package policy_test

// Tests for ClassifyNovelty + DefaultNoveltyThresholds.
//
// Python parity baseline: agents/tests/test_novelty_check.py (5 cases —
// one per class + empty memory). Go ports those, then adds:
//
//   - all 4 class boundaries with just-below / at / just-above probes
//   - custom thresholds (Python module constants {0.4, 0.7, 0.93}, which
//     diverge from runtime D11 values {0.3, 0.7, 0.95}; both must work)
//   - score formula: round(1.0 - sim, 4) — including the inverted polarity
//     contract (higher score = more novel)
//   - DefaultNoveltyThresholds value lock (silent constant drift would
//     change recall behavior on every capture; gate it explicitly)
//   - NoveltyClass enum string values lock (wire format)
//
// Black-box style — only the public Parse-equivalent (ClassifyNovelty) and
// exported constants are exercised.

import (
	"math"
	"testing"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/policy"
)

// classification — covers all 4 classes with default D11 runtime thresholds
// {0.3, 0.7, 0.95}. Python parity (test_novelty_check.py:5-42) plus
// just-below / at-boundary probes that Python does not gate.
//
// Boundary contract (policy/novelty.go:37-49):
//
//	sim <  0.3   → novel
//	0.3 ≤ sim <  0.7   → evolution
//	0.7 ≤ sim <  0.95  → related
//	sim ≥  0.95  → near_duplicate
//
// The boundaries are LEFT-CLOSED, RIGHT-OPEN — i.e., 0.3 itself is
// evolution, not novel. Tested explicitly because Python uses the same
// rule but never asserts the boundary.
func TestClassifyNovelty_DefaultThresholds(t *testing.T) {
	cases := []struct {
		name string
		sim  float64
		want domain.NoveltyClass
	}{
		// novel — sim < 0.3
		{"novel_zero", 0.0, domain.NoveltyClassNovel},
		{"novel_python_parity", 0.2, domain.NoveltyClassNovel},
		{"novel_just_below_boundary", 0.299, domain.NoveltyClassNovel},
		// evolution — 0.3 ≤ sim < 0.7
		{"evolution_at_lower_boundary", 0.3, domain.NoveltyClassEvolution},
		{"evolution_python_parity", 0.5, domain.NoveltyClassEvolution},
		{"evolution_just_below_upper", 0.6999, domain.NoveltyClassEvolution},
		// related — 0.7 ≤ sim < 0.95
		{"related_at_lower_boundary", 0.7, domain.NoveltyClassRelated},
		{"related_python_parity", 0.85, domain.NoveltyClassRelated},
		{"related_just_below_upper", 0.9499, domain.NoveltyClassRelated},
		// near_duplicate — sim ≥ 0.95
		{"near_duplicate_at_boundary", 0.95, domain.NoveltyClassNearDuplicate},
		{"near_duplicate_python_parity", 0.97, domain.NoveltyClassNearDuplicate},
		{"near_duplicate_one", 1.0, domain.NoveltyClassNearDuplicate},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := policy.ClassifyNovelty(tc.sim, policy.DefaultNoveltyThresholds)
			if got != tc.want {
				t.Errorf("ClassifyNovelty(%v).class = %q, want %q", tc.sim, got, tc.want)
			}
		})
	}
}

// score — inverted polarity (1.0 - sim) rounded to 4 decimals. Python
// parity (test_novelty_check.py:10, 18, 26, 34, 42).
//
// Polarity contract (policy/novelty.go:30-31): "higher score means more
// novel" — intentionally inverted from raw similarity. The tests below
// pin the contract at both extremes (sim=0 → score=1.0, sim=1 → score=0.0)
// and at non-trivial midpoints to prevent a future "I thought score was
// just sim" refactor.
//
// The chosen sim values do NOT need to be exactly representable in
// binary float — round(_, 4) absorbs the natural drift (e.g.,
// 1.0 - 0.97 = 0.030000000000000027 → *10000 = 300.0000... → round → 300
// → /10000 = 0.03). What we DO avoid is the Python banker's rounding
// vs Go round-half-away-from-zero divergence at exact .x...5 boundaries.
// None of the values below land on such a boundary; the dedicated
// TestClassifyNovelty_ScoreRounding probes near-boundary cases.
func TestClassifyNovelty_Score(t *testing.T) {
	cases := []struct {
		name string
		sim  float64
		want float64
	}{
		{"sim_zero_max_novelty", 0.0, 1.0},
		{"sim_one_zero_novelty", 1.0, 0.0},
		{"sim_python_parity_near_duplicate_097", 0.97, 0.03},
		{"sim_half", 0.5, 0.5},
		{"sim_python_parity_related_085", 0.85, 0.15},
		{"sim_python_parity_novel_02", 0.2, 0.8},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := policy.ClassifyNovelty(tc.sim, policy.DefaultNoveltyThresholds)
			// Score is 4-decimal rounded; compare with epsilon for the
			// Python pytest.approx equivalent.
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("ClassifyNovelty(%v).score = %v, want %v", tc.sim, got, tc.want)
			}
		})
	}
}

// score rounding — Python uses banker's rounding (round half to even);
// Go math.Round rounds half away from zero. For score = 1.0 - sim where
// the resulting (1-sim)*10000 lands on a half boundary, the two
// languages can disagree by 1e-4.
//
// Verified empirically (python3 + Go math.Round):
//
//	sim=0.12345 → 1-sim ≈ 0.87654999999... → *10000 ≈ 8765.5 (just over)
//	  Python round(_, 4) = 0.8765   (banker's rounds half to even)
//	  Go     0.8766                 (away-from-zero)
//	sim=0.12355 → 1-sim ≈ 0.87645    → *10000 = 8764.5 (exact half)
//	  Python = 0.8764, Go = 0.8765
//
// The "banker_round_diverges_from_python" case below witnesses the
// divergence; the other cases stay on Go's contract without ambiguity.
//
// TODO(yg): if a future bit-identity audit insists on round-half-to-
// even parity, swap math.Round for a custom banker's helper in
// novelty.go and update the divergence case below.
func TestClassifyNovelty_ScoreRounding(t *testing.T) {
	cases := []struct {
		name string
		sim  float64
		want float64
	}{
		// 1.0 - 0.123456 = 0.876544 → *10000 = 8765.44 → round → 8765 → 0.8765
		// (5th decimal is 4, no half-boundary, both langs agree)
		{"rounds_down_below_half", 0.123456, 0.8765},
		// 1.0 - 0.987654 = 0.012346 (post-format) → *10000 ≈ 123.46 → round → 123 → 0.0123
		// (float32 reality: 0.012345999... — still < 0.5 fractional)
		{"rounds_down_far_from_half", 0.987654, 0.0123},
		// 1.0 - 0.99996 ≈ 4e-5 → *10000 ≈ 0.4 → round → 0 → 0.0
		{"underflow_to_zero", 0.99996, 0.0},
		// **Python ↔ Go divergence witness** — sim=0.12355 makes
		// (1-sim)*10000 = 8764.5 EXACTLY (verified via python3). Python's
		// banker's rounding picks the even neighbor (8764 → 0.8764); Go's
		// math.Round picks away-from-zero (8765 → 0.8765). We assert the
		// Go value here. If this case starts failing because Go gives
		// 0.8764, that means novelty.go switched to banker's — update the
		// production-code TODO above.
		{"banker_round_diverges_from_python", 0.12355, 0.8765},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := policy.ClassifyNovelty(tc.sim, policy.DefaultNoveltyThresholds)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("score rounding: ClassifyNovelty(%v).score = %v, want %v",
					tc.sim, got, tc.want)
			}
		})
	}
}

// custom thresholds — exercises the threshold parameter rather than the
// runtime D11 default. The values used here are Python module constants
// from embedding.py:L16-18 (NOVELTY_THRESHOLD_{NOVEL,RELATED,NEAR_DUPLICATE}
// = 0.4 / 0.7 / 0.93), which Python tests do NOT use as a set
// (test_novelty_check.py only mixes them per-call).
//
// The runtime D11 mismatch (server.py:L102-104 passes 0.3/0.7/0.95
// explicitly) is part of the agent-delegated SOT (architecture.md §Scope).
// This test gates that ClassifyNovelty respects the per-call threshold
// argument — which is the only way the runtime override works.
func TestClassifyNovelty_CustomThresholds(t *testing.T) {
	pyModuleThresholds := policy.NoveltyThresholds{
		Novel:   0.4,
		Related: 0.7,
		NearDup: 0.93,
	}

	cases := []struct {
		name string
		sim  float64
		want domain.NoveltyClass
	}{
		// 0.35 is novel under {0.4, ...} but evolution under default {0.3, ...}.
		{"novel_under_higher_novel_threshold", 0.35, domain.NoveltyClassNovel},
		// 0.4 is the new evolution boundary under module thresholds.
		{"evolution_at_module_lower_boundary", 0.4, domain.NoveltyClassEvolution},
		// 0.93 is near_duplicate under module thresholds but related under default.
		{"near_duplicate_at_module_boundary", 0.93, domain.NoveltyClassNearDuplicate},
		// 0.94 is near_duplicate under module but related under default {<0.95}.
		{"near_duplicate_above_module_boundary", 0.94, domain.NoveltyClassNearDuplicate},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := policy.ClassifyNovelty(tc.sim, pyModuleThresholds)
			if got != tc.want {
				t.Errorf("ClassifyNovelty(%v, module).class = %q, want %q",
					tc.sim, got, tc.want)
			}
		})
	}
}

// DefaultNoveltyThresholds — runtime D11 values (server.py:L102-104).
// A silent change here would shift every capture's classification —
// gate the exact bytes.
func TestDefaultNoveltyThresholds_LockedToD11Values(t *testing.T) {
	if got := policy.DefaultNoveltyThresholds.Novel; got != 0.3 {
		t.Errorf("DefaultNoveltyThresholds.Novel = %v, want 0.3 (D11)", got)
	}
	if got := policy.DefaultNoveltyThresholds.Related; got != 0.7 {
		t.Errorf("DefaultNoveltyThresholds.Related = %v, want 0.7 (D11)", got)
	}
	if got := policy.DefaultNoveltyThresholds.NearDup; got != 0.95 {
		t.Errorf("DefaultNoveltyThresholds.NearDup = %v, want 0.95 (D11)", got)
	}
}

// NoveltyClass enum — wire format gate. These string values appear in
// the capture response JSON and capture_log.jsonl entries (D20). Any
// change is a breaking schema change — lock them at the test layer.
//
// Each constant is its own subtest so a paired-swap mistake (someone
// reorders BOTH the constant and the want literal together) is visible
// in the test output as a renamed subtest, and a single-side swap
// (e.g., changing only the const value) fails the matching subtest.
func TestNoveltyClass_WireValues(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"novel", string(domain.NoveltyClassNovel), "novel"},
		{"evolution", string(domain.NoveltyClassEvolution), "evolution"},
		{"related", string(domain.NoveltyClassRelated), "related"},
		{"near_duplicate", string(domain.NoveltyClassNearDuplicate), "near_duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("NoveltyClass enum: got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
