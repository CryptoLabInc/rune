# Agent Integration Guide

Rune works with all major AI agents via native MCP (Model Context Protocol) support.

## Supported Agents

| Agent | Integration | Setup |
|-------|-------------|-------|
| **Claude Code** | MCP Native (stdio) | ⭐ Easy |
| **Codex CLI** | MCP Native (stdio) | ⭐ One Command |
| **Gemini CLI** | MCP Native (stdio) | ⭐ One Command |
| **OpenAI GPT** | MCP Native (stdio) | ⭐ Easy |

> **Note**: The MCP server uses **stdio transport only**. HTTP/SSE mode is not supported.

---

## LLM Provider Configuration

Rune's Scribe (capture) and Retriever (recall) pipelines use a lightweight LLM for filtering and synthesis. The provider is configured in `~/.rune/config.json`:

```json
{
  "llm": {
    "provider": "anthropic",
    "tier2_provider": "anthropic"
  }
}
```

| Provider | Value | Models |
|----------|-------|--------|
| Anthropic | `"anthropic"` | claude-sonnet-4, claude-haiku-4.5 |
| OpenAI | `"openai"` | gpt-4o-mini |
| Google | `"google"` | gemini-2.0-flash-exp |
| Auto-detect | `"auto"` | Inferred from MCP client identity |

API keys are read from environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GOOGLE_API_KEY` / `GEMINI_API_KEY`) or from the `llm` config section. See [config/README.md](config/README.md) for the full schema.

---

## Claude Code

### Automatic Setup (Recommended)

```bash
cd rune
./scripts/install.sh
```

This automatically registers the `envector` MCP server in Claude Code/Desktop.

### Manual Setup

```bash
claude mcp add --scope user --transport stdio \
  -e ENVECTOR_CONFIG="$HOME/.rune/config.json" \
  -e ENVECTOR_AUTO_KEY_SETUP=false \
  -e PYTHONPATH="/path/to/rune/mcp" \
  envector -- \
  /path/to/rune/.venv/bin/python3 \
  /path/to/rune/mcp/server/server.py --mode stdio
```

Restart Claude Code → MCP tools auto-load.

---

## Codex CLI

### One-Command Installation

```bash
cd rune
./scripts/install-codex.sh
```

This automatically:
1. Creates `.venv` and installs Python dependencies
2. Registers Rune MCP server as `rune-envector` in Codex

### Verify

```bash
codex mcp list
# Should show rune-envector
```

### Configuration

After installation, configure credentials:
```bash
cp config/config.template.json ~/.rune/config.json
# Edit with your vault/envector credentials
```

Set LLM provider to OpenAI (default for Codex):
```bash
export RUNE_LLM_PROVIDER="openai"
export OPENAI_API_KEY="your-key"
```

---

## Gemini CLI

### Option 1: Gemini SDK

```bash
pip install google-generativeai mcp
```

```python
import google.generativeai as genai
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

genai.configure(api_key="your-api-key")

async def main():
    server_params = StdioServerParameters(
        command="/path/to/rune/.venv/bin/python3",
        args=["/path/to/rune/mcp/server/server.py"],
        env={"PYTHONPATH": "/path/to/rune/mcp"}
    )

    async with stdio_client(server_params) as (read, write):
        async with ClientSession(read, write) as session:
            model = genai.GenerativeModel(
                model_name='gemini-2.0-flash-exp',
                tools=[session]  # MCP session
            )

            chat = model.start_chat()
            response = await chat.send_message_async(
                "Create an index called team-decisions"
            )
            print(response.text)

import asyncio
asyncio.run(main())
```

### Option 2: Gemini CLI MCP Config

```bash
npm install -g @google/generative-ai-cli
```

Edit `~/.gemini/mcp_config.json`:
```json
{
  "mcpServers": {
    "envector": {
      "command": "/path/to/rune/.venv/bin/python3",
      "args": ["/path/to/rune/mcp/server/server.py"],
      "env": {
        "PYTHONPATH": "/path/to/rune/mcp"
      }
    }
  }
}
```

```bash
gemini chat
> Create an index called team-decisions
# MCP tools auto-called
```

---

## OpenAI GPT

**Update 2025**: OpenAI has [native MCP support](https://venturebeat.com/programming-development/openai-updates-its-new-responses-api-rapidly-with-mcp-support-gpt-4o-native-image-gen-and-more-enterprise-features) via Responses API.

### Option 1: OpenAI Agents SDK (Recommended)

```bash
pip install openai-agents
```

```python
from openai_agents import Agent
from openai_agents.mcp import MCPServerStdio

mcp_server = MCPServerStdio(
    command="/path/to/rune/.venv/bin/python3",
    args=["/path/to/rune/mcp/server/server.py"],
    env={"PYTHONPATH": "/path/to/rune/mcp"}
)

agent = Agent(
    model="gpt-4o",
    mcp_servers=[mcp_server]
)

response = agent.run("Create an index called team-decisions")
print(response.final_output)
```

### Option 2: Responses API (stdio via MCP SDK)

```bash
pip install openai mcp
```

```python
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

# Connect to Rune MCP server via stdio
server_params = StdioServerParameters(
    command="/path/to/rune/.venv/bin/python3",
    args=["/path/to/rune/mcp/server/server.py"],
    env={"PYTHONPATH": "/path/to/rune/mcp"}
)

# Use with OpenAI function calling by listing MCP tools
async with stdio_client(server_params) as (read, write):
    async with ClientSession(read, write) as session:
        tools = await session.list_tools()
        # Convert MCP tools to OpenAI function format and call as needed
```

---

## Multi-Agent Collaboration

Multiple agents can share the same Rune infrastructure via stdio:

```
# Each agent launches its own MCP server process (stdio)
# All connect to the same enVector Cloud index + Rune-Vault
```

**Architecture**:
```
Claude ──→ MCP Server (stdio) ──┐
                                ├──→ enVector Cloud (encrypted)
Gemini ──→ MCP Server (stdio) ──┤       └──→ Rune-Vault (secret key)
                                │
GPT ─────→ MCP Server (stdio) ──┘
```

Each agent spawns its own MCP server process. Shared state is maintained via enVector Cloud (encrypted vectors) and Rune-Vault (decryption keys).

---

## Troubleshooting

### MCP server won't start
```bash
cd rune
source .venv/bin/activate
python mcp/server/server.py --help
```

### Missing environment variables
```bash
cat ~/.rune/config.json

# Or set environment variables directly:
export ENVECTOR_ENDPOINT="runestone-xxx.clusters.envector.io"
export ENVECTOR_API_KEY="your-api-key"
export RUNEVAULT_ENDPOINT="vault-yourteam.oci.envector.io:50051"
export RUNEVAULT_TOKEN="your-token"
# Optional: explicit gRPC target override (auto-derived from RUNEVAULT_ENDPOINT if omitted)
# export RUNEVAULT_GRPC_TARGET="vault-host:50051"
```

### Verify MCP tools are available

In Claude Code, after plugin installation:
```
/rune:status
```

---

## References

- [MCP Protocol](https://modelcontextprotocol.io)
- [Google Cloud MCP Announcement](https://cloud.google.com/blog/products/ai-machine-learning/announcing-official-mcp-support-for-google-services)
- [OpenAI Responses API MCP](https://venturebeat.com/programming-development/openai-updates-its-new-responses-api-rapidly-with-mcp-support-gpt-4o-native-image-gen-and-more-enterprise-features)
- [Gemini CLI MCP Docs](https://geminicli.com/docs/tools/mcp-server/)
- [FastMCP](https://github.com/jlowin/fastmcp)
