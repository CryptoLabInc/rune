package domain

// BootError — structured failure surfaced by the boot loop, exposed via
// diagnostics so agents (and humans) can fast-fail with a specific kind +
// hint instead of probing the system manually.
//
// Spec rationale: rune-mcp's boot loop currently logs errors and stores a
// free-form string in lifecycle.Manager.LastError(). Callers (diagnostics,
// SKILL.md flows) need a stable enum to branch on. See
// docs/v04/decisions/D??-boot-error-surface.md (planned).
//
// Leaf type: imports stdlib only. Classifier lives in internal/lifecycle.

import "time"

// BootErrorKind — stable enum string for agent/UI branching.
// Add new values only at the end to preserve schema compatibility.
type BootErrorKind string

const (
	// Catch-all for unrecognized failures. Detail contains the raw message;
	// SKILL.md should ask the user to share it with an admin.
	BootErrUnknown BootErrorKind = "unknown"

	// ── Config-side (terminal Dormant) ───────────────────────────────
	BootErrConfigMissing      BootErrorKind = "config_missing"       // ~/.rune/config.json absent
	BootErrConfigInvalid      BootErrorKind = "config_invalid"       // state=unknown / parse fail
	BootErrConfigParse        BootErrorKind = "config_parse"         // JSON parse fail
	BootErrUserDeactivated    BootErrorKind = "user_deactivated"     // state=dormant by /rune:deactivate
	BootErrVaultNotConfigured BootErrorKind = "vault_not_configured" // endpoint/token empty

	// ── Vault dial / NewClient (sync, before any RPC) ────────────────
	BootErrVaultBadEndpoint BootErrorKind = "vault_bad_endpoint" // ParseEndpoint failed
	BootErrVaultCAFile      BootErrorKind = "vault_ca_file"      // CA cert path unreadable / not PEM
	BootErrVaultDialOpts    BootErrorKind = "vault_dial_opts"    // grpc.NewClient rejected options

	// ── Vault RPC (GetAgentManifest path) ────────────────────────────
	BootErrVaultTLSHandshake BootErrorKind = "vault_tls_handshake" // x509: signed by unknown authority / expired / etc.
	BootErrVaultTLSHostname  BootErrorKind = "vault_tls_hostname"  // cert SAN does not match endpoint host
	BootErrVaultDNS          BootErrorKind = "vault_dns"           // hostname resolution failed
	BootErrVaultNetwork      BootErrorKind = "vault_network"       // TCP unreachable / refused / reset
	BootErrVaultTimeout      BootErrorKind = "vault_timeout"       // gRPC DeadlineExceeded
	BootErrVaultAuth         BootErrorKind = "vault_auth"          // gRPC Unauthenticated (bad token)
	BootErrVaultPermission   BootErrorKind = "vault_permission"    // gRPC PermissionDenied (role lacks scope)
	BootErrVaultRateLimit    BootErrorKind = "vault_rate_limit"    // gRPC ResourceExhausted
	BootErrVaultInvalidInput BootErrorKind = "vault_invalid_input" // gRPC InvalidArgument
	BootErrVaultManifest     BootErrorKind = "vault_manifest"      // Vault responded but manifest empty/invalid/unparseable
	BootErrVaultInternal     BootErrorKind = "vault_internal"      // gRPC Internal / other server-side

	// ── Post-Vault adapters ──────────────────────────────────────────
	BootErrEmbedderUnreachable BootErrorKind = "embedder_unreachable" // UDS socket missing / runed down
	BootErrEnvectorInit        BootErrorKind = "envector_init"        // envector.NewClient failed
	BootErrEnvectorIndex       BootErrorKind = "envector_index"       // OpenIndex failed
	BootErrKeySave             BootErrorKind = "key_save"             // SaveEncKey / KeyDir filesystem failure
	BootErrLocalIO             BootErrorKind = "local_io"             // generic local FS / permissions
)

// BootPhase — which step of the boot sequence produced the error.
// Useful for distinguishing same-kind errors at different phases.
type BootPhase string

const (
	BootPhaseConfigLoad    BootPhase = "config_load"
	BootPhaseConfigCheck   BootPhase = "config_check"
	BootPhaseVaultDial     BootPhase = "vault_dial"
	BootPhaseVaultManifest BootPhase = "vault_manifest"
	BootPhaseKeySave       BootPhase = "key_save"
	BootPhaseEmbedderDial  BootPhase = "embedder_dial"
	BootPhaseEnvectorInit  BootPhase = "envector_init"
	BootPhaseEnvectorIndex BootPhase = "envector_index"
)

// BootError — surfaced via diagnostics.vault.last_boot_error.
//
// JSON shape (stable contract for SKILL.md / agents):
//
//	{
//	  "kind":     "vault_tls_handshake",
//	  "detail":   "rpc error: code = Unavailable desc = ... x509: ...",
//	  "hint":     "CA cert at /Users/.../ca.pem does not verify server cert from tcp://X. Re-fetch the current CA from your Vault admin.",
//	  "phase":    "vault_manifest",
//	  "at":       "2026-05-16T10:09:23Z",
//	  "attempts": 4
//	}
//
// Hint is interpolated with concrete values (endpoint, ca path) so the agent
// can relay it to the user verbatim. Detail is raw text for human debugging
// and the unknown-kind fallback path.
type BootError struct {
	Kind     BootErrorKind `json:"kind"`
	Detail   string        `json:"detail"`
	Hint     string        `json:"hint,omitempty"`
	Phase    BootPhase     `json:"phase,omitempty"`
	At       time.Time     `json:"at"`
	Attempts int           `json:"attempts,omitempty"`
}

// Retryable — true when blind retry (no user action) has a reasonable chance
// of succeeding. False when the user must change something first (re-issue
// token, fix CA cert, edit config, etc.).
//
// The boot loop itself may still retry on bootRetry results regardless of
// this flag — Retryable is for SKILL.md / UI to decide whether to suggest
// "wait + recheck" vs "fix the underlying issue before recheck."
func (e *BootError) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	// Won't self-heal without user action.
	case BootErrConfigMissing,
		BootErrConfigInvalid,
		BootErrConfigParse,
		BootErrUserDeactivated,
		BootErrVaultNotConfigured,
		BootErrVaultBadEndpoint,
		BootErrVaultCAFile,
		BootErrVaultDialOpts,
		BootErrVaultTLSHandshake,
		BootErrVaultTLSHostname,
		BootErrVaultAuth,
		BootErrVaultPermission,
		BootErrVaultInvalidInput,
		BootErrVaultManifest:
		return false
	default:
		// Transient: network blips, DNS, timeouts, rate limits, daemon down
		// while restarting, etc.
		return true
	}
}
