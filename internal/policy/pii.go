package policy

import (
	"fmt"
	"regexp"
	"strings"
)

// PII redaction — Python: agents/scribe/record_builder.py:L89-95 SENSITIVE_PATTERNS
// + L406-418 _redact_sensitive.
//
// Called from BuildPhases entry (L228) in BOTH legacy and agent-delegated modes
// (rune-mcp is responsible for PII redaction per D13 Option A).

type sensitivePattern struct {
	regex       *regexp.Regexp
	replacement string
}

// sensitivePatterns — 5 patterns, exact regex from Python L89-95.
// Order matters: more specific patterns (e.g., API key with prefix) must come
// before the generic long-alphanumeric pattern.
var sensitivePatterns = []sensitivePattern{
	{regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`), "[EMAIL]"},
	{regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`), "[PHONE]"},
	{regexp.MustCompile(`(?i)\b(?:sk|pk|api|key|token|secret|password)[_\-][a-zA-Z0-9_\-]{15,}\b`), "[API_KEY]"},
	{regexp.MustCompile(`\b[A-Za-z0-9]{32,}\b`), "[API_KEY]"},
	{regexp.MustCompile(`\b[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}\b`), "[CARD]"},
}

// RedactSensitive — Python: record_builder.py:L406-418 _redact_sensitive.
// Returns the redacted text and an optional notes string (e.g. "Redacted 2 [EMAIL]; Redacted 1 [PHONE]").
// Also applies MAX_INPUT_CHARS truncation after redaction.
func RedactSensitive(text string) (string, string) {
	redacted := text
	var notes []string

	for _, sp := range sensitivePatterns {
		matches := sp.regex.FindAllString(redacted, -1)
		if len(matches) > 0 {
			redacted = sp.regex.ReplaceAllString(redacted, sp.replacement)
			notes = append(notes, fmt.Sprintf("Redacted %d %s", len(matches), sp.replacement))
		}
	}

	redacted = truncRunes(redacted, MaxInputChars)

	noteStr := ""
	if len(notes) > 0 {
		noteStr = strings.Join(notes, "; ")
	}
	return redacted, noteStr
}
