---
description: Deactivate Rune — pause organizational memory (active → dormant) without clearing credentials
allowed-tools: mcp__plugin_rune_rune__deactivate
---

# /rune:deactivate — Pause Rune

Flip Rune from active to dormant. Credentials and connection stay in
`~/.rune/config.json` — resume anytime with `/rune:activate`.

## Steps

1. Call `mcp__plugin_rune_rune__deactivate`. The server persists `state=dormant` (reason
   `user_deactivated`) and re-runs the boot loop into the dormant state, so
   `capture` / `recall` immediately return `PIPELINE_NOT_READY` until
   reactivated.
2. Render the result:
   - `ok` with `state == "dormant"` → "Rune is now dormant. Organizational
     memory is paused — `/rune:activate` to resume."
   - any other `state` → relay it and suggest `/rune:status` to inspect.
