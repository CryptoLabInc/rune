#!/bin/bash
set -e

# Bootstrap and run the enVector MCP server
# Called by Claude Code / Gemini CLI / Codex — creates venv on first run, then execs the server

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV_DIR="$PLUGIN_DIR/.venv"
REQUIREMENTS="$PLUGIN_DIR/requirements.txt"
SERVER="$PLUGIN_DIR/mcp/server/server.py"

# Deactivate any active venv to prevent pip shebang contamination.
# If bootstrap runs inside another venv, `pip install --upgrade pip` rewrites
# the pip shebang to point at the *outer* venv's python, causing all future
# pip installs to target the wrong site-packages.
unset VIRTUAL_ENV 2>/dev/null || true

# Create venv if missing or contaminated.
# Claude Code's plugin system may copy the repo directory (including .venv) into
# its cache. A copied venv carries pip shebangs pointing at the *original* path,
# so packages silently install into the wrong site-packages. Detect this by
# checking whether pip's shebang references our VENV_DIR; if not, nuke and rebuild.
NEED_VENV=0
if [ ! -f "$VENV_DIR/bin/python3" ]; then
    NEED_VENV=1
elif [ -f "$VENV_DIR/bin/pip" ]; then
    PIP_SHEBANG="$(head -1 "$VENV_DIR/bin/pip")"
    case "$PIP_SHEBANG" in
        *"$VENV_DIR"*) ;;  # shebang points here — OK
        *) echo "[rune] Contaminated venv detected (pip shebang: $PIP_SHEBANG) — rebuilding..." >&2
           rm -rf "$VENV_DIR"
           NEED_VENV=1 ;;
    esac
fi
if [ "$NEED_VENV" -eq 1 ]; then
    python3 -m venv "$VENV_DIR" >&2
    "$VENV_DIR/bin/python3" -m pip install --quiet --upgrade pip >&2
fi

# Install/update deps when requirements change or previous install was incomplete.
# .deps_installed stores the checksum of the last successfully installed requirements.
DEPS_STAMP="$VENV_DIR/.deps_installed"
REQ_HASH="$(md5sum "$REQUIREMENTS" 2>/dev/null | cut -d' ' -f1)"
PREV_HASH="$(cat "$DEPS_STAMP" 2>/dev/null || true)"
if [ "$REQ_HASH" != "$PREV_HASH" ]; then
    echo "[rune] Installing/updating dependencies..." >&2
    "$VENV_DIR/bin/python3" -m pip install --quiet -r "$REQUIREMENTS" >&2
    echo "$REQ_HASH" > "$DEPS_STAMP"
fi

# Self-healing: clean up incomplete fastembed model downloads
# If a previous download was interrupted, .incomplete blob files remain and
# fastembed treats the snapshot as "already cached", skipping re-download.
# This leaves model_optimized.onnx missing → ONNXRuntime crashes the server.
# Resolve the same way fastembed does: FASTEMBED_CACHE_PATH → tempfile.gettempdir()/fastembed_cache
_tmpdir="$("$VENV_DIR/bin/python3" -c "import tempfile; print(tempfile.gettempdir())" 2>/dev/null || true)"
FASTEMBED_CACHE="${FASTEMBED_CACHE_PATH:-${_tmpdir}/fastembed_cache}"
if [ -z "$FASTEMBED_CACHE" ] || [ "$FASTEMBED_CACHE" = "/fastembed_cache" ]; then
    echo "[rune] Warning: could not resolve fastembed cache path, skipping self-heal" >&2
elif [ -d "$FASTEMBED_CACHE" ] && \
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

# Setup-only mode: install venv/deps then exit (used by install.sh)
[ "${SETUP_ONLY:-}" = "1" ] && exit 0

# Run the MCP server (exec replaces this shell process)
export PYTHONPATH="$PLUGIN_DIR/mcp"
exec "$VENV_DIR/bin/python3" "$SERVER" --mode stdio
