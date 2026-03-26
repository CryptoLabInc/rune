# Memory-as-Filter: Brain-Inspired Capture Pipeline

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement the associated plan.

**Goal:** Replace the pattern-based capture filter with a brain-inspired, self-evolving system where the memory itself acts as the filter — and redefine the shape of stored knowledge for optimal semantic recall.

**Architecture:** Agent generates a dense natural-language "reusable insight" (the engram). Before storing, enVector recall checks novelty against existing memory. Only novel or evolutionary knowledge passes through. No patterns. No maintenance. The filter improves as memory grows.

---

## Part 1: Why Rune Is Not What You Think

### The Filing Cabinet vs The Brain

Most people approach Rune with the wrong mental model:

```
         What people expect              What Rune actually is
         ━━━━━━━━━━━━━━━━━              ━━━━━━━━━━━━━━━━━━━━

         ┌──────────────┐               ┌ ─ ─ ─ ─ ─ ─ ─ ─ ┐
         │  FILING      │                  YOUR AGENT'S
         │  CABINET     │               │  LONG-TERM       │
         │              │                  MEMORY
         │  ┌────┐      │               │                  │
         │  │ A  │      │                ╭─────────────╮
         │  ├────┤      │               ││  Experiences ││
         │  │ B  │      │                │  from every  │
         │  ├────┤      │               ││  team member ││
         │  │ C  │      │                ╰─────────────╯
         │  └────┘      │               │                  │
         └──────────────┘               └ ─ ─ ─ ─ ─ ─ ─ ─ ┘

         You open drawer B.             You don't search.
         You find document #42.         You just... know things
         Exact match.                   you never learned.
```

### Three Conversations That Show The Difference

**Scenario 1: The New Team Member**

```
╭─ Without Rune ──────────────────────────────────────╮
│                                                      │
│  Developer: "Should we use MongoDB for this?"        │
│                                                      │
│  Agent: "MongoDB is great for flexible schemas..."   │
│         (generic advice, no team context)             │
│                                                      │
│  Developer builds with MongoDB.                      │
│  Two weeks later: "Wait, the team rejected           │
│  MongoDB three months ago for ACID reasons??"        │
│                                                      │
╰──────────────────────────────────────────────────────╯

╭─ With Rune ─────────────────────────────────────────╮
│                                                      │
│  Developer: "Should we use MongoDB for this?"        │
│                                                      │
│  Agent: "Your team chose PostgreSQL over MongoDB     │
│         for the financial data layer in March.       │
│         The key reason was ACID compliance for       │
│         transaction integrity. MongoDB's eventual    │
│         consistency was considered unacceptable."    │
│                                  ── from Rune        │
│                                                      │
│  The developer never searched.                       │
│  The agent just knew.                                │
│                                                      │
╰──────────────────────────────────────────────────────╯
```

**Scenario 2: The Architecture Review**

```
╭─ Without Rune ──────────────────────────────────────╮
│                                                      │
│  Lead: "Let's discuss the auth middleware rewrite"   │
│                                                      │
│  Agent: (starts from scratch)                        │
│         "Here are some common approaches..."         │
│                                                      │
│  30 minutes wasted re-deriving context that          │
│  already existed in Slack threads nobody reads.      │
│                                                      │
╰──────────────────────────────────────────────────────╯

╭─ With Rune ─────────────────────────────────────────╮
│                                                      │
│  Lead: "Let's discuss the auth middleware rewrite"   │
│                                                      │
│  Agent: "Before we start — your team has prior       │
│         context on this:                             │
│         - Legal flagged the current middleware for   │
│           session token storage (compliance issue)   │
│         - The team evaluated JWT vs session-based    │
│           in January, leaning toward JWT             │
│         - Security team requires token rotation      │
│           under 24h for the new solution"            │
│                                                      │
│  Nobody asked. The context just appeared.            │
│  The meeting starts 30 minutes ahead.                │
│                                                      │
╰──────────────────────────────────────────────────────╯
```

**Scenario 3: The Debugging Session**

