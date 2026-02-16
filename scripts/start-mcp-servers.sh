#!/bin/bash
set -e

# Start Rune MCP Servers
# This script starts the enVector MCP server locally
# Note: Vault MCP runs on a remote server deployed by team admin

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
    echo "Please run: /rune:configure"
    exit 1
fi

# Activate virtual environment
if [ ! -d "$PLUGIN_DIR/.venv" ]; then
    print_error "Virtual environment not found. Please run install.sh first."
    exit 1
fi

source "$PLUGIN_DIR/.venv/bin/activate"

# Check if MCP server exists
if [ ! -f "$PLUGIN_DIR/mcp/server/server.py" ]; then
    print_error "MCP server not found at mcp/server/server.py. Please reinstall."
    exit 1
fi

# Check if MCP server is already running
if pgrep -f "mcp/server/server.py" > /dev/null; then
    print_warn "enVector MCP server is already running"
    echo "PID: $(pgrep -f 'mcp/server/server.py')"
else
    print_info "Starting enVector MCP server..."

    cd "$PLUGIN_DIR"
    PYTHONPATH="$PLUGIN_DIR/mcp" nohup "$PLUGIN_DIR/.venv/bin/python3" mcp/server/server.py > "$LOG_DIR/envector-mcp.log" 2>&1 &
    ENVECTOR_PID=$!

    # Wait a moment and check if it's still running
    sleep 2
    if ps -p $ENVECTOR_PID > /dev/null; then
        print_info "enVector MCP server started (PID: $ENVECTOR_PID)"
        echo "  Log: $LOG_DIR/envector-mcp.log"
    else
        print_error "enVector MCP server failed to start"
        echo "Check logs at: $LOG_DIR/envector-mcp.log"
        exit 1
    fi
fi

print_info "Local MCP server is running"
echo ""
echo "Note: Vault MCP runs on remote server (configured in ~/.rune/config.json)"
echo "      Claude connects to it via SSE at your team's Vault URL"
echo ""
echo "To view logs:"
echo "  tail -f $LOG_DIR/envector-mcp.log"
echo ""
echo "To stop server:"
echo "  pkill -f 'mcp/server/server.py'"
echo ""
echo "Next: Restart Claude to connect to MCP servers"
