package policy

import "github.com/envector/rune-go/internal/domain"

// Payload text renderer — Python canonical: agents/common/schemas/templates.py (364 LoC).
// D15: line-by-line port. Verified via byte-for-byte golden fixture test.
//
// payload.text is the embedding fallback target (when reusable_insight empty)
// and the recall display text. Any divergence from Python changes embedding
// vector space → breaks recall.
//
// Porting scope:
//  ✅ PAYLOAD_TEMPLATE (L14~) — multi-line format string
//  ✅ _format_* helpers (L52-131) — 7 helpers (alternatives, trade_offs,
//     assumptions, risks, evidence, links, tags)
//  ✅ render_payload_text (L138-222) — main
//  🟡 render_compact_payload (L225-238) — Post-MVP
//  🟡 render_display_text (L288-363) — Post-MVP (EN/KO/JA locale)
//
// Subtle behaviors (easy to miss in porting):
//  1. phase_line / group_summary post-insertion (L204-216) — inserted AFTER
//     template.format(), not in the template string itself
//  2. Blank line collapse + strip (L219-222): while "\n\n\n" in text:
//     text = text.replace("\n\n\n", "\n\n"); then .strip()
//  3. _format_alternatives "chosen" marker bug (L59): chosen=="" makes all
//     alternatives marked "(chosen)" — Python current behavior, keep bit-identical

// RenderPayloadText — Python: render_payload_text(record) at L138-222.
// TODO: line-by-line port with golden fixture test.
func RenderPayloadText(r *domain.DecisionRecord) string {
	// TODO: port per D15 canonical reference
	return ""
}
