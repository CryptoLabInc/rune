#!/bin/bash
set -e

# Rune Codex Installer
# Sets up Python dependencies and registers Rune MCP in Codex CLI.

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG_FILE="$HOME/.rune/config.json"
MCP_NAME="rune-envector"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_header() {
    echo -e "\n${BLUE}================================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================================${NC}\n"
}

print_info() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_header "Rune Installer for Codex"

if ! command -v codex >/dev/null 2>&1; then
    echo "codex CLI not found in PATH."
    echo "Install Codex first, then rerun this script."
    exit 1
fi
print_info "codex CLI detected"

# Reuse existing Python setup flow.
bash "$PLUGIN_DIR/scripts/install.sh"

# Optional Vault env passthrough if config is present.
VAULT_ENDPOINT=""
VAULT_TOKEN=""
if [ -f "$CONFIG_FILE" ]; then
    VAULT_ENDPOINT=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('vault',{}).get('endpoint',''))" 2>/dev/null || true)
    VAULT_TOKEN=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('vault',{}).get('token',''))" 2>/dev/null || true)
fi

# Idempotent re-register.
codex mcp remove "$MCP_NAME" >/dev/null 2>&1 || true

MCP_ADD_ARGS=(
    mcp add "$MCP_NAME"
    --env "ENVECTOR_CONFIG=$CONFIG_FILE"
    --env "ENVECTOR_AUTO_KEY_SETUP=false"
    --env "PYTHONPATH=$PLUGIN_DIR/mcp"
)

if [ -n "$VAULT_ENDPOINT" ]; then
    MCP_ADD_ARGS+=(--env "RUNEVAULT_ENDPOINT=$VAULT_ENDPOINT")
fi
if [ -n "$VAULT_TOKEN" ]; then
    MCP_ADD_ARGS+=(--env "RUNEVAULT_TOKEN=$VAULT_TOKEN")
fi

MCP_ADD_ARGS+=(
    --
    "$PLUGIN_DIR/.venv/bin/python3"
    "$PLUGIN_DIR/mcp/server/server.py"
    --mode
    stdio
)

codex "${MCP_ADD_ARGS[@]}"
print_info "Registered MCP server in Codex: $MCP_NAME"

if [ ! -f "$CONFIG_FILE" ]; then
    print_warn "Config not found at $CONFIG_FILE"
    echo "  Create it via: cp $PLUGIN_DIR/config/config.template.json $CONFIG_FILE"
    echo "  Then fill vault/envector credentials."
fi

echo ""
print_info "Done. Verify with: codex mcp list"
