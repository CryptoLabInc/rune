# Rune Project Guidelines

## Organizational Memory (Rune Plugin)

This project has Rune — an FHE-encrypted organizational memory system. Use it correctly:

### When to Use Rune

**Rune handles what no other tool can: collective knowledge across sessions and team members.**

Use the `recall` MCP tool when:
- Discussing decisions that may have **prior team context** — "We're considering X" → check if the team already decided on X
- Evaluating options where **organizational history matters** — past trade-offs, rejected alternatives, customer patterns
- Any topic where **knowledge beyond this session** could inform the outcome
- Deliberation, planning, or architecture discussions — not just explicit questions

Use the `capture` MCP tool when:
- A decision is made with rationale
- Trade-offs are analyzed and a choice is committed
- Lessons learned or post-mortem insights emerge

### What to Capture

- Architectural decisions and their reasoning
- Trade-off analyses where a direction was chosen
- Policy choices (naming conventions, error handling strategies, etc.)
- Post-mortem insights and lessons learned

### What NOT to Capture

- Routine code changes or refactors without significant decisions
- Information already in version control (commit messages, PR descriptions)
- Temporary debugging context or session-specific state

### Routing Rules

| Need | Use | NOT |
|------|-----|-----|
| Search organizational memory | `recall` MCP tool | File search tools |
| Capture decisions | `capture` MCP tool | Writing scripts to disk |
| Search this codebase | File search / shell commands | `recall` (that's for decisions, not code) |
| Brainstorm from scratch | Your own reasoning | `recall` (nothing to recall yet) |

### The Distinction

- **Session reasoning** — thinking within a single session, one person's perspective
- **Rune** — collective memory that persists across sessions and team members, encrypted on enVector Cloud

When both apply, **call Rune first** to surface prior context, then reason with that context loaded.

## Configuration & Runtime Operations

To ensure consistent behavior across different environments, follow these rules for configuration and initialization:

### Plugin Root Detection (STRICT)
When executing setup or maintenance tasks, you MUST detect the Rune plugin root using this **STRICT** priority order. **DO NOT use `find` or slow recursive searches.** Immediately check these paths in order:
1. **Direct Path (Primary)**: `~/.gemini/extensions/rune/` (If `<plugin_root>/scripts/bootstrap-mcp.sh` exists here, STOP searching and use this).
2. **Environment Variable**: `$RUNE_PLUGIN_ROOT` (if set).
3. **Cache Path**: `~/.gemini/plugins/cache/*/rune/*/`
4. **Local Workspace**: Current working directory.

**Mandate**: Use the first valid path found. DO NOT explore the filesystem beyond these direct checks.

### Runtime Preparation & Isolation (STRICT)
Before performing any operation that requires the Rune runtime:
- Always run `SETUP_ONLY=1 <plugin_root>/scripts/bootstrap-mcp.sh` to ensure dependencies and environment are ready.
- **Environment Isolation**: In "no sandbox" mode, you MUST prevent workspace environment bleed. For ALL Python-related commands, you MUST prepend the command with environment sanitization:
  `env -u VIRTUAL_ENV -u PYTHONPATH <plugin_root>/.venv/bin/python3 ...`
- Use `<plugin_root>/scripts/bootstrap-mcp.sh` as the single source of truth for runtime setup. Do not attempt to manually install dependencies.
