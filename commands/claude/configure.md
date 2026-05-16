---
description: Configure Rune — collect Vault credentials and write ~/.rune/config.json (Go v0.4)
allowed-tools: Bash(mkdir:*), Bash(chmod:*), Bash(cp:*), Bash(test:*), Read, Write, AskUserQuestion, Edit, mcp__envector__reload_pipelines, mcp__envector__diagnostics
---

# /rune:configure — Setup & Configuration

Single entry after `claude plugin install rune`. Collects Vault credentials,
writes `~/.rune/config.json`, and triggers `reload_pipelines` to bring Rune
online.

In v0.4 the MCP server is a single Go binary
(`${CLAUDE_PLUGIN_ROOT}/bin/rune-mcp`) that Claude Code auto-spawns from the
plugin manifest. There is no Python venv, no install script, and no separate
`claude mcp add` step.

## Quick Update Mode

If $ARGUMENTS contains any of: `--vault-token`, `--vault-endpoint`:

1. Read existing `~/.rune/config.json`.
   - If not found: respond "Not configured yet. Run `/rune:configure` without arguments first." and stop.
2. Update only the specified field(s):
   - `--vault-token <value>` → `vault.token`
   - `--vault-endpoint <value>` → `vault.endpoint` (auto-prepend `tcp://` if no scheme)
3. Write back to `~/.rune/config.json` with `chmod 600`.
4. Update `metadata.lastUpdated` to current ISO timestamp.
5. Call `reload_pipelines` to apply.
6. Show: "Updated [field]. Use `/rune:status` to verify."

Skip all steps below.

---

## Full Setup Steps

**Turn budget**: 4 main turns total. Aggressively batch into **single
AskUserQuestion calls** and **parallel tool_use blocks** to keep wall-clock
+ token cost low. Concrete plan below.

### 1. Probe existing state (one turn)

In a single turn, run `Read` on `~/.rune/config.json` (if it exists).
This serves two purposes at once:

- Detect whether the user is re-configuring (existing values) or fresh-setup.
- Satisfy the `Write` tool's "must Read before Write" requirement so Step 3
  does NOT need a separate defensive Read turn later. If the file does
  not exist, the Read fails harmlessly — skip and proceed.

If the existing config has all credentials and the user does not say they
want to reconfigure: skip to Step 4 (reload only). Otherwise continue.

### 2. Collect credentials — **one AskUserQuestion call, three questions** (one turn)

Issue a SINGLE `AskUserQuestion` with three bundled questions, not three
separate calls. The tool accepts 1–4 questions per call; using one call
saves 2 turns (each separate call costs ~5s wall + an extra round-trip of
~50k cache_read tokens).

Questions to bundle:

1. **Vault Endpoint** (required, format: `tcp://<host>:50051`).
   Auto-prepend `tcp://` when the user value omits the scheme.
2. **Vault Token** (required, format: `evt_xxx...`).
3. **TLS Mode**:
   - `self-signed` — team uses a self-signed CA (Recommended).
   - `public_ca` — Let's Encrypt etc., system CA pool handles verification.
   - `no_tls` — local dev only; Vault must also be running with
     `server.grpc.tls.disable: true`. Warn if selected.

**If `self-signed` was chosen**: issue a single follow-up `AskUserQuestion`
asking only "Path to CA certificate PEM file:" (one question, one turn).
Otherwise skip the follow-up entirely.

Resulting config mapping:

| TLS mode    | ca_cert                    | tls_disable |
|-------------|----------------------------|-------------|
| self-signed | `<HOME>/.rune/certs/ca.pem`| false       |
| public_ca   | ""                         | false       |
| no_tls      | ""                         | true        |

### 3. Provision and write — **parallel tool_use in one turn**

Emit BOTH tool calls in the same turn (parallel `tool_use` blocks). Anthropic
runs them in parallel and returns both `tool_result`s in the next user
message:

- **`Bash`** — only when `tls_mode == self-signed`. Sets up the cert dir
  and copies the user-supplied CA in one command (so a partial failure
  leaves nothing half-written):
  ```bash
  mkdir -p ~/.rune/certs && chmod 700 ~/.rune && \
    cp <user_ca_path> ~/.rune/certs/ca.pem && \
    chmod 600 ~/.rune/certs/ca.pem
  ```
  If `cp` fails (file not found / permission denied): surface the error
  and ask the user for a readable path (one more `AskUserQuestion`).
