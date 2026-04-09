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

Use `capture` MCP tool (with `extracted` JSON parameter) when:
- A decision is made with rationale
- Trade-offs are analyzed and a choice is committed
- Lessons learned or post-mortem insights emerge

### Automatic Capture (Proactive Scribe)

When Rune state is `"active"`, **proactively capture significant decisions** when you detect any of the following in the conversation:

- A choice is made among alternatives ("A로 가자", "let's go with X")
- Trade-offs are weighed and committed ("X의 단점이 있지만 Y 때문에 감수")
- Strategy or direction is confirmed ("이 방향이 맞아", "this approach works")
- A lesson or insight crystallizes ("안 된 이유는...", "the root cause was...")
- A framework, process, or standard is established

**How to capture:**
- Follow the **Agent-Delegated Mode** instructions in `agents/gemini/scribe.md`.
- Evaluate if the conversation segment contains a significant decision (Tier 2).
- Extract structured JSON (Tier 3) according to Format A, B, or C in `scribe.md`.
- Call `mcp_envector_capture` with the `extracted` parameter.
- In the `text` parameter, include ONLY the relevant conversation excerpt (not the full context).
- Do NOT pause or interrupt the main conversation — continue responding to the user.
- Do NOT announce the capture to the user unless they ask.

**Do NOT auto-capture:**
- Brainstorming in progress without commitment (options listed but none chosen)
- Questions, status updates, or casual discussion
- Decisions that are hypothetical or deferred ("maybe later", "let's revisit")

**Session-end sweep:** A `SessionEnd` hook will remind you when the session is ending. Review the conversation for any uncaptured significant decisions and use the `mcp_envector_batch_capture` tool to capture them now as a batch if needed.

### State Gate

**Before calling `capture` or `recall` MCP tools, check `~/.rune/config.json` state:**
- `"active"` — proceed normally.
- `"dormant"` or missing — **do NOT call MCP tools.** Skip silently. This avoids unnecessary overhead when Rune is deactivated.

### Routing Rules

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
3. **Local Workspace**: Current working directory.

**Mandate**: Use the first valid path found. DO NOT explore the filesystem beyond these direct checks.

### Runtime Preparation & Isolation (STRICT)
Before performing any operation that requires the Rune runtime:
- Always run `SETUP_ONLY=1 <plugin_root>/scripts/bootstrap-mcp.sh` to ensure dependencies and environment are ready.
- **Environment Isolation**: In "no sandbox" mode, you MUST prevent workspace environment bleed. For ALL Python-related commands, you MUST prepend the command with environment sanitization:
  `env -u VIRTUAL_ENV -u PYTHONPATH <plugin_root>/.venv/bin/python3 ...`
- Use `<plugin_root>/scripts/bootstrap-mcp.sh` as the single source of truth for runtime setup. Do not attempt to manually install dependencies.
