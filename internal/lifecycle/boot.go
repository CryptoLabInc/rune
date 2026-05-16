// Package lifecycle manages rune-mcp boot sequence + state machine.
// Spec: docs/v04/spec/components/rune-mcp.md §부팅 시퀀스 + §상태 머신.
// Python: mcp/server/server.py main() + _init_pipelines + RunMCPServer.
//
// State machine:
//
//	(spawn) → starting ──(Vault OK)──→ active ←──┐
//	              ↓                      ↓       │
//	              └─(Vault fail)→ waiting_for_vault │
//	                                     ↕       │
//	                                /rune:deactivate
//	                                     ↕       │
//	                                   dormant ──┘
//	                                /rune:activate
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/envector/rune-go/internal/adapters/config"
	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/keymanager"
	"github.com/envector/rune-go/internal/adapters/vault"
	"github.com/envector/rune-go/internal/domain"
)

// BootAdapterInjector decouples lifecycle from mcp.Deps to break the
// adapter ↔ handler import cycle. The boot loop pushes adapter clients +
// per-token Vault bundle metadata through this interface; the concrete
// implementation (mcp.Deps) propagates them onto the 3 service structs.
type BootAdapterInjector interface {
	InjectVault(client vault.Client)
	InjectEmbedder(client embedder.Client)
	InjectEnvector(client envector.Client)
	ApplyVaultBundle(bundle *vault.Bundle)
}

// State — atomic-safe enum.
type State int32

const (
	StateStarting State = iota
	StateWaitingForVault
	StateActive
	StateDormant
)

func (s State) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateWaitingForVault:
		return "waiting_for_vault"
	case StateActive:
		return "active"
	case StateDormant:
		return "dormant"
	}
	return "unknown"
}

// Manager — atomic state + Vault boot loop control.
type Manager struct {
	state       atomic.Int32
	lastError   atomic.Value // string — free-form, kept for slog parity
	lastBootErr atomic.Value // *domain.BootError — structured, surfaced via diagnostics
	attempts    atomic.Int32
	onReload    atomic.Value // func()
}

// NewManager — initial state = Starting.
func NewManager() *Manager {
	m := &Manager{}
	m.state.Store(int32(StateStarting))
	// Seed lastBootErr with a typed nil so atomic.Value.Load returns a
	// consistent type after the first SetBootError call (atomic.Value
	// requires all stored values to be the same concrete type).
	m.lastBootErr.Store((*domain.BootError)(nil))
	return m
}

// Current — atomic load.
func (m *Manager) Current() State {
	return State(m.state.Load())
}

// SetState — atomic store.
func (m *Manager) SetState(s State) {
	m.state.Store(int32(s))
}

// SetReloadFunc installs the callback that respawns the boot loop. main.go
// wires this so service.LifecycleService.ReloadPipelines can ask for a
// fresh attempt without taking a circular import dependency on
// lifecycle.RunBootLoop.
//
// The callback should spawn a fresh RunBootLoop goroutine bound to the
// long-lived ctx + the same Deps (BootAdapterInjector). Manager itself
// does not invoke the callback unless Retrigger is called and state is
// Dormant — see Retrigger.
func (m *Manager) SetReloadFunc(f func()) {
	m.onReload.Store(f)
}

// Retrigger respawns the boot loop only if no loop is currently running and
// only one caller wins when called concurrently
// Transitioning Active to Starting (or Dormant to Starting) atomically claims
// right to spawn RunBootLoop
func (m *Manager) Retrigger() {
	v := m.onReload.Load()
	if v == nil {
		return
	}

	f, ok := v.(func())
	if !ok || f == nil {
		return
	}

	// Atomically claim the right to spawn. Losers fall through and return.
	if !m.state.CompareAndSwap(int32(StateActive), int32(StateStarting)) &&
		!m.state.CompareAndSwap(int32(StateDormant), int32(StateStarting)) {
		return
	}
	f()
}

