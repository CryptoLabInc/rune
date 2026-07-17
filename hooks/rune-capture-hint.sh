#!/bin/bash
# SessionStart hook: nudge the agent to proactively capture team-relevant
# knowledge. Best-effort and fail-closed: it stays silent (exit 0, no output)
# unless RUNE is configured and active, so a dormant or unconfigured install
# never nudges toward a capture that would fail.
cfg="$HOME/.rune/config.json"
[ -f "$cfg" ] || exit 0
grep -q '"state"[[:space:]]*:[[:space:]]*"active"' "$cfg" || exit 0

cat <<'EOF'
RUNE organizational memory is active. Proactively run /rune:capture when you would save something to your own memory, or when the work produces a decision worth sharing with the team, a troubleshooting resolution, or context worth preserving for teammates. The capture command guides you to compose a concise insight plus a fuller context.
EOF
