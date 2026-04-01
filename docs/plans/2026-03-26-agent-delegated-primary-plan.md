# Agent-Delegated Primary Refactoring Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restructure the capture pipeline so agent-delegated mode is clearly the primary path, and the standard 3-tier pipeline (Tier1 pattern + Tier2 Haiku + Tier3 extraction) is a clearly-marked legacy fallback.

**Architecture:** The MCP server's `tool_capture` currently has two paths at the same level — agent-delegated (with `extracted` JSON) and standard (3-tier pipeline). This refactoring inverts the priority: agent-delegated becomes the default path documented first in code, and standard pipeline moves into a `_legacy_standard_capture()` helper. The `DetectionResult` dependency in agent-delegated mode is replaced by constructing it from agent-provided metadata.

**Tech Stack:** Python 3.12+, FastMCP, Pydantic

**Branch:** `refactor/agent-delegated-primary`

---

### Task 1: Decouple agent-delegated path from DetectionResult

Currently `server.py:590` calls `detector.detect(text)` even in agent-delegated mode, just for metadata. The agent already provides `domain`, `confidence`, and `tags` — the detector call is redundant.

**Files:**
- Modify: `mcp/server/server.py:571-693`
- Test: `agents/tests/test_agent_delegated.py`

**Step 1: Write a failing test**

In `agents/tests/test_agent_delegated.py`, add a test that proves agent-delegated mode works without a detector:

```python
def test_agent_delegated_without_detector():
    """Agent-delegated mode should not require DecisionDetector."""
    from agents.scribe.detector import DetectionResult
    from agents.scribe.record_builder import RecordBuilder, RawEvent
    from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

    builder = RecordBuilder()
    raw = RawEvent(
        text="We decided to use PostgreSQL over MongoDB",
        user="dev", channel="eng", timestamp="1711000000", source="claude_agent",
    )
    # Construct DetectionResult from agent data, no PatternCache needed
    detection = DetectionResult(
        is_significant=True,
        confidence=0.85,
        domain="architecture",
        category="architecture",
    )
    pre_extraction = ExtractionResult(
        group_title="Use PostgreSQL over MongoDB",
        status_hint="accepted",
        tags=["database", "architecture"],
        confidence=0.85,
        single=ExtractedFields(
            title="Use PostgreSQL over MongoDB",
            rationale="Better ACID compliance for financial data",
            problem="Need reliable database for transactions",
            alternatives=["MongoDB"],
            trade_offs=["Less flexible schema"],
            status_hint="accepted",
            tags=["database"],
        ),
    )
    records = builder.build_phases(raw, detection, pre_extraction=pre_extraction)
    assert len(records) == 1
    assert records[0].domain.value == "architecture"
    assert records[0].quality.scribe_confidence == 0.85
```

**Step 2: Run test to verify it passes (baseline — this should already pass)**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/test_agent_delegated.py::test_agent_delegated_without_detector -v`

**Step 3: Create helper to build DetectionResult from agent JSON**

In `mcp/server/server.py`, add a helper function near the top (after imports):

```python
def _detection_from_agent_data(
    domain: str = "general",
    confidence: float = 0.0,
    category: str = "",
) -> "DetectionResult":
    """Build DetectionResult from agent-provided metadata.

    In agent-delegated mode the calling agent has already evaluated
    significance.  We construct a minimal DetectionResult so that
    RecordBuilder can consume it without running the pattern detector.
    """
    from agents.scribe.detector import DetectionResult
    return DetectionResult(
        is_significant=True,  # Agent said capture=true
        confidence=confidence,
        domain=domain,
        category=category or domain,
    )
```

**Step 4: Replace detector.detect() in agent-delegated path**

In the agent-delegated branch of `tool_capture` (around line 590), replace:

```python
# OLD
detection = detector.detect(text)
if agent_domain and agent_domain != "general":
    from dataclasses import replace as dc_replace
    detection = dc_replace(detection, domain=agent_domain)
