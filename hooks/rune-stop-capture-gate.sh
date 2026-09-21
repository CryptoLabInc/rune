#!/bin/bash
# Stop hook: at the end of a turn, ask the agent once whether the work just
# done produced something the team should remember, and if so to call
# mcp__plugin_rune_rune__capture before finishing.
#
# Why a Stop hook: insights are produced at the END of work, but the only
# other nudge (SessionStart hint) sits at the top of the context and is buried
# by the time a decision is made. This fires at the moment the insight exists.
#
# Gating (all must hold, otherwise silent exit 0 so the turn ends normally):
#   1. RUNE is configured and active (fail-closed like the SessionStart hint).
#   2. stop_hook_active is not true — we already nudged this turn; never loop.
#   3. The transcript grew by at least RUNE_STOP_HOOK_MIN_LINES (default 30)
#      since the last nudge in this session, i.e. real work happened.
#   4. No capture tool call appears in that new span — the agent already
#      captured on its own.
#
# When it fires it prints {"decision":"block","reason":...}: Claude Code then
# continues the turn with the reason as instruction. The reason tells the
# agent to capture only if warranted and to otherwise stop, so the cost of a
# false positive is one short model turn.
#
# Dependencies: bash, grep, sed, wc, tail, mkdir. No jq/python.

cfg="$HOME/.rune/config.json"
[ -f "$cfg" ] || exit 0
grep -q '"state"[[:space:]]*:[[:space:]]*"active"' "$cfg" || exit 0

input=$(cat)
[ -n "$input" ] || exit 0

# Minimal JSON field extraction; the hook payload is flat and machine-written.
field() { printf '%s' "$input" | grep -oE "\"$1\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed -E 's/^"[^"]*"[[:space:]]*:[[:space:]]*"//; s/"$//'; }

printf '%s' "$input" | grep -qE '"stop_hook_active"[[:space:]]*:[[:space:]]*true' && exit 0

transcript=$(field transcript_path)
[ -n "$transcript" ] && [ -f "$transcript" ] || exit 0

session=$(field session_id)
[ -n "$session" ] || session=$(printf '%s' "$transcript" | cksum | cut -d' ' -f1)

state_dir="$HOME/.rune/hooks/stop"
mkdir -p "$state_dir" 2>/dev/null || exit 0
state="$state_dir/$session"

last=0
[ -f "$state" ] && last=$(tr -dc '0-9' < "$state")
[ -n "$last" ] || last=0

cur=$(wc -l < "$transcript" | tr -dc '0-9')
min=${RUNE_STOP_HOOK_MIN_LINES:-30}
new=$((cur - last))
[ "$new" -ge "$min" ] || exit 0

# Always advance the mark first so a crash below can never re-nudge the same span.
printf '%s' "$cur" > "$state"

if tail -n +$((last + 1)) "$transcript" | grep -q 'mcp__plugin_rune_rune__capture'; then
  exit 0
fi

cat <<'EOF'
{"decision":"block","reason":"RUNE checkpoint before you finish: review the work in this turn. If it produced a decision and its rationale, a root cause and fix, a rejected alternative, or context a teammate would search for later, call mcp__plugin_rune_rune__capture now with a self-contained insight (and context). Near-duplicates are dropped server-side, so capture when in doubt. If nothing in this turn is worth the team remembering, do not explain — just stop."}
EOF
exit 0
