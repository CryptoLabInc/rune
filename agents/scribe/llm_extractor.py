"""
LLM-based Field Extractor

Extracts structured decision record fields from non-English text using LLM.
All outputs are translated to English for embedding consistency.

Supports phase-aware extraction: long reasoning processes (>800 chars) are
automatically split into logical phases, each becoming a linked DecisionRecord.
"""

import json
import logging
from dataclasses import dataclass, field
from typing import List, Optional

logger = logging.getLogger("rune.scribe.llm_extractor")

# Texts longer than this threshold trigger multi-phase extraction
PHASE_SPLIT_THRESHOLD = 800

# Texts longer than this, or with many detail items, trigger bundle split
BUNDLE_SPLIT_THRESHOLD = 1500


@dataclass
class ExtractedFields:
    """Fields extracted by LLM from non-English text"""
    title: str = ""
    rationale: str = ""
    problem: str = ""
    alternatives: List[str] = field(default_factory=list)
    trade_offs: List[str] = field(default_factory=list)
    status_hint: str = ""       # "proposed" | "accepted" | "rejected"
    tags: List[str] = field(default_factory=list)


@dataclass
class PhaseExtractedFields:
    """Fields for a single phase within a multi-phase reasoning chain"""
    phase_title: str = ""
    phase_decision: str = ""
    phase_rationale: str = ""
    phase_problem: str = ""
    alternatives: List[str] = field(default_factory=list)
    trade_offs: List[str] = field(default_factory=list)
    tags: List[str] = field(default_factory=list)


@dataclass
class ExtractionResult:
    """Result of LLM extraction — may be single, multi-phase, or bundle"""
    group_title: str = ""
    group_type: str = ""  # "phase_chain", "bundle", or "" (single)
    status_hint: str = ""
    tags: List[str] = field(default_factory=list)
    single: Optional[ExtractedFields] = None
    phases: Optional[List[PhaseExtractedFields]] = None

    @property
    def is_multi_phase(self) -> bool:
        return self.phases is not None and len(self.phases) > 1

    @property
    def is_bundle(self) -> bool:
        return self.group_type == "bundle" and self.phases is not None and len(self.phases) > 1


EXTRACTION_PROMPT = """You are a structured information extractor for organizational decision records.

Given a message (which may be in any language), extract the following fields.
IMPORTANT: All output values MUST be in English (translate if needed).

Respond with a valid JSON object with these keys:
- "title": A short title for the decision (5-60 chars, in English)
- "rationale": The reasoning behind the decision (in English, empty string if not found)
- "problem": The problem being solved (in English, empty string if not found)
- "alternatives": List of alternatives considered (in English, empty list if none)
- "trade_offs": List of trade-offs mentioned (in English, empty list if none)
- "status_hint": One of "proposed", "accepted", "rejected" based on the tone/language
- "tags": List of relevant topic tags (in English, e.g. ["database", "migration"])

Rules:
- Translate ALL values to English
- Keep the title concise and descriptive
- If a field is not clearly present in the text, use empty string or empty list
- For status_hint: use "accepted" if the message indicates a finalized decision, "proposed" if tentative, "rejected" if something was decided against

Message to extract from:
{text}

JSON:"""


PHASE_EXTRACTION_PROMPT = """You are a structured information extractor for organizational decision records.

Given a long message containing a multi-part reasoning process (which may be in any language), split it into LOGICAL PHASES and extract structured information for each phase.

IMPORTANT:
- Split by LOGICAL REASONING PHASES, not by paragraph or character count.
- Each phase should represent a distinct sub-decision, conclusion, or reasoning step.
- All output values MUST be in English (translate if needed).
- Aim for 2-5 phases. Do not create more than 7 phases.
- If the text is actually a single decision (not multi-phase), return a single phase.

Respond with a valid JSON object:
{{
    "group_title": "Overall title for the entire reasoning chain (5-60 chars, English)",
    "status_hint": "proposed" or "accepted" or "rejected",
    "tags": ["relevant", "topic", "tags"],
    "phases": [
        {{
            "phase_title": "Short title for this phase (e.g., 'Target Market Analysis')",
            "phase_decision": "The key decision or conclusion of this phase",
            "phase_rationale": "Why this conclusion was reached",
            "phase_problem": "The sub-problem this phase addresses",
            "alternatives": ["alternatives considered in this phase"],
            "trade_offs": ["trade-offs for this phase"],
            "tags": ["phase-specific tags"]
        }}
    ]
}}

Rules:
- Translate ALL values to English
- Each phase_decision should be self-contained and meaningful on its own
- phase_title should indicate the topic/aspect (e.g., "Positioning Strategy", "Pricing Model", "Go-to-Market Timeline")

Message to extract from:
{text}

JSON:"""


