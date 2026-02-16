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
  - Example: user enters `0.tcp.jp.ngrok.io:17404` → store as `tcp://0.tcp.jp.ngrok.io:17404`
- **Vault Token** (optional, format: `evt_xxx`)

If Vault fields are skipped, note that the plugin will start in dormant state.

### 4. Write ~/.rune/config.json

```bash
mkdir -p ~/.rune && chmod 700 ~/.rune
```

Write the config file:
```json
{
  "vault": {"endpoint": "<vault_endpoint>", "token": "<vault_token>"},
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

Next steps:
  1. Restart Claude Code to load the MCP server
  2. After restart, run /rune:activate to validate and enable
```

If Vault was not configured, add: "Vault not configured — plugin will start in dormant state. Reconfigure anytime with `/rune:configure`."
