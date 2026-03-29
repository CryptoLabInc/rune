# Memory-as-Filter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the Memory-as-Filter architecture: `reusable_insight` as primary embedding target + enVector novelty check before capture.

**Architecture:** Agent generates a dense NL gist (`reusable_insight`). Before storing, the MCP server runs `envector_client.search_with_text(reusable_insight)` to check novelty. Only NOVEL or EVOLUTION captures pass. The embedding target changes from verbose `payload.text` to `reusable_insight`. Schema bumps to 2.1.

**Tech Stack:** Python 3.12+, FastMCP, Pydantic, pyenvector, fastembed

**Branch:** `refactor/agent-delegated-primary` (continues existing work)

**Design doc:** `docs/plans/2026-03-26-memory-as-filter-design.md`

**Test runner:** `/Users/sunchuljung/repo/cryptolab/Rune/rune/.venv/bin/python -m pytest`

---

### Task 1: Add `reusable_insight` field to DecisionRecord schema

**Files:**
- Modify: `agents/common/schemas/decision_record.py:166-202`
- Test: `agents/tests/test_schemas.py`

**Step 1: Write the failing test**

Add to `agents/tests/test_schemas.py`:

```python
def test_reusable_insight_field():
    """DecisionRecord should have reusable_insight field with schema 2.1."""
    from agents.common.schemas import DecisionRecord, DecisionDetail, Domain

    record = DecisionRecord(
        id="dec_2026-03-28_architecture_test",
        title="Test",
        decision=DecisionDetail(what="Test decision"),
        reusable_insight="We chose PostgreSQL over MongoDB because ACID compliance is critical.",
    )
    assert record.reusable_insight == "We chose PostgreSQL over MongoDB because ACID compliance is critical."
    assert record.schema_version == "2.1"


def test_reusable_insight_defaults_empty():
    """reusable_insight should default to empty string for backward compat."""
    from agents.common.schemas import DecisionRecord, DecisionDetail

    record = DecisionRecord(
        id="dec_2026-03-28_architecture_test",
        title="Test",
        decision=DecisionDetail(what="Test decision"),
    )
    assert record.reusable_insight == ""
```

**Step 2: Run test to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_schemas.py::test_reusable_insight_field -v`
Expected: FAIL (no `reusable_insight` attribute)

**Step 3: Add field to DecisionRecord**

In `agents/common/schemas/decision_record.py`, add after line 199 (`group_summary` field):

```python
    # PRIMARY embedding target (schema 2.1+)
    reusable_insight: str = Field(
        default="",
        description=(
            "Dense natural-language paragraph capturing the core knowledge. "
            "PRIMARY text embedded in enVector for semantic search. "
            "128-512 tokens, no markdown, self-contained, causality-preserving."
        ),
    )
```

And change schema_version default on line 172:

```python
    schema_version: str = Field(default="2.1")
```

**Step 4: Run test to verify it passes**

Run: `.venv/bin/python -m pytest agents/tests/test_schemas.py::test_reusable_insight_field agents/tests/test_schemas.py::test_reusable_insight_defaults_empty -v`
Expected: PASS

**Step 5: Commit**

```bash
git add agents/common/schemas/decision_record.py agents/tests/test_schemas.py
git commit -m "feat: add reusable_insight field to DecisionRecord (schema 2.1)

New field for dense natural-language gist that becomes the primary
embedding target. Defaults to empty string for backward compat
with schema 2.0 records."
```

---

### Task 2: Switch embedding target from `payload.text` to `reusable_insight`

The key line in `server.py:688` is:
```python
texts = [r.payload.text for r in records]
```
This is what gets embedded in enVector. Change it to use `reusable_insight` when available.

**Files:**
- Modify: `mcp/server/server.py:685-695`
- Test: `agents/tests/test_agent_delegated.py`

**Step 1: Write the failing test**

Add to `agents/tests/test_agent_delegated.py`:

```python
def test_reusable_insight_used_for_embedding_text():
    """When reusable_insight is set, it should be the embedding target."""
    from agents.common.schemas import DecisionRecord, DecisionDetail, Payload

    record = DecisionRecord(
        id="dec_test",
        title="Test",
        decision=DecisionDetail(what="Test"),
        reusable_insight="Dense gist paragraph for embedding.",
        payload=Payload(text="# Full markdown\n## Decision\nVerbose content"),
    )
    # The function that selects embedding text
    from mcp.server.server import _embedding_text_for_record
    assert _embedding_text_for_record(record) == "Dense gist paragraph for embedding."


