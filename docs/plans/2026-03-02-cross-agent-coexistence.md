# Cross-Agent Co-existence Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restructure Rune to support Claude Code, Gemini CLI, and Codex CLI from a single repo via co-existing manifests, removing the OpenClaw TS wrapper.

**Architecture:** Each agent platform discovers its own manifest at the repo root and ignores others. Shared core (MCP server, Python agents, config) remains unchanged. Agent-specific files (commands, specs, hooks, context) live in dedicated locations per platform.

**Tech Stack:** Python (MCP server), Markdown/TOML (commands), JSON (manifests), Bash (scripts)

**Design Doc:** `docs/plans/2026-03-02-cross-agent-coexistence-design.md`

---

### Task 1: Remove OpenClaw TypeScript Layer

Remove all OpenClaw-specific files. The functionality they provided is already covered by
Claude Code's `.claude-plugin/` system and native MCP tools.

**Files:**
- Delete: `src/commands.ts`
- Delete: `src/config.ts`
- Delete: `src/hooks.ts`
- Delete: `src/mcp-client.ts`
- Delete: `src/mcp-service.ts`
- Delete: `src/tools.ts`
- Delete: `index.ts`
- Delete: `openclaw.plugin.json`

**Step 1: Delete the files**

```bash
rm -rf src/
rm index.ts
rm openclaw.plugin.json
```

**Step 2: Clean up package.json**

Remove openclaw-specific fields. Keep the file for version metadata only:

```json
{
  "name": "@cryptolab/rune",
  "version": "0.2.0",
  "description": "FHE-encrypted organizational memory for teams",
  "private": true,
  "license": "Apache-2.0",
  "author": "Sunchul Jung <zotanika@cryptolab.co.kr>",
  "keywords": ["mcp", "fhe", "memory", "gemini", "claude"]
}
```

Remove: `type`, `main`, `files`, `dependencies` (@sinclair/typebox), `peerDependencies` (openclaw),
`openclaw` section. Add `"private": true` since this isn't published to npm.

**Step 3: Verify Claude commands still exist**

Check that `.claude-plugin/commands/` has all 7 command files:
activate.md, configure.md, deactivate.md, memorize.md, recall.md, reset.md, status.md

**Step 4: Commit**

```bash
git add -A
git commit -m "refactor: remove OpenClaw TypeScript wrapper

The TS layer (src/, index.ts, openclaw.plugin.json) is replaced by
native per-agent manifests. Claude Code commands already exist in
.claude-plugin/commands/. MCP tools are the universal interface."
```

---

### Task 2: Move Agent Specs to Per-Agent Subdirectories

Agent spec `.md` files need per-agent variants because Claude Code and Gemini CLI
have different spec formats and conventions.

**Files:**
- Move: `agents/scribe.md` → `agents/claude/scribe.md`
- Move: `agents/retriever.md` → `agents/claude/retriever.md`
- Modify: `.claude-plugin/plugin.json` — update agents paths

**Step 1: Create directory and move files**

```bash
mkdir -p agents/claude
git mv agents/scribe.md agents/claude/scribe.md
git mv agents/retriever.md agents/claude/retriever.md
```

**Step 2: Update .claude-plugin/plugin.json**

Change agents array paths:

```json
"agents": [
    "./agents/claude/scribe.md",
    "./agents/claude/retriever.md"
]
```

Note: paths in `.claude-plugin/plugin.json` are relative to the repo root
(CLAUDE_PLUGIN_ROOT), not to the `.claude-plugin/` directory.

**Step 3: Commit**

```bash
git add -A
git commit -m "refactor: move agent specs to agents/claude/ for per-agent separation"
```

---

### Task 3: Fix Version Strings

Two files still have `0.1.1`. Update to `0.2.0`.

**Files:**
- Modify: `scripts/install.sh:7` — `VERSION="0.1.1"` → `VERSION="0.2.0"`
- Modify: `agents/__init__.py:19` — `__version__ = "0.1.1"` → `__version__ = "0.2.0"`

**Step 1: Update install.sh**

```bash
# Line 7: VERSION="0.1.1" → VERSION="0.2.0"
```

**Step 2: Update agents/__init__.py**

```python
# Line 19: __version__ = "0.1.1" → __version__ = "0.2.0"
```

**Step 3: Verify no 0.1.1 remains**

```bash
grep -r "0\.1\.1" --include="*.py" --include="*.sh" --include="*.json" --include="*.ts" .
```

