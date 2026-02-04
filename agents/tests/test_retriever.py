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
    def mock_config(self):
        config = Mock()
        config.retriever.topk = 10
        config.envector.collection = "test-collection"
        return config

    @pytest.fixture
    def searcher(self, mock_client, mock_embedding, mock_config):
        from agents.retriever.searcher import Searcher
        return Searcher(mock_client, mock_embedding, mock_config)

    def test_search_returns_results(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = searcher.search(query)

        assert len(results) > 0
        assert results[0].record_id == "dec_2024-01-01_arch_postgres"

    def test_search_result_has_payload_text(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = searcher.search(query)

        assert results[0].payload_text != ""
        assert "Decision Record" in results[0].payload_text

    def test_search_result_has_certainty(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = searcher.search(query)

        assert results[0].certainty == "supported"

    def test_is_reliable_check(self, searcher):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        query = processor.parse("Why PostgreSQL?")

        results = searcher.search(query)

        # Supported certainty should be reliable
        assert results[0].is_reliable is True


class TestSynthesizer:
    """Tests for Synthesizer"""

    @pytest.fixture
    def synthesizer_no_llm(self):
        from agents.retriever.synthesizer import Synthesizer
        return Synthesizer(anthropic_api_key=None)

    @pytest.fixture
    def sample_results(self):
        from agents.retriever.searcher import SearchResult

        return [
            SearchResult(
                record_id="dec_2024-01-01_arch_postgres",
                title="Adopt PostgreSQL",
                payload_text="# Decision Record: Adopt PostgreSQL\n\nWe chose PostgreSQL for better JSON support.",
                domain="architecture",
                certainty="supported",
                status="accepted",
                score=0.85,
            ),
            SearchResult(
                record_id="dec_2024-01-02_arch_redis",
                title="Use Redis for caching",
                payload_text="# Decision Record: Use Redis\n\nRedis for caching due to performance.",
                domain="architecture",
                certainty="partially_supported",
                status="accepted",
                score=0.75,
            ),
        ]

    @pytest.fixture
    def sample_query(self):
        from agents.retriever.query_processor import QueryProcessor

        processor = QueryProcessor()
        return processor.parse("Why did we choose PostgreSQL?")

    def test_has_llm_property(self, synthesizer_no_llm):
        assert synthesizer_no_llm.has_llm is False

    def test_synthesize_no_results(self, synthesizer_no_llm, sample_query):
        result = synthesizer_no_llm.synthesize(sample_query, [])

        assert "No relevant records found" in result.answer
        assert result.confidence == 0.0
        assert len(result.sources) == 0

    def test_synthesize_fallback(self, synthesizer_no_llm, sample_query, sample_results):
        result = synthesizer_no_llm.synthesize(sample_query, sample_results)

        # Should use fallback formatting
        assert "Search Results" in result.answer or "PostgreSQL" in result.answer
        assert len(result.sources) > 0
        assert any("LLM not available" in w for w in result.warnings)

    def test_confidence_calculation(self, synthesizer_no_llm, sample_query, sample_results):
        result = synthesizer_no_llm.synthesize(sample_query, sample_results)

        # Should have some confidence based on results
        assert result.confidence > 0.0
        assert result.confidence <= 1.0

    def test_sources_extraction(self, synthesizer_no_llm, sample_query, sample_results):
        result = synthesizer_no_llm.synthesize(sample_query, sample_results)

        assert len(result.sources) == 2
        assert result.sources[0]["record_id"] == "dec_2024-01-01_arch_postgres"
        assert result.sources[0]["certainty"] == "supported"

    def test_warnings_for_uncertain_evidence(self, synthesizer_no_llm, sample_query):
        from agents.retriever.searcher import SearchResult

        uncertain_results = [
            SearchResult(
                record_id="dec_1",
                title="Uncertain decision",
                payload_text="Some decision",
                domain="general",
                certainty="unknown",
                status="proposed",
                score=0.7,
            ),
        ]

        result = synthesizer_no_llm.synthesize(sample_query, uncertain_results)

        # Should have warning about uncertain/unknown evidence or LLM not available
        has_warning = any("uncertain" in w.lower() or "unknown" in w.lower() or "LLM" in w for w in result.warnings)
        assert has_warning

    def test_related_queries_suggestion(self, synthesizer_no_llm, sample_query, sample_results):
        result = synthesizer_no_llm.synthesize(sample_query, sample_results)

        assert len(result.related_queries) > 0


class TestFormatAnswerForDisplay:
    """Tests for answer formatting"""

    def test_format_complete_answer(self):
        from agents.retriever.synthesizer import SynthesizedAnswer, format_answer_for_display

        answer = SynthesizedAnswer(
            answer="PostgreSQL was chosen for better JSON support.",
            confidence=0.85,
            sources=[
                {"record_id": "dec_1", "title": "PostgreSQL Decision", "certainty": "supported"}
            ],
            related_queries=["What alternatives were considered?"],
            warnings=["1 record(s) have partial evidence"],
        )

        formatted = format_answer_for_display(answer)

        assert "PostgreSQL was chosen" in formatted
        assert "85%" in formatted
        assert "dec_1" in formatted
        assert "alternatives" in formatted.lower()
