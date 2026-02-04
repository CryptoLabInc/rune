"""
Decision Record Schema v2

Core principle: Memory items always have a "text payload" that can fully reproduce the context.
The embedding is generated from that text payload.
"Why" cannot be written definitively without evidence.
"""

from datetime import datetime
from typing import List, Optional, Literal
from pydantic import BaseModel, Field
from enum import Enum


# ============================================================================
# Enums
# ============================================================================

class Domain(str, Enum):
    """Decision domain categories"""
    ARCHITECTURE = "architecture"
    SECURITY = "security"
    PRODUCT = "product"
    EXEC = "exec"
    OPS = "ops"
    DESIGN = "design"
    DATA = "data"
    HR = "hr"
    MARKETING = "marketing"
    GENERAL = "general"


class Sensitivity(str, Enum):
    """Data sensitivity levels"""
    PUBLIC = "public"
    INTERNAL = "internal"
    RESTRICTED = "restricted"


class Status(str, Enum):
    """Decision status"""
    PROPOSED = "proposed"
    ACCEPTED = "accepted"
    SUPERSEDED = "superseded"
    REVERTED = "reverted"


class Certainty(str, Enum):
    """Evidence certainty level for 'Why'"""
    SUPPORTED = "supported"
    PARTIALLY_SUPPORTED = "partially_supported"
    UNKNOWN = "unknown"


class ReviewState(str, Enum):
    """Human review state"""
    UNREVIEWED = "unreviewed"
    APPROVED = "approved"
    EDITED = "edited"
    REJECTED = "rejected"


class SourceType(str, Enum):
    """Source types for evidence"""
    SLACK = "slack"
    MEETING = "meeting"
    DOC = "doc"
    GITHUB = "github"
    EMAIL = "email"
    NOTION = "notion"
    OTHER = "other"


# ============================================================================
# Sub-models
# ============================================================================

class SourceRef(BaseModel):
    """Reference to the source of evidence"""
    type: SourceType
    url: Optional[str] = None
    pointer: Optional[str] = None  # e.g., "channel:#arch thread_ts:123" or "timestamp:00:32:14"


class Evidence(BaseModel):
    """Evidence supporting a claim with direct quote"""
    claim: str = Field(..., description="What is being claimed")
    quote: str = Field(..., description="Direct quote (1-2 sentences)")
    source: SourceRef


class Assumption(BaseModel):
    """Assumption with confidence level"""
    assumption: str
    confidence: float = Field(ge=0.0, le=1.0, default=0.5)


class Risk(BaseModel):
    """Risk with mitigation strategy"""
    risk: str
    mitigation: Optional[str] = None


class DecisionDetail(BaseModel):
    """What was decided, by whom, when, where"""
    what: str = Field(..., description="The actual decision statement")
    who: List[str] = Field(default_factory=list, description="Participants (role:cto, user:alice)")
    where: str = Field(default="", description="Channel/meeting where decided")
    when: str = Field(default="", description="Date of decision (YYYY-MM-DD)")


class Context(BaseModel):
    """Context surrounding the decision"""
    problem: str = Field(default="", description="Problem being solved")
    scope: Optional[str] = None
    constraints: List[str] = Field(default_factory=list)
    alternatives: List[str] = Field(default_factory=list)
    chosen: str = Field(default="", description="Chosen alternative")
    trade_offs: List[str] = Field(default_factory=list)
    assumptions: List[Assumption] = Field(default_factory=list)
    risks: List[Risk] = Field(default_factory=list)


class Why(BaseModel):
    """
    Rationale for the decision.

    CRITICAL RULE: certainty cannot be 'supported' without evidence.
    If evidence is missing, certainty MUST be 'unknown'.
    """
    rationale_summary: str = Field(default="", description="Summary of why this decision was made")
    certainty: Certainty = Field(default=Certainty.UNKNOWN)
    missing_info: List[str] = Field(default_factory=list, description="What information is missing")


class Quality(BaseModel):
    """Quality metrics for the capture"""
    scribe_confidence: float = Field(ge=0.0, le=1.0, default=0.5)
    review_state: ReviewState = Field(default=ReviewState.UNREVIEWED)
    reviewed_by: Optional[str] = None
    review_notes: Optional[str] = None


class Payload(BaseModel):
    """
    The normalized text payload for embedding.
    This is the SINGLE SOURCE OF TRUTH for memory reproduction.
    """
    format: Literal["markdown"] = "markdown"
    text: str = Field(default="", description="Markdown text for embedding")


# ============================================================================
# Main Schema
# ============================================================================

class DecisionRecord(BaseModel):
    """
    Decision Record Schema v2

    Core principle: payload.text must be able to fully reproduce the memory.
    """
    schema_version: str = Field(default="2.0")
    id: str = Field(..., description="Unique ID: dec_YYYY-MM-DD_domain_slug")
    type: Literal["decision_record"] = "decision_record"

    domain: Domain = Field(default=Domain.GENERAL)
    sensitivity: Sensitivity = Field(default=Sensitivity.INTERNAL)
    status: Status = Field(default=Status.PROPOSED)
    superseded_by: Optional[str] = None
    timestamp: datetime = Field(default_factory=datetime.utcnow)

    title: str = Field(..., description="Short title for the decision")
    decision: DecisionDetail
    context: Context = Field(default_factory=Context)
    why: Why = Field(default_factory=Why)
    evidence: List[Evidence] = Field(default_factory=list)

    links: List[dict] = Field(default_factory=list, description="Related links (ADR, PR, etc.)")
    tags: List[str] = Field(default_factory=list)

    quality: Quality = Field(default_factory=Quality)
    payload: Payload = Field(default_factory=Payload)

    def validate_evidence_certainty(self) -> bool:
        """
        Validate that certainty is appropriate given evidence.
        Returns True if valid, False if certainty should be downgraded.
        """
        has_quotes = any(e.quote for e in self.evidence)

        if self.why.certainty == Certainty.SUPPORTED and not has_quotes:
            return False
        return True

    def ensure_evidence_certainty_consistency(self) -> None:
        """
        Enforce: Why cannot be 'supported' without evidence quotes.
        Mutates the record to fix inconsistencies.
        """
        has_quotes = any(e.quote for e in self.evidence)

        if not has_quotes:
            if self.why.certainty == Certainty.SUPPORTED:
                self.why.certainty = Certainty.UNKNOWN
                if "No direct quotes found in evidence" not in self.why.missing_info:
                    self.why.missing_info.append("No direct quotes found in evidence")

        # If no evidence at all, status should be proposed
        if not self.evidence:
            if self.status == Status.ACCEPTED:
                self.status = Status.PROPOSED


def generate_record_id(timestamp: datetime, domain: Domain, title: str) -> str:
    """Generate a unique ID for a decision record"""
    date_str = timestamp.strftime("%Y-%m-%d")
    # Create slug from title (first 3 words, lowercase, underscored)
    words = title.lower().split()[:3]
    slug = "_".join(w for w in words if w.isalnum() or w.replace("_", "").isalnum())
    return f"dec_{date_str}_{domain.value}_{slug}"