// LastError reports the most recent transient failure recorded by the boot
// loop (empty string when none). Used by diagnostics tools.
func (m *Manager) LastError() string {
	v := m.lastError.Load()
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// LastBootError reports the structured boot error from the most recent boot
// attempt (nil when boot is currently active / has not yet been attempted /
// has explicitly been cleared on success). Surfaced via
// service.LifecycleService.Diagnostics so agents can fast-fail on a stable
// Kind + Hint instead of pattern-matching LastError() strings.
func (m *Manager) LastBootError() *domain.BootError {
	v := m.lastBootErr.Load()
	if v == nil {
		return nil
	}
	be, _ := v.(*domain.BootError)
	return be
}

// SetBootError stores a classified boot error in atomic state AND appends
// it to the on-disk boot log (~/.rune/logs/boot.log). nil is treated as
// "clear" — atomic is reset, log is left intact (past attempts remain
// inspectable).
func (m *Manager) SetBootError(be *domain.BootError) {
	// atomic.Value requires consistent concrete type — store typed nil
	// rather than an untyped nil interface{} for the clear case.
	if be == nil {
		m.lastBootErr.Store((*domain.BootError)(nil))
		return
	}
	m.lastBootErr.Store(be)
	// Best-effort persist. PersistBootError swallows file errors so this
	// can never break the boot loop.
	PersistBootError(be)
}

// Attempts reports the cumulative retry count from the most recent boot run
// (reset to 0 each time RunBootLoop starts). Exposed for diagnostics.
func (m *Manager) Attempts() int {
	return int(m.attempts.Load())
}

// BootBackoffs — Python server.py Vault retry schedule.
// Total to cap: 1s → 2s → 5s → 15s → 30s → 60s (then loop at 60s).
var BootBackoffs = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

// DefaultKeyDim is the FHE slot dimension matching the Qwen3-Embedding-0.6B
// production deployment (spec/components/embedder.md §불변 계약). The Vault
// manifest does not currently carry a dim field; once embedder.Info is
// available end-to-end, the boot loop should source dim from there instead.
const DefaultKeyDim = 1024

// bootResult is the outcome of one bootOnce attempt.
type bootResult int

const (
	// bootRetry — transient failure (Vault unreachable, network blip, partial
	// init error). Caller should backoff and try again.
	bootRetry bootResult = iota

	// bootActive — full success: Vault dialed, manifest parsed, keys persisted,
	// adapters wired, services injected. Caller should set StateActive and exit.
	bootActive

	// bootDormant — terminal: config missing, config.State="dormant", or vault
	// endpoint/token unconfigured. Retrying won't help — only /rune:configure
	// (or a process restart) will. Caller should set StateDormant and exit;
	// service.LifecycleService.ReloadPipelines is responsible for re-spawning
	// RunBootLoop after the user fixes config.
	bootDormant
)

// RunBootLoop drives the boot sequence per spec/components/rune-mcp.md §부팅
// 시퀀스. It runs to completion (Active or Dormant terminal) then returns.
// Re-init after dormant↔active transitions is the responsibility of
// service.LifecycleService.ReloadPipelines (which spawns a fresh RunBootLoop
// goroutine).
//
// Failure modes:
//   - config.json missing             → terminal Dormant (await /rune:configure)
//   - config.State="dormant"          → terminal Dormant (user explicit)
//   - vault endpoint/token empty      → terminal Dormant (await /rune:configure)
//   - vault dial / GetAgentManifest   → state=WaitingForVault, exp backoff retry
//   - keymanager / embedder / envector init → exp backoff retry (might be
//     transient — daemon down, etc.)
//   - other config error (parse fail) → exp backoff retry (user might be editing)
//   - ctx cancellation                → return immediately
//
// Every attempt that fails after a successful Vault dial closes the partial
// adapter conns it created (vault, embedder, envector) before retrying so
// gRPC connections do not leak across retries.
func RunBootLoop(ctx context.Context, m *Manager, deps BootAdapterInjector) {
	m.SetState(StateStarting)
	m.attempts.Store(0)
	// Don't clear lastBootErr here — keep the previous run's error visible
	// until this run produces a new outcome. That way a manual /rune:status
	// during the first ~second of a Retrigger still shows context.

	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		m.attempts.Store(int32(attempt))

		switch bootOnce(ctx, m, deps, attempt) {
		case bootActive:
			m.SetState(StateActive)
			m.lastError.Store("")
			m.SetBootError(nil)
			m.attempts.Store(int32(attempt))
			slog.Info("boot: pipelines initialized and active")
			return

		case bootDormant:
			// State + lastError + lastBootErr already set inside bootOnce.
			m.attempts.Store(int32(attempt))
			slog.Info("boot: dormant — awaiting /rune:configure or /rune:reload_pipelines",
				"reason", m.LastError())
			return

		case bootRetry:
			if attempt > 0 && attempt%20 == 0 {
				slog.Error("boot: persistent failure — check config or network",
					"attempt", attempt,
					"last_error", m.LastError())
			}
			sleepBackoff(ctx, attempt)
			attempt++
		}
	}
}

