"""
Tests for Agent-Delegated Mode

Tests the pre_extraction path in RecordBuilder and the extracted parameter
in tool_capture (JSON → ExtractionResult conversion).
"""

import json
import pytest
from unittest.mock import Mock


class TestRecordBuilderPreExtraction:
    """Tests for RecordBuilder.build_phases with pre_extraction parameter"""

    @pytest.fixture
    def builder(self):
        from agents.scribe.record_builder import RecordBuilder
        from agents.common.schemas import Sensitivity

        return RecordBuilder(default_sensitivity=Sensitivity.INTERNAL)

    @pytest.fixture
    def sample_raw_event(self):
        from agents.scribe.record_builder import RawEvent

        return RawEvent(
            text='We decided to use PostgreSQL over MySQL because of better JSON support.',
            user="alice",
            channel="architecture",
            timestamp="1706799600.123456",
            source="claude_agent",
        )

    @pytest.fixture
    def sample_detection(self):
        from agents.scribe.detector import DetectionResult

        return DetectionResult(
            is_significant=True,
            confidence=0.85,
            matched_pattern="We decided to use",
            category="architecture",
            domain="architecture",
            priority="high",
        )

    def test_single_pre_extraction(self, builder, sample_raw_event, sample_detection):
        """Test single record from pre_extraction"""
        from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

        pre = ExtractionResult(
            group_title="Adopt PostgreSQL",
            status_hint="accepted",
            tags=["database", "postgresql"],
            confidence=0.90,
            single=ExtractedFields(
                title="Adopt PostgreSQL",
                rationale="Better JSON support and team familiarity",
                problem="Need reliable database with JSON support",
                alternatives=["MySQL", "MongoDB"],
                trade_offs=["Higher memory usage"],
                status_hint="accepted",
                tags=["database", "postgresql"],
            ),
        )

        records = builder.build_phases(sample_raw_event, sample_detection, pre_extraction=pre)

        assert len(records) == 1
        record = records[0]
        assert "PostgreSQL" in record.title
        assert record.quality.scribe_confidence == 0.90  # From pre_extraction
        assert record.payload.text != ""
        assert record.domain.value == "architecture"

    def test_multi_phase_pre_extraction(self, builder, sample_raw_event, sample_detection):
        """Test multi-phase records from pre_extraction"""
        from agents.scribe.llm_extractor import ExtractionResult, PhaseExtractedFields

        pre = ExtractionResult(
            group_title="Database Strategy",
            group_type="phase_chain",
            status_hint="accepted",
            tags=["database"],
            confidence=0.85,
            phases=[
                PhaseExtractedFields(
                    phase_title="Requirements Analysis",
                    phase_decision="Need ACID guarantees",
                    phase_rationale="Production workload requires consistency",
                    phase_problem="Current NoSQL limitations",
                ),
                PhaseExtractedFields(
                    phase_title="Technology Selection",
                    phase_decision="Adopt PostgreSQL",
                    phase_rationale="Best JSON support among RDBMS",
                    phase_problem="Need SQL + JSON support",
                    alternatives=["MySQL", "CockroachDB"],
                    trade_offs=["Higher memory usage"],
                    tags=["postgresql"],
                ),
            ],
        )

        records = builder.build_phases(sample_raw_event, sample_detection, pre_extraction=pre)

        assert len(records) == 2
        # All records share group_id
        assert records[0].group_id == records[1].group_id
        assert records[0].group_id is not None
        # Phase ordering
        assert records[0].phase_seq == 0
        assert records[1].phase_seq == 1
        assert records[0].phase_total == 2
        # Confidence from pre_extraction
        assert records[0].quality.scribe_confidence == 0.85
        assert records[1].quality.scribe_confidence == 0.85

    def test_bundle_pre_extraction(self, builder, sample_raw_event, sample_detection):
        """Test bundle records from pre_extraction"""
        from agents.scribe.llm_extractor import ExtractionResult, PhaseExtractedFields

        pre = ExtractionResult(
            group_title="Auth Strategy",
            group_type="bundle",
            status_hint="accepted",
            tags=["auth", "security"],
            confidence=0.92,
            phases=[
                PhaseExtractedFields(
                    phase_title="Core Decision",
                    phase_decision="Use JWT with refresh tokens",
                    phase_rationale="Stateless, scales with microservices",
                    phase_problem="Need auth for distributed system",
                ),
                PhaseExtractedFields(
                    phase_title="Alternatives Analysis",
                    phase_decision="Compared session-based, OAuth2, JWT",
                    phase_rationale="Sessions don't scale",
                    phase_problem="",
                    alternatives=["Session cookies", "OAuth2 server"],
                ),
            ],
        )

        records = builder.build_phases(sample_raw_event, sample_detection, pre_extraction=pre)

        assert len(records) == 2
        assert records[0].group_type == "bundle"
        assert records[1].group_type == "bundle"
        assert records[0].group_id == records[1].group_id

    def test_pre_extraction_without_confidence_uses_detection(self, builder, sample_raw_event, sample_detection):
        """Test that missing confidence falls back to detection.confidence"""
        from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

        pre = ExtractionResult(
            single=ExtractedFields(
                title="Test Decision",
                rationale="Test reason",
                status_hint="proposed",
            ),
        )

        records = builder.build_phases(sample_raw_event, sample_detection, pre_extraction=pre)

        assert len(records) == 1
        # Should use detection.confidence (0.85), not None
        assert records[0].quality.scribe_confidence == 0.85

    def test_no_pre_extraction_no_llm_falls_back(self, builder, sample_raw_event, sample_detection):
        """Test backward compat: no pre_extraction, no LLM → single record via build()"""
        records = builder.build_phases(sample_raw_event, sample_detection)

        assert len(records) == 1
        # Should still produce a valid record
        assert records[0].payload.text != ""


