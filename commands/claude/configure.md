---
description: Configure Rune — take the registration string from the invite email; it bootstraps the credentials and brings Rune online. Prompts before overwriting an existing setup.
argument-hint: <runev1_… registration string>
allowed-tools: Read, Bash(~/.rune/bin/rune install:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune install:*), mcp__plugin_rune_rune__configure, mcp__plugin_rune_rune__activate
---

# /rune:configure — Setup & Reconfigure

Bring Rune online from the registration string in the user's invite email:
collect it and call `mcp__plugin_rune_rune__configure` — that one call bootstraps
the credentials and brings the pipelines online. TLS is always on — there is no
plaintext mode.

## 1. Existing setup? (reconfigure gate)

`Read ~/.rune/config.json`:

- Missing, or `console.endpoint` empty → fresh install. Go to step 2.
- `console.endpoint` set → already configured. Ask the user to confirm:
  "Rune is already configured (endpoint: `<endpoint>`). Reconfigure with a new
  registration string?"
  - Declines → stop: "Keeping the existing configuration." (Suggest
    `/rune:status` to check health, or `/rune:activate` if it is dormant.)
  - Confirms → continue. Reconfigure resets the old credentials server-side and
    re-bootstraps from the new string.

## 2. Get the registration string

A registration string is a single `runev1_…` token from the invite email — it
is all that's needed (endpoint, token, and CA are derived from it).

- If `$ARGUMENTS` contains a `runev1_…` token, use it.
- Otherwise ask once: "Paste your Rune registration string (starts with
  `runev1_`)."
- Treat it as a credential — never echo it back.

## 3. Configure (bootstraps + activates)

1. Call `mcp__plugin_rune_rune__configure` with
   `{ "registration_string": "runev1_…" }`. The server runs the 3-stage
   bootstrap (decode → fetch + pin the Console CA → unwrap the one-time handle
   into the real token), writes `~/.rune/config.json`, and then drives the boot
   loop to bring the pipelines online — configure owns activation, so a separate
   `/rune:activate` is not needed. It returns the real `state` and a `next_step`.
2. Branch on the result:
   - `error.code == "REGISTRATION_CONSUMED"` — the one-time handle was spent but
     persisting the credentials failed (disk/permissions, or a locked keyring).
     Relay `error.recovery_hint`; the user needs a fresh invite. Do NOT retry the
     same string.
   - `state == "active"` — "Rune configured and activated. Organizational memory
     is online." Suggest `/rune:status` later.
   - `state == "waiting_for_bootstrap"` — credentials are in effect and runed is
     downloading its embedding model (one-time, after a fresh install). Relay
     `next_step` — a background watcher completes activation on its own. Do NOT
     poll or re-run anything.
   - `next_step` names an agent-recovery `rune install` command — runed could
     not be spawned because it is not installed yet. The invite is already
     redeemed and the credentials are saved: run the named `rune install`
     command via Bash, then call `mcp__plugin_rune_rune__activate` once and
     branch on its result like `/rune:activate` does. The user never runs
     `rune install` themselves. Do NOT retry `configure` — the registration
     string is spent.
   - any other `state` (e.g. `waiting_for_console`) — the invite is already
     redeemed and the credentials are saved, so this is a connectivity/boot
     problem, never a spent-invite one. Relay `next_step` verbatim (it carries
     the boot error hint) and suggest `/rune:activate` to retry or `/rune:status`
     to inspect. Do NOT request a new invite. Do not shell-probe to second-guess
     the classifier.
