---
description: Activate Rune — resume from dormant and bring Console pipelines online
allowed-tools: Bash(~/.rune/bin/rune install:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune install:*), mcp__plugin_rune_rune__activate
---

# /rune:activate — Bring Rune Online

Resume Rune from dormant (or finish first-time boot) using the existing config.
Credentials are untouched — this only flips Rune to active.

## Steps

Call `mcp__plugin_rune_rune__activate` and branch on `status`. Relay the tool's
`hint` verbatim — it already names the exact next step; do not shell-probe
(openssl/nc) to second-guess the classifier.

- **`active`** — pipelines are up. Report "Rune is active. Organizational memory
  is online."
- **`configure_required`** — no credentials. Relay `hint` and stop (points at
  `/rune:configure`).
- **`install_pending`** — the runed daemon isn't reachable. Relay `hint`, run
  the agent-recovery command it names (`rune install`), then retry
  `/rune:activate` once. The user never runs `rune install` themselves.
- **`waiting_for_bootstrap`** — runed is downloading its model (one-time, after
  a fresh install). Summarize `bootstrap`: the `phase`, plus
  `bytes_done`/`bytes_total` in MB when `bytes_total > 0`. No action needed — a
  background watcher completes activation, and `/rune:capture` / `/rune:recall`
  will succeed once it reaches active. Do NOT poll or re-run.
- **`waiting_for_console` / `dormant`** — boot ran but didn't reach active.
  Relay `reload.last_boot_error.hint` verbatim as one block, plus a single
  re-run suggestion (`/rune:configure` for credential errors, `/rune:activate`
  after fixing substrate). Do NOT loop.
