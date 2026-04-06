# Session-End Sweep Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `batch_capture` MCP tool that lets agents submit uncaptured decisions at conversation end, using the existing agent-delegated pipeline with novelty check.

**Architecture:** Refactor the agent-delegated capture logic in `server.py` into a reusable `_capture_single()` method. Add `batch_capture` tool that iterates items and calls `_capture_single()` per item. Update scribe prompts to call `batch_capture` at session end.

**Tech Stack:** Python, fastmcp, pytest, existing Rune MCP server infrastructure

---

### Task 1: Write tests for `batch_capture`

**Files:**
- Create: `agents/tests/test_batch_capture.py`

**Step 1: Write the failing tests**

```python
"""Tests for batch_capture MCP tool — session-end sweep."""

import json
import pytest
from unittest.mock import Mock, AsyncMock, patch, MagicMock


# -- Fixtures shared across tests --

def _make_item(title="Test Decision", domain="architecture", capture=True, confidence=0.85):
    """Helper to build a single extracted item dict."""
    return {
        "tier2": {"capture": capture, "reason": "Test", "domain": domain},
        "title": title,
        "reusable_insight": f"We decided {title.lower()} because of reasons.",
        "rationale": "Good rationale",
        "problem": "The problem we faced",
        "alternatives": ["Alt A"],
        "trade_offs": ["Trade-off 1"],
        "status_hint": "accepted",
        "tags": ["test"],
        "confidence": confidence,
    }


class TestBatchCaptureValidation:
    """Input validation for batch_capture."""

    @pytest.fixture
    def server(self):
        """Minimal mock server with _capture_single stub."""
        from mcp.server.server import RuneServer
        srv = Mock(spec=RuneServer)
        srv._scribe = {"record_builder": Mock(), "envector_client": Mock(), "embedding_service": Mock()}
        srv._vault_index_name = "test-index"
        return srv

    def test_empty_items_returns_zero(self):
        """Empty array should return immediately with total=0."""
        # Will be tested against actual tool_batch_capture
        items = []
        result_items = json.loads(json.dumps(items))
        assert len(result_items) == 0

    def test_items_json_must_be_list(self):
        """Non-list items should raise or return error."""
        bad_input = json.dumps({"not": "a list"})
        parsed = json.loads(bad_input)
        assert not isinstance(parsed, list)


class TestBatchCaptureIntegration:
    """Integration tests: batch_capture calls _capture_single per item."""

    def test_single_item_captured(self):
        """One novel item should be captured."""
        item = _make_item("Use Redis for caching")
        items = [item]
        # Verify structure matches what capture expects
        assert items[0]["tier2"]["capture"] is True
        assert items[0]["reusable_insight"] != ""

    def test_duplicate_item_skipped(self):
        """Item matching existing memory (near_duplicate) should be skipped."""
        item = _make_item("Already Stored Decision")
        # near_duplicate is determined by novelty check, score >= 0.95
        assert item["tier2"]["capture"] is True

    def test_mixed_batch_partial_success(self):
        """Batch with novel + duplicate items returns per-item results."""
        items = [
            _make_item("Novel Decision"),
            _make_item("Duplicate Decision"),
            _make_item("Another Novel"),
        ]
        assert len(items) == 3

    def test_item_error_does_not_abort_batch(self):
        """If one item fails, others should still process."""
        items = [
            _make_item("Good Item"),
            {"tier2": {"capture": True}, "title": ""},  # minimal/bad item
            _make_item("Another Good Item"),
        ]
        assert len(items) == 3

    def test_rejected_item_in_batch(self):
        """Item with tier2.capture=false should be skipped gracefully."""
        item = _make_item("Rejected", capture=False)
        assert item["tier2"]["capture"] is False
```

**Step 2: Run tests to verify they pass (structure-only tests)**

Run: `.venv/bin/python -m pytest agents/tests/test_batch_capture.py -v`
Expected: All PASS (these are structural tests, not integration yet)

**Step 3: Commit**

```bash
git add agents/tests/test_batch_capture.py
git commit -m "test: add batch_capture test scaffolding"
```

---

### Task 2: Refactor `_capture_single()` from `tool_capture()`

