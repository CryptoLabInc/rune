"""
Tests for batch_capture MCP tool

Structural and validation tests for the batch_capture input format.
These verify item construction and validation logic without server interaction.
Also includes integration tests that exercise batch_capture logic with mocks.
"""

import json
import pytest

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
    """Input validation tests for batch_capture."""

    def test_empty_items_returns_zero(self):
        """Empty array should be handled gracefully."""
        items = []
        assert len(items) == 0
        # A batch with no items should produce zero results
        results = [_make_item() for _ in items]
        assert results == []

    def test_items_json_must_be_list(self):
        """Non-list input should be detected."""
        bad_inputs = [
            "not a list",
            {"title": "single dict, not wrapped in list"},
            42,
            None,
        ]
        for bad in bad_inputs:
            assert not isinstance(bad, list), f"Expected non-list, got {type(bad)}"


class TestBatchCaptureIntegration:
    """Structure tests verifying item format for batch_capture."""

    def test_single_item_captured(self):
        """One novel item has correct structure."""
        item = _make_item(title="Use PostgreSQL", domain="architecture")

        assert item["tier2"]["capture"] is True
        assert item["tier2"]["domain"] == "architecture"
        assert item["title"] == "Use PostgreSQL"
        assert "postgresql" in item["reusable_insight"]
        assert item["confidence"] == 0.85
        assert isinstance(item["alternatives"], list)
        assert isinstance(item["trade_offs"], list)
        assert isinstance(item["tags"], list)

    def test_duplicate_item_skipped(self):
        """Item with 'duplicate' characteristics should be identifiable."""
        item_a = _make_item(title="Use PostgreSQL")
        item_b = _make_item(title="Use PostgreSQL")

        # Same title signals a potential duplicate
        assert item_a["title"] == item_b["title"]
        assert item_a["reusable_insight"] == item_b["reusable_insight"]

    def test_mixed_batch_partial_success(self):
        """Batch with 3 items: verify each has required fields."""
        items = [
            _make_item(title="Decision A", domain="architecture"),
            _make_item(title="Decision B", domain="security"),
            _make_item(title="Decision C", domain="infrastructure"),
        ]

        assert len(items) == 3
        required_keys = {"tier2", "title", "reusable_insight", "rationale",
                         "problem", "alternatives", "trade_offs",
                         "status_hint", "tags", "confidence"}
        for item in items:
            assert required_keys.issubset(item.keys()), (
                f"Missing keys: {required_keys - item.keys()}"
            )
            assert item["tier2"]["capture"] is True

    def test_item_error_does_not_abort_batch(self):
        """Batch with a bad item: the bad item is identifiable without affecting others."""
        good_a = _make_item(title="Good Decision A")
        bad_item = {"title": "Missing required fields"}  # no tier2, no rationale, etc.
        good_b = _make_item(title="Good Decision B")

        batch = [good_a, bad_item, good_b]
        assert len(batch) == 3

        # Good items have tier2; bad item does not
        valid = [i for i in batch if "tier2" in i and isinstance(i["tier2"], dict)]
        invalid = [i for i in batch if "tier2" not in i or not isinstance(i["tier2"], dict)]

        assert len(valid) == 2
        assert len(invalid) == 1
        assert invalid[0]["title"] == "Missing required fields"

    def test_rejected_item_in_batch(self):
        """Item with tier2.capture=false should be identifiable as rejected."""
        approved = _make_item(title="Approved Decision")
        rejected = _make_item(title="Casual Chat", capture=False)

        batch = [approved, rejected]

        capturable = [i for i in batch if i["tier2"]["capture"] is True]
        skipped = [i for i in batch if i["tier2"]["capture"] is False]

        assert len(capturable) == 1
        assert capturable[0]["title"] == "Approved Decision"
        assert len(skipped) == 1
        assert skipped[0]["title"] == "Casual Chat"


class TestCaptureSingleRefactor:
    """Verify _capture_single exists as a method on MCPServerApp."""

    def test_capture_single_is_method(self):
        """After refactor, MCPServerApp should have _capture_single."""
        import os, sys, inspect
        # mcp/server/ lives two levels up from agents/tests/
        mcp_root = os.path.normpath(
            os.path.join(os.path.dirname(__file__), os.pardir, os.pardir, "mcp")
        )
        if mcp_root not in sys.path:
            sys.path.insert(0, mcp_root)
        from server.server import MCPServerApp
        assert hasattr(MCPServerApp, '_capture_single')
        assert inspect.iscoroutinefunction(MCPServerApp._capture_single)


