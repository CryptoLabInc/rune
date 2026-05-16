---
description: Activate Rune (resume from dormant) and verify pipelines come up healthy
allowed-tools: Read, Edit, mcp__envector__reload_pipelines, mcp__envector__diagnostics, mcp__envector__vault_status
---

# /rune:activate — Activate Plugin

Resume Rune from a dormant state and verify the boot loop reaches Active.

In v0.4 the MCP server is a single Go binary auto-spawned by Claude Code
from the plugin manifest. The boot loop runs as soon as `state == "active"`
in `~/.rune/config.json`, so `/rune:activate`'s job is to flip state to
active, ask the server to re-run the boot loop, and confirm health.

## Steps

1. Read `~/.rune/config.json`.
   - Not found: respond "Not configured. Run `/rune:configure` first." and stop.

2. Verify required fields:
   - `vault.endpoint` and `vault.token` must be present.
   - Missing: report which fields are missing and suggest `/rune:configure`.
   - enVector credentials are delivered via the Vault bundle at runtime —
     they are not stored locally.

3. If `state` is already `"active"`, skip to Step 5 (just verify health).

4. If `state` is `"dormant"`, update the config:
   - Set `state` to `"active"`.
   - Remove any `dormant_reason` and `dormant_since` fields.
   - Update `metadata.lastUpdated` to the current ISO timestamp.

5. Call `mcp__envector__reload_pipelines`. This re-spawns the boot loop:
   - Dial Vault → `GetAgentManifest` → persist `EncKey` to disk
   - Dial runed → connect to enVector → open the team index
   - Transition state to Active

6. Call `mcp__envector__diagnostics` (fall back to `vault_status` if
   diagnostics is unavailable) and render a per-subsystem report:

   ```
   Infrastructure Validation
   =========================
   - Vault           : reachable (<endpoint>)
   - Encryption Key  : loaded (key_id: <id>)
   - Embedder        : ready
   - enVector Cloud  : reachable (<latency>ms)
   - Pipeline State  : Active
   ```
   Use a check mark for healthy items, "x" for failures with the specific
   message on the same line.

7. If the boot loop succeeded (`state == "active"` and all subsystems healthy):
   - Respond: "Rune activated. Organizational memory is now online."

8. **If boot failed (state != active)** — fast-fail on `vault.last_boot_error`:
   - Read `diagnostics.vault.last_boot_error`. If present, it already contains
     the boot loop's classified root cause + a user-actionable hint.
   - Render the recovery in **one block** — relay the `hint` verbatim and stop:
     ```
     Activation failed — <kind>
     <hint>

     Details: <detail>   (only when kind == "unknown" or hint is generic)
     Attempts: <attempts> (only when > 1)
     ```
   - Common `kind` mapping for the suggestion line:
     - `vault_tls_handshake` / `vault_tls_hostname` / `vault_ca_file` → user must
       fix the CA cert and re-run `/rune:activate`.
     - `vault_auth` / `vault_permission` → re-issue token with correct role,
       then `/rune:configure`.
     - `vault_network` / `vault_dns` → verify endpoint reachability (firewall,
       DNS), then re-run `/rune:activate`.
     - `embedder_unreachable` → start the `runed` daemon (`runed start`).
     - `envector_init` / `envector_index` → contact Vault admin to verify
       the envector endpoint / index provisioning.
     - `vault_manifest` → token is not provisioned for an active agent. Ask
       admin to bind the token to an agent.
   - DO NOT retry `reload_pipelines` automatically. DO NOT probe with shell
     tools. The classifier has already inspected the underlying error.
   - If `last_boot_error` is unexpectedly missing (older rune-mcp binary), fall
     back to the diagnostics `vault.error` / `embedding.health_error` /
     `envector.error` fields and surface those.

**Note**: This is a session-local resume — the MCP server stays the same
process. There is no Claude Code restart required (Task #28 wired the
reload to re-spawn the boot loop on dormant terminals).
