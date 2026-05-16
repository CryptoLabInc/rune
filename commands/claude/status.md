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
  [check] Embedder      : <status> (model: <model>, dim: <vector_dim>, uptime: <human>, requests: <n>)
                          socket: <socket_path>
                          info error:   <info_error>    (only when present)
                          health error: <health_error>  (only when present)
  [check] enVector Cloud: reachable (<latency>ms) / unreachable

Recommendations:
  - <actionable suggestions based on what's missing>
```

Use checkmarks for healthy items, X marks for issues.

**Embedder rendering rules** (from diagnostics `embedding` section):
- Check when `status == "OK"` and neither `info_error` nor `health_error` is set
- X mark when `status` is `LOADING` / `DEGRADED` / `SHUTTING_DOWN` / `UNSPECIFIED`, or when any error field is populated
- Show `socket_path` always when populated - it's the only way users can tell which runed instance they're talking to. Recall: runed is a shared singleton across sessions, so divergent socket paths between teammates indicate a misconfiguration.
- Format `uptime_seconds` as a human-readable duration (e.g. `8h8m`, `37s`)
- Omit the entire `(model: ..., dim: ...)` parenthetical when the embedder is not initialized (i.e. `model` empty AND `socket_path` empty — pre-boot). In that case render just `Embedder : not initialized`.

**Dormant Reason Display**: When `dormant_reason` is present in config or diagnostics, translate reason code into a user-friendly message:
- `vault_unreachable`: "Vault server could not be reached. Check if it's running and the endpoint is correct."
- `vault_token_invalid`: "Vault token was rejected. Token may be expired — run `/rune:configure` to update."
- `envector_unreachable`: "enVector Cloud could not be reached. Check network and endpoint."
- `envector_key_invalid`: "enVector API key was rejected. Contact your Vault administrator."
- `envector_not_provisioned`: "No enVector Cloud endpoint is configured on Rune-Vault. Contact your Vault administrator."
- `pipeline_init_failed`: "Pipeline initialization failed. Run `/rune:activate` to retry."
- `user_deactivated`: "Manually deactivated by user via `/rune:deactivate`."
- Other/unknown: show raw reason string with "Run `/rune:activate` to retry."

**Boot Error Display** (`vault.last_boot_error`): When `state != "active"` AND
`diagnostics.vault.last_boot_error` is set, render its `hint` field
prominently in the **Recommendations** section. The boot loop has already
classified the root cause — relay it verbatim instead of guessing.

Render shape (one-block, no extra investigation):

```
Recommendations:
  Boot failure (<kind>):
    <hint>

  Details: <detail>  (only when the hint is generic / kind is "unknown")
  Attempts: <attempts>  (only when > 1, to show retry was tried)
```

Examples:
- `kind=vault_tls_handshake` → "CA cert at … does not verify the server cert.
  The CA was likely regenerated on the server side. Re-fetch from your
  Vault admin and replace `~/.rune/certs/ca.pem`."
- `kind=vault_auth` → "Vault rejected the token. Re-issue with
  `runevault token issue --user <name> --role member`."
- `kind=vault_network` → "Vault endpoint `<endpoint>` is not reachable.
  Verify the host/port and your network/firewall."
- `kind=embedder_unreachable` → "Embedder daemon (`runed`) is not running on
  its UDS socket. Start it with `runed start`."
- `kind=unknown` → show both `hint` and `detail` and suggest sharing with admin.

DO NOT call shell tools (`openssl`, `nc`, `curl`, etc.) to "verify" the
classifier's verdict. The classifier already inspected the underlying gRPC /
TLS / DNS error; manual probing only adds turns + cost without changing the
recommendation. The user can decide to manually verify later if they want.

