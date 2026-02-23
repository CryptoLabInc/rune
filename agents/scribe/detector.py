"""
Decision Detector

Similarity-based decision detection using pre-embedded patterns.
Core component of the Scribe agent's Stage 1 pipeline.
"""

from dataclasses import dataclass
from typing import Optional, List, Tuple

from ..common.pattern_cache import PatternCache, PatternEntry


@dataclass
class DetectionResult:
    """Result of decision detection"""
    is_significant: bool
    confidence: float
    matched_pattern: Optional[str] = None
    category: Optional[str] = None
    domain: Optional[str] = None
    priority: Optional[str] = None
    top_matches: Optional[List[Tuple[str, float]]] = None  # For debugging


class DecisionDetector:
    """
    Detects significant decisions using similarity search.

    Algorithm:
    1. Embed incoming text
    2. Compute similarity to all pre-embedded patterns
    3. If max similarity > threshold, mark as significant
    4. Return detection result with confidence and matched pattern

    This replaces ML-based classification with on-device similarity search.
    """

    def __init__(
        self,
        pattern_cache: PatternCache,
        threshold: float = 0.35,
        high_confidence_threshold: float = 0.7
    ):
        """
        Initialize decision detector.

        Args:
            pattern_cache: PatternCache with pre-embedded patterns
            threshold: Minimum similarity to consider significant
            high_confidence_threshold: Threshold for auto-capture (no review)
        """
        self._cache = pattern_cache
        self._threshold = threshold
        self._high_confidence_threshold = high_confidence_threshold

    @property
    def threshold(self) -> float:
        return self._threshold

    @property
    def high_confidence_threshold(self) -> float:
        return self._high_confidence_threshold

    def detect(self, text: str) -> DetectionResult:
        """
        Detect if text contains a significant decision.

        Args:
            text: Input text to analyze

        Returns:
            DetectionResult with significance, confidence, and matched pattern
        """
        if not text or not text.strip():
            return DetectionResult(
                is_significant=False,
                confidence=0.0,
            )

        # Skip very short messages
        if len(text.strip()) < 20:
            return DetectionResult(
                is_significant=False,
                confidence=0.0,
            )

        # Find best matching pattern
        match, score = self._cache.find_best_match(text, threshold=0.0)

        # Determine significance
        is_significant = score >= self._threshold

        if match:
            return DetectionResult(
                is_significant=is_significant,
                confidence=score,
                matched_pattern=match.text,
                category=match.category,
                domain=match.domain,
                priority=match.priority,
            )

        return DetectionResult(
            is_significant=False,
            confidence=score,
        )

    def detect_with_details(self, text: str, top_k: int = 5) -> DetectionResult:
        """
        Detect with additional details including top matches.

        Useful for debugging and understanding why a decision was detected.

        Args:
            text: Input text to analyze
            top_k: Number of top matches to include

        Returns:
            DetectionResult with top_matches for debugging
        """
        if not text or not text.strip():
            return DetectionResult(
                is_significant=False,
                confidence=0.0,
                top_matches=[],
            )

        # Find top matches
        matches = self._cache.find_top_matches(text, top_k=top_k, threshold=0.0)

        if not matches:
            return DetectionResult(
                is_significant=False,
                confidence=0.0,
                top_matches=[],
            )

        # Best match
        best_pattern, best_score = matches[0]
        is_significant = best_score >= self._threshold

        # Format top matches for debugging
        top_matches = [(p.text, s) for p, s in matches]

        return DetectionResult(
            is_significant=is_significant,
            confidence=best_score,
            matched_pattern=best_pattern.text,
            category=best_pattern.category,
            domain=best_pattern.domain,
            priority=best_pattern.priority,
            top_matches=top_matches,
        )

    def should_auto_capture(self, result: DetectionResult) -> bool:
        """
        Check if detection result warrants auto-capture (skip review).

        Auto-capture when:
        - Is significant AND
        - Confidence >= high_confidence_threshold

        Args:
            result: Detection result to evaluate

        Returns:
            True if should auto-capture, False if needs review
        """
        if not result.is_significant:
            return False

        return result.confidence >= self._high_confidence_threshold

    def needs_review(self, result: DetectionResult) -> bool:
        """
        Check if detection result needs human review.

        Review needed when:
        - Is significant but confidence < high_confidence_threshold
        - Medium priority pattern

        Args:
            result: Detection result to evaluate

        Returns:
            True if needs review, False otherwise
        """
        if not result.is_significant:
            return False

        return not self.should_auto_capture(result)

    def explain_detection(self, result: DetectionResult) -> str:
        """
        Generate human-readable explanation of detection.

        Args:
            result: Detection result to explain

        Returns:
            Explanation string
        """
        if not result.is_significant:
            return f"Not significant (confidence: {result.confidence:.2f}, threshold: {self._threshold})"

        lines = [
            f"Significant decision detected (confidence: {result.confidence:.2f})",
            f"  Matched pattern: \"{result.matched_pattern}\"",
            f"  Category: {result.category}",
            f"  Domain: {result.domain}",
            f"  Priority: {result.priority}",
        ]

        if self.should_auto_capture(result):
            lines.append("  Action: AUTO-CAPTURE (high confidence)")
        else:
            lines.append("  Action: NEEDS REVIEW (moderate confidence)")

        if result.top_matches:
            lines.append("  Top matches:")
            for pattern, score in result.top_matches[:3]:
                lines.append(f"    - \"{pattern[:50]}...\" ({score:.2f})")

        return "\n".join(lines)
