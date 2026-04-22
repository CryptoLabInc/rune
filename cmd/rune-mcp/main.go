// Command rune-mcp is a session-local MCP server ported from Python rune v0.3.x
// (agent-delegated path only — see docs/v04/overview/architecture.md §Scope).
//
// Spawn model: Claude Code launches one instance per session via stdio.
// Lifecycle: starting → waiting_for_vault → active ↔ dormant.
// Tools: 8 MCP tools (capture, recall, batch_capture, capture_history,
//        delete_capture, vault_status, diagnostics, reload_pipelines).
//
// Python reference: mcp/server/server.py (2002 LoC)
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO Phase 1: config.Load("~/.rune/config.json") — 3-section schema
	// TODO Phase 2: lifecycle.RunBootLoop(ctx, deps) — Vault.GetPublicKey retry
	// TODO Phase 3: mcp.NewServer + RegisterTools + Serve(stdio)
	// TODO Phase 4: graceful shutdown (30s) on stdin EOF / SIGTERM
	//
	// Python reference: server.py:main() + RunMCPServer lifecycle.

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Println("rune-mcp skeleton — not yet implemented")

	select {
	case <-ctx.Done():
	case <-sigCh:
		cancel()
	}
}
