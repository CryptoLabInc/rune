#!/bin/bash
set -e

# Rune Plugin Uninstaller
# Removes all Rune artifacts: config, venv, MCP registrations

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Colors
RED='\033[0;31m'
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

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_header "Rune Plugin Uninstaller"

echo "This will remove:"
echo "  - ~/.rune/ (configuration and logs)"
echo "  - $PLUGIN_DIR/.venv (Python virtual environment)"
echo "  - $PLUGIN_DIR/mcp/.env (server config)"
echo "  - MCP server entries from Claude Code and Claude Desktop"
echo ""

read -p "Continue with uninstall? (y/n): " CONFIRM
if [ "$CONFIRM" != "y" ]; then
    echo "Uninstall cancelled."
    exit 0
fi

echo ""

# 1. Stop running MCP servers
if pgrep -f "mcp/server/server.py" > /dev/null 2>&1; then
    pkill -f "mcp/server/server.py" 2>/dev/null || true
    print_info "Stopped running MCP server processes"
else
    print_info "No running MCP servers found"
fi

# 2. Remove MCP entries from Claude Code (via CLI)
CLAUDE_CMD=$(CLAUDECODE= command -v claude 2>/dev/null || true)
if [ -n "$CLAUDE_CMD" ]; then
    for name in envector rune-vault envector-mcp-server; do
        CLAUDECODE= "$CLAUDE_CMD" mcp remove --scope user "$name" 2>/dev/null || true
    done
    print_info "Removed MCP entries from Claude Code (user scope)"
else
    print_warn "claude CLI not found — skipping Claude Code MCP cleanup"
fi

# 3. Remove MCP entries from Claude Desktop (JSON)
remove_desktop_mcp_entries() {
    local config_file="$1"
    local config_name="$2"

    if [ ! -f "$config_file" ]; then
        return
    fi

    python3 -c "
import json, sys

config_path = '$config_file'

try:
    with open(config_path, 'r') as f:
        config = json.load(f)
except (json.JSONDecodeError, FileNotFoundError):
    sys.exit(0)

servers = config.get('mcpServers', {})
removed = []
for key in ['rune-vault', 'envector', 'envector-mcp-server']:
    if key in servers:
        del servers[key]
        removed.append(key)

if removed:
    with open(config_path, 'w') as f:
        json.dump(config, f, indent=2)
    print(', '.join(removed))
" 2>/dev/null

    if [ $? -eq 0 ]; then
        print_info "Removed Rune entries from $config_name"
    fi
}

# Claude Desktop (macOS)
remove_desktop_mcp_entries "$HOME/Library/Application Support/Claude/claude_desktop_config.json" "Claude Desktop (macOS)"

# Claude Desktop (Linux)
remove_desktop_mcp_entries "$HOME/.config/claude/claude_desktop_config.json" "Claude Desktop (Linux)"

# 4. Cleanup stale files from previous versions
for stale in "$HOME/.claude/mcp_settings.json"; do
    if [ -f "$stale" ]; then
        rm -f "$stale"
        print_info "Removed stale $stale"
    fi
done

# 5. Remove ~/.rune/ directory
if [ -d "$HOME/.rune" ]; then
    rm -rf "$HOME/.rune"
    print_info "Removed ~/.rune/"
else
    print_info "~/.rune/ not found (already clean)"
fi

# 6. Remove .venv
if [ -d "$PLUGIN_DIR/.venv" ]; then
    rm -rf "$PLUGIN_DIR/.venv"
    print_info "Removed .venv/"
fi

# 7. Remove MCP server .env
if [ -f "$PLUGIN_DIR/mcp/.env" ]; then
    rm -f "$PLUGIN_DIR/mcp/.env"
    print_info "Removed mcp/.env"
fi

# 8. Remove legacy startup scripts (from old install.sh v0.1.0)
for legacy_script in "$PLUGIN_DIR/start-mcp-server.sh" "$PLUGIN_DIR/test-connection.sh"; do
    if [ -f "$legacy_script" ]; then
        rm -f "$legacy_script"
        print_info "Removed legacy $(basename $legacy_script)"
    fi
done

print_header "Uninstall Complete"
echo "  All Rune artifacts have been removed."
echo ""
echo "  To reinstall: ./scripts/install.sh"
echo ""
