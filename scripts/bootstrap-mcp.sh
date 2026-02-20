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

# Self-healing: clean up incomplete fastembed model downloads
# If a previous download was interrupted, .incomplete blob files remain and
# fastembed treats the snapshot as "already cached", skipping re-download.
# This leaves model_optimized.onnx missing → ONNXRuntime crashes the server.
# Resolve the same way fastembed does: FASTEMBED_CACHE_PATH → tempfile.gettempdir()/fastembed_cache
FASTEMBED_CACHE="${FASTEMBED_CACHE_PATH:-$("$VENV_DIR/bin/python3" -c "import tempfile; print(tempfile.gettempdir())" 2>/dev/null)/fastembed_cache}"
if [ -d "$FASTEMBED_CACHE" ] && \
   find "$FASTEMBED_CACHE" -name "*.incomplete" -print -quit 2>/dev/null | grep -q .; then
    echo "[rune] Incomplete model cache detected — purging for re-download..." >&2
    for model_dir in "$FASTEMBED_CACHE"/models--*; do
        [ -d "$model_dir" ] || continue
        if find "$model_dir" -name "*.incomplete" -print -quit 2>/dev/null | grep -q .; then
            rm -rf "$model_dir"
            echo "[rune] Removed: $(basename "$model_dir")" >&2
        fi
    done
fi

# Run the MCP server (exec replaces this shell process)
export PYTHONPATH="$PLUGIN_DIR/mcp"
exec "$VENV_DIR/bin/python3" "$SERVER" --mode stdio
