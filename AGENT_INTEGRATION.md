# Agent Integration Guide

Rune works with all major AI agents via native MCP (Model Context Protocol) support.

## Supported Agents

| Agent | Integration | Setup |
|-------|-------------|-------|
| **Claude** | MCP Native | ⭐ Easy |
| **Gemini** | MCP Native | ⭐ Easy |
| **OpenAI GPT** | MCP Native | ⭐ Easy |

---

## Claude

### Automatic Setup (Recommended)

```bash
cd rune
./install.sh
# Select: 1) Claude (MCP native support)
```

This automatically adds `envector-mcp-server` to `~/.claude/mcp_settings.json`.

### Manual Setup

Edit `~/.claude/mcp_settings.json`:
```json
{
  "mcpServers": {
    "envector-mcp-server": {
      "command": "/path/to/rune/mcp/envector-mcp-server/.venv/bin/python",
      "args": ["/path/to/rune/mcp/envector-mcp-server/srcs/server.py"],
      "env": {
        "PYTHONPATH": "/path/to/rune/mcp/envector-mcp-server"
      }
    }
  }
}
```

Restart Claude Code → MCP tools auto-load.

---

## Gemini

**Update 2026**: Gemini has [native MCP support](https://cloud.google.com/blog/products/ai-machine-learning/announcing-official-mcp-support-for-google-services).

### Option 1: Gemini SDK (Recommended)

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
        command="python",
        args=["/path/to/rune/mcp/envector-mcp-server/srcs/server.py"],
        env={"PYTHONPATH": "/path/to/rune/mcp/envector-mcp-server"}
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

### Option 2: Gemini CLI (Easiest)

```bash
npm install -g @google/generative-ai-cli
```

Edit `~/.gemini/mcp_config.json`:
```json
{
  "mcpServers": {
    "envector-mcp-server": {
      "command": "python",
      "args": ["/path/to/rune/mcp/envector-mcp-server/srcs/server.py"],
      "env": {
        "PYTHONPATH": "/path/to/rune/mcp/envector-mcp-server"
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

### Option 1: Responses API (Recommended)

```bash
pip install openai
```

```python
from openai import OpenAI

client = OpenAI(api_key="your-api-key")

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "user", "content": "Create an index called team-decisions"}
    ],
    tools=[
        {
            "type": "mcp_server",
            "mcp_server": {
                "url": "http://127.0.0.1:8000",
                "transport": "http_sse"
            }
        }
    ]
)

print(response.choices[0].message.content)
```

### Option 2: OpenAI Agents SDK

```bash
pip install openai-agents
```

```python
from openai_agents import Agent
from openai_agents.mcp import MCPServerStdio

mcp_server = MCPServerStdio(
    command="python",
    args=["/path/to/rune/mcp/envector-mcp-server/srcs/server.py"],
    env={"PYTHONPATH": "/path/to/rune/mcp/envector-mcp-server"}
)

agent = Agent(
    model="gpt-4o",
    mcp_servers=[mcp_server]
)

response = agent.run("Create an index called team-decisions")
print(response.final_output)
```

### Option 3: ChatGPT Developer Mode

1. ChatGPT Plus/Pro account
2. Enable Developer Mode
3. Add MCP server:
   ```json
   {
     "mcp_servers": {
       "rune": {
         "url": "http://127.0.0.1:8000",
         "transport": "http_sse"
       }
     }
   }
   ```

---

## Multi-Agent Collaboration

Multiple agents can share the same Rune instance:

```bash
# Start MCP server once
./start-mcp-server.sh

# Claude, Gemini, GPT all connect to http://127.0.0.1:8000
# → Shared enVector Cloud index
# → Shared Rune-Vault secret key
```

**Architecture**:
```
Claude ──┐
         ├──→ Rune MCP Server ──→ enVector Cloud (encrypted)
Gemini ──┤                    └──→ Rune-Vault (secret key)
GPT ─────┘
```

---

## Security

### Local Deployment (Recommended)
```bash
--host 127.0.0.1  # Localhost only
```

### Production Deployment
- **Authentication**: API keys or OAuth
- **HTTPS**: Reverse proxy (nginx, Caddy)
- **Firewall**: Team network only

---

## Troubleshooting

### MCP server won't start
```bash
cd rune/mcp/envector-mcp-server
source .venv/bin/activate
python srcs/server.py --mode http --verbose
```

### Missing environment variables
```bash
cat rune/mcp/envector-mcp-server/.env

# Required:
ENVECTOR_ADDRESS="cluster.envector.io:443"
ENVECTOR_API_KEY="your-api-key"
RUNEVAULT_ENDPOINT="https://vault-url"
RUNEVAULT_TOKEN="your-token"
```

---

## References

- [MCP Protocol](https://modelcontextprotocol.io)
- [Google Cloud MCP Announcement](https://cloud.google.com/blog/products/ai-machine-learning/announcing-official-mcp-support-for-google-services)
- [OpenAI Responses API MCP](https://venturebeat.com/programming-development/openai-updates-its-new-responses-api-rapidly-with-mcp-support-gpt-4o-native-image-gen-and-more-enterprise-features)
- [Gemini CLI MCP Docs](https://geminicli.com/docs/tools/mcp-server/)
- [FastMCP](https://github.com/jlowin/fastmcp)
