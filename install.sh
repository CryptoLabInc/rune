#!/bin/bash
set -e

# Rune Plugin Installer (Agent-Agnostic)
# Works with Claude, Gemini, Codex, and any MCP-compatible AI agent

VERSION="0.1.0"

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

check_python() {
    if ! command -v python3.12 &> /dev/null; then
        print_error "Python 3.12 is not installed"
        echo "Rune requires Python 3.12 for pyenvector compatibility"
        echo ""
        echo "Installation:"
        echo "  - macOS: brew install python@3.12"
        echo "  - Linux: sudo apt install python3.12 python3.12-venv"
        exit 1
    fi

    PYTHON_VERSION=$(python3.12 --version | cut -d' ' -f2)
    print_info "Python $PYTHON_VERSION detected"
}

setup_mcp_server() {
    print_step "Setting up envector-mcp-server..."

    cd mcp/envector-mcp-server

    # Create virtual environment with Python 3.12
    if [ ! -d ".venv" ]; then
        print_info "Creating Python 3.12 virtual environment..."
        python3.12 -m venv .venv
    else
        print_info "Virtual environment already exists"
    fi

    # Activate venv
    source .venv/bin/activate

    # Install dependencies
    print_info "Installing dependencies (pyenvector from CryptoLab PyPI)..."
    pip install --quiet --upgrade pip

    # Install pyenvector from CryptoLab PyPI
    print_info "Installing pyenvector 1.2.2..."
    pip install --quiet \
        pyenvector==1.2.2 \
        --index-url https://pypi.org/simple \
        --extra-index-url https://pypi.cryptolab.co.kr/simple

    # Install other dependencies
    print_info "Installing FastMCP and other packages..."
    pip install --quiet -r requirements.txt

    print_info "Dependencies installed successfully!"

    cd ../..
}

configure_credentials() {
    print_step "Configuring team credentials..."

    MCP_DIR="mcp/envector-mcp-server"
    ENV_FILE="$MCP_DIR/.env"

    # Check if .env already exists
    if [ -f "$ENV_FILE" ]; then
        print_warn ".env file already exists"
        read -p "Overwrite? (y/n): " OVERWRITE
        if [ "$OVERWRITE" != "y" ]; then
            print_info "Keeping existing configuration"
            return
        fi
    fi

    echo "Enter team credentials (provided by your admin):"
    echo ""

    # enVector Cloud
    read -p "enVector Cloud Address (e.g., cluster-xxx.envector.io:443): " ENVECTOR_ADDRESS
    read -p "enVector API Key: " ENVECTOR_API_KEY

    # Rune-Vault (optional)
    echo ""
    read -p "Rune-Vault Endpoint (optional, press Enter to skip): " RUNEVAULT_ENDPOINT
    if [ -n "$RUNEVAULT_ENDPOINT" ]; then
        read -p "Rune-Vault Token: " RUNEVAULT_TOKEN
    fi

    # Generate .env file
    cat > "$ENV_FILE" <<EOF
# MCP Server Configuration
MCP_SERVER_MODE="http"
MCP_SERVER_HOST="127.0.0.1"
MCP_SERVER_PORT="8000"
MCP_SERVER_NAME="envector_mcp_server"

# enVector Cloud (Team-shared)
ENVECTOR_ADDRESS="$ENVECTOR_ADDRESS"
ENVECTOR_API_KEY="$ENVECTOR_API_KEY"

# enVector Options
ENVECTOR_KEY_ID="mcp_key"
ENVECTOR_KEY_PATH="./keys"
ENVECTOR_EVAL_MODE="rmp"
ENVECTOR_ENCRYPTED_QUERY="false"
ENVECTOR_AUTO_KEY_SETUP="true"

# Rune-Vault Integration (Optional)
RUNEVAULT_ENDPOINT="$RUNEVAULT_ENDPOINT"
RUNEVAULT_TOKEN="$RUNEVAULT_TOKEN"

# Embedding Configuration
EMBEDDING_MODE="femb"
EMBEDDING_MODEL="sentence-transformers/all-MiniLM-L6-v2"
EOF

    print_info "Credentials configured in $ENV_FILE"
}

setup_agent_integration() {
    print_step "Choose your AI agent integration..."

    echo "Rune works with multiple AI agents:"
    echo "1) Claude (MCP native support)"
    echo "2) Gemini / Codex / Other (HTTP endpoint)"
    echo "3) Skip (configure manually later)"
    echo ""
    read -p "Select (1, 2, or 3): " AGENT_CHOICE

    case "$AGENT_CHOICE" in
        1)
            setup_claude_mcp
            ;;
        2)
            show_http_setup
            ;;
        3)
            print_info "Skipping agent setup (configure manually)"
            ;;
        *)
            print_warn "Invalid selection, skipping agent setup"
            ;;
    esac
}

