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

// BootAdapterInjector — breaks import cycle with mcp.Deps.
type BootAdapterInjector interface {
	InjectVault(client vault.Client)
	InjectEmbedder(client embedder.Client)
	InjectEnvector(client envector.Client)
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

func RunBootLoop(ctx context.Context, m *Manager, deps BootAdapterInjector) {
	m.SetState(StateStarting)

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg, err := config.Load()
		if err != nil {
			slog.Error("boot: failed to load config", "err", err)
			time.Sleep(BootBackoffs[0])
			continue
		}

		if cfg.Vault.Endpoint == "" || cfg.Vault.Token == "" {
			m.SetState(StateWaitingForVault)
			slog.Warn("boot: vault endpoint or token is empty, waiting...")
			time.Sleep(BootBackoffs[len(BootBackoffs)-1])
			continue
		}

		vaultOpts := vault.ClientOpts{
			CACertPath: cfg.Vault.CACert,
			TLSDisable: cfg.Vault.TLSDisable,
		}

		vaultClient, err := vault.NewClient(cfg.Vault.Endpoint, cfg.Vault.Token, vaultOpts)
		if err != nil {
			slog.Error("boot: failed to connect to vault", "err", err)
			sleepBackoff(ctx, attempt)
			attempt++
			continue
		}

		bundle, err := vaultClient.GetAgentManifest(ctx)
		if err != nil {
			m.SetState(StateWaitingForVault)
			m.lastError.Store(fmt.Sprintf("vault get manifest: %v", err))

			if attempt > 0 && attempt%20 == 0 {
				slog.Error("boot: persistent failure to reach vault - check config or network", "attempt", attempt)
			} else {
				slog.Warn("boot: waiting for vault...", "err", err)
			}
			sleepBackoff(ctx, attempt)
			attempt++

			continue
		}

		if err := keymanager.SaveKeys(bundle.KeyID, bundle.EncKey); err != nil {
			slog.Error("boot: failed to save keys to disk", "err", err)
			sleepBackoff(ctx, attempt)
			attempt++
			continue
		}

		// Embedder
		embedderClient, err := embedder.New("") // TODO: config resolution
		if err != nil {
			slog.Error("boot: failed to connect to embedder", "err", err)
			sleepBackoff(ctx, attempt)
			attempt++
			continue
		}

		// enVector SDK
		runedir, _ := config.RuneDir()
		envectorClient, err := envector.NewClient(envector.ClientConfig{
			Endpoint:  bundle.EnvectorEndpoint,
			APIKey:    bundle.EnvectorAPIKey,
			KeyPath:   runedir + "/keys",
			KeyID:     bundle.KeyID,
			IndexName: bundle.IndexName,
		})
		if err != nil {
			slog.Error("boot: failed to connect to envector", "err", err)
			sleepBackoff(ctx, attempt)
			attempt++
			continue
		}

		if err := envectorClient.OpenIndex(ctx); err != nil {
			slog.Error("boot: envector index activation failed", "err", err)
			sleepBackoff(ctx, attempt)
			attempt++
			continue
		}

		deps.InjectVault(vaultClient)
		deps.InjectEmbedder(embedderClient)
		deps.InjectEnvector(envectorClient)

		m.lastError.Store("")
		m.attempts.Store(int32(attempt))
		m.SetState(StateActive)

		slog.Info("boot: pipelines initialized and active")
		return
	}
}

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
