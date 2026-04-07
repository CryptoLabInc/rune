# Rune Technical Whitepaper & Slides Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a technical whitepaper (EN+KO, 8-12 pages each) and self-serving reveal.js slide deck (EN+KO, ~22 slides each) for developers familiar with agentic workflows.

**Architecture:** Content-generation plan. Each task produces one deliverable file. English versions first, Korean versions as proper localizations (not mechanical translations). Whitepaper in Markdown, slides in standalone HTML with reveal.js CDN.

**Tech Stack:** Markdown, HTML, reveal.js (CDN), CSS

---

## Task 1: English Whitepaper

**Files:**
- Create: `tmp/whitepaper-rune-en.md`

**Reference materials:**
- Design doc: `docs/plans/2026-03-11-whitepaper-slides-design.md`
- README: `README.md`
- Existing blogs (for differentiation, NOT reuse): `tmp/blog-rune-hashnode.md`, `tmp/blog-rune-hashnode-developer-style.md`

**Step 1: Write the complete English whitepaper**

Write `tmp/whitepaper-rune-en.md` with these sections and constraints:

```
Title: Rune — Encrypted Organizational Memory for Agentic Workflows
Subtitle: A Technical Overview for Developers
```

**Section 1: The Context Evaporation Problem (~1.5 pages)**
- Open with a concrete scenario: a developer in a Claude Code session evaluating PostgreSQL vs MongoDB for a payment service
- Show the actual reasoning that happens: "MongoDB's multi-document transactions have a 60-second timeout, our settlement pipeline can exceed that during peak load, PostgreSQL's row-level locking is more predictable"
- Then show what survives: `git log` says "Switch to PostgreSQL", PR description says "PostgreSQL better fits our requirements"
- The reasoning — the most valuable artifact — evaporates when the terminal closes
- Frame the scale: 10 developers × 20 micro-decisions/day = 200 reasoning artifacts lost daily
- NO corpus callosum or GPU interconnect analogies (those are in the blog)

**Section 2: Why Existing Approaches Fall Short (~2 pages)**
- **Local agent memory** (`.claude/memory`, `CLAUDE.md`): personal scope, no team sharing, lost on machine change. Show that it solves individual continuity but not collective knowledge.
- **RAG pipelines**: Destructure the problem technically:
  - Chunking destroys reasoning structure (a decision spanning 3 messages gets split into unrelated chunks)
  - Embedding loss: the semantic relationship between "we rejected X" and "we chose Y because of Z" is lost when chunked separately
  - Pipeline tax: index maintenance, chunk size tuning, retrieval quality monitoring — ongoing engineering cost
  - Fundamental mismatch: RAG indexes documents, but reasoning is not a document — it's a structured argument
- **Documentation/wikis**: Manual capture, async (decisions happen in real-time, docs are written after the fact), not agent-accessible autonomously
- **Comparison matrix table:**

| Criterion | Local Memory | RAG Pipeline | Wiki/Docs | Rune |
|-----------|-------------|-------------|-----------|------|
| Agent Autonomy | Agent reads own files | Agent queries index | Agent can't access | Agent captures & recalls autonomously |
| Reasoning Preservation | Raw text | Chunked fragments | Prose summaries | Structured decision records (Phase Chain / Bundle Split) |
| Team Sharing | No | Shared index | Shared docs | Encrypted shared memory |
| Security | Local only | Plaintext vectors in cloud | Plaintext docs | FHE-encrypted vectors, secret key in Vault |
| Vendor Independence | Tied to one agent | Tied to embedding model + vector DB | N/A | MCP protocol, any agent |
| Maintenance Cost | None | High (pipeline engineering) | High (manual effort) | Low (plugin install) |

**Section 3: Rune Architecture — Two Primitives (~2.5 pages)**

Introduce Capture and Recall through scenarios, then show the technical pipeline.

**Capture scenario:**
```
Developer (in Claude Code session):
  "We need a caching layer for the product catalog. Redis is the obvious
   choice but our team has limited Redis ops experience. Let's use
   PostgreSQL's built-in caching with pg_prewarm and materialized views
   instead — we already have PG expertise and the read pattern is
   predictable enough."

Agent detects: architectural decision with rationale + rejected alternative
```

Then walk through the 3-tier pipeline:
- Tier 1 (Embedding Similarity): Zero-LLM pattern matching. Embed the text, cosine similarity against pre-embedded decision patterns (19 categories: architecture, debugging, compliance...). Threshold 0.35. Cost: ~0ms LLM tokens.
- Tier 2 (Policy Filter): Claude Haiku (~200 tokens) evaluates organizational relevance. "Is this a team-level decision or just a personal preference?"
- Tier 3 (Structured Extraction): Claude Sonnet (~500 tokens) extracts structured fields:
  - Decision: "Use PostgreSQL caching (pg_prewarm + materialized views) instead of Redis"
  - Rationale: "Team has PG expertise, predictable read patterns"
  - Alternatives rejected: "Redis — limited ops experience"
  - Status: "accepted" (detected from "Let's use")
  - Certainty: "supported" (direct quotes present)

