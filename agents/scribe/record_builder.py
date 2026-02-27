"""
Record Builder

Builds Decision Records from raw events and detection results.
Core component of the Scribe agent's Stage 2 pipeline.

Key Rules:
- Evidence without quotes → certainty = "unknown"
- No evidence → status = "proposed"
- Always generate payload.text for embedding
"""

import re
from datetime import datetime, timezone
from typing import Optional, List, Dict, Any
from dataclasses import dataclass

from ..common.schemas import (
    DecisionRecord,
    DecisionDetail,
    Context,
    Why,
    Evidence,
    SourceRef,
    Quality,
    Payload,
    Domain,
    Sensitivity,
    Status,
    Certainty,
    ReviewState,
    SourceType,
    generate_record_id,
    generate_group_id,
)
from ..common.schemas.templates import render_payload_text
from ..common.language import LanguageInfo
from .detector import DetectionResult
from .llm_extractor import LLMExtractor, ExtractionResult


@dataclass
class RawEvent:
    """Raw event from a source (Slack, GitHub, etc.)"""
    text: str
    user: str
    channel: str
    timestamp: str
    source: str  # "slack", "github", "notion", etc.
    thread_ts: Optional[str] = None
    url: Optional[str] = None
    additional_context: Optional[Dict[str, Any]] = None


