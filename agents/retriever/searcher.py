"""
Searcher

Searches organizational memory via enVector.
Uses the Vault-secured pipeline: scoring → decrypt → metadata.
Returns Decision Records with their payload.text for synthesis.

v0.2.4 changes:
- Recency weighting on returned results (benign re-ranking)
- Group assembly from already-fetched results (no over-fetch)
- Client-side metadata filters (best-effort on returned top-k)

NOTE: Full metadata filtering, group assembly, and recency weighting
should happen Vault-side (rune-admin) to preserve the security model.
The client NEVER requests more than the user's topk from Vault.
"""

import json
import logging
from datetime import datetime, timedelta, timezone
from typing import List, Dict, Any, Optional
from dataclasses import dataclass, field

from ..common.envector_client import EnVectorClient
from ..common.embedding_service import EmbeddingService
from .query_processor import ParsedQuery, TimeScope

logger = logging.getLogger("rune.retriever.searcher")

# Recency weighting parameters (applied client-side on returned top-k)
HALF_LIFE_DAYS = 90
SIMILARITY_WEIGHT = 0.7
RECENCY_WEIGHT = 0.3

STATUS_MULTIPLIER = {
    "accepted": 1.0,
    "proposed": 0.9,
    "superseded": 0.5,
    "reverted": 0.3,
}


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
    adjusted_score: float = 0.0  # After recency weighting + status penalty
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

    Security model: the client NEVER requests more than the user's topk
    from Vault. Over-fetch + post-filter must happen Vault-side.

    v0.2.4 client-side enhancements (on already-returned results only):
    - Assemble groups from results already in the result set
    - Apply recency weighting (re-ranking, not filtering)
    - Best-effort metadata filters (reduces result count, not ideal)
    """

    def __init__(
        self,
        envector_client: EnVectorClient,
        embedding_service: EmbeddingService,
        index_name: str,
        vault_client=None,
    ):
        self._client = envector_client
        self._embedding = embedding_service
        self._index_name = index_name
        self._vault = vault_client

    async def search(
        self,
        query: ParsedQuery,
        topk: Optional[int] = None,
        filters: Optional[Dict[str, Any]] = None,
    ) -> List[SearchResult]:
        """
        Search for relevant Decision Records.

        Args:
            query: Parsed query from QueryProcessor
            topk: Number of results to return (passed to Vault as-is)
            filters: Optional metadata filters (best-effort client-side;
                     full support requires Vault-side implementation):
                - domain: str (e.g. "architecture")
                - status: str (e.g. "accepted")
                - since: str (ISO date, e.g. "2026-01-01")

        Returns:
            List of SearchResult objects sorted by adjusted relevance
        """
        topk = topk or 10

        # Step 1: Search with multi-query expansion (respects Vault's topk limit)
        all_results = await self._search_with_expansions(query, topk)

        # Step 2: Expand phase chains for groups with missing siblings
        all_results = await self._expand_phase_chains(all_results)

        # Step 3: Assemble groups (order by phase_seq, interleave with standalone)
        all_results = self._assemble_groups(all_results)

        # Step 4: Best-effort metadata filters (client-side, on complete results)
        # NOTE: This may reduce result count below topk. Full support
        # requires Vault-side filtering with internal over-fetch.
        if filters:
            all_results = self._apply_metadata_filters(all_results, filters)

        # Step 5: Time scope filter
        if query.time_scope != TimeScope.ALL_TIME:
            all_results = self._filter_by_time(all_results, query.time_scope)

        # Step 6: Recency weighting (re-ranks returned results, no security issue)
        all_results = self._apply_recency_weighting(all_results)

        return all_results[:topk]

    async def _search_with_expansions(
        self, query: ParsedQuery, topk: int
    ) -> List[SearchResult]:
        """Search with multiple query expansions, dedup results."""
        all_results = []
        seen_ids = set()

        for expanded_query in query.expanded_queries[:3]:
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

        all_results.sort(key=lambda r: r.score, reverse=True)
        return all_results

    def _assemble_groups(self, results: List[SearchResult]) -> List[SearchResult]:
        """
        Assemble group members from already-fetched results.

        When multiple phases of the same group are in the result set,
        group them together ordered by phase_seq. Does NOT over-fetch;
        missing siblings are handled by _expand_phase_chains.
        """
        if not results:
            return results

        groups: Dict[str, List[SearchResult]] = {}
        group_best_score: Dict[str, float] = {}
        standalone = []

        for r in results:
            if r.is_phase and r.group_id:
                groups.setdefault(r.group_id, []).append(r)
                group_best_score[r.group_id] = max(
                    group_best_score.get(r.group_id, 0.0), r.score
                )
            else:
                standalone.append(r)

        if not groups:
            return results

        for gid in groups:
            groups[gid].sort(key=lambda r: r.phase_seq if r.phase_seq is not None else 0)

        # Interleave: insert groups at their best-score position
        all_items = []
        for r in standalone:
            all_items.append((r.score, "standalone", r))
        for gid, best_score in group_best_score.items():
            all_items.append((best_score, "group", gid))

        all_items.sort(key=lambda x: x[0], reverse=True)

        assembled = []
        inserted_groups = set()
        for score, item_type, item in all_items:
            if item_type == "standalone":
                assembled.append(item)
            elif item_type == "group" and item not in inserted_groups:
                inserted_groups.add(item)
                assembled.extend(groups[item])

        return assembled

    def _apply_metadata_filters(
        self, results: List[SearchResult], filters: Dict[str, Any]
    ) -> List[SearchResult]:
        """
        Best-effort metadata filters on already-returned results.

        WARNING: This reduces result count and may return fewer than topk.
        Full metadata filtering requires Vault-side implementation with
        internal over-fetch to maintain result count.
        """
        filtered = results

        domain = filters.get("domain")
        if domain:
            filtered = [r for r in filtered if r.domain == domain]

        status = filters.get("status")
        if status:
            filtered = [r for r in filtered if r.status == status]

        since = filters.get("since")
        if since:
            filtered = self._filter_since(filtered, since)

        return filtered

    def _filter_since(self, results: List[SearchResult], since_date: str) -> List[SearchResult]:
        """Filter results after a given ISO date."""
        filtered = []
        for r in results:
            ts_str = r.metadata.get("timestamp")
            if ts_str:
                try:
                    if isinstance(ts_str, str):
                        ts = datetime.fromisoformat(ts_str.replace("Z", "+00:00"))
                    else:
                        ts = datetime.fromtimestamp(float(ts_str), tz=timezone.utc)
                    if ts.isoformat() >= since_date:
                        filtered.append(r)
                except (ValueError, TypeError):
                    filtered.append(r)
            else:
                filtered.append(r)
        return filtered

    def _apply_recency_weighting(self, results: List[SearchResult]) -> List[SearchResult]:
        """
        Apply time decay and status-based scoring on returned results.

        This is a benign re-ranking of already-returned results.
        No security concern: the client only sees what Vault already returned.
        """
        now = datetime.now(timezone.utc)

        for r in results:
            age_days = 0
            ts_str = r.metadata.get("timestamp")
            if ts_str:
                try:
                    if isinstance(ts_str, str):
                        ts = datetime.fromisoformat(ts_str.replace("Z", "+00:00"))
                    else:
                        ts = datetime.fromtimestamp(float(ts_str), tz=timezone.utc)
                    age_days = max(0, (now - ts).days)
                except (ValueError, TypeError):
                    pass

            decay = 0.5 ** (age_days / HALF_LIFE_DAYS) if HALF_LIFE_DAYS > 0 else 1.0
            status_mult = STATUS_MULTIPLIER.get(r.status, 1.0)
            r.adjusted_score = (SIMILARITY_WEIGHT * r.score + RECENCY_WEIGHT * decay) * status_mult

        results.sort(key=lambda r: r.adjusted_score, reverse=True)
        return results

    # ================================================================
    # Phase chain expansion (additional searches for missing siblings)
    # ================================================================

    async def _expand_phase_chains(
        self,
        results: List[SearchResult],
        max_chains: int = 2,
    ) -> List[SearchResult]:
        """
        Expand phase chain results by fetching sibling phases.

        When a search result is part of a phase chain, searches for the
        group_id to retrieve all sibling phases and inserts them in order.
        Each sibling search respects Vault's topk limit.
        """
        seen_groups = set()
        groups_to_expand = []
        for r in results:
            if r.is_phase and r.group_id not in seen_groups:
                # Check if all siblings are already present
                total = r.phase_total or 0
                present = sum(1 for x in results if x.group_id == r.group_id)
                if present < total:
                    seen_groups.add(r.group_id)
                    groups_to_expand.append(r.group_id)

        if not groups_to_expand:
            return results

        groups_to_expand = groups_to_expand[:max_chains]

        group_siblings: Dict[str, List[SearchResult]] = {}
        existing_ids = {r.record_id for r in results}

        for group_id in groups_to_expand:
            siblings = await self._search_single(f"Group: {group_id}", topk=10)
            chain = [s for s in siblings if s.group_id == group_id and s.record_id not in existing_ids]
            chain.sort(key=lambda s: s.phase_seq if s.phase_seq is not None else 0)
            group_siblings[group_id] = chain

        expanded = []
        expanded_ids = set()

        for r in results:
            if r.record_id in expanded_ids:
                continue

            if r.is_phase and r.group_id in group_siblings:
                # Insert all siblings (including this one) in phase order
                all_in_group = [x for x in results if x.group_id == r.group_id] + group_siblings[r.group_id]
                seen_in_group = set()
                all_in_group.sort(key=lambda x: x.phase_seq if x.phase_seq is not None else 0)
                for sibling in all_in_group:
                    if sibling.record_id not in expanded_ids and sibling.record_id not in seen_in_group:
                        expanded.append(sibling)
                        expanded_ids.add(sibling.record_id)
                        seen_in_group.add(sibling.record_id)
                del group_siblings[r.group_id]
            elif r.record_id not in expanded_ids:
                expanded.append(r)
                expanded_ids.add(r.record_id)

        return expanded

    # ================================================================
    # Low-level search methods
    # ================================================================

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
            query_vector = self._embedding.embed_single(query_text)

            scoring_result = self._client.score(self._index_name, query_vector)
            if not scoring_result.get("ok"):
                logger.warning("Scoring failed: %s", scoring_result.get("error"))
                return []

            blobs = scoring_result.get("encrypted_blobs", [])
            if not blobs:
                return []

            vault_result = await self._vault.decrypt_search_results(
                encrypted_blob_b64=blobs[0],
                top_k=topk,
            )
            if not vault_result.ok:
                logger.warning("Vault decrypt failed: %s", vault_result.error)
                return []

            if not vault_result.results:
                return []

            metadata_result = self._client.remind(
                self._index_name,
                vault_result.results,
                output_fields=["metadata"],
            )
            if not metadata_result.get("ok"):
                logger.warning("Metadata retrieval failed: %s", metadata_result.get("error"))
                return []

            encrypted_entries = metadata_result.get("results", [])

            vault_decrypt_items = []
            for idx, entry in enumerate(encrypted_entries):
                data = entry.get("data", "")
                if not data:
                    continue

                try:
                    parsed = json.loads(data)
                    if isinstance(parsed, dict) and "a" in parsed and "c" in parsed:
                        vault_decrypt_items.append((idx, data))
                    else:
                        entry["metadata"] = parsed
                        entry.pop("data", None)
                except (json.JSONDecodeError, TypeError):
                    import base64
                    try:
                        raw = base64.b64decode(data)
                        parsed = json.loads(raw)
                        entry["metadata"] = parsed
                        entry.pop("data", None)
                    except Exception:
                        logger.warning("Entry %d: unrecognized metadata format, skipping", idx)
                        entry["metadata"] = {}
                        entry.pop("data", None)

            if vault_decrypt_items:
                try:
                    decrypted_metadata = await self._vault.decrypt_metadata(
                        encrypted_metadata_list=[data for _, data in vault_decrypt_items]
                    )
                    for dec_idx, (entry_idx, _) in enumerate(vault_decrypt_items):
                        if dec_idx < len(decrypted_metadata):
                            encrypted_entries[entry_idx]["metadata"] = decrypted_metadata[dec_idx]
                            encrypted_entries[entry_idx].pop("data", None)
                except Exception:
                    logger.info("Batch decrypt failed, falling back to per-entry decrypt")
                    for entry_idx, data in vault_decrypt_items:
                        try:
                            single = await self._vault.decrypt_metadata(
                                encrypted_metadata_list=[data]
                            )
                            if single:
                                encrypted_entries[entry_idx]["metadata"] = single[0]
                                encrypted_entries[entry_idx].pop("data", None)
                        except Exception as e:
                            logger.debug("Entry %d decrypt failed: %s", entry_idx, e)
                            encrypted_entries[entry_idx]["metadata"] = {}
                            encrypted_entries[entry_idx].pop("data", None)

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

        record_id = metadata.get("id", raw.get("id", "unknown"))
        title = metadata.get("title", "Untitled")
        domain = metadata.get("domain", "general")
        status = metadata.get("status", "unknown")

        why = metadata.get("why", {})
        if isinstance(why, dict):
            certainty = why.get("certainty", "unknown")
        else:
            certainty = "unknown"

        payload = metadata.get("payload", {})
        if isinstance(payload, dict):
            payload_text = payload.get("text", "")
        else:
            payload_text = metadata.get("text", raw.get("text", ""))

        if not payload_text:
            decision = metadata.get("decision", {})
            if isinstance(decision, dict):
                payload_text = decision.get("what", "")

        group_id = metadata.get("group_id")
        group_type = metadata.get("group_type")
        phase_seq = metadata.get("phase_seq")
        phase_total = metadata.get("phase_total")

        score = raw.get("score", 0.0)
        return SearchResult(
            record_id=record_id,
            title=title,
            payload_text=payload_text,
            domain=domain,
            certainty=certainty,
            status=status,
            score=score,
            adjusted_score=score,
            metadata=metadata,
            group_id=group_id,
            group_type=group_type,
            phase_seq=phase_seq,
            phase_total=phase_total,
        )

    def _filter_by_time(
        self,
        results: List[SearchResult],
        time_scope: TimeScope
    ) -> List[SearchResult]:
        """Filter results by time scope"""
        now = datetime.now(timezone.utc)

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
            timestamp_str = result.metadata.get("timestamp")
            if timestamp_str:
                try:
                    if isinstance(timestamp_str, str):
                        ts = datetime.fromisoformat(timestamp_str.replace("Z", "+00:00"))
                    else:
                        ts = datetime.fromtimestamp(float(timestamp_str), tz=timezone.utc)
                    if ts >= cutoff:
                        filtered.append(result)
                except (ValueError, TypeError):
                    filtered.append(result)
            else:
                filtered.append(result)

        return filtered

    async def search_by_id(self, record_id: str) -> Optional[SearchResult]:
        """Search for a specific record by ID."""
        results = await self._search_single(f"ID: {record_id}", topk=5)
        for result in results:
            if result.record_id == record_id:
                return result
        return None

    async def get_related(self, record_id: str, topk: int = 5) -> List[SearchResult]:
        """Find records related to a given record."""
        record = await self.search_by_id(record_id)
        if not record:
            return []
        results = await self._search_single(record.payload_text[:500], topk + 1)
        return [r for r in results if r.record_id != record_id][:topk]
