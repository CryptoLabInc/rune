---
description: Configure Rune — sets up Python environment, collects credentials, registers MCP servers
allowed-tools: Bash(python3:*), Bash(find:*), Bash(cat ~/.rune/*), Bash(mkdir:*), Bash(chmod:*), Bash(bash:*), Bash(scripts/*), Bash(timeout:*), Bash(curl:*), Read, Write, AskUserQuestion, Edit, mcp__envector__reload_pipelines
---

# /rune:configure — Full Setup & Configuration

Single entrypoint after `claude plugin install rune`. Handles environment setup, credential collection, config generation, MCP server registration, and auto-activation.

## Quick Update Mode

If $ARGUMENTS contains any of: `--vault-token`, `--vault-endpoint`:

1. Read existing `~/.rune/config.json`
   - If not found: respond "Not configured yet. Run `/rune:configure` without arguments first." and stop.
2. Update only the specified field(s):
   - `--vault-token <value>` → `vault.token`
   - `--vault-endpoint <value>` → `vault.endpoint` (auto-prepend `tcp://` if no scheme)
3. Write back to `~/.rune/config.json` with `chmod 600`
4. Update `metadata.lastUpdated` to current ISO timestamp
5. If state is "active", call `reload_pipelines` MCP tool to apply changes
6. Show: "Updated [field]. Use `/rune:status` to verify."

Skip all steps below.

---

## Full Setup Steps

### 1. Detect Plugin Root

Find where the plugin is installed:

```bash
PLUGIN_ROOT=$(find ~/.claude/plugins/cache -name "plugin.json" -path "*/rune/*" -exec dirname {} \; 2>/dev/null | head -1 | xargs dirname 2>/dev/null)
```

- If empty: also check the current working directory and its parents for `.claude-plugin/plugin.json`
- If still not found: respond "Rune plugin not found. Run `claude plugin install rune` first." and stop.

### 2. Setup Python Environment (automatic)

Check if `$PLUGIN_ROOT/.venv` exists.

- If it exists: skip to Step 3.
- If missing: run `bash $PLUGIN_ROOT/scripts/install.sh`
  - This creates the venv and installs dependencies (non-interactive).
  - If it fails (e.g. Python < 3.12): show the error output and stop.
  - Show progress: "Setting up Python environment..."

### 3. Collect Credentials (conversational)

Check if `~/.rune/config.json` already exists.
- If it exists: read and show current values (masked API keys), ask user if they want to reconfigure.
- If user declines: skip to Step 5.

Ask user for each credential one at a time:
- **Vault Endpoint** (required, format: `tcp://vault-TEAM.oci.envector.io:50051`)
  - If the user enters a value without a scheme prefix (no `tcp://`, `http://`, or `https://`), auto-prepend `tcp://`.
  - Example: user enters `vault.example.com:50051` → store as `tcp://vault.example.com:50051`
- **Vault Token** (required, format: `evt_xxx`)

Then ask the TLS question:

**"How does your Vault server handle TLS?"**

1. **Self-signed certificate** — "My team uses a self-signed CA (provide CA cert path)"
   - Follow-up: "Enter the path to your CA certificate PEM file:"
   - Support `~` expansion in the path
   - Copy the file to `~/.rune/certs/ca.pem` (`cp <user_path> ~/.rune/certs/ca.pem && chmod 600 ~/.rune/certs/ca.pem`)
   - If copy fails (file not found, permission denied), show error and ask again
   - Inform user: "CA certificate copied to ~/.rune/certs/ca.pem"
   - → config: `ca_cert: "~/.rune/certs/ca.pem"`, `tls_disable: false`

2. **Public CA (default)** — "Vault uses a publicly-signed certificate (e.g., Let's Encrypt)"
   - No additional input needed, system CA handles verification
   - → config: `ca_cert: ""`, `tls_disable: false`

3. **No TLS** — "Connect without TLS (not recommended — traffic is unencrypted)"
   - Show warning: "This should only be used for local development. All gRPC traffic will be sent in plaintext."
   - → config: `ca_cert: ""`, `tls_disable: true`

### 4. Write ~/.rune/config.json

```bash
mkdir -p ~/.rune && chmod 700 ~/.rune
```

Write the config file:
```json
{
  "vault": {"endpoint": "<vault_endpoint>", "token": "<vault_token>", "ca_cert": "<ca_cert_path or empty>", "tls_disable": false},
  "state": "dormant",
  "metadata": {"configVersion": "2.0", "lastUpdated": "<ISO timestamp>", "installedFrom": "<PLUGIN_ROOT>"}
}
```

Note: enVector credentials are no longer stored locally — they are delivered automatically via the Vault bundle at session start.

Then: `chmod 600 ~/.rune/config.json`

### 5. Register MCP Servers

Run: `bash $PLUGIN_ROOT/scripts/configure-claude-mcp.sh`

This registers the envector MCP server via `claude mcp add --scope user` (Claude Code) and JSON merge (Claude Desktop).

### 6. Auto-Activate (if Vault configured)

Vault endpoint and token are always provided (required). Proceed to auto-activate:

1. Run infrastructure validation:
   - Check Vault connectivity by parsing the scheme from `vault.endpoint`:
     - If `http://` or `https://`: `curl -sf <vault-endpoint>/health`
     - If `tcp://`: extract host and port, then test TCP connectivity:
       ```bash
       python3 -c "import socket; s=socket.socket(); s.settimeout(10); s.connect(('<host>', <port>)); print('OK'); s.close()"
       ```
   - Check MCP server can import: `$PLUGIN_ROOT/.venv/bin/python3 -c "import mcp"`

2. If all checks pass:
   - Update `state` to `"active"` in `~/.rune/config.json`
   - Call `reload_pipelines` MCP tool if available
   - Show: "Infrastructure validated. Rune is now active."

3. If checks fail:
   - Keep `state` as `"dormant"`
   - Show what failed
   - Suggest: "Run `/rune:activate` after fixing the issues above."

Skip this step if Vault endpoint or token was not provided.

### 7. Completion

Show summary:
```
Rune Configuration Complete
============================
  Config    : ~/.rune/config.json
  Plugin    : <PLUGIN_ROOT>
  Python    : <PLUGIN_ROOT>/.venv
  MCP       : registered via claude mcp add (user scope)
  State     : <active|dormant>
  Vault TLS : <enabled (system CA) | enabled (custom CA: <path>) | disabled>

Next steps:
  1. Restart Claude Code to load the MCP server
  2. After restart, run /rune:activate to validate and enable
```

If auto-activation succeeded, show: "Rune is active. Organizational memory is now online."
