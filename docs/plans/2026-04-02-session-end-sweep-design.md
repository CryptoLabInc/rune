# Session-End Sweep: Passive Capture Design

**Date:** 2026-04-02
**Status:** Approved
**Author:** Sunchul Jung + Claude

## Problem

Agent-delegated capture is the primary path for Rune, but it depends on the scribe agent proactively calling `capture` during the conversation. If the agent forgets, skips, or deprioritizes capture (e.g., during complex multi-step tasks), decisions are lost — exactly the "context evaporation" problem Rune exists to solve.

Inspired by cczz's background extraction pattern (forked agent runs after every query turn), we need a safety net that catches decisions the agent didn't capture in real-time.

## Design: Session-End Sweep

At conversation end (or when the user wraps up a task), the agent reviews the session for **uncaptured decisions** and submits them in bulk via a new `batch_capture` MCP tool.

### Key Principle

The agent does the thinking (structuring, evaluating significance). The MCP server does the filtering (novelty check via enVector). No additional API keys or LLM calls required on the server side.

## New MCP Tool: `batch_capture`

### Interface

```
Tool: batch_capture
Arguments:
  items: str       — JSON array, each element matches existing `capture` extracted format
  source: str      — e.g., "claude_agent", "gemini_agent"
  user: str?       — username if available
  channel: str?    — context location

Returns: JSON object with per-item results
```

### Example Input

```json
{
  "items": [
    {
      "tier2": {"capture": true, "reason": "Architecture decision", "domain": "architecture"},
      "title": "Use Redis for session caching",
      "reusable_insight": "We chose Redis over Memcached for session caching because...",
      "rationale": "...",
      "alternatives": ["Memcached"],
      "trade_offs": ["Higher memory usage"],
      "status_hint": "accepted",
      "tags": ["redis", "caching"],
      "confidence": 0.8
    },
    {
      "tier2": {"capture": true, "reason": "Debugging insight", "domain": "debugging"},
      "title": "Connection pool exhaustion root cause",
      "reusable_insight": "The intermittent 503 errors were caused by...",
      "rationale": "...",
      "status_hint": "accepted",
      "tags": ["debugging", "connection-pool"],
      "confidence": 0.9
    }
  ],
  "source": "claude_agent",
  "user": "sunchul"
}
```

### Example Output

```json
{
  "total": 2,
  "results": [
    {"index": 0, "title": "Use Redis for session caching", "status": "captured", "novelty": "novel"},
    {"index": 1, "title": "Connection pool exhaustion root cause", "status": "near_duplicate", "novelty": "near_duplicate"}
  ],
  "captured": 1,
  "skipped": 1,
  "errors": 0
}
```

### Server Implementation

```python
async def tool_batch_capture(items_json, source, user, channel):
    items = json.loads(items_json)
    results = []
    for i, item in enumerate(items):
        try:
            # Reuse existing agent-delegated capture path
            result = await _capture_single(
                text="[batch_capture]",
                source=source,
                user=user,
                channel=channel,
                extracted=json.dumps(item),
            )
            results.append({"index": i, "title": item.get("title", ""), "status": result.status, "novelty": result.novelty_class})
        except Exception as e:
            results.append({"index": i, "title": item.get("title", ""), "status": "error", "error": str(e)})

    captured = sum(1 for r in results if r["status"] == "captured")
    skipped = sum(1 for r in results if r["status"] == "near_duplicate")
    errors = sum(1 for r in results if r["status"] == "error")

    return {"total": len(items), "results": results, "captured": captured, "skipped": skipped, "errors": errors}
```

Key: `_capture_single` is a refactored extract of the existing agent-delegated path in `tool_capture()`, made callable independently.

### Error Handling

- Individual item failure does not abort the batch (partial success)
- Each result includes status: `captured` / `near_duplicate` / `error`
- Empty items array returns immediately with `{"total": 0, ...}`
- Malformed items return per-item `error` with message

## Scribe Prompt Changes

Add to both `agents/claude/scribe.md` and `agents/gemini/scribe.md`:

```markdown
## Session-End Sweep

When the conversation is ending or the user is wrapping up a task:

1. Review this conversation for decisions you have NOT yet captured
2. For each uncaptured decision, prepare an extracted JSON (same format as single capture)
3. Submit all uncaptured decisions via `batch_capture` tool in one call
4. Do NOT re-submit decisions you already captured during the conversation
   (the server's novelty check will catch duplicates, but avoid unnecessary calls)

Trigger signals that a conversation is ending:
- User says goodbye, thanks, or indicates they're done
- User switches to a completely different topic
- Extended period with no new decisions being made
```

## Data Flow

```
During conversation (existing):
  Real-time capture ──► capture tool ──► novelty check ──► store

At conversation end (new):
  Agent identifies uncaptured decisions
       │
       ▼
  batch_capture tool call
  (array of extracted JSON)
       │
       ▼
  MCP server: iterate items
       │
       ├── Item 1 ──► novelty check ──► novel → store
       ├── Item 2 ──► novelty check ──► near_duplicate → skip
       └── Item 3 ──► novelty check ──► evolution → store (annotated)
       │
       ▼
  Return per-item summary
```

## Scope of Changes

| Component | Change |
|---|---|
| `mcp/server/server.py` | Add `tool_batch_capture()`, refactor capture into `_capture_single()` |
| `agents/claude/scribe.md` | Add Session-End Sweep section |
| `agents/gemini/scribe.md` | Add Session-End Sweep section |
| Tests | `test_batch_capture.py` — normal/duplicate/mixed/empty/error cases |

## Out of Scope (YAGNI)

- Heartbeat / periodic capture — extend later via prompt addition if needed
- Server-side "what was already captured" query — `capture_history` tool already exists
- Custom novelty threshold for batch — use existing 0.95
- Rate limiting on batch size — trust agent judgment for now
- Retry logic for failed items — caller can re-submit

## Relationship to Existing Architecture

- **Agent-delegated capture**: Unchanged. Still the primary real-time path.
- **batch_capture**: Safety net. Uses the exact same pipeline (novelty check, record building, enVector insert).
- **Legacy 3-tier pipeline**: Not involved. batch_capture requires agent-structured `extracted` JSON.
- **Memory-as-Filter**: Fully active. Each batch item independently checked for novelty.

## Success Criteria

1. `batch_capture` processes N items and returns per-item results
2. Near-duplicate items (already captured in real-time) are correctly skipped
3. Novel items are stored with correct novelty annotation
4. Partial failures don't block other items
5. Scribe agents call `batch_capture` at conversation end when uncaptured decisions exist
6. No regression in existing `capture` tool behavior