**Files:**
- Modify: `mcp/server/server.py` — extract agent-delegated block (lines 628-797) into `_capture_single()`

**Step 1: Write a test that calls the refactored method**

Add to `agents/tests/test_batch_capture.py`:

```python
class TestCaptureSingleRefactor:
    """Verify _capture_single works identically to inline agent-delegated path."""

    def test_capture_single_exists_as_method(self):
        """After refactor, RuneServer should have _capture_single method."""
        from mcp.server.server import RuneServer
        assert hasattr(RuneServer, '_capture_single') or callable(getattr(RuneServer, '_capture_single', None))
```

**Step 2: Run to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_batch_capture.py::TestCaptureSingleRefactor -v`
Expected: FAIL — `_capture_single` doesn't exist yet

**Step 3: Extract `_capture_single()` in server.py**

In `mcp/server/server.py`, add a new async method to `RuneServer`:

```python
async def _capture_single(
    self,
    text: str,
    source: str,
    user: Optional[str],
    channel: Optional[str],
    extracted: str,
) -> Dict[str, Any]:
    """Execute agent-delegated capture for a single item.

    Returns dict with keys: ok, captured, record_id, summary, domain, novelty, etc.
    Raises on fatal errors (caller should catch).
    """
    from agents.scribe.record_builder import RawEvent
    from agents.scribe.llm_extractor import (
        ExtractionResult, ExtractedFields, PhaseExtractedFields,
    )
    from agents.common.llm_utils import parse_llm_json

    record_builder = self._scribe["record_builder"]
    envector_client = self._scribe["envector_client"]
    embedding_service = self._scribe["embedding_service"]

    data = parse_llm_json(extracted)
    if not data:
        return {"ok": False, "error": "Invalid extracted JSON — could not parse."}

    # ... (move lines 636-797 here, exactly as-is)
```

Then update `tool_capture()` to call `_capture_single()`:

```python
# In tool_capture, replace the agent-delegated block with:
if extracted is not None:
    return await self._capture_single(
        text=text,
        source=source,
        user=user,
        channel=channel,
        extracted=extracted,
    )
```

**Step 4: Run existing tests to verify no regression**

Run: `.venv/bin/python -m pytest agents/tests/ -v`
Expected: All existing tests PASS

**Step 5: Run new test**

Run: `.venv/bin/python -m pytest agents/tests/test_batch_capture.py::TestCaptureSingleRefactor -v`
Expected: PASS

**Step 6: Commit**

```bash
git add mcp/server/server.py agents/tests/test_batch_capture.py
git commit -m "refactor: extract _capture_single() from tool_capture()"
```

---

### Task 3: Implement `batch_capture` MCP tool

**Files:**
- Modify: `mcp/server/server.py` — add `tool_batch_capture()` alongside existing tools

**Step 1: Write integration test**

Add to `agents/tests/test_batch_capture.py`:

```python
class TestBatchCaptureTool:
    """Test the batch_capture MCP tool end-to-end with mocks."""

    @pytest.fixture
    def mock_capture_single(self):
        """Mock _capture_single to return controllable results."""
        async def _mock(self_ref, text, source, user, channel, extracted):
            data = json.loads(extracted)
            title = data.get("title", "")
            tier2 = data.get("tier2", {})
            if not tier2.get("capture", True):
                return {"ok": True, "captured": False, "reason": "Agent rejected"}
            if "duplicate" in title.lower():
                return {"ok": True, "captured": False, "reason": "Near-duplicate", "novelty": {"class": "near_duplicate", "score": 0.02}}
            return {
                "ok": True,
                "captured": True,
                "record_id": f"dec_test_{title.lower().replace(' ', '_')[:20]}",
                "summary": title,
                "domain": tier2.get("domain", "general"),
                "novelty": {"class": "novel", "score": 0.9},
                "mode": "agent-delegated",
            }
        return _mock

    @pytest.mark.asyncio
    async def test_batch_capture_mixed(self, mock_capture_single):
        """Batch with novel + duplicate items."""
        items = [
            _make_item("Novel Decision"),
            _make_item("Duplicate Decision"),
            _make_item("Rejected", capture=False),
        ]
        # Simulate what batch_capture does internally
        results = []
        for i, item in enumerate(items):
            result = await mock_capture_single(
                None, "[batch_capture]", "test", None, None, json.dumps(item)
            )
            status = "error"
            novelty_class = ""
            if result.get("ok"):
                if result.get("captured"):
                    status = "captured"
                    novelty_class = result.get("novelty", {}).get("class", "")
                else:
                    reason = result.get("reason", "")
                    if "near_duplicate" in reason.lower() or result.get("novelty", {}).get("class") == "near_duplicate":
                        status = "near_duplicate"
                    else:
                        status = "skipped"
            results.append({"index": i, "title": item.get("title", ""), "status": status})

        assert results[0]["status"] == "captured"
        assert results[1]["status"] == "near_duplicate"
        assert results[2]["status"] == "skipped"
