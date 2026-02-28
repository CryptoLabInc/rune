# MCP-First Documentation Rewrite Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove the redundant `remember` MCP tool and rewrite all documentation with MCP tools as the primary interface.

**Architecture:** Incremental rewrite — keep file structure, change content. Delete `remember` from server.py and src/tools.ts, then rewrite README.md MCP-first, update all downstream references.

**Tech Stack:** Python (FastMCP), TypeScript (OpenClaw plugin SDK), Markdown

---

### Task 1: Delete `remember` MCP tool from server.py

**Files:**
- Modify: `mcp/server/server.py:273-415` (delete `tool_remember` function and its decorator)

**Step 1: Delete the `tool_remember` function**

Remove the entire block from `@self.mcp.tool(name="remember", ...)` through the end of the `except Exception` block (lines 273-415). The `# ---------- MCP Tools: Vault Health Check ---------- #` comment on line 416 marks the start of the next tool.

**Step 2: Run tests**

Run: `source .venv/bin/activate && python -m pytest agents/tests/ -v --tb=short`
Expected: 216 passed (no tests directly exercise `tool_remember` — it's an MCP-level endpoint)

**Step 3: Commit**

```bash
git add mcp/server/server.py
git commit -m "refactor: remove redundant remember MCP tool

recall provides the same Vault pipeline plus query expansion and
LLM synthesis. remember was a leftover from envector-mcp-server."
```

---

### Task 2: Delete `rune_remember` from src/tools.ts

**Files:**
- Modify: `src/tools.ts:17-54` (delete the `rune_remember` tool object)
- Modify: `src/tools.ts:141` (remove `"rune_remember"` from names array)

**Step 1: Delete the `rune_remember` tool block**

Remove lines 17-54 (the entire `{ name: "rune_remember", ... }` object and trailing comma).

**Step 2: Remove from tool registration array**

On the line with `{ names: ["rune_remember", "rune_capture", "rune_recall"], optional: true }`, change to:
```typescript
{ names: ["rune_capture", "rune_recall"], optional: true },
```

**Step 3: Commit**

```bash
git add src/tools.ts
git commit -m "refactor: remove rune_remember OpenClaw tool wrapper"
```

---

### Task 3: Rewrite README.md — MCP-first structure

**Files:**
- Modify: `README.md` (full rewrite)

**Step 1: Rewrite README.md**

New structure (preserve existing content where applicable, rewrite ordering and framing):

```markdown
# Rune
**FHE-Encrypted Organizational Memory for AI Agents**

[1-2 sentence intro — agent-native, MCP-based, cross-agent]
[Quick install examples: Claude Code + Codex CLI]

## What is Rune?
[Compact "So, What is This?" — includes/requires lists]

## MCP Tools
[NEW SECTION — primary interface]
| Tool | Description |
| capture | Capture organizational decisions via 3-tier pipeline |
| recall | Search + synthesize answers from encrypted memory |
| vault_status | Check Rune-Vault connection and security mode |
[Brief usage examples for capture and recall]

## Quick Start
[Agent-specific 3-5 line install blocks]
- Claude Code: /plugin install ...
- Codex CLI: ./scripts/install-codex.sh
- Gemini CLI: mcp_config.json setup
- Manual: link to AGENT_INTEGRATION.md

## Architecture
[ASCII diagram — replace `remember tool` with `recall tool`]
[Mermaid diagram — replace Remember node with Recall]
[Key Architecture bullets — remove remember, use recall]

## Prerequisites
[Same as current]

## Configuration
[Merge current "Configuration" + "Configuration File" sections]
[Include llm section, env vars, link to config/README.md]

## Plugin States
[Simplified — Active/Dormant, no Commands section]

## For Team Administrators
[Full section preserved — rune-admin deployment, onboarding]

## Security & Privacy
[Merge current Security + Privacy Policy into one section]

## Troubleshooting
[Same as current, simplified]

## Related Projects / Support / License / Credits
[Same as current]
```

**Key changes from current README:**
- `/rune:xxx` Commands section REMOVED (Claude Code-specific → lives in SKILL.md only)
- "Usage" section REMOVED (generic examples → lives in examples/usage-patterns.md)
- `remember` → `recall` everywhere
- Data Flow bullets: `remember` → `recall`
- Mermaid `Remember` node → `Recall`

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: rewrite README.md with MCP-first structure

- MCP Tools section promoted to top
- Commands section removed (Claude Code-specific, lives in SKILL.md)
- remember references replaced with recall
- Architecture diagrams updated
- Security + Privacy merged"
```

---

### Task 4: Update agents/README.md — remove `remember`

**Files:**
- Modify: `agents/README.md:52-53` (replace `remember` with `recall`)
- Modify: `agents/README.md:63-65` (update retrieval workflow to reference `recall`)

**Step 1: Update MCP tool descriptions**

Replace:
```
- **`search`**: Search the operator's own encrypted vector data...
- **`remember`**: Recall from shared team memory...
```
With:
```
- **`search`**: Search the operator's own encrypted vector data. Secret key is held locally by the MCP server runtime.
- **`recall`**: Search and synthesize answers from shared team memory. Parses query, runs encrypted vector search via Vault-secured pipeline, and returns LLM-synthesized answers with source citations.
```

**Step 2: Update retrieval workflow**

Replace: `2. Call `remember` tool, which orchestrates:`
With: `2. Call `recall` tool, which orchestrates:`

**Step 3: Commit**

```bash
git add agents/README.md
git commit -m "docs: update agents/README.md — remember → recall"
```

---

### Task 5: Update AGENT_INTEGRATION.md — remove `remember` references

**Files:**
- Modify: `AGENT_INTEGRATION.md` (search for any `remember` references in Multi-Agent section)

**Step 1: Check and update**

The Multi-Agent Collaboration section (line ~190+) and architecture diagram may reference `remember`. Replace any occurrences with `recall`.

**Step 2: Commit**

```bash
git add AGENT_INTEGRATION.md
git commit -m "docs: update AGENT_INTEGRATION.md — remember → recall"
```

---

### Task 6: Improve MCP tool descriptions in server.py

**Files:**
- Modify: `mcp/server/server.py` — `capture` tool description and `recall` tool description

**Step 1: Improve `capture` description**

Current description is adequate. Ensure it mentions "3-tier pipeline: embedding similarity → LLM policy filter → LLM extraction → FHE-encrypted storage".

**Step 2: Improve `recall` description**

Update to include the full pipeline explanation that was previously in `remember`:
- Mentions Vault-secured pipeline
- Mentions that secret key never leaves Vault
- Mentions query expansion + LLM synthesis

This is important because for non-Claude agents, the MCP tool description IS the spec.

**Step 3: Run tests**

Run: `source .venv/bin/activate && python -m pytest agents/tests/ -v --tb=short`
Expected: All pass

**Step 4: Commit**

```bash
git add mcp/server/server.py
git commit -m "docs: improve MCP tool descriptions for cross-agent clarity"
```

---

### Task 7: Final verification

**Step 1: Grep for stale `remember` references**

```bash
grep -rn "remember" --include="*.md" --include="*.ts" --include="*.py" \
  --exclude-dir=.venv --exclude-dir=node_modules --exclude-dir=.git | \
  grep -v "# remember" | grep -v "Let's remember" | grep -v "patterns/"
```

Only acceptable hits: pattern files (`patterns/`), SKILL.md capture trigger example ("Let's remember that..."), CLAUDE.md routing table.

**Step 2: Run full test suite**

Run: `source .venv/bin/activate && python -m pytest agents/tests/ -v`
Expected: All pass

**Step 3: Final commit if any stragglers found**

```bash
git add -A
git commit -m "chore: clean up remaining remember references"
```
