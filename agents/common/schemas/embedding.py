"""
Embedding text selection and novelty classification for DecisionRecords.

Schema 2.1+ uses reusable_insight as the primary embedding target.
Schema 2.0 falls back to payload.text.
"""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .decision_record import DecisionRecord


# Novelty thresholds (Memory-as-Filter)
NOVELTY_THRESHOLD_NOVEL = 0.3
NOVELTY_THRESHOLD_RELATED = 0.7
NOVELTY_THRESHOLD_NEAR_DUPLICATE = 0.95


def embedding_text_for_record(record: "DecisionRecord") -> str:
    """Select the text to embed in enVector.

    Schema 2.1+: use reusable_insight (dense NL gist).
    Schema 2.0 fallback: use payload.text (verbose markdown).
    """
    insight = getattr(record, "reusable_insight", "")
    if insight and insight.strip():
        return insight.strip()
    return record.payload.text


def classify_novelty(
    max_similarity: float,
    threshold_novel: float = NOVELTY_THRESHOLD_NOVEL,
    threshold_related: float = NOVELTY_THRESHOLD_RELATED,
    threshold_near_duplicate: float = NOVELTY_THRESHOLD_NEAR_DUPLICATE,
) -> dict:
    """Classify capture novelty based on similarity to existing memory.

    Returns dict with 'score' (0-1, higher=more novel) and 'class'.
    Classes (annotation-only except near_duplicate):
      - near_duplicate (>= 0.95): blocks capture
      - related (>= 0.7): annotation only
      - evolution (>= 0.3): annotation only
      - novel (< 0.3): annotation only
    """
    novelty_score = 1.0 - max_similarity
    if max_similarity >= threshold_near_duplicate:
        return {"class": "near_duplicate", "score": round(novelty_score, 4)}
    elif max_similarity >= threshold_related:
        return {"class": "related", "score": round(novelty_score, 4)}
    elif max_similarity >= threshold_novel:
        return {"class": "evolution", "score": round(novelty_score, 4)}
    else:
        return {"class": "novel", "score": round(novelty_score, 4)}
