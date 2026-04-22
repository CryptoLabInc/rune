package policy

// PII redaction — Python: agents/scribe/record_builder.py:L89-95 SENSITIVE_PATTERNS
// + L406-418 _redact_sensitive.
//
// Called from BuildPhases entry (L228) in BOTH legacy and agent-delegated modes
// (rune-mcp is responsible for PII redaction per D13 Option A).
//
// 5 patterns (exact regex to port from Python):
//  1. Email:       [A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+
//  2. Phone:       \d{3}[-.]?\d{3}[-.]?\d{4}
//  3. API key:     [sk|pk|api|key|token|secret|password]_[a-zA-Z0-9_-]{15,}
//  4. Long hex:    [A-Za-z0-9]{32,}
//  5. Credit card: [0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}
//
// Replacement labels per Python (exact strings): see record_builder.py:L89-95.

// RedactSensitive — Python: record_builder.py:L406-418 _redact_sensitive.
// Also applies MAX_INPUT_CHARS truncate (L227) after redaction.
//
// TODO: port 5 regex + replacement labels + length cap.
func RedactSensitive(text string) string {
	// TODO: bit-identical to _redact_sensitive
	if len(text) > MaxInputChars {
		text = text[:MaxInputChars]
	}
	return text
}
