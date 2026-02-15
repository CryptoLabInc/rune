---
description: Activate Rune after infrastructure is ready
allowed-tools: Bash(python3:*), Bash(cat ~/.rune/*), Bash(curl:*), Bash(scripts/*), Read, Write
---

# /rune:activate — Activate Plugin

Validate infrastructure and switch to active state.

## Steps

1. Check if `~/.rune/config.json` exists.
   - NO: Respond "Not configured. Run `/rune:configure` first." and stop.

2. Read config and verify required fields exist (`vault.endpoint`, `vault.token`, `envector.endpoint`, `envector.api_key`).
   - Missing fields: Show what's missing, suggest `/rune:configure`.

3. Run infrastructure validation:
   - Check Vault connectivity: `curl -sf <vault-url>/health`
   - Check Python venv exists
   - Check MCP server can import

4. If all checks pass:
   - Update `~/.rune/config.json` setting `state` to `"active"`
   - Respond: "Rune activated. Organizational memory is now online."

5. If checks fail:
   - Keep `state` as `"dormant"`
   - Show detailed error report for each failed check
   - Suggest: `/rune:status` for more info

**Note**: This is the ONLY command that makes network requests to validate infrastructure.
