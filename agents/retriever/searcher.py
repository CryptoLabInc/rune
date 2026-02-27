"""
Searcher

Searches organizational memory via enVector.
Uses the Vault-secured pipeline: scoring → decrypt → metadata.
Returns Decision Records with their payload.text for synthesis.
"""

import json
import logging
from typing import List, Dict, Any, Optional
from dataclasses import dataclass, field

from ..common.envector_client import EnVectorClient
from ..common.embedding_service import EmbeddingService
from .query_processor import ParsedQuery, TimeScope

logger = logging.getLogger("rune.retriever.searcher")


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
    # Group fields (phase_chain or bundle)
    group_id: Optional[str] = None
    group_type: Optional[str] = None
    phase_seq: Optional[int] = None
    phase_total: Optional[int] = None

    @property
    def is_reliable(self) -> bool:
        """Check if result has reliable evidence"""
        return self.certainty in ("supported", "partially_supported")

    @property
    def is_phase(self) -> bool:
        """Check if this result is part of a group (phase_chain or bundle)"""
        return self.group_id is not None

    @property
    def summary(self) -> str:
        """Short summary for display"""
        return f"{self.title} ({self.domain}, {self.certainty})"


class Searcher:
    """
    Searches organizational memory using enVector.

    Uses the Vault-secured pipeline (scoring → decrypt → metadata)
    when a vault_client is provided. Falls back to direct search otherwise.

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
        index_name: str,
        vault_client=None,
    ):
        """
        Initialize searcher.

        Args:
            envector_client: EnVector client for vector search
            embedding_service: For embedding queries
            index_name: enVector index name (provided by Vault)
            vault_client: VaultClient for Vault-secured search pipeline
        """
        self._client = envector_client
        self._embedding = embedding_service
        self._index_name = index_name
        self._vault = vault_client

    async def search(
        self,
        query: ParsedQuery,
        topk: Optional[int] = None,
        expand_phases: bool = True,
    ) -> List[SearchResult]:
        """
        Search for relevant Decision Records.

        Args:
            query: Parsed query from QueryProcessor
            topk: Number of results (default from config)
            expand_phases: If True, automatically fetch sibling phases
                when a phase chain member is found

        Returns:
            List of SearchResult objects sorted by relevance
        """
        topk = topk or 10

        # Search with multiple query expansions for better recall
        all_results = []
        seen_ids = set()

        for expanded_query in query.expanded_queries[:3]:  # Limit expansions
            results = await self._search_single(expanded_query, topk)

            for result in results:
                if result.record_id not in seen_ids:
                    seen_ids.add(result.record_id)
                    all_results.append(result)

        # Also search with original query
        if query.original not in query.expanded_queries:
            results = await self._search_single(query.original, topk)
            for result in results:
                if result.record_id not in seen_ids:
                    seen_ids.add(result.record_id)
                    all_results.append(result)

        # Sort by score
        all_results.sort(key=lambda r: r.score, reverse=True)

        # Apply time filtering if specified
        if query.time_scope != TimeScope.ALL_TIME:
            all_results = self._filter_by_time(all_results, query.time_scope)

        # Trim to topk before phase expansion
        all_results = all_results[:topk]

        # Expand phase chains: fetch sibling phases for any phase results
        if expand_phases:
            all_results = await self._expand_phase_chains(all_results)

        return all_results

    async def _search_single(self, query_text: str, topk: int) -> List[SearchResult]:
        """Execute a single search query via the appropriate pipeline."""
        if self._vault:
            return await self._search_via_vault(query_text, topk)
        return self._search_direct(query_text, topk)

    async def _search_via_vault(self, query_text: str, topk: int) -> List[SearchResult]:
        """
        Vault-secured search pipeline:
        1. Embed query → encrypted similarity scoring on enVector Cloud
        2. Vault decrypts result ciphertext, selects top-k
        3. Retrieve encrypted metadata from enVector Cloud
        4. Vault decrypts metadata
        """
        try:
            # Embed query
            query_vector = self._embedding.embed_single(query_text)

            # Step 1: encrypted similarity scoring → result ciphertext
            scoring_result = self._client.score(self._index_name, query_vector)
            if not scoring_result.get("ok"):
                logger.warning("Scoring failed: %s", scoring_result.get("error"))
                return []

            blobs = scoring_result.get("encrypted_blobs", [])
            if not blobs:
                return []

            # Step 2: Vault decrypts scores + top-k
            vault_result = await self._vault.decrypt_search_results(
                encrypted_blob_b64=blobs[0],
                top_k=topk,
            )
            if not vault_result.ok:
                logger.warning("Vault decrypt failed: %s", vault_result.error)
                return []

            if not vault_result.results:
                return []

            # Step 3: Retrieve encrypted metadata
            metadata_result = self._client.remind(
                self._index_name,
                vault_result.results,
                output_fields=["metadata"],
            )
            if not metadata_result.get("ok"):
                logger.warning("Metadata retrieval failed: %s", metadata_result.get("error"))
                return []

            # Step 4: Decrypt metadata via Vault
            encrypted_entries = metadata_result.get("results", [])
            encrypted_blobs = [entry.get("data", "") for entry in encrypted_entries]

            if encrypted_blobs and any(encrypted_blobs):
                non_empty = [(idx, b) for idx, b in enumerate(encrypted_blobs) if b]
                decrypted_metadata = await self._vault.decrypt_metadata(
                    encrypted_metadata_list=[b for _, b in non_empty]
                )
                for dec_idx, (entry_idx, _) in enumerate(non_empty):
                    if dec_idx < len(decrypted_metadata):
                        encrypted_entries[entry_idx]["metadata"] = decrypted_metadata[dec_idx]
                for entry in encrypted_entries:
                    entry.pop("data", None)

            return [self._to_search_result(r) for r in encrypted_entries]

        except Exception as e:
            logger.error("Vault search error: %s", e, exc_info=True)
            return []

    def _search_direct(self, query_text: str, topk: int) -> List[SearchResult]:
        """Fallback: direct search without Vault (for non-Vault deployments)."""
        try:
            raw_result = self._client.search_with_text(
                index_name=self._index_name,
                query_text=query_text,
                embedding_service=self._embedding,
                topk=topk
            )

            if not raw_result.get("ok"):
                logger.warning("Direct search failed: %s", raw_result.get("error"))
                return []

            parsed = self._client.parse_search_results(raw_result)
            return [self._to_search_result(r) for r in parsed]

        except Exception as e:
            logger.error("Direct search error: %s", e)
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

        # Extract group fields
        group_id = metadata.get("group_id")
        group_type = metadata.get("group_type")
        phase_seq = metadata.get("phase_seq")
        phase_total = metadata.get("phase_total")

        return SearchResult(
            record_id=record_id,
            title=title,
            payload_text=payload_text,
            domain=domain,
            certainty=certainty,
            status=status,
            score=raw.get("score", 0.0),
            metadata=metadata,
            group_id=group_id,
            group_type=group_type,
            phase_seq=phase_seq,
            phase_total=phase_total,
        )

    async def _expand_phase_chains(
        self,
        results: List[SearchResult],
        max_chains: int = 2,
    ) -> List[SearchResult]:
        """
        Expand phase chain results by fetching sibling phases.

        When a search result is part of a phase chain, searches for the
        group_id to retrieve all sibling phases and inserts them in order.

        Args:
            results: Current search results
            max_chains: Max number of chains to expand (cost control)

        Returns:
            Results with phase chains expanded inline
        """
        # Find unique group_ids from phase results
        seen_groups = set()
        groups_to_expand = []
        for r in results:
            if r.is_phase and r.group_id not in seen_groups:
                seen_groups.add(r.group_id)
                groups_to_expand.append(r.group_id)

        if not groups_to_expand:
            return results

        # Limit expansion for cost control
        groups_to_expand = groups_to_expand[:max_chains]

        # Fetch siblings for each group
        group_siblings: Dict[str, List[SearchResult]] = {}
        existing_ids = {r.record_id for r in results}

        for group_id in groups_to_expand:
            # Search by group_id (embedded in payload.text)
            siblings = await self._search_single(f"Group: {group_id}", topk=10)
            chain = [s for s in siblings if s.group_id == group_id]
            # Sort by phase_seq
            chain.sort(key=lambda s: s.phase_seq if s.phase_seq is not None else 0)
            group_siblings[group_id] = chain

        # Rebuild result list with chains expanded inline
        expanded = []
        expanded_ids = set()

        for r in results:
            if r.record_id in expanded_ids:
                continue

            if r.is_phase and r.group_id in group_siblings:
                # Insert entire chain at this position
                for sibling in group_siblings[r.group_id]:
                    if sibling.record_id not in expanded_ids:
                        expanded.append(sibling)
                        expanded_ids.add(sibling.record_id)
                # Remove group from dict so we don't expand again
                del group_siblings[r.group_id]
            else:
                expanded.append(r)
                expanded_ids.add(r.record_id)

        return expanded

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

    async def search_by_id(self, record_id: str) -> Optional[SearchResult]:
        """
        Search for a specific record by ID.

        Useful for retrieving full context of a referenced record.
        """
        # Search with the ID as query (will match in payload.text)
        results = await self._search_single(f"ID: {record_id}", topk=5)

        # Find exact match
        for result in results:
            if result.record_id == record_id:
                return result

        return None

    async def get_related(
        self,
        record_id: str,
        topk: int = 5
    ) -> List[SearchResult]:
        """
        Find records related to a given record.

        Useful for "See also" suggestions.
        """
        # First get the record
        record = await self.search_by_id(record_id)
        if not record:
            return []

        # Search using its payload.text as query
        results = await self._search_single(record.payload_text[:500], topk + 1)

        # Exclude the original record
        return [r for r in results if r.record_id != record_id][:topk]
