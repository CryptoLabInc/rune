#!/usr/bin/env bash
#
# Rune Scribe — Slack App Setup Helper
#
# Starts the Scribe server and optionally opens an ngrok tunnel.
#
# Usage:
#   bash rune/scripts/setup-slack-app.sh [--ngrok]
#
# Environment Variables (set before running):
#   ENVECTOR_ENDPOINT     — enVector Cloud endpoint (required)
#   ENVECTOR_API_KEY      — enVector API key (required)
#   SLACK_SIGNING_SECRET  — Slack signing secret (optional for dev)
#   ANTHROPIC_API_KEY     — For Tier 2/3 LLM (optional)
#   SCRIBE_PORT           — Server port (default: 8080)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RUNE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG_DIR="$HOME/.rune"
CONFIG_FILE="$CONFIG_DIR/config.json"
PORT="${SCRIBE_PORT:-8080}"
USE_NGROK=false

# Parse arguments
for arg in "$@"; do
    case "$arg" in
        --ngrok) USE_NGROK=true ;;
        *) echo "Unknown argument: $arg"; exit 1 ;;
    esac
done

echo "=============================================="
echo "  Rune Scribe — Slack App Setup"
echo "=============================================="

# ------------------------------------------------------------------
# 1. Check prerequisites
# ------------------------------------------------------------------
echo ""
echo "[1/4] Checking prerequisites..."

# Check Python
if ! command -v python3 &>/dev/null; then
    echo "  ✗ python3 not found"
    exit 1
fi
PYTHON_VERSION=$(python3 --version 2>&1)
echo "  ✓ $PYTHON_VERSION"

# Check required env vars
if [ -z "${ENVECTOR_ENDPOINT:-}" ] && [ ! -f "$CONFIG_FILE" ]; then
    echo "  ✗ ENVECTOR_ENDPOINT not set and no config file found"
    echo ""
    echo "  Set environment variables:"
    echo "    export ENVECTOR_ENDPOINT=<cloud-endpoint>"
    echo "    export ENVECTOR_API_KEY=<api-key>"
    echo ""
    echo "  Or create $CONFIG_FILE (see SLACK_SETUP.md)"
    exit 1
fi
echo "  ✓ enVector config available"

# Check ngrok (if requested)
if [ "$USE_NGROK" = true ]; then
    if ! command -v ngrok &>/dev/null; then
        echo "  ✗ ngrok not found — install with: brew install ngrok"
        exit 1
    fi
    echo "  ✓ ngrok available"
fi

# ------------------------------------------------------------------
# 2. Ensure config directory
# ------------------------------------------------------------------
echo ""
echo "[2/4] Ensuring config directory..."

mkdir -p "$CONFIG_DIR"
mkdir -p "$CONFIG_DIR/logs"
mkdir -p "$CONFIG_DIR/keys"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "  Creating default config: $CONFIG_FILE"
    cat > "$CONFIG_FILE" << 'CONF'
{
  "state": "active",
  "scribe": {
    "slack_webhook_port": 8080,
    "slack_signing_secret": "",
    "similarity_threshold": 0.5,
    "auto_capture_threshold": 0.8,
    "tier2_enabled": true
  },
  "envector": {
    "endpoint": "",
    "api_key": "",
    "collection": "rune-context"
  },
  "retriever": {
    "anthropic_api_key": ""
  }
}
CONF
    chmod 600 "$CONFIG_FILE"
    echo "  ✓ Created $CONFIG_FILE (edit with your credentials)"
else
    echo "  ✓ Config exists: $CONFIG_FILE"
fi

# ------------------------------------------------------------------
# 3. Start ngrok (if requested)
# ------------------------------------------------------------------
if [ "$USE_NGROK" = true ]; then
    echo ""
    echo "[3/4] Starting ngrok tunnel..."

    # Kill any existing ngrok
    pkill -f "ngrok http" 2>/dev/null || true
    sleep 1

    ngrok http "$PORT" --log=stdout > "$CONFIG_DIR/logs/ngrok.log" 2>&1 &
    NGROK_PID=$!
    echo "  ngrok PID: $NGROK_PID"

    # Wait for ngrok to start and get public URL
    sleep 3
    NGROK_URL=$(curl -s http://127.0.0.1:4040/api/tunnels 2>/dev/null \
                | python3 -c "import sys,json; print(json.load(sys.stdin)['tunnels'][0]['public_url'])" 2>/dev/null \
                || echo "")

    if [ -n "$NGROK_URL" ]; then
        echo "  ✓ ngrok tunnel: $NGROK_URL"
        echo ""
        echo "  ┌─────────────────────────────────────────────────────┐"
        echo "  │ Slack Event Subscription URL:                       │"
        echo "  │   ${NGROK_URL}/slack/events"
        echo "  └─────────────────────────────────────────────────────┘"
    else
        echo "  ✗ Could not get ngrok URL (check $CONFIG_DIR/logs/ngrok.log)"
    fi
else
    echo ""
    echo "[3/4] Skipping ngrok (use --ngrok to enable)"
fi

# ------------------------------------------------------------------
# 4. Start Scribe server
# ------------------------------------------------------------------
echo ""
echo "[4/4] Starting Scribe server on port $PORT..."
echo ""

cd "$RUNE_DIR"
export PYTHONPATH="${RUNE_DIR}:${PYTHONPATH:-}"

exec python3 -m agents.scribe.server
