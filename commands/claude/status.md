---
description: Check Rune plugin activation status and infrastructure health
allowed-tools: Bash(python3:*), Bash(cat ~/.rune/*), Bash(ls:*), Bash(scripts/*), Read
---

# /rune:status — Plugin Status

Read `~/.rune/config.json` and show a status report.

## Steps

1. Check if `~/.rune/config.json` exists. If not, show "Not configured" and suggest `/rune:configure`.

2. Read the config and display:

```
Rune Plugin Status
==================
State: Active / Dormant

Configuration:
  [check] Config file: ~/.rune/config.json
  [check] Vault Endpoint: <url or "not set">
  [check] enVector: <endpoint or "not set">

Infrastructure:
  [check] Python venv: <installedFrom>/.venv (read installedFrom from config metadata)
  [check] MCP server logs: <recent or "stale/missing">

Recommendations:
  - <actionable suggestions based on what's missing>
```

Use checkmarks for present items, X marks for missing.
