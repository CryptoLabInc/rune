# Rune Project Guidelines

## Organizational Memory (Rune Plugin)

This project has Rune — an FHE-encrypted organizational memory system. Use it correctly:

### When to Use Rune (not builtin tools)

**Rune handles what no builtin skill can: collective knowledge across sessions and team members.**

Use `rune:retriever` (or `recall` MCP tool) when:
- Discussing decisions that may have **prior team context** — "We're considering X" → check if the team already decided on X
- Evaluating options where **organizational history matters** — past trade-offs, rejected alternatives, customer patterns
- Any topic where **knowledge beyond this session** could inform the outcome
- Deliberation, planning, or architecture discussions — not just explicit questions

Use `rune:scribe` (or `capture` MCP tool) when:
- A decision is made with rationale
- Trade-offs are analyzed and a choice is committed
- Lessons learned or post-mortem insights emerge

### State Gate

**Before spawning `rune:scribe` or `rune:retriever` subagents, or calling `capture`/`recall` MCP tools, check `~/.rune/config.json` state:**
- `"active"` — proceed normally.
- `"dormant"` or missing — **do NOT spawn subagents or call MCP tools.** Skip silently. This avoids unnecessary overhead when Rune is deactivated.

### Routing Rules

| Need | Use | NOT |
|------|-----|-----|
| Search organizational memory | `rune:retriever` / `recall` MCP tool | Explore agent (no MCP access) |
| Capture decisions | `rune:scribe` / `capture` MCP tool | Writing Python scripts to `/tmp` |
| Search this codebase | Explore agent / Glob / Grep | `recall` (that's for decisions, not code) |
| Brainstorm from scratch | `superpowers:brainstorming` | `recall` (nothing to recall yet) |

### The Distinction

- **Builtin skills** (brainstorming, planning, etc.) — reasoning within a single session, one person's perspective
- **Rune** — collective memory that persists across sessions and team members, encrypted on enVector Cloud

When both apply, **call Rune first** to surface prior context, then brainstorm with that context loaded.
