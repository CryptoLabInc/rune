"""
Searcher

Searches organizational memory via enVector.
Returns Decision Records with their payload.text for synthesis.
"""

import json
from typing import List, Dict, Any, Optional
from dataclasses import dataclass, field

from ..common.envector_client import EnVectorClient
from ..common.embedding_service import EmbeddingService
from ..common.config import RuneConfig
from .query_processor import ParsedQuery, TimeScope


@dataclass
class SearchResult:
    """A single search result from enVector"""
    record_id: str
    title: str
    payload_text: str  # The key output for synthesis
    domain: str
    certainty: str
    status: str
    score: float
    metadata: Dict[str, Any] = field(default_factory=dict)

    @property
    def is_reliable(self) -> bool:
        """Check if result has reliable evidence"""
        return self.certainty in ("supported", "partially_supported")

    @property
    def summary(self) -> str:
        """Short summary for display"""
        return f"{self.title} ({self.domain}, {self.certainty})"


class Searcher:
    """
    Searches organizational memory using enVector.

    Features:
    - Multi-query expansion for better recall
    - Time filtering
    - Deduplication
    - Result ranking
    """

    def __init__(
        self,
        envector_client: EnVectorClient,
        embedding_service: EmbeddingService,
        config: RuneConfig
    ):
        """
        Initialize searcher.

        Args:
            envector_client: EnVector client for vector search
            embedding_service: For embedding queries
            config: Rune configuration
        """
        self._client = envector_client
        self._embedding = embedding_service
        self._config = config

    def search(
        self,
        query: ParsedQuery,
        topk: Optional[int] = None
    ) -> List[SearchResult]:
        """
        Search for relevant Decision Records.

        Args:
            query: Parsed query from QueryProcessor
            topk: Number of results (default from config)

        Returns:
            List of SearchResult objects sorted by relevance
        """
        topk = topk or self._config.retriever.topk

        # Search with multiple query expansions for better recall
        all_results = []
        seen_ids = set()

        for expanded_query in query.expanded_queries[:3]:  # Limit expansions
            results = self._search_single(expanded_query, topk)

            for result in results:
                if result.record_id not in seen_ids:
                    seen_ids.add(result.record_id)
                    all_results.append(result)

        # Also search with original query
        if query.original not in query.expanded_queries:
            results = self._search_single(query.original, topk)
            for result in results:
                if result.record_id not in seen_ids:
                    seen_ids.add(result.record_id)
                    all_results.append(result)

        # Sort by score
        all_results.sort(key=lambda r: r.score, reverse=True)

        # Apply time filtering if specified
        if query.time_scope != TimeScope.ALL_TIME:
            all_results = self._filter_by_time(all_results, query.time_scope)

        # Return top results
        return all_results[:topk]

    def _search_single(self, query_text: str, topk: int) -> List[SearchResult]:
        """Execute a single search query"""
        try:
            # Search enVector
            raw_result = self._client.search_with_text(
                index_name=self._config.envector.collection,
                query_text=query_text,
                embedding_service=self._embedding,
                topk=topk
            )

            if not raw_result.get("ok"):
                print(f"[Searcher] Search failed: {raw_result.get('error')}")
                return []

            # Parse results
            parsed = self._client.parse_search_results(raw_result)
            return [self._to_search_result(r) for r in parsed]

        except Exception as e:
            print(f"[Searcher] Search error: {e}")
            return []

    def _to_search_result(self, raw: Dict[str, Any]) -> SearchResult:
        """Convert raw result to SearchResult"""
        metadata = raw.get("metadata", {})

        # Extract fields from metadata (Decision Record structure)
        record_id = metadata.get("id", raw.get("id", "unknown"))
        title = metadata.get("title", "Untitled")
        domain = metadata.get("domain", "general")
        status = metadata.get("status", "unknown")

        # Extract certainty from nested 'why' field
        why = metadata.get("why", {})
        if isinstance(why, dict):
            certainty = why.get("certainty", "unknown")
        else:
            certainty = "unknown"

        # Extract payload.text - this is key for synthesis
        payload = metadata.get("payload", {})
        if isinstance(payload, dict):
            payload_text = payload.get("text", "")
        else:
            payload_text = metadata.get("text", raw.get("text", ""))

        # If no payload.text, fall back to decision text
        if not payload_text:
            decision = metadata.get("decision", {})
            if isinstance(decision, dict):
                payload_text = decision.get("what", "")

        return SearchResult(
            record_id=record_id,
            title=title,
            payload_text=payload_text,
            domain=domain,
            certainty=certainty,
            status=status,
            score=raw.get("score", 0.0),
            metadata=metadata,
        )

    def _filter_by_time(
        self,
        results: List[SearchResult],
        time_scope: TimeScope
    ) -> List[SearchResult]:
        """Filter results by time scope"""
        from datetime import datetime, timedelta, timezone

        now = datetime.now(timezone.utc)

        # Define time ranges
        time_ranges = {
            TimeScope.LAST_WEEK: timedelta(days=7),
            TimeScope.LAST_MONTH: timedelta(days=30),
            TimeScope.LAST_QUARTER: timedelta(days=90),
            TimeScope.LAST_YEAR: timedelta(days=365),
        }

        if time_scope not in time_ranges:
            return results

        cutoff = now - time_ranges[time_scope]
        filtered = []

        for result in results:
            # Try to parse timestamp from metadata
            timestamp_str = result.metadata.get("timestamp")
            if timestamp_str:
                try:
                    # Handle ISO format
                    ts = datetime.fromisoformat(timestamp_str.replace("Z", "+00:00"))
                    if ts.replace(tzinfo=None) >= cutoff:
                        filtered.append(result)
                except (ValueError, TypeError):
                    # If can't parse, include it
                    filtered.append(result)
            else:
                # No timestamp, include it
                filtered.append(result)

        return filtered

    def search_by_id(self, record_id: str) -> Optional[SearchResult]:
        """
        Search for a specific record by ID.

        Useful for retrieving full context of a referenced record.
        """
        # Search with the ID as query (will match in payload.text)
        results = self._search_single(f"ID: {record_id}", topk=5)

        # Find exact match
        for result in results:
            if result.record_id == record_id:
                return result

        return None

    def get_related(
        self,
        record_id: str,
        topk: int = 5
    ) -> List[SearchResult]:
        """
        Find records related to a given record.

        Useful for "See also" suggestions.
        """
        # First get the record
        record = self.search_by_id(record_id)
        if not record:
            return []

        # Search using its payload.text as query
        results = self._search_single(record.payload_text[:500], topk + 1)

        # Exclude the original record
        return [r for r in results if r.record_id != record_id][:topk]