// bootOnce runs one boot attempt. Returns:
//   - bootActive  on full success
//   - bootDormant on terminal config-side failures (caller should not retry)
//   - bootRetry   on transient failures (caller backs off and retries)
//
// On any failure path, state + lastError are updated. On post-Vault-dial
// failures the partially-constructed adapter conns are closed before return
// to avoid gRPC connection leak.
func bootOnce(ctx context.Context, m *Manager, deps BootAdapterInjector, attempt int) bootResult {
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Fresh install — config.json not provisioned. Retrying won't help;
			// user must run /rune:configure first. Persist the dormant state
			// to config.json so the next boot picks up the same reason
			// (Python parity: server.py _set_dormant_with_reason).
			m.SetState(StateDormant)
			m.lastError.Store("config.json not found — run /rune:configure to set up")
			m.SetBootError(ClassifyDormantReason("not_configured"))
			if dErr := config.MarkDormant("not_configured"); dErr != nil {
				slog.Warn("boot: failed to persist dormant state to config.json", "err", dErr)
			}
			slog.Warn("boot: config.json not found — entering dormant",
				"hint", "run /rune:configure")
			return bootDormant
		}
		// Other config errors (JSON parse, permission denied, etc.) — could be
		// transient (user editing the file). Retry.
		m.lastError.Store(fmt.Sprintf("config load: %v", err))
		m.SetBootError(ClassifyBootError(err, BootErrCtx{
			Phase:    domain.BootPhaseConfigLoad,
			Attempts: attempt,
		}))
		slog.Error("boot: failed to load config", "err", err)
		return bootRetry
	}

	// Anything other than config.State == "active" is treated as dormant:
	//   - "dormant"        — user explicitly deactivated (or a previous boot
	//                         persisted dormant via MarkDormant)
	//   - ""               — fresh install or hand-edited config without state
	//   - other / unknown  — corrupted config
	//
	// Python parity: server.py:L1544 — `if rune_config.state != "active":
	// return result`. Strict check covers all non-active values uniformly.
	// /rune:activate transitions config.State back to "active" and re-spawns
	// RunBootLoop.
	if cfg.State != "active" {
		m.SetState(StateDormant)

		var reason string
		switch cfg.State {
		case "dormant":
			reason = cfg.DormantReason
			if reason == "" {
				reason = "user_deactivated"
			}
		case "":
			reason = "not_configured"
		default:
			reason = "invalid_state"
		}

		m.lastError.Store("dormant: " + reason)
		m.SetBootError(ClassifyDormantReason(reason))
		if dErr := config.MarkDormant(reason); dErr != nil {
			slog.Warn("boot: failed to persist dormant state to config.json", "err", dErr)
		}
		slog.Info("boot: state != active — staying dormant",
			"config.state", cfg.State,
			"reason", reason)
		return bootDormant
	}

	if cfg.Vault.Endpoint == "" || cfg.Vault.Token == "" {
		// Config exists but Vault credentials are missing. Same UX as missing
		// config — user must run /rune:configure. No retry. Persist to disk
		// so the next boot picks up the same dormant_reason.
		m.SetState(StateDormant)
		m.lastError.Store("vault endpoint or token missing in config — run /rune:configure")
		m.SetBootError(ClassifyDormantReason("vault_unconfigured"))
		if dErr := config.MarkDormant("vault_unconfigured"); dErr != nil {
			slog.Warn("boot: failed to persist dormant state to config.json", "err", dErr)
		}
		slog.Warn("boot: vault endpoint/token missing — entering dormant",
			"hint", "run /rune:configure")
		return bootDormant
	}

	// classify — helper closure that classifies an error with the current
	// phase + interpolates the user's endpoint/CA path into the hint. Pulled
	// out so each error site stays a single readable line.
	classify := func(err error, phase domain.BootPhase) *domain.BootError {
		return ClassifyBootError(err, BootErrCtx{
			Phase:         phase,
			VaultEndpoint: cfg.Vault.Endpoint,
			VaultCAPath:   cfg.Vault.CACert,
			Attempts:      attempt,
		})
	}

	vaultClient, err := vault.NewClient(cfg.Vault.Endpoint, cfg.Vault.Token, vault.ClientOpts{
		CACertPath: cfg.Vault.CACert,
		TLSDisable: cfg.Vault.TLSDisable,
	})
	if err != nil {
		m.SetState(StateWaitingForVault)
		m.lastError.Store(fmt.Sprintf("vault dial: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseVaultDial))
		slog.Error("boot: failed to connect to vault", "err", err)
		return bootRetry
	}

	bundle, err := vaultClient.GetAgentManifest(ctx)
	if err != nil {
		m.SetState(StateWaitingForVault)
		m.lastError.Store(fmt.Sprintf("vault get manifest: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseVaultManifest))
		slog.Warn("boot: waiting for vault...", "err", err)
		_ = vaultClient.Close()
		return bootRetry
	}

	if err := keymanager.SaveEncKey(bundle.KeyID, bundle.EncKey); err != nil {
		m.lastError.Store(fmt.Sprintf("save EncKey: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseKeySave))
		slog.Error("boot: failed to save keys to disk", "err", err)
		_ = vaultClient.Close()
		return bootRetry
	}

	embedderClient, err := embedder.New(embedder.ResolveSocketPath(""))
	if err != nil {
		m.lastError.Store(fmt.Sprintf("embedder dial: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseEmbedderDial))
		slog.Error("boot: failed to connect to embedder", "err", err)
		_ = vaultClient.Close()
		return bootRetry
	}

	keyDir, err := keymanager.KeyDir(bundle.KeyID)
	if err != nil {
		m.lastError.Store(fmt.Sprintf("resolve key dir: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseKeySave))
		slog.Error("boot: failed to resolve key dir", "err", err)
		_ = vaultClient.Close()
		_ = embedderClient.Close()
		return bootRetry
	}

	envectorClient, err := envector.NewClient(envector.ClientConfig{
		Endpoint:  bundle.EnvectorEndpoint,
		APIKey:    bundle.EnvectorAPIKey,
		KeyPath:   keyDir,
		KeyID:     bundle.KeyID,
		KeyDim:    DefaultKeyDim,
		IndexName: bundle.IndexName,
	})
	if err != nil {
		m.lastError.Store(fmt.Sprintf("envector new client: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseEnvectorInit))
		slog.Error("boot: failed to connect to envector", "err", err)
		_ = vaultClient.Close()
		_ = embedderClient.Close()
		return bootRetry
	}

	if err := envectorClient.OpenIndex(ctx); err != nil {
		m.lastError.Store(fmt.Sprintf("envector open index: %v", err))
		m.SetBootError(classify(err, domain.BootPhaseEnvectorIndex))
		slog.Error("boot: envector index activation failed", "err", err)
		_ = vaultClient.Close()
		_ = embedderClient.Close()
		_ = envectorClient.Close()
		return bootRetry
	}

	deps.InjectVault(vaultClient)
	deps.InjectEmbedder(embedderClient)
	deps.InjectEnvector(envectorClient)
	deps.ApplyVaultBundle(bundle)

	return bootActive
}

// sleepBackoff sleeps for BootBackoffs[min(attempt, len-1)] but returns
// early if ctx is cancelled.
func sleepBackoff(ctx context.Context, attempt int) {
	idx := attempt
	if idx >= len(BootBackoffs) {
		idx = len(BootBackoffs) - 1
	}
	select {
	case <-time.After(BootBackoffs[idx]):
	case <-ctx.Done():
	}
}