```
╭─ Without Rune ──────────────────────────────────────╮
│                                                      │
│  Dev: "The payment service is timing out"            │
│                                                      │
│  Agent: (generic debugging approach)                 │
│                                                      │
│  4 hours later: discovers the same issue was         │
│  debugged by another team member last month.         │
│                                                      │
╰──────────────────────────────────────────────────────╯

╭─ With Rune ─────────────────────────────────────────╮
│                                                      │
│  Dev: "The payment service is timing out"            │
│                                                      │
│  Agent: "A similar timeout was investigated last     │
│         month — the root cause was connection pool   │
│         exhaustion under concurrent webhook          │
│         processing. The fix was switching to         │
│         async batch processing with a 50-connection  │
│         pool limit."                                 │
│                                                      │
│  4 hours saved. Knowledge from a teammate's          │
│  debugging session, surfaced automatically.          │
│                                                      │
╰──────────────────────────────────────────────────────╯
```

### The Key Insight

```
╔══════════════════════════════════════════════════════════╗
║                                                          ║
║  Database:  "I know I don't know. Let me search."       ║
║                                                          ║
║  Memory:    "I didn't know I knew. It just surfaced."   ║
║                                                          ║
║  Rune is memory, not a database.                        ║
║                                                          ║
╚══════════════════════════════════════════════════════════╝
```

You don't "query" your long-term memory. You're in a conversation, and relevant experience surfaces — sometimes from events you weren't even present for. That's what Rune does for your agent.

---

## Part 2: The Brain Analogy (Technical Foundation)

### How Your Brain Stores Memories

```
  EXPERIENCE                HIPPOCAMPUS              LONG-TERM MEMORY
  (what happens)            (the filter)             (what persists)
  ━━━━━━━━━━━━             ━━━━━━━━━━━━             ━━━━━━━━━━━━━━━

  30-minute                 Evaluates:               Stores only the
  meeting about    ───────► Is this new?    ───────► GIST:
  database choice           Already know it?
                            Connected to              "PostgreSQL for
  Every word,               what I know?              financial data.
  every tangent,                                      ACID required.
  coffee break,             Filters ~99% out.         MongoDB rejected."
  weather chat...           Keeps the essence.
                                                      Not the meeting.
                                                      Not who spoke.
                                                      The INSIGHT.
```

**Key properties of biological memory filtering:**

| Brain Mechanism | What It Does | Rune Equivalent |
|---|---|---|
| **Novelty detection** | Hippocampus fires more for unfamiliar stimuli | enVector recall → similarity check |
| **Emotional tagging** | Amygdala marks high-impact events | Agent's significance judgment |
| **Gist extraction** | Cortex abstracts away details, keeps meaning | `reusable_insight` generation |
| **Consolidation** | During sleep, replay + filter + store | Capture pipeline |
| **Associative recall** | A cue triggers related memories | Semantic search on enVector |

### The Current Problem: A Pattern-Based Hippocampus

```
  CURRENT PIPELINE (Tier 1: Pattern Matching)

  "We decided to use           200+ handcrafted
   PostgreSQL because    ────► trigger patterns     ────► Match found?
   ACID compliance..."         "We decided to..."          │
                               "Let's go with..."     Yes ─┤─ No ──► LOST
                               "Trade-off is..."           │
                               ...                         ▼
                                                      Tier 2 (LLM)
  Problems:
  ┌─────────────────────────────────────────────────┐
  │ 1. 200+ patterns = maintenance nightmare        │
  │ 2. Quality unmeasurable (no quantitative metric)│
  │ 3. Doesn't improve with use                     │
  │ 4. Language-dependent (en/ko/ja separately)     │
  │ 5. New domains require new patterns             │
  └─────────────────────────────────────────────────┘
```

---

## Part 3: The New Design — Memory-as-Filter

### Core Idea: The Memory IS The Filter

```
  NEW PIPELINE

  Agent (Prefrontal Cortex)          enVector (Hippocampus)
  ━━━━━━━━━━━━━━━━━━━━━━           ━━━━━━━━━━━━━━━━━━━━━

  "This is a significant     ────►  recall(insight, top_k=3)
   decision."
                                    Compare against ALL
  Generates reusable_insight:       existing memories
  "PostgreSQL chosen over
   MongoDB for financial            ┌────────────────────┐
   data. ACID compliance            │ max_similarity:     │
   non-negotiable."                 │                     │
                                    │ < 0.3  → NOVEL     │
                             ◄────  │ 0.3-0.7 → EVOLVE  │
                                    │ > 0.7  → REDUNDANT │
  Result:                           └────────────────────┘
  ┌──────────────────────┐
  │ NOVEL → Store it     │     No patterns.
  │ EVOLVE → Store+Link  │     No maintenance.
  │ REDUNDANT → Skip     │     Self-improving.
  └──────────────────────┘
```

