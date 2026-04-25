// Command rune-mcp is a session-local MCP server ported from Python rune v0.3.x
// (agent-delegated path only — see docs/v04/overview/architecture.md §Scope).
//
// Spawn model: Claude Code launches one instance per session via stdio.
// Lifecycle: starting → waiting_for_vault → active ↔ dormant.
// Tools: 8 MCP tools (capture, recall, batch_capture, capture_history,
//        delete_capture, vault_status, diagnostics, reload_pipelines).
//
// Phase A (current): MCP handshake + tools/list only. All 8 handlers return
// "not yet implemented" CallToolResult. RunBootLoop · Vault · envector ·
// embedder are not wired. Phase 4-5 brings real adapters + service logic.
//
// Python reference: mcp/server/server.py (2002 LoC)
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/envector/rune-go/internal/mcp"
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

	// Phase A: empty Deps. RunBootLoop / config.Load / adapter wiring deferred.
	deps := &mcp.Deps{}

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
