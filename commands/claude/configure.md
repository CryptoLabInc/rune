---
description: Configure Rune — take the registration string from the invite email, then configure and activate. Prompts before overwriting an existing setup.
allowed-tools: Bash(mkdir:*), Bash(cp:*), Read, mcp__plugin_rune_rune__configure, mcp__plugin_rune_rune__activate
---

# /rune:configure — Setup & Reconfigure

Bring Rune online from the registration string in the user's invite email:
collect it, call `configure`, then `activate`. TLS is always on — there is no
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

## 3. Configure, then activate

1. Call `configure` with `{ "registration_string": "runev1_…" }`. The server
   runs the 3-stage bootstrap (decode → fetch + pin the Console CA → unwrap the
   one-time handle into the real token), writes `~/.rune/config.json`, and probes.
   - On `error.code == "REGISTRATION_CONSUMED"`: the one-time handle was spent
     but the local write failed. Relay `error.recovery_hint`; the user needs a
     fresh invite. Do NOT retry the same string.
2. Then call `activate` to bring pipelines up. Relay its result — `activate`
   owns the recovery hints (`hint` / `last_boot_error.hint`); surface them
   verbatim and do not shell-probe to second-guess the classifier.
3. On success: "Rune configured and activated." Suggest `/rune:status` later.

## Manual fallback (no registration string)

Only when the user has raw values from a Console admin instead of a string:
copy a self-signed CA into place if one was given
(`mkdir -p ~/.rune/certs && cp <path> ~/.rune/certs/ca.pem && chmod 600 ~/.rune/certs/ca.pem`),
then call `configure` with `{ endpoint, token, ca_cert_path? }` — omit
`ca_cert_path` for a public/system-CA endpoint — and continue at step 3.2.
