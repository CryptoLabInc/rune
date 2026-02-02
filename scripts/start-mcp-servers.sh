#!/bin/bash
set -e

# Start Rune MCP Servers
# This script starts the Vault MCP server in the background

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$HOME/.rune/logs"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_info() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Create log directory
mkdir -p "$LOG_DIR"

# Check if config exists
if [ ! -f "$HOME/.rune/config.json" ]; then
    print_error "Configuration not found at ~/.rune/config.json"
    echo "Please run: /rune configure"
    exit 1
fi

# Activate virtual environment
if [ ! -d "$PLUGIN_DIR/.venv" ]; then
    print_error "Virtual environment not found. Please run install.sh first."
    exit 1
fi

source "$PLUGIN_DIR/.venv/bin/activate"

# Check if Vault MCP is already running
if pgrep -f "vault_mcp.py" > /dev/null; then
    print_warn "Vault MCP server is already running"
    echo "PID: $(pgrep -f vault_mcp.py)"
else
    print_info "Starting Vault MCP server..."

    cd "$PLUGIN_DIR/mcp/vault"
    nohup python3 vault_mcp.py > "$LOG_DIR/vault-mcp.log" 2>&1 &
    VAULT_PID=$!

    # Wait a moment and check if it's still running
    sleep 2
    if ps -p $VAULT_PID > /dev/null; then
        print_info "Vault MCP server started (PID: $VAULT_PID)"
        echo "  Log: $LOG_DIR/vault-mcp.log"
    else
        print_error "Vault MCP server failed to start"
        echo "Check logs at: $LOG_DIR/vault-mcp.log"
        exit 1
    fi
fi

# TODO: Start envector-mcp-server when available
# For now, envector-mcp-server should be installed separately

print_info "MCP servers are running"
echo ""
echo "To view logs:"
echo "  tail -f $LOG_DIR/vault-mcp.log"
echo ""
echo "To stop servers:"
echo "  pkill -f vault_mcp.py"
echo ""
echo "Next: Restart Claude to connect to MCP servers"
