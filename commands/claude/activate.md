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

7. If the boot loop succeeded:
   - Respond: "Rune activated. Organizational memory is now online."

8. If any subsystem failed:
   - Show the full validation report.
   - Surface the specific recovery action per failure:
     - Vault unreachable: "Verify the Vault server is running and the endpoint is correct."
     - Vault token rejected: "Token may be expired — run `/rune:configure` to update."
     - runed not running: "Start the embedding daemon."
     - enVector unreachable: "Check the cluster's external connectivity from this host."
   - Suggest `/rune:status` for the full health snapshot.

**Note**: This is a session-local resume — the MCP server stays the same
process. There is no Claude Code restart required (Task #28 wired the
reload to re-spawn the boot loop on dormant terminals).
