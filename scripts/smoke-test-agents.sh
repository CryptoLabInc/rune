#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

check_path() {
  local label="$1"
  local root="$2"
  local venv="$root/.venv"
  local pip_bin="$venv/bin/pip"

  echo "[$label] $root"
  if [ ! -d "$root" ]; then
    echo -e "  ${YELLOW}⚠${NC} missing root path"
    return 0
  fi

  if [ ! -f "$venv/bin/python3" ]; then
    echo -e "  ${RED}✗${NC} missing .venv/bin/python3"
    return 1
  fi

  echo -e "  ${GREEN}✓${NC} .venv exists"
  if [ -f "$pip_bin" ]; then
    local shebang
    shebang="$(head -1 "$pip_bin" 2>/dev/null || true)"
    if [[ "$shebang" == *"$venv"* ]]; then
      echo -e "  ${GREEN}✓${NC} pip shebang points to local .venv"
    else
      echo -e "  ${RED}✗${NC} pip shebang contamination: $shebang"
      return 1
    fi
  else
    echo -e "  ${YELLOW}⚠${NC} pip binary missing"
  fi
  return 0
}

FAIL=0
check_path "codex" "${RUNE_CODEX_ROOT:-$HOME/.codex/skills/rune}" || FAIL=1
check_path "claude" "${RUNE_CLAUDE_ROOT:-$HOME/.claude/plugins/cache/cryptolab/rune/0.2.5}" || FAIL=1
check_path "gemini" "${RUNE_GEMINI_ROOT:-$HOME/.gemini/extensions/rune}" || FAIL=1

if [ "$FAIL" -eq 0 ]; then
  echo ""
  echo -e "${GREEN}All available agent roots passed.${NC}"
else
  echo ""
  echo -e "${RED}One or more agent roots failed.${NC}"
  exit 1
fi
