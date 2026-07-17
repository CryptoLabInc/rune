---
description: Search RUNE organizational memory for relevant past decisions and context
argument-hint: <question or topic>
allowed-tools: mcp__plugin_rune_rune__recall
---

# /rune:recall — Search Organizational Memory

Call `mcp__plugin_rune_rune__recall` with `query` set to `$ARGUMENTS` (a
question, topic, or statement — the embedding search handles any form).
Optional: `topk` (default 5), `since` (ISO date).

The tool returns recency-weighted results, each `{ record_id, author, insight,
context, score }`. Synthesize them into a short, direct answer:

- Lead with what organizational memory says, grounded in the `insight` fields;
  draw on `context` for detail.
- Cite sources by `record_id`; attribute to `author` when who decided it matters.
- Empty results → say no relevant records were found and answer from general
  knowledge only, marked as such. If the conversation is producing a decision
  worth saving, suggest `/rune:capture`.
- `ok: false` → relay the error.
