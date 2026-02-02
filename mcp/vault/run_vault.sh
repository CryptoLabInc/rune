#!/bin/bash
# Get the directory of this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
WORKSPACE_ROOT="$DIR/../.."
VENV_PATH="$WORKSPACE_ROOT/.vault_venv"

if [ -f "$VENV_PATH/bin/activate" ]; then
    source "$VENV_PATH/bin/activate"
else
    echo "Virtual environment not found at $VENV_PATH. Please create it first."
    exit 1
fi

echo "Starting enVector-Vault MCP Server on port 50080..."
python3 "$DIR/vault_mcp.py" server