### Why This Works

```
  EMPTY MEMORY (Day 1)            RICH MEMORY (Month 6)
  ━━━━━━━━━━━━━━━━━━━            ━━━━━━━━━━━━━━━━━━━━━

  Everything is novel.            Most things are redundant.
  Captures aggressively.          Captures selectively.

  ┌─────────┐                     ┌─────────┐
  │ ○       │  "PostgreSQL        │ ○○○○○○  │  "PostgreSQL
  │         │   for ACID"         │ ○○○○○○  │   for ACID"
  │         │  → NOVEL (capture)  │ ○○○○○○  │  → REDUNDANT (skip)
  │         │                     │ ○○○○○○  │
  │         │  "Auth middleware   │ ○○○○○○  │  "Read replica
  │         │   rewrite"         │ ○○○○○○  │   for PostgreSQL"
  │         │  → NOVEL (capture)  │ ○○○○○○  │  → EVOLUTION (capture)
  └─────────┘                     └─────────┘

  Like a new employee:            Like a veteran:
  Everything is worth             Only genuinely new
  remembering.                    insights get stored.
```

### Novelty Score: The Quantitative Quality Metric

```
  novelty_score = 1 - max_similarity_to_existing

  ┌──────────────────────────────────────────────────┐
  │                                                    │
  │  1.0 ┤ ████ Completely new domain/topic           │
  │      │                                             │
  │  0.8 ┤ ███ New decision in familiar domain         │
  │      │                                             │
  │  0.6 ┤ ██ Evolution of existing decision           │
  │      │                                             │
  │  0.4 ┤ █ Update to recent decision                 │
  │      │        ─ ─ ─ CAPTURE THRESHOLD ─ ─ ─        │
  │  0.2 ┤ Minor variation of stored knowledge         │
  │      │                                             │
  │  0.0 ┤ Exact duplicate                             │
  │      └─────────────────────────────────────────    │
  │                                                    │
  │  This number is trackable, graphable, tunable.    │
  │  Pattern matching gave us nothing like this.       │
  └──────────────────────────────────────────────────┘
```

---

## Part 4: Reusable Insight — The Shape of Memory

### What Gets Embedded Matters More Than Anything

The quality of semantic search depends entirely on what text was embedded. Currently, Rune embeds a verbose markdown document:

```
  CURRENT payload.text (embedded)
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  # Decision Record: PostgreSQL
  ID: dec_2026-03-26_arch_postgresql
  Status: accepted | Sensitivity: internal│ Domain: architecture
  When/Where: 2026-03-26 | eng-channel

  ## Decision
  Use PostgreSQL for financial data layer

  ## Problem
  Need reliable database for transactions

  ## Alternatives Considered
  - MongoDB
  - PostgreSQL (chosen)

  ## Why (Rationale)
  ACID compliance critical
  Certainty: supported

  ## Trade-offs
  - Less flexible schema

  ## Evidence (Quotes)
  ...
  ## Tags
  database, postgresql


  Problems with embedding this:
  ┌─────────────────────────────────────┐
  │ "##", "Status:", "ID:" pollute      │
  │ the semantic space                  │
  │                                     │
  │ Causal relationships broken         │
  │ across fields                       │
  │                                     │
  │ "Domain: architecture" is           │
  │ identical for ALL arch decisions    │
  │ → dilutes uniqueness                │
  │                                     │
  │ Novelty detection confused by       │
  │ structural markers, not content     │
  └─────────────────────────────────────┘
```

### The Brain Doesn't Store Forms

