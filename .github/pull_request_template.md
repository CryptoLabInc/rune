## Summary

- What changed:
- Why:
- Scope:

## Validation

- [ ] Tests run (or explain why not):
- [ ] Docs updated (if behavior/setup changed)

## Cross-Agent Invariants

- [ ] `scripts/bootstrap-mcp.sh` remains the single source of truth for runtime prep (venv/deps/self-heal)
- [ ] No agent-specific script duplicates bootstrap/setup logic
- [ ] Agent-specific scripts remain thin adapters (registration/wiring only)
- [ ] Codex-only commands (`codex mcp ...`) are clearly separated from cross-agent/common instructions
- [ ] Claude/Gemini/OpenAI instructions do not include Codex-only commands
- [ ] `SKILL.md`, `commands/rune/*.toml`, and `AGENT_INTEGRATION.md` stay consistent on boundaries

## Notes for Reviewers

- Risk areas:
- Backward compatibility impact:
- Follow-up work (if any):
