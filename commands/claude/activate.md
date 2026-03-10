---
description: Activate Rune after infrastructure is ready
allowed-tools: Bash(python3:*), Bash(find:*), Bash(cat ~/.rune/*), Bash(curl:*), Bash(bash:*), Bash(scripts/*), Bash(timeout:*), Read, Write, Edit, mcp__envector__reload_pipelines
---

# /rune:activate — Activate Plugin

Validate infrastructure and switch to active state.

## Steps

1. Check if `~/.rune/config.json` exists.
   - NO: Respond "Not configured. Run `/rune:configure` first." and stop.

2. Read config and verify required fields exist (`vault.endpoint`, `vault.token`, `envector.endpoint`, `envector.api_key`).
   - Missing fields: Show what's missing, suggest `/rune:configure`.

3. Detect plugin root:
   ```bash
   PLUGIN_ROOT=$(find ~/.claude/plugins/cache -name "plugin.json" -path "*/rune/*" -exec dirname {} \; 2>/dev/null | head -1 | xargs dirname 2>/dev/null)
   ```
   - Also check the current working directory and its parents for `.claude-plugin/plugin.json`.

4. Check Python venv at `$PLUGIN_ROOT/.venv`:
   - If missing: auto-run `bash $PLUGIN_ROOT/scripts/install.sh` to set up.
   - If install fails: show error and stop.

5. Run infrastructure validation:
   - Check Vault connectivity by parsing the scheme from `vault.endpoint`:
     - If `http://` or `https://`: `curl -sf <vault-endpoint>/health`
     - If `tcp://`: extract host and port, then test TCP connectivity using Python (portable across macOS/Linux, works in both bash and zsh):
      ```bash
      python3 -c "import socket; s=socket.socket(); s.settimeout(10); s.connect(('<host>', <port>)); print('OK'); s.close()"
      ```
      - Do NOT use `bash -c 'echo > /dev/tcp/...'` — macOS `/bin/bash` may not support `/dev/tcp`.
     - Do NOT blindly curl a `tcp://` endpoint — curl does not support the `tcp://` scheme.
   - Check MCP server can import: `$PLUGIN_ROOT/.venv/bin/python3 -c "import mcp"`

6. If all checks pass:
   - Update `~/.rune/config.json` setting `state` to `"active"`
   - Call `reload_pipelines` as a **native MCP tool** (`mcp__envector__reload_pipelines`) — invoke it directly like any other tool, do NOT use `claude mcp call` via Bash (that subcommand doesn't exist).
   - If `reload_pipelines` is not available as a tool (MCP server not running), note that a Claude Code restart is needed for changes to take effect.
   - If reload_pipelines returns errors, show them and suggest restarting Claude Code as fallback.
   - Respond: "Rune activated. Organizational memory is now online."

7. If checks fail:
   - Keep `state` as `"dormant"`
   - Show detailed error report for each failed check
   - Suggest: `/rune:status` for more info

**Note**: This is the ONLY command that makes network requests to validate infrastructure.
