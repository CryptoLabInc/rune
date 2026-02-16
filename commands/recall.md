---
description: Search organizational memory for past decisions and context
allowed-tools: Bash(python3:*), Bash(cat ~/.rune/*), Read, mcp__plugin_rune_envector__*
---

# /rune:recall — Search Memory

Search encrypted organizational memory for relevant context.

The argument `$ARGUMENTS` contains the search query.

## Activation Check

1. Read `~/.rune/config.json`. If missing or `state` is not `"active"`, respond:
   "Rune is dormant. Run `/rune:configure` and `/rune:activate` first."
   Do NOT attempt any search.

## When Active

1. Parse `$ARGUMENTS` as the search query.
2. Use the envector MCP tools to search encrypted vectors.
3. Return relevant results with:
   - Source attribution (who/when)
   - Relevant excerpts
   - Confidence/certainty level
4. Offer to elaborate on any result.

## Example

```
/rune:recall Why did we choose PostgreSQL?
```
