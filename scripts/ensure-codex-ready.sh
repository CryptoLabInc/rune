#!/bin/bash
set -euo pipefail

# Codex-only adapter:
# - Reuses bootstrap-mcp.sh as the single setup source of truth
# - Optionally registers MCP in Codex via install-codex.sh

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(cd "$SELF_DIR/.." && pwd)"

REGISTER=0
if [ "${1:-}" = "--register" ]; then
  REGISTER=1
fi

if [ "$REGISTER" -eq 1 ]; then
  # install-codex.sh already runs bootstrap via install.sh — no need to bootstrap twice.
  if ! command -v codex >/dev/null 2>&1; then
    echo "[rune] codex CLI not found; cannot register MCP automatically" >&2
    exit 1
  fi
  echo "[rune] registering MCP for Codex (includes runtime bootstrap)"
  "$PLUGIN_DIR/scripts/install-codex.sh"
else
  if [ ! -w "$PLUGIN_DIR" ]; then
    echo "[rune] plugin dir is not writable: $PLUGIN_DIR" >&2
    echo "[rune] For Codex install, allow write access to this directory and rerun." >&2
    exit 1
  fi
  echo "[rune] checking local runtime (no network install)"
  "$PLUGIN_DIR/scripts/bootstrap-mcp.sh" --local-only
fi

echo "[rune] ready"