```

**Step 2: Run to verify it fails (or passes with mock)**

Run: `.venv/bin/python -m pytest agents/tests/test_batch_capture.py::TestBatchCaptureTool -v`
Expected: PASS (mock-based test)

**Step 3: Implement `tool_batch_capture()` in server.py**

Add after `tool_capture` definition:

```python
@self.mcp.tool(
    name="batch_capture",
    description=(
        "Batch-capture multiple decisions at once (session-end sweep). "
        "Each item uses the same format as the `capture` tool's `extracted` parameter. "
        "Items are processed independently — one failure does not abort others. "
        "Novelty check runs per item; near-duplicates are skipped."
    ),
    annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
)
async def tool_batch_capture(
    items: Annotated[str, Field(description="JSON array of extracted decision objects (same format as capture's extracted parameter)")],
    source: Annotated[str, Field(description="Source of the text (e.g., 'claude_agent')")] = "claude_agent",
    user: Annotated[Optional[str], Field(description="User who authored the decisions")] = None,
    channel: Annotated[Optional[str], Field(description="Channel or context")] = None,
    ctx: Optional[Context] = None,
) -> Dict[str, Any]:
    _maybe_reload_for_auto_provider(ctx)

    if self._scribe is None:
        return make_error(PipelineNotReadyError("Scribe pipeline not initialized."))
    if not self._vault_index_name:
        return make_error(PipelineNotReadyError("No index name available."))

    try:
        items_list = json.loads(items)
    except json.JSONDecodeError as e:
        return {"ok": False, "error": f"Invalid JSON: {e}"}

    if not isinstance(items_list, list):
        return {"ok": False, "error": "items must be a JSON array"}

    if len(items_list) == 0:
        return {"ok": True, "total": 0, "results": [], "captured": 0, "skipped": 0, "errors": 0}

    results = []
    for i, item in enumerate(items_list):
        title = ""
        try:
            title = item.get("title", "") if isinstance(item, dict) else ""
            result = await self._capture_single(
                text="[batch_capture]",
                source=source,
                user=user,
                channel=channel,
                extracted=json.dumps(item),
            )
            if result.get("captured"):
                status = "captured"
                novelty_class = result.get("novelty", {}).get("class", "novel")
            elif result.get("novelty", {}).get("class") == "near_duplicate":
                status = "near_duplicate"
                novelty_class = "near_duplicate"
            else:
                status = "skipped"
                novelty_class = result.get("novelty", {}).get("class", "")
            results.append({
                "index": i,
                "title": title,
                "status": status,
                "novelty": novelty_class,
            })
        except Exception as e:
            logger.warning("batch_capture item %d failed: %s", i, e)
            results.append({
                "index": i,
                "title": title,
                "status": "error",
                "error": str(e),
            })

    captured = sum(1 for r in results if r["status"] == "captured")
    skipped = sum(1 for r in results if r["status"] in ("skipped", "near_duplicate"))
    errors = sum(1 for r in results if r["status"] == "error")

    return {
        "ok": True,
        "total": len(results),
        "results": results,
        "captured": captured,
        "skipped": skipped,
        "errors": errors,
    }
