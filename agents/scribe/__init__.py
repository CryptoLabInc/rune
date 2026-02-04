"""
Scribe Agent - Organizational Context Capture

Monitors team communications (Slack, GitHub, Notion) to identify and capture
significant decisions using on-device similarity search.

Key Components:
- DecisionDetector: Pattern-based decision detection
- RecordBuilder: Creates Decision Record with evidence
- ReviewQueue: Manages human review for low-confidence captures
- Handlers: Source-specific event processing (Slack, GitHub, etc.)

10 Rules for Scribe:
1. Not a logger - only capture significant decisions
2. Schema v2 only - JSON + payload.text
3. Why cannot be confirmed without evidence
4. Evidence requires at least 1 quote
5. Quotes should be 1-2 sentences (direct)
6. No assumptions about finality without explicit signals
7. Update status on decision reversal
8. Default sensitivity to 'internal' when unclear
9. Remove PII/credentials, note in review_notes
10. Output always includes JSON + payload.text
"""

from .detector import DecisionDetector, DetectionResult
from .record_builder import RecordBuilder
from .pattern_parser import parse_capture_triggers
from .review_queue import ReviewQueue, ReviewItem, ReviewAnswers

__all__ = [
    "DecisionDetector",
    "DetectionResult",
    "RecordBuilder",
    "parse_capture_triggers",
    "ReviewQueue",
    "ReviewItem",
    "ReviewAnswers",
]
