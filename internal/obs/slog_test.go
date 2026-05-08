package obs

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestRedact_Tokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"sk- prefix", "auth=sk-ABCD1234567890XYZ ok", "auth=sk-ABCD1*** ok"},
		{"pk- prefix", "key pk-1234567890abc", "key pk-12345***"},
		{"api_ prefix", "use api_FOOBARBAZ12345", "use api_FOOB***"},
		{"envector_ prefix", "tok=envector_secret_12345", "tok=envector***"},
		{"evt_ prefix", "evt_AABBCCDD11223344", "evt_AABB***"},
		{"too short to redact prefix kept literal", "sk-AB", "sk-AB"},
		{"no match", "hello world 123", "hello world 123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redact(c.in); got != c.want {
				t.Fatalf("redact(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRedact_LabeledSecrets(t *testing.T) {
	cases := []struct{ in, want string }{
		{"token: ABCDEFGHIJKLMNOPQRSTUVWX", "token: A***"},
		{`{"key":"ABCDEFGHIJKLMNOPQRSTUVWX"}`, `{"key":"AB***"}`},
		{"SECRET=ABCDEFGHIJKLMNOPQRSTUVWX", "SECRET=A***"},
		{"Password \"ABCDEFGHIJKLMNOPQRSTUVWX\"", "Password***\""},
		{"token: short", "token: short"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := redact(c.in); got != c.want {
				t.Fatalf("redact(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestRedact_NoFalsePositive — substrings inside longer identifiers
// must not trigger the labelled-secret pattern. mykey/keystore/keyboard
// /tokenizer all share a substring with the bare label list but are
// not themselves secret declarations.
func TestRedact_NoFalsePositive(t *testing.T) {
	inputs := []string{
		"mykey: ABCDEFGHIJKLMNOPQRSTUVWX",
		"keystore=ABCDEFGHIJKLMNOPQRSTUVWX",
		"keyboard=ABCDEFGHIJKLMNOPQRSTUVWX",
		"tokenizer: ABCDEFGHIJKLMNOPQRSTUVWX",
		"my_key=ABCDEFGHIJKLMNOPQRSTUVWX",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			got := redact(in)
			if got != in {
				t.Fatalf("redact(%q) modified the string to %q — false positive", in, got)
			}
		})
	}
}

// captureHandler — collects emitted records for assertion.
type captureHandler struct {
	buf     *bytes.Buffer
	handler slog.Handler
}

func newCapture() *captureHandler {
	buf := &bytes.Buffer{}
	return &captureHandler{
		buf:     buf,
		handler: slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}
}

func (c *captureHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return c.handler.Enabled(ctx, l)
}
func (c *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	return c.handler.Handle(ctx, r)
}
func (c *captureHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return &captureHandler{buf: c.buf, handler: c.handler.WithAttrs(a)}
}
func (c *captureHandler) WithGroup(n string) slog.Handler {
	return &captureHandler{buf: c.buf, handler: c.handler.WithGroup(n)}
}

func TestFilteringHandler_RedactsAttrs(t *testing.T) {
	cap := newCapture()
	logger := slog.New(NewHandler(cap, slog.LevelInfo))
	logger.Info("vault dial",
		slog.String("token", "evt_AABBCCDDEEFF112233"),
		slog.String("endpoint", "tcp://vault.example.com:50051"),
		slog.Int("attempt", 1),
	)

	out := cap.buf.String()
	if !strings.Contains(out, "evt_AABB***") {
		t.Errorf("expected redacted token in output, got: %q", out)
	}
	if strings.Contains(out, "evt_AABBCCDDEEFF112233") {
		t.Errorf("raw token should not appear in output: %q", out)
	}
	if !strings.Contains(out, "tcp://vault.example.com:50051") {
		t.Errorf("non-secret string should pass through: %q", out)
	}
	if !strings.Contains(out, "attempt=1") {
		t.Errorf("non-string attrs should pass through: %q", out)
	}
}

func TestFilteringHandler_RedactsMessage(t *testing.T) {
	cap := newCapture()
	logger := slog.New(NewHandler(cap, slog.LevelInfo))
	logger.Info("auth via sk-AAABBBCCCDDD failed")

	out := cap.buf.String()
	if !strings.Contains(out, "sk-AAABB***") {
		t.Errorf("expected redacted token in message, got: %q", out)
	}
	if strings.Contains(out, "sk-AAABBBCCCDDD") {
		t.Errorf("raw token should not appear in message: %q", out)
	}
}

func TestFilteringHandler_WithAttrs(t *testing.T) {
	cap := newCapture()
	logger := slog.New(NewHandler(cap, slog.LevelInfo))
	scoped := logger.With(slog.String("api_key", "api_ABCDEFGHIJKLMN"))
	scoped.Info("request")

	out := cap.buf.String()
	if !strings.Contains(out, "api_ABCD***") {
		t.Errorf("expected redacted attr from With(), got: %q", out)
	}
	if strings.Contains(out, "api_ABCDEFGHIJKLMN") {
		t.Errorf("raw token should not appear: %q", out)
	}
}

// secretValuer wraps a secret in a slog.LogValuer. The Resolve()
// returns a string; without KindLogValuer handling in redactAttr the
// secret would land in the inner handler unredacted.
type secretValuer string

func (s secretValuer) LogValue() slog.Value { return slog.StringValue(string(s)) }

func TestFilteringHandler_RedactsLogValuer(t *testing.T) {
	cap := newCapture()
	logger := slog.New(NewHandler(cap, slog.LevelInfo))
	logger.Info("vault dial",
		slog.Any("token", secretValuer("evt_AABBCCDDEEFF112233")),
	)

	out := cap.buf.String()
	if !strings.Contains(out, "evt_AABB***") {
		t.Errorf("expected redacted token from LogValuer, got: %q", out)
	}
	if strings.Contains(out, "evt_AABBCCDDEEFF112233") {
		t.Errorf("raw token leaked through LogValuer: %q", out)
	}
}

// panickyStringer panics from String(). The handler must not crash
// the program — losing redaction on this attr is acceptable; taking
// down the logger is not.
type panickyStringer struct{}

func (panickyStringer) String() string { panic("intentional test panic") }

func TestFilteringHandler_StringerPanicNonFatal(t *testing.T) {
	cap := newCapture()
	logger := slog.New(NewHandler(cap, slog.LevelInfo))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("logger should swallow Stringer panic, got: %v", r)
		}
	}()

	logger.Info("attempt", slog.Any("payload", panickyStringer{}))

	if cap.buf.Len() == 0 {
		t.Errorf("expected the rest of the record to still be logged after Stringer panic")
	}
}

func TestNewRequestID_Format(t *testing.T) {
	id := NewRequestID()
	// 8-4-4-4-12 hex with 4 dashes
	if len(id) != 36 {
		t.Fatalf("expected length 36, got %d (%q)", len(id), id)
	}
	for _, i := range []int{8, 13, 18, 23} {
		if id[i] != '-' {
			t.Fatalf("expected '-' at index %d, got %q (%q)", i, id[i], id)
		}
	}
	// v4 marker
	if id[14] != '4' {
		t.Fatalf("expected v4 marker at index 14, got %q (%q)", id[14], id)
	}
	// variant marker (one of 8/9/a/b)
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Fatalf("unexpected variant marker at index 19: %q (%q)", id[19], id)
	}
}

func TestNewRequestID_Unique(t *testing.T) {
	seen := make(map[string]bool, 1024)
	for i := 0; i < 1024; i++ {
		id := NewRequestID()
		if seen[id] {
			t.Fatalf("duplicate id %q at iteration %d", id, i)
		}
		seen[id] = true
	}
}

func TestRequestID_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if got := RequestID(ctx); got != "" {
		t.Errorf("expected empty RequestID for bare context, got %q", got)
	}
	id := NewRequestID()
	ctx = WithRequestID(ctx, id)
	if got := RequestID(ctx); got != id {
		t.Errorf("RequestID = %q, want %q", got, id)
	}
}