Expected: no matches.

**Step 4: Commit**

```bash
git add scripts/install.sh agents/__init__.py
git commit -m "chore: bump remaining version strings to 0.2.0"
```

---

### Task 4: Clean Up openclaw References in Python

The MCP server has an `openclaw` string in auto-provider detection. Keep it for
backward compatibility but rename the client identity string.

**Files:**
- Modify: `mcp/server/server.py:255` — keep `"openclaw"` in detection list (backward compat)

**Step 1: Review the auto-provider detection line**

```python
# mcp/server/server.py line ~255
if any(token in client_name for token in ("gemini", "google", "antigravity", "openclaw")):
```

This detects the MCP client identity to auto-select LLM provider. `"openclaw"` should
stay as a compatibility token since older installations may still report that name.
No change needed here.

**Step 2: Search for other openclaw references in Python**

```bash
grep -rn "openclaw" --include="*.py" .
```

If any non-compatibility references exist, update or remove them.

**Step 3: Commit (only if changes were made)**

```bash
git add -A
git commit -m "chore: clean up openclaw references in Python layer"
```

---

### Task 5: Create Gemini CLI Extension Manifest

Create `gemini-extension.json` at the repo root so `gemini extensions install` works.

**Files:**
- Create: `gemini-extension.json`

**Step 1: Create the manifest**

```json
{
  "name": "rune",
  "version": "0.2.0",
  "description": "FHE-encrypted organizational memory for teams. Capture and retrieve institutional knowledge with zero-knowledge privacy.",
  "contextFileName": "GEMINI.md",
  "mcpServers": {
    "envector": {
      "command": "bash",
      "args": ["${extensionPath}${/}scripts${/}bootstrap-mcp.sh"],
      "cwd": "${extensionPath}"
    }
  },
  "settings": [
    {
      "name": "envector_endpoint",
      "description": "enVector cluster endpoint (e.g. cluster-xxx.envector.io)",
      "envVar": "ENVECTOR_ENDPOINT"
    },
    {
      "name": "envector_api_key",
      "description": "enVector API key",
      "envVar": "ENVECTOR_API_KEY",
      "sensitive": true
    },
    {
      "name": "vault_endpoint",
      "description": "Rune-Vault gRPC endpoint (e.g. tcp://vault-TEAM.oci.envector.io:50051)",
      "envVar": "RUNEVAULT_ENDPOINT"
    },
    {
      "name": "vault_token",
      "description": "Vault authentication token",
      "envVar": "RUNEVAULT_TOKEN",
      "sensitive": true
    }
  ]
}
```

**Step 2: Verify bootstrap-mcp.sh works standalone**

```bash
bash scripts/bootstrap-mcp.sh --help 2>&1 || true
```

The script should be able to find the venv and launch the MCP server.
If it relies on CLAUDE_PLUGIN_ROOT, it needs to also support extensionPath or cwd fallback.

**Step 3: Commit**

```bash
git add gemini-extension.json
git commit -m "feat: add Gemini CLI extension manifest"
```

---

### Task 6: Create GEMINI.md Context File

Gemini CLI loads this as persistent context for every session. Adapt from CLAUDE.md
but with Gemini-appropriate instructions (no Claude-specific references).

**Files:**
- Create: `GEMINI.md`
- Reference: `CLAUDE.md` (for content to adapt)

**Step 1: Create GEMINI.md**

Adapt the Rune usage guidelines from CLAUDE.md. Key differences:
- Reference Gemini CLI conventions instead of Claude Code
- Use MCP tool names (`capture`, `recall`) since Gemini calls them directly
- Remove Claude-specific state gate instructions (Gemini uses settings/envVars)
- Keep the routing rules and distinction sections

**Step 2: Commit**

```bash
git add GEMINI.md
git commit -m "docs: add GEMINI.md context file for Gemini CLI"
```

---

### Task 7: Create Gemini Commands (TOML)

Port the essential slash commands to Gemini's TOML command format.
Gemini commands use `commands/groupName/command.toml` structure.

**Files:**
- Create: `commands/rune/configure.toml`
- Create: `commands/rune/activate.toml`
- Create: `commands/rune/recall.toml`
- Create: `commands/rune/memorize.toml`
- Create: `commands/rune/status.toml`

**Step 1: Create commands directory**

```bash
mkdir -p commands/rune
```

Gemini convention: `commands/rune/recall.toml` → invoked as `/rune:recall`

