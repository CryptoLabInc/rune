"""
Review Queue

Manages human review for low-confidence captures.
Implements Stage 3 of the Scribe pipeline.

Review Questions (minimum 3):
Q1. Is this worth saving? (Capture/Ignore)
Q2. Is the "Why" supported by evidence? (Supported/Partial/Unknown)
Q3. Is the sensitivity label correct? (public/internal/restricted)
Q4. (Optional) Is this the final decision? (proposed/accepted/superseded/reverted)
"""

import json
from datetime import datetime, timezone
from pathlib import Path
from typing import List, Optional, Dict, Any
from dataclasses import dataclass, field, asdict
from enum import Enum

from ..common.schemas import (
    DecisionRecord,
    Certainty,
    Sensitivity,
    Status,
    ReviewState,
)
from ..common.schemas.templates import render_payload_text
from ..common.config import REVIEW_QUEUE_PATH


class ReviewAnswer(str, Enum):
    """Possible answers for review questions"""
    # Q1: Worth saving?
    CAPTURE = "capture"
    IGNORE = "ignore"

    # Q2: Evidence supported?
    SUPPORTED = "supported"
    PARTIALLY_SUPPORTED = "partially_supported"
    UNKNOWN = "unknown"

    # Q3: Sensitivity
    PUBLIC = "public"
    INTERNAL = "internal"
    RESTRICTED = "restricted"

    # Q4: Status
    PROPOSED = "proposed"
    ACCEPTED = "accepted"
    SUPERSEDED = "superseded"
    REVERTED = "reverted"


@dataclass
class ReviewAnswers:
    """Answers to review questions"""
    q1_worth_saving: ReviewAnswer  # capture or ignore
    q2_evidence_supported: ReviewAnswer  # supported, partially_supported, unknown
    q3_sensitivity: ReviewAnswer  # public, internal, restricted
    q4_status: Optional[ReviewAnswer] = None  # proposed, accepted, superseded, reverted
    reviewer_notes: Optional[str] = None


@dataclass
class ReviewItem:
    """Item in the review queue"""
    record_id: str
    record_json: Dict[str, Any]
    detection_confidence: float
    created_at: str
    questions: List[str] = field(default_factory=list)
    status: str = "pending"  # pending, reviewed, expired


