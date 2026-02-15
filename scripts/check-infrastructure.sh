#!/bin/bash

# Check Infrastructure Availability
# Returns 0 if infrastructure is ready, 1 otherwise

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_check() {
    echo -e "${GREEN}✓${NC} $1"
}

print_fail() {
    echo -e "${RED}✗${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

INFRASTRUCTURE_READY=0

# Check if config exists
if [ ! -f "$HOME/.rune/config.json" ]; then
    print_fail "Configuration not found at ~/.rune/config.json"
    echo "  Run: /rune configure"
    exit 1
fi

print_check "Configuration file found"

# Extract Vault URL from config (basic check, assumes JSON is valid)
RUNEVAULT_ENDPOINT=$(grep -o '"url"[[:space:]]*:[[:space:]]*"[^"]*"' "$HOME/.rune/config.json" | head -1 | sed 's/.*"\(.*\)".*/\1/')

if [ -z "$RUNEVAULT_ENDPOINT" ]; then
    print_fail "Vault URL not found in configuration"
    exit 1
fi

print_check "Vault URL: $RUNEVAULT_ENDPOINT"

# Check if Vault is accessible
echo "Checking Vault connectivity..."
if command -v curl &> /dev/null; then
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "$RUNEVAULT_ENDPOINT/health" 2>/dev/null)

    if [ "$HTTP_CODE" = "200" ]; then
        print_check "Vault is accessible (HTTP $HTTP_CODE)"
    else
        print_fail "Vault is not accessible (HTTP $HTTP_CODE)"
        echo "  Make sure Rune-Vault is deployed and running"
        echo "  URL: $RUNEVAULT_ENDPOINT"
        exit 1
    fi
else
    print_warn "curl not found, skipping Vault connectivity check"
fi

# Check if local MCP server is running
if pgrep -f "mcp/server/server.py" > /dev/null; then
    print_check "enVector MCP server is running (PID: $(pgrep -f 'mcp/server/server.py'))"
else
    print_warn "enVector MCP server is not running locally"
    echo "  Start with: scripts/start-mcp-servers.sh"
    # Not failing here, as it can be started later
fi

# Note: Vault MCP runs on remote server (already checked via HTTP above)

# Check if virtual environment exists
PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -d "$PLUGIN_DIR/.venv" ]; then
    print_check "Python virtual environment found"
else
    print_fail "Virtual environment not found"
    echo "  Run: scripts/install.sh"
    exit 1
fi

print_check "Infrastructure checks passed ✓"
echo ""
echo "Infrastructure is ready. You can activate the plugin with:"
echo "  /rune configure"
exit 0
