# Documentation Rewrite Design — MCP-First

**Date**: 2026-02-28
**Status**: Approved

## Context

The Rune plugin documentation was originally written for Claude Code. With cross-agent support (v0.2.0), the docs have become inconsistent — MCP tools are the universal interface but documentation is structured around Claude Code slash commands.

## Decisions

1. **MCP-first documentation** — README centers on MCP tools (`capture`, `recall`) as the primary interface. Agent-specific setup lives in AGENT_INTEGRATION.md.
2. **Remove `remember` MCP tool** — Completely redundant with `recall` (same 4-step Vault pipeline, `recall` adds query expansion + LLM synthesis on top).
3. **Admin tools stay non-MCP** — `configure`, `status`, `activate` remain as Claude Code slash commands + install scripts. No new MCP tools for admin.
4. **Agent specs stay Claude Code-specific** — `agents/scribe.md` and `agents/retriever.md` are only read by Claude Code. Other agents see MCP tool descriptions from `server.py`.
5. **Incremental rewrite** — Keep file structure, rewrite content. No new files or directory restructure.
6. **Admin section preserved** — "For Team Administrators" section stays in README with full explanation (rune-admin is critical infrastructure).

## MCP Tool Surface (after cleanup)

| Tool | Purpose | Annotations |
|------|---------|-------------|
| `capture` | Capture organizational decisions (3-tier pipeline) | destructive |
| `recall` | Search + synthesize from encrypted memory | read-only |
| `vault_status` | Check Rune-Vault connection | read-only |
| `reload_pipelines` | Reinitialize scribe/retriever after config change | non-destructive |

**Removed**: `remember` (redundant with `recall`)

## README.md New Structure

```
# Rune
## What is Rune?
## MCP Tools
  - capture, recall, vault_status
## Quick Start
  - Claude Code / Codex CLI / Gemini CLI / Manual
## Architecture (mermaid diagram, no `remember`)
## Prerequisites
## Configuration
  - config.json schema
  - LLM provider settings
  - Environment variables
## Plugin States (simplified)
## For Team Administrators (full section)
## Security & Privacy (merged)
## Troubleshooting
## Related Projects
```

## File Change Matrix

| File | Change | Scope |
|------|--------|-------|
| `mcp/server/server.py` | Delete `tool_remember` (~140 lines), improve `recall`/`capture` descriptions | Code |
| `src/tools.ts` | Delete `rune_remember` registration | Code |
| `README.md` | MCP-first rewrite per structure above | Doc |
| `AGENT_INTEGRATION.md` | Remove `remember` references, update diagrams | Doc |
| `agents/README.md` | `remember` → `recall` in MCP tool list | Doc |
| `agents/retriever.md` | Remove `remember` references | Doc |
| `SKILL.md` | Clean up `remember` mentions in recall description | Doc |
| `examples/usage-patterns.md` | No change | — |
| `agents/scribe.md` | No change | — |
| `config/README.md` | No change | — |
| `CONTRIBUTING.md` | No change | — |
