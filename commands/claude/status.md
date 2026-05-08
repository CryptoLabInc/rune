---
description: Check Rune plugin activation status and infrastructure health
allowed-tools: Bash(cat ~/.rune/*), Bash(ls:*), Read, mcp__envector__diagnostics, mcp__envector__vault_status
---

# /rune:status — Plugin Status

Read `~/.rune/config.json` and show a status report.

## Steps

1. Check if `~/.rune/config.json` exists. If not, show "Not configured" and suggest `/rune:configure`.

2. Read the config and display the basic configuration section.

3. Call the `diagnostics` MCP tool to get system health. If unavailable, fall back to `vault_status`.

4. Display the full status report:

```
Rune Plugin Status
==================
State: Active / Dormant
Dormant Reason: <reason>  (only when dormant with a reason)
Dormant Since:  <timestamp>  (only when dormant with a timestamp)

Configuration:
  [check] Config file: ~/.rune/config.json
  [check] Vault Endpoint: <url or "not set">
  [check] enVector: <endpoint or "not set">

System Health:
  [check] Vault         : healthy / unreachable
  [check] Encryption Key: loaded (key_id) / not loaded
  [check] Agent DEK     : loaded / not loaded
  [check] Scribe        : ready / not initialized
  [check] Retriever     : ready / not initialized
  [check] LLM Provider  : <provider or "none">
  [check] enVector Cloud: reachable (<latency>ms) / unreachable

Recommendations:
  - <actionable suggestions based on what's missing>
```

Use checkmarks for healthy items, X marks for issues.

**Dormant Reason Display**: When `dormant_reason` is present in config or diagnostics, translate reason code into a user-friendly message:
- `vault_unreachable`: "Vault server could not be reached. Check if it's running and the endpoint is correct."
- `vault_token_invalid`: "Vault token was rejected. Token may be expired — run `/rune:configure` to update."
- `envector_unreachable`: "enVector Cloud could not be reached. Check network and endpoint."
- `envector_key_invalid`: "enVector API key was rejected. Contact your Vault administrator."
- `envector_not_provisioned`: "No enVector Cloud endpoint is configured on Rune-Vault. Contact your Vault administrator."
- `pipeline_init_failed`: "Pipeline initialization failed. Run `/rune:activate` to retry."
- `user_deactivated`: "Manually deactivated by user via `/rune:deactivate`."
- Other/unknown: show raw reason string with "Run `/rune:activate` to retry."

