package mcp

import (
	"strings"

	"github.com/envector/rune-go/internal/domain"
	"github.com/envector/rune-go/internal/lifecycle"
)

// State gate — called at every tool handler entry.
// Returns appropriate RuneError for non-active states (Python _ensure_pipelines).
//
// Python: server.py:L1503-1518 _ensure_pipelines.
// Recovery hints differ by internal state (rune-mcp.md §에러 처리):
//   - starting            → "Wait 1-2s and retry"
//   - waiting_for_vault   → "Last vault error: {err}. Run /rune:vault_status"
//   - dormant(user)       → "Run /rune:activate"
//   - dormant(vault)      → "Check config.vault.endpoint"
//   - dormant(envector)   → "Check network · API key"
func CheckState(m *lifecycle.Manager) error {
	switch m.Current() {
	case lifecycle.StateActive:
		return nil
	case lifecycle.StateStarting:
		return withHint(domain.ErrPipelineNotReady, "Rune is starting up. Wait 1-2 seconds and retry.")
	case lifecycle.StateWaitingForVault:
		return withHint(domain.ErrPipelineNotReady, "Waiting for Vault connection. Run /rune:vault_status for diagnostics.")
	case lifecycle.StateDormant:
		return withHint(domain.ErrPipelineNotReady, "Rune is deactivated. Run /rune:activate to re-enable.")
	}
	return domain.ErrInternal
}

func withHint(base *domain.RuneError, hint string) *domain.RuneError {
	return &domain.RuneError{
		Code:         base.Code,
		Message:      base.Message,
		Retryable:    base.Retryable,
		RecoveryHint: hint,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Input validation (Phase 2 entries)
// ─────────────────────────────────────────────────────────────────────────────

// ValidateCaptureRequest — Python server.py:L1240-1242 (parse_llm_json check).
//   - text empty → ErrInvalidInput
//   - extracted nil → ErrInvalidInput ("Invalid extracted JSON — could not parse")
func ValidateCaptureRequest(req *domain.CaptureRequest) error {
	if strings.TrimSpace(req.Text) == "" {
		return domain.ErrInvalidInput
	}
	if req.Extracted == nil {
		return domain.ErrInvalidInput
	}
	return nil
}

// ValidateRecallArgs — Python server.py:L910-932.
//   - query empty → ErrInvalidInput (D24 early reject)
//   - topk > 10 → ErrInvalidInput
//   - topk == 0 → default 5
func ValidateRecallArgs(args *domain.RecallArgs) error {
	if strings.TrimSpace(args.Query) == "" {
		return domain.ErrInvalidInput
	}
	if args.TopK == 0 {
		args.TopK = 5
	}
	if args.TopK > 10 {
		return domain.ErrInvalidInput
	}
	return nil
}

// TruncateTitle — D3. 60-rune truncate (UTF-8 aware).
func TruncateTitle(s string) string {
	runes := []rune(s)
	if len(runes) > domain.MaxTitleLen {
		runes = runes[:domain.MaxTitleLen]
	}
	return string(runes)
}

// ClampConfidence — [0.0, 1.0].
func ClampConfidence(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}
