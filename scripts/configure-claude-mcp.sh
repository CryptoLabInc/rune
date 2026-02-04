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

# Merge with existing config using jq or Python fallback
merge_json_with_python() {
    python3 - "$CLAUDE_CONFIG" "$TEMP_CONFIG" << 'PYEOF'
import json
import sys

def deep_merge(base, overlay):
    """Recursively merge overlay into base."""
    for key, value in overlay.items():
        if key in base and isinstance(base[key], dict) and isinstance(value, dict):
            deep_merge(base[key], value)
        else:
            base[key] = value
    return base

if len(sys.argv) != 3:
    print("Error: Expected exactly 2 file path arguments (config_file, template_file)", file=sys.stderr)
    sys.exit(1)

config_file = sys.argv[1]
template_file = sys.argv[2]

try:
    with open(config_file, 'r') as f:
        base = json.load(f)
except (json.JSONDecodeError, FileNotFoundError):
    base = {}

try:
    with open(template_file, 'r') as f:
        overlay = json.load(f)
except (json.JSONDecodeError, FileNotFoundError):
    print("Error: Failed to read template config", file=sys.stderr)
    sys.exit(1)

merged = deep_merge(base, overlay)

with open(config_file, 'w') as f:
    json.dump(merged, f, indent=2)

print("✓ MCP servers configured successfully")
PYEOF
}

if command -v jq &> /dev/null; then
    # Use jq if available
    jq -s '.[0] * .[1]' "$CLAUDE_CONFIG" "$TEMP_CONFIG" > "$CLAUDE_CONFIG.tmp"
    mv "$CLAUDE_CONFIG.tmp" "$CLAUDE_CONFIG"
    echo "✓ MCP servers configured successfully"
elif command -v python3 &> /dev/null; then
    # Fallback: use Python for JSON merging
    merge_json_with_python
else
    echo "Error: Neither jq nor python3 found. Cannot merge JSON configuration."
    echo "Please install jq (recommended) or python3."
    rm "$TEMP_CONFIG"
    exit 1
fi

rm "$TEMP_CONFIG"

echo ""
echo "Claude MCP configuration updated."
echo "Please restart Claude Code or Claude Desktop to activate the servers."
echo ""
echo "MCP servers added:"
echo "  - rune-vault (Vault MCP for key management)"
echo "  - envector (enVector MCP for encrypted vectors)"
