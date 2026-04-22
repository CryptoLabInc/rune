// Package obs — observability: slog handler with sensitive data filter + request_id.
// Spec: docs/v04/spec/components/rune-mcp.md §Observability.
// Python: mcp/server/server.py:L25-40 _SensitiveFilter.
package obs

import (
	"context"
	"log/slog"
	"os"
)

// SensitivePatterns — 2 regex (Python server.py:L28-31):
//
//  1. (sk-|pk-|api_|envector_|evt_)[a-zA-Z0-9_-]{10,}
//     — 6 prefixes, 10+ chars
//
//  2. (?i)(token|key|secret|password)["\s:=]+[a-zA-Z0-9_-]{20,}
//     — 4 field names + separator + 20+ chars
//
// Replacement: m.group()[:8] + "***" (preserve first 8 chars).
//
// TODO: port both regex + replacement into a slog.Handler.
var SensitivePatterns = []string{
	// TODO: compile `(sk-|pk-|api_|envector_|evt_)[a-zA-Z0-9_-]{10,}`
	// TODO: compile `(?i)(token|key|secret|password)["\s:=]+[a-zA-Z0-9_-]{20,}`
}

// NewHandler returns a slog.Handler that scrubs sensitive data from messages.
// TODO: wrap slog.NewJSONHandler and redact per SensitivePatterns.
func NewHandler(level slog.Level) slog.Handler {
	// TODO: return &filteringHandler{inner: slog.NewJSONHandler(os.Stderr, ...), patterns: compiled}
	return slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
}

// ─────────────────────────────────────────────────────────────────────────────
// Request ID — per tool call, propagated via context
// ─────────────────────────────────────────────────────────────────────────────

type ctxKey int

const requestIDKey ctxKey = 0

// WithRequestID stores an ID in context (UUID generated at MCP handler entry).
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the ID (empty if not set).
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// NewRequestID generates a UUID-like request ID.
// TODO: use crypto/rand for UUID v4 or sequential monotonic ID.
func NewRequestID() string {
	// TODO
	return ""
}
