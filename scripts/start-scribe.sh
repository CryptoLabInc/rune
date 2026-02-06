#!/bin/bash
set -e

# Start Rune Scribe Agent
# This script starts the Scribe FastAPI server locally for webhook handling

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

# Check dependencies
if ! pip show uvicorn > /dev/null; then
    print_error "Scribe dependencies (uvicorn, fastapi) not found."
    echo "Please run: pip install -r requirements.txt"
    exit 1
fi

# Check if Scribe is already running
if pgrep -f "agents.scribe.server:app" > /dev/null; then
    print_warn "Scribe server is already running"
    echo "PID: $(pgrep -f "agents.scribe.server:app")"
else
    print_info "Starting Scribe Agent server..."

    cd "$PLUGIN_DIR"
    nohup uvicorn agents.scribe.server:app --host 0.0.0.0 --port 8000 > "$LOG_DIR/scribe.log" 2>&1 &
    SCRIBE_PID=$!

    # Wait a moment and check if it's still running
    sleep 2
    if ps -p $SCRIBE_PID > /dev/null; then
        print_info "Scribe Agent server started (PID: $SCRIBE_PID)"
        echo "  Port: 8000"
        echo "  Log:  $LOG_DIR/scribe.log"
    else
        print_error "Scribe Agent server failed to start"
        echo "Check logs at: $LOG_DIR/scribe.log"
        exit 1
    fi
fi

echo ""
echo "🔌 Usage with Slack:"
echo "   1. Expose locahost:8000 to the internet (e.g., ngrok http 8000)"
echo "   2. Configure Slack App Event Subscription URL to: <ngrok-url>/slack/events"
echo ""
echo "To stop server:"
echo "  pkill -f 'agents.scribe.server:app'"
echo ""
