package lifecycle

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
)

// Graceful shutdown — spec/components/rune-mcp.md §프로세스 수명 Exit sequence.
//
// Triggered by stdin EOF or SIGTERM (handled in cmd/rune-mcp/main.go).
//
// Sequence:
//  1. drain inflight tool calls (timeout ShutdownTimeout = 30s)
//  2. close adapters: envector (Keys + Index + Client) → Vault conn → embedder
//  3. zeroize DEK(s) (best-effort — not hard guarantee, GC may already copy)
//  4. return — caller os.Exit

// ShutdownTimeout — spec L22 "timeout 30s".
const ShutdownTimeout = 30 * time.Second

// InflightTracker counts active tool handler invocations.
// Handler entry: Begin(); defer End().
// Shutdown waits until Active() == 0 or timeout.
type InflightTracker struct {
	active atomic.Int32
}

// NewInflightTracker.
func NewInflightTracker() *InflightTracker { return &InflightTracker{} }

// Begin increments counter. Call at MCP handler entry.
func (t *InflightTracker) Begin() { t.active.Add(1) }

// End decrements counter. Call in defer at handler exit.
func (t *InflightTracker) End() { t.active.Add(-1) }

// Active returns current inflight count.
func (t *InflightTracker) Active() int32 { return t.active.Load() }

// Closer — all adapters (vault.Client, envector.Client, embedder.Client) satisfy this.
type Closer interface {
	Close() error
}

// GracefulShutdown orchestrates the 3-step Exit sequence.
//
//	tracker   — pass the process-wide inflight tracker (may be nil to skip drain)
//	closers   — ordered adapter close list (envector before vault recommended)
//	deks      — byte slices to zeroize (agent_dek, any local AES key caches)
func GracefulShutdown(ctx context.Context, tracker *InflightTracker, closers []Closer, deks ...[]byte) error {
	// Step 1 — drain inflight
	drainCtx, cancel := context.WithTimeout(ctx, ShutdownTimeout)
	defer cancel()
	if tracker != nil {
		_ = waitInflight(drainCtx, tracker)
	}

	// Step 2 — adapter close (best-effort; one failure should not block others)
	for _, c := range closers {
		if c == nil {
			continue
		}
		_ = c.Close()
	}

	// Step 3 — zeroize DEKs
	for _, dek := range deks {
		ZeroizeDEK(dek)
	}

	// Step 4 — flush + close the boot log file so the OS doesn't hold a
	// stale fd after exit. Idempotent; safe to call even if never opened.
	CloseBootLog()

	return nil
}

// waitInflight polls tracker.Active() with 50ms ticks until zero or ctx done.
// Returns ctx.Err() on timeout (caller logs and proceeds anyway).
func waitInflight(ctx context.Context, tracker *InflightTracker) error {
	if tracker.Active() == 0 {
		return nil
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if tracker.Active() == 0 {
				return nil
			}
		}
	}
}

// ZeroizeDEK clears a byte slice. runtime.KeepAlive prevents the compiler
// from optimizing the zeroing away (the slice "stays referenced" until after
// the KeepAlive call).
//
// This is a best-effort defense — a determined attacker with memory access
// after process death has no guarantees. GC may also have copied the data
// before this point. Ported per rune-mcp.md L24 pattern.
func ZeroizeDEK(dek []byte) {
	for i := range dek {
		dek[i] = 0
	}
	runtime.KeepAlive(dek)
}