BUNDLE_SPLIT_PROMPT = """You are a structured information extractor for organizational decision records.

Given a message describing a SINGLE decision with rich details (which may be in any language), split it into a CORE record plus DETAIL FACETS. This is NOT about sequential reasoning — it's about organizing the supporting material of one decision.

IMPORTANT:
- The first item MUST be the "Core Decision" — a concise summary of the main decision.
- Subsequent items are detail facets: alternatives analysis, trade-offs, implementation plan, rationale deep-dive, etc.
- All output values MUST be in English (translate if needed).
- Aim for 2-4 facets total (including core). Do not create more than 5.
- Each facet should be self-contained and meaningful on its own.

Respond with a valid JSON object:
{{
    "group_title": "Overall title for the decision (5-60 chars, English)",
    "status_hint": "proposed" or "accepted" or "rejected",
    "tags": ["relevant", "topic", "tags"],
    "phases": [
        {{
            "phase_title": "Core Decision",
            "phase_decision": "The main decision statement — concise",
            "phase_rationale": "Brief summary of why",
            "phase_problem": "The problem being solved",
            "alternatives": [],
            "trade_offs": [],
            "tags": []
        }},
        {{
            "phase_title": "Alternatives Analysis",
            "phase_decision": "Detailed comparison of alternatives considered",
            "phase_rationale": "Why the chosen option was selected over others",
            "phase_problem": "",
            "alternatives": ["alt1", "alt2", "alt3"],
            "trade_offs": ["trade-off for each"],
            "tags": []
        }}
    ]
}}

Rules:
- Translate ALL values to English
- First facet is always "Core Decision" with the essential what/why
- Other facets organize the supporting detail (alternatives, trade-offs, implementation, evidence, etc.)
- Each phase_decision should be self-contained — readable without other facets

Message to extract from:
{text}

JSON:"""


