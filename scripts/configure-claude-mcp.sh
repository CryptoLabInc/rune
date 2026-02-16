#!/bin/bash
set -e

# Configure Claude MCP Servers
# Registers Rune's enVector MCP server with Claude Code and Claude Desktop.
#
# Claude Code:   Uses `claude mcp add --scope user` (official CLI)
# Claude Desktop: JSON deep merge into claude_desktop_config.json

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

CONFIGURED=0

# ============================================================================
# 1. Claude Code CLI — `claude mcp add --scope user`
# ============================================================================
register_claude_code() {
    # Unset CLAUDECODE to allow running inside a Claude Code session
    local CLAUDE_CMD
    CLAUDE_CMD=$(CLAUDECODE= command -v claude 2>/dev/null || true)

    if [ -z "$CLAUDE_CMD" ]; then
        print_warn "claude CLI not found in PATH — skipping Claude Code registration"
        return 1
    fi

    # Remove stale entries first
    for name in envector rune-vault envector-mcp-server; do
        CLAUDECODE= "$CLAUDE_CMD" mcp remove --scope user "$name" 2>/dev/null || true
    done

    # Read Vault credentials from config.json (single source of truth)
    local RUNE_CONFIG="$HOME/.rune/config.json"
    local VAULT_ENV_FLAGS=()
    if [ -f "$RUNE_CONFIG" ]; then
        local vault_ep vault_tk
        vault_ep=$(python3 -c "import json; c=json.load(open('$RUNE_CONFIG')); print(c.get('vault',{}).get('endpoint',''))" 2>/dev/null || true)
        vault_tk=$(python3 -c "import json; c=json.load(open('$RUNE_CONFIG')); print(c.get('vault',{}).get('token',''))" 2>/dev/null || true)
        [ -n "$vault_ep" ] && VAULT_ENV_FLAGS+=(-e "RUNEVAULT_ENDPOINT=$vault_ep")
        [ -n "$vault_tk" ] && VAULT_ENV_FLAGS+=(-e "RUNEVAULT_TOKEN=$vault_tk")
    fi

    # Register envector MCP server (name must precede -e to avoid variadic parsing)
    CLAUDECODE= "$CLAUDE_CMD" mcp add envector \
        --scope user --transport stdio \
        -e ENVECTOR_CONFIG="$HOME/.rune/config.json" \
        -e ENVECTOR_AUTO_KEY_SETUP=false \
        -e PYTHONPATH="$PLUGIN_DIR/mcp" \
        "${VAULT_ENV_FLAGS[@]}" \
        -- "$PLUGIN_DIR/.venv/bin/python3" \
        "$PLUGIN_DIR/mcp/server/server.py" \
        --mode stdio

    print_info "MCP server registered in Claude Code (user scope)"
    CONFIGURED=$((CONFIGURED + 1))
}

register_claude_code

# ============================================================================
# 2. Claude Desktop — JSON deep merge (no CLI available)
# ============================================================================

# Read Vault credentials for Desktop config
RUNE_CONFIG="$HOME/.rune/config.json"
DESKTOP_VAULT_EP=""
DESKTOP_VAULT_TK=""
if [ -f "$RUNE_CONFIG" ]; then
    DESKTOP_VAULT_EP=$(python3 -c "import json; c=json.load(open('$RUNE_CONFIG')); print(c.get('vault',{}).get('endpoint',''))" 2>/dev/null || true)
    DESKTOP_VAULT_TK=$(python3 -c "import json; c=json.load(open('$RUNE_CONFIG')); print(c.get('vault',{}).get('token',''))" 2>/dev/null || true)
fi

# Build MCP config JSON for Desktop
TEMP_CONFIG=$(mktemp)

if [ -f "$PLUGIN_DIR/.claude/mcp_servers.template.json" ]; then
    sed -e "s|PLUGIN_DIR|$PLUGIN_DIR|g" \
        -e "s|USER_HOME|$HOME|g" \
        -e "s|RUNEVAULT_ENDPOINT_VALUE|$DESKTOP_VAULT_EP|g" \
        -e "s|RUNEVAULT_TOKEN_VALUE|$DESKTOP_VAULT_TK|g" \
        "$PLUGIN_DIR/.claude/mcp_servers.template.json" > "$TEMP_CONFIG"