```
  What the brain stores                What Rune should store
  ━━━━━━━━━━━━━━━━━━━━               ━━━━━━━━━━━━━━━━━━━━━

  NOT:                                NOT:
  ┌──────────────────────┐            # Decision Record: ...
  │ Name: PostgreSQL     │            Status: accepted
  │ Category: Database   │            Domain: architecture
  │ Pro: ACID            │            ## Problem
  │ Con: Rigid schema    │            ...
  └──────────────────────┘            ## Alternatives
                                      ...

  BUT:                                BUT:

  "We chose PostgreSQL               "We chose PostgreSQL
   over MongoDB for the               over MongoDB for the
   financial data layer                financial data layer
   because ACID compliance             because ACID compliance
   is non-negotiable for               is non-negotiable for
   transaction integrity.              transaction integrity.
   Eventual consistency                Eventual consistency
   was rejected as too                 was rejected as too
   risky for financial                 risky for financial
   calculations."                      calculations. Trade-off:
                                       less schema flexibility."

  A NARRATIVE, not a form.            The GIST, not the record.
```

### Dual Representation

```
  ┌─────────────────────────────────────────────────────┐
  │                  DecisionRecord                      │
  │                                                      │
  │  ┌─────────────────────────────────────────────┐    │
  │  │  reusable_insight (NEW)                      │    │
  │  │                                              │    │
  │  │  "We chose PostgreSQL over MongoDB for the   │    │
  │  │   financial data layer because ACID           │    │
  │  │   compliance is non-negotiable. MongoDB's     │    │
  │  │   eventual consistency was rejected. Trade-   │    │
  │  │   off: less schema flexibility."              │    │
  │  │                                              │    │
  │  │  ──────────────────────────────────────────  │    │
  │  │  This gets EMBEDDED in enVector.             │    │
  │  │  Dense. Natural language. Self-contained.    │    │
  │  │  128-512 tokens. No structural markers.      │    │
  │  │  Optimized for semantic search + novelty.    │    │
  │  └─────────────────────────────────────────────┘    │
  │                                                      │
  │  ┌─────────────────────────────────────────────┐    │
  │  │  metadata (existing fields)                  │    │
  │  │                                              │    │
  │  │  title: "PostgreSQL selection"               │    │
  │  │  domain: architecture                        │    │
  │  │  status: accepted                            │    │
  │  │  tags: [database, postgresql]                │    │
  │  │  who: [user:alice, user:bob]                 │    │
  │  │  when: 2026-03-26                            │    │
  │  │                                              │    │
  │  │  ──────────────────────────────────────────  │    │
  │  │  NOT embedded. Stored alongside.             │    │
  │  │  Used for display, filtering, attribution.   │    │
  │  └─────────────────────────────────────────────┘    │
  │                                                      │
  │  ┌─────────────────────────────────────────────┐    │
  │  │  payload.text (existing, role changes)       │    │
  │  │                                              │    │
  │  │  Full markdown rendering for human display.  │    │
  │  │  NOT the embedding target anymore.           │    │
  │  │  Used for render_display_text() only.        │    │
  │  └─────────────────────────────────────────────┘    │
  │                                                      │
  └─────────────────────────────────────────────────────┘
```

### Reusable Insight: Writing Rules

The agent generates `reusable_insight` following these principles:

```
  ╔══════════════════════════════════════════════════════╗
  ║  "If someone on this team in 6 months asks about    ║
  ║   this topic, what's the one paragraph they need?"  ║
  ╚══════════════════════════════════════════════════════╝
```

| Rule | Why |
|---|---|
| One natural-language paragraph | Embedding models perform best on coherent prose |
| 128-512 tokens (sweet spot) | Too short = underspecified, too long = diluted |
| No markdown/structural markers | `##`, `**`, `- ` pollute the semantic space |
| Self-contained | Should make sense without reading metadata |
| Preserve causality | "because", "due to", "despite" — keeps relationships |
| Include rejections | "MongoDB was rejected" embeds differently than omitting it |
| English (for embedding consistency) | Embedding model works best in training language |

### Examples of Good vs Bad Reusable Insights

```
  BAD (too structural):
  "Decision: PostgreSQL. Domain: architecture. Reason: ACID."
  → Embedding sees: generic database keywords

  BAD (too verbose):
  "During our March 26 meeting at 2pm in the eng-general channel,
   Alice proposed using PostgreSQL and Bob agreed because he had
   experience with it at his previous company, and then Carol
   mentioned that MongoDB might be simpler but..."
  → Embedding sees: meeting noise, diluted signal

  BAD (too short):
  "Use PostgreSQL."
  → Embedding sees: almost nothing to match against

  GOOD:
  "We chose PostgreSQL over MongoDB for the financial data layer
   because ACID compliance is non-negotiable for transaction
   integrity. MongoDB's eventual consistency model was rejected
   as it could lead to data inconsistencies in financial
   calculations. Trade-off: less schema flexibility, but
   acceptable given our structured financial data model."
  → Embedding sees: rich semantic content with clear causality
```

