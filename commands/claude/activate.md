---
description: Activate Rune after infrastructure is ready
allowed-tools: Bash(python3:*), Bash(find:*), Bash(cat ~/.rune/*), Bash(curl:*), Bash(bash:*), Bash(scripts/*), Bash(timeout:*), Read, Write, Edit, mcp__envector__reload_pipelines
---

# /rune:activate — Activate Plugin

Validate infrastructure end-to-end and switch to active state.

## Steps

1. Check if `~/.rune/config.json` exists.
   - NO: Respond "Not configured. Run `/rune:configure` first." and stop.

2. Read config and verify required fields exist (`vault.endpoint`, `vault.token`).
   - Missing fields: Show what's missing, suggest `/rune:configure`.
   - Note: enVector credentials are delivered via the Vault bundle at runtime and are not stored in config.

3. Detect plugin root:
   ```bash
   PLUGIN_ROOT=$(find ~/.claude/plugins/cache -name "plugin.json" -path "*/rune/*" -exec dirname {} \; 2>/dev/null | head -1 | xargs dirname 2>/dev/null)
   ```
   - Also check the current working directory and its parents for `.claude-plugin/plugin.json`.

4. Check Python venv at `$PLUGIN_ROOT/.venv`:
   - If missing: auto-run `bash $PLUGIN_ROOT/scripts/install.sh` to set up.
   - If install fails: show error and stop.

5. Run infrastructure validation - test **all subsystems**, not just connectivity:

   **5a. Vault: connectivity + token validity**
   - Check Vault connectivity by parsing the scheme from `vault.endpoint`:
     - If `http://` or `https://`: `curl -sf <vault-endpoint>/health`
     - If `tcp://`: extract host and port, then test TCP connectivity using Python (portable across macOS/Linux, works in both bash and zsh):
      ```bash
      python3 -c "import socket; s=socket.socket(); s.settimeout(10); s.connect(('<host>', <port>)); print('OK'); s.close()"
      ```
      - Do NOT use `bash -c 'echo > /dev/tcp/...'` — macOS `/bin/bash` may not support `/dev/tcp`.
     - Do NOT blindly curl a `tcp://` endpoint — curl does not support the `tcp://` scheme.
   - Validate Vault token:
     ```bash
     $PLUGIN_ROOT/.venv/bin/python3 -c "
     import asyncio, sys, os
     sys.path.insert(0, '$PLUGIN_ROOT/mcp')
     from adapter.vault_client import VaultClient
     async def check():
         c = VaultClient(
             vault_endpoint='<vault_endpoint>',
             vault_token='<vault_token>',
             ca_cert='<ca_cert_or_empty>' or None,
             tls_disable=<tls_disable_bool>,
         )
         try:
             bundle = await c.get_public_key()
             key_id = bundle.get('key_id', '')
             index = bundle.get('index_name', '')
             print(f'OK key_id={key_id} index={index}')
         finally:
             await c.close()
     asyncio.run(check())
     "
     ```
   - If key fetch fails, report **specifically**: "Vault reachable but token rejected - check your token or run `/rune:configure`."

   **5b. Python environment**
   - Check MCP server can import: `$PLUGIN_ROOT/.venv/bin/python3 -c "import mcp"`

6. Display a per-subsystem validation report:
   ```
   Infrastructure Validation
   =========================
   - Vault reachable        (tcp://vault.example.com:50051)
   - Vault token valid      (key_id: abc123)
   - Python environment     (.venv OK)
   ```
   Use "x" mark for failures with the specific error message on the same line.

7. If all checks pass:
   - Update `~/.rune/config.json` setting `state` to `"active"`
   - **Clear dormant reason**: remove `dormant_reason` and `dormant_since` fields from config if present
   - Call `reload_pipelines` as a **native MCP tool** (`mcp__envector__reload_pipelines`) — invoke it directly like any other tool, do NOT use `claude mcp call` via Bash (that subcommand doesn't exist).
   - If `reload_pipelines` is not available as a tool (MCP server not running), note that a Claude Code restart is needed for changes to take effect.
   - If reload_pipelines returns errors, show them and suggest restarting Claude Code as fallback.
   - Respond: "Rune activated. Organizational memory is now online."

8. If any check fails:
   - Keep `state` as `"dormant"`
   - Show the full validation report (passed and failed items)
   - For each failure, include the specific recovery action:
     - Vault unreachable: "Check if Vault server is running and endpoint is correct"
     - Vault token invalid: "Token may be expired or incorrect - run `/rune:configure` to update"
   - Suggest: `/rune:status` for more detailed diagnostics

**Note**: This is the ONLY command that makes network requests to validate infrastructure.
