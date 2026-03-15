#!/bin/bash
set -e

# Register Rune as a Claude Code plugin
# Creates entries in installed_plugins.json and settings.json

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN_NAME="rune"
MARKETPLACE="cryptolab"
PLUGIN_KEY="${PLUGIN_NAME}@${MARKETPLACE}"
VERSION=$(python3 -c "import json; print(json.load(open('$PLUGIN_DIR/.claude-plugin/plugin.json'))['version'])")


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

# ---- 1. Copy plugin files into cache ----
mkdir -p "$CACHE_DIR"
# Remove stale symlink from previous versions
if [ -L "$CACHE_DIR" ]; then
    rm "$CACHE_DIR"
    mkdir -p "$CACHE_DIR"
fi
rsync -a --delete \
    --exclude '.venv' \
    --exclude '.pytest_cache' \
    --exclude '__pycache__' \
    --exclude '.DS_Store' \
    --exclude 'benchmark' \
    --exclude 'tmp' \
    "$PLUGIN_DIR/" "$CACHE_DIR/"
print_info "Plugin copied: $PLUGIN_DIR → $CACHE_DIR"

# ---- 1b. Remove stale cache directories ----
STALE_MARKETPLACES=("cryptolab-rune")

# Remove old marketplace directories entirely
for stale_mp in "${STALE_MARKETPLACES[@]}"; do
    stale_dir="$PLUGINS_DIR/cache/$stale_mp"
    if [ -d "$stale_dir" ]; then
        rm -rf "$stale_dir"
        print_info "Removed stale cache: $stale_dir"
    fi
done

# Remove old version directories under current marketplace
CURRENT_PLUGIN_CACHE="$PLUGINS_DIR/cache/${MARKETPLACE}/${PLUGIN_NAME}"
if [ -d "$CURRENT_PLUGIN_CACHE" ]; then
    for ver_dir in "$CURRENT_PLUGIN_CACHE"/*/; do
        ver_dir="${ver_dir%/}"
        ver_name="$(basename "$ver_dir")"
        if [ "$ver_name" != "$VERSION" ] && [ "$ver_name" != "*" ]; then
            rm -rf "$ver_dir"
            print_info "Removed old version cache: $ver_dir"
        fi
    done
fi

# ---- 2. Register in installed_plugins.json ----
if [ ! -f "$INSTALLED_FILE" ]; then
    echo '{"version": 2, "plugins": {}}' > "$INSTALLED_FILE"
fi

NOW=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")

python3 -c "
import json

path = '$INSTALLED_FILE'
key = '$PLUGIN_KEY'
stale_marketplaces = ['cryptolab-rune']

with open(path, 'r') as f:
    data = json.load(f)

data.setdefault('version', 2)
data.setdefault('plugins', {})

# Remove stale plugin keys (e.g. rune@cryptolab-rune)
for sm in stale_marketplaces:
    stale_key = 'rune@' + sm
    if stale_key in data['plugins']:
        del data['plugins'][stale_key]

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
stale_marketplaces = ['cryptolab-rune']

with open(path, 'r') as f:
    data = json.load(f)

data.setdefault('enabledPlugins', {})

# Remove stale enabledPlugins keys (e.g. rune@cryptolab-rune)
for sm in stale_marketplaces:
    stale_key = 'rune@' + sm
    if stale_key in data['enabledPlugins']:
        del data['enabledPlugins'][stale_key]

data['enabledPlugins'][key] = True

with open(path, 'w') as f:
    json.dump(data, f, indent=2)
"
print_info "Enabled in settings.json: enabledPlugins['$PLUGIN_KEY'] = true"

echo ""
print_info "Rune plugin registered in Claude Code"
echo "  Restart Claude Code to activate."
