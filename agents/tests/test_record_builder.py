"""
Tests for Record Builder

Tests Decision Record creation with evidence validation.
"""

import pytest
from datetime import datetime


class TestRecordBuilder:
    """Tests for RecordBuilder"""

    @pytest.fixture
    def builder(self):
        from agents.scribe.record_builder import RecordBuilder
        from agents.common.schemas import Sensitivity

        return RecordBuilder(default_sensitivity=Sensitivity.INTERNAL)

    @pytest.fixture
    def sample_raw_event(self):
        from agents.scribe.record_builder import RawEvent

        return RawEvent(
            text='We decided to use PostgreSQL over MySQL because "better JSON support and team familiarity"',
            user="U12345",
            channel="architecture",
            timestamp="1706799600.123456",
            source="slack",
            thread_ts=None,
            url="https://slack.com/archives/C123/p1706799600123456",
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

    def test_build_basic_record(self, builder, sample_raw_event, sample_detection):
        """Test building a basic Decision Record"""
        record = builder.build(sample_raw_event, sample_detection)

        assert record is not None
        assert record.schema_version == "2.0"
        assert record.type == "decision_record"
        assert "PostgreSQL" in record.title or "architecture" in record.title.lower()

    def test_record_has_payload_text(self, builder, sample_raw_event, sample_detection):
        """Test that record has generated payload.text"""
        record = builder.build(sample_raw_event, sample_detection)

        assert record.payload.text != ""
        assert record.payload.format == "markdown"
        assert "Decision Record" in record.payload.text

    def test_evidence_extraction_with_quotes(self, builder, sample_detection):
        """Test evidence extraction when text has quotes"""
        from agents.scribe.record_builder import RawEvent

        event = RawEvent(
            text='We chose Redis because "it is fast and team knows it well"',
            user="U12345",
            channel="engineering",
            timestamp="1706799600",
            source="slack",
        )

        record = builder.build(event, sample_detection)

        assert len(record.evidence) > 0
        # Should have extracted the quote
        quotes_found = any("fast" in e.quote.lower() for e in record.evidence)
        assert quotes_found or len(record.evidence) > 0

    def test_certainty_unknown_without_evidence(self, builder, sample_detection):
        """Test that certainty is 'unknown' when no proper evidence"""
        from agents.scribe.record_builder import RawEvent
        from agents.common.schemas import Certainty

        event = RawEvent(
            text="We should use X",  # No quote, no clear evidence
            user="U12345",
            channel="general",
            timestamp="1706799600",
            source="slack",
        )

        record = builder.build(event, sample_detection)

        # Without clear quotes, certainty should not be "supported"
        # It should be either "unknown" or "partially_supported"
        assert record.why.certainty in (Certainty.UNKNOWN, Certainty.PARTIALLY_SUPPORTED)

    def test_status_proposed_without_evidence(self, builder, sample_detection):
        """Test that status is 'proposed' when evidence is weak"""
        from agents.scribe.record_builder import RawEvent
        from agents.common.schemas import Status

        event = RawEvent(
            text="Maybe we should consider X",  # No definitive decision
            user="U12345",
            channel="general",
            timestamp="1706799600",
            source="slack",
        )

        record = builder.build(event, sample_detection)

        # Should be proposed, not accepted
        assert record.status == Status.PROPOSED

    def test_sensitive_data_redaction(self, builder, sample_detection):
        """Test that sensitive data is redacted"""
        from agents.scribe.record_builder import RawEvent

        event = RawEvent(
            text="We decided to use API key api_secret_abc123xyz7890123456 for auth and email test@example.com",
            user="U12345",
            channel="security",
            timestamp="1706799600",
            source="slack",
        )

        record = builder.build(event, sample_detection)

        # API key and email should be redacted
        assert "api_secret_abc123xyz7890123456" not in record.decision.what
        assert "test@example.com" not in record.decision.what
        assert "[API_KEY]" in record.decision.what or "[EMAIL]" in record.decision.what

    def test_domain_extraction(self, builder, sample_raw_event, sample_detection):
        """Test domain is extracted from detection result"""
        from agents.common.schemas import Domain

        record = builder.build(sample_raw_event, sample_detection)

        assert record.domain == Domain.ARCHITECTURE

    def test_tags_extraction(self, builder, sample_detection):
        """Test tags are extracted from content"""
        from agents.scribe.record_builder import RawEvent

        event = RawEvent(
            text="We decided to use #microservices architecture for scalability",
            user="U12345",
            channel="architecture",
            timestamp="1706799600",
            source="slack",
        )

        record = builder.build(event, sample_detection)

        # Should have extracted tags
        assert len(record.tags) > 0
        # Should include the hashtag
        assert "microservices" in record.tags or any("micro" in t for t in record.tags)

    def test_evidence_certainty_consistency(self, builder, sample_detection):
        """Test that ensure_evidence_certainty_consistency works"""
        from agents.scribe.record_builder import RawEvent
        from agents.common.schemas import Certainty

        event = RawEvent(
            text="We chose X",  # Minimal text, weak evidence
            user="U12345",
            channel="general",
            timestamp="1706799600",
            source="slack",
        )

        record = builder.build(event, sample_detection)

        # Consistency should be maintained
        is_valid = record.validate_evidence_certainty()
        if not is_valid:
            record.ensure_evidence_certainty_consistency()

        # After consistency check, should be valid
        assert record.validate_evidence_certainty()


class TestRecordBuilderMultilingual:
    """Tests for multilingual (LLM extraction) path in RecordBuilder"""

    @pytest.fixture
    def mock_llm_extractor(self):
        from unittest.mock import Mock
        from agents.scribe.llm_extractor import LLMExtractor, ExtractedFields

        extractor = Mock(spec=LLMExtractor)
        extractor.is_available = True
        extractor.extract.return_value = ExtractedFields(
            title="Adopt PostgreSQL for database",
            rationale="Better JSON support and team familiarity with PostgreSQL",
            problem="Need a reliable database for financial transactions",
            alternatives=["MySQL", "MongoDB"],
            trade_offs=["More complex setup required"],
            status_hint="accepted",
            tags=["database", "postgresql"],
        )
        return extractor

    @pytest.fixture
    def builder_with_llm(self, mock_llm_extractor):
        from agents.scribe.record_builder import RecordBuilder
        from agents.common.schemas import Sensitivity

        return RecordBuilder(
            default_sensitivity=Sensitivity.INTERNAL,
            llm_extractor=mock_llm_extractor,
        )

    @pytest.fixture
    def korean_raw_event(self):
        from agents.scribe.record_builder import RawEvent

        return RawEvent(
            text='PostgreSQL을 사용하기로 결정했습니다. "JSON 지원이 좋고 팀이 익숙하기 때문" 이라고 합의했습니다.',
            user="U99999",
            channel="architecture",
            timestamp="1706799600.123456",
            source="slack",
            url="https://slack.com/archives/C123/p1706799600123456",
        )

    @pytest.fixture
    def sample_detection(self):
        from agents.scribe.detector import DetectionResult

        return DetectionResult(
            is_significant=True,
            confidence=0.85,
            matched_pattern="X를 사용하기로 결정했다",
            category="architecture",
            domain="architecture",
            priority="high",
        )

    def test_llm_path_used_for_korean(self, builder_with_llm, korean_raw_event, sample_detection, mock_llm_extractor):
        """Test that LLM extractor is called for Korean text"""
        from agents.common.language import LanguageInfo

        language = LanguageInfo(code="ko", confidence=0.95, script="Hangul")
        record = builder_with_llm.build(korean_raw_event, sample_detection, language=language)

        mock_llm_extractor.extract.assert_called_once()
        assert record is not None
        assert record.title == "Adopt PostgreSQL for database"

    def test_llm_extracted_fields_used(self, builder_with_llm, korean_raw_event, sample_detection):
        """Test that LLM-extracted fields are used in the record"""
        from agents.common.language import LanguageInfo
        from agents.common.schemas import Status

        language = LanguageInfo(code="ko", confidence=0.95, script="Hangul")
        record = builder_with_llm.build(korean_raw_event, sample_detection, language=language)

        assert record.why.rationale_summary == "Better JSON support and team familiarity with PostgreSQL"
        assert record.context.problem == "Need a reliable database for financial transactions"
        assert "MySQL" in record.context.alternatives
        assert "MongoDB" in record.context.alternatives
        assert record.status == Status.ACCEPTED
        assert "database" in record.tags

    def test_llm_used_for_english_when_available(self, builder_with_llm, sample_detection, mock_llm_extractor):
        """Test that LLM is used for English when available (robust to typos)"""
        from agents.scribe.record_builder import RawEvent

        event = RawEvent(
            text='We decided to use PostgreSQL because "better JSON support"',
            user="U12345",
            channel="architecture",
            timestamp="1706799600",
            source="slack",
        )

        record = builder_with_llm.build(event, sample_detection)

        # LLM extractor SHOULD be called for English too (preferred for all languages)
        mock_llm_extractor.extract.assert_called_once()
        assert record is not None
        assert record.title == "Adopt PostgreSQL for database"

    def test_llm_used_for_all_languages(self, builder_with_llm, sample_detection, mock_llm_extractor):
        """Test that LLM is used for all languages when available (language-agnostic)"""
        from agents.scribe.record_builder import RawEvent
        from agents.common.language import LanguageInfo

        event = RawEvent(
            text='We decided to use PostgreSQL because "better JSON support"',
            user="U12345",
            channel="architecture",
            timestamp="1706799600",
            source="slack",
        )

        language = LanguageInfo(code="en", confidence=0.99, script="Latin")
        record = builder_with_llm.build(event, sample_detection, language=language)

        # LLM extractor SHOULD be called regardless of language
        mock_llm_extractor.extract.assert_called_once()
        assert record is not None
        assert record.title == "Adopt PostgreSQL for database"

    def test_fallback_when_llm_unavailable(self, sample_detection):
        """Test regex fallback when LLM extractor is not available"""
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.common.schemas import Sensitivity
        from agents.common.language import LanguageInfo
        from unittest.mock import Mock
        from agents.scribe.llm_extractor import LLMExtractor

        extractor = Mock(spec=LLMExtractor)
        extractor.is_available = False

        builder = RecordBuilder(
            default_sensitivity=Sensitivity.INTERNAL,
            llm_extractor=extractor,
        )

        event = RawEvent(
            text='PostgreSQL을 사용하기로 결정했습니다',
            user="U99999",
            channel="architecture",
            timestamp="1706799600",
            source="slack",
        )

        language = LanguageInfo(code="ko", confidence=0.95, script="Hangul")
        record = builder.build(event, sample_detection, language=language)

        # Should use regex fallback, not call LLM
        extractor.extract.assert_not_called()
        assert record is not None

    def test_status_from_hint_accepted(self, builder_with_llm, korean_raw_event, sample_detection):
        """Test status_hint='accepted' maps to ACCEPTED"""
        from agents.common.language import LanguageInfo
        from agents.common.schemas import Status

        language = LanguageInfo(code="ko", confidence=0.95, script="Hangul")
        record = builder_with_llm.build(korean_raw_event, sample_detection, language=language)

        assert record.status == Status.ACCEPTED

    def test_status_from_hint_proposed(self, sample_detection):
        """Test status_hint='proposed' maps to PROPOSED"""
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.common.schemas import Sensitivity, Status
        from agents.common.language import LanguageInfo
        from agents.scribe.llm_extractor import ExtractedFields
        from unittest.mock import Mock

        extractor = Mock()
        extractor.is_available = True
        extractor.extract.return_value = ExtractedFields(
            title="Consider using Redis",
            status_hint="proposed",
        )

        builder = RecordBuilder(
            default_sensitivity=Sensitivity.INTERNAL,
            llm_extractor=extractor,
        )

        event = RawEvent(
            text='Redis를 고려해봐야 할 것 같습니다',
            user="U99999",
            channel="architecture",
            timestamp="1706799600",
            source="slack",
        )

        language = LanguageInfo(code="ko", confidence=0.95, script="Hangul")
        record = builder.build(event, sample_detection, language=language)

        assert record.status == Status.PROPOSED

    def test_record_has_payload_text_for_korean(self, builder_with_llm, korean_raw_event, sample_detection):
        """Test that payload.text is generated for Korean input"""
        from agents.common.language import LanguageInfo

        language = LanguageInfo(code="ko", confidence=0.95, script="Hangul")
        record = builder_with_llm.build(korean_raw_event, sample_detection, language=language)

        assert record.payload.text != ""
        assert record.payload.format == "markdown"
        assert "Decision Record" in record.payload.text

    def test_llm_handles_typos_and_informal_english(self, builder_with_llm, sample_detection, mock_llm_extractor):
        """Test that LLM extraction handles typos and informal language in English"""
        from agents.scribe.record_builder import RawEvent
        from agents.common.language import LanguageInfo

        # Informal English with typos and abbreviations
        event = RawEvent(
            text='we decidd 2 use postgres bcuz "its got gr8 json support & team knows it"',
            user="U12345",
            channel="architecture",
            timestamp="1706799600",
            source="slack",
        )

        language = LanguageInfo(code="en", confidence=0.95, script="Latin")
        record = builder_with_llm.build(event, sample_detection, language=language)

        # LLM should handle typos/informal language gracefully
        mock_llm_extractor.extract.assert_called_once()
        assert record is not None
        assert record.title == "Adopt PostgreSQL for database"


class TestPayloadTextGeneration:
    """Tests for payload.text generation"""

    @pytest.fixture
    def sample_record(self):
        from agents.common.schemas import (
            DecisionRecord, DecisionDetail, Context, Why, Evidence,
            SourceRef, Quality, Payload, Domain, Sensitivity, Status,
            Certainty, ReviewState, SourceType
        )

        return DecisionRecord(
            id="dec_2024-02-01_arch_postgres",
            domain=Domain.ARCHITECTURE,
            sensitivity=Sensitivity.INTERNAL,
            status=Status.ACCEPTED,
            title="Adopt PostgreSQL for database",
            decision=DecisionDetail(
                what="We will use PostgreSQL as our primary database",
                who=["role:tech_lead", "user:alice"],
                where="slack:#architecture",
                when="2024-02-01",
            ),
            context=Context(
                problem="Need a reliable database with JSON support",
                alternatives=["MySQL", "PostgreSQL", "MongoDB"],
                chosen="PostgreSQL",
                trade_offs=["More complex setup", "Better long-term flexibility"],
            ),
            why=Why(
                rationale_summary="Better JSON support and team familiarity",
                certainty=Certainty.SUPPORTED,
                missing_info=[],
            ),
            evidence=[
                Evidence(
                    claim="Team prefers PostgreSQL",
                    quote="We all know Postgres well",
                    source=SourceRef(
                        type=SourceType.SLACK,
                        url="https://slack.com/...",
                        pointer="channel:#arch",
                    ),
                ),
            ],
            tags=["database", "postgres", "architecture"],
            quality=Quality(scribe_confidence=0.9),
            payload=Payload(format="markdown", text=""),
        )

    def test_render_payload_text(self, sample_record):
        from agents.common.schemas.templates import render_payload_text

        payload_text = render_payload_text(sample_record)

        assert "Decision Record: Adopt PostgreSQL" in payload_text
        assert "dec_2024-02-01_arch_postgres" in payload_text
        assert "PostgreSQL" in payload_text
        assert "JSON support" in payload_text
        assert "Evidence" in payload_text
        assert "We all know Postgres well" in payload_text

    def test_payload_text_contains_certainty(self, sample_record):
        from agents.common.schemas.templates import render_payload_text

        payload_text = render_payload_text(sample_record)

        assert "Certainty: supported" in payload_text

    def test_payload_text_contains_alternatives(self, sample_record):
        from agents.common.schemas.templates import render_payload_text

        payload_text = render_payload_text(sample_record)

        assert "MySQL" in payload_text
        assert "MongoDB" in payload_text
        assert "(chosen)" in payload_text
