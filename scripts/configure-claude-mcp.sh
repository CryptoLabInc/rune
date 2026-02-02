#!/bin/bash
set -e

# Configure Claude MCP Servers
# This script updates Claude's MCP configuration to include Rune servers

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Detect Claude configuration location
if [ -f "$HOME/Library/Application Support/Claude/claude_desktop_config.json" ]; then
    CLAUDE_CONFIG="$HOME/Library/Application Support/Claude/claude_desktop_config.json"
elif [ -f "$HOME/.config/claude/claude_desktop_config.json" ]; then
    CLAUDE_CONFIG="$HOME/.config/claude/claude_desktop_config.json"
elif [ -f "$HOME/.claude/config.json" ]; then
    CLAUDE_CONFIG="$HOME/.claude/config.json"
else
    echo "Claude configuration not found."
    echo "Creating new configuration at ~/.claude/config.json"
    mkdir -p "$HOME/.claude"
    CLAUDE_CONFIG="$HOME/.claude/config.json"
    echo '{"mcpServers":{}}' > "$CLAUDE_CONFIG"
fi

echo "Updating Claude MCP configuration..."
echo "Config file: $CLAUDE_CONFIG"

# Read template and replace PLUGIN_DIR
TEMP_CONFIG=$(mktemp)
sed "s|PLUGIN_DIR|$PLUGIN_DIR|g" "$PLUGIN_DIR/.claude/mcp_servers.template.json" > "$TEMP_CONFIG"

# Merge with existing config (simple approach: add rune servers)
# TODO: Proper JSON merging for production
if command -v jq &> /dev/null; then
    # Use jq if available
    jq -s '.[0] * .[1]' "$CLAUDE_CONFIG" "$TEMP_CONFIG" > "$CLAUDE_CONFIG.tmp"
    mv "$CLAUDE_CONFIG.tmp" "$CLAUDE_CONFIG"
    echo "✓ MCP servers configured successfully"
else
    # Fallback: just append (may create duplicate keys)
    echo "Warning: jq not found. Using simple append."
    cat "$TEMP_CONFIG" >> "$CLAUDE_CONFIG"
    echo "✓ MCP configuration appended (you may need to manually clean up)"
fi

rm "$TEMP_CONFIG"

echo ""
echo "Claude MCP configuration updated."
echo "Please restart Claude Code or Claude Desktop to activate the servers."
echo ""
echo "MCP servers added:"
echo "  - rune-vault (Vault MCP for key management)"
echo "  - envector (enVector MCP for encrypted vectors)"
