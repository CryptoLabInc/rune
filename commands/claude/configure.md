---
description: Configure Rune — sets up Python environment, collects credentials, registers MCP servers
allowed-tools: Bash(python3:*), Bash(find:*), Bash(cat ~/.rune/*), Bash(mkdir:*), Bash(chmod:*), Bash(bash:*), Bash(scripts/*), Read, Write
---

# /rune:configure — Full Setup & Configuration

Single entrypoint after `claude plugin install rune`. Handles environment setup, credential collection, config generation, and MCP server registration.

## Steps

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
- **enVector Endpoint** (required, format: `cluster-xxx.envector.io`)
- **enVector API Key** (required, format: `envector_xxx`)
- **Vault Endpoint** (optional, format: `tcp://vault-TEAM.oci.envector.io:50051`)
  - If the user enters a value without a scheme prefix (no `tcp://`, `http://`, or `https://`), auto-prepend `tcp://`.
  - Example: user enters `vault.example.com:50051` → store as `tcp://vault.example.com:50051`
- **Vault Token** (optional, format: `evt_xxx`)

If Vault Endpoint and Token were both provided, ask the TLS question:

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

If Vault fields are skipped, note that the plugin will start in dormant state.

### 4. Write ~/.rune/config.json

```bash
mkdir -p ~/.rune && chmod 700 ~/.rune
```

Write the config file:
```json
{
  "vault": {"endpoint": "<vault_endpoint>", "token": "<vault_token>", "ca_cert": "<ca_cert_path or empty>", "tls_disable": false},
  "envector": {"endpoint": "<envector_endpoint>", "api_key": "<envector_api_key>"},
  "state": "dormant",
  "metadata": {"configVersion": "1.0", "lastUpdated": "<ISO timestamp>", "installedFrom": "<PLUGIN_ROOT>"}
}
```

Then: `chmod 600 ~/.rune/config.json`

### 5. Register MCP Servers

Run: `bash $PLUGIN_ROOT/scripts/configure-claude-mcp.sh`

This registers the envector MCP server via `claude mcp add --scope user` (Claude Code) and JSON merge (Claude Desktop).

### 6. Completion

Show summary:
```
Rune Configuration Complete
============================
  Config    : ~/.rune/config.json
  Plugin    : <PLUGIN_ROOT>
  Python    : <PLUGIN_ROOT>/.venv
  MCP       : registered via claude mcp add (user scope)
  Vault TLS : <enabled (system CA) | enabled (custom CA: <path>) | disabled>

Next steps:
  1. Restart Claude Code to load the MCP server
  2. After restart, run /rune:activate to validate and enable
```

If Vault was not configured, add: "Vault not configured — plugin will start in dormant state. Reconfigure anytime with `/rune:configure`."
