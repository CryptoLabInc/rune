# Cross-Agent Co-existence Design

Date: 2026-03-02
Status: Approved
Branch: feat/cross-agents

## Problem

Rune needs to support Claude Code, Gemini CLI, and Codex CLI from a single repository.
The current approach uses an OpenClaw TypeScript wrapper that was originally Claude Code-specific
and has become an awkward agent-agnostic layer that doesn't fit any platform well.

Each agent platform has its own manifest format, command syntax, hook mechanism, and
agent spec format. A single repo must accommodate all of them without a deploy/build step
that would break one-command install (`claude plugin install`, `gemini extensions install`).

## Decision

**Co-existence**: all agent manifests live in the same repo root. Each agent discovers its
own manifest and ignores others. No overlay scripts, no separate repos.

## Architecture

```
Single Repo (CryptoLabInc/rune)
  ├── Agent Manifests (per-platform, root-level)
  ├── Agent-Specific Files (commands/, agents/specs, hooks/, skills/)
  └── Shared Core (mcp/, agents/python, config/, scripts/)
```

### What's Shared (all agents)

- **MCP server** (`mcp/server/server.py`): stdio transport, tools: capture, recall, vault_status, reload_pipelines
- **Python agents** (`agents/scribe/`, `agents/retriever/`, `agents/common/`): detection, extraction, search, synthesis
- **Config**: `~/.rune/config.json`, multi-provider LLM abstraction
- **Encryption pipeline**: enVector Cloud + Rune-Vault
- **Scripts**: `install.sh` (venv+deps), `bootstrap-mcp.sh` (MCP launcher)

### What Diverges (per-agent)

| Concern | Claude Code | Gemini CLI | Codex CLI |
|---------|------------|-----------|----------|
| Manifest | `.claude-plugin/plugin.json` | `gemini-extension.json` | TBD |
| Context file | `CLAUDE.md` | `GEMINI.md` | TBD |
| Commands | `commands/claude/*.md` | `commands/gemini/*.toml` | N/A (MCP only) |
| Agent specs | `agents/claude/*.md` | `agents/gemini/*.md` | N/A |
| Hooks | Claude Code hooks (settings.json) | `hooks/hooks.json` + scripts | TBD |
| Skills | `.claude-plugin` skills | `skills/*/SKILL.md` | N/A |
| MCP registration | manifest mcpServers | manifest mcpServers | `codex mcp add` |
| Install | `claude plugin install` | `gemini extensions install` | `install-codex.sh` |

### Directory Structure

```
rune/
├── .claude-plugin/
│   ├── plugin.json                 # commands → ./commands/claude/
│   └── marketplace.json
├── gemini-extension.json           # mcpServers, settings, contextFileName
│
├── CLAUDE.md
├── GEMINI.md
│
├── commands/
│   ├── claude/                     # .md format
│   │   ├── configure.md
│   │   ├── activate.md
│   │   ├── recall.md
│   │   ├── memorize.md
│   │   └── status.md
│   └── gemini/                     # .toml format
│       ├── configure.toml
│       ├── activate.toml
│       ├── recall.toml
│       ├── memorize.toml
│       └── status.toml
│
├── agents/
│   ├── claude/                     # Claude agent specs
│   │   ├── scribe.md
│   │   └── retriever.md
│   ├── gemini/                     # Gemini agent specs
│   │   ├── scribe.md
│   │   └── retriever.md
│   ├── scribe/                     # Python (shared)
│   ├── retriever/                  # Python (shared)
│   ├── common/                     # Python (shared)
│   └── tests/
│
├── hooks/
│   └── hooks.json                  # Gemini lifecycle hooks
│
├── skills/                         # Gemini skills
│
├── mcp/                            # Shared MCP server
│   ├── server/
│   ├── adapter/
│   └── tests/
│
├── scripts/
│   ├── install.sh                  # Shared: venv + deps
│   ├── bootstrap-mcp.sh            # Shared: MCP server launcher
│   ├── configure-claude-mcp.sh     # Claude-specific MCP registration
│   ├── register-plugin.sh          # Claude marketplace registration
│   └── install-codex.sh            # Codex MCP registration
│
├── config/
│   └── config.template.json
└── README.md
```

## Removals

The OpenClaw TypeScript wrapper is removed entirely:

- `src/commands.ts` — replaced by `commands/claude/*.md` (already exist)
- `src/tools.ts` — replaced by MCP tools (already native)
- `src/hooks.ts` — replaced by Claude Code hooks (settings.json) / Gemini hooks.json
- `src/mcp-client.ts` — no longer needed (MCP server runs via manifest)
- `src/mcp-service.ts` — no longer needed
- `src/config.ts` — Python config handles this
- `index.ts` — OpenClaw entry point, removed
- `openclaw.plugin.json` — replaced by per-agent manifests
- `package.json` — openclaw peer dependency removed; keep only if needed for version metadata
- `tsconfig.json` — TS build no longer needed

## Version Synchronization

All version references updated to `0.2.0`:

- `.claude-plugin/plugin.json`
- `gemini-extension.json`
- `agents/__init__.py`
- `scripts/install.sh`
- `scripts/register-plugin.sh`

## Manifest Details

### Claude Code (`.claude-plugin/plugin.json`)

```json
{
  "name": "rune",
  "version": "0.2.0",
  "commands": "./commands/claude/",
  "agents": ["./agents/claude/scribe.md", "./agents/claude/retriever.md"],
  "mcpServers": {
    "envector": {
      "command": "${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap-mcp.sh",
      "env": {
        "ENVECTOR_CONFIG": "${HOME}/.rune/config.json",
        "ENVECTOR_AUTO_KEY_SETUP": "false"
      }
    }
  }
}
```

### Gemini CLI (`gemini-extension.json`)

```json
{
  "name": "rune",
  "version": "0.2.0",
  "description": "FHE-encrypted organizational memory for teams",
  "contextFileName": "GEMINI.md",
  "mcpServers": {
    "envector": {
      "command": "bash",
      "args": ["${extensionPath}${/}scripts${/}bootstrap-mcp.sh"],
      "cwd": "${extensionPath}"
    }
  },
  "settings": [
    { "name": "envector_endpoint", "description": "enVector cluster endpoint", "envVar": "ENVECTOR_ENDPOINT" },
    { "name": "envector_api_key", "description": "enVector API key", "envVar": "ENVECTOR_API_KEY", "sensitive": true },
    { "name": "vault_endpoint", "description": "Rune-Vault endpoint", "envVar": "RUNEVAULT_ENDPOINT" },
    { "name": "vault_token", "description": "Vault auth token", "envVar": "RUNEVAULT_TOKEN", "sensitive": true }
  ]
}
```

## Migration Path

1. Remove OpenClaw TS layer (src/, index.ts, openclaw.plugin.json)
2. Move existing Claude commands from TS to commands/claude/ (already .md files)
3. Update .claude-plugin/plugin.json paths
4. Create gemini-extension.json manifest
5. Create GEMINI.md context file
6. Port commands to commands/gemini/*.toml format
7. Create agents/gemini/ specs (adapt from Claude versions)
8. Create hooks/hooks.json for Gemini lifecycle
9. Sync all versions to 0.2.0
10. Clean up openclaw references in server.py (keep as compatibility token in auto-detect)
11. Update README.md and docs
