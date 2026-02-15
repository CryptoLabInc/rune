#!/bin/bash
set -e

# Rune Plugin Installer v0.3.0
# Orchestrates scripts/ to produce the same end-state as /plugin install
#
# Usage:
#   ./install.sh              # Interactive installation
#   ./install.sh --uninstall  # Remove Rune completely

VERSION="0.3.0"
RUNE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

print_step() {
    echo -e "\n${BLUE}▸${NC} $1\n"
}

# ============================================================================
# Uninstall
# ============================================================================
if [ "$1" = "--uninstall" ]; then
    if [ -f "$RUNE_ROOT/scripts/uninstall.sh" ]; then
        exec bash "$RUNE_ROOT/scripts/uninstall.sh"
    else
        print_error "scripts/uninstall.sh not found"
        exit 1
    fi
fi

# ============================================================================
# Main Installation
# ============================================================================
print_header "Rune Plugin Installer v${VERSION}"

echo "FHE-encrypted organizational memory for teams"
echo ""

# ----------------------------------------------------------------------------
# Step 1: Delegate to scripts/install.sh (venv + deps + ~/.rune/ directory)
# ----------------------------------------------------------------------------
print_step "Step 1/4: Setting up Python environment and dependencies..."

if [ ! -f "$RUNE_ROOT/scripts/install.sh" ]; then
    print_error "scripts/install.sh not found. Is this the Rune plugin directory?"
    exit 1
fi

bash "$RUNE_ROOT/scripts/install.sh"
print_info "Python environment ready"

# ----------------------------------------------------------------------------
# Step 2: Collect credentials (interactive) and create ~/.rune/config.json
# ----------------------------------------------------------------------------
print_step "Step 2/4: Configure team credentials..."

CONFIG_FILE="$HOME/.rune/config.json"
mkdir -p "$HOME/.rune"
chmod 700 "$HOME/.rune"

# Check for existing config
if [ -f "$CONFIG_FILE" ]; then
    print_warn "Existing configuration found at $CONFIG_FILE"
    read -p "Overwrite? (y/n): " OVERWRITE
    if [ "$OVERWRITE" != "y" ]; then
        print_info "Keeping existing configuration"
        # Read existing values for .env generation
        ENVECTOR_ADDRESS=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('envector',{}).get('endpoint',''))" 2>/dev/null || echo "")
        ENVECTOR_API_KEY=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('envector',{}).get('api_key',''))" 2>/dev/null || echo "")
        RUNEVAULT_ENDPOINT=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('vault',{}).get('url',''))" 2>/dev/null || echo "")
        RUNEVAULT_TOKEN=$(python3 -c "import json; c=json.load(open('$CONFIG_FILE')); print(c.get('vault',{}).get('token',''))" 2>/dev/null || echo "")
    fi
fi

if [ ! -f "$CONFIG_FILE" ] || [ "$OVERWRITE" = "y" ]; then
    echo "Enter team credentials (provided by your admin):"
    echo ""

    # enVector Cloud (required)
    read -p "  enVector Cloud Address (e.g., cluster-xxx.envector.io): " ENVECTOR_ADDRESS
    if [ -z "$ENVECTOR_ADDRESS" ]; then
        print_error "enVector Address is required"
        exit 1
    fi

    read -p "  enVector API Key: " ENVECTOR_API_KEY
    if [ -z "$ENVECTOR_API_KEY" ]; then
        print_error "enVector API Key is required"
        exit 1
    fi

    # Rune-Vault (optional)
    echo ""
    echo "  Rune-Vault credentials (optional — skip to configure later via /rune configure):"
    read -p "  Rune-Vault URL (press Enter to skip): " RUNEVAULT_ENDPOINT
    RUNEVAULT_TOKEN=""
    if [ -n "$RUNEVAULT_ENDPOINT" ]; then
        read -p "  Rune-Vault Token: " RUNEVAULT_TOKEN
    fi

    # Determine initial state
    INITIAL_STATE="dormant"

    # Generate ~/.rune/config.json
    python3 -c "
import json
from datetime import datetime

config = {
    'vault': {
        'url': '$RUNEVAULT_ENDPOINT',
        'token': '$RUNEVAULT_TOKEN'
    },
    'envector': {
        'endpoint': '$ENVECTOR_ADDRESS',
        'api_key': '$ENVECTOR_API_KEY',
        'collection': 'rune-context'
    },
    'state': '$INITIAL_STATE',
    'metadata': {
        'configVersion': '1.0',
        'lastUpdated': datetime.now().isoformat(),
        'installedFrom': '$RUNE_ROOT'
    }
}

with open('$CONFIG_FILE', 'w') as f:
    json.dump(config, f, indent=2)
"
    chmod 600 "$CONFIG_FILE"
    print_info "Configuration saved to $CONFIG_FILE"

    if [ -z "$RUNEVAULT_ENDPOINT" ]; then
        print_warn "Vault not configured — plugin starts in dormant state"
        echo "    Configure later with: /rune configure"
    fi
