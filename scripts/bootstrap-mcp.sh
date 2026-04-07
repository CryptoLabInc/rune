#!/bin/bash
set -e

# Bootstrap and run the enVector MCP server
# Called by Claude Code / Gemini CLI / Codex — creates venv on first run, then execs the server

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV_DIR="$PLUGIN_DIR/.venv"
REQUIREMENTS="$PLUGIN_DIR/requirements.txt"
SERVER="$PLUGIN_DIR/mcp/server/server.py"
MODE="run"

for arg in "$@"; do
    case "$arg" in
        --local-only) MODE="local-only" ;;
        --install-deps) MODE="install-deps" ;;
    esac
done

# Backward compatibility:
# - SETUP_ONLY=1 previously meant "prepare runtime", which installed deps.
# - Keep that behavior unless --local-only is explicitly provided.
if [ "${SETUP_ONLY:-}" = "1" ] && [ "$MODE" = "run" ]; then
    MODE="install-deps"
fi

if [ "$MODE" = "run" ]; then
    MODE="install-deps"
fi

if [ ! -w "$PLUGIN_DIR" ]; then
    echo "[rune] Plugin directory is not writable: $PLUGIN_DIR" >&2
    echo "[rune] Codex users: grant write access to ~/.codex/skills/rune, then rerun setup." >&2
    exit 1
fi

# Deactivate any active venv to prevent pip shebang contamination.
# If bootstrap runs inside another venv, `pip install --upgrade pip` rewrites
# the pip shebang to point at the *outer* venv's python, causing all future
# pip installs to target the wrong site-packages.
unset VIRTUAL_ENV 2>/dev/null || true

# Create venv if missing or contaminated.
# Claude Code's plugin system may copy the repo directory (including .venv) into
# its cache. A copied venv carries pip shebangs pointing at the *original* path,
# so packages silently install into the wrong site-packages. Detect this by
# checking whether pip's shebang references our VENV_DIR
NEED_VENV=0
if [ ! -f "$VENV_DIR/bin/python3" ]; then
    NEED_VENV=1
else
    # Detect Python version mismatch between the venv interpreter and installed packages
    _VENV_PYVER="$("$VENV_DIR/bin/python3" -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")' 2>/dev/null || true)"
    _SITE_PYVER=""
    for dir in "$VENV_DIR"/lib/python*/site-packages; do
        [ -d "$dir" ] || continue
        # Get version which site-packages installed
        if [ "$(ls -A "$dir" 2>/dev/null | head -1)" ]; then
            _SITE_PYVER="$(basename "$(dirname "$dir")")"  # e.g. python3.10
            _SITE_PYVER="${_SITE_PYVER#python}"            # e.g. 3.10
            break
        fi
    done
    if [ -n "$_VENV_PYVER" ] && [ -n "$_SITE_PYVER" ] && [ "$_VENV_PYVER" != "$_SITE_PYVER" ]; then
        echo "[rune] Python version mismatch (venv runs $_VENV_PYVER, packages built for $_SITE_PYVER) - rebuilding..." >&2
        rm -rf "$VENV_DIR"
        NEED_VENV=1
    else
        # Check pip shebangs for path consistency (same Python version, different path)
        _PIP_BIN=""
        [ -f "$VENV_DIR/bin/pip" ]  && _PIP_BIN="$VENV_DIR/bin/pip"
        [ -z "$_PIP_BIN" ] && [ -f "$VENV_DIR/bin/pip3" ] && _PIP_BIN="$VENV_DIR/bin/pip3"
        if [ -n "$_PIP_BIN" ]; then
            PIP_SHEBANG="$(head -1 "$_PIP_BIN")"
            case "$PIP_SHEBANG" in
                *"$VENV_DIR"*) ;;  # shebang points
                *) echo "[rune] Contaminated venv detected ($_PIP_BIN shebang: $PIP_SHEBANG) - rewriting shebangs..." >&2
                   _VENV_PYTHON="$VENV_DIR/bin/python3"
                   for _script in "$VENV_DIR/bin/"*; do
                       [ -f "$_script" ] || continue
                       file -b --mime "$_script" 2>/dev/null | grep -q "^text/" || continue
                       _first="$(head -1 "$_script")"
                       case "$_first" in
                           \#\!*python*)
                                if sed --version >/dev/null 2>&1; then
                                    # GNU sed (Linux)
                                    sed -i "1s|^#\!.*|#\!$_VENV_PYTHON|" "$_script"
                                else
                                    # BSD sed (macOS)
                                    sed -i '' "1s|^#\!.*|#\!$_VENV_PYTHON|" "$_script"
                                fi ;;
                       esac
                   done
                   echo "[rune] Shebangs fixed." >&2 ;;
            esac
        fi
    fi
fi
if [ "$NEED_VENV" -eq 1 ]; then
    python3 -m venv "$VENV_DIR" >&2
    if [ "$MODE" = "install-deps" ]; then
        "$VENV_DIR/bin/python3" -m pip install --quiet --upgrade pip >&2
    fi
fi

# Install/update deps when requirements change or previous install was incomplete.
# .deps_installed stores the checksum of the last successfully installed requirements.
DEPS_STAMP="$VENV_DIR/.deps_installed"
REQ_HASH="$(md5sum "$REQUIREMENTS" 2>/dev/null | cut -d' ' -f1)"
PREV_HASH="$(cat "$DEPS_STAMP" 2>/dev/null || true)"
if [ "$MODE" = "install-deps" ] && [ "$REQ_HASH" != "$PREV_HASH" ]; then
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

# Setup-only mode: prepare local runtime then exit.
[ "${SETUP_ONLY:-}" = "1" ] && exit 0

# Configuration safety net:
# If ~/.rune/config.json is missing but Vault environment variables are set, generate it.
# enVector credentials are delivered via the Vault bundle at runtime.
CONFIG_FILE="$HOME/.rune/config.json"
if [ ! -f "$CONFIG_FILE" ] && [ -n "${RUNEVAULT_ENDPOINT:-${VAULT_ENDPOINT:-}}" ] && [ -n "${RUNEVAULT_TOKEN:-${VAULT_TOKEN:-}}" ]; then
    mkdir -p "$HOME/.rune" && chmod 700 "$HOME/.rune"
    cat <<EOF > "$CONFIG_FILE"
{
  "vault": {
    "endpoint": "${RUNEVAULT_ENDPOINT:-${VAULT_ENDPOINT:-}}",
    "token": "${RUNEVAULT_TOKEN:-${VAULT_TOKEN:-}}"
  },
  "state": "dormant",
  "metadata": {
    "configVersion": "1.1",
    "lastUpdated": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  }
}
EOF
    chmod 600 "$CONFIG_FILE"
    echo "[rune] Generated config from environment variables at $CONFIG_FILE" >&2
fi
[ "$MODE" = "local-only" ] && exit 0

# Run the MCP server (exec replaces this shell process)
export PYTHONPATH="$PLUGIN_DIR/mcp"
exec "$VENV_DIR/bin/python3" "$SERVER" --mode stdio
