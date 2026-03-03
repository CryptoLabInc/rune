"""
Tests for Retriever Agent

Tests query processing, searching, and synthesis.
"""

import pytest
from unittest.mock import Mock, MagicMock


class TestQueryProcessor:
    """Tests for QueryProcessor"""

    @pytest.fixture
    def processor(self):
        from agents.retriever.query_processor import QueryProcessor
        return QueryProcessor()

    def test_parse_decision_rationale_query(self, processor):
        from agents.retriever.query_processor import QueryIntent

        result = processor.parse("Why did we choose PostgreSQL over MySQL?")

        assert result.intent == QueryIntent.DECISION_RATIONALE
        assert "PostgreSQL" in result.entities or "postgresql" in result.cleaned

    def test_parse_feature_history_query(self, processor):
        from agents.retriever.query_processor import QueryIntent

        result = processor.parse("Have customers asked for dark mode?")

        assert result.intent == QueryIntent.FEATURE_HISTORY

    def test_parse_pattern_lookup_query(self, processor):
        from agents.retriever.query_processor import QueryIntent

        result = processor.parse("How do we handle authentication?")

        assert result.intent == QueryIntent.PATTERN_LOOKUP

    def test_parse_technical_context_query(self, processor):
        from agents.retriever.query_processor import QueryIntent

        result = processor.parse("What's our architecture for the payment system?")

        assert result.intent == QueryIntent.TECHNICAL_CONTEXT

    def test_parse_general_query(self, processor):
        from agents.retriever.query_processor import QueryIntent

        result = processor.parse("Tell me about our database")

        assert result.intent == QueryIntent.GENERAL

    def test_time_scope_detection(self, processor):
        from agents.retriever.query_processor import TimeScope

        result = processor.parse("What decisions did we make last week?")
        assert result.time_scope == TimeScope.LAST_WEEK

        result = processor.parse("What happened in Q3?")
        assert result.time_scope == TimeScope.LAST_QUARTER

    def test_entity_extraction_quoted(self, processor):
        result = processor.parse('Why did we choose "React Native"?')

        assert "React Native" in result.entities

    def test_entity_extraction_capitalized(self, processor):
        result = processor.parse("Why did we use PostgreSQL instead of MySQL?")

        # Should extract capitalized tech names
        entities_lower = [e.lower() for e in result.entities]
        assert "postgresql" in entities_lower or "mysql" in entities_lower

    def test_keyword_extraction(self, processor):
        result = processor.parse("Why did we choose PostgreSQL for the database?")

        assert "postgresql" in result.keywords or "database" in result.keywords

    def test_query_expansion(self, processor):
        result = processor.parse("Why PostgreSQL?")

        assert len(result.expanded_queries) > 1
        # Original should be included
        assert any("postgresql" in q.lower() for q in result.expanded_queries)

    def test_format_for_search(self, processor):
        result = processor.parse("Why did we choose PostgreSQL?")
        formatted = processor.format_for_search(result)

        assert "postgresql" in formatted.lower()


