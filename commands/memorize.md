---
description: Store organizational context to encrypted memory
allowed-tools: Bash(python3:*), Bash(cat ~/.rune/*), Read, mcp__plugin_rune_envector__*
---

# /rune:memorize — Store Context

Manually store organizational context to encrypted memory.

The argument `$ARGUMENTS` contains the context to store.

## Activation Check

1. Read `~/.rune/config.json`. If missing or `state` is not `"active"`, respond:
   "Rune is dormant. Run `/rune:configure` and `/rune:activate` first."
   Do NOT attempt any storage.

## When Active

1. Parse `$ARGUMENTS` as the context to memorize.
2. Add metadata: timestamp, domain classification (infer from content).
3. Use the envector MCP tools to embed and store the context.
4. Confirm what was stored with a brief summary.

## Example

```
/rune:memorize We chose PostgreSQL over MongoDB for better ACID guarantees
```
