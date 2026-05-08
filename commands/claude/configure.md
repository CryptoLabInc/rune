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

### 1. Collect Credentials (conversational, via AskUserQuestion)

If `~/.rune/config.json` already exists, show current values (mask token), ask
if the user wants to reconfigure. If they decline, skip to Step 4 (reload only).

Ask each credential one at a time:

- **Vault Endpoint** (required, format: `tcp://<host>:50051`).
  Auto-prepend `tcp://` when the user enters a value without a scheme prefix.
  Example: `vault.example.com:50051` → `tcp://vault.example.com:50051`
- **Vault Token** (required, format: `evt_xxx...`).

### 2. TLS Choice

Ask: "How does your Vault server handle TLS?"

1. **Self-signed certificate** (Recommended) — team uses a self-signed CA.
   - Follow-up: "Path to CA certificate PEM file:" (support `~` expansion).
   - Copy via:
     ```bash
     mkdir -p ~/.rune/certs && chmod 700 ~/.rune
     cp <user_path> ~/.rune/certs/ca.pem
     chmod 600 ~/.rune/certs/ca.pem
     ```
   - If the cp fails (file not found, permission denied), surface the error
     and ask for a path the current user can read. Common fix:
     `sudo cp /opt/runevault/certs/ca.pem ~/.rune/ca.pem && sudo chown $USER ~/.rune/ca.pem`.
   - → config: `ca_cert: "<HOME>/.rune/certs/ca.pem"`, `tls_disable: false`
2. **Public CA** (Let's Encrypt etc.) — system CA pool handles verification.
   - → config: `ca_cert: ""`, `tls_disable: false`
3. **No TLS** — local dev only. Vault must also be running with
   `server.grpc.tls.disable: true` — `install-dev.sh`'s default does NOT do
   this, so this option only works when the user has explicitly disabled TLS
   on the Vault side.
   - Warn: "Plaintext gRPC. Make sure your Vault server has tls.disable: true."
   - → config: `ca_cert: ""`, `tls_disable: true`

### 3. Write ~/.rune/config.json

```bash
mkdir -p ~/.rune && chmod 700 ~/.rune
```

Write:
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

Then `chmod 600 ~/.rune/config.json`.

Note: enVector credentials (endpoint, API key, EvalKey, SecKey) are not
stored locally — Vault delivers them via the agent manifest on first
connection.

### 4. Trigger Boot Loop

Call `mcp__envector__reload_pipelines`. This re-runs the boot loop:

1. Dial Vault (TLS or plaintext per config) and call `GetAgentManifest`.
2. Persist the returned `EncKey` to `~/.rune/keys/<keyID>/EncKey.json`.
3. Dial runed at `~/.runed/embedding.sock` (or `RUNE_EMBEDDER_SOCKET`).
4. Connect to enVector cluster using the bundle endpoint + API key.
5. Open the team index.

If `reload_pipelines` returns an error or state stays dormant:
- Surface the message — it is the boot loop's `LastError`.
- Common causes:
  - Vault unreachable — verify `endpoint` reachable from this host.
  - Token invalid / role lacks permission — re-issue with
    `runevault token issue --user <name> --role member`.
  - runed not running — start the embedding daemon.
  - enVector cluster unreachable — check the cluster's external
    connectivity from this host.
- Suggest `/rune:status` for the full health snapshot.

### 5. Verify Health (call diagnostics)

After `reload_pipelines` returns, call `mcp__envector__diagnostics` to
get a real-time per-subsystem health snapshot. This is what the
completion summary renders — `reload_pipelines` is the trigger,
`diagnostics` is the ground-truth probe.

The diagnostics result has these sections (only render the ones with
meaningful content):

- `state` + `dormant_reason` + `dormant_since`
- `vault.healthy` + `vault.endpoint` (+ `vault.error` if unhealthy)
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