Then explain Phase Chain and Bundle Split:
- **Phase Chain**: When reasoning exceeds 800 chars, auto-decompose into 2-7 linked phases (e.g., "Requirements Analysis → Technology Evaluation → Decision"). Each phase is independently searchable but linked via `group_id`.
- **Bundle Split**: When a single decision has >3 alternatives AND >3 trade-offs (or >1500 chars), split into core decision + detail facets (alternatives analysis, trade-offs deep-dive, implementation plan). Each facet is independently queryable.
- Both use `group_id` linking: when any member is found during recall, siblings are auto-fetched.

Finally, show the encryption step: text → embedding → FHE encryption (EncKey) → enVector Cloud. Plaintext never leaves the machine.

**Recall scenario:**
```
Two weeks later, different team member (in Codex CLI session):
  "I'm adding a recommendation engine. Need to cache computed
   recommendations — should I use Redis or stick with PostgreSQL?"

Agent autonomously queries Rune shared memory...
```

Walk through:
1. Query processor: expand query into 3-5 semantic variants ("caching strategy", "Redis vs PostgreSQL", "caching layer decision")
2. Each variant: embed → send to enVector Cloud → encrypted similarity scoring (HE operations on ciphertext) → result ciphertext to Vault
3. Vault: decrypt scores only (secret key never leaves Vault), return top-k indices
4. MCP server: retrieve full records, decrypt metadata
5. Synthesizer: reconstruct decision narrative respecting certainty levels
6. Return to agent: "Two weeks ago, the team chose PostgreSQL caching over Redis due to team expertise. The read pattern was predictable enough for materialized views. Consider whether recommendation caching has similar access patterns."

Include ASCII architecture diagram:
```
Your Machine                         Cloud / Team Infrastructure
─────────────                        ──────────────────────────
AI Agent (Claude/Codex/Gemini)
  │
  ├─ capture(text, extracted?)
  │    ├─ Tier 1: Embedding similarity (local, zero LLM)
  │    ├─ Tier 2: Policy filter (Haiku, ~200 tokens)
  │    ├─ Tier 3: Structured extraction (Sonnet, ~500 tokens)
  │    ├─ Phase Chain / Bundle Split (auto)
  │    └─ FHE encrypt (EncKey) ──────────→ enVector Cloud
  │                                         (encrypted vectors only)
  ├─ recall(query, topk?)
  │    ├─ Query expansion (3-5 variants)
  │    ├─ Embed + send ──────────────────→ Encrypted similarity scoring
  │    │                                    (HE ops on ciphertext)
  │    │                                         │
  │    │                                    Result ciphertext
  │    │                                         │
  │    │                                         ▼
  │    │                                   Rune-Vault
  │    │                                   (secret key holder)
  │    │                                    Decrypt scores
  │    │                                    Return top-k indices
  │    │    ◄────────────────────────────────────┘
  │    ├─ Retrieve records + decrypt metadata
  │    ├─ Phase chain expansion (auto-fetch siblings)
  │    └─ Synthesize decision narrative
  │
  └─ vault_status() / reload_pipelines()
```

**Section 4: Trust Model — FHE in Practice (~2 pages)**

Define threat model explicitly:
- **Threat 1**: Cloud provider is compromised (reads stored data) → FHE: all stored data is ciphertext, mathematically unreadable without secret key
- **Threat 2**: Network intercept (man-in-the-middle) → Data in transit is already encrypted (FHE ciphertext), additional TLS layer
- **Threat 3**: Agent is compromised (malicious plugin/tool) → Agent has EncKey (public) but not SecKey. Can insert but cannot bulk-decrypt. Vault limits query rate and logs audit trail.
- **Threat 4**: Vault is compromised → Vault has SecKey but never sees raw vectors or query text. It decrypts similarity scores only, which are meaningless without the original vectors.

Component knowledge table:

| Component | Has | Does NOT Have |
|-----------|-----|---------------|
| AI Agent + MCP Server | Plaintext (local), EncKey, EvalKey | SecKey |
| enVector Cloud | Encrypted vectors, EvalKey | SecKey, plaintext, EncKey |
| Rune-Vault | SecKey, score ciphertext | Raw vectors, plaintext queries, EncKey |

