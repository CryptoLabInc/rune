#!/bin/bash
set -e

# Rune Plugin Installation Script
# This script sets up the Python environment and dependencies for the plugin

VERSION="0.1.0"
PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

print_header "Rune Plugin Installer v${VERSION}"

# Check Python
print_step "Checking Python installation..."
if ! command -v python3 &> /dev/null; then
    print_error "Python 3 is not installed"
    echo "Please install Python 3.12 or higher:"
    echo "  - macOS: brew install python3"
    echo "  - Linux: sudo apt install python3 python3-pip"
    exit 1
fi

PYTHON_VERSION=$(python3 --version | cut -d' ' -f2)
PYTHON_MAJOR=$(echo "$PYTHON_VERSION" | cut -d. -f1)
PYTHON_MINOR=$(echo "$PYTHON_VERSION" | cut -d. -f2)
if [ "$PYTHON_MAJOR" -lt 3 ] || { [ "$PYTHON_MAJOR" -eq 3 ] && [ "$PYTHON_MINOR" -lt 12 ]; }; then
    print_error "Python 3.12 or higher is required (found $PYTHON_VERSION)"
    echo "pyenvector requires Python 3.12. Please upgrade:"
    echo "  - macOS: brew install python@3.12"
    echo "  - Linux: sudo apt install python3.12 python3.12-venv"
    exit 1
fi
print_info "Python $PYTHON_VERSION detected"

# Create virtual environment
print_step "Setting up Python virtual environment..."
cd "$PLUGIN_DIR"

if [ ! -d ".venv" ]; then
    print_info "Creating virtual environment..."
    python3 -m venv .venv
else
    print_info "Virtual environment already exists"
fi

# Activate venv
source .venv/bin/activate

# Install dependencies
print_step "Installing Python dependencies..."
print_info "This may take a few minutes..."
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

print_info "Dependencies installed successfully!"

# Create config directory
print_step "Creating configuration directory..."
mkdir -p ~/.rune
chmod 700 ~/.rune
print_info "Created ~/.rune directory"

# Installation complete
print_header "Installation Complete!"

echo "✓ Python virtual environment: ${PLUGIN_DIR}/.venv"
echo "✓ Dependencies installed"
echo "✓ Config directory: ~/.rune"
echo ""
echo "Next steps:"
echo "  1. Configure credentials: /rune:configure"
echo "  2. Start MCP servers: scripts/start-mcp-servers.sh"
echo "  3. Restart Claude"
echo ""
print_info "Setup complete! 🎉"
