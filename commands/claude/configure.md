---
description: Configure Rune — collect Console credentials and write ~/.rune/config.json
allowed-tools: Bash(cp:*), Bash(~/.rune/bin/rune install:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune install:*), Read, AskUserQuestion, mcp__plugin_rune_rune__configure, mcp__plugin_rune_rune__activate, mcp__plugin_rune_rune__diagnostics
---

# /rune:configure — Setup & Configuration

Single entry after `claude plugin install rune`. Collects credentials,
calls `mcp__plugin_rune_rune__configure` (atomic 0600 write + soft console probe), and
hands off to `mcp__plugin_rune_rune__activate` to bring pipelines online.

There are two ways to configure. **Prefer the registration string** — it is the
one-paste path a teammate gets by email; the manual endpoint/token flow below is
the fallback for operators who were handed raw values.

## Registration string path (recommended)

If the user has a **registration string** — a single opaque token that starts
with `runev1_`, delivered in their Rune invite email — that is all they need.
Do NOT ask for endpoint, token, or CA separately.

1. Get the string: if `$ARGUMENTS` already contains a `runev1_…` token, use it;
   otherwise ask once ("Paste your Rune registration string (starts with
   `runev1_`)"). Treat it as a **credential** — never echo it back.
2. Call `mcp__plugin_rune_rune__configure` with a single field:
   ```jsonc
   { "registration_string": "runev1_…" }
   ```
   The server runs the 3-stage bootstrap itself: decode → fetch the console CA
   over an untrusted channel and verify it against the pinned SHA-256 → unwrap
   the one-time handle into the real access token → write `~/.rune/config.json`
   (endpoint / token / pinned `ca_cert`) and probe. The one-time handle is
   consumed on first use, so a second `configure` with the SAME string will fail
   ("already used") — that is the tamper signal; the user must request a fresh
   invite.
3. Branch on the response exactly as in "Decide what to do next" (§5) below, then
   call `mcp__plugin_rune_rune__activate`. Skip the manual collection steps.

Fall through to the manual flow only when the user has no registration string.

The MCP server is a Go binary at `~/.rune/bin/rune-mcp`. The plugin manifest
spawns it via the committed bash wrapper `${CLAUDE_PLUGIN_ROOT}/bin/rune
mcp-server` (always present at session start). On a fresh install the wrapper
bootstraps the rune CLI and self-installs rune-mcp, then execs it — so the MCP
server comes online in the SAME session, with no restart.

## Preflight: the first MCP call self-installs

On a fresh `claude plugin install rune`, the first `mcp__plugin_rune_rune__*` call spawns
`${CLAUDE_PLUGIN_ROOT}/bin/rune mcp-server`, which self-installs rune-mcp
(downloading the CLI + rune-mcp if needed) and then serves — so the call is
EXPECTED to succeed in-session. On a cold download it may be slow (bounded by
the manifest's spawn timeout); that is normal, not an error.

You normally do NOT need to run anything here. ONLY if a `mcp__plugin_rune_rune__*` call
actually fails with a transport / connection / spawn error (e.g. the server
shows failed in `/mcp`) — a genuinely broken bootstrap — recover by running
ONE of these via the Bash tool, then retry the failed MCP call once:

1. **`~/.rune/bin/rune install`** - when the canonical Go binary already
   exists and is executable (steady state).
2. **`bash -c "${CLAUDE_PLUGIN_ROOT}/bin/rune install"`** - when
   `~/.rune/bin/rune` doesn't exist yet (the bash wrapper downloads the Go
   binary, then installs).

Surface the install output to the user verbatim. If the retry ALSO fails,
surface the error and stop — do NOT loop. The user never types `rune install`
themselves; this recovery is the agent's only sanctioned path.

## Quick Update Mode

If $ARGUMENTS contains any of: `--console-token`, `--console-endpoint`:

1. `Read ~/.rune/config.json`.
   - Not found: respond `"Not configured yet. Run /rune:configure without arguments first."` and stop.
2. Merge the partial update into the existing values:
   - `--console-token <value>`: use as the new `token`, keep existing `endpoint`/`ca_cert`/`tls_disable`.
   - `--console-endpoint <value>`: auto-prepend `tcp://` if no scheme, keep existing `token`/`ca_cert`/`tls_disable`.
3. Call `mcp__plugin_rune_rune__configure` with the merged values. Server-side
   handles atomic write + 0600 perms + `metadata.lastUpdated` refresh +
   the soft Console probe.
4. Call `mcp__plugin_rune_rune__activate` to apply.
5. Render: `"Updated [field]. Use /rune:status to verify."`

Skip all steps below.

---

## Full Setup Steps