```
  BAD:
  "Auth middleware change. Compliance issue. JWT selected."

  GOOD:
  "The auth middleware is being rewritten because legal flagged
   the current session token storage as non-compliant with new
   data regulations. JWT with 24-hour rotation was chosen over
   session-based auth to eliminate server-side token storage
   entirely. The security team requires rotation under 24 hours."
```

```
  BAD:
  "Payment timeout fixed. Connection pool was the problem."

  GOOD:
  "Payment service timeouts were caused by connection pool
   exhaustion under concurrent webhook processing. The connection
   pool had no upper bound, leading to database connection storms
   during traffic spikes. Fix: async batch processing with a
   50-connection pool limit. Monitor pool saturation metric
   going forward."
```

---

## Part 5: Complete Pipeline Flow

### Capture Flow (Agent-Delegated + Memory-as-Filter)

```
  ┌─────────────────────────────────────────────────────────┐
  │                    AI AGENT (Claude/Gemini/Codex)        │
  │                                                          │
  │  Conversation happening...                               │
  │                                                          │
  │  ┌────────────────────────────────────────────────────┐  │
  │  │ SKILL.md instructs the agent:                      │  │
  │  │                                                    │  │
  │  │ "When you observe a significant decision with      │  │
  │  │  rationale, a trade-off analysis, a lesson         │  │
  │  │  learned, or a commitment — capture it."           │  │
  │  │                                                    │  │
  │  │  Agent uses its FULL conversation context          │  │
  │  │  to judge significance. No patterns needed.        │  │
  │  └────────────────────────────────────────────────────┘  │
  │                          │                               │
  │                          ▼                               │
  │  ┌────────────────────────────────────────────────────┐  │
  │  │ Agent generates:                                   │  │
  │  │                                                    │  │
  │  │ {                                                  │  │
  │  │   "tier2": {"capture": true, "domain": "arch"},   │  │
  │  │   "reusable_insight": "We chose PostgreSQL...",    │  │
  │  │   "title": "PostgreSQL selection",                 │  │
  │  │   "confidence": 0.85,                              │  │
  │  │   ...metadata...                                   │  │
  │  │ }                                                  │  │
  │  └────────────────────────────────────────────────────┘  │
  │                          │                               │
  └──────────────────────────┼───────────────────────────────┘
                             │
          capture(text=..., extracted=JSON)
                             │
                             ▼
  ┌─────────────────────────────────────────────────────────┐
  │                    MCP SERVER                            │
  │                                                          │
  │  Step 1: Parse extracted JSON                            │
  │          Extract reusable_insight                         │
  │                                                          │
  │  Step 2: NOVELTY CHECK (new!)                            │
  │  ┌────────────────────────────────────────────────────┐  │
  │  │                                                    │  │
  │  │  recall(reusable_insight, top_k=3)                 │  │
  │  │         │                                          │  │
  │  │         ▼                                          │  │
  │  │  ┌─ similarity < 0.3 ─► NOVEL                     │  │
  │  │  │                       Store new record          │  │
  │  │  │                                                 │  │
  │  │  ├─ similarity 0.3-0.7 ► EVOLUTION                 │  │
  │  │  │                       Store + link to related   │  │
  │  │  │                                                 │  │
  │  │  └─ similarity > 0.7 ─► REDUNDANT                  │  │
  │  │                          Return existing record    │  │
  │  │                          Agent can show it         │  │
  │  └────────────────────────────────────────────────────┘  │
  │                                                          │
  │  Step 3: Build DecisionRecord                            │
  │          Embed reusable_insight (NOT payload.text)        │
  │          Encrypt via FHE                                  │
  │          Store in enVector                                │
  │                                                          │
  │  Step 4: Return result with novelty_score                │
  │                                                          │
  └─────────────────────────────────────────────────────────┘
```

### Recall Flow (Unchanged, but Better)