def test_embedding_text_fallback_to_payload():
    """When reusable_insight is empty, fall back to payload.text."""
    from agents.common.schemas import DecisionRecord, DecisionDetail, Payload

    record = DecisionRecord(
        id="dec_test",
        title="Test",
        decision=DecisionDetail(what="Test"),
        reusable_insight="",
        payload=Payload(text="Fallback payload text"),
    )
    from mcp.server.server import _embedding_text_for_record
    assert _embedding_text_for_record(record) == "Fallback payload text"
```

**Step 2: Run test to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_agent_delegated.py::test_reusable_insight_used_for_embedding_text -v`
Expected: FAIL (no `_embedding_text_for_record` function)

**Step 3: Add helper and update capture flow**

In `mcp/server/server.py`, add helper near the top (after `_detection_from_agent_data`):

```python
def _embedding_text_for_record(record) -> str:
    """Select the text to embed in enVector.

    Schema 2.1+: use reusable_insight (dense NL gist).
    Schema 2.0 fallback: use payload.text (verbose markdown).
    """
    insight = getattr(record, "reusable_insight", "")
    if insight and insight.strip():
        return insight.strip()
    return record.payload.text
```

Then in `tool_capture`'s agent-delegated block, replace (around line 688):

```python
                    # OLD
                    texts = [r.payload.text for r in records]
```

With:

```python
                    # Embed reusable_insight (schema 2.1) or payload.text (fallback)
                    texts = [_embedding_text_for_record(r) for r in records]
```

**Step 4: Run tests**

Run: `.venv/bin/python -m pytest agents/tests/test_agent_delegated.py -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add mcp/server/server.py agents/tests/test_agent_delegated.py
git commit -m "feat: switch embedding target to reusable_insight

New _embedding_text_for_record() selects reusable_insight when
available (schema 2.1+), falls back to payload.text for older
records. This embeds dense NL gist instead of verbose markdown."
```

---

### Task 3: Populate `reusable_insight` from agent-delegated JSON

The agent already sends `reusable_insight` in the extracted JSON. Wire it through to the DecisionRecord.

**Files:**
- Modify: `mcp/server/server.py:622-676` (agent-delegated extraction block)
- Modify: `agents/scribe/record_builder.py:203-360` (build_phases)
- Test: `agents/tests/test_agent_delegated.py`

**Step 1: Write the failing test**

Add to `agents/tests/test_agent_delegated.py`:

```python
def test_reusable_insight_flows_to_record():
    """reusable_insight from agent JSON should appear on the built record."""
    from agents.scribe.detector import DetectionResult
    from agents.scribe.record_builder import RecordBuilder, RawEvent
    from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

    insight = "We chose PostgreSQL over MongoDB for ACID compliance in financial data."
    builder = RecordBuilder()
    raw = RawEvent(text="...", user="dev", channel="eng", timestamp="1711000000", source="claude_agent")
    detection = DetectionResult(is_significant=True, confidence=0.85, domain="architecture")
    pre_extraction = ExtractionResult(
        group_title="PostgreSQL selection",
        status_hint="accepted",
        tags=["database"],
        confidence=0.85,
        group_summary=insight,
        single=ExtractedFields(
            title="PostgreSQL selection",
            rationale="ACID compliance",
            status_hint="accepted",
            tags=["database"],
        ),
    )
    records = builder.build_phases(raw, detection, pre_extraction=pre_extraction)
    assert records[0].reusable_insight == insight
```

**Step 2: Run test to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_agent_delegated.py::test_reusable_insight_flows_to_record -v`
Expected: FAIL (reusable_insight is empty)

**Step 3: Wire reusable_insight in RecordBuilder.build_phases()**

In `agents/scribe/record_builder.py`, in `build_phases()`, after line 283 (`record.payload.text = render_payload_text(record)`), add:

```python
            # Populate reusable_insight from pre_extraction group_summary
            if pre_extraction and getattr(pre_extraction, 'group_summary', None):
                record.reusable_insight = pre_extraction.group_summary
```

Do the same in the multi-record loop (after line 356, `record.payload.text = render_payload_text(record)`):

```python
                # Populate reusable_insight from pre_extraction group_summary
                if getattr(extraction, 'group_summary', None):
                    record.reusable_insight = extraction.group_summary
```

**Step 4: Run tests**

Run: `.venv/bin/python -m pytest agents/tests/test_agent_delegated.py -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add agents/scribe/record_builder.py agents/tests/test_agent_delegated.py
git commit -m "feat: wire reusable_insight from agent JSON through to DecisionRecord