**Turn budget**: ~3-4 turns total. Bundle questions into a single
`AskUserQuestion` call and pair the configure + activate calls when safe
to do so.

### 1. Probe existing state (one turn)

`Read ~/.rune/config.json`:

- File missing: fresh setup. Continue to Step 2.
- File present: mask the token (first 8 chars + "***") and show the
  current `endpoint`, `ca_cert`, `tls_disable`, `state`, masked token.
  Then issue a single `AskUserQuestion("Reconfigure these values?")`:
    - User declines: call `mcp__plugin_rune_rune__activate` and stop (just bring
      the existing config online).
    - User confirms: continue to Step 2 with the existing values as
      defaults the user can override.

### 2. Collect credentials — **one AskUserQuestion call, three questions** (one turn)

Issue a SINGLE `AskUserQuestion` with three bundled questions. The tool
accepts 1–4 questions per call; bundling saves 2 turns + ~50k cache_read
tokens per separated call.

**Mental model**: the user comes in with an admin-issued endpoint, an
`evt_...` token, and (for self-signed) a `ca.pem` - all from their Console
admin. The agent's job is to collect and format-check those values, NOT to
invent `localhost` defaults. Give each question exactly two paths: "I have
it (paste below)" or "I don't have it yet (stop)".

Questions (use this exact option intent - do not synthesize `tcp://localhost`-style defaults):

1. **Console Endpoint** (required, format: `tcp://<host>:50051`; auto-prepend `tcp://` if the scheme is omitted).
   - "Paste endpoint below":  paste the `tcp://host:port` value from your admin into the Other field.
   - "I don't have one yet": stop; request the endpoint from your Console admin first.
2. **Console Token** (required, format: `evt_xxx...`).
   - "Paste token below": paste the `evt_...` token from your admin.
   - "I don't have one yet": stop; request a token from your Console admin first.
3. **TLS Mode**:
   - `self-signed`: team uses a self-signed CA (Recommended).
   - `public_ca`: Let's Encrypt etc.; system CA pool handles verification.
   - `no_tls`: local dev only; Console must also be running with `server.grpc.tls.disable: true`. Warn if selected.

**If `self-signed` was chosen**: follow-up `AskUserQuestion` with the single
question "Path to CA certificate PEM file:" - offer "Paste path below"
(`~` expansion supported) and "I don't have it yet" (stop; request `ca.pem` from your admin).
Otherwise skip the follow-up.

**On any "I don't have it yet" answer**: stop the flow immediately.
Tell the user exactly what to request from their Console admin (endpoint / `evt_...` token / `ca.pem`),
point them at `setup/check-prerequisites.md`, and exit **without writing any files** - do not call `mcp__plugin_rune_rune__configure`.

Resulting argument mapping for the configure call:

| TLS mode    | ca_cert_path                | tls_disable |
|-------------|-----------------------------|-------------|
| self-signed | `<HOME>/.rune/certs/ca.pem` | false       |
| public_ca   | ""                          | false       |
| no_tls      | ""                          | true        |

### 3. (self-signed only) Copy the CA cert into place

When `tls_mode == self-signed`, run a single `Bash` command to copy the
user's CA into `~/.rune/certs/ca.pem`. The MCP tool doesn't move files
itself — the agent provides the final path to `ca_cert_path`:

```bash
mkdir -p ~/.rune/certs && cp <user_ca_path> ~/.rune/certs/ca.pem && chmod 600 ~/.rune/certs/ca.pem
```

If `cp` fails (file not found / permission denied), surface the error
and ask the user for a readable path (one more `AskUserQuestion`). Common
recovery: `mkdir -p ~/.rune/certs && sudo cp /opt/rune-console/certs/ca.pem ~/.rune/certs/ca.pem && sudo chown $USER ~/.rune/certs/ca.pem`.

### 4. Call `mcp__plugin_rune_rune__configure`

```jsonc
{
  "endpoint": "<console_endpoint>",
  "token": "<console_token>",
  "ca_cert_path": "<HOME>/.rune/certs/ca.pem"  // or "" if not self-signed
  "tls_disable": false                          // true only if no_tls
}
```

Server-side does:
- Atomic write to `~/.rune/config.json` with 0600 perms
- Sets `state: "active"`, clears any prior `dormant_reason` / `dormant_since`
- Refreshes `metadata.lastUpdated`
- Runs a best-effort 5s Console dial + HealthCheck

Response:

```jsonc
{
  "ok": true,
  "path": "/home/.../.rune/config.json",
  "state": "active",
  "configured_at": "<ISO timestamp>",
  "next_step": "Run /rune:activate to apply the new credentials." | "Console unreachable from this host - verify endpoint/token, then run /rune:activate to retry with backoff.",
  "console_reachable": true | false,
  "probe_error": "<dial / health error if console_reachable=false>"
}
```