- **`Write`** — `~/.rune/config.json` with the JSON below.
  ```json
  {
    "vault": {
      "endpoint": "<vault_endpoint>",
      "token": "<vault_token>",
      "ca_cert": "<ca_cert_path or empty>",
      "tls_disable": <true|false>
    },
    "state": "active",
    "metadata": {
      "configVersion": "2.0",
      "lastUpdated": "<ISO timestamp>"
    }
  }
  ```

Note: enVector credentials (endpoint, API key, EvalKey, SecKey) are not
stored locally — Vault delivers them via the agent manifest on first
connection.

### 4. Lock down + trigger boot — **parallel tool_use in one turn**

Again emit both tool calls in the same turn:

- **`Bash`** — `chmod 600 ~/.rune/config.json` (config file may contain the
  vault token; ensure owner-only readable).
- **`mcp__envector__reload_pipelines`** — kicks the boot loop. Returns
  after up to 5s while the loop attempts to reach Active, so the response
  may already contain `last_boot_error` on failure (see Step 5).

The two are independent: the chmod doesn't affect the boot loop's read,
and the boot loop will have already loaded the config before chmod
finishes most of the time. Running them in parallel is safe and saves
one round-trip.

### 5. Read the response — **fast-fail on `last_boot_error`**

Inspect the `reload_pipelines` response from Step 4 first:

- **`state == "active"` AND no `last_boot_error`** → success path. Optionally
  call `mcp__envector__diagnostics` ONCE for the rich per-subsystem snapshot
  used in Step 6's completion summary. (You can skip diagnostics if you only
  need to confirm activation — `reload_pipelines.state` is authoritative.)

- **`last_boot_error` is populated** → fast-fail. Use it directly; do NOT
  call diagnostics, do NOT retry. The boot loop has already classified the
  root cause.

- **`state != "active"` AND `last_boot_error` is absent** → boot loop is
  still in flight (rare; only when the 5s wait window expired without an
  error). Call `mcp__envector__diagnostics` ONCE — its
  `vault.last_boot_error` will likely be populated by now (the boot loop
  keeps running in the background).

**Do NOT** retry `reload_pipelines`, poll `diagnostics`, or probe with shell
commands. The `last_boot_error` field is the boot loop's structured verdict.

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
| `vault_timeout`       | Vault didn't respond in time. Could be network or server overload — show `hint`. |
| `vault_manifest`      | Vault connected but no manifest for this token. Token probably not provisioned for an agent. |
| `vault_rate_limit`    | Token throttled. Show `hint`. Wait and retry. |
| `vault_bad_endpoint`  | Endpoint syntax invalid. Show `hint`. Re-run `/rune:configure` with corrected format. |
| `embedder_unreachable`| `runed` daemon not running. Show `hint`. User should run `runed start`. |
| `envector_init` / `envector_index` | Envector side. Show `hint` + `detail`. |
| `key_save` / `local_io` | Local FS issue. Show `hint` + suggest checking `~/.rune/` permissions. |
| anything else (incl. `unknown`) | Show `kind`, `hint`, and `detail`. Suggest user share the detail with their Vault admin. |

The agent-facing output for a fast-fail case should be **one block**: the
matched explanation above + the hint string verbatim + a single next-action
suggestion. Do NOT loop on `reload_pipelines`. Do NOT call shell tools to
verify (`openssl`, `nc`, etc.) unless the user explicitly asks — the
classifier has already done that work server-side.

**If `state == "active"` (success path):** proceed to Step 6.

The diagnostics result has these sections (only render the ones with
meaningful content) — used for the success summary in Step 6:

- `state` + `dormant_reason` + `dormant_since`
- `vault.healthy` + `vault.endpoint` (+ `vault.error` if unhealthy)
- `vault.last_boot_error` (only when state != active — see fast-fail above)
- `keys.enc_key_loaded` + `keys.key_id` + `keys.agent_dek_loaded`
- `pipelines.scribe_initialized` + `pipelines.retriever_initialized`
- `embedding.model` + `embedding.mode` + `embedding.vector_dim` (+ `embedding.daemon_version` if present)
- `envector.reachable` + `envector.latency_ms` (+ `envector.error` / `envector.hint` if not)

### 6. Completion Summary

Render the snapshot in this layout (use ✓ for healthy, ✗ for failures
with the specific error on the same line; omit a row when the field
isn't populated):

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