setup_claude_mcp() {
    print_step "Configuring Claude MCP..."

    CLAUDE_CONFIG="$HOME/.claude/mcp_settings.json"
    RUNE_ROOT="$(pwd)"

    # Check if Claude config exists
    if [ ! -f "$CLAUDE_CONFIG" ]; then
        print_warn "Claude MCP config not found at $CLAUDE_CONFIG"
        print_info "Creating new config..."
        mkdir -p "$HOME/.claude"
        echo '{"mcpServers":{}}' > "$CLAUDE_CONFIG"
    fi

    # Add envector-mcp-server to Claude config
    print_info "Adding envector-mcp-server to Claude MCP settings..."

    # Use Python to update JSON
    python3.12 -c "
import json

config_path = '$CLAUDE_CONFIG'
rune_root = '$RUNE_ROOT'

with open(config_path, 'r') as f:
    config = json.load(f)

if 'mcpServers' not in config:
    config['mcpServers'] = {}

config['mcpServers']['envector-mcp-server'] = {
    'command': f'{rune_root}/mcp/envector-mcp-server/.venv/bin/python',
    'args': [f'{rune_root}/mcp/envector-mcp-server/srcs/server.py'],
    'env': {
        'PYTHONPATH': f'{rune_root}/mcp/envector-mcp-server'
    }
}

with open(config_path, 'w') as f:
    json.dump(config, f, indent=2)

print('✓ Claude MCP configuration updated')
"

    print_info "Claude will auto-start MCP server on next launch"
}

show_http_setup() {
    print_header "HTTP Endpoint Setup"

    RUNE_ROOT="$(pwd)"

    echo "Rune MCP server runs as HTTP endpoint for non-Claude agents."
    echo ""
    echo "1️⃣  Start the MCP server:"
    echo "   cd $RUNE_ROOT/mcp/envector-mcp-server"
    echo "   source .venv/bin/activate"
    echo "   python srcs/server.py --mode http --host 127.0.0.1 --port 8000"
    echo ""
    echo "2️⃣  Server will be available at:"
    echo "   http://127.0.0.1:8000"
    echo ""
    echo "3️⃣  Configure your AI agent to use MCP endpoint:"
    echo ""
    echo "   For Gemini:"
    echo "   - Use Google AI SDK with custom tools"
    echo "   - Point to http://127.0.0.1:8000/mcp/v1/tools"
    echo ""
    echo "   For OpenAI Codex:"
    echo "   - Use OpenAI function calling"
    echo "   - Proxy MCP tools via http://127.0.0.1:8000/mcp/v1/tools"
    echo ""
    echo "   For Custom Agents:"
    echo "   - OpenAPI spec: http://127.0.0.1:8000/openapi.json"
    echo "   - MCP protocol: https://modelcontextprotocol.io"
    echo ""

    print_info "HTTP setup guide shown above"
}

create_startup_scripts() {
    print_step "Creating startup scripts..."

    RUNE_ROOT="$(pwd)"

    # Create start script
    cat > "start-mcp-server.sh" <<'EOF'
#!/bin/bash
# Start Rune MCP Server (HTTP mode for all agents)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/mcp/envector-mcp-server"

echo "Starting Rune MCP Server..."
echo "Endpoint: http://127.0.0.1:8000"
echo "Press Ctrl+C to stop"
echo ""

source .venv/bin/activate
python srcs/server.py --mode http --host 127.0.0.1 --port 8000
EOF

    chmod +x "start-mcp-server.sh"
    print_info "Created start-mcp-server.sh"

    # Create test script
    cat > "test-connection.sh" <<'EOF'
#!/bin/bash
# Test Rune MCP Server connection

echo "Testing Rune MCP Server..."
echo ""

# Check if server is running
if curl -s http://127.0.0.1:8000/health > /dev/null 2>&1; then
    echo "✓ Server is running"
    echo ""
    echo "Available tools:"
    curl -s http://127.0.0.1:8000/mcp/v1/tools | python3 -m json.tool
else
    echo "✗ Server is not running"
    echo ""
    echo "Start with: ./start-mcp-server.sh"
fi
EOF

    chmod +x "test-connection.sh"
    print_info "Created test-connection.sh"
}

show_next_steps() {
    print_header "Installation Complete! 🎉"

    echo "Rune is ready to use with any AI agent."
    echo ""
    echo "Quick Start:"
    echo ""
    echo "1️⃣  For Claude users:"
    echo "   - Restart Claude Code CLI"
    echo "   - MCP server auto-starts"
    echo "   - Try: 'Create a test index'"
    echo ""
    echo "2️⃣  For Gemini/Codex/Other agents:"
    echo "   - Run: ./start-mcp-server.sh"
    echo "   - Server runs at http://127.0.0.1:8000"
    echo "   - Configure your agent to use MCP endpoint"
    echo ""
    echo "3️⃣  Test installation:"
    echo "   ./test-connection.sh"
    echo ""
    echo "📚 Documentation:"
    echo "   - MCP Server: mcp/envector-mcp-server/README.md"
    echo "   - E2E Tests: ../rune-e2e-test/README.md"
    echo "   - Agent Integration: AGENT_INTEGRATION.md"
    echo ""
}

# Main installation flow
print_header "Rune Plugin Installer v${VERSION}"

echo "Agent-agnostic organizational memory system"
echo "Works with: Claude, Gemini, Codex, and any MCP-compatible agent"
echo ""

print_step "Prerequisites check..."
check_python

print_step "Installation steps:"
echo "1. Setup envector-mcp-server"
echo "2. Configure team credentials"
echo "3. Choose AI agent integration"
echo "4. Create startup scripts"
echo ""
read -p "Continue? (y/n): " CONTINUE

if [ "$CONTINUE" != "y" ]; then
    echo "Installation cancelled."
    exit 0
fi

# Run installation
setup_mcp_server
configure_credentials
setup_agent_integration
create_startup_scripts

show_next_steps

print_info "Installation complete! 🚀"
