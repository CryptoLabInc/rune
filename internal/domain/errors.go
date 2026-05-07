package domain

// RuneError — 8-code taxonomy (Python 7 bit-identical + Go-specific EMBEDDER_UNREACHABLE).
// Spec: docs/v04/spec/components/rune-mcp.md §에러 처리.
// Python: mcp/server/errors.py (118 LoC).

import "errors"

// Code enum — 8 codes.
const (
	CodeInternal            = "INTERNAL_ERROR"
	CodeVaultConnection     = "VAULT_CONNECTION_ERROR"
	CodeVaultDecryption     = "VAULT_DECRYPTION_ERROR"
	CodeEnvectorConnection  = "ENVECTOR_CONNECTION_ERROR"
	CodeEnvectorInsert      = "ENVECTOR_INSERT_ERROR"
	CodePipelineNotReady    = "PIPELINE_NOT_READY"
	CodeInvalidInput        = "INVALID_INPUT"
	CodeEmbedderUnreachable = "EMBEDDER_UNREACHABLE" // Go-specific (D30)
	CodeEmptyEmbedText      = "EMPTY_EMBED_TEXT"     // D5 — dedicated code for missing embed text
	CodeExtractionMissing   = "EXTRACTION_MISSING"   // D14 — agent must provide pre_extraction
)

// RuneError — MCP error response body (Python make_error equivalent).
type RuneError struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	RecoveryHint string `json:"recovery_hint,omitempty"`
}

func (e *RuneError) Error() string { return e.Message }

// Predefined errors (Python errors.py equivalents).
var (
	ErrInternal            = &RuneError{Code: CodeInternal, Retryable: false}
	ErrVaultConnection     = &RuneError{Code: CodeVaultConnection, Retryable: true}
	ErrVaultDecryption     = &RuneError{Code: CodeVaultDecryption, Retryable: false}
	ErrEnvectorConnection  = &RuneError{Code: CodeEnvectorConnection, Retryable: true}
	ErrEnvectorInsert      = &RuneError{Code: CodeEnvectorInsert, Retryable: true}
	ErrPipelineNotReady    = &RuneError{Code: CodePipelineNotReady, Retryable: false}
	ErrInvalidInput        = &RuneError{Code: CodeInvalidInput, Retryable: false}
	ErrEmbedderUnreachable = &RuneError{Code: CodeEmbedderUnreachable, Retryable: true}
	ErrEmptyEmbedText      = &RuneError{Code: CodeEmptyEmbedText, Retryable: false}
	ErrExtractionMissing   = &RuneError{Code: CodeExtractionMissing, Retryable: false}
)

// MakeError — Python make_error equivalent. Wraps an error as MCP response.
func MakeError(err error) map[string]any {
	var runeErr *RuneError
	if errors.As(err, &runeErr) {
		errMap := map[string]any{
			"code":      runeErr.Code,
			"message":   runeErr.Message,
			"retryable": runeErr.Retryable,
		}
		if runeErr.RecoveryHint != "" {
			errMap["recovery_hint"] = runeErr.RecoveryHint
		}
		return map[string]any{
			"ok":    false,
			"error": errMap,
		}
	}
	return map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":      CodeInternal,
			"message":   err.Error(),
			"retryable": false,
		},
	}
}