fi

# ----------------------------------------------------------------------------
# Step 3: Generate mcp/.env (server runtime config)
# ----------------------------------------------------------------------------
print_step "Step 3/4: Generating MCP server configuration..."

MCP_ENV_FILE="$RUNE_ROOT/mcp/.env"

cat > "$MCP_ENV_FILE" <<EOF
# Generated by install.sh v${VERSION}
# MCP Server Configuration
MCP_SERVER_NAME="envector_mcp_server"

# enVector Cloud
ENVECTOR_ADDRESS="$ENVECTOR_ADDRESS"
ENVECTOR_API_KEY="$ENVECTOR_API_KEY"

# enVector Options
ENVECTOR_KEY_ID="mcp_key"
ENVECTOR_KEY_PATH="./keys"
ENVECTOR_EVAL_MODE="rmp"
ENVECTOR_ENCRYPTED_QUERY="false"
ENVECTOR_AUTO_KEY_SETUP="false"

# Rune-Vault Integration
RUNEVAULT_ENDPOINT="$RUNEVAULT_ENDPOINT"
RUNEVAULT_TOKEN="$RUNEVAULT_TOKEN"

# Embedding Configuration
EMBEDDING_MODE="femb"
EMBEDDING_MODEL="sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
EOF

print_info "MCP server .env generated"

# ----------------------------------------------------------------------------
# Step 4: Register as Claude Code plugin
# ----------------------------------------------------------------------------
print_step "Step 4/4: Registering with Claude Code..."

MARKETPLACE_DIR="$(cd "$RUNE_ROOT/.." && pwd)"
MARKETPLACE_NAME="rune-local"
PLUGIN_KEY="rune@${MARKETPLACE_NAME}"

# Find claude CLI
CLAUDE_BIN=""
if command -v claude &>/dev/null; then
    CLAUDE_BIN="claude"
elif [ -x "$HOME/.local/bin/claude" ]; then
    CLAUDE_BIN="$HOME/.local/bin/claude"
fi

if [ -n "$CLAUDE_BIN" ]; then
    # Verify marketplace.json exists at parent directory
    if [ ! -f "$MARKETPLACE_DIR/.claude-plugin/marketplace.json" ]; then
        print_warn "marketplace.json not found at $MARKETPLACE_DIR/.claude-plugin/"
        print_warn "Skipping plugin registration. Install manually:"
        echo "    claude plugin marketplace add $MARKETPLACE_DIR"
        echo "    claude plugin install $PLUGIN_KEY --scope user"
    else
        # 4a. Add marketplace (remove first if already added, for idempotency)
        if "$CLAUDE_BIN" plugin marketplace list 2>/dev/null | grep -q "$MARKETPLACE_NAME"; then
            "$CLAUDE_BIN" plugin marketplace remove "$MARKETPLACE_NAME" 2>/dev/null || true
        fi
        "$CLAUDE_BIN" plugin marketplace add "$MARKETPLACE_DIR" 2>&1 && \
            print_info "Marketplace '${MARKETPLACE_NAME}' added" || \
            print_warn "Failed to add marketplace (may already exist)"

        # 4b. Install plugin (idempotent — reinstalls if already present)
        "$CLAUDE_BIN" plugin install "$PLUGIN_KEY" --scope user 2>&1 && \
            print_info "Plugin '${PLUGIN_KEY}' installed" || \
            print_error "Failed to install plugin"
    fi
else
    print_warn "Claude Code CLI not found. Install the plugin manually after installing Claude Code:"
    echo "    claude plugin marketplace add $MARKETPLACE_DIR"
    echo "    claude plugin install $PLUGIN_KEY --scope user"
fi

# 4c. Register MCP in Claude Desktop (for Desktop app users)
if [ -f "$RUNE_ROOT/scripts/configure-claude-mcp.sh" ]; then
    bash "$RUNE_ROOT/scripts/configure-claude-mcp.sh"
fi

# ============================================================================
# Summary
# ============================================================================
print_header "Installation Complete!"

echo "  Config    : $CONFIG_FILE"
echo "  Venv      : $RUNE_ROOT/.venv"
echo "  MCP .env  : $MCP_ENV_FILE"
echo "  Plugin    : $PLUGIN_KEY"
echo ""

if [ -z "$RUNEVAULT_ENDPOINT" ]; then
    echo "  State: Dormant (Vault not configured)"
    echo ""
    echo "  Next steps:"
    echo "    1. Restart Claude Code to load the plugin"
    echo "    2. Use /rune:configure to set Vault credentials"
    echo "    3. Use /rune:activate to enable"
else
    echo "  State: Dormant (run /rune:activate to enable)"
    echo ""
    echo "  Next steps:"
    echo "    1. Restart Claude Code to load the plugin"
    echo "    2. Use /rune:activate"
fi

echo ""
echo "  To uninstall:"
echo "    ./install.sh --uninstall"
echo ""

print_info "Setup complete! Restart Claude Code to use the Rune plugin."