Key invariants:
1. Secret key never leaves Vault
2. Vault never sees raw vectors or original text
3. enVector Cloud never sees plaintext
4. Agent never contacts Vault directly (MCP server mediates)
5. No single component compromise exposes the full data

Show data form at each boundary (table):

| Boundary | Data Form |
|----------|-----------|
| Agent → MCP Server | Plaintext (local process) |
| MCP Server → enVector Cloud | FHE ciphertext (encrypted vector) |
| enVector Cloud → MCP Server | Result ciphertext (encrypted similarity scores) |
| MCP Server → Vault | Score ciphertext (for decryption) |
| Vault → MCP Server | Cleartext indices + scores (no vectors, no text) |

**Section 5: Agent-Agnostic Design (~1 page)**
- MCP (Model Context Protocol) as universal agent interface — stdio transport, any MCP-compatible client
- Same `capture` and `recall` tools across Claude Code, Codex CLI, Gemini CLI
- Agent-delegated mode: calling agent performs LLM evaluation, passes structured JSON via `extracted` parameter. Rune MCP server does zero LLM calls — only FHE operations. Result: no Anthropic/OpenAI/Google API key needed by Rune itself.
- Show the `capture` call with `extracted` parameter:

```python
capture(
    text="We chose PostgreSQL caching over Redis...",
    extracted='{"group_title": "Caching Strategy", "phases": [...]}'
)
# Tiers 1-3 skipped entirely. MCP server does encryption + storage only.
```

- Memory portability: Monday Claude Code → Tuesday Codex → Wednesday Gemini CLI. Same encrypted organizational memory across all.

**Section 6: Getting Started (~1 page)**
- Installation (Claude Code, Codex CLI, Gemini CLI examples)
- Prerequisites: Rune-Vault access (from team admin) + enVector Cloud credentials
- First capture example (CLI)
- First recall example (CLI)
- Link to rune-admin for infrastructure deployment

**Footer:**
- GitHub: github.com/CryptoLabInc/rune
- enVector Cloud: envector.io
- CryptoLab: cryptolab.co.kr

**Step 2: Review for accuracy**

Cross-check all technical claims against the codebase:
- Tier thresholds (0.35 for Tier 1, 0.7 for auto-capture)
- Phase Chain threshold (800 chars)
- Bundle Split threshold (1500 chars OR >3 alternatives AND >3 trade-offs)
- Certainty rules (no quotes → downgrade to UNKNOWN)
- Token costs (~200 Haiku for Tier 2, ~500 Sonnet for Tier 3)
- Architecture diagram accuracy

**Step 3: Commit**

```bash
git add tmp/whitepaper-rune-en.md
git commit -m "docs: add English technical whitepaper for Rune"
```

---

## Task 2: Korean Whitepaper

**Files:**
- Create: `tmp/whitepaper-rune-ko.md`

**Step 1: Write the Korean whitepaper as a proper localization**

Use `tmp/whitepaper-rune-en.md` as the source but produce a natural Korean document, not a translation. The Korean developer blog posts (`tmp/blog-rune-hashnode-ko.md`, `tmp/blog-rune-hashnode-life-style.md`) provide tone reference — but the whitepaper should be more technical and less narrative than those blogs.

Key localization notes:
- Technical terms keep English in parentheses: 완전 동형 암호(FHE), 의미론적 압축(Semantic Compression)
- Code blocks stay in English
- Table headers can be Korean
- Same structure, same technical depth, natural Korean prose

**Step 2: Commit**

```bash
git add tmp/whitepaper-rune-ko.md
git commit -m "docs: add Korean technical whitepaper for Rune"
```

---

## Task 3: English Slides (reveal.js HTML)

**Files:**
- Create: `tmp/slides-rune-en.html`

**Step 1: Write the complete reveal.js HTML slide deck**

Standalone HTML file. reveal.js loaded from CDN:
```html
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/reveal.js@5.1.0/dist/reveal.css">
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/reveal.js@5.1.0/dist/theme/white.css">
<script src="https://cdn.jsdelivr.net/npm/reveal.js@5.1.0/dist/reveal.js"></script>
```

Design constraints:
- Self-serving deck: enough text to be readable without a presenter
- Clean, minimal design (white theme with dark text)
- Code blocks with syntax highlighting (highlight.js plugin via CDN)
- Each slide should have a clear heading + 2-4 bullet points or a diagram/table
- Use `<section>` tags for slides, `---` not needed in HTML

Slide content (derived from whitepaper, condensed for slide format):

**Slide 1: Title**
- "Rune: Encrypted Organizational Memory for Agentic Workflows"
- "A Technical Overview" / CryptoLab

**Slide 2: The Problem**
- "What disappears after `git commit`?"
- Show: git log captures WHAT changed, not WHY

