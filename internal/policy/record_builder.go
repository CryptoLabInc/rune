package policy

import (
	"time"

	"github.com/envector/rune-go/internal/domain"
)

// Record builder — Python canonical: agents/scribe/record_builder.py (703 LoC).
// D13 Option A: Go ports all logic (not delegated to agent).
// D14: pre_extraction required (no LLM fallback).
// Spec: docs/v04/spec/flows/capture.md Phase 5 + canonical-reference section.

// MAX_INPUT_CHARS — Python L227. Truncate cleanText before extraction.
const MaxInputChars = 12_000

// SensitivePatterns — 5 regex for PII redaction (Python L89-95).
// Full patterns live in pii.go (separate file for clarity).

// QuotePatterns — 4 regex (Python L72-77): double "", single '', Japanese 「」,
// French «». Min 10 chars.
var QuotePatterns = []string{
	// TODO: port from record_builder.py:L72-77
}

// RationalePatterns — 5 regex (Python L80-86): because / reason / rationale /
// since / due to.
var RationalePatterns = []string{
	// TODO: port from record_builder.py:L80-86
}

// BuildPhases — Python: record_builder.py build_phases(rawEvent, detection, pre_extraction).
//
// Agent-delegated mode (D14) requires pre_extraction != nil; otherwise returns
// ErrExtractionMissing.
//
// Order-critical (Python L196-199, L310-311, L395-396):
//  1. Redact PII from rawEvent.Text → cleanText (ALWAYS, even in agent-delegated)
//  2. Assemble record(s) with payload.text = ""
//  3. EnsureEvidenceCertaintyConsistency per record (§7.1)
//  4. Render payload.text = RenderPayloadText(record)
//  5. Set reusable_insight = pre_extraction.group_summary (if present)
//
// Returns 1-7 records (single / phase_chain / bundle per ExtractionResult.GroupType).
//
// TODO: implement. Python reference is 703 LoC; port line-by-line with golden fixture test.
func BuildPhases(
	rawEvent *domain.RawEvent,
	detection *domain.Detection,
	preExtraction *domain.ExtractionResult,
	now time.Time,
) ([]domain.DecisionRecord, error) {
	if preExtraction == nil {
		return nil, domain.ErrExtractionMissing
	}
	// TODO: bit-identical to record_builder.py:L196-404
	// Methods to port: _build_single_record_from_extraction (L264-317),
	//                  _build_multi_record_from_extraction (L319-404).
	return nil, nil
}
