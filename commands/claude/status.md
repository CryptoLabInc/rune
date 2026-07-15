---
description: Check Rune plugin activation status and infrastructure health
allowed-tools: Bash(cat ~/.rune/*), Bash(ls:*), Read, mcp__plugin_rune_rune__diagnostics, mcp__plugin_rune_rune__console_status
---

# /rune:status — Plugin Status

Read `~/.rune/config.json` and show a status report.

## Steps

1. Check if `~/.rune/config.json` exists. If not, show "Not configured" and suggest `/rune:configure`.

2. Read the config and display the basic configuration section.

3. Call the `diagnostics` MCP tool to get system health. If unavailable, fall back to `console_status`.

4. Display the full status report:

```
Rune Plugin Status
==================
State: Active / Dormant
Dormant Reason: <reason>  (only when dormant with a reason)
Dormant Since:  <timestamp>  (only when dormant with a timestamp)

Configuration:
  [check] Config file: ~/.rune/config.json
  [check] Console Endpoint: <url or "not set">

System Health:
  [check] Console         : healthy / unreachable
  [check] Encryption Key: loaded (key_id) / not loaded
  [check] Agent DEK     : loaded / not loaded
  [check] Scribe        : ready / not initialized
  [check] Retriever     : ready / not initialized
  [check] Embedder      : <status> (model: <model>, dim: <vector_dim>, uptime: <human>, requests: <n>)
                          socket: <socket_path>
                          info error:   <info_error>    (only when present)
                          health error: <health_error>  (only when present)

Recommendations:
  - <actionable suggestions based on what's missing>
```

Use checkmarks for healthy items, X marks for issues.

**Embedder rendering rules** (from diagnostics `embedding` section):
- Check when `status == "OK"` and neither `info_error` nor `health_error` is set
- **`IDLE` is healthy, not a fault** — render a paused/ready marker (e.g. `⏸`), NOT an X. The embedder intentionally stopped its llama-server after an idle period (`RUNED_IDLE_TIMEOUT`) to free model memory; the next capture/recall resumes it automatically. Surface the diagnostics `message` (e.g. "…resumes on next request") so the user knows nothing is broken — no action needed.
- X mark when `status` is `LOADING` / `DEGRADED` / `SHUTTING_DOWN` / `UNSPECIFIED`, or when any error field is populated
- Show `socket_path` always when populated - it's the only way users can tell which runed instance they're talking to. Recall: runed is a shared singleton across sessions, so divergent socket paths between teammates indicate a misconfiguration.
- Format `uptime_seconds` as a human-readable duration (e.g. `8h8m`, `37s`)
- Omit the entire `(model: ..., dim: ...)` parenthetical when the embedder is not initialized (i.e. `model` empty AND `socket_path` empty — pre-boot). In that case render just `Embedder : not initialized`.

**Dormant Reason Display**: When `dormant_reason` is present in config or diagnostics, translate reason code into a user-friendly message:
- `not_configured`: "Rune is not configured yet. Run `/rune:configure` to set up Console credentials."
- `console_unconfigured`: "Console endpoint or token is missing in config. Run `/rune:configure`."
- `user_deactivated`: "Manually deactivated by user via `/rune:deactivate`. Run `/rune:activate` to resume."
- `invalid_state`: "config.json has an unknown state value. Re-run `/rune:configure` to reset."
- Other/unknown: show raw reason string with "Run `/rune:activate` to retry."

**Boot Error Display** (`console.last_boot_error`): When
`diagnostics.console.last_boot_error` is set (regardless of `state` — a transient
failure keeps the persisted `state` at `"active"` while the boot loop retries),
render its `hint` field prominently in the **Recommendations** section. The boot
loop has already classified the root cause — relay it verbatim instead of
guessing.

Render shape (one-block, no extra investigation):

```
Recommendations:
  Boot failure (<kind>):
    <hint>

  Details: <detail>  (only when the hint is generic / kind is "unknown")
  Attempts: <attempts>  (only when > 1, to show retry was tried)
```

Examples:
- `kind=console_tls_handshake` → "CA cert at … does not verify the server cert.
  The CA was likely regenerated on the server side. Re-fetch from your
  Console admin and replace `~/.rune/certs/ca.pem`."
- `kind=console_auth` → "Console rejected the token (expired/revoked/consumed).
  Ask your Console admin for a fresh invite and re-run `/rune:configure` with
  the new registration string."
- `kind=console_network` → "Console endpoint `<endpoint>` is not reachable.
  Verify the host/port and your network/firewall."
- `kind=embedder_unreachable` → "Embedder daemon (`runed`) is not running on
  its UDS socket. Re-run `/rune:activate` to (re)spawn it."
- `kind=unknown` → show both `hint` and `detail` and suggest sharing with admin.

DO NOT call shell tools (`openssl`, `nc`, `curl`, etc.) to "verify" the
classifier's verdict. The classifier already inspected the underlying gRPC /
TLS / DNS error; manual probing only adds turns + cost without changing the
recommendation. The user can decide to manually verify later if they want.

