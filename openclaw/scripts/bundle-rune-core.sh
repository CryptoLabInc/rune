#!/bin/bash
set -e

# Bundle rune-core Python sources into the npm package.
# Called by prepack — copies from the parent rune repo root
# into a local rune-core/ directory so npm pack includes the files.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SOURCE="${PLUGIN_DIR}/.."
TARGET="${PLUGIN_DIR}/rune-core"

if [ ! -d "$SOURCE/mcp" ]; then
  echo "ERROR: rune repo root not found at $SOURCE"
  echo "Expected to find mcp/, agents/, patterns/, etc. in the parent directory."
  exit 1
fi

echo "Bundling rune-core into npm package..."

# Clean previous bundle
rm -rf "$TARGET"
mkdir -p "$TARGET"

# Copy only the directories needed at runtime
cp -r "$SOURCE/mcp"        "$TARGET/mcp"
cp -r "$SOURCE/agents"     "$TARGET/agents"
cp -r "$SOURCE/patterns"   "$TARGET/patterns"
cp -r "$SOURCE/scripts"    "$TARGET/scripts"
cp -r "$SOURCE/config"     "$TARGET/config"
cp    "$SOURCE/requirements.txt" "$TARGET/requirements.txt"

# Remove Python caches and venvs from the bundle
find "$TARGET" -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
find "$TARGET" -type d -name ".venv" -exec rm -rf {} + 2>/dev/null || true
find "$TARGET" -name "*.pyc" -delete 2>/dev/null || true

echo "Done. Bundled $(du -sh "$TARGET" | cut -f1) into rune-core/"
