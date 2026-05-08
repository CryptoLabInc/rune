# Agent Integration Guide

Rune works with all major AI agents via native MCP (Model Context Protocol)
support. In v0.4 the MCP server is a single Go binary
(`bin/rune-mcp`) that the host CLI auto-spawns over stdio — no Python
runtime, no `pip install`, no manual `mcp add` for the supported CLIs.

## Integration Principles

### Cross-agent common (single source of truth)
- The Go binary at `cmd/rune-mcp/` is the only MCP server entry point.
  Plugin / extension manifests point each CLI at the same binary.
- Runtime preparation happens at install time (the binary is already
  built and shipped with the plugin tarball — see Task #30 for the
  release pipeline). Nothing needs to be (re)bootstrapped at session
  start.

### Agent-specific adapters (thin layer only)
- Codex-only tasks: `codex mcp add/remove/list` registration flows
- Claude / Gemini / OpenAI: each client's native MCP registration flow

Keep these layers separate to avoid cross-agent drift.

## Supported Agents

| Agent | Integration | Setup |
|-------|-------------|-------|
| **Claude Code** | MCP Native (stdio) | ⭐ Plugin install |
| **Codex CLI** | MCP Native (stdio) | ⭐ Skill install |
| **Gemini CLI** | MCP Native (stdio) | ⭐ Extension install |
| **OpenAI GPT** | MCP Native (stdio) | ⭐ Programmatic |

> The MCP server uses **stdio transport only**. HTTP/SSE mode is not supported.

---

## Claude Code

### Plugin install (recommended)

```bash
# From terminal (local clone)
$ claude plugin marketplace add ./
$ claude plugin install rune

# From inside a Claude Code session (remote)
> /plugin marketplace add https://github.com/CryptoLabInc/rune
> /plugin install rune
```

The plugin manifest (`.claude-plugin/plugin.json`) declares the binary
path; Claude Code spawns `${CLAUDE_PLUGIN_ROOT}/bin/rune-mcp` via stdio
on session start. enVector Cloud credentials are delivered automatically
via the Vault bundle — you never set `ENVECTOR_*` env vars directly.

### Configure credentials

```
> /rune:configure
```

Walks you through Vault endpoint + token + TLS choice, writes
`~/.rune/config.json`, and triggers the boot loop.

### Verify

```
> /rune:status
```

Renders per-subsystem health (Vault / EncKey / AgentDEK / Embedder /
enVector) via the `diagnostics` MCP tool.

### Dev mode (running from a local clone)

```bash
$ claude --plugin-dir /path/to/rune
```

Loads the plugin from the working tree instead of the installed cache.
Useful for iterating on `commands/claude/*.md` or the Go binary without
re-installing the plugin.

---

## Codex CLI

### Skill install

```
> $skill-installer install https://github.com/CryptoLabInc/rune.git
```

### Configure

```
> $rune configure
```

### Verify

```bash
codex mcp list
# Should show rune
```

If `rune` is not listed after install, re-register manually:
```bash
codex mcp add rune --command /path/to/bin/rune-mcp --transport stdio
```

---

## Gemini CLI

### Extension install

```bash
$ gemini extensions install https://github.com/CryptoLabInc/rune.git
```

### Configure

```
> /rune:configure
```

### Manual `mcp_config.json` (advanced)

```json
{
  "mcpServers": {
    "rune": {
      "command": "/path/to/bin/rune-mcp",
      "transport": "stdio"
    }
  }
}
```

> The legacy `gemini-extension.json` still references
> `scripts/bootstrap-mcp.sh`; rewriting it for the Go binary is tracked
> as a follow-up under the Gemini track.

---

## OpenAI GPT

OpenAI's Responses API has [native MCP support](https://venturebeat.com/programming-development/openai-updates-its-new-responses-api-rapidly-with-mcp-support-gpt-4o-native-image-gen-and-more-enterprise-features),
so you point it at the same Go binary via stdio:

```python
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

server_params = StdioServerParameters(
    command="/path/to/bin/rune-mcp",
    args=[],  # binary takes no args; reads ~/.rune/config.json
)

async with stdio_client(server_params) as (read, write):
    async with ClientSession(read, write) as session:
        tools = await session.list_tools()
        # forward tools to OpenAI function calling
```

The OpenAI Agents SDK pattern is identical — pass the same `command`
into `MCPServerStdio`.

---

## Multi-Agent Collaboration

Each agent spawns its own MCP server process; shared state is
maintained via enVector Cloud (encrypted vectors) and Rune-Vault
(decryption keys).

```
Claude ──→ rune-mcp (stdio) ──┐
                              ├──→ enVector Cloud (encrypted)
Gemini ──→ rune-mcp (stdio) ──┤       └──→ Rune-Vault (secret key)
                              │
GPT    ──→ rune-mcp (stdio) ──┘
```

All three connect to the same team index + Vault, so a capture from
one agent is recallable by another.

---

## Troubleshooting

### MCP server won't start

```bash
# Run the binary directly to see startup errors
/path/to/bin/rune-mcp
# (it will block on stdin — Ctrl-D to exit; you're looking for slog
# error output before the block)
```

If you set `RUNE_MCP_LOG_FILE=` in the spawning shell, the server tees
its slog to `~/.rune/logs/rune-mcp.log` so you can `tail -f` while the
host CLI runs it.

### Codex registration repair

```bash
codex mcp list
codex mcp add rune --command /path/to/bin/rune-mcp --transport stdio
```

### Missing or wrong credentials

```bash
cat ~/.rune/config.json
# vault.endpoint, vault.token, ca_cert, tls_disable, state
```

enVector credentials are delivered automatically via the Vault bundle
at boot — they live in memory only and are not stored locally. You do
NOT need to set `ENVECTOR_ENDPOINT` or `ENVECTOR_API_KEY`.

### Verify MCP tools are available

In Claude Code after plugin install:
```
/plugin            # confirm 'rune' loaded, Errors 0
/rune:status      # confirm Active + diagnostics snapshot
```

---

## References

- [MCP Protocol](https://modelcontextprotocol.io)
- [Google Cloud MCP Announcement](https://cloud.google.com/blog/products/ai-machine-learning/announcing-official-mcp-support-for-google-services)
- [OpenAI Responses API MCP](https://venturebeat.com/programming-development/openai-updates-its-new-responses-api-rapidly-with-mcp-support-gpt-4o-native-image-gen-and-more-enterprise-features)
- [Gemini CLI MCP Docs](https://geminicli.com/docs/tools/mcp-server/)
