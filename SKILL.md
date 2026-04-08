---
name: rune
description: "Captures, stores, and retrieves FHE-encrypted organizational memories (decisions, rationale, lessons learned) via Rune's MCP tools. Use when the user invokes /rune commands, asks to store or recall team decisions, references organizational memory, or discusses past choices that may have prior context."
---

# Rune - Organizational Memory System

Encrypted organizational memory using FHE (Fully Homomorphic Encryption). Captures team decisions, retrieves institutional knowledge, and maintains zero-knowledge privacy across Claude Code, Codex CLI, Gemini CLI, and any MCP-compatible agent.

## Execution Model

- `scripts/bootstrap-mcp.sh` is the **single source of truth** for runtime preparation (venv, deps, self-healing). All agent integrations reuse this bootstrap flow.
- **Agent-specific adapters** handle only registration (e.g., `codex mcp add` for Codex, native MCP registration for Claude/Gemini). Never mix Codex-only commands into cross-agent instructions.

## Activation State

Two states: **Active** or **Dormant**, determined by local config checks only.

### Activation Check (Run Every Session Start)

**No network calls during this check.**

0. **Runtime check**: Locate `scripts/bootstrap-mcp.sh` by searching (in order): `$RUNE_PLUGIN_ROOT`, `~/.claude/plugins/cache/*/rune/*/scripts/`, `~/.codex/skills/rune/scripts/`, then CWD and parents. Run `SETUP_ONLY=1 scripts/bootstrap-mcp.sh`. If it fails → Dormant.
1. **Config check**: Does `~/.rune/config.json` exist? NO → Dormant.
2. **Validation**: Config must have `vault.endpoint`, `vault.token`, `envector.endpoint`, `envector.api_key`, and `state: "active"`. Missing any → Dormant.
3. **State**: `state` is `"active"` → Active. Otherwise → Dormant.

### Active State

- All MCP tools enabled. Automatically capture significant context and respond to recall queries.
- **On failure**: Immediately switch to Dormant, update `config.json` state to `"dormant"`, notify user once: "Infrastructure unavailable. Switched to dormant mode. Run /rune:status for details." Do not retry.

### Dormant State

- Do NOT attempt capture, retrieval, or network requests.
- When `/rune` commands are used, show setup instructions: run `scripts/check-infrastructure.sh`, then `/rune:configure`, then `scripts/start-mcp-servers.sh`.

## MCP Tools

The MCP server (`mcp/server/server.py`) exposes these tools. Use them when Active:

| Tool | Purpose | Key Parameters |
|------|---------|---------------|
| `capture` | Store organizational context with metadata | `text` (content), `title`, `domain` |
| `recall` | Semantic search over encrypted memories | `query` (natural language) |
| `vault_status` | Check Vault connectivity and health | — |
| `diagnostics` | Full system diagnostics | — |
| `capture_history` | List recent captures | `limit`, `domain`, `since` |
| `delete_capture` | Remove a stored capture by ID | `record_id` |
| `reload_pipelines` | Reload Scribe/Retriever pipelines | — |

## Commands

### `/rune:configure`

Collect credentials and validate infrastructure:

1. Ask for **enVector Endpoint** (required, e.g., `cluster-xxx.envector.io`) and **API Key** (required, e.g., `envector_xxx`).
2. Ask for **Vault Endpoint** (optional, e.g., `tcp://vault-TEAM.oci.envector.io:50051`) — auto-prepend `tcp://` if no scheme given. Ask for **Vault Token** (optional, e.g., `evt_xxx`).
3. If both Vault fields provided, ask **TLS mode**:
   - **Self-signed CA**: Get cert path, copy to `~/.rune/certs/ca.pem` (chmod 600) → `tls_disable: false`
   - **Public CA** (default): No extra input → `tls_disable: false`
   - **No TLS**: Warn about plaintext traffic → `tls_disable: true`
   - If Vault fields skipped, plugin starts Dormant.
4. Validate via `scripts/check-infrastructure.sh`. Create `~/.rune/config.json` with `state: "active"` or `"dormant"` based on result.

### `/rune:status`

Check config existence, show Active/Dormant state, run infrastructure checks (config file, Vault endpoint, enVector endpoint, MCP server logs, venv). Display checklist with recommendations.

### `/rune:capture <context>`

Manual override to store context when automatic capture missed it. If Dormant, prompt to configure. If Active, call the `capture` MCP tool.

```
/rune:capture "We chose PostgreSQL over MongoDB for better ACID guarantees"
```

### `/rune:recall <query>`

Explicit memory search. If Dormant, prompt to configure. If Active, call the `recall` MCP tool. Note: natural questions like "Why did we choose PostgreSQL?" trigger Retriever automatically.

```
/rune:recall "Why PostgreSQL?"
```

### `/rune:activate` (or `/rune:wakeup`)

Re-validate infrastructure after deployment. This is the **only** command that makes network requests. If checks pass, set state to Active. If config missing, redirect to `/rune:configure`.

### `/rune:reset`

Confirm with user, stop MCP servers, delete `~/.rune/config.json`, return to Dormant.

## Automatic Behavior (When Active)

### Context Capture (Scribe)

Automatically identifies and stores significant decisions, rationale, and lessons learned. Redacts API keys, passwords, tokens, and PII before storage. Significance threshold: 0.7.

See [patterns/capture-triggers.md](patterns/capture-triggers.md) for 200+ trigger phrases by role and domain.

### Context Retrieval (Retriever)

Automatically detects recall-intent questions about past decisions and searches encrypted memory. Returns results ranked by relevance and recency with source attribution.

See [patterns/retrieval-patterns.md](patterns/retrieval-patterns.md) for 150+ query patterns by intent and domain.

## Security

- All data stored as FHE-encrypted vectors — enVector Cloud cannot read plaintext.
- Credentials stored locally in `~/.rune/config.json` (permissions 600). Redact sensitive data before capture.
- Same Vault Endpoint + Token = shared team memory. Revoke access by rotating Vault tokens.

## References

- Troubleshooting: Run `/rune:status` for diagnostics, `scripts/check-infrastructure.sh` for full checks.
- Admin deployment: [rune-admin](https://github.com/CryptoLabInc/rune-admin) and its [deployment guide](https://github.com/CryptoLabInc/rune-admin/blob/main/deployment/README.md).
