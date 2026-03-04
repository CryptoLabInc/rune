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
    echo "  Run: /rune:configure"
    exit 1
fi

print_check "Configuration file found"

# Extract Vault URL from config (basic check, assumes JSON is valid)
RUNEVAULT_ENDPOINT=$(grep -o '"endpoint"[[:space:]]*:[[:space:]]*"[^"]*"' "$HOME/.rune/config.json" | head -1 | sed 's/.*"\(.*\)".*/\1/')

if [ -z "$RUNEVAULT_ENDPOINT" ]; then
    print_fail "Vault URL not found in configuration"
    exit 1
fi

print_check "Vault URL: $RUNEVAULT_ENDPOINT"

# Check if Vault is accessible
echo "Checking Vault connectivity..."
if [[ "$RUNEVAULT_ENDPOINT" =~ ^tcp:// ]]; then
    # TCP endpoint — extract host and port, use /dev/tcp
    VAULT_HOST=$(echo "$RUNEVAULT_ENDPOINT" | sed 's|tcp://||' | cut -d: -f1)
    VAULT_PORT=$(echo "$RUNEVAULT_ENDPOINT" | sed 's|tcp://||' | cut -d: -f2)
    if timeout 5 bash -c "echo > /dev/tcp/$VAULT_HOST/$VAULT_PORT" 2>/dev/null; then
        print_check "Vault is accessible (TCP $VAULT_HOST:$VAULT_PORT)"
    else
        print_fail "Vault is not accessible (TCP $VAULT_HOST:$VAULT_PORT)"
        echo "  Make sure Rune-Vault is deployed and running"
        echo "  URL: $RUNEVAULT_ENDPOINT"
        exit 1
    fi
elif [[ "$RUNEVAULT_ENDPOINT" =~ ^https?:// ]]; then
    # HTTP/HTTPS endpoint — use curl health check
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
else
    print_warn "Unknown Vault endpoint scheme: $RUNEVAULT_ENDPOINT, skipping connectivity check"
fi

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
echo "  /rune:configure"
exit 0
