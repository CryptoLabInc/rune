#!/usr/bin/env bash
# SessionStart hook — inject Rune's automatic capture/recall behavior into the
# session context, but ONLY when Rune is active. This is what makes "automatic"
# actually automatic: the instructions are present every session regardless of
# whether the user copied Rune's guidance into their own CLAUDE.md.
#
# Contract (Claude Code SessionStart hook):
#   - stdin: JSON event payload (unused here)
#   - exit 0 + stdout: stdout is injected into the session context
#   - any other exit / empty stdout: nothing injected
#
# Gating: read ~/.rune/config.json (honoring RUNE_HOME). If state is not
# "active", exit 0 silently with no output — no context injected, no network,
# no token spend. Mirrors the plugin's dormant fail-safe.
set -euo pipefail

RUNE_HOME="${RUNE_HOME:-$HOME/.rune}"
config="$RUNE_HOME/config.json"

# No config → dormant/unconfigured → stay silent.
[ -f "$config" ] || exit 0

# Extract state without hard-depending on jq (dev machines may not have it).
if command -v jq >/dev/null 2>&1; then
  state="$(jq -r '.state // empty' "$config" 2>/dev/null || true)"
else
  state="$(grep -o '"state"[[:space:]]*:[[:space:]]*"[^"]*"' "$config" 2>/dev/null \
    | head -1 | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/' || true)"
fi

[ "$state" = "active" ] || exit 0

# Active → inject the automatic-behavior contract. Kept concise because this
# rides in the context of every session. Full reference lives in SKILL.md.
cat <<'EOF'
[Rune organizational memory is ACTIVE for this session]

Rune gives this session access to the team's encrypted organizational memory
via MCP tools. Use it proactively — the user should not have to run any
slash command for capture/recall to happen.

RECALL (before answering):
- When the user asks something where prior team context could matter —
  decision rationale, past trade-offs, rejected alternatives, "have we done
  this before", architecture/planning discussion — call the Rune `recall` MCP
  tool first, then fold any relevant hits into your answer with attribution.
- If nothing relevant comes back, answer normally. Do not announce empty recalls.

CAPTURE (as decisions crystallize):
- When a significant decision, committed trade-off, confirmed direction,
  lesson learned, or established standard emerges in the conversation, call the
  Rune `capture` MCP tool with just the relevant excerpt (not the whole
  session). Do not pause the conversation and do not announce the capture
  unless the user asks.
- Redact secrets/PII (API keys, passwords, tokens) before capturing.
- Do NOT capture: brainstorming without commitment, questions, status updates,
  casual chat, or hypothetical/deferred ideas ("maybe later").
- Session-end sweep: when the user is wrapping up, review for any uncaptured
  significant decisions and store them in one `batch_capture` call.

If any Rune tool call fails, fall back silently to normal behavior — do not
retry in a loop or block the conversation.
EOF
exit 0