class RecordBuilder:
    """
    Builds Decision Records from raw events.

    Pipeline:
    1. Extract decision details from text
    2. Extract evidence (quotes) from text
    3. Determine certainty based on evidence
    4. Generate payload.text for embedding

    Rules enforced:
    - certainty cannot be "supported" without evidence quotes
    - status is "proposed" if no evidence
    - PII/credentials are redacted
    """

    # Patterns for extracting quotes from text
    QUOTE_PATTERNS = [
        r'"([^"]{10,})"',  # Double quotes
        r"'([^']{10,})'",  # Single quotes
        r'「([^」]{10,})」',  # Japanese quotes
        r'«([^»]{10,})»',  # French quotes
    ]

    # Patterns for extracting rationale
    RATIONALE_PATTERNS = [
        r'because\s+(.{10,}?)(?:\.|$)',
        r'reason(?:ing)?(?:\s+is)?[:\s]+(.{10,}?)(?:\.|$)',
        r'rationale[:\s]+(.{10,}?)(?:\.|$)',
        r'since\s+(.{10,}?)(?:\.|$)',
        r'due to\s+(.{10,}?)(?:\.|$)',
    ]

    # Patterns for sensitive data to redact
    SENSITIVE_PATTERNS = [
        (r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b', '[EMAIL]'),  # Email
        (r'\b\d{3}[-.]?\d{3}[-.]?\d{4}\b', '[PHONE]'),  # Phone
        (r'\b(?:sk|pk|api|key|token|secret|password)[_-][a-zA-Z0-9_-]{15,}\b', '[API_KEY]'),  # API keys with prefix
        (r'\b[A-Za-z0-9]{32,}\b', '[API_KEY]'),  # Long alphanumeric tokens (32+ chars)
        (r'\b[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}\b', '[CARD]'),  # Credit card
    ]

    def __init__(
        self,
        default_sensitivity: Sensitivity = Sensitivity.INTERNAL,
        llm_extractor: Optional[LLMExtractor] = None,
    ):
        """
        Initialize record builder.

        Args:
            default_sensitivity: Default sensitivity when unclear
            llm_extractor: Optional LLM extractor for non-English text
        """
        self._default_sensitivity = default_sensitivity
        self._llm_extractor = llm_extractor

    def build(
        self,
        raw_event: RawEvent,
        detection: DetectionResult,
        language: Optional[LanguageInfo] = None,
    ) -> DecisionRecord:
        """
        Build a Decision Record from raw event and detection result.

        Args:
            raw_event: Raw event data
            detection: Detection result from DecisionDetector
            language: Optional detected language info

        Returns:
            Complete DecisionRecord with payload.text
        """
        # Redact sensitive data
        clean_text, redaction_notes = self._redact_sensitive(raw_event.text)

        if self._llm_extractor and self._llm_extractor.is_available:
            # ===== LLM extraction (preferred for all languages) =====
            # Robust to typos, abbreviations, colloquialisms
            extracted = self._llm_extractor.extract_single(clean_text)
            title = extracted.title or self._extract_title(clean_text, detection)
            rationale = extracted.rationale
            problem = extracted.problem
            alternatives = extracted.alternatives
            trade_offs = extracted.trade_offs
            tags = extracted.tags
            evidence = self._extract_evidence(raw_event, clean_text)
            certainty, missing_info = self._determine_certainty(evidence, rationale)
            status = self._status_from_hint(extracted.status_hint, evidence, clean_text)
            decision_detail = self._extract_decision_detail(raw_event, clean_text)
            context = Context(
                problem=problem,
                alternatives=alternatives[:5],
                trade_offs=trade_offs[:5],
            )
        else:
            # ===== Fallback: regex extraction (when LLM unavailable) =====
            title = self._extract_title(clean_text, detection)
            decision_detail = self._extract_decision_detail(raw_event, clean_text)
            context = self._extract_context(clean_text)
            evidence = self._extract_evidence(raw_event, clean_text)
            rationale = self._extract_rationale(clean_text)
            certainty, missing_info = self._determine_certainty(evidence, rationale)
            status = self._determine_status(evidence, clean_text)
            tags = None  # will be extracted below

        # Determine domain
        domain = self._parse_domain(detection.domain)

        # Generate ID
        timestamp = datetime.now(timezone.utc)
        record_id = generate_record_id(timestamp, domain, title)

        # Build the record
        record = DecisionRecord(
            id=record_id,
            domain=domain,
            sensitivity=self._default_sensitivity,
            status=status,
            timestamp=timestamp,
            title=title,
            decision=decision_detail,
            context=context,
            why=Why(
                rationale_summary=rationale,
                certainty=certainty,
                missing_info=missing_info,
            ),
            evidence=evidence,
            tags=tags if tags is not None else self._extract_tags(clean_text, detection),
            quality=Quality(
                scribe_confidence=detection.confidence,
                review_state=ReviewState.UNREVIEWED,
                review_notes=redaction_notes if redaction_notes else None,
            ),
            payload=Payload(format="markdown", text=""),
        )

        # Ensure consistency
        record.ensure_evidence_certainty_consistency()

        # Generate payload.text
        record.payload.text = render_payload_text(record)

        return record

    def build_phases(
        self,
        raw_event: RawEvent,
        detection: DetectionResult,
        language: Optional[LanguageInfo] = None,
    ) -> List[DecisionRecord]:
        """
        Build one or more Decision Records, splitting into phases if needed.

        For short texts or when LLM is unavailable, returns a single-element list
        (delegating to build()). For long reasoning chains, splits into linked
        phase records sharing a group_id.

        Args:
            raw_event: Raw event data
            detection: Detection result from DecisionDetector
            language: Optional detected language info

        Returns:
            List of DecisionRecords (1 for single, 2-7 for phase chain)
        """
        # Without LLM, fall back to single record
        if not self._llm_extractor or not self._llm_extractor.is_available:
            return [self.build(raw_event, detection, language)]

        clean_text, redaction_notes = self._redact_sensitive(raw_event.text)

        # Phase-aware extraction (auto-detects short vs long)
        extraction: ExtractionResult = self._llm_extractor.extract(clean_text)

        if not extraction.is_multi_phase:
            # Single record — use the single extraction result
            fields = extraction.single
            if fields is None:
                return [self.build(raw_event, detection, language)]

            title = fields.title or self._extract_title(clean_text, detection)
            evidence = self._extract_evidence(raw_event, clean_text)
            certainty, missing_info = self._determine_certainty(evidence, fields.rationale)
            status = self._status_from_hint(fields.status_hint, evidence, clean_text)
            domain = self._parse_domain(detection.domain)
            timestamp = datetime.now(timezone.utc)
            record_id = generate_record_id(timestamp, domain, title)

            record = DecisionRecord(
                id=record_id,
                domain=domain,
                sensitivity=self._default_sensitivity,
                status=status,
                timestamp=timestamp,
                title=title,
                decision=self._extract_decision_detail(raw_event, clean_text),
                context=Context(
                    problem=fields.problem,
                    alternatives=fields.alternatives[:5],
                    trade_offs=fields.trade_offs[:5],
                ),
                why=Why(
                    rationale_summary=fields.rationale,
                    certainty=certainty,
                    missing_info=missing_info,
                ),
                evidence=evidence,
                tags=fields.tags or self._extract_tags(clean_text, detection),
                quality=Quality(
                    scribe_confidence=detection.confidence,
                    review_state=ReviewState.UNREVIEWED,
                    review_notes=redaction_notes if redaction_notes else None,
                ),
                payload=Payload(format="markdown", text=""),
            )
            record.ensure_evidence_certainty_consistency()
            record.payload.text = render_payload_text(record)
            return [record]

        # ===== Multi-phase: build linked records =====
        phases = extraction.phases
        domain = self._parse_domain(detection.domain)
        timestamp = datetime.now(timezone.utc)
        group_title = extraction.group_title or self._extract_title(clean_text, detection)
        group_id = generate_group_id(timestamp, domain, group_title)
        phase_total = len(phases)

        records: List[DecisionRecord] = []
        for seq, phase in enumerate(phases):
            phase_title = phase.phase_title or f"Phase {seq + 1}"
            record_id = generate_record_id(timestamp, domain, phase_title) + f"_p{seq}"

            # Parse timestamp for decision detail
            when = ""
            if raw_event.timestamp:
                try:
                    ts = float(raw_event.timestamp)
                    when = datetime.fromtimestamp(ts).strftime("%Y-%m-%d")
                except (ValueError, TypeError):
                    when = raw_event.timestamp

            decision_detail = DecisionDetail(
                what=phase.phase_decision[:500],
                who=[f"user:{raw_event.user}"] if raw_event.user else [],
                where=f"{raw_event.source}:{raw_event.channel}" if raw_event.channel else raw_event.source,
                when=when,
            )

            evidence = self._extract_evidence(raw_event, clean_text)
            certainty, missing_info = self._determine_certainty(evidence, phase.phase_rationale)
            status = self._status_from_hint(extraction.status_hint, evidence, clean_text)

            record = DecisionRecord(
                id=record_id,
                domain=domain,
                sensitivity=self._default_sensitivity,
                status=status,
                timestamp=timestamp,
                title=phase_title,
                decision=decision_detail,
                context=Context(
                    problem=phase.phase_problem,
                    alternatives=phase.alternatives[:5],
                    trade_offs=phase.trade_offs[:5],
                ),
                why=Why(
                    rationale_summary=phase.phase_rationale,
                    certainty=certainty,
                    missing_info=missing_info,
                ),
                evidence=evidence,
                tags=phase.tags or extraction.tags or self._extract_tags(clean_text, detection),
                quality=Quality(
                    scribe_confidence=detection.confidence,
                    review_state=ReviewState.UNREVIEWED,
                    review_notes=redaction_notes if redaction_notes else None,
                ),
                payload=Payload(format="markdown", text=""),
                # Phase chain fields
                group_id=group_id,
                phase_seq=seq,
                phase_total=phase_total,
            )
            record.ensure_evidence_certainty_consistency()
            record.payload.text = render_payload_text(record)
            records.append(record)

        return records

    def _redact_sensitive(self, text: str) -> tuple[str, Optional[str]]:
        """Redact sensitive data from text"""
        redacted = text
        redactions = []

        for pattern, replacement in self.SENSITIVE_PATTERNS:
            matches = re.findall(pattern, redacted, re.IGNORECASE)
            if matches:
                redacted = re.sub(pattern, replacement, redacted, flags=re.IGNORECASE)
                redactions.append(f"Redacted {len(matches)} {replacement}")

        notes = "; ".join(redactions) if redactions else None
        return redacted, notes

    def _extract_title(self, text: str, detection: DetectionResult) -> str:
        """Extract a short title from text"""
        # Try to find a decision statement
        title_patterns = [
            r'(?:decided|chose|going with|adopting)\s+(.{5,50}?)(?:\.|,|because)',
            r'decision[:\s]+(.{5,50}?)(?:\.|$)',
        ]

        for pattern in title_patterns:
            match = re.search(pattern, text, re.IGNORECASE)
            if match:
                return match.group(1).strip()

        # Fall back to first sentence or category
        first_sentence = text.split('.')[0][:60]
        if len(first_sentence) > 10:
            return first_sentence.strip()

        return f"{detection.category or 'General'} decision"

    def _extract_decision_detail(self, raw_event: RawEvent, text: str) -> DecisionDetail:
        """Extract decision details"""
        # Parse timestamp
        when = ""
        if raw_event.timestamp:
            try:
                ts = float(raw_event.timestamp)
                when = datetime.fromtimestamp(ts).strftime("%Y-%m-%d")
            except (ValueError, TypeError):
                when = raw_event.timestamp

        return DecisionDetail(
            what=text[:500],  # Limit length
            who=[f"user:{raw_event.user}"] if raw_event.user else [],
            where=f"{raw_event.source}:{raw_event.channel}" if raw_event.channel else raw_event.source,
            when=when,
        )

    def _extract_context(self, text: str) -> Context:
        """Extract context from text"""
        # Try to find problem statement
        problem = ""
        problem_patterns = [
            r'(?:problem|issue|challenge)[:\s]+(.{10,200}?)(?:\.|$)',
            r'(?:because|since)\s+(.{10,200}?)(?:,|we)',
        ]
        for pattern in problem_patterns:
            match = re.search(pattern, text, re.IGNORECASE)
            if match:
                problem = match.group(1).strip()
                break

        # Try to find alternatives
        alternatives = []
        alt_patterns = [
            r'(?:alternatives?|options?|considered)[:\s]+(.{10,200}?)(?:\.|$)',
            r'(?:instead of|over|rather than)\s+(\w+(?:\s+\w+){0,3})',
        ]
        for pattern in alt_patterns:
            matches = re.findall(pattern, text, re.IGNORECASE)
            alternatives.extend([m.strip() for m in matches if len(m.strip()) > 2])

        # Try to find trade-offs
        trade_offs = []
        tradeoff_patterns = [
            r'(?:trade-?off|downside|con)[:\s]+(.{10,100}?)(?:\.|$)',
            r'(?:but|however)\s+(.{10,100}?)(?:\.|$)',
        ]
        for pattern in tradeoff_patterns:
            matches = re.findall(pattern, text, re.IGNORECASE)
            trade_offs.extend([m.strip() for m in matches if len(m.strip()) > 5])

        return Context(
            problem=problem,
            alternatives=alternatives[:5],  # Limit
            trade_offs=trade_offs[:5],
        )

    def _extract_evidence(self, raw_event: RawEvent, text: str) -> List[Evidence]:
        """Extract evidence with quotes from text"""
        evidence = []

        # Find quotes in text
        for pattern in self.QUOTE_PATTERNS:
            matches = re.findall(pattern, text)
            for quote in matches:
                if len(quote) >= 10:
                    evidence.append(Evidence(
                        claim="Quoted statement from discussion",
                        quote=quote[:200],  # Limit quote length
                        source=SourceRef(
                            type=self._parse_source_type(raw_event.source),
                            url=raw_event.url,
                            pointer=f"channel:{raw_event.channel}" if raw_event.channel else None,
                        ),
                    ))

        # If no quotes found, create evidence from the text itself
        # but mark it as needing verification
        if not evidence and len(text) >= 20:
            # Use the text as a paraphrase, not a quote
            evidence.append(Evidence(
                claim="Decision statement (paraphrased)",
                quote=text[:150] + "..." if len(text) > 150 else text,
                source=SourceRef(
                    type=self._parse_source_type(raw_event.source),
                    url=raw_event.url,
                    pointer=f"channel:{raw_event.channel}" if raw_event.channel else None,
                ),
            ))

        return evidence[:3]  # Limit to 3 pieces of evidence

    def _extract_rationale(self, text: str) -> str:
        """Extract rationale/reasoning from text"""
        for pattern in self.RATIONALE_PATTERNS:
            match = re.search(pattern, text, re.IGNORECASE)
            if match:
                return match.group(1).strip()

        # If no explicit rationale, return empty
        return ""

    def _determine_certainty(
        self,
        evidence: List[Evidence],
        rationale: str
    ) -> tuple[Certainty, List[str]]:
        """
        Determine certainty level based on evidence.

        Rules:
        - No evidence → unknown
        - Evidence without direct quotes → partially_supported
        - Evidence with direct quotes → supported (if rationale also present)
        """
        missing_info = []

        if not evidence:
            missing_info.append("No evidence found")
            return Certainty.UNKNOWN, missing_info

        # Check if evidence has actual quotes (not paraphrases)
        has_direct_quotes = any(
            "paraphrase" not in e.claim.lower()
            for e in evidence
        )

        if not has_direct_quotes:
            missing_info.append("No direct quotes - evidence is paraphrased")
            return Certainty.PARTIALLY_SUPPORTED, missing_info

        if not rationale:
            missing_info.append("Explicit rationale not found")
            return Certainty.PARTIALLY_SUPPORTED, missing_info

        return Certainty.SUPPORTED, missing_info

    def _determine_status(self, evidence: List[Evidence], text: str) -> Status:
        """
        Determine decision status.

        Rules:
        - No evidence → proposed
        - Explicit acceptance markers → accepted
        - Default → proposed (conservative)
        """
        if not evidence:
            return Status.PROPOSED

        # Look for acceptance markers
        acceptance_patterns = [
            r'\b(?:approved|accepted|confirmed|finalized|agreed|decided)\b',
            r'\b(?:final decision|it\'s decided|we\'re going with)\b',
        ]

        text_lower = text.lower()
        for pattern in acceptance_patterns:
            if re.search(pattern, text_lower):
                return Status.ACCEPTED

        # Default to proposed (conservative)
        return Status.PROPOSED

    def _status_from_hint(
        self,
        hint: str,
        evidence: List[Evidence],
        text: str,
    ) -> Status:
        """Determine status from LLM-provided hint with fallback to rules."""
        hint_lower = hint.lower().strip()
        if hint_lower == "accepted":
            return Status.ACCEPTED
        if hint_lower == "rejected":
            return Status.PROPOSED  # Rejected proposals are still proposals, not superseded
        if hint_lower == "proposed":
            return Status.PROPOSED
        # Fallback to regex-based detection
        return self._determine_status(evidence, text)

    def _parse_domain(self, domain_str: Optional[str]) -> Domain:
        """Parse domain string to Domain enum"""
        if not domain_str:
            return Domain.GENERAL

        domain_lower = domain_str.lower()

        # Map string to enum
        domain_map = {
            "architecture": Domain.ARCHITECTURE,
            "security": Domain.SECURITY,
            "product": Domain.PRODUCT,
            "exec": Domain.EXEC,
            "ops": Domain.OPS,
            "design": Domain.DESIGN,
            "data": Domain.DATA,
            "hr": Domain.HR,
            "marketing": Domain.MARKETING,
            "incident": Domain.INCIDENT,
            "debugging": Domain.DEBUGGING,
            "qa": Domain.QA,
            "legal": Domain.LEGAL,
            "finance": Domain.FINANCE,
            "sales": Domain.SALES,
            "customer_success": Domain.CUSTOMER_SUCCESS,
            "customer_escalation": Domain.CUSTOMER_SUCCESS,
            "research": Domain.RESEARCH,
            "risk": Domain.RISK,
        }

        for key, value in domain_map.items():
            if key in domain_lower:
                return value

        return Domain.GENERAL

    def _parse_source_type(self, source: str) -> SourceType:
        """Parse source string to SourceType enum"""
        source_lower = source.lower()

        if "slack" in source_lower:
            return SourceType.SLACK
        if "github" in source_lower:
            return SourceType.GITHUB
        if "notion" in source_lower:
            return SourceType.NOTION
        if "meeting" in source_lower:
            return SourceType.MEETING
        if "email" in source_lower:
            return SourceType.EMAIL
        if "doc" in source_lower:
            return SourceType.DOC

        return SourceType.OTHER

    def _extract_tags(self, text: str, detection: DetectionResult) -> List[str]:
        """Extract relevant tags"""
        tags = []

        # Add domain as tag
        if detection.domain:
            tags.append(detection.domain)

        # Add category as tag
        if detection.category and detection.category != detection.domain:
            tags.append(detection.category.replace("_", "-"))

        # Extract hashtags if present
        hashtags = re.findall(r'#(\w+)', text)
        tags.extend(hashtags[:5])

        # Common keywords as tags
        keywords = [
            "microservices", "monolith", "database", "api", "security",
            "performance", "scalability", "migration", "refactor",
            "deprecation", "compliance", "gdpr", "sso", "auth",
        ]
        text_lower = text.lower()
        for kw in keywords:
            if kw in text_lower and kw not in tags:
                tags.append(kw)

        return list(set(tags))[:10]  # Unique, max 10
