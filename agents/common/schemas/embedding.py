"""
Embedding text selection for DecisionRecords.

Schema 2.1+ uses reusable_insight as the primary embedding target.
Schema 2.0 falls back to payload.text.
"""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .decision_record import DecisionRecord


def embedding_text_for_record(record: "DecisionRecord") -> str:
    """Select the text to embed in enVector.

    Schema 2.1+: use reusable_insight (dense NL gist).
    Schema 2.0 fallback: use payload.text (verbose markdown).
    """
    insight = getattr(record, "reusable_insight", "")
    if insight and insight.strip():
        return insight.strip()
    return record.payload.text
