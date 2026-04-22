package domain

// Capture log entry.
// Spec: docs/v04/spec/types.md §6.
// Python: mcp/server/server.py:L115-138 _append_capture_log. Format: D20 bit-identical.
// Written to ~/.rune/capture_log.jsonl (0600, append-only).

// CaptureLogEntry — §6. One JSONL line per capture / delete.
// NoveltyScore uses pointer so we can omit when caller doesn't set it
// (Python if novelty_class: check → Go explicit nil).
type CaptureLogEntry struct {
	TS           string   `json:"ts"`     // RFC3339 UTC
	Action       string   `json:"action"` // "captured" | "deleted"
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Domain       string   `json:"domain"`
	Mode         string   `json:"mode"` // "standard" | "soft-delete" | ...
	NoveltyClass string   `json:"novelty_class,omitempty"`
	NoveltyScore *float64 `json:"novelty_score,omitempty"`
}
