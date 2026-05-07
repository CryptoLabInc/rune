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
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/envector/rune-go/internal/adapters/config"
	"github.com/envector/rune-go/internal/adapters/embedder"
	"github.com/envector/rune-go/internal/adapters/envector"
	"github.com/envector/rune-go/internal/adapters/keymanager"
	"github.com/envector/rune-go/internal/adapters/vault"
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
	state     atomic.Int32
	lastError atomic.Value // string
	attempts  atomic.Int32
}

// NewManager — initial state = Starting.
func NewManager() *Manager {
	m := &Manager{}
	m.state.Store(int32(StateStarting))
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

// RunBootLoop drives the boot sequence per spec/components/rune-mcp.md §부팅
// 시퀀스. It runs to completion of one successful boot (Vault → keys → adapters
// → state=Active), then returns. Re-init after dormant↔active transitions is
// the responsibility of service.LifecycleService.ReloadPipelines (which spawns
// a fresh RunBootLoop goroutine).
//
// Failure modes:
//   - config missing                  → state stays Starting, retry with short backoff
//   - vault endpoint/token empty      → state=WaitingForVault, retry with long backoff
//   - vault dial / GetAgentManifest   → state=WaitingForVault, exp backoff retry
//   - keymanager / envector init      → state stays Starting, exp backoff retry
//   - ctx cancellation                → return immediately
//
// Every attempt that fails after a successful Vault dial closes the partial
// adapter conns it created (vault, embedder, envector) before retrying so
// gRPC connections do not leak across retries.
func RunBootLoop(ctx context.Context, m *Manager, deps BootAdapterInjector) {
	m.SetState(StateStarting)

	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}

		ok := bootOnce(ctx, m, deps)
		if ok {
			m.SetState(StateActive)
			m.lastError.Store("")
			m.attempts.Store(int32(attempt))
			slog.Info("boot: pipelines initialized and active")
			return
		}

		if attempt > 0 && attempt%20 == 0 {
			slog.Error("boot: persistent failure — check config or network",
				"attempt", attempt,
				"last_error", m.LastError())
		}
		sleepBackoff(ctx, attempt)
		attempt++
	}
}

// bootOnce runs one boot attempt. Returns true on full success (Vault dialed,
// manifest parsed, keys persisted, adapters wired, services injected).
// On any failure, state + lastError are updated and any partially-constructed
// resources are closed before returning false.
func bootOnce(ctx context.Context, m *Manager, deps BootAdapterInjector) bool {
	cfg, err := config.Load()
	if err != nil {
		m.lastError.Store(fmt.Sprintf("config load: %v", err))
		slog.Error("boot: failed to load config", "err", err)
		return false
	}

	if cfg.Vault.Endpoint == "" || cfg.Vault.Token == "" {
		m.SetState(StateWaitingForVault)
		m.lastError.Store("vault endpoint or token is empty in config")
		slog.Warn("boot: vault endpoint or token is empty, waiting...")
		return false
	}

	vaultClient, err := vault.NewClient(cfg.Vault.Endpoint, cfg.Vault.Token, vault.ClientOpts{
		CACertPath: cfg.Vault.CACert,
		TLSDisable: cfg.Vault.TLSDisable,
	})
	if err != nil {
		m.SetState(StateWaitingForVault)
		m.lastError.Store(fmt.Sprintf("vault dial: %v", err))
		slog.Error("boot: failed to connect to vault", "err", err)
		return false
	}

	bundle, err := vaultClient.GetAgentManifest(ctx)
	if err != nil {
		m.SetState(StateWaitingForVault)
		m.lastError.Store(fmt.Sprintf("vault get manifest: %v", err))
		slog.Warn("boot: waiting for vault...", "err", err)
		_ = vaultClient.Close()
		return false
	}

	if err := keymanager.SaveEncKey(bundle.KeyID, bundle.EncKey); err != nil {
		m.lastError.Store(fmt.Sprintf("save EncKey: %v", err))
		slog.Error("boot: failed to save keys to disk", "err", err)
		_ = vaultClient.Close()
		return false
	}

	embedderClient, err := embedder.New(embedder.ResolveSocketPath(""))
	if err != nil {
		m.lastError.Store(fmt.Sprintf("embedder dial: %v", err))
		slog.Error("boot: failed to connect to embedder", "err", err)
		_ = vaultClient.Close()
		return false
	}

	keyDir, err := keymanager.KeyDir(bundle.KeyID)
	if err != nil {
		m.lastError.Store(fmt.Sprintf("resolve key dir: %v", err))
		slog.Error("boot: failed to resolve key dir", "err", err)
		_ = vaultClient.Close()
		_ = embedderClient.Close()
		return false
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
		slog.Error("boot: failed to connect to envector", "err", err)
		_ = vaultClient.Close()
		_ = embedderClient.Close()
		return false
	}

	if err := envectorClient.OpenIndex(ctx); err != nil {
		m.lastError.Store(fmt.Sprintf("envector open index: %v", err))
		slog.Error("boot: envector index activation failed", "err", err)
		_ = vaultClient.Close()
		_ = embedderClient.Close()
		_ = envectorClient.Close()
		return false
	}

	deps.InjectVault(vaultClient)
	deps.InjectEmbedder(embedderClient)
	deps.InjectEnvector(envectorClient)
	deps.ApplyVaultBundle(bundle)

	return true
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