class TestSearcher:
    """Tests for Searcher"""

    @pytest.fixture
    def mock_client(self):
        client = Mock()
        client.search_with_text.return_value = {
            "ok": True,
            "results": [
                {
                    "id": "1",
                    "distance": 0.2,
                    "metadata": {
                        "id": "dec_2024-01-01_arch_postgres",
                        "title": "Adopt PostgreSQL",
                        "domain": "architecture",
                        "why": {"certainty": "supported"},
                        "payload": {"text": "# Decision Record: Adopt PostgreSQL\n..."},
                    }
                }
            ]
        }
        client.parse_search_results.return_value = [
            {
                "id": "1",
                "score": 0.8,
                "metadata": {
                    "id": "dec_2024-01-01_arch_postgres",
                    "title": "Adopt PostgreSQL",
                    "domain": "architecture",
                    "why": {"certainty": "supported"},
                    "payload": {"text": "# Decision Record: Adopt PostgreSQL\n..."},
                }
            }
        ]
        return client

    @pytest.fixture
    def mock_embedding(self):
        embedding = Mock()
        embedding.embed_single.return_value = [0.1] * 384
        return embedding

    @pytest.fixture
    def searcher(self, mock_client, mock_embedding):
        from agents.retriever.searcher import Searcher
        return Searcher(mock_client, mock_embedding, "test-collection")

    @pytest.mark.asyncio
    async def test_search_returns_results(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = await searcher.search(query)

        assert len(results) > 0
        assert results[0].record_id == "dec_2024-01-01_arch_postgres"

    @pytest.mark.asyncio
    async def test_search_result_has_payload_text(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = await searcher.search(query)

        assert results[0].payload_text != ""
        assert "Decision Record" in results[0].payload_text

    @pytest.mark.asyncio
    async def test_search_result_has_certainty(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = await searcher.search(query)

        assert results[0].certainty == "supported"

    @pytest.mark.asyncio
    async def test_is_reliable_check(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = await searcher.search(query)

        # Supported certainty should be reliable
        assert results[0].is_reliable is True


class TestExpandPhaseChains:
    """Tests for phase chain expansion in Searcher"""

    @pytest.fixture
    def mock_client(self):
        from unittest.mock import AsyncMock
        client = Mock()
        client.search_with_text = Mock(return_value={"ok": True, "results": []})
        client.parse_search_results = Mock(return_value=[])
        return client

    @pytest.fixture
    def mock_embedding(self):
        embedding = Mock()
        embedding.embed_single.return_value = [0.1] * 384
        return embedding

    @pytest.fixture
    def searcher(self, mock_client, mock_embedding):
        from agents.retriever.searcher import Searcher
        return Searcher(mock_client, mock_embedding, "test-collection")

    def _make_result(self, record_id, group_id=None, group_type=None, phase_seq=None, phase_total=None, score=0.8):
        from agents.retriever.searcher import SearchResult
        return SearchResult(
            record_id=record_id,
            title=f"Title {record_id}",
            payload_text=f"Content of {record_id}",
            domain="architecture",
            certainty="supported",
            status="accepted",
            score=score,
            group_id=group_id,
            group_type=group_type,
            phase_seq=phase_seq,
            phase_total=phase_total,
        )

    @pytest.mark.asyncio
    async def test_expand_fetches_siblings(self, searcher):
        """Test that expansion fetches siblings for phase results"""
        grp = "grp_2026-01-01_arch_plg"
        results = [
            self._make_result("dec_p0", group_id=grp, group_type="phase_chain", phase_seq=0, phase_total=3),
        ]

        # Mock _search_single to return siblings
        sibling_results = [
            self._make_result("dec_p1", group_id=grp, group_type="phase_chain", phase_seq=1, phase_total=3),
            self._make_result("dec_p2", group_id=grp, group_type="phase_chain", phase_seq=2, phase_total=3),
        ]
        from unittest.mock import AsyncMock
        searcher._search_single = AsyncMock(return_value=sibling_results)

        expanded = await searcher._expand_phase_chains(results)

        # Should have fetched siblings
        searcher._search_single.assert_called_once()
        # Should contain siblings ordered by phase_seq
        assert len(expanded) == 2  # Only siblings (originals filtered by existing_ids)
        assert expanded[0].phase_seq == 1
        assert expanded[1].phase_seq == 2

    @pytest.mark.asyncio
    async def test_expand_orders_by_phase_seq(self, searcher):
        """Test that expanded results are ordered by phase_seq"""
        grp = "grp_2026-01-01_arch_test"
        results = [
            self._make_result("dec_p1", group_id=grp, group_type="phase_chain", phase_seq=1, phase_total=3),
        ]

        # Return siblings out of order
        from unittest.mock import AsyncMock
        searcher._search_single = AsyncMock(return_value=[
            self._make_result("dec_p2", group_id=grp, group_type="phase_chain", phase_seq=2, phase_total=3),
            self._make_result("dec_p0", group_id=grp, group_type="phase_chain", phase_seq=0, phase_total=3),
        ])

        expanded = await searcher._expand_phase_chains(results)

        # Siblings should be sorted by phase_seq
        seqs = [r.phase_seq for r in expanded]
        assert seqs == sorted(seqs)

    @pytest.mark.asyncio
    async def test_expand_no_duplicate_record_ids(self, searcher):
        """Test that expanded results have no duplicate record_ids"""
        grp = "grp_2026-01-01_arch_dedup"
        results = [
            self._make_result("dec_p0", group_id=grp, group_type="phase_chain", phase_seq=0, phase_total=2),
        ]

        # Sibling search returns the original + new one
        from unittest.mock import AsyncMock
        searcher._search_single = AsyncMock(return_value=[
            self._make_result("dec_p0", group_id=grp, group_type="phase_chain", phase_seq=0, phase_total=2),
            self._make_result("dec_p1", group_id=grp, group_type="phase_chain", phase_seq=1, phase_total=2),
        ])

        expanded = await searcher._expand_phase_chains(results)

        # No duplicates
        ids = [r.record_id for r in expanded]
        assert len(ids) == len(set(ids))

    @pytest.mark.asyncio
    async def test_expand_standalone_untouched(self, searcher):
        """Test that standalone (non-phase) results are passed through"""
        results = [
            self._make_result("dec_standalone", score=0.9),
        ]

        from unittest.mock import AsyncMock
        searcher._search_single = AsyncMock()

        expanded = await searcher._expand_phase_chains(results)

        # No search should be made for non-phase results
        searcher._search_single.assert_not_called()
        assert len(expanded) == 1
        assert expanded[0].record_id == "dec_standalone"




class TestQueryProcessorMultilingual:
    """Tests for multilingual query processing"""

    @pytest.fixture
    def processor_no_llm(self):
        """QueryProcessor without LLM (regex fallback for all languages)"""
        from agents.retriever.query_processor import QueryProcessor
        return QueryProcessor()

    @pytest.fixture
    def mock_llm_processor(self):
        """QueryProcessor with mocked LLM client"""
        from agents.retriever.query_processor import QueryProcessor
        import json

        processor = QueryProcessor()

        # Mock the LLMClient
        mock_llm = Mock()
        mock_llm.is_available = True
        mock_llm.generate.return_value = json.dumps({
            "intent": "decision_rationale",
            "english_query": "Why did we choose PostgreSQL?",
            "entities": ["PostgreSQL"],
            "keywords": ["choose", "database", "postgresql"],
            "time_scope": "all_time",
        })
        processor._llm = mock_llm

        return processor

    def test_korean_query_detected_as_non_english(self, processor_no_llm):
        """Test that Korean query gets language=ko in ParsedQuery"""
        result = processor_no_llm.parse("왜 PostgreSQL을 선택했나요?")

        assert result.language is not None
        assert result.language.code == "ko"

    def test_english_query_detected_as_english(self, processor_no_llm):
        """Test that English query gets language=en"""
        result = processor_no_llm.parse("Why did we choose PostgreSQL?")

        assert result.language is not None
        assert result.language.code == "en"

    def test_korean_query_falls_back_to_regex_without_llm(self, processor_no_llm):
        """Test that Korean query uses regex path when no LLM available"""
        result = processor_no_llm.parse("왜 PostgreSQL을 선택했나요?")

        # Should still return a valid ParsedQuery
        assert result.original == "왜 PostgreSQL을 선택했나요?"
        assert result.intent is not None

    def test_korean_query_uses_llm_when_available(self, mock_llm_processor):
        """Test that Korean query uses LLM for intent classification"""
        from agents.retriever.query_processor import QueryIntent

        result = mock_llm_processor.parse("왜 PostgreSQL을 선택했나요?")

        # LLM should be called
        mock_llm_processor._llm.generate.assert_called_once()

        assert result.intent == QueryIntent.DECISION_RATIONALE
        assert "PostgreSQL" in result.entities

    def test_multilingual_expanded_queries(self, mock_llm_processor):
        """Test that expanded_queries includes both original and English translation"""
        result = mock_llm_processor.parse("왜 PostgreSQL을 선택했나요?")

        # Should include original Korean query AND English translation
        assert any("PostgreSQL을" in q for q in result.expanded_queries)
        assert any("Why" in q or "choose" in q.lower() for q in result.expanded_queries)

    def test_english_query_skips_llm(self, mock_llm_processor):
        """Test that English query does NOT use LLM even when available"""
        result = mock_llm_processor.parse("Why did we choose PostgreSQL?")

        # LLM should NOT be called for English
        mock_llm_processor._llm.generate.assert_not_called()

    def test_llm_parse_failure_falls_back(self, mock_llm_processor):
        """Test that LLM failure falls back to regex parsing"""
        mock_llm_processor._llm.generate.side_effect = Exception("API error")

        result = mock_llm_processor.parse("왜 PostgreSQL을 선택했나요?")

        # Should still return a valid ParsedQuery (regex fallback)
        assert result is not None
        assert result.original == "왜 PostgreSQL을 선택했나요?"

    def test_parsed_query_language_field(self, processor_no_llm):
        """Test ParsedQuery.language field is populated"""
        result = processor_no_llm.parse("Tell me about our database")

        assert result.language is not None
        assert result.language.code == "en"
        assert result.language.is_english is True




class TestDisplayTextLocalization:
    """Tests for render_display_text localization"""

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
            title="Adopt PostgreSQL",
            decision=DecisionDetail(
                what="Use PostgreSQL as primary database",
                who=["user:alice"],
                where="slack:#architecture",
                when="2024-02-01",
            ),
            context=Context(problem="Need reliable database"),
            why=Why(
                rationale_summary="Better JSON support",
                certainty=Certainty.SUPPORTED,
            ),
            evidence=[],
            tags=["database"],
            quality=Quality(scribe_confidence=0.9),
            payload=Payload(format="markdown", text=""),
        )

    def test_render_display_text_english(self, sample_record):
        from agents.common.schemas.templates import render_display_text

        text = render_display_text(sample_record, language="en")

        assert "Decision Record" in text
        assert "Alternatives Considered" in text

    def test_render_display_text_korean(self, sample_record):
        from agents.common.schemas.templates import render_display_text

        text = render_display_text(sample_record, language="ko")

        assert "결정 기록" in text
        assert "검토한 대안" in text

    def test_render_display_text_japanese(self, sample_record):
        from agents.common.schemas.templates import render_display_text

        text = render_display_text(sample_record, language="ja")

        assert "決定記録" in text
        assert "検討した代替案" in text

    def test_render_display_text_unknown_lang_falls_back(self, sample_record):
        from agents.common.schemas.templates import render_display_text

        text = render_display_text(sample_record, language="fr")

        # Should fall back to English
        assert "Decision Record" in text

    def test_render_payload_text_always_english(self, sample_record):
        """Verify render_payload_text is always English (for embedding consistency)"""
        from agents.common.schemas.templates import render_payload_text

        text = render_payload_text(sample_record)

        assert "Decision Record" in text
        assert "Alternatives Considered" in text