```

With:

```python
# NEW — no detector needed in agent-delegated mode
agent_confidence = ...  # (already parsed above)
detection = _detection_from_agent_data(
    domain=agent_domain,
    confidence=float(agent_confidence) if agent_confidence is not None else 0.0,
)
```

**Step 5: Run full agent-delegated test suite**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/test_agent_delegated.py -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add mcp/server/server.py agents/tests/test_agent_delegated.py
git commit -m "refactor: decouple agent-delegated capture from DetectionDetector

Agent-delegated mode no longer calls detector.detect(). Instead,
DetectionResult is constructed from the agent's own metadata (domain,
confidence). This removes the implicit dependency on PatternCache
initialization for agent-delegated captures."
```

---

### Task 2: Extract standard pipeline into a legacy helper

Move the standard 3-tier pipeline code (server.py lines ~695-761) into a clearly-named private method.

**Files:**
- Modify: `mcp/server/server.py`

**Step 1: Extract method `_legacy_standard_capture()`**

Move lines 695-761 (the standard pipeline block starting with `# ===== Standard mode`) into:

```python
async def _legacy_standard_capture(
    self,
    text: str,
    raw_event: "RawEvent",
    detector,
    tier2_filter,
    record_builder,
    envector_client,
    embedding_service,
    redaction_notes: str = None,
) -> Dict[str, Any]:
    """Standard 3-tier capture pipeline (legacy).

    Requires API keys for Tier 2 (LLM filter) and Tier 3 (LLM extraction).
    Retained for backward compatibility with deployments that have
    ANTHROPIC_API_KEY configured and prefer server-side evaluation.

    Most deployments should use agent-delegated mode instead — pass
    the ``extracted`` parameter to let the calling agent handle
    evaluation and extraction.
    """
    # Tier 1: Embedding similarity detection (0 LLM tokens)
    detection = detector.detect(text)
    ...  # (existing code moved here)
```

**Step 2: Update tool_capture to call the extracted method**

After the agent-delegated block, replace inline code with:

```python
# ===== Legacy: Standard 3-tier pipeline (requires API keys) =====
return await self._legacy_standard_capture(
    text=text, raw_event=raw_event,
    detector=detector, tier2_filter=tier2_filter,
    record_builder=record_builder,
    envector_client=envector_client,
    embedding_service=embedding_service,
)
```

**Step 3: Run existing tests to verify no regression**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/ -v --timeout=30`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add mcp/server/server.py
git commit -m "refactor: extract standard pipeline into _legacy_standard_capture()

Moves the 3-tier pipeline (pattern detection → Haiku filter → LLM extraction)
into a clearly-named legacy helper. No behavior change — pure code reorganization
to make agent-delegated the visually primary path in tool_capture()."
```

---

### Task 3: Restructure tool_capture flow with clear documentation

Rewrite the `tool_capture` docstring and flow so agent-delegated is documented first.

**Files:**
- Modify: `mcp/server/server.py` (tool_capture function)

**Step 1: Update tool description**

```python
name="capture",
description=(
    "Capture a significant organizational decision into FHE-encrypted team memory. "
    "PRIMARY: Agent-delegated mode — pass `extracted` JSON with the agent's own "
    "evaluation and extraction. The MCP server stores it without additional LLM calls. "
    "LEGACY: If `extracted` is omitted and API keys are configured, falls back to "
    "a 3-tier server-side pipeline (pattern detection → LLM filter → LLM extraction)."
),
```

**Step 2: Add inline section markers**

```python
async def tool_capture(self, text, source, user, channel, extracted, ctx):
    # ... validation ...

    # ===== PRIMARY: Agent-delegated mode =====
    # The calling agent (Claude/Gemini/Codex) has already evaluated and
    # extracted the decision.  We just validate, build records, and store.
    if extracted is not None:
        ...

    # ===== FALLBACK: Legacy 3-tier pipeline (requires API keys) =====
    # Retained for backward compatibility.  New integrations should use
    # agent-delegated mode above.
    return await self._legacy_standard_capture(...)
```