### 5. Decide what to do next based on the probe

**`console_reachable: true`** - credentials look good. Call
`mcp__plugin_rune_rune__activate` to bring pipelines up. Proceed to Step 6.

**`console_reachable: false`** - early warning. The file IS written and
`state` IS active, but the probe couldn't dial Console. Two ways to proceed:

  - **Common case (transient / first-time):** still call
    `mcp__plugin_rune_rune__activate`. The boot loop has retries with backoff,
    and the classified `last_boot_error` it produces will be richer than
    the probe error.
  - **Obvious typo case** (`probe_error` contains "no such host" /
    "connection refused" with a hostname the user can read and recognize
    as wrong): show the `probe_error` verbatim + suggest re-running
    `/rune:configure` with the corrected value, instead of activating.

If you do call `activate`, branch on its response - same logic as
`/rune:activate`'s skill. The full per-`kind` table is in §6 below for
the rare case `last_boot_error.hint` needs supplementation.

### 6. `last_boot_error.kind` table (reference)

Render based on `last_boot_error.kind`:

| kind | what to tell the user |
|---|---|
| `console_tls_handshake` | CA cert mismatch. Show `hint` verbatim. Ask user to re-fetch the current CA from the Console admin and replace `~/.rune/certs/ca.pem`, then re-run `/rune:configure`. |
| `console_tls_hostname`  | Server cert doesn't cover the endpoint hostname. Show `hint`. |
| `console_ca_file`       | CA file path unreadable. Show `hint` — likely a typo or permissions. |
| `console_auth`          | Token rejected (expired/revoked/consumed). Show `hint`. Ask the Console admin for a fresh invite and re-run `/rune:configure` with the new registration string. |
| `console_permission`    | Token's role lacks the needed scope, or the member has no write group. Show `hint`. Ask the Console admin to grant the right role/group, then retry. |
| `console_network`       | Endpoint unreachable. Show `hint`. User should verify TCP connectivity (e.g., `nc -vz host port`). |
| `console_dns`           | Hostname doesn't resolve. Show `hint`. Likely a typo in endpoint. |
| `console_timeout`       | Console didn't respond in time — show `hint`. |
| `console_manifest`      | Console connected but no manifest for this token. Token probably not provisioned for an agent. |
| `console_rate_limit`    | Token throttled. Show `hint`. Wait and retry. |
| `console_bad_endpoint`  | Endpoint syntax invalid. Show `hint`. Re-run `/rune:configure` with corrected format. |
| `embedder_unreachable`| `runed` daemon not running. Show `hint`. Re-run `/rune:activate` to (re)spawn the daemon; if it persists, the agent runs the Preflight install, then `/rune:activate`. |
| `runespace_init` / `runespace_index` | Index backend (runespace) side. Show `hint` + `detail`. |
| `key_save` / `local_io` | Local FS issue. Show `hint` + suggest checking `~/.rune/` permissions. |
| anything else (incl. `unknown`) | Show `kind`, `hint`, and `detail`. Suggest user share the detail with their Console admin. |

The agent-facing output for a fast-fail case should be **one block**: the
matched explanation above + the `hint` string verbatim + a single
next-action suggestion. Do NOT loop on `activate`. Do NOT call shell tools
to verify (`openssl`, `nc`, etc.) unless the user explicitly asks — the
classifier has already done that work server-side.

### 7. Completion Summary (success path)

When `activate.status == "active"`, optionally call
`mcp__plugin_rune_rune__diagnostics` once for the rich per-subsystem snapshot and
render:

```
Rune Configuration Complete
============================
  Config        : ~/.rune/config.json
  Plugin        : ${CLAUDE_PLUGIN_ROOT}
  Console         : <endpoint>
  TLS           : <enabled (system CA) | enabled (custom CA: <path>) | disabled>
  State         : <active | dormant: <reason>>

  Console         : ✓ healthy / ✗ <error>
  Encryption    : ✓ loaded (key_id: <id>) / ✗ not loaded
  Agent DEK     : ✓ loaded / ✗ not loaded
  Scribe        : ✓ initialized / ✗ not initialized
  Retriever     : ✓ initialized / ✗ not initialized
  Embedder      : ✓ <model> (<mode>, dim=<vector_dim>) / ✗ not initialized
  Index backend : ✓ reachable (<latency_ms>ms) / ✗ <error> — <hint>

Next steps:
  - /rune:status      — re-check pipeline health later
  - /rune:capture     — capture your first decision
  - /rune:recall      — query organizational memory
```
