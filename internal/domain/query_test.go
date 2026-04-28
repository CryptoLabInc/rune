// Tests for query/recall domain helpers — port of Python's SearchResult
// is_reliable / is_phase predicates plus the v0.4-simplified payload
// extraction. Python: agents/retriever/searcher.py.

package domain_test

import (
	"testing"

	"github.com/envector/rune-go/internal/domain"
)

// SearchHit.IsReliable — supported / partially_supported map to true,
// everything else (including empty string) is unreliable.
// Python: searcher.py:SearchResult.is_reliable.
func TestSearchHit_IsReliable(t *testing.T) {
	cases := []struct {
		name      string
		certainty string
		want      bool
	}{
		{"supported", "supported", true},
		{"partially_supported", "partially_supported", true},
		{"unknown", "unknown", false},
		{"unsupported", "unsupported", false},
		{"empty", "", false},
		{"unrelated_string", "high", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &domain.SearchHit{Certainty: tc.certainty}
			if got := h.IsReliable(); got != tc.want {
				t.Errorf("IsReliable(%q) = %v, want %v", tc.certainty, got, tc.want)
			}
		})
	}
}

// SearchHit.IsPhase — GroupID non-nil ⇔ true.
// Python: searcher.py:SearchResult.is_phase.
func TestSearchHit_IsPhase(t *testing.T) {
	t.Run("group_id_nil_is_not_phase", func(t *testing.T) {
		h := &domain.SearchHit{}
		if h.IsPhase() {
			t.Error("IsPhase with nil GroupID should be false")
		}
	})

	t.Run("group_id_set_is_phase", func(t *testing.T) {
		gid := "grp_2026-01-01_arch_strategy"
		h := &domain.SearchHit{GroupID: &gid}
		if !h.IsPhase() {
			t.Error("IsPhase with non-nil GroupID should be true")
		}
	})

	t.Run("group_id_pointer_to_empty_string_is_phase", func(t *testing.T) {
		// Documenting current behavior: pointer presence drives the predicate,
		// not string contents. If empty string should also be "no phase",
		// the predicate must change.
		empty := ""
		h := &domain.SearchHit{GroupID: &empty}
		if !h.IsPhase() {
			t.Error("IsPhase with pointer to empty string is currently true (predicate uses pointer presence)")
		}
	})
}

// ExtractPayloadText — strict v2.1 (D32). No v1/v2.0 fallback.
// Python: searcher.py:L487-496 (v0.4 simplified to payload.text only).
func TestExtractPayloadText(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want string
	}{
		{
			name: "standard_payload_text",
			meta: map[string]any{"payload": map[string]any{"text": "hello"}},
			want: "hello",
		},
		{
			name: "empty_text_field",
			meta: map[string]any{"payload": map[string]any{"text": ""}},
			want: "",
		},
		{
			name: "payload_missing",
			meta: map[string]any{},
			want: "",
		},
		{
			name: "payload_not_a_map",
			meta: map[string]any{"payload": "raw string"},
			want: "",
		},
		{
			name: "text_field_not_a_string",
			meta: map[string]any{"payload": map[string]any{"text": 42}},
			want: "",
		},
		{
			name: "text_field_missing",
			meta: map[string]any{"payload": map[string]any{"format": "markdown"}},
			want: "",
		},
		{
			name: "nil_metadata",
			meta: nil,
			want: "",
		},
		{
			name: "payload_is_nil",
			meta: map[string]any{"payload": nil},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.ExtractPayloadText(tc.meta)
			if got != tc.want {
				t.Errorf("ExtractPayloadText(%v) = %q, want %q", tc.meta, got, tc.want)
			}
		})
	}
}
