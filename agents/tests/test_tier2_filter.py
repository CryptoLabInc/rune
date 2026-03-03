"""
Tests for Tier 2 LLM Filter

Tests the Haiku-based policy evaluator that filters Tier 1 candidates.
"""

import json
import pytest
from unittest.mock import Mock, MagicMock, patch


class TestFilterResult:
    """Tests for FilterResult dataclass"""

    def test_capture_result(self):
        from agents.scribe.tier2_filter import FilterResult

        result = FilterResult(
            should_capture=True,
            reason="Contains a concrete technology decision with rationale",
            domain="architecture",
        )

        assert result.should_capture is True
        assert "decision" in result.reason
        assert result.domain == "architecture"

    def test_reject_result(self):
        from agents.scribe.tier2_filter import FilterResult

        result = FilterResult(
            should_capture=False,
            reason="Casual greeting, no decision content",
            domain="general",
        )

        assert result.should_capture is False


class TestTier2Filter:
    """Tests for Tier2Filter"""

    @pytest.fixture
    def mock_anthropic_response(self):
        """Create a mock Anthropic API response"""
        def _make_response(capture: bool, reason: str, domain: str = "general"):
            response = Mock()
            content_block = Mock()
            content_block.text = json.dumps({
                "capture": capture,
                "reason": reason,
                "domain": domain,
            })
            response.content = [content_block]
            return response
        return _make_response

    @pytest.fixture
    def filter_with_mock(self, mock_anthropic_response):
        """Create a Tier2Filter with mocked LLM client"""
        from agents.scribe.tier2_filter import Tier2Filter

        f = Tier2Filter.__new__(Tier2Filter)
        f._provider = "anthropic"
        f._model = "claude-haiku-4-5-20251001"

        mock_llm = Mock()
        mock_llm.is_available = True
        mock_llm.generate.return_value = json.dumps({
            "capture": True,
            "reason": "Contains a concrete architecture decision",
            "domain": "architecture",
        })
        f._llm = mock_llm
        return f

    def test_evaluate_captures_decision(self, filter_with_mock, mock_anthropic_response):
        """Test that a real decision is captured"""
        filter_with_mock._llm.generate.return_value = json.dumps({
            "capture": True,
            "reason": "Concrete technology choice with rationale",
            "domain": "architecture",
        })

        result = filter_with_mock.evaluate(
            "We decided to use PostgreSQL instead of MongoDB because we need ACID compliance",
            tier1_score=0.75,
            tier1_pattern="We decided to use X instead of Y because...",
        )

        assert result.should_capture is True
        assert result.domain == "architecture"

    def test_evaluate_rejects_casual(self, filter_with_mock, mock_anthropic_response):
        """Test that casual chat is rejected"""
        filter_with_mock._llm.generate.return_value = json.dumps({
            "capture": False,
            "reason": "Casual greeting with no decision content",
            "domain": "general",
        })

        result = filter_with_mock.evaluate(
            "Good morning everyone! How was your weekend?",
            tier1_score=0.55,
        )

        assert result.should_capture is False

    def test_evaluate_rejects_vague(self, filter_with_mock, mock_anthropic_response):
        """Test that vague opinions are rejected"""
        filter_with_mock._llm.generate.return_value = json.dumps({
            "capture": False,
            "reason": "Vague opinion without commitment or concrete decision",
            "domain": "general",
        })

        result = filter_with_mock.evaluate(
            "Maybe we should consider using a different database sometime",
            tier1_score=0.52,
        )

        assert result.should_capture is False

    def test_evaluate_captures_policy(self, filter_with_mock, mock_anthropic_response):
        """Test that policy statements are captured"""
        filter_with_mock._llm.generate.return_value = json.dumps({
            "capture": True,
            "reason": "Establishes a team security policy",
            "domain": "security",
        })

        result = filter_with_mock.evaluate(
            "All API keys must be rotated every 90 days, no exceptions",
            tier1_score=0.68,
        )

        assert result.should_capture is True
        assert result.domain == "security"

    def test_evaluate_fallback_on_unavailable(self):
        """Test fallback when LLM is unavailable"""
        from agents.scribe.tier2_filter import Tier2Filter

        f = Tier2Filter.__new__(Tier2Filter)
        f._provider = "anthropic"
        f._model = "claude-haiku-4-5-20251001"
        mock_llm = Mock()
        mock_llm.is_available = False
        f._llm = mock_llm

        assert f.is_available is False

        result = f.evaluate("Some text")
        # Should pass through (default to capture)
        assert result.should_capture is True
        assert "unavailable" in result.reason.lower()

    def test_evaluate_fallback_on_error(self, filter_with_mock):
        """Test fallback when LLM call fails"""
        filter_with_mock._llm.generate.side_effect = Exception("API error")

        result = filter_with_mock.evaluate("Some text")
        # Should pass through on error
        assert result.should_capture is True
        assert "error" in result.reason.lower()

    def test_parse_response_valid_json(self):
        """Test parsing valid JSON response"""
        from agents.scribe.tier2_filter import Tier2Filter

        f = Tier2Filter.__new__(Tier2Filter)
        f._provider = "anthropic"
        f._model = "test"
        mock_llm = Mock()
        mock_llm.is_available = False
        f._llm = mock_llm

        result = f._parse_response('{"capture": true, "reason": "test", "domain": "ops"}')
        assert result.should_capture is True
        assert result.domain == "ops"

    def test_parse_response_with_markdown_fences(self):
        """Test parsing response wrapped in markdown code fences"""
        from agents.scribe.tier2_filter import Tier2Filter

        f = Tier2Filter.__new__(Tier2Filter)
        f._provider = "anthropic"
        f._model = "test"
        mock_llm = Mock()
        mock_llm.is_available = False
        f._llm = mock_llm

        result = f._parse_response('```json\n{"capture": false, "reason": "not relevant", "domain": "general"}\n```')
        assert result.should_capture is False

    def test_parse_response_invalid_json(self):
        """Test fallback on invalid JSON"""
        from agents.scribe.tier2_filter import Tier2Filter

        f = Tier2Filter.__new__(Tier2Filter)
        f._provider = "anthropic"
        f._model = "test"
        mock_llm = Mock()
        mock_llm.is_available = False
        f._llm = mock_llm

        result = f._parse_response("This is not JSON at all")
        # Should default to capture
        assert result.should_capture is True

    def test_system_prompt_content(self):
        """Test that the policy prompt covers key concepts"""
        from agents.scribe.tier2_filter import FILTER_POLICY

        # Should mention what to capture
        assert "decision" in FILTER_POLICY.lower()
        assert "policy" in FILTER_POLICY.lower()
        assert "trade-off" in FILTER_POLICY.lower()

        # Should mention what NOT to capture
        assert "casual" in FILTER_POLICY.lower()
        assert "status update" in FILTER_POLICY.lower()
        assert "vague" in FILTER_POLICY.lower()

    def test_text_truncation(self, filter_with_mock, mock_anthropic_response):
        """Test that very long messages are truncated"""
        filter_with_mock._llm.generate.return_value = json.dumps({
            "capture": True, "reason": "test", "domain": "general",
        })

        long_text = "x" * 1000
        filter_with_mock.evaluate(long_text)

        # Verify the call was made with truncated text
        call_args = filter_with_mock._llm.generate.call_args
        user_msg = call_args.args[0]
        # Message prefix "<message>\n" + 500 chars max + "\n</message>"
        assert len(user_msg) <= 600


class TestTier2Integration:
    """Integration tests for the 3-tier pipeline flow"""

    def test_tier1_pass_tier2_reject(self):
        """Tier 1 passes but Tier 2 correctly rejects false positive"""
        from agents.scribe.tier2_filter import FilterResult

        # Simulate: "We decided to order pizza" triggers Tier 1 ("We decided")
        # but Tier 2 should reject it as non-organizational
        tier2_result = FilterResult(
            should_capture=False,
            reason="Food order, not organizational decision",
        )

        assert tier2_result.should_capture is False

    def test_tier1_pass_tier2_pass(self):
        """Both tiers agree on a real decision"""
        from agents.scribe.tier2_filter import FilterResult

        tier2_result = FilterResult(
            should_capture=True,
            reason="Concrete database technology choice with ACID rationale",
            domain="architecture",
        )

        assert tier2_result.should_capture is True
        assert tier2_result.domain == "architecture"
