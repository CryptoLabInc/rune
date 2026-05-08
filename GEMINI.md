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

**Session-end sweep:** When the user signals they're done (goodbye, thanks, wrapping up), review the conversation for any uncaptured significant decisions and use the `mcp_envector_batch_capture` tool to capture them now as a batch if needed.

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

In v0.4 Rune ships as a single Go binary (`bin/rune-mcp`) declared by the
plugin / extension manifest. The host CLI (Gemini / Claude / Codex) auto-
spawns it on session start over stdio — there is no venv, no install
script, no manual `pip` step. Runtime preparation happens at install time;
nothing needs to be (re)bootstrapped at session start.

### Plugin Root Detection
You only need the plugin root for diagnostics or to inspect
`~/.gemini/extensions/rune/` artifacts. The MCP server itself is started
by the CLI; you never invoke it manually.

If you do need the path (e.g. to read extension files):
1. **Default install location**: `~/.gemini/extensions/rune/`
2. **Environment variable**: `$RUNE_PLUGIN_ROOT` (if set)
3. **Local workspace**: current working directory (dev mode)

Use the first valid path found. Do NOT shell out to `find` or recurse
through the filesystem.

### Runtime Health
The MCP server's health is observable via the `diagnostics` and
`vault_status` tools — call those instead of probing the binary directly.
There is no Python environment, no venv, no `bootstrap-mcp.sh` to source.
If the boot loop hasn't reached Active, `/rune:status` surfaces the exact
sub-system that's failing (Vault / Embedder / enVector / etc.) and the
recovery action.