**Step 3: Run tests**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/ -v --timeout=30`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add mcp/server/server.py
git commit -m "docs: clarify agent-delegated as primary capture path

Update tool_capture description and inline comments to make it clear
that agent-delegated mode is the primary path and standard pipeline
is a legacy fallback."
```

---

### Task 4: Make standard pipeline initialization conditional

Currently `reload_pipelines()` always initializes PatternCache, DetectionDetector, Tier2Filter, and LLMExtractor. Make this conditional — only init when API keys are present.

**Files:**
- Modify: `mcp/server/server.py` (reload_pipelines method, ~lines 1000-1170)

**Step 1: Separate pipeline initialization into two phases**

```python
# Phase 1: Core infrastructure (always needed)
# - EnVectorSDKAdapter
# - EmbeddingAdapter
# - VaultClient
# - RecordBuilder (without LLMExtractor)

# Phase 2: Legacy pipeline components (only if API keys present)
# - PatternCache + DecisionDetector
# - Tier2Filter (if tier2_enabled AND key available)
# - LLMExtractor (if key available)
```

**Step 2: Update self._scribe structure**

The `self._scribe` dict should always be initialized (for RecordBuilder + enVector + embedding), but `detector` and `tier2_filter` become optional:

```python
self._scribe = {
    "record_builder": record_builder,
    "envector_client": envector_client,
    "embedding_service": embedding_service,
    # Legacy pipeline components (None if no API keys)
    "detector": detector,          # None when patterns not loaded
    "tier2_filter": tier2_filter,  # None when no API key
}
```

**Step 3: Update tool_capture legacy fallback guard**

```python
# ===== FALLBACK: Legacy 3-tier pipeline =====
detector = self._scribe.get("detector")
if detector is None:
    return {
        "ok": True,
        "captured": False,
        "reason": "No `extracted` JSON provided and legacy pipeline not available "
                  "(no API keys configured). Use agent-delegated mode by passing "
                  "the `extracted` parameter.",
    }
return await self._legacy_standard_capture(...)
```

**Step 4: Run tests**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/ -v --timeout=30`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add mcp/server/server.py
git commit -m "refactor: make legacy pipeline initialization conditional

PatternCache, DetectionDetector, Tier2Filter, and LLMExtractor are
now only initialized when API keys are present. Core components
(RecordBuilder, enVector, embeddings) are always initialized.
When extracted parameter is omitted and no API keys exist, the
capture tool returns a helpful error directing to agent-delegated mode."
```

---

### Task 5: Update config defaults and documentation

**Files:**
- Modify: `config/config.template.json`
- Modify: `agents/common/config.py`
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `SKILL.md` (capture command section)

**Step 1: Update config.template.json**

Add a comment-friendly field and change defaults:

```json
{
  "scribe": {
    "tier2_enabled": false,
    "tier2_model": "claude-haiku-4-5-20251001",
    "_comment": "tier2_enabled defaults to false. Set true only if you have LLM API keys and prefer server-side evaluation over agent-delegated mode."
  }
}
```

**Step 2: Update config.py default**

Change `tier2_enabled` default from `True` to `False`:

```python
@dataclass
class ScribeConfig:
    tier2_enabled: bool = False  # Legacy: only enable if API keys configured
```

**Step 3: Update README.md capture pipeline description**

Replace the current 3-tier description with agent-delegated as primary:

```markdown
### Capture Pipeline

**Primary (agent-delegated):** The calling AI agent evaluates significance
and extracts structured fields, passing them as JSON to the `capture` MCP tool.
The server stores the encrypted record without additional LLM calls.

**Legacy fallback:** If `ANTHROPIC_API_KEY` is configured and `tier2_enabled` is
true, the server can run its own 3-tier pipeline (embedding similarity →
LLM filter → LLM extraction). This is retained for backward compatibility.
```