RecordBuilder.build_phases() now populates reusable_insight from
the pre_extraction group_summary field (which is set from the
agent's reusable_insight JSON field in server.py)."
```

---

### Task 4: Implement novelty check in capture flow

The core Memory-as-Filter logic: before storing, run `search_with_text(reusable_insight)` against existing memory.

**Files:**
- Modify: `mcp/server/server.py:685-715` (after build_phases, before insert)
- Test: `agents/tests/test_novelty_check.py` (new)

**Step 1: Write the failing test**

Create `agents/tests/test_novelty_check.py`:

```python
"""Tests for novelty check logic."""
import pytest


def test_classify_novel():
    """Score below NOVEL_THRESHOLD = novel."""
    from mcp.server.server import _classify_novelty
    result = _classify_novelty(max_similarity=0.2, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "novel"
    assert result["score"] == pytest.approx(0.8)  # 1 - 0.2


def test_classify_evolution():
    """Score between thresholds = evolution."""
    from mcp.server.server import _classify_novelty
    result = _classify_novelty(max_similarity=0.5, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "evolution"
    assert result["score"] == pytest.approx(0.5)


def test_classify_redundant():
    """Score above REDUNDANT_THRESHOLD = redundant."""
    from mcp.server.server import _classify_novelty
    result = _classify_novelty(max_similarity=0.85, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "redundant"
    assert result["score"] == pytest.approx(0.15)


def test_classify_empty_memory():
    """No existing records = max novelty."""
    from mcp.server.server import _classify_novelty
    result = _classify_novelty(max_similarity=0.0, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "novel"
    assert result["score"] == pytest.approx(1.0)
```

**Step 2: Run test to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_novelty_check.py -v`
Expected: FAIL (no `_classify_novelty`)

**Step 3: Implement novelty classification**

In `mcp/server/server.py`, add helper near top:

```python
# Novelty thresholds (Memory-as-Filter)
NOVELTY_THRESHOLD_NOVEL = 0.3
NOVELTY_THRESHOLD_REDUNDANT = 0.7


def _classify_novelty(
    max_similarity: float,
    threshold_novel: float = NOVELTY_THRESHOLD_NOVEL,
    threshold_redundant: float = NOVELTY_THRESHOLD_REDUNDANT,
) -> dict:
    """Classify capture novelty based on similarity to existing memory.

    Returns dict with 'score' (0-1, higher=more novel) and 'class'.
    """
    novelty_score = 1.0 - max_similarity
    if max_similarity < threshold_novel:
        return {"class": "novel", "score": round(novelty_score, 4)}
    elif max_similarity >= threshold_redundant:
        return {"class": "redundant", "score": round(novelty_score, 4)}
    else:
        return {"class": "evolution", "score": round(novelty_score, 4)}
```

**Step 4: Run test to verify it passes**

Run: `.venv/bin/python -m pytest agents/tests/test_novelty_check.py -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add mcp/server/server.py agents/tests/test_novelty_check.py
git commit -m "feat: add novelty classification logic for Memory-as-Filter

_classify_novelty() maps similarity scores to novel/evolution/redundant
classes using configurable thresholds (0.3/0.7). Returns novelty_score
(1 - max_similarity) as quantitative metric."
```

---

### Task 5: Integrate novelty check into capture flow

Wire the novelty check into the agent-delegated capture path in `tool_capture`.

**Files:**
- Modify: `mcp/server/server.py:685-715`

**Step 1: Add novelty check between build_phases and insert**

In `tool_capture`, after `records = record_builder.build_phases(...)` (line 685) and before the insert call (line 690), add:

```python
                    # ===== Novelty check (Memory-as-Filter) =====
                    # Compare reusable_insight against existing memory
                    embedding_text = _embedding_text_for_record(records[0])
                    novelty_info = {"score": 1.0, "class": "novel", "related": []}

                    try:
                        search_result = envector_client.search_with_text(
                            index_name=self._vault_index_name,
                            query_text=embedding_text,
                            embedding_service=embedding_service,
                            topk=3,
                        )
                        parsed = envector_client.parse_search_results(search_result)
                        if parsed:
                            max_sim = max(r.get("score", 0.0) for r in parsed)
                            novelty_info = _classify_novelty(max_sim)
                            novelty_info["related"] = [
                                {
                                    "id": r.get("metadata", {}).get("id", ""),
                                    "title": r.get("metadata", {}).get("title", ""),
                                    "similarity": round(r.get("score", 0.0), 3),
                                }
                                for r in parsed[:3]
                            ]

                            # REDUNDANT → skip capture
                            if novelty_info["class"] == "redundant":
                                best = parsed[0]
                                return {
                                    "ok": True,
                                    "captured": False,
                                    "reason": "Redundant — similar insight already stored",
                                    "novelty": novelty_info,
                                }
                    except Exception as e:
                        # Novelty check failure is non-fatal — proceed with capture
                        logger.warning("Novelty check failed: %s", e)
```

**Step 2: Add novelty metadata to success response**

Update the success response (around line 701) to include novelty info:

```python
                    first = records[0]
                    result = {
                        "ok": True,
                        "captured": True,
                        "record_id": first.id,
                        "summary": first.title,
                        "domain": first.domain.value,
                        "certainty": first.why.certainty.value,
                        "mode": "agent-delegated",
                        "novelty": novelty_info,
                    }
```

**Step 3: Run all tests**

Run: `.venv/bin/python -m pytest agents/tests/ -v`
Expected: ALL PASS (novelty check uses live enVector, but tests use mocks so no network calls)

**Step 4: Commit**

```bash
git add mcp/server/server.py
git commit -m "feat: integrate novelty check into capture flow

Before storing, capture now runs search_with_text() against existing
memory to compute novelty score. REDUNDANT captures (similarity > 0.7)
are skipped with a helpful message. Novelty metadata included in all
capture responses. Failure is non-fatal — captures proceed if check fails."
```

---

### Task 6: Update scribe.md prompt — make reusable_insight mandatory for all captures

Currently `reusable_insight` is only documented for code-context captures. Expand to all.

**Files:**
- Modify: `agents/claude/scribe.md`
- Modify: `agents/gemini/scribe.md` (if exists)

**Step 1: Update Format A (Single Decision) in scribe.md**

Find the Format A section and ensure `reusable_insight` is listed as a required field:

```markdown
"reusable_insight": "Dense natural-language paragraph (128-512 tokens) capturing the core knowledge. No markdown. Self-contained. Must answer: 'If someone in 6 months asks about this topic, what do they need to know?' Include what was decided, why, what was rejected, and key trade-offs."
```

**Step 2: Update Format B (Phase Chain) and Format C (Bundle)**

Ensure `reusable_insight` appears at the top level of all formats.

**Step 3: Remove the code-context-only restriction**

Remove or update the note that says "Include these fields when capturing from coding/debugging/optimization context. Omit for non-code decisions."

**Step 4: Commit**

```bash
git add agents/claude/scribe.md agents/gemini/scribe.md
git commit -m "docs: make reusable_insight mandatory for all capture formats

Previously reusable_insight was only for code-context captures.
Now required for all formats (A/B/C) as it becomes the primary
embedding target in the Memory-as-Filter architecture."
```

---

### Task 7: Update templates.py — render_payload_text docstring

Update the docstring to clarify payload.text is now display-only, not the embedding target.

**Files:**
- Modify: `agents/common/schemas/templates.py:138-148`

**Step 1: Update docstring**

```python
def render_payload_text(record: "DecisionRecord") -> str:
    """
    Render a DecisionRecord to payload.text (Markdown).

    This text is used for:
    1. Memory reproduction (human-readable full context)
    2. Display in recall results

    NOTE (schema 2.1+): Embedding generation now uses
    record.reusable_insight instead of this text.
    For schema 2.0 records without reusable_insight,
    this text is still used as embedding fallback.
    """
```

**Step 2: Commit**

```bash
git add agents/common/schemas/templates.py
git commit -m "docs: clarify payload.text is display-only in schema 2.1+

render_payload_text() docstring updated to reflect that embedding
now uses reusable_insight. payload.text remains for human display
and backward compat with schema 2.0."
```

---

### Task 8: Final integration test

**Step 1: Run all tests**

Run: `.venv/bin/python -m pytest agents/tests/ -v`
Expected: ALL PASS (excluding pre-existing failures in test_tier2_filter.py and test_retriever.py)

**Step 2: Verify MCP server loads**

Run: `.venv/bin/python -c "from mcp.server.server import RuneMCPServer, _classify_novelty, _embedding_text_for_record; print('All imports OK')"`
Expected: `All imports OK`

**Step 3: Review diff**

Run: `git log --oneline refactor/agent-delegated-primary --not main`

**Step 4: Verify schema version**

Run: `.venv/bin/python -c "from agents.common.schemas import DecisionRecord, DecisionDetail; r = DecisionRecord(id='test', title='t', decision=DecisionDetail(what='w')); print(f'schema={r.schema_version} insight_field={hasattr(r, \"reusable_insight\")}')"`
Expected: `schema=2.1 insight_field=True`

---

## Summary of Changes

| What | Before | After |
|------|--------|-------|
| Schema version | 2.0 | 2.1 |
| Embedding target | `payload.text` (markdown) | `reusable_insight` (NL gist) |
| `reusable_insight` field | Not on schema | Required field, default="" |
| Novelty check | None | `search_with_text()` before every capture |
| REDUNDANT captures | Stored | Skipped with explanation |
| Capture response | No novelty info | `novelty: {score, class, related}` |
| scribe.md prompt | `reusable_insight` code-only | All formats |
| `payload.text` role | Embed + display | Display only (schema 2.1+) |

**Not changed:**
- Vault — no modifications
- enVector index — single index, no firmware
- Legacy pipeline — untouched
- Existing schema 2.0 records — fall back to payload.text for embedding
