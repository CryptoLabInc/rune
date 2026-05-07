// Command rune-mcp is a session-local MCP server ported from Python rune v0.3.x
// (agent-delegated path only — see docs/v04/overview/architecture.md §Scope).
//
// Spawn model: Claude Code launches one instance per session via stdio.
// Lifecycle: starting → waiting_for_vault → active ↔ dormant.
// Tools: 8 MCP tools (capture, recall, batch_capture, capture_history,
//        delete_capture, vault_status, diagnostics, reload_pipelines).
//
// Wiring: Deps holds a State manager + 3 services. Adapter clients (vault /
// envector / embedder) are populated on the services by the boot loop after
// Vault returns the bundle. Until boot completes, write tools fail with
// PIPELINE_NOT_READY through CheckState; read-only tools work degraded.
//
// Python reference: mcp/server/server.py (2002 LoC)
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/adapters/config"
	"github.com/envector/rune-go/internal/adapters/logio"
	"github.com/envector/rune-go/internal/lifecycle"
	"github.com/envector/rune-go/internal/mcp"
	"github.com/envector/rune-go/internal/service"
)

// version is the rune-mcp protocol version surfaced in MCP `initialize`.
// Phase A is "0.4.0-alpha" until adapters are wired.
const version = "0.4.0-alpha"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SIGINT / SIGTERM → cancel ctx → srv.Run unblocks.
	// stdin EOF (Claude window closed) also unblocks Run via the StdioTransport.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	deps := buildDeps()

	// Wire ReloadPipelines → fresh RunBootLoop. Without this, the boot
	// loop's first call to bootDormant returns and the goroutine exits;
	// LifecycleService.ReloadPipelines (called by /rune:configure on a
	// freshly-spawned MCP server with empty config) would then have no
	// way to re-trigger the loop short of process restart.
	deps.State.SetReloadFunc(func() {
		go lifecycle.RunBootLoop(ctx, deps.State, deps)
	})

	go lifecycle.RunBootLoop(ctx, deps.State, deps)

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "rune-mcp",
		Version: version,
	}, nil)

	if err := mcp.Register(srv, deps); err != nil {
		slog.Error("rune-mcp register failed", "err", err)
		os.Exit(1)
	}

	if err := srv.Run(ctx, &sdkmcp.StdioTransport{}); err != nil && !isNormalShutdown(err) {
		slog.Error("rune-mcp serve error", "err", err)
		os.Exit(1)
	}
}

// isNormalShutdown reports whether err corresponds to expected stdio teardown.
// The SDK's `Connection.Wait` filters io.EOF to nil before returning, so on
// stdin EOF Run returns nil. The only other expected exit is ctx cancel from
// SIGINT/SIGTERM, which surfaces as context.Canceled.
func isNormalShutdown(err error) bool {
	return err == nil || errors.Is(err, context.Canceled)
}

// buildDeps wires the state manager + 3 services so that handler dispatch can
// proceed immediately. Adapter clients (vault.Client, embedder.Client,
// envector.Client) and DEK/key state are populated by RunBootLoop once Vault
// returns the bundle — until then, the services see nil adapters and write
// tools are state-gated to PIPELINE_NOT_READY.
//
// State is shared across services so a single Manager.SetState transition
// updates the gate from every code path uniformly.
func buildDeps() *mcp.Deps {
	mgr := lifecycle.NewManager()

	// ~/.rune as the config / log root. RuneDir() returns ~/.rune (creating
	// the parent if needed); fallback to plain "$HOME/.rune" if HOME unset
	// so handler dispatch never panics during boot/handshake.
	runeDir, err := config.RuneDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		runeDir = filepath.Join(home, ".rune")
	}
	captureLog := logio.New(filepath.Join(runeDir, logio.DefaultFilename))

	cap := service.NewCaptureService()
	cap.State = mgr
	cap.CaptureLog = captureLog

	life := service.NewLifecycleService()
	life.State = mgr
	life.ConfigDir = runeDir

	return &mcp.Deps{
		State:     mgr,
		Capture:   cap,
		Recall:    service.NewRecallService(),
		Lifecycle: life,
	}
}