class TestExtractedJSONParsing:
    """Tests for parsing extracted JSON in the capture pipeline"""

    def _parse_extracted(self, json_str: str):
        """Helper: simulate the JSON→ExtractionResult conversion from server.py"""
        from agents.common.llm_utils import parse_llm_json
        from agents.scribe.llm_extractor import (
            ExtractionResult, ExtractedFields, PhaseExtractedFields,
        )

        data = parse_llm_json(json_str)
        if not data:
            return None, "Invalid JSON"

        # Tier 2 check
        tier2 = data.get("tier2", {})
        if not tier2.get("capture", True):
            return None, f"Rejected: {tier2.get('reason', 'no reason')}"

        agent_confidence = data.get("confidence")
        if isinstance(agent_confidence, (int, float)):
            agent_confidence = max(0.0, min(1.0, float(agent_confidence)))
        else:
            agent_confidence = None

        phases_data = data.get("phases")
        if phases_data and len(phases_data) > 1:
            phases = []
            for p in phases_data[:7]:
                phases.append(PhaseExtractedFields(
                    phase_title=str(p.get("phase_title", ""))[:60],
                    phase_decision=str(p.get("phase_decision", "")),
                    phase_rationale=str(p.get("phase_rationale", "")),
                    phase_problem=str(p.get("phase_problem", "")),
                    alternatives=[str(a) for a in p.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                    tags=[str(t).lower() for t in p.get("tags", []) if t],
                ))
            result = ExtractionResult(
                group_title=str(data.get("group_title", ""))[:60],
                group_type=str(data.get("group_type", "phase_chain")),
                status_hint=str(data.get("status_hint", "")).lower(),
                tags=[str(t).lower() for t in data.get("tags", []) if t],
                confidence=agent_confidence,
                phases=phases,
            )
        else:
            if phases_data and len(phases_data) == 1:
                p = phases_data[0]
                single = ExtractedFields(
                    title=str(p.get("phase_title", data.get("title", "")))[:60],
                    rationale=str(p.get("phase_rationale", data.get("rationale", ""))),
                    problem=str(p.get("phase_problem", data.get("problem", ""))),
                    alternatives=[str(a) for a in p.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                    status_hint=str(data.get("status_hint", "")).lower(),
                    tags=[str(t).lower() for t in p.get("tags", data.get("tags", [])) if t],
                )
            else:
                single = ExtractedFields(
                    title=str(data.get("title", ""))[:60],
                    rationale=str(data.get("rationale", "")),
                    problem=str(data.get("problem", "")),
                    alternatives=[str(a) for a in data.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in data.get("trade_offs", []) if t],
                    status_hint=str(data.get("status_hint", "")).lower(),
                    tags=[str(t).lower() for t in data.get("tags", []) if t],
                )
            result = ExtractionResult(
                group_title=single.title,
                status_hint=single.status_hint,
                tags=single.tags,
                confidence=agent_confidence,
                single=single,
            )

        return result, None

    def test_single_json_parsed(self):
        """Test single decision JSON parsing"""
        extracted = json.dumps({
            "tier2": {"capture": True, "reason": "Technology decision", "domain": "architecture"},
            "title": "Adopt PostgreSQL",
            "rationale": "Better JSON support",
            "problem": "Need reliable database",
            "alternatives": ["MongoDB", "MySQL"],
            "trade_offs": ["Higher operational cost"],
            "status_hint": "accepted",
            "tags": ["database", "postgresql"],
            "confidence": 0.85,
        })

        result, error = self._parse_extracted(extracted)

        assert error is None
        assert result is not None
        assert result.single is not None
        assert result.single.title == "Adopt PostgreSQL"
        assert result.confidence == 0.85
        assert not result.is_multi_phase

    def test_multi_phase_json_parsed(self):
        """Test multi-phase JSON parsing"""
        extracted = json.dumps({
            "tier2": {"capture": True, "reason": "Multi-step reasoning", "domain": "architecture"},
            "group_title": "Database Strategy",
            "group_type": "phase_chain",
            "status_hint": "accepted",
            "tags": ["database"],
            "confidence": 0.85,
            "phases": [
                {
                    "phase_title": "Requirements",
                    "phase_decision": "Need ACID",
                    "phase_rationale": "Production requires consistency",
                    "phase_problem": "NoSQL limitations",
                    "alternatives": [],
                    "trade_offs": [],
                    "tags": [],
                },
                {
                    "phase_title": "Selection",
                    "phase_decision": "Use PostgreSQL",
                    "phase_rationale": "Best JSON support",
                    "phase_problem": "Need SQL + JSON",
                    "alternatives": ["MySQL"],
                    "trade_offs": ["Memory"],
                    "tags": ["postgresql"],
                },
            ],
        })

        result, error = self._parse_extracted(extracted)

        assert error is None
        assert result.is_multi_phase
        assert len(result.phases) == 2
        assert result.group_type == "phase_chain"

    def test_bundle_json_parsed(self):
        """Test bundle JSON parsing"""
        extracted = json.dumps({
            "tier2": {"capture": True, "reason": "Rich decision", "domain": "security"},
            "group_title": "Auth Strategy",
            "group_type": "bundle",
            "status_hint": "accepted",
            "tags": ["auth"],
            "confidence": 0.90,
            "phases": [
                {
                    "phase_title": "Core Decision",
                    "phase_decision": "Use JWT",
                    "phase_rationale": "Stateless",
                    "phase_problem": "Auth needed",
                    "alternatives": [],
                    "trade_offs": [],
                    "tags": [],
                },
                {
                    "phase_title": "Alternatives",
                    "phase_decision": "Compared options",
                    "phase_rationale": "Sessions don't scale",
                    "phase_problem": "",
                    "alternatives": ["Sessions", "OAuth2"],
                    "trade_offs": ["JWT size"],
                    "tags": [],
                },
            ],
        })

        result, error = self._parse_extracted(extracted)

        assert error is None
        assert result.group_type == "bundle"
        assert len(result.phases) == 2

    def test_tier2_rejection(self):
        """Test tier2 capture=false is respected"""
        extracted = json.dumps({
            "tier2": {"capture": False, "reason": "Casual chat", "domain": "general"},
        })

        result, error = self._parse_extracted(extracted)

        assert result is None
        assert "Rejected" in error

    def test_invalid_json_returns_error(self):
        """Test invalid JSON string"""
        result, error = self._parse_extracted("not valid json at all {{{")

        assert result is None
        assert "Invalid JSON" in error

    def test_confidence_clamped(self):
        """Test confidence is clamped to 0.0-1.0"""
        extracted = json.dumps({
            "tier2": {"capture": True, "reason": "Test", "domain": "general"},
            "title": "Test",
            "confidence": 1.5,
        })

        result, error = self._parse_extracted(extracted)

        assert error is None
        assert result.confidence == 1.0

    def test_missing_confidence_is_none(self):
        """Test missing confidence defaults to None"""
        extracted = json.dumps({
            "tier2": {"capture": True, "reason": "Test", "domain": "general"},
            "title": "Test Decision",
        })

        result, error = self._parse_extracted(extracted)

        assert error is None
        assert result.confidence is None

    def test_agent_delegated_without_detector(self):
        """Agent-delegated mode should not require DecisionDetector."""
        from agents.scribe.detector import DetectionResult
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

        builder = RecordBuilder()
        raw = RawEvent(
            text="We decided to use PostgreSQL over MongoDB",
            user="dev", channel="eng", timestamp="1711000000", source="claude_agent",
        )
        # Construct DetectionResult from agent data, no PatternCache needed
        detection = DetectionResult(
            is_significant=True,
            confidence=0.85,
            domain="architecture",
            category="architecture",
        )
        pre_extraction = ExtractionResult(
            group_title="Use PostgreSQL over MongoDB",
            status_hint="accepted",
            tags=["database", "architecture"],
            confidence=0.85,
            single=ExtractedFields(
                title="Use PostgreSQL over MongoDB",
                rationale="Better ACID compliance for financial data",
                problem="Need reliable database for transactions",
                alternatives=["MongoDB"],
                trade_offs=["Less flexible schema"],
                status_hint="accepted",
                tags=["database"],
            ),
        )
        records = builder.build_phases(raw, detection, pre_extraction=pre_extraction)
        assert len(records) == 1
        assert records[0].domain.value == "architecture"
        assert records[0].quality.scribe_confidence == 0.85

    def test_single_phase_treated_as_single(self):
        """Test that phases with 1 element is treated as single record"""
        extracted = json.dumps({
            "tier2": {"capture": True, "reason": "Single", "domain": "architecture"},
            "group_title": "Single Phase",
            "phases": [
                {
                    "phase_title": "Only Phase",
                    "phase_decision": "Do X",
                    "phase_rationale": "Because Y",
                    "phase_problem": "Problem Z",
                },
            ],
            "confidence": 0.8,
        })

        result, error = self._parse_extracted(extracted)

        assert error is None
        assert not result.is_multi_phase
        assert result.single is not None
        assert result.single.title == "Only Phase"


def test_reusable_insight_used_for_embedding_text():
    """When reusable_insight is set, it should be the embedding target."""
    from agents.common.schemas import DecisionRecord, DecisionDetail, Payload
    from agents.common.schemas.embedding import embedding_text_for_record

    record = DecisionRecord(
        id="dec_test",
        title="Test",
        decision=DecisionDetail(what="Test"),
        reusable_insight="Dense gist paragraph for embedding.",
        payload=Payload(text="# Full markdown\n## Decision\nVerbose content"),
    )
    assert embedding_text_for_record(record) == "Dense gist paragraph for embedding."


def test_embedding_text_fallback_to_payload():
    """When reusable_insight is empty, fall back to payload.text."""
    from agents.common.schemas import DecisionRecord, DecisionDetail, Payload
    from agents.common.schemas.embedding import embedding_text_for_record

    record = DecisionRecord(
        id="dec_test",
        title="Test",
        decision=DecisionDetail(what="Test"),
        reusable_insight="",
        payload=Payload(text="Fallback payload text"),
    )
    assert embedding_text_for_record(record) == "Fallback payload text"


def test_reusable_insight_flows_to_record():
    """reusable_insight from agent JSON should appear on the built record."""
    from agents.scribe.detector import DetectionResult
    from agents.scribe.record_builder import RecordBuilder, RawEvent
    from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

    insight = "We chose PostgreSQL over MongoDB for ACID compliance in financial data."
    builder = RecordBuilder()
    raw = RawEvent(text="...", user="dev", channel="eng", timestamp="1711000000", source="claude_agent")
    detection = DetectionResult(is_significant=True, confidence=0.85, domain="architecture")
    pre_extraction = ExtractionResult(
        group_title="PostgreSQL selection",
        status_hint="accepted",
        tags=["database"],
        confidence=0.85,
        group_summary=insight,
        single=ExtractedFields(
            title="PostgreSQL selection",
            rationale="ACID compliance",
            status_hint="accepted",
            tags=["database"],
        ),
    )
    records = builder.build_phases(raw, detection, pre_extraction=pre_extraction)
    assert records[0].reusable_insight == insight


def test_single_record_json_reusable_insight_wiring():
    """reusable_insight from agent JSON must reach DecisionRecord in single-record path.

    Regression test: the server.py single-record path was missing group_summary,
    so reusable_insight was always empty for Format A captures.
    """
    import json
    from agents.common.llm_utils import parse_llm_json
    from agents.scribe.detector import DetectionResult
    from agents.scribe.record_builder import RecordBuilder, RawEvent
    from agents.scribe.llm_extractor import ExtractionResult, ExtractedFields

    # Simulate agent JSON (Format A — single decision, no phases)
    agent_json = {
        "tier2": {"capture": True, "reason": "Architecture decision", "domain": "architecture"},
        "title": "Adopt PostgreSQL",
        "reusable_insight": "We chose PostgreSQL over MongoDB because ACID compliance is critical for financial transaction data. MongoDB was rejected due to eventual consistency risks.",
        "rationale": "ACID compliance",
        "problem": "Need reliable database",
        "alternatives": ["MongoDB"],
        "trade_offs": ["Less flexible schema"],
        "status_hint": "accepted",
        "tags": ["database"],
        "confidence": 0.9,
    }
    data = agent_json

    # Reproduce server.py single-record path (no phases or 0 phases)
    single = ExtractedFields(
        title=str(data.get("title", ""))[:60],
        rationale=str(data.get("rationale", "")),
        problem=str(data.get("problem", "")),
        alternatives=[str(a) for a in data.get("alternatives", []) if a],
        trade_offs=[str(t) for t in data.get("trade_offs", []) if t],
        status_hint=str(data.get("status_hint", "")).lower(),
        tags=[str(t).lower() for t in data.get("tags", []) if t],
    )
    pre_extraction = ExtractionResult(
        group_title=single.title,
        group_summary=str(data.get("reusable_insight", "")) or "",
        status_hint=single.status_hint,
        tags=single.tags,
        confidence=0.9,
        single=single,
    )

    builder = RecordBuilder()
    raw = RawEvent(text="...", user="dev", channel="eng", timestamp="1711000000", source="claude_agent")
    detection = DetectionResult(is_significant=True, confidence=0.9, domain="architecture")

    records = builder.build_phases(raw, detection, pre_extraction=pre_extraction)
    assert records[0].reusable_insight == agent_json["reusable_insight"]