**Step 4: Update SKILL.md — capture command**

Ensure the `/rune:capture` documentation emphasizes that agents should provide `extracted` JSON.

**Step 5: Update CLAUDE.md — routing rules**

Ensure the routing table reflects agent-delegated as default.

**Step 6: Commit**

```bash
git add config/config.template.json agents/common/config.py README.md CLAUDE.md SKILL.md
git commit -m "docs: update config defaults and docs for agent-delegated primary

tier2_enabled now defaults to false. Documentation updated to present
agent-delegated mode as the primary capture path across README, CLAUDE.md,
SKILL.md, and config template."
```

---

### Task 6: Clean up module exports

**Files:**
- Modify: `agents/__init__.py`
- Modify: `agents/scribe/__init__.py`

**Step 1: Review and update __init__.py exports**

Keep `DetectionResult` (it's a value class used by RecordBuilder), but remove `parse_capture_triggers` from top-level exports since it's a legacy detail:

```python
# agents/scribe/__init__.py
from .detector import DecisionDetector, DetectionResult
from .record_builder import RecordBuilder, RawEvent

# Legacy (available but not promoted)
# from .tier2_filter import Tier2Filter
# from .llm_extractor import LLMExtractor
# from .pattern_parser import parse_capture_triggers
```

**Step 2: Run import smoke test**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -c "from agents.scribe import RecordBuilder, DetectionResult, RawEvent; print('OK')"`
Expected: `OK`

**Step 3: Run full test suite**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/ -v --timeout=30`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add agents/__init__.py agents/scribe/__init__.py
git commit -m "refactor: clean up module exports

DetectionResult and RecordBuilder remain primary exports. Legacy
pipeline components (Tier2Filter, LLMExtractor, pattern_parser)
are still importable but removed from top-level __init__ exports."
```

---

### Task 7: Verify benchmarks still work

**Files:**
- Check: `benchmark/runners/scribe_bench.py`

**Step 1: Verify benchmarks use agent-delegated mode**

The scribe benchmark should already test agent-delegated mode (confirmed in exploration). Run a dry check:

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -c "from benchmark.runners import scribe_bench; print('benchmark importable')"`

**Step 2: Run benchmark smoke test (if infrastructure available)**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python benchmark/runners/scribe_bench.py --dry-run 2>&1 | head -20`

**Step 3: Commit (if any benchmark adjustments needed)**

---

### Task 8: Final integration test

**Step 1: Run all tests**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python -m pytest agents/tests/ -v --timeout=60`
Expected: ALL PASS

**Step 2: Verify MCP server starts**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && timeout 5 python -c "from mcp.server.server import RuneMCPServer; print('Server class loads OK')" 2>&1`
Expected: `Server class loads OK`

**Step 3: Review diff**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && git diff main --stat`

Verify:
- No files deleted (backward compat preserved)
- Changes focused on server.py, config, docs, exports
- Standard pipeline code still present, just reorganized

---

## Summary of Changes

| What | Before | After |
|------|--------|-------|
| `tool_capture` primary path | Standard 3-tier | Agent-delegated |
| Agent-delegated needs `detector` | Yes (line 590) | No |
| Standard pipeline location | Inline in tool_capture | `_legacy_standard_capture()` |
| `tier2_enabled` default | `True` | `False` |
| Pipeline init without API keys | Fails silently | Core only, legacy skipped |
| Documentation emphasis | 3-tier pipeline | Agent-delegated mode |

**Not changed (backward compat):**
- All standard pipeline code remains (detector, tier2_filter, llm_extractor, patterns)
- Config fields preserved (tier2_enabled, tier2_model, similarity_threshold)
- Tests for standard pipeline still run
- `agents/scribe/server.py` (webhook server) untouched
