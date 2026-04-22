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
	"sync/atomic"
	"time"
)

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

// RunBootLoop — background goroutine. Calls Vault.GetPublicKey with exp backoff
// until success, then SetState(Active). Stays alive for entire process to
// re-enter waiting_for_vault if Vault dies.
//
// TODO:
//  1. state = Starting
//  2. loop: call deps.vault.GetPublicKey
//     success → cache keys (disk + memory), init envector, state = Active, return
//     fail → state = WaitingForVault, log, sleep backoff[min(attempt, len-1)], retry
//  3. every attempt=20, log "persistent failure — check config"
func RunBootLoop(ctx context.Context, m *Manager /*, deps *Deps*/) {
	// TODO: bit-identical to rune-mcp.md runBootLoop pseudocode
	_ = ctx
	_ = m
}