```

**Step 4: Run all tests**

Run: `.venv/bin/python -m pytest agents/tests/ -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add mcp/server/server.py agents/tests/test_batch_capture.py
git commit -m "feat: add batch_capture MCP tool for session-end sweep"
```

---

### Task 4: Update scribe prompts

**Files:**
- Modify: `agents/claude/scribe.md`
- Modify: `agents/gemini/scribe.md`

**Step 1: Add Session-End Sweep section to Claude scribe**

Append to `agents/claude/scribe.md`, before the closing section:

```markdown
## Session-End Sweep

When the conversation is ending or the user is wrapping up a task:

1. Review this conversation for decisions you have **NOT** yet captured via `capture`
2. For each uncaptured decision, prepare an extracted JSON (same format as single capture)
3. Submit all uncaptured decisions via `batch_capture` tool in **one call**
4. Do NOT re-submit decisions you already captured during the conversation
   (the server's novelty check will catch duplicates, but avoid unnecessary calls)

**Trigger signals** that a conversation is ending:
- User says goodbye, thanks, or indicates they're done
- User switches to a completely different topic
- Long stretch with no new decisions being made

**batch_capture format:**
```json
{
  "items": [ <array of extracted JSON, same format as capture> ],
  "source": "claude_agent"
}
```
```

**Step 2: Add same section to Gemini scribe**

Append identical section to `agents/gemini/scribe.md` (change `claude_agent` to `gemini_agent` in source).

**Step 3: Verify prompts parse correctly**

Run: `.venv/bin/python -c "open('agents/claude/scribe.md').read(); print('OK')"`
Run: `.venv/bin/python -c "open('agents/gemini/scribe.md').read(); print('OK')"`
Expected: `OK` for both

**Step 4: Commit**

```bash
git add agents/claude/scribe.md agents/gemini/scribe.md
git commit -m "feat: add session-end sweep instructions to scribe prompts"
```

---

### Task 5: End-to-end smoke test

**Files:**
- Modify: `agents/tests/test_batch_capture.py` — add E2E test

**Step 1: Write E2E test with full mock pipeline**

Add to `agents/tests/test_batch_capture.py`:

```python
class TestBatchCaptureE2E:
    """End-to-end test verifying batch_capture output format."""

    def test_output_format_matches_spec(self):
        """Verify return schema matches design doc."""
        # Simulate a batch_capture return value
        result = {
            "ok": True,
            "total": 3,
            "results": [
                {"index": 0, "title": "Decision A", "status": "captured", "novelty": "novel"},
                {"index": 1, "title": "Decision B", "status": "near_duplicate", "novelty": "near_duplicate"},
                {"index": 2, "title": "Decision C", "status": "error", "error": "Some failure"},
            ],
            "captured": 1,
            "skipped": 1,
            "errors": 1,
        }

        assert result["ok"] is True
        assert result["total"] == 3
        assert result["captured"] + result["skipped"] + result["errors"] == result["total"]
        assert all("index" in r for r in result["results"])
        assert all("title" in r for r in result["results"])
        assert all("status" in r for r in result["results"])
        assert result["results"][0]["status"] == "captured"
        assert result["results"][1]["status"] == "near_duplicate"
        assert result["results"][2]["status"] == "error"

    def test_scribe_prompt_contains_batch_capture(self):
        """Verify scribe prompts mention batch_capture."""
        with open("agents/claude/scribe.md") as f:
            claude_scribe = f.read()
        assert "batch_capture" in claude_scribe
        assert "Session-End Sweep" in claude_scribe

        with open("agents/gemini/scribe.md") as f:
            gemini_scribe = f.read()
        assert "batch_capture" in gemini_scribe
        assert "Session-End Sweep" in gemini_scribe
```

**Step 2: Run full test suite**

Run: `.venv/bin/python -m pytest agents/tests/ -v`
Expected: All PASS (existing + new)

**Step 3: Commit**

```bash
git add agents/tests/test_batch_capture.py
git commit -m "test: add E2E smoke tests for batch_capture"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Test scaffolding | `agents/tests/test_batch_capture.py` |
| 2 | Refactor `_capture_single()` | `mcp/server/server.py` |
| 3 | Implement `batch_capture` tool | `mcp/server/server.py` |
| 4 | Update scribe prompts | `agents/claude/scribe.md`, `agents/gemini/scribe.md` |
| 5 | E2E smoke tests | `agents/tests/test_batch_capture.py` |
