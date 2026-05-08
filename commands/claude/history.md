---
description: View recent capture history
allowed-tools: mcp__envector__capture_history
---

# /rune:history — View Capture History

Show recent captures from organizational memory.

## Steps

1. Call `capture_history` MCP tool with parameters from $ARGUMENTS:
   - `--domain <domain>` → filter by domain
   - `--since <YYYY-MM-DD>` → filter by date
   - `--limit <N>` → number of results (default 20)
   - No arguments → show last 20 captures

2. Display results as a table:

```
Recent Captures
===============
Timestamp            | Record ID                        | Title                    | Domain
2026-03-16T10:00:00Z | dec_20260316_arch_abc             | JWT Auth Migration       | security
2026-03-15T14:30:00Z | dec_20260315_debug_xyz            | Connection Pool Fix      | debugging
...
```

3. If no entries found:
   - "No captures recorded yet. Rune captures decisions automatically during conversations."

4. Show tip at the bottom:
   - "Use `/rune:delete <record_id>` to remove a capture."
