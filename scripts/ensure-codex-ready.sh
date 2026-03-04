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
  echo "[rune] ensuring local runtime via bootstrap-mcp.sh"
  SETUP_ONLY=1 "$PLUGIN_DIR/scripts/bootstrap-mcp.sh"
fi

echo "[rune] ready"