class LLMExtractor:
    """Extracts structured fields from text using Claude API.

    Supports phase-aware extraction for long reasoning chains.
    """

    def __init__(
        self,
        anthropic_api_key: Optional[str] = None,
        model: str = "claude-sonnet-4-20250514",
    ):
        self._api_key = anthropic_api_key
        self._model = model
        self._client = None

        if anthropic_api_key:
            try:
                import anthropic
                self._client = anthropic.Anthropic(api_key=anthropic_api_key)
            except ImportError:
                logger.warning("anthropic package not installed")
            except Exception as e:
                logger.warning("Failed to init Anthropic client: %s", e)

    @property
    def is_available(self) -> bool:
        """Check if LLM client is ready"""
        return self._client is not None

    def extract(self, text: str) -> ExtractionResult:
        """Extract structured fields, auto-detecting split strategy.

        Split strategy:
        - Short text (<=800 chars): single extraction, then bundle check
        - Long text (>800 chars): phase extraction first, bundle fallback

        Args:
            text: Input text (any language)

        Returns:
            ExtractionResult (check .is_multi_phase or .is_bundle)
        """
        if not self.is_available:
            return ExtractionResult(single=ExtractedFields())

        if len(text) <= PHASE_SPLIT_THRESHOLD:
            fields = self._extract_single(text)
            result = ExtractionResult(
                group_title=fields.title,
                status_hint=fields.status_hint,
                tags=fields.tags,
                single=fields,
            )
            # Check if even short text has overflow details
            if self._needs_bundle_split(text, fields):
                try:
                    return self._extract_bundle(text)
                except Exception as e:
                    logger.warning("Bundle extraction failed for short text: %s", e)
            return result

        # Long text: try phase extraction first
        try:
            result = self._extract_phases(text)
            if result.is_multi_phase:
                result.group_type = "phase_chain"
                return result
            # Phase returned single — check if bundle needed
            if self._needs_bundle_split(text, result.single):
                try:
                    return self._extract_bundle(text)
                except Exception as e:
                    logger.warning("Bundle extraction failed after phase: %s", e)
            return result
        except Exception as e:
            logger.warning("Phase extraction failed: %s", e)
            # Try bundle before falling back to single
            if len(text) > BUNDLE_SPLIT_THRESHOLD:
                try:
                    return self._extract_bundle(text)
                except Exception as e2:
                    logger.warning("Bundle extraction also failed: %s", e2)
            fields = self._extract_single(text)
            return ExtractionResult(
                group_title=fields.title,
                status_hint=fields.status_hint,
                tags=fields.tags,
                single=fields,
            )

    def extract_single(self, text: str) -> ExtractedFields:
        """Extract as single record (backward-compatible entry point)."""
        if not self.is_available:
            return ExtractedFields()
        return self._extract_single(text)

    def _extract_single(self, text: str) -> ExtractedFields:
        """Single-phase extraction (original logic)."""
        try:
            prompt = EXTRACTION_PROMPT.format(text=text)
            response = self._client.messages.create(
                model=self._model,
                max_tokens=512,
                messages=[{"role": "user", "content": prompt}],
            )
            raw = response.content[0].text.strip()
            return self._parse_single_response(raw)
        except Exception as e:
            logger.warning("Single extraction failed: %s", e)
            return ExtractedFields()

    def _extract_phases(self, text: str) -> ExtractionResult:
        """Multi-phase extraction for long reasoning chains."""
        prompt = PHASE_EXTRACTION_PROMPT.format(text=text)
        response = self._client.messages.create(
            model=self._model,
            max_tokens=2048,
            messages=[{"role": "user", "content": prompt}],
        )
        raw = response.content[0].text.strip()
        data = self._parse_json(raw)

        phases_data = data.get("phases", [])
        group_title = str(data.get("group_title", ""))[:60]
        status_hint = str(data.get("status_hint", "")).lower()
        tags = [str(t).lower() for t in data.get("tags", []) if t]

        # If LLM returned 0 or 1 phase, treat as single
        if len(phases_data) <= 1:
            p = phases_data[0] if phases_data else {}
            return ExtractionResult(
                group_title=group_title,
                status_hint=status_hint,
                tags=tags,
                single=ExtractedFields(
                    title=str(p.get("phase_title", group_title))[:60],
                    rationale=str(p.get("phase_rationale", "")),
                    problem=str(p.get("phase_problem", "")),
                    alternatives=[str(a) for a in p.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                    status_hint=status_hint,
                    tags=tags,
                ),
            )

        # Multi-phase
        phases = []
        for p in phases_data[:7]:  # cap at 7
            phases.append(PhaseExtractedFields(
                phase_title=str(p.get("phase_title", ""))[:60],
                phase_decision=str(p.get("phase_decision", "")),
                phase_rationale=str(p.get("phase_rationale", "")),
                phase_problem=str(p.get("phase_problem", "")),
                alternatives=[str(a) for a in p.get("alternatives", []) if a],
                trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                tags=[str(t).lower() for t in p.get("tags", []) if t],
            ))

        return ExtractionResult(
            group_title=group_title,
            status_hint=status_hint,
            tags=tags,
            phases=phases,
        )

    def _needs_bundle_split(self, text: str, fields: Optional[ExtractedFields]) -> bool:
        """Check if content has detail overflow that warrants bundle splitting."""
        # Long text that exceeds single record capacity
        if len(text) > BUNDLE_SPLIT_THRESHOLD:
            return True
        # Moderate text with many detail items in multiple categories
        if fields and len(fields.alternatives) > 3 and len(fields.trade_offs) > 3:
            return True
        return False

    def _extract_bundle(self, text: str) -> ExtractionResult:
        """Bundle extraction: split a single decision into detail facets."""
        prompt = BUNDLE_SPLIT_PROMPT.format(text=text)
        response = self._client.messages.create(
            model=self._model,
            max_tokens=2048,
            messages=[{"role": "user", "content": prompt}],
        )
        raw = response.content[0].text.strip()
        data = self._parse_json(raw)

        phases_data = data.get("phases", [])
        group_title = str(data.get("group_title", ""))[:60]
        status_hint = str(data.get("status_hint", "")).lower()
        tags = [str(t).lower() for t in data.get("tags", []) if t]

        # If LLM returned 0 or 1 facet, not a real bundle
        if len(phases_data) <= 1:
            p = phases_data[0] if phases_data else {}
            return ExtractionResult(
                group_title=group_title,
                status_hint=status_hint,
                tags=tags,
                single=ExtractedFields(
                    title=str(p.get("phase_title", group_title))[:60],
                    rationale=str(p.get("phase_rationale", "")),
                    problem=str(p.get("phase_problem", "")),
                    alternatives=[str(a) for a in p.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                    status_hint=status_hint,
                    tags=tags,
                ),
            )

        # Multi-facet bundle — reuse PhaseExtractedFields structure
        phases = []
        for p in phases_data[:5]:  # cap at 5 facets
            phases.append(PhaseExtractedFields(
                phase_title=str(p.get("phase_title", ""))[:60],
                phase_decision=str(p.get("phase_decision", "")),
                phase_rationale=str(p.get("phase_rationale", "")),
                phase_problem=str(p.get("phase_problem", "")),
                alternatives=[str(a) for a in p.get("alternatives", []) if a],
                trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                tags=[str(t).lower() for t in p.get("tags", []) if t],
            ))

        return ExtractionResult(
            group_title=group_title,
            group_type="bundle",
            status_hint=status_hint,
            tags=tags,
            phases=phases,
        )

    def _parse_json(self, raw: str) -> dict:
        """Parse JSON from LLM response, handling code fences."""
        if raw.startswith("```"):
            lines = raw.split("\n")
            lines = [l for l in lines if not l.strip().startswith("```")]
            raw = "\n".join(lines)

        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            start = raw.find("{")
            end = raw.rfind("}") + 1
            if start >= 0 and end > start:
                try:
                    return json.loads(raw[start:end])
                except json.JSONDecodeError:
                    pass
        return {}

    def _parse_single_response(self, raw: str) -> ExtractedFields:
        """Parse LLM JSON response into ExtractedFields."""
        data = self._parse_json(raw)
        if not data:
            return ExtractedFields()

        return ExtractedFields(
            title=str(data.get("title", ""))[:60],
            rationale=str(data.get("rationale", "")),
            problem=str(data.get("problem", "")),
            alternatives=[str(a) for a in data.get("alternatives", []) if a],
            trade_offs=[str(t) for t in data.get("trade_offs", []) if t],
            status_hint=str(data.get("status_hint", "")).lower(),
            tags=[str(t).lower() for t in data.get("tags", []) if t],
        )
