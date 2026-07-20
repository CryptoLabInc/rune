---
description: Check Rune's status and infrastructure health
allowed-tools: mcp__plugin_rune_rune__diagnostics
---

# /rune:status — Status & Health

Call `mcp__plugin_rune_rune__diagnostics` (a read-only snapshot that works even
pre-active) and render its sections. Do NOT read `config.json` or shell-probe
(openssl/nc) — diagnostics is the single source and already classifies failures.

If diagnostics reports no config (`state` absent and `console.configured` is
false): show "Not configured — run `/rune:configure`." and stop.

## Report

Use ✓ for healthy, ✗ for problems.

```
Rune Status
===========
State      : <state>   (+ dormant_reason / dormant_since when dormant)
Console    : ✓ healthy (<endpoint>) | ✗ <error>
Keys       : ✓ key_id=<key_id> (custodian: console) | ✗ not loaded
Embedder   : <see rules>  (model=<model>, dim=<vector_dim>, up=<uptime>, reqs=<n>)
             socket: <socket_path>
```

**Embedder** (`embedding.status`):
- `OK` → ✓.
- `IDLE` → ⏸ healthy, not a fault: the daemon paused its llama-server after an
  idle period and resumes on the next capture/recall. Surface `embedding.message`
  so the user knows nothing is broken.
- `LOADING` → show `phase` + `bytes_done`/`bytes_total` in MB (it is downloading).
- `DEGRADED` / `SHUTTING_DOWN`, or any `info_error` / `health_error` → ✗ with the error.
- Empty `model` and `socket_path` → "not initialized".

## Recommendations

When `console.last_boot_error` is set — it can be present even while `state` is
`active`, a transient failure the boot loop is retrying — relay its `hint`
verbatim as the recommendation. The boot loop already classified the root cause;
do not guess or re-derive it. Add `detail` only when the hint is generic.
