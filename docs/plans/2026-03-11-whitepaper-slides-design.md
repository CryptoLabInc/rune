# Design: Rune Technical Whitepaper & Slides

## Overview
Create a technical whitepaper (8-12 pages, EN+KO) and self-serving reveal.js slide deck (EN+KO) targeting developers familiar with agentic workflows.

## Approach: Developer Workflow-Centered
Structure around concrete developer scenarios rather than abstract analogies (which the blog already covers well). Show the problem and solution through actual CLI interactions, code, and data flow.

## Differentiation from Existing Blog Content
- Blogs use narrative/essay style with metaphors (corpus callosum, GPU interconnect, locked safe)
- Whitepaper uses concrete code scenarios, architecture diagrams, comparison matrices, and trust model tables
- Minimal reuse of blog analogies; fresh framing throughout

## Deliverables

### 1. Whitepaper (EN + KO)
- `tmp/whitepaper-rune-en.md`
- `tmp/whitepaper-rune-ko.md`
- ~8-12 pages each

### 2. Slides (EN + KO)
- `tmp/slides-rune-en.html` (reveal.js)
- `tmp/slides-rune-ko.html` (reveal.js)
- Self-serving deck (readable without presenter)
- ~20-25 slides each

## Whitepaper Structure

### Section 1: The Context Evaporation Problem (~1.5 pages)
- Concrete scenario: developer choosing DB in agent session
- Show what git log/PR captures vs. what evaporates
- CLI-level demonstration of the gap

### Section 2: Why Existing Approaches Fall Short (~2 pages)
- Local agent memory (`.claude/memory`, `CLAUDE.md`) — personal scope only
- RAG pipelines — chunking loss, reasoning structure destruction, maintenance cost
- Documentation + wikis — manual, async, not agent-accessible
- Comparison matrix table (agent autonomy, reasoning preservation, security, vendor independence)

### Section 3: Rune Architecture — Two Primitives (~2.5 pages)
- **Capture scenario**: caching strategy discussion → decision detection → semantic compression (Phase Chain + Bundle Split) → FHE encryption → enVector Cloud
- **Recall scenario**: different team member, 2 weeks later → encrypted query → similarity scoring on ciphertext → Vault decryption → decision narrative reconstruction
- Architecture diagram with data flow

### Section 4: Trust Model — FHE in Practice (~2 pages)
- Explicit threat model definition
- Component knowledge table (Agent / enVector Cloud / Vault — knows / doesn't know)
- Data flow diagram showing data form at each boundary
- Key trust boundaries: secret key never leaves Vault, Cloud never sees plaintext, Agent never contacts Vault directly

### Section 5: Agent-Agnostic Design (~1 page)
- MCP protocol-based vendor independence
- Same `capture`/`recall` across Claude Code, Codex CLI, Gemini CLI
- Agent-delegated mode (no LLM API key required by Rune)

### Section 6: Getting Started (~1 page)
- Installation walkthrough
- First capture/recall concrete example
- Infrastructure requirements summary

## Slides Structure (~20-25 slides)

1. Title slide
2. The problem: what disappears after `git commit`
3. Scenario: DB selection reasoning → evaporation
4. What tools capture today (git diff, PR, docs)
5. The gap: reasoning is the missing layer
6. Existing approaches comparison matrix
7. Introducing Rune: organizational memory for agents
8. Two primitives: Capture & Recall
9. Capture flow scenario
10. Semantic compression: Phase Chain
11. Semantic compression: Bundle Split
12. Recall flow scenario
13. Architecture overview diagram
14. Trust model: who knows what
15. FHE data flow: encryption boundaries
16. Data form at each boundary (visual)
17. Agent-agnostic: MCP protocol
18. Cross-agent memory demonstration
19. Getting started: 2-line install
20. Capture/Recall CLI example
21. Infrastructure overview
22. Summary / key takeaways

## Technical Decisions
- reveal.js loaded from CDN (no local dependencies)
- Whitepaper in Markdown (convertible to PDF via any Markdown→PDF tool)
- Diagrams described in text/ASCII art within Markdown; mermaid or SVG in slides
- Korean version is a proper localization, not a mechanical translation
