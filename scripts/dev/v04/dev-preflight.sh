#!/usr/bin/env bash
# dev-preflight.sh — Rune v0.4 dev verification pre-flight.
#
# Verifies + auto-fixes the prereqs before running /rune:configure smoke
# tests against a freshly-spawned MCP server. Idempotent — safe to re-run.
#
# Auto-fixes (no user action needed):
#   - Re-builds bin/rune-mcp from the current branch.
#   - Removes ~/.rune (so /rune:configure starts from scratch).
#   - Empties ~/.claude.json mcpServers (uses --plugin-dir, not direct registration).
#   - Removes ~/.claude/plugins/cache/cryptolab (legacy / earlier dev wiring).
#
# Surfaces failures (user must act):
#   - Vault daemon not listening on 127.0.0.1:50051
#   - runed daemon socket missing at ~/.runed/embedding.sock
#
# Exit code: 0 = ready to verify; 1 = a prereq failed and needs attention.

set -euo pipefail

REPO="${REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

GREEN=$'\033[0;32m'
RED=$'\033[0;31m'
YELLOW=$'\033[0;33m'
BOLD=$'\033[1m'
NC=$'\033[0m'

ok()      { printf "  ${GREEN}✓${NC} %s\n" "$*"; }
fail()    { printf "  ${RED}✗${NC} %s\n" "$*"; }
warn()    { printf "  ${YELLOW}⚠${NC} %s\n" "$*"; }
section() { printf "\n${BOLD}%s${NC}\n" "$*"; }

# Track whether anything fatal happened so we can exit non-zero at the end
# (instead of bailing on first failure — the user wants a full picture).
FATAL=0

# ─── 1. binary fresh build ──────────────────────────────────────────────
section "1. bin/rune-mcp"
cd "$REPO"
if BUILD_OUT=$(go build -o bin/rune-mcp ./cmd/rune-mcp 2>&1); then
  size=$(stat -f%z bin/rune-mcp 2>/dev/null || stat -c%s bin/rune-mcp)
  ok "built ($(printf "%'d" "$size") bytes)"
else
  fail "build failed:"
  printf '%s\n' "$BUILD_OUT" | sed 's/^/      /'
  FATAL=1
fi

# ─── 2. plugin source intact ────────────────────────────────────────────
section "2. plugin source"
[ -f "$REPO/.claude-plugin/plugin.json" ] \
  && ok ".claude-plugin/plugin.json" \
  || { fail ".claude-plugin/plugin.json missing"; FATAL=1; }
[ -x "$REPO/bin/rune-mcp" ] \
  && ok "bin/rune-mcp executable" \
  || { fail "bin/rune-mcp missing or not executable"; FATAL=1; }

# ─── 3. vault daemon ────────────────────────────────────────────────────
section "3. vault daemon (127.0.0.1:50051)"
if nc -vz -w 2 127.0.0.1 50051 >/dev/null 2>&1; then
  ok "TCP 50051 reachable"
else
  fail "TCP 50051 unreachable — start with:"
  printf "      sudo /usr/local/bin/runevault daemon start --config /opt/runevault/configs/runevault.conf\n"
  FATAL=1
fi

# ─── 4. runed daemon ────────────────────────────────────────────────────
section "4. runed embedding daemon"
if [ -S "$HOME/.runed/embedding.sock" ]; then
  ok "~/.runed/embedding.sock alive"
else
  fail "~/.runed/embedding.sock missing — start runed with model + llama-server:"
  printf "      cd ~/cryptolab/rune-project/runed && RUNED_LLAMA_SERVER=… RUNED_MODEL=… bin/runed\n"
  FATAL=1
fi

# ─── 5. ~/.rune state ───────────────────────────────────────────────────
section "5. ~/.rune"
if [ -d "$HOME/.rune" ]; then
  rm -rf "$HOME/.rune"
  ok "removed (was present)"
else
  ok "already absent"
fi

# ─── 6. ~/.claude.json mcpServers ───────────────────────────────────────
section "6. ~/.claude.json mcpServers"
python3 - <<'PY'
import json, pathlib, sys
p = pathlib.Path.home() / '.claude.json'
if not p.exists():
    print('  ✓ (no ~/.claude.json yet)')
    sys.exit(0)
d = json.loads(p.read_text())
servers = d.get('mcpServers') or {}
if servers:
    d['mcpServers'] = {}
    p.write_text(json.dumps(d, indent=2))
    print(f'  ✓ cleared {len(servers)} entr(y/ies): {", ".join(servers.keys())}')
else:
    if 'mcpServers' not in d:
        d['mcpServers'] = {}
        p.write_text(json.dumps(d, indent=2))
    print('  ✓ already empty')
PY

# ─── 7. plugin cache jail ───────────────────────────────────────────────
section "7. plugin cache (cryptolab/)"
if [ -d "$HOME/.claude/plugins/cache/cryptolab" ]; then
  rm -rf "$HOME/.claude/plugins/cache/cryptolab"
  ok "removed (was present — likely from earlier marketplace install)"
else
  ok "already absent"
fi

# ─── summary ────────────────────────────────────────────────────────────
echo
if [ "$FATAL" -ne 0 ]; then
  printf "${RED}${BOLD}NOT READY${NC} — fix the ✗ items above and re-run.\n"
  exit 1
fi

printf "${GREEN}${BOLD}READY.${NC} Start verification in a new terminal:\n\n"
printf "  claude --plugin-dir %s\n\n" "$REPO"

cat <<'EOF'
Then in the new session:
  /plugin                          # confirm 'rune' loaded, Errors 0
  /rune:status                     # expect Dormant (no config yet)
  /rune:configure                  # vault creds + TLS Self-signed
                                   #   endpoint:  tcp://127.0.0.1:50051
                                   #   token:     evt_… (sudo runevault token issue --user redcourage --role member)
                                   #   TLS:       1 (Self-signed)
                                   #   ca cert:   /opt/runevault/certs/ca.pem
                                   #              (sudo cp 권한 부족 시 ~/.rune/ca.pem 미리 복사)
  /rune:status                     # expect Active (Task #28 — restart 없이 도달)
  /rune:capture "..."              # first capture → record_id
  /rune:capture_history            # ~/.rune/capture_log.jsonl 에서 read
  /rune:recall "..."               # score-based 결과
EOF