else
    # Build vault env entries
    VAULT_ENV_JSON=""
    [ -n "$DESKTOP_VAULT_EP" ] && VAULT_ENV_JSON="$VAULT_ENV_JSON
        \"RUNEVAULT_ENDPOINT\": \"$DESKTOP_VAULT_EP\","
    [ -n "$DESKTOP_VAULT_TK" ] && VAULT_ENV_JSON="$VAULT_ENV_JSON
        \"RUNEVAULT_TOKEN\": \"$DESKTOP_VAULT_TK\","

    cat > "$TEMP_CONFIG" <<TMPL
{
  "mcpServers": {
    "envector": {
      "command": "$PLUGIN_DIR/.venv/bin/python3",
      "args": ["$PLUGIN_DIR/mcp/server/server.py", "--mode", "stdio"],
      "env": {
        "ENVECTOR_CONFIG": "$HOME/.rune/config.json",
        "ENVECTOR_AUTO_KEY_SETUP": "false",${VAULT_ENV_JSON}
        "PYTHONPATH": "$PLUGIN_DIR/mcp"
      },
      "description": "enVector MCP server for encrypted vector operations"
    }
  }
}
TMPL
fi

merge_into_config() {
    local config_file="$1"
    local label="$2"

    if [ ! -f "$config_file" ]; then
        return 1
    fi

    python3 -c "
import json, sys

def deep_merge(base, overlay):
    for key, value in overlay.items():
        if key in base and isinstance(base[key], dict) and isinstance(value, dict):
            deep_merge(base[key], value)
        else:
            base[key] = value
    return base

base_file = '$config_file'
overlay_file = '$TEMP_CONFIG'

try:
    with open(base_file, 'r') as f:
        base = json.load(f)
except (json.JSONDecodeError, FileNotFoundError):
    base = {}

with open(overlay_file, 'r') as f:
    overlay = json.load(f)

# Remove stale rune entries not present in overlay
rune_keys = ['rune-vault', 'envector', 'envector-mcp-server']
overlay_servers = set(overlay.get('mcpServers', {}).keys())
for key in rune_keys:
    if key in base.get('mcpServers', {}) and key not in overlay_servers:
        del base['mcpServers'][key]

merged = deep_merge(base, overlay)

with open(base_file, 'w') as f:
    json.dump(merged, f, indent=2)
"
    print_info "MCP servers registered in $label ($config_file)"
    CONFIGURED=$((CONFIGURED + 1))
}

# Claude Desktop (macOS)
DESKTOP_MAC="$HOME/Library/Application Support/Claude/claude_desktop_config.json"
if [ -f "$DESKTOP_MAC" ] || [ -d "$HOME/Library/Application Support/Claude" ]; then
    if [ ! -f "$DESKTOP_MAC" ]; then
        mkdir -p "$(dirname "$DESKTOP_MAC")"
        echo '{"mcpServers":{}}' > "$DESKTOP_MAC"
    fi
    merge_into_config "$DESKTOP_MAC" "Claude Desktop (macOS)"
fi

# Claude Desktop (Linux)
DESKTOP_LINUX="$HOME/.config/claude/claude_desktop_config.json"
if [ -f "$DESKTOP_LINUX" ]; then
    merge_into_config "$DESKTOP_LINUX" "Claude Desktop (Linux)"
fi

rm -f "$TEMP_CONFIG"

# ============================================================================
# 3. Cleanup stale mcp_settings.json (no longer used by Claude Code)
# ============================================================================
STALE_FILE="$HOME/.claude/mcp_settings.json"
if [ -f "$STALE_FILE" ]; then
    rm -f "$STALE_FILE"
    print_info "Removed stale $STALE_FILE"
fi

# ============================================================================
# Summary
# ============================================================================
if [ "$CONFIGURED" -eq 0 ]; then
    print_warn "No Claude configuration targets found"
    echo "  Register manually: claude mcp add --scope user --transport stdio envector -- $PLUGIN_DIR/.venv/bin/python3 $PLUGIN_DIR/mcp/server/server.py --mode stdio"
fi

echo ""
echo "Please restart Claude Code or Claude Desktop to activate the MCP server."
echo ""
echo "MCP servers registered:"
echo "  - envector (enVector MCP for encrypted vectors)"
