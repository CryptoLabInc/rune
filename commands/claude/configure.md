---
description: Configure Rune — collect Vault credentials and write ~/.rune/config.json
allowed-tools: Bash(cp:*), Bash(~/.rune/bin/rune install:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune install:*), Read, AskUserQuestion, mcp__plugin_rune_rune__configure, mcp__plugin_rune_rune__activate, mcp__plugin_rune_rune__diagnostics
---

# /rune:configure — Setup & Configuration

Single entry after `claude plugin install rune`. Collects Vault credentials,
calls `mcp__plugin_rune_rune__configure` (atomic 0600 write + soft Vault probe), and
hands off to `mcp__plugin_rune_rune__activate` to bring pipelines online.

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

If $ARGUMENTS contains any of: `--vault-token`, `--vault-endpoint`:

1. `Read ~/.rune/config.json`.
   - Not found: respond `"Not configured yet. Run /rune:configure without arguments first."` and stop.
2. Merge the partial update into the existing values:
   - `--vault-token <value>`: use as the new `token`, keep existing `endpoint`/`ca_cert`/`tls_disable`.
   - `--vault-endpoint <value>`: auto-prepend `tcp://` if no scheme, keep existing `token`/`ca_cert`/`tls_disable`.
3. Call `mcp__plugin_rune_rune__configure` with the merged values. Server-side
   handles atomic write + 0600 perms + `metadata.lastUpdated` refresh +
   the soft Vault probe.
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
`evt_...` token, and (for self-signed) a `ca.pem` - all from their Vault
admin. The agent's job is to collect and format-check those values, NOT to
invent `localhost` defaults. Give each question exactly two paths: "I have
it (paste below)" or "I don't have it yet (stop)".

Questions (use this exact option intent - do not synthesize `tcp://localhost`-style defaults):

1. **Vault Endpoint** (required, format: `tcp://<host>:50051`; auto-prepend `tcp://` if the scheme is omitted).
   - "Paste endpoint below":  paste the `tcp://host:port` value from your admin into the Other field.
   - "I don't have one yet": stop; request the endpoint from your Vault admin first.
2. **Vault Token** (required, format: `evt_xxx...`).
   - "Paste token below": paste the `evt_...` token from your admin.
   - "I don't have one yet": stop; request a token from your Vault admin first.
3. **TLS Mode**:
   - `self-signed`: team uses a self-signed CA (Recommended).
   - `public_ca`: Let's Encrypt etc.; system CA pool handles verification.
   - `no_tls`: local dev only; Vault must also be running with `server.grpc.tls.disable: true`. Warn if selected.

**If `self-signed` was chosen**: follow-up `AskUserQuestion` with the single
question "Path to CA certificate PEM file:" - offer "Paste path below"
(`~` expansion supported) and "I don't have it yet" (stop; request `ca.pem` from your admin).
Otherwise skip the follow-up.

**On any "I don't have it yet" answer**: stop the flow immediately.
Tell the user exactly what to request from their Vault admin (endpoint / `evt_...` token / `ca.pem`),
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
recovery: `mkdir -p ~/.rune/certs && sudo cp /opt/runevault/certs/ca.pem ~/.rune/certs/ca.pem && sudo chown $USER ~/.rune/certs/ca.pem`.

### 4. Call `mcp__plugin_rune_rune__configure`

```jsonc
{
  "endpoint": "<vault_endpoint>",
  "token": "<vault_token>",
  "ca_cert_path": "<HOME>/.rune/certs/ca.pem"  // or "" if not self-signed
  "tls_disable": false                          // true only if no_tls
}
```

Server-side does:
- Atomic write to `~/.rune/config.json` with 0600 perms
- Sets `state: "active"`, clears any prior `dormant_reason` / `dormant_since`
- Refreshes `metadata.lastUpdated`
- Runs a best-effort 5s Vault dial + HealthCheck

Response:

```jsonc
{
  "ok": true,
  "path": "/home/.../.rune/config.json",
  "state": "active",
  "configured_at": "<ISO timestamp>",
  "next_step": "Run /rune:activate to apply the new credentials." | "Vault unreachable from this host - verify endpoint/token, then run /rune:activate to retry with backoff.",
  "vault_reachable": true | false,
  "probe_error": "<dial / health error if vault_reachable=false>"
}
```

### 5. Decide what to do next based on the probe

**`vault_reachable: true`** - credentials look good. Call
`mcp__plugin_rune_rune__activate` to bring pipelines up. Proceed to Step 6.

**`vault_reachable: false`** - early warning. The file IS written and
`state` IS active, but the probe couldn't dial Vault. Two ways to proceed:

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
| `vault_tls_handshake` | CA cert mismatch. Show `hint` verbatim. Ask user to re-fetch the current CA from the Vault admin and replace `~/.rune/certs/ca.pem`, then re-run `/rune:configure`. |
| `vault_tls_hostname`  | Server cert doesn't cover the endpoint hostname. Show `hint`. |
| `vault_ca_file`       | CA file path unreadable. Show `hint` — likely a typo or permissions. |
| `vault_auth`          | Token rejected. Show `hint`. Suggest `runevault token issue --user <name> --role member`. |
| `vault_permission`    | Token lacks role. Show `hint`. Re-issue with correct role. |
| `vault_network`       | Endpoint unreachable. Show `hint`. User should verify TCP connectivity (e.g., `nc -vz host port`). |
| `vault_dns`           | Hostname doesn't resolve. Show `hint`. Likely a typo in endpoint. |
| `vault_timeout`       | Vault didn't respond in time — show `hint`. |
| `vault_manifest`      | Vault connected but no manifest for this token. Token probably not provisioned for an agent. |
| `vault_rate_limit`    | Token throttled. Show `hint`. Wait and retry. |
| `vault_bad_endpoint`  | Endpoint syntax invalid. Show `hint`. Re-run `/rune:configure` with corrected format. |
| `embedder_unreachable`| `runed` daemon not running. Show `hint`. Re-run `/rune:activate` to (re)spawn the daemon; if it persists, the agent runs the Preflight install, then `/rune:activate`. |
| `envector_init` / `envector_index` | Envector side. Show `hint` + `detail`. |
| `key_save` / `local_io` | Local FS issue. Show `hint` + suggest checking `~/.rune/` permissions. |
| anything else (incl. `unknown`) | Show `kind`, `hint`, and `detail`. Suggest user share the detail with their Vault admin. |

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
  Vault         : <endpoint>
  TLS           : <enabled (system CA) | enabled (custom CA: <path>) | disabled>
  State         : <active | dormant: <reason>>

  Vault         : ✓ healthy / ✗ <error>
  Encryption    : ✓ loaded (key_id: <id>) / ✗ not loaded
  Agent DEK     : ✓ loaded / ✗ not loaded
  Scribe        : ✓ initialized / ✗ not initialized
  Retriever     : ✓ initialized / ✗ not initialized
  Embedder      : ✓ <model> (<mode>, dim=<vector_dim>) / ✗ not initialized
  enVector      : ✓ reachable (<latency_ms>ms) / ✗ <error> — <hint>

Next steps:
  - /rune:status      — re-check pipeline health later
  - /rune:capture     — capture your first decision
  - /rune:recall      — query organizational memory
```
