package domain_test

// Tests for MakeError + RuneError vars + error code constants.
//
// Python canonical: mcp/server/errors.py (118 LoC).
// Python test baseline: NONE — Go establishes first-time coverage.
//
// MakeError is on the wire path (capture/recall responses), so format
// drift would break agent error-handling. Tests gate the JSON shape
// (`ok`, `error.{code, message, retryable, recovery_hint}`) and the
// `errors.As` unwrap path.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/envector/rune-go/internal/domain"
)

// mustErrMap — pulls and type-asserts `got["error"]` as map. Centralizes
// the panic-vs-comma-ok decision: tests should fail loudly with a clear
// message when shape drift makes the cast impossible, rather than panic
// mid-table and skip later assertions in the same function.
func mustErrMap(t *testing.T, got map[string]any) map[string]any {
	t.Helper()
	errMap, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("got[\"error\"] is %T, want map[string]any", got["error"])
	}
	return errMap
}

// MakeError wraps a *RuneError into the documented MCP error response
// shape. Python parity: errors.py:L93-118 make_error.
func TestMakeError_RuneErrorReturnsFullShape(t *testing.T) {
	err := &domain.RuneError{
		Code:         domain.CodeVaultConnection,
		Message:      "vault unreachable",
		Retryable:    true,
		RecoveryHint: "check endpoint",
	}
	got := domain.MakeError(err)

	if ok, _ := got["ok"].(bool); ok {
		t.Errorf("ok = %v, want false (RuneError must produce ok=false)", got["ok"])
	}
	errMap := mustErrMap(t, got)
	if errMap["code"] != domain.CodeVaultConnection {
		t.Errorf("code = %v, want %q", errMap["code"], domain.CodeVaultConnection)
	}
	if errMap["message"] != "vault unreachable" {
		t.Errorf("message = %v, want %q", errMap["message"], "vault unreachable")
	}
	if r, _ := errMap["retryable"].(bool); !r {
		t.Errorf("retryable = %v, want true", errMap["retryable"])
	}
	if errMap["recovery_hint"] != "check endpoint" {
		t.Errorf("recovery_hint = %v, want %q",
			errMap["recovery_hint"], "check endpoint")
	}
}

// recovery_hint OMITTED when empty. Python parity: errors.py:L106-108
// `if hint: result["error"]["recovery_hint"] = hint` — only set when truthy.
func TestMakeError_RuneErrorOmitsEmptyRecoveryHint(t *testing.T) {
	err := &domain.RuneError{
		Code:      domain.CodeInvalidInput,
		Message:   "bad input",
		Retryable: false,
		// RecoveryHint left empty
	}
	got := domain.MakeError(err)
	errMap := mustErrMap(t, got)

	if _, present := errMap["recovery_hint"]; present {
		t.Errorf("recovery_hint should be ABSENT when empty; got %v",
			errMap["recovery_hint"])
	}
}

// generic (non-RuneError) error → fallback INTERNAL_ERROR shape.
// Python parity: errors.py:L110-118 — fallback branch.
func TestMakeError_GenericErrorWrapsAsInternal(t *testing.T) {
	err := errors.New("something exploded")
	got := domain.MakeError(err)
	errMap := mustErrMap(t, got)

	if errMap["code"] != domain.CodeInternal {
		t.Errorf("code = %v, want %q", errMap["code"], domain.CodeInternal)
	}
	if errMap["message"] != "something exploded" {
		t.Errorf("message = %v, want %q", errMap["message"], "something exploded")
	}
	if r, _ := errMap["retryable"].(bool); r {
		t.Errorf("retryable = true, want false (INTERNAL_ERROR is non-retryable)")
	}
	if _, present := errMap["recovery_hint"]; present {
		t.Errorf("recovery_hint should be absent for generic err; got %v",
			errMap["recovery_hint"])
	}
}

// `fmt.Errorf("ctx: %w", runeErr)` must be detected via errors.As — the
// MakeError implementation uses errors.As(err, &runeErr), so a wrapped
// RuneError is treated as the inner code, not as INTERNAL_ERROR.
//
// **Go-specific behavioral enhancement, NOT Python parity.** Python
// errors.py:L97 uses `isinstance(exc, RuneError)` which does NOT unwrap
// `__cause__` — a Python-side wrapped RuneError would fall to the
// INTERNAL_ERROR branch. Go's errors.As walks the Unwrap() chain,
// preserving the typed code through wrapping context. This is the
// bridge between deep adapter call sites that wrap errors for context
// and the wire response that should still surface the typed code.
func TestMakeError_WrappedRuneErrorUnwrapsViaErrorsAs_GoSpecific(t *testing.T) {
	inner := &domain.RuneError{
		Code:      domain.CodeEnvectorInsert,
		Message:   "insert rejected",
		Retryable: true,
	}
	wrapped := fmt.Errorf("during capture phase 5: %w", inner)
	got := domain.MakeError(wrapped)
	errMap := mustErrMap(t, got)

	if errMap["code"] != domain.CodeEnvectorInsert {
		t.Errorf("wrapped RuneError lost code: got %v, want %q",
			errMap["code"], domain.CodeEnvectorInsert)
	}
	// The Message is taken from the inner RuneError, not the wrapper —
	// errors.As assigns the inner pointer; we then read inner.Message.
	if errMap["message"] != "insert rejected" {
		t.Errorf("message = %v, want inner message %q",
			errMap["message"], "insert rejected")
	}
}

