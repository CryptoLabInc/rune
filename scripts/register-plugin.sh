#!/bin/bash
set -e

# Register Rune as a Claude Code plugin
# Creates entries in installed_plugins.json and settings.json

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN_NAME="rune"
MARKETPLACE="local"
PLUGIN_KEY="${PLUGIN_NAME}@${MARKETPLACE}"
VERSION="0.1.0"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info() { echo -e "${GREEN}✓${NC} $1"; }
print_warn() { echo -e "${YELLOW}⚠${NC} $1"; }

CLAUDE_DIR="$HOME/.claude"
PLUGINS_DIR="$CLAUDE_DIR/plugins"
CACHE_DIR="$PLUGINS_DIR/cache/${MARKETPLACE}/${PLUGIN_NAME}/${VERSION}"
INSTALLED_FILE="$PLUGINS_DIR/installed_plugins.json"
SETTINGS_FILE="$CLAUDE_DIR/settings.json"

# ---- 1. Create cache directory with symlink ----
mkdir -p "$(dirname "$CACHE_DIR")"
if [ -L "$CACHE_DIR" ]; then
    rm "$CACHE_DIR"
fi
if [ -d "$CACHE_DIR" ]; then
    rm -rf "$CACHE_DIR"
fi
ln -s "$PLUGIN_DIR" "$CACHE_DIR"
print_info "Plugin linked: $CACHE_DIR → $PLUGIN_DIR"

# ---- 2. Register in installed_plugins.json ----
if [ ! -f "$INSTALLED_FILE" ]; then
    echo '{"version": 2, "plugins": {}}' > "$INSTALLED_FILE"
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")

python3 -c "
import json

path = '$INSTALLED_FILE'
key = '$PLUGIN_KEY'

with open(path, 'r') as f:
    data = json.load(f)

data.setdefault('version', 2)
data.setdefault('plugins', {})

data['plugins'][key] = [{
    'scope': 'user',
    'installPath': '$CACHE_DIR',
    'version': '$VERSION',
    'installedAt': '$NOW',
    'lastUpdated': '$NOW',
}]

with open(path, 'w') as f:
    json.dump(data, f, indent=2)
"
print_info "Registered in installed_plugins.json as '$PLUGIN_KEY'"

# ---- 3. Enable in settings.json ----
if [ ! -f "$SETTINGS_FILE" ]; then
    echo '{}' > "$SETTINGS_FILE"
fi

python3 -c "
import json

path = '$SETTINGS_FILE'
key = '$PLUGIN_KEY'

with open(path, 'r') as f:
    data = json.load(f)

data.setdefault('enabledPlugins', {})
data['enabledPlugins'][key] = True

with open(path, 'w') as f:
    json.dump(data, f, indent=2)
"
print_info "Enabled in settings.json: enabledPlugins['$PLUGIN_KEY'] = true"

# ---- 4. Register local marketplace if not present ----
MARKETPLACES_FILE="$PLUGINS_DIR/known_marketplaces.json"
if [ -f "$MARKETPLACES_FILE" ]; then
    python3 -c "
import json

path = '$MARKETPLACES_FILE'

with open(path, 'r') as f:
    data = json.load(f)

if 'local' not in data:
    data['local'] = {
        'source': {
            'source': 'local',
            'path': '$HOME/.claude/plugins/cache/local'
        },
        'installLocation': '$HOME/.claude/plugins/cache/local',
        'lastUpdated': '$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")'
    }
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)
    print('  Added local marketplace')
else:
    print('  Local marketplace already registered')
"
fi

echo ""
print_info "Rune plugin registered in Claude Code"
echo "  Restart Claude Code to activate."
