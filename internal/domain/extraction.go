package domain

// Agent extraction types (ExtractionResult hierarchy).
// Spec: docs/v04/spec/types.md §3a.
// Python: agents/scribe/llm_extractor.py:L28-70 — types only in agent-delegated mode.
//
// Wire format: agent sends flat JSON in CaptureRequest.Extracted.
// Internal: rune-mcp splits into Detection (see query.go) + ExtractionResult.
// Mapping table: see types.md §3a.4.

// ExtractedFields — §3a.1. Single-record extraction (no phases).
// Python: llm_extractor.py:L28-37.
type ExtractedFields struct {
	Title        string   `json:"title,omitempty"` // 60-rune truncate
	Rationale    string   `json:"rationale,omitempty"`
	Problem      string   `json:"problem,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"`
	TradeOffs    []string `json:"trade_offs,omitempty"`
	StatusHint   string   `json:"status_hint,omitempty"` // "proposed" | "accepted" | "rejected"
	Tags         []string `json:"tags,omitempty"`
}

// PhaseExtractedFields — §3a.2. One phase in phase_chain or bundle.
// Python: llm_extractor.py:L40-49.
type PhaseExtractedFields struct {
	PhaseTitle     string   `json:"phase_title,omitempty"` // 60-rune truncate
	PhaseDecision  string   `json:"phase_decision,omitempty"`
	PhaseRationale string   `json:"phase_rationale,omitempty"`
	PhaseProblem   string   `json:"phase_problem,omitempty"`
	Alternatives   []string `json:"alternatives,omitempty"`
	TradeOffs      []string `json:"trade_offs,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// ExtractionResult — §3a.3. Top-level (single / phase_chain / bundle).
// Python: llm_extractor.py:L52-70.
type ExtractionResult struct {
	GroupTitle   string                 `json:"group_title,omitempty"`
	GroupType    string                 `json:"group_type,omitempty"`    // "phase_chain" | "bundle" | "" (single)
	GroupSummary string                 `json:"group_summary,omitempty"` // shared across all phases
	StatusHint   string                 `json:"status_hint,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Confidence   *float64               `json:"confidence,omitempty"` // agent-provided [0.0, 1.0]
	Single       *ExtractedFields       `json:"single,omitempty"`
	Phases       []PhaseExtractedFields `json:"phases,omitempty"` // cap 7 (phase) / 5 (bundle)
}

// IsMultiPhase — Python property: llm_extractor.py:L64-66.
func (r *ExtractionResult) IsMultiPhase() bool {
	return len(r.Phases) > 1
}

// IsBundle — Python property: llm_extractor.py:L68-70.
func (r *ExtractionResult) IsBundle() bool {
	return r.GroupType == "bundle" && len(r.Phases) > 1
}

// ParseExtractionFromAgent builds Detection + ExtractionResult from the flat
// CaptureRequest.Extracted dict sent by the agent. Wire → internal conversion.
//
// Mapping table: docs/v04/spec/types.md §3a.4.
// Python reference: mcp/server/server.py:L1244-1324 (_capture_single).
//
// TODO: extract tier2.{capture,reason,domain} into Detection;
//	extract remaining fields into ExtractionResult;
//	handle single vs phases dispatch.
func ParseExtractionFromAgent(extracted map[string]any) (*Detection, *ExtractionResult, error) {
	// TODO: implement per types.md §3a.4 mapping table
	return &Detection{}, &ExtractionResult{}, nil
}