// error code constants — these strings are the wire format. Locking each
// to its byte-exact value catches silent renames.
func TestErrorCodes_LockedToWireValues(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"internal", domain.CodeInternal, "INTERNAL_ERROR"},
		{"vault_connection", domain.CodeVaultConnection, "VAULT_CONNECTION_ERROR"},
		{"vault_decryption", domain.CodeVaultDecryption, "VAULT_DECRYPTION_ERROR"},
		{"envector_connection", domain.CodeEnvectorConnection, "ENVECTOR_CONNECTION_ERROR"},
		{"envector_insert", domain.CodeEnvectorInsert, "ENVECTOR_INSERT_ERROR"},
		{"pipeline_not_ready", domain.CodePipelineNotReady, "PIPELINE_NOT_READY"},
		{"invalid_input", domain.CodeInvalidInput, "INVALID_INPUT"},
		{"embedder_unreachable", domain.CodeEmbedderUnreachable, "EMBEDDER_UNREACHABLE"},
		{"empty_embed_text", domain.CodeEmptyEmbedText, "EMPTY_EMBED_TEXT"},
		{"extraction_missing", domain.CodeExtractionMissing, "EXTRACTION_MISSING"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q (wire format drift)", tc.name, tc.got, tc.want)
			}
		})
	}
}

// predefined RuneError vars — Code + Retryable correctness against the
// Python class-level attributes. Coverage scope is intentionally limited
// to the (Code, Retryable) tuple; Python's `recovery_hint` strings
// (errors.py:L38-L89) are NOT replicated in Go's predefined vars (which
// ship with empty Message and RecoveryHint — callers fill in per-call
// site). This test does NOT gate the hint gap; that's a deliberate
// design choice (Python: rich class hints; Go: caller-provided context).
//
// Test split:
//   - This test:                 6 vars with Python parity
//   - _GoSpecificCodes test:     3 vars with no Python equivalent
//
// Connection / insert / embedder errors are retryable (transient
// network); auth / decryption / contract violations are not. Locking
// the boolean keeps agent retry logic correct on the wire.
func TestPredefinedRuneErrors_CodeAndRetryableMatchPython(t *testing.T) {
	cases := []struct {
		name      string
		got       *domain.RuneError
		wantCode  string
		retryable bool
		pyLine    string
	}{
		// Retryable (transient).
		{"vault_connection", domain.ErrVaultConnection, domain.CodeVaultConnection, true, "errors.py:L37"},
		{"envector_connection", domain.ErrEnvectorConnection, domain.CodeEnvectorConnection, true, "errors.py:L58"},
		{"envector_insert", domain.ErrEnvectorInsert, domain.CodeEnvectorInsert, true, "errors.py:L68"},
		// Non-retryable (misconfiguration / contract violation).
		{"internal", domain.ErrInternal, domain.CodeInternal, false, "errors.py:L21"},
		{"vault_decryption", domain.ErrVaultDecryption, domain.CodeVaultDecryption, false, "errors.py:L47"},
		{"pipeline_not_ready", domain.ErrPipelineNotReady, domain.CodePipelineNotReady, false, "errors.py:L78"},
		{"invalid_input", domain.ErrInvalidInput, domain.CodeInvalidInput, false, "errors.py:L88"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Code != tc.wantCode {
				t.Errorf("%s.Code = %q, want %q (Python: %s)",
					tc.name, tc.got.Code, tc.wantCode, tc.pyLine)
			}
			if tc.got.Retryable != tc.retryable {
				t.Errorf("%s.Retryable = %v, want %v (Python: %s)",
					tc.name, tc.got.Retryable, tc.retryable, tc.pyLine)
			}
		})
	}
}

// Go-specific predefined errors with no Python equivalent. Locked at Go
// semantics. EmbedderUnreachable is retryable (transient network — daemon
// restart, gRPC reconnect); ExtractionMissing and EmptyEmbedText are
// contract violations (agent must provide; caller bug → not retryable).
func TestPredefinedRuneErrors_GoSpecificCodes(t *testing.T) {
	cases := []struct {
		name      string
		got       *domain.RuneError
		wantCode  string
		retryable bool
		why       string
	}{
		{"embedder_unreachable", domain.ErrEmbedderUnreachable, domain.CodeEmbedderUnreachable, true,
			"D30 — embedder gRPC daemon transient unavailability"},
		{"empty_embed_text", domain.ErrEmptyEmbedText, domain.CodeEmptyEmbedText, false,
			"D5 — agent must produce embed text; caller bug"},
		{"extraction_missing", domain.ErrExtractionMissing, domain.CodeExtractionMissing, false,
			"D14 — agent must provide pre_extraction; caller bug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Code != tc.wantCode {
				t.Errorf("%s.Code = %q, want %q", tc.name, tc.got.Code, tc.wantCode)
			}
			if tc.got.Retryable != tc.retryable {
				t.Errorf("%s.Retryable = %v, want %v (%s)",
					tc.name, tc.got.Retryable, tc.retryable, tc.why)
			}
		})
	}
}

// RuneError satisfies the standard `error` interface via a pointer
// receiver Error() that returns Message. errors.As in MakeError uses
// reflection-based assignability checks (not pointer comparison) to
// match `&RuneError{}`-shaped targets. The test asserts both:
//
//	(1) compile-time: *RuneError is assignable to `error`
//	(2) runtime:      Error() returns the Message field unchanged
func TestRuneError_ImplementsErrorInterface(t *testing.T) {
	r := &domain.RuneError{Code: domain.CodeInternal, Message: "boom"}
	var e error = r          // (1) compile-time check
	if e.Error() != "boom" { // (2) runtime check
		t.Errorf("Error() = %q, want %q", e.Error(), "boom")
	}
}
