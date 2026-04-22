package domain

// MCP Capture tool I/O.
// Spec: docs/v04/spec/types.md §4.1.
// Python: mcp/server/server.py:L698-806 (entry) · L1208-1407 (_capture_single).
// Flow: docs/v04/spec/flows/capture.md (7-phase).

// CaptureRequest — §4.1.
// Extracted is the flat wire format split into Detection + ExtractionResult
// internally (see types.md §3a.4 mapping).
type CaptureRequest struct {
	Text      string         `json:"text"`
	Source    string         `json:"source"`
	User      string         `json:"user,omitempty"`
	Channel   string         `json:"channel,omitempty"`
	Extracted map[string]any `json:"extracted"` // see types.md §3a.4
}

// CaptureResponse — §4.1.
// Note: no `similar_to` field (D10 Archived — Python parity). Duplicate record
// info flows via Novelty.Related[] (see NoveltyInfo in query.go).
type CaptureResponse struct {
	OK       bool   `json:"ok"`
	Captured bool   `json:"captured"`
	RecordID string `json:"record_id,omitempty"`
	Title    string `json:"title,omitempty"`
	Domain   Domain `json:"domain,omitempty"`

	Reason  string       `json:"reason,omitempty"`
	Novelty *NoveltyInfo `json:"novelty,omitempty"`

	Error string `json:"error,omitempty"`
}

// RawEvent — input to record_builder.BuildPhases.
// Python: agents/scribe/record_builder.py RawEvent (dataclass at top of file).
type RawEvent struct {
	Text    string
	Source  string
	User    string
	Channel string
	// TS, metadata fields — see Python original
}