**Step 2: Create each command**

Gemini TOML format uses `prompt = """..."""` with `{{args}}` for arguments
and `!{command}` for shell execution. Commands tell the LLM what to do.

Example for `recall.toml`:
```toml
prompt = """Search encrypted organizational memory for relevant context.

The argument `{{args}}` contains the search query.

Use the envector MCP `recall` tool with the query to search organizational memory.
Return relevant results with source attribution, excerpts, and confidence level."""
```

Create similar TOML files for configure, activate, memorize, status.
Reference `.claude-plugin/commands/*.md` for the prompt content to port.

**Step 3: Commit**

```bash
git add commands/
git commit -m "feat: add Gemini CLI commands (TOML format)"
```

---

### Task 8: Create Gemini Agent Specs

Create Gemini-format agent specs adapted from the Claude versions.

**Files:**
- Create: `agents/gemini/scribe.md`
- Create: `agents/gemini/retriever.md`

**Step 1: Create directory**

```bash
mkdir -p agents/gemini
```

**Step 2: Adapt specs from Claude versions**

Read `agents/claude/scribe.md` and `agents/claude/retriever.md`.
Adapt for Gemini conventions:
- Gemini uses `agents/` directory referenced by the extension
- Remove Claude-specific instructions (subagent spawning, skill references)
- Keep MCP tool call instructions (these are universal)

**Step 3: Commit**

```bash
git add agents/gemini/
git commit -m "feat: add Gemini agent specs"
```

---

### Task 9: Create Gemini Hooks

Set up `hooks/hooks.json` for Gemini lifecycle events.

**Files:**
- Create: `hooks/hooks.json`

**Step 1: Research hooks.json format**

Check Gemini CLI docs for the exact hooks.json schema and supported lifecycle events.
Initial skeleton (events TBD based on Gemini support):

```json
{
  "hooks": []
}
```

If Gemini supports pre/post tool-call hooks, add auto-capture triggers.
This may be minimal initially and expanded as Gemini's hook system matures.

**Step 2: Commit**

```bash
git add hooks/
git commit -m "feat: add Gemini hooks skeleton"
```

---

### Task 10: Update bootstrap-mcp.sh for Cross-Agent Compatibility

Ensure the MCP server launcher works when invoked from any agent's context.

**Files:**
- Modify: `scripts/bootstrap-mcp.sh`

**Step 1: Review current script**

The script currently may rely on `CLAUDE_PLUGIN_ROOT`. It needs to also work when:
- Gemini sets `cwd` to `${extensionPath}`
- Codex runs it from the install directory

Strategy: resolve the script's own directory as plugin root fallback.

```bash
# Resolve plugin root from script location if not set by agent
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
```

**Step 2: Test the script resolves paths correctly**

```bash
cd /path/to/rune && bash scripts/bootstrap-mcp.sh
```

**Step 3: Commit**

```bash
git add scripts/bootstrap-mcp.sh
git commit -m "fix: make bootstrap-mcp.sh agent-agnostic"
```

---

### Task 11: Update Documentation

Update README.md to reflect the new cross-agent structure.

**Files:**
- Modify: `README.md`
- Modify: `CONTRIBUTING.md` — update directory structure, remove openclaw references

**Step 1: Update README.md**

Add installation instructions for each agent:
- Claude Code: `claude plugin install CryptoLabInc/rune`
- Gemini CLI: `gemini extensions install https://github.com/CryptoLabInc/rune`
- Codex CLI: `./scripts/install-codex.sh`

**Step 2: Update CONTRIBUTING.md**

Remove `openclaw.plugin.json` references and `src/` directory from the structure docs.
Update the directory tree to match the new layout.

**Step 3: Commit**

```bash
git add README.md CONTRIBUTING.md
git commit -m "docs: update README and CONTRIBUTING for cross-agent structure"
```

---

### Task 12: Sync Changes to Plugin Cache

Copy the modified files to the Claude plugin cache so changes take effect
without reinstalling.

**Step 1: Copy changed files**

```bash
CACHE=~/.claude/plugins/cache/cryptolab/rune/0.2.0
cp -r agents/ "$CACHE/agents/"
cp -r mcp/ "$CACHE/mcp/"
cp -r scripts/ "$CACHE/scripts/"
# Don't copy .claude-plugin/ — Claude reads from cache's own structure
```

**Step 2: Verify**

Restart Claude Code and test `/rune:status`.
