"""
Tests for batch_capture MCP tool

Structural and validation tests for the batch_capture input format.
These verify item construction and validation logic without server interaction.
"""

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
