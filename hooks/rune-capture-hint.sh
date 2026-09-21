#!/bin/bash
# SessionStart hook: nudge the agent to proactively capture team-relevant
# knowledge. Best-effort and fail-closed: it stays silent (exit 0, no output)
# unless RUNE is configured and active, so a dormant or unconfigured install
# never nudges toward a capture that would fail.
cfg="$HOME/.rune/config.json"
[ -f "$cfg" ] || exit 0
grep -q '"state"[[:space:]]*:[[:space:]]*"active"' "$cfg" || exit 0

cat <<'EOF'
RUNE team memory is active. This is the TEAM's shared memory, separate from your own memory: call mcp__plugin_rune_rune__capture proactively, without being asked, whenever the work produces a decision and its rationale, a root cause and fix, a rejected alternative, or context a teammate would search for later. Near-duplicates are dropped server-side, so capture when in doubt. Before a design choice or a debugging session, call mcp__plugin_rune_rune__recall to check whether a teammate already solved it.
EOF
