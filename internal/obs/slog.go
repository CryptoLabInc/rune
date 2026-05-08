// Package obs — observability: slog handler with sensitive-data redaction
// + per-request id helpers.
//
// The redacting handler wraps another slog.Handler and rewrites every
// string-typed attribute value through SensitivePatterns before
// dispatch. Two patterns are compiled (Python parity —
// mcp/server/server.py:L25-40 _SensitiveFilter):
//
//  1. token-shaped strings starting with sk- / pk- / api_ / envector_ /
//     evt_ followed by 10+ identifier chars
//  2. labelled secrets like `token:`, `key=`, `secret "`, `password ` —
//     case-insensitive — followed by a separator and 20+ identifier
//     chars
//
// Replacement keeps the first 8 characters of the match plus "***" so
// log readers retain enough prefix to disambiguate without exposing
// the body. Log lines stay grep-able for the unredacted prefix.
//
// Spec: docs/v04/spec/components/rune-mcp.md §Observability.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"regexp"
)

// SensitivePatterns — runtime-compiled. Order matters only for callers
// that introspect (none today); both apply on every string.
//
// The labelled-secret pattern uses `\b` so substrings inside a longer
// identifier (mykey, keystore, keyboard, tokenizer) are not mistakenly
// treated as the bare `key` / `token` label and over-redacted.
var SensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(sk-|pk-|api_|envector_|evt_)[a-zA-Z0-9_-]{10,}`),
	regexp.MustCompile(`(?i)\b(token|key|secret|password)["\s:=]+[a-zA-Z0-9_-]{20,}`),
}

// redact applies every SensitivePatterns regex, replacing each match
// with `match[:8] + "***"`. Strings shorter than 8 runes get the full
// "***" — preserving prefix would leak the secret.
func redact(s string) string {
	for _, re := range SensitivePatterns {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			if len(m) <= 8 {
				return "***"
			}
			return m[:8] + "***"
		})
	}
	return s
}

// NewHandler wraps an inner slog.Handler with sensitive-data redaction.
// Falls back to a stderr text handler at the given level when inner is
// nil.
func NewHandler(inner slog.Handler, level slog.Level) slog.Handler {
	if inner == nil {
		inner = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	return &filteringHandler{inner: inner}
}

// filteringHandler walks every Record's attrs and rewrites string
// values via redact, then forwards to inner. It also rewrites the
// Record's Message itself (capture/recall payload sometimes lands
// there during incident-time logging).
type filteringHandler struct {
	inner slog.Handler
}

func (h *filteringHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *filteringHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = redact(r.Message)

	// Rebuild attrs through the redactor. We construct a new Record so
	// the original (which may be cached upstream) stays untouched.
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h *filteringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	scrubbed := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		scrubbed[i] = redactAttr(a)
	}
	return &filteringHandler{inner: h.inner.WithAttrs(scrubbed)}
}

func (h *filteringHandler) WithGroup(name string) slog.Handler {
	return &filteringHandler{inner: h.inner.WithGroup(name)}
}

// redactAttr rewrites string-valued attrs and recurses into groups.
// Non-string scalar kinds (int / bool / float / time / duration) pass
// through untouched — secrets only appear as strings in this codebase.
func redactAttr(a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, redact(a.Value.String()))
	case slog.KindGroup:
		group := a.Value.Group()
		out := make([]slog.Attr, len(group))
		for i, g := range group {
			out[i] = redactAttr(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	case slog.KindLogValuer:
		// slog.Logger does not pre-resolve LogValuer when a wrapping
		// Handler sits above the leaf — the Resolve happens inside the
		// inner Handler.Handle, after our filter. So secrets wrapped in
		// LogValuer would otherwise leak. Resolve here, then recurse so
		// the resolved value (may be string / group / any) flows back
		// through the same redactor.
		return redactAttr(slog.Attr{Key: a.Key, Value: a.Value.Resolve()})
	case slog.KindAny:
		// Stringer values may hold secrets too. Best-effort: a panicking
		// Stringer must not take the whole logger down — log lines are
		// for incident response, not the source of new incidents. The
		// recover scope is per-attr so one bad attr only loses its own
		// redaction, not the rest of the record.
		if v := a.Value.Any(); v != nil {
			if s, ok := v.(fmt.Stringer); ok {
				if str, ok := safeStringer(s); ok {
					return slog.String(a.Key, redact(str))
				}
			}
		}
	}
	return a
}

// safeStringer returns s.String() with panics swallowed. If String
// panics, ok=false and the caller should leave the attr unredacted —
// dropping the attr entirely would be worse than logging it raw.
func safeStringer(s fmt.Stringer) (out string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			out = ""
			ok = false
		}
	}()
	return s.String(), true
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

// NewRequestID returns a UUID v4 string. crypto/rand is used so multiple
// MCP servers across different processes can't collide; the cost
// (~1µs per call) is negligible vs the request itself. Falls back to
// "req-anon" when crypto/rand is unavailable — logging an anonymous
// request beats panicking on a broken host RNG.
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-anon"
	}
	// RFC 4122 v4: clear/set version + variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32]
}
