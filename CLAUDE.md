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

### Automatic Capture (Proactive Scribe)

When Rune state is `"active"`, **proactively spawn a background `rune:scribe` subagent** when you detect any of the following in the conversation:

- A choice is made among alternatives ("A로 가자", "let's go with X")
- Trade-offs are weighed and committed ("X의 단점이 있지만 Y 때문에 감수")
- Strategy or direction is confirmed ("이 방향이 맞아", "this approach works")
- A lesson or insight crystallizes ("안 된 이유는...", "the root cause was...")
- A framework, process, or standard is established

**How to spawn:**
- Use `Agent` tool with `subagent_type: "rune:scribe"` and `run_in_background: true`
- In the prompt, include ONLY the relevant conversation excerpt (not the full context)
- Do NOT pause or interrupt the main conversation — continue responding to the user
- Do NOT announce the capture to the user unless they ask

**Do NOT auto-capture:**
- Brainstorming in progress without commitment (options listed but none chosen)
- Questions, status updates, or casual discussion
- Decisions that are hypothetical or deferred ("maybe later", "let's revisit")

### State Gate

**Before spawning `rune:scribe` or `rune:retriever` subagents, or calling `capture`/`recall` MCP tools, check `~/.rune/config.json` state:**
- `"active"` — proceed normally.
- `"dormant"` or missing — **do NOT spawn subagents or call MCP tools.** Skip silently. This avoids unnecessary overhead when Rune is deactivated.

### Routing Rules

| Need | Use | NOT |
|------|-----|-----|
| Search organizational memory | `rune:retriever` / `recall` MCP tool | Explore agent (no MCP access) |
| Capture decisions | `rune:scribe` / `capture` MCP tool (agent-delegated: pass `extracted` JSON) | Writing Python scripts to `/tmp` |
| Search this codebase | Explore agent / Glob / Grep | `recall` (that's for decisions, not code) |
| Brainstorm from scratch | `superpowers:brainstorming` | `recall` (nothing to recall yet) |

### The Distinction

- **Builtin skills** (brainstorming, planning, etc.) — reasoning within a single session, one person's perspective
- **Rune** — collective memory that persists across sessions and team members, encrypted on enVector Cloud

When both apply, **call Rune first** to surface prior context, then brainstorm with that context loaded.