```
  Agent: "Should we use MongoDB?"
                │
                ▼
  recall("MongoDB for this service")
                │
                ▼
  enVector: semantic search on reusable_insights
                │
                ▼
  Returns: "We chose PostgreSQL over MongoDB for the
            financial data layer because ACID compliance
            is non-negotiable..."
                │
                ▼
  Agent weaves this into response naturally:
  "Your team chose PostgreSQL over MongoDB in March..."
                                     ── from Rune
```

Because `reusable_insight` is dense natural language (not a markdown form), the semantic match between the query "MongoDB for this service" and the stored insight is much stronger.

---

## Part 6: Schema Changes

### DecisionRecord: Add `reusable_insight` Field

```python
class DecisionRecord(BaseModel):
    # ... existing fields ...

    # NEW: The dense natural-language gist — PRIMARY embedding target
    reusable_insight: str = Field(
        default="",
        description=(
            "Dense natural-language paragraph capturing the core knowledge. "
            "This is the PRIMARY text embedded in enVector for semantic search. "
            "128-512 tokens, no markdown, self-contained, causality-preserving."
        ),
    )

    # EXISTING (role change): Now used for display only, NOT embedding
    payload: Payload = Field(default_factory=Payload)
```

### Extracted JSON Schema: `reusable_insight` Required

```json
{
  "tier2": {
    "capture": true,
    "reason": "Significant architecture decision with clear rationale",
    "domain": "architecture"
  },
  "reusable_insight": "We chose PostgreSQL over MongoDB for the financial data layer because ACID compliance is non-negotiable for transaction integrity. MongoDB's eventual consistency model was rejected as it could lead to data inconsistencies in financial calculations. Trade-off: less schema flexibility.",
  "title": "PostgreSQL selection for financial data",
  "rationale": "ACID compliance critical for financial transactions",
  "problem": "Need reliable database for transaction-heavy financial service",
  "alternatives": ["MongoDB", "CockroachDB"],
  "trade_offs": ["Less schema flexibility", "Higher operational complexity"],
  "status_hint": "accepted",
  "tags": ["database", "postgresql", "financial"],
  "confidence": 0.85
}
```

### Capture Response: Add Novelty Metadata

```json
{
  "ok": true,
  "captured": true,
  "record_id": "dec_2026-03-26_architecture_postgresql_selection",
  "summary": "PostgreSQL selection for financial data",
  "domain": "architecture",
  "mode": "agent-delegated",
  "novelty": {
    "score": 0.82,
    "class": "novel",
    "related": [
      {
        "id": "dec_2026-01-15_architecture_database_evaluation",
        "title": "Initial database evaluation",
        "similarity": 0.18
      }
    ]
  }
}
```

```json
{
  "ok": true,
  "captured": false,
  "reason": "Redundant — similar insight already stored",
  "novelty": {
    "score": 0.12,
    "class": "redundant",
    "existing": {
      "id": "dec_2026-03-20_architecture_postgresql_selection",
      "title": "PostgreSQL selection for financial data",
      "similarity": 0.88
    }
  }
}
```

---

## Part 7: Messaging — How To Explain Rune

### The One-Liner

> **"Your agent already knows what your team knows."**

### The Elevator Pitch

> Rune is not a knowledge base you search. It's a shared memory that your AI agent draws from unconsciously — like a senior engineer who's sat in on every team meeting, every architecture review, every debugging session. You don't query it. You just work, and better decisions happen because your agent has the team's experience built in.

### FAQ (For Internal Teams)

**Q: "I can't find what I'm looking for with Rune."**

A: That's like saying "I can't find my memory of breakfast." You don't search your memory — it surfaces when relevant. If your agent is working on a database decision, Rune surfaces past database decisions automatically. If nothing surfaced, either the team hasn't captured relevant knowledge yet, or your current task isn't related enough to trigger recall.

**Q: "Can I search by domain? By date? By author?"**

A: No, and intentionally. Rune uses semantic search because that's how memory works — by meaning, not by category. When you try to remember "that database decision from March," your brain doesn't open a folder labeled "March" — it follows the semantic trail from "database" to the relevant memory. Rune works the same way.

**Q: "How do I know Rune is working?"**

A: The best sign that Rune is working is when your agent gives you context you didn't ask for — and it's useful. If your agent says "your team evaluated this before" or "a similar issue was debugged last month," that's Rune. We're adding subtle attribution so you'll see when team knowledge is influencing your agent's responses.

