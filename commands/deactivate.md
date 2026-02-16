---
description: Deactivate Rune to pause organizational memory without clearing configuration
allowed-tools: Read, Edit, mcp__envector__reload_pipelines
---

# /rune:deactivate — Deactivate Plugin

Switch from active to dormant state. Configuration is preserved — use `/rune:activate` to re-enable.

## Steps

1. Read `~/.rune/config.json`.
   - Not found: Respond "Nothing to deactivate. No configuration exists." and stop.

2. Check current `state`:
   - Already `"dormant"`: Respond "Rune is already dormant." and stop.

3. Update `state` to `"dormant"` in `~/.rune/config.json`.

4. Call `reload_pipelines` as a **native MCP tool** (`mcp__envector__reload_pipelines`) — invoke it directly like any other tool, do NOT use `claude mcp call` via Bash (that subcommand doesn't exist).
   - This ensures MCP tools (`capture`/`recall`) immediately return errors instead of processing.
   - If `reload_pipelines` is not available as a tool (MCP server not running), note that MCP pipelines will remain live until session restart.

5. Respond: "Rune deactivated. Organizational memory is paused. Config preserved — `/rune:activate` to resume."
