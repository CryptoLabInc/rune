---
description: Capture a decision, insight, or context into RUNE organizational memory
argument-hint: <what to remember>
allowed-tools: mcp__plugin_rune_rune__capture
---

# /rune:capture — Save to Organizational Memory

Compose the memory yourself from the current work and call
`mcp__plugin_rune_rune__capture`. The server timestamps it, attributes it to you
(from your Console identity), embeds the insight, and seals it.

Pass two fields:

- `insight` — the concise, self-contained essence someone should find when
  searching later: what was decided or learned, and why, in a few sentences.
  This is what gets embedded and searched, so write it to stand alone (no "as
  discussed above").
- `context` — the fuller background: the problem, alternatives, trade-offs, and
  specifics. Stored and returned on recall but not searched. Optional, but
  usually worth including.

Use `$ARGUMENTS` as the subject when given; otherwise capture the most
significant decision or insight from the current conversation.

Result handling:
- `captured: true` → confirm briefly, citing `record_id`.
- `captured: false` → it was a near-duplicate of `novelty.related`; say so and
  do not retry.
- `ok: false` → relay the error.