**Slide 3: Scenario — Context Evaporation**
- Developer evaluates PostgreSQL vs MongoDB
- Reasoning: transaction timeouts, row-level locking, settlement pipeline constraints
- After commit: "Switch to PostgreSQL" — reasoning gone

**Slide 4: The Scale**
- 10 developers × 20 decisions/day = 200 reasoning artifacts lost daily
- Each decision: architecture choices, rejected alternatives, trade-off analysis

**Slide 5: What Existing Tools Capture**
- Git: diffs and commit messages (WHAT)
- PRs: sanitized summaries
- Docs/wikis: manual, after-the-fact
- Agent memory: personal scope only

**Slide 6: Existing Approaches Comparison**
- Comparison matrix table (condensed from whitepaper Section 2)

**Slide 7: Introducing Rune**
- "Organizational memory for AI agents"
- Two primitives: Capture + Recall
- Agent-native, encrypted, vendor-independent

**Slide 8: Capture — What Happens**
- Developer discusses caching strategy with agent
- Agent detects decision → structured extraction → FHE encryption → cloud

**Slide 9: 3-Tier Capture Pipeline**
- Tier 1: Embedding similarity (0ms LLM, pattern match)
- Tier 2: Policy filter (Haiku, ~200 tokens)
- Tier 3: Structured extraction (Sonnet, ~500 tokens)
- Visual: funnel narrowing from raw text to structured record

**Slide 10: Semantic Compression — Phase Chain**
- Long reasoning → 2-7 linked phases
- Example: "Requirements → Evaluation → Decision"
- Each phase independently searchable, linked via group_id

**Slide 11: Semantic Compression — Bundle Split**
- Rich decision → core + facets
- Core Decision / Alternatives Analysis / Trade-offs / Implementation Plan
- Auto-fetch siblings on recall

**Slide 12: Recall — What Happens**
- Different developer, 2 weeks later, same domain
- Agent autonomously queries encrypted shared memory
- Returns decision narrative with context

**Slide 13: Recall Pipeline**
- Query expansion → encrypted scoring → Vault decryption → synthesis
- 4-step visual flow

**Slide 14: Architecture Diagram**
- Full architecture (capture + recall flows)
- ASCII art rendered as styled diagram

**Slide 15: Trust Model — Threat Scenarios**
- 4 threats: cloud compromise, network intercept, agent compromise, vault compromise
- How FHE addresses each

**Slide 16: Who Knows What**
- Component knowledge table (Agent / enVector / Vault)

**Slide 17: Data Form at Each Boundary**
- Visual: plaintext → ciphertext → ciphertext → ciphertext → cleartext scores
- Color-coded by encryption state

**Slide 18: Agent-Agnostic Design**
- MCP protocol = universal interface
- Same tools across Claude Code, Codex CLI, Gemini CLI
- Agent-delegated mode: zero LLM keys for Rune

**Slide 19: Memory Portability**
- Monday: Claude Code / Tuesday: Codex / Wednesday: Gemini
- Same organizational memory throughout

**Slide 20: Getting Started**
- 2-line install for each platform
- Prerequisites: Vault access + enVector credentials

**Slide 21: Capture/Recall Demo**
- Code blocks showing actual capture() and recall() calls
- Expected output format

**Slide 22: Key Takeaways**
- Reasoning is the most valuable artifact — stop losing it
- FHE makes shared cloud memory safe by mathematics, not trust
- Agent-native: no new UI, no pipeline, just a plugin
- Links: GitHub, enVector, CryptoLab

**Step 2: Test locally**

Open the HTML file in a browser to verify it renders correctly:
```bash
open tmp/slides-rune-en.html
```

**Step 3: Commit**

```bash
git add tmp/slides-rune-en.html
git commit -m "docs: add English reveal.js slide deck for Rune"
```

---

## Task 4: Korean Slides (reveal.js HTML)

**Files:**
- Create: `tmp/slides-rune-ko.html`

**Step 1: Write the Korean slide deck**

Same structure as English slides. Proper Korean localization (not translation). Technical terms in English with Korean in parentheses or vice versa as appropriate for slides.

Same reveal.js CDN setup, same design. Korean content naturally written.

**Step 2: Test locally**

```bash
open tmp/slides-rune-ko.html
```

**Step 3: Commit**

```bash
git add tmp/slides-rune-ko.html
git commit -m "docs: add Korean reveal.js slide deck for Rune"
```

---

## Execution Notes

- Tasks 1-2 (whitepapers) are independent of Tasks 3-4 (slides), but slides derive content from whitepapers, so write whitepapers first.
- English versions before Korean versions (Korean localizes from English).
- Total deliverables: 4 files in `tmp/`.