class ReviewQueue:
    """
    Manages the review queue for human review of captures.

    The queue is persisted to ~/.rune/review_queue.json.

    Workflow:
    1. Scribe adds low-confidence captures to queue
    2. Human reviews via UI or CLI
    3. Approved items are stored to enVector
    4. Ignored items are discarded
    """

    # Standard review questions
    QUESTIONS = [
        "Q1. Is this decision/learning/rejection worth saving to organizational memory?",
        "Q2. Is the 'Why' (rationale) supported by the evidence (quotes)?",
        "Q3. Is the sensitivity label (public/internal/restricted) correct?",
        "Q4. (Optional) Is this the final decision status?",
    ]

    def __init__(self, queue_path: Optional[Path] = None):
        """
        Initialize review queue.

        Args:
            queue_path: Path to queue file (default: ~/.rune/review_queue.json)
        """
        self._queue_path = queue_path or REVIEW_QUEUE_PATH
        self._queue: List[ReviewItem] = []
        self._load_queue()

    def _load_queue(self) -> None:
        """Load queue from disk"""
        if not self._queue_path.exists():
            self._queue = []
            return

        try:
            with open(self._queue_path) as f:
                data = json.load(f)

            self._queue = [
                ReviewItem(
                    record_id=item["record_id"],
                    record_json=item["record_json"],
                    detection_confidence=item["detection_confidence"],
                    created_at=item["created_at"],
                    questions=item.get("questions", self.QUESTIONS),
                    status=item.get("status", "pending"),
                )
                for item in data
            ]
        except (json.JSONDecodeError, IOError, KeyError) as e:
            print(f"[ReviewQueue] Warning: Failed to load queue: {e}")
            self._queue = []

    def _save_queue(self) -> None:
        """Save queue to disk"""
        self._queue_path.parent.mkdir(parents=True, exist_ok=True)

        data = [
            {
                "record_id": item.record_id,
                "record_json": item.record_json,
                "detection_confidence": item.detection_confidence,
                "created_at": item.created_at,
                "questions": item.questions,
                "status": item.status,
            }
            for item in self._queue
        ]

        with open(self._queue_path, "w") as f:
            json.dump(data, f, indent=2, default=str)

    def add(
        self,
        record: DecisionRecord,
        detection_confidence: float
    ) -> str:
        """
        Add a record to the review queue.

        Args:
            record: Decision record to review
            detection_confidence: Confidence from detector

        Returns:
            Record ID
        """
        # Convert record to dict for JSON storage
        record_dict = record.model_dump(mode='json')

        item = ReviewItem(
            record_id=record.id,
            record_json=record_dict,
            detection_confidence=detection_confidence,
            created_at=datetime.now(timezone.utc).isoformat(),
            questions=self.QUESTIONS.copy(),
            status="pending",
        )

        self._queue.append(item)
        self._save_queue()

        print(f"[ReviewQueue] Added {record.id} for review (confidence: {detection_confidence:.2f})")
        return record.id

    def get_pending(self) -> List[ReviewItem]:
        """Get all pending review items"""
        return [item for item in self._queue if item.status == "pending"]

    def get_item(self, record_id: str) -> Optional[ReviewItem]:
        """Get a specific review item by ID"""
        for item in self._queue:
            if item.record_id == record_id:
                return item
        return None

    def submit_review(
        self,
        record_id: str,
        answers: ReviewAnswers,
        reviewer: Optional[str] = None
    ) -> Optional[DecisionRecord]:
        """
        Submit review for an item.

        Args:
            record_id: ID of the record being reviewed
            answers: Review answers
            reviewer: Reviewer identifier

        Returns:
            Updated DecisionRecord if approved, None if ignored

        Side effects:
            - Updates item status in queue
            - Modifies record based on answers
        """
        item = self.get_item(record_id)
        if not item:
            print(f"[ReviewQueue] Item not found: {record_id}")
            return None

        # Check Q1: Worth saving?
        if answers.q1_worth_saving == ReviewAnswer.IGNORE:
            item.status = "rejected"
            self._save_queue()
            print(f"[ReviewQueue] Item {record_id} rejected by reviewer")
            return None

        # Reconstruct record from JSON
        record = DecisionRecord.model_validate(item.record_json)

        # Apply Q2: Update certainty
        certainty_map = {
            ReviewAnswer.SUPPORTED: Certainty.SUPPORTED,
            ReviewAnswer.PARTIALLY_SUPPORTED: Certainty.PARTIALLY_SUPPORTED,
            ReviewAnswer.UNKNOWN: Certainty.UNKNOWN,
        }
        if answers.q2_evidence_supported in certainty_map:
            record.why.certainty = certainty_map[answers.q2_evidence_supported]

        # Apply Q3: Update sensitivity
        sensitivity_map = {
            ReviewAnswer.PUBLIC: Sensitivity.PUBLIC,
            ReviewAnswer.INTERNAL: Sensitivity.INTERNAL,
            ReviewAnswer.RESTRICTED: Sensitivity.RESTRICTED,
        }
        if answers.q3_sensitivity in sensitivity_map:
            record.sensitivity = sensitivity_map[answers.q3_sensitivity]

        # Apply Q4: Update status (if provided)
        if answers.q4_status:
            status_map = {
                ReviewAnswer.PROPOSED: Status.PROPOSED,
                ReviewAnswer.ACCEPTED: Status.ACCEPTED,
                ReviewAnswer.SUPERSEDED: Status.SUPERSEDED,
                ReviewAnswer.REVERTED: Status.REVERTED,
            }
            if answers.q4_status in status_map:
                record.status = status_map[answers.q4_status]

        # Update quality metadata
        record.quality.review_state = ReviewState.APPROVED
        record.quality.reviewed_by = reviewer
        if answers.reviewer_notes:
            existing_notes = record.quality.review_notes or ""
            record.quality.review_notes = f"{existing_notes}\nReviewer: {answers.reviewer_notes}".strip()

        # Regenerate payload.text with updated values
        record.payload.text = render_payload_text(record)

        # Update queue status
        item.status = "reviewed"
        self._save_queue()

        print(f"[ReviewQueue] Item {record_id} approved by {reviewer or 'unknown'}")
        return record

    def remove(self, record_id: str) -> bool:
        """Remove an item from the queue"""
        for i, item in enumerate(self._queue):
            if item.record_id == record_id:
                del self._queue[i]
                self._save_queue()
                return True
        return False

    def clear_reviewed(self) -> int:
        """Clear all reviewed items from queue"""
        original_len = len(self._queue)
        self._queue = [item for item in self._queue if item.status == "pending"]
        self._save_queue()
        return original_len - len(self._queue)

    def get_stats(self) -> Dict[str, int]:
        """Get queue statistics"""
        stats = {
            "total": len(self._queue),
            "pending": 0,
            "reviewed": 0,
            "rejected": 0,
        }
        for item in self._queue:
            if item.status in stats:
                stats[item.status] += 1
        return stats

    def format_for_review(self, item: ReviewItem) -> str:
        """Format a review item for display"""
        record_dict = item.record_json

        lines = [
            "=" * 60,
            f"REVIEW ITEM: {item.record_id}",
            f"Detection Confidence: {item.detection_confidence:.2f}",
            f"Created: {item.created_at}",
            "=" * 60,
            "",
            f"Title: {record_dict.get('title', 'N/A')}",
            f"Domain: {record_dict.get('domain', 'N/A')}",
            f"Current Sensitivity: {record_dict.get('sensitivity', 'N/A')}",
            f"Current Status: {record_dict.get('status', 'N/A')}",
            "",
            "Decision:",
            f"  {record_dict.get('decision', {}).get('what', 'N/A')[:200]}",
            "",
            "Why (Rationale):",
            f"  {record_dict.get('why', {}).get('rationale_summary', 'N/A')[:200]}",
            f"  Certainty: {record_dict.get('why', {}).get('certainty', 'N/A')}",
            "",
            "Evidence:",
        ]

        evidence = record_dict.get('evidence', [])
        if evidence:
            for i, e in enumerate(evidence[:3], 1):
                lines.append(f"  {i}. Claim: {e.get('claim', 'N/A')[:100]}")
                lines.append(f"     Quote: \"{e.get('quote', 'N/A')[:100]}\"")
        else:
            lines.append("  (none)")

        lines.extend([
            "",
            "-" * 60,
            "REVIEW QUESTIONS:",
        ])

        for i, q in enumerate(item.questions, 1):
            lines.append(f"  {q}")

        lines.append("=" * 60)

        return "\n".join(lines)
