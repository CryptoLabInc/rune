"""
Tests for Decision Detector

Tests pattern-based decision detection using similarity search.
"""

import pytest
from unittest.mock import Mock, MagicMock


class TestDetectionResult:
    """Tests for DetectionResult dataclass"""

    def test_detection_result_significant(self):
        from agents.scribe.detector import DetectionResult

        result = DetectionResult(
            is_significant=True,
            confidence=0.85,
            matched_pattern="We decided to use",
            category="architecture",
            domain="architecture",
            priority="high",
        )

        assert result.is_significant is True
        assert result.confidence == 0.85
        assert result.matched_pattern == "We decided to use"

    def test_detection_result_not_significant(self):
        from agents.scribe.detector import DetectionResult

        result = DetectionResult(
            is_significant=False,
            confidence=0.3,
        )

        assert result.is_significant is False
        assert result.matched_pattern is None


class TestDecisionDetector:
    """Tests for DecisionDetector"""

    @pytest.fixture
    def mock_pattern_cache(self):
        """Create a mock pattern cache"""
        from agents.common.pattern_cache import PatternEntry

        cache = Mock()
        cache.pattern_count = 10

        # Default behavior: no match
        cache.find_best_match.return_value = (None, 0.3)
        cache.find_top_matches.return_value = []

        return cache

    @pytest.fixture
    def detector(self, mock_pattern_cache):
        """Create detector with mock cache"""
        from agents.scribe.detector import DecisionDetector

        return DecisionDetector(
            pattern_cache=mock_pattern_cache,
            threshold=0.7,
            high_confidence_threshold=0.8
        )

    def test_detect_significant_decision(self, detector, mock_pattern_cache):
        """Test detecting a significant decision"""
        from agents.common.pattern_cache import PatternEntry

        # Mock a match
        matched_pattern = PatternEntry(
            text="We decided to use",
            category="architecture",
            priority="high",
            embedding=[0.1] * 384,
            domain="architecture"
        )
        mock_pattern_cache.find_best_match.return_value = (matched_pattern, 0.85)

        result = detector.detect("We decided to use PostgreSQL for better JSON support")

        assert result.is_significant is True
        assert result.confidence == 0.85
        assert result.matched_pattern == "We decided to use"
        assert result.category == "architecture"

    def test_detect_not_significant(self, detector, mock_pattern_cache):
        """Test detecting a non-significant message"""
        mock_pattern_cache.find_best_match.return_value = (None, 0.3)

        result = detector.detect("Good morning everyone!")

        assert result.is_significant is False
        assert result.confidence == 0.3

    def test_detect_empty_text(self, detector):
        """Test with empty text"""
        result = detector.detect("")

        assert result.is_significant is False
        assert result.confidence == 0.0

    def test_detect_short_text(self, detector):
        """Test with very short text"""
        result = detector.detect("Hi")

        assert result.is_significant is False

    def test_should_auto_capture_high_confidence(self, detector, mock_pattern_cache):
        """Test auto-capture for high confidence results"""
        from agents.scribe.detector import DetectionResult

        result = DetectionResult(
            is_significant=True,
            confidence=0.9,
            matched_pattern="We decided to",
            priority="high",
        )

        assert detector.should_auto_capture(result) is True

    def test_should_not_auto_capture_low_confidence(self, detector):
        """Test no auto-capture for low confidence"""
        from agents.scribe.detector import DetectionResult

        result = DetectionResult(
            is_significant=True,
            confidence=0.75,  # Below high_confidence_threshold
            matched_pattern="We decided to",
            priority="high",
        )

        assert detector.should_auto_capture(result) is False

    def test_needs_review_moderate_confidence(self, detector):
        """Test review needed for moderate confidence"""
        from agents.scribe.detector import DetectionResult

        result = DetectionResult(
            is_significant=True,
            confidence=0.75,
            matched_pattern="After discussion",
            priority="medium",
        )

        assert detector.needs_review(result) is True

    def test_explain_detection(self, detector):
        """Test detection explanation"""
        from agents.scribe.detector import DetectionResult

        result = DetectionResult(
            is_significant=True,
            confidence=0.85,
            matched_pattern="We decided to use",
            category="architecture",
            domain="architecture",
            priority="high",
        )

        explanation = detector.explain_detection(result)

        assert "Significant decision detected" in explanation
        assert "We decided to use" in explanation
        assert "architecture" in explanation


class TestPatternMatching:
    """Integration tests for pattern matching (requires embedding service)"""

    @pytest.mark.skip(reason="Requires embedding service - run manually")
    def test_real_pattern_matching(self):
        """Test with real embeddings"""
        from agents.common.embedding_service import EmbeddingService
        from agents.common.pattern_cache import PatternCache
        from agents.scribe.detector import DecisionDetector
        from agents.scribe.pattern_parser import get_builtin_patterns

        # Initialize
        embedding = EmbeddingService()
        cache = PatternCache(embedding)
        cache.load_patterns(get_builtin_patterns())

        detector = DecisionDetector(cache, threshold=0.7)

        # Test decision text
        result = detector.detect(
            "We decided to use PostgreSQL instead of MySQL because of better JSON support"
        )

        assert result.is_significant is True
        assert result.confidence > 0.7

        # Test routine text
        result = detector.detect("Good morning team!")

        assert result.is_significant is False
