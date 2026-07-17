---
description: Update Rune's runtime binaries (rune-mcp, runed, the rune CLI itself) in place to the published manifest versions
allowed-tools: Bash(~/.rune/bin/rune update:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune update:*)
---

# /rune:update — Update Runtime Binaries

Refresh the runtime binaries (rune-mcp, runed, and the rune CLI itself) in
place — no plugin reinstall. Non-destructive; works whether Rune is active or
dormant. The user never runs `rune update` themselves — the agent runs it.

## Run

Prefer the installed CLI; fall back to the plugin wrapper only if it is absent.
Always pass `--plugin-root "${CLAUDE_PLUGIN_ROOT}"` so the CLI can run the
plugin-compatibility check:

- `~/.rune/bin/rune update --plugin-root "${CLAUDE_PLUGIN_ROOT}"`
- `bash -c "${CLAUDE_PLUGIN_ROOT}/bin/rune update --plugin-root ${CLAUDE_PLUGIN_ROOT}"`
  — only when `~/.rune/bin/rune` does not exist yet.

If `$ARGUMENTS` contains `check`, add `--check` to report available updates
without applying.

## Interpret

Surface all output verbatim — the CLI's own lines already name the next step.
The two that need a nudge beyond what it prints:

- `updated rune_mcp: … (run /mcp to reconnect now)` → applies next session; run
  `/mcp` to pick it up immediately.
- `updated runed: … (staged; applies on next daemon start)` → run `/rune:activate`
  to start the daemon on the new binary now.
- `updated rune_cli: … (applies on the next rune invocation)` → nothing to do;
  the next `rune` command already runs the new binary.

(`daemon reloaded`, `all binaries are up to date`, `refusing to apply …`, and
the plugin-version notes are self-explanatory — relay them as-is; the refusal
names its own fix, `claude plugin update rune`.)

Binaries apply independently, so one run can update one and fail the other —
relay each `updated …` line even alongside an error. Do NOT loop or retry;
surface the message and stop.