**Q: "Isn't this just RAG?"**

A: RAG retrieves documents you point it at. Rune captures knowledge your team generates during work and makes it available to every agent on the team — encrypted, so even the cloud server can't read it. It's the difference between a library card (RAG) and actually having read the books (Rune).

**Q: "What if Rune captures something wrong?"**

A: The novelty filter prevents duplicates and low-value captures. And because Rune uses semantic search, a single bad capture doesn't pollute results — it just becomes one memory among many, diluted by the weight of correct knowledge. Over time, frequently-recalled knowledge naturally surfaces more than rarely-used noise.

### The Mental Model Shift

```
  ╔══════════════════════════════════════════════════════════╗
  ║                                                          ║
  ║            STOP thinking:  "Rune is a tool I use"       ║
  ║                                                          ║
  ║           START thinking:  "Rune is experience           ║
  ║                             my agent already has"        ║
  ║                                                          ║
  ╚══════════════════════════════════════════════════════════╝
```

---

## Part 8: Thresholds and Tuning

### Novelty Thresholds (Starting Values)

| Threshold | Value | Meaning |
|---|---|---|
| `NOVEL_THRESHOLD` | 0.3 | Below this similarity = completely new knowledge |
| `EVOLUTION_THRESHOLD` | 0.7 | Above NOVEL, below this = update to existing knowledge |
| `REDUNDANT_THRESHOLD` | 0.7 | Above this = already stored, skip |

These are tunable per deployment. The novelty score provides a quantitative metric for tuning:
- Track `novelty_score` distribution over time
- If too many captures are REDUNDANT → lower threshold
- If too few captures → raise threshold
- Correlate with recall frequency for quality validation

### Cold Start Behavior

```
  Day 1 (0 memories):
  ┌───────────────────────────────────────────┐
  │ Every insight has novelty_score = 1.0     │
  │ Everything gets captured.                  │
  │ This is correct — an empty brain           │
  │ captures aggressively.                     │
  └───────────────────────────────────────────┘

  Week 1 (~50 memories):
  ┌───────────────────────────────────────────┐
  │ Common topics start showing similarity.    │
  │ Second "we use PostgreSQL" gets filtered.  │
  │ Novel topics still captured freely.        │
  └───────────────────────────────────────────┘

  Month 3 (~500 memories):
  ┌───────────────────────────────────────────┐
  │ Memory is selective.                       │
  │ Only genuinely new insights pass.          │
  │ Evolution captures track how decisions     │
  │ change over time.                          │
  └───────────────────────────────────────────┘
```

---

## Part 9: Migration Path

### Embedding Target Change

| Before | After |
|---|---|
| `payload.text` (markdown) embedded | `reusable_insight` (gist) embedded |
| `payload.text` for display + search | `payload.text` for display only |
| No novelty check | enVector recall before every capture |

### Backward Compatibility

1. **Existing records**: Records without `reusable_insight` continue to use `payload.text` for recall
2. **New records**: Use `reusable_insight` as embedding target
3. **Mixed state**: enVector searches across both — old verbose records and new gist records coexist
4. **Optional migration**: Batch job can generate `reusable_insight` for old records using LLM

### Schema Version Bump

```python
schema_version: str = Field(default="2.1")  # was "2.0"
```

Records with version "2.1" have `reusable_insight` as primary embedding target.
Records with version "2.0" fall back to `payload.text`.

---

## Summary

```
  ┌──────────────────────────────────────────────────────┐
  │                                                        │
  │  BEFORE                        AFTER                   │
  │  ━━━━━━                        ━━━━━                   │
  │                                                        │
  │  200+ patterns (manual)    →   0 patterns (memory)     │
  │  Unmeasurable quality      →   novelty_score (0-1)     │
  │  Static filter             →   Self-evolving filter    │
  │  Markdown form embedded    →   Dense gist embedded     │
  │  "Search tool"             →   "Team memory"           │
  │                                                        │
  │  What stayed the same:                                 │
  │  - FHE encryption (zero-knowledge)                     │
  │  - enVector Cloud (encrypted vectors)                  │
  │  - Rune-Vault (secret key isolation)                   │
  │  - Agent-delegated capture                             │
  │  - Semantic recall                                     │
  │                                                        │
  └──────────────────────────────────────────────────────┘
```