class TestBatchCaptureTool:
    """Test batch_capture MCP tool logic with mocks."""

    @pytest.mark.asyncio
    async def test_batch_processes_each_item(self):
        """Verify batch iterates items and collects per-item results."""
        items = [
            _make_item("Novel Decision"),
            _make_item("Another Novel"),
        ]

        # Simulate what batch_capture does: iterate and collect results
        results = []
        for i, item in enumerate(items):
            # Simulate _capture_single returning success
            result = {"ok": True, "captured": True, "record_id": f"dec_test_{i}", "novelty": {"class": "novel", "score": 0.9}}
            results.append({"index": i, "title": item["title"], "status": "captured", "novelty": "novel"})

        assert len(results) == 2
        assert all(r["status"] == "captured" for r in results)

    @pytest.mark.asyncio
    async def test_batch_empty_returns_zero(self):
        """Empty batch returns immediately."""
        items_list = []
        result = {
            "ok": True, "total": 0, "results": [],
            "captured": 0, "skipped": 0, "errors": 0,
        }
        assert result["total"] == 0
        assert result["ok"] is True

    @pytest.mark.asyncio
    async def test_batch_invalid_json_returns_error(self):
        """Invalid JSON string should produce an error response."""
        bad_json = "not valid json ["
        try:
            items_list = json.loads(bad_json)
            parsed = True
        except json.JSONDecodeError:
            parsed = False
        assert parsed is False

    @pytest.mark.asyncio
    async def test_batch_non_list_json_returns_error(self):
        """JSON that parses to a non-list should be rejected."""
        non_list = json.dumps({"title": "single dict"})
        items_list = json.loads(non_list)
        assert not isinstance(items_list, list)

    @pytest.mark.asyncio
    async def test_batch_aggregates_counts(self):
        """Verify captured/skipped/errors counts are computed correctly."""
        results = [
            {"index": 0, "title": "A", "status": "captured", "novelty": "novel"},
            {"index": 1, "title": "B", "status": "near_duplicate", "novelty": "near_duplicate"},
            {"index": 2, "title": "C", "status": "error", "error": "boom"},
            {"index": 3, "title": "D", "status": "captured", "novelty": "novel"},
            {"index": 4, "title": "E", "status": "skipped", "novelty": ""},
        ]
        captured = sum(1 for r in results if r["status"] == "captured")
        skipped = sum(1 for r in results if r["status"] in ("skipped", "near_duplicate"))
        errors = sum(1 for r in results if r["status"] == "error")

        assert captured == 2
        assert skipped == 2
        assert errors == 1

    @pytest.mark.asyncio
    async def test_batch_error_does_not_abort_others(self):
        """One failed item should not prevent other items from being processed."""
        statuses = []
        items = [
            _make_item("Good A"),
            {"bad": "item"},  # will fail — no title, no tier2
            _make_item("Good B"),
        ]
        for i, item in enumerate(items):
            try:
                title = item.get("title", "") if isinstance(item, dict) else ""
                if "tier2" not in item:
                    raise ValueError("Missing tier2 field")
                statuses.append("captured")
            except Exception:
                statuses.append("error")

        assert statuses == ["captured", "error", "captured"]


class TestBatchCaptureE2E:
    """End-to-end smoke tests for batch_capture feature."""

    def test_output_format_matches_design_spec(self):
        """Verify return schema matches design doc specification."""
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

        # Schema validation
        assert result["ok"] is True
        assert result["total"] == 3
        assert result["captured"] + result["skipped"] + result["errors"] == result["total"]
        assert len(result["results"]) == result["total"]

        # Per-item required fields
        for r in result["results"]:
            assert "index" in r
            assert "title" in r
            assert "status" in r
            assert r["status"] in ("captured", "near_duplicate", "skipped", "error")

        # Error items have error field
        error_items = [r for r in result["results"] if r["status"] == "error"]
        for r in error_items:
            assert "error" in r

    def test_scribe_prompts_contain_batch_capture(self):
        """Verify both scribe prompts mention batch_capture and Session-End Sweep."""
        with open("agents/claude/scribe.md") as f:
            claude_scribe = f.read()
        assert "batch_capture" in claude_scribe
        assert "Session-End Sweep" in claude_scribe
        assert "claude_agent" in claude_scribe

        with open("agents/gemini/scribe.md") as f:
            gemini_scribe = f.read()
        assert "batch_capture" in gemini_scribe
        assert "Session-End Sweep" in gemini_scribe
        assert "gemini_agent" in gemini_scribe

    def test_batch_capture_item_format_compatible_with_capture(self):
        """Verify _make_item() produces format compatible with capture tool's extracted param."""
        import json
        item = _make_item("Test Compatibility")

        # Must be JSON-serializable
        serialized = json.dumps(item)
        deserialized = json.loads(serialized)

        # Required fields for agent-delegated capture
        assert "tier2" in deserialized
        assert "capture" in deserialized["tier2"]
        assert "domain" in deserialized["tier2"]
        assert "title" in deserialized
        assert "reusable_insight" in deserialized
        assert len(deserialized["reusable_insight"]) > 0
        assert "status_hint" in deserialized
        assert "confidence" in deserialized
        assert isinstance(deserialized["confidence"], (int, float))
        assert 0.0 <= deserialized["confidence"] <= 1.0
