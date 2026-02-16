#!/bin/bash
set -e

# Bootstrap and run the enVector MCP server
# Called by Claude Code plugin system — creates venv on first run, then execs the server

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV_DIR="$PLUGIN_DIR/.venv"
REQUIREMENTS="$PLUGIN_DIR/requirements.txt"
SERVER="$PLUGIN_DIR/mcp/server/server.py"

# Create venv and install deps if missing
if [ ! -f "$VENV_DIR/bin/python3" ]; then
    python3 -m venv "$VENV_DIR" >&2
    "$VENV_DIR/bin/pip" install --quiet --upgrade pip >&2
    "$VENV_DIR/bin/pip" install --quiet -r "$REQUIREMENTS" >&2
fi

# Run the MCP server (exec replaces this shell process)
export PYTHONPATH="$PLUGIN_DIR/mcp"
exec "$VENV_DIR/bin/python3" "$SERVER" --mode stdio
