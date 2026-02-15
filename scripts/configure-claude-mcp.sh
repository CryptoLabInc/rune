#!/bin/bash
set -e

# Configure Claude MCP Servers
# This script updates Claude's MCP configuration to include Rune servers
# Supports both Claude Desktop and Claude Code CLI
#
# Key behaviors:
#   - Uses .venv/bin/python3 (not system python) so dependencies resolve
#   - Only registers rune-vault if vault URL is actually configured
#   - Expands $HOME (not ~) for env vars so MCP servers can find config

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

# ----------------------------------------------------------------------------
# Build MCP config JSON dynamically
# ----------------------------------------------------------------------------
CONFIG_FILE="$HOME/.rune/config.json"
VAULT_URL=""

if [ -f "$CONFIG_FILE" ]; then
    VAULT_URL=$(python3 -c "
import json
try:
    c = json.load(open('$CONFIG_FILE'))
    print(c.get('vault', {}).get('url', ''))
except: pass
" 2>/dev/null || true)
fi

# Generate MCP config using the template + substitutions
TEMP_CONFIG=$(mktemp)

if [ -f "$PLUGIN_DIR/.claude/mcp_servers.template.json" ]; then
    sed -e "s|PLUGIN_DIR|$PLUGIN_DIR|g" \
        -e "s|USER_HOME|$HOME|g" \
        "$PLUGIN_DIR/.claude/mcp_servers.template.json" > "$TEMP_CONFIG"
else
    # Fallback: generate minimal config inline
    cat > "$TEMP_CONFIG" <<TMPL
{
  "mcpServers": {
    "envector": {
      "command": "$PLUGIN_DIR/.venv/bin/python3",
      "args": ["$PLUGIN_DIR/mcp/server/server.py", "--mode", "stdio"],
      "env": {
        "ENVECTOR_CONFIG": "$HOME/.rune/config.json",
        "ENVECTOR_AUTO_KEY_SETUP": "false",
        "PYTHONPATH": "$PLUGIN_DIR/mcp"
      },
      "description": "enVector MCP server for encrypted vector operations"
    }
  }
}
TMPL
    print_warn "Template not found, using fallback MCP config"
fi

# Inject rune-vault entry if vault URL is configured
if [ -n "$VAULT_URL" ]; then
    python3 -c "
import json
with open('$TEMP_CONFIG', 'r') as f:
    config = json.load(f)
config['mcpServers']['rune-vault'] = {
    'type': 'sse',
    'url': '$VAULT_URL/sse',
    'description': 'Remote Rune-Vault MCP server for FHE decryption'
}
with open('$TEMP_CONFIG', 'w') as f:
    json.dump(config, f, indent=2)
"
    print_info "Vault configured — rune-vault MCP server included"
else
    print_warn "Vault not configured — skipping rune-vault MCP server"
    echo "    Configure later with: /rune configure"
fi

# ----------------------------------------------------------------------------
# merge_into_config <config_file> <label>
# Merges TEMP_CONFIG into the given JSON config file
# ----------------------------------------------------------------------------
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
    return 0
}

# ----------------------------------------------------------------------------
# create_and_merge <config_file> <label>
# Creates the config file if needed, then merges
# ----------------------------------------------------------------------------
create_and_merge() {
    local config_file="$1"
    local label="$2"
    local dir="$(dirname "$config_file")"

    if [ ! -f "$config_file" ]; then
        mkdir -p "$dir"
        echo '{"mcpServers":{}}' > "$config_file"
    fi

    merge_into_config "$config_file" "$label"
}

CONFIGURED=0

# ---- Claude Desktop (macOS) ----
DESKTOP_MAC="$HOME/Library/Application Support/Claude/claude_desktop_config.json"
if [ -f "$DESKTOP_MAC" ]; then
    merge_into_config "$DESKTOP_MAC" "Claude Desktop (macOS)" && CONFIGURED=$((CONFIGURED + 1))
elif [ -d "$HOME/Library/Application Support/Claude" ]; then
    create_and_merge "$DESKTOP_MAC" "Claude Desktop (macOS)" && CONFIGURED=$((CONFIGURED + 1))
fi

# ---- Claude Desktop (Linux) ----
DESKTOP_LINUX="$HOME/.config/claude/claude_desktop_config.json"
if [ -f "$DESKTOP_LINUX" ]; then
    merge_into_config "$DESKTOP_LINUX" "Claude Desktop (Linux)" && CONFIGURED=$((CONFIGURED + 1))
fi

# ---- Claude Code CLI ----
CLAUDE_CODE="$HOME/.claude/mcp_settings.json"
if [ -f "$CLAUDE_CODE" ]; then
    merge_into_config "$CLAUDE_CODE" "Claude Code CLI" && CONFIGURED=$((CONFIGURED + 1))
elif [ -d "$HOME/.claude" ]; then
    create_and_merge "$CLAUDE_CODE" "Claude Code CLI" && CONFIGURED=$((CONFIGURED + 1))
fi

# ---- Fallback: nothing found ----
if [ "$CONFIGURED" -eq 0 ]; then
    print_warn "No Claude configuration directory found"
    echo "  Creating Claude Code config at $CLAUDE_CODE"
    create_and_merge "$CLAUDE_CODE" "Claude Code CLI"
fi

rm -f "$TEMP_CONFIG"

echo ""
echo "Please restart Claude Desktop or Claude Code to activate the MCP servers."
echo ""
echo "MCP servers registered:"
echo "  - envector (enVector MCP for encrypted vectors)"
if [ -n "$VAULT_URL" ]; then
    echo "  - rune-vault (Vault MCP for FHE decryption)"
fi
