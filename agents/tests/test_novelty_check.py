"""Tests for novelty check logic."""
import pytest


def test_classify_novel():
    """Score below NOVEL_THRESHOLD = novel."""
    from agents.common.schemas.embedding import classify_novelty
    result = classify_novelty(max_similarity=0.2, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "novel"
    assert result["score"] == pytest.approx(0.8)  # 1 - 0.2


def test_classify_evolution():
    """Score between thresholds = evolution."""
    from agents.common.schemas.embedding import classify_novelty
    result = classify_novelty(max_similarity=0.5, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "evolution"
    assert result["score"] == pytest.approx(0.5)


def test_classify_redundant():
    """Score above REDUNDANT_THRESHOLD = redundant."""
    from agents.common.schemas.embedding import classify_novelty
    result = classify_novelty(max_similarity=0.85, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "redundant"
    assert result["score"] == pytest.approx(0.15)


def test_classify_empty_memory():
    """No existing records = max novelty."""
    from agents.common.schemas.embedding import classify_novelty
    result = classify_novelty(max_similarity=0.0, threshold_novel=0.3, threshold_redundant=0.7)
    assert result["class"] == "novel"
    assert result["score"] == pytest.approx(1.0)
