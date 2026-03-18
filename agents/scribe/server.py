"""
Scribe Server

FastAPI server for receiving webhooks and capturing organizational context.

Endpoints:
- POST /slack/events: Slack webhook endpoint
- POST /notion/events: Notion webhook endpoint
- GET /health: Health check
- GET /review: Get pending reviews
- POST /review/{record_id}: Submit review

Pipeline:
1. Receive webhook event
2. Parse with appropriate handler
3. Detect significance with pattern matching
4. Build Decision Record
5. Auto-capture or add to review queue
6. Store to enVector
"""

import json
import logging
import os
from datetime import datetime, timezone
from typing import Optional
from contextlib import asynccontextmanager

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
logger = logging.getLogger("rune.scribe")

from fastapi import FastAPI, Request, HTTPException, Header, BackgroundTasks
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from ..common.config import load_config, RuneConfig, ensure_directories
from ..common.embedding_service import EmbeddingService, get_embedding_service
from ..common.envector_client import EnVectorClient
from ..common.llm_client import LLMClient
from ..common.pattern_cache import PatternCache
from ..common.language import detect_language
from .pattern_parser import load_default_patterns, load_all_language_patterns
from .detector import DecisionDetector, DetectionResult
from .record_builder import RecordBuilder, RawEvent
from .llm_extractor import LLMExtractor
from .review_queue import ReviewQueue, ReviewAnswers, ReviewAnswer
from .tier2_filter import Tier2Filter
from .handlers import SlackHandler, NotionHandler, Message


# Global state
config: Optional[RuneConfig] = None
index_name: Optional[str] = None
detector: Optional[DecisionDetector] = None
tier2_filter: Optional[Tier2Filter] = None
record_builder: Optional[RecordBuilder] = None
envector_client: Optional[EnVectorClient] = None
review_queue: Optional[ReviewQueue] = None
slack_handler: Optional[SlackHandler] = None
notion_handler: Optional[NotionHandler] = None
embedding_service: Optional[EmbeddingService] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize components on startup"""
    global config, index_name, detector, tier2_filter, record_builder, envector_client, review_queue
    global slack_handler, notion_handler, embedding_service

    logger.info("Starting up...")

    # Add mcp/ to sys.path so that `from adapter.envector_sdk import ...` works
    import sys as _sys
    from pathlib import Path as _Path
    _mcp_root = str(_Path(__file__).parent.parent.parent / "mcp")
    if _mcp_root not in _sys.path:
        _sys.path.insert(0, _mcp_root)

    # Ensure directories exist
    ensure_directories()

    # Load config
    config = load_config()
    logger.info("Loaded config (state: %s)", config.state)

    # Index name must be provided via environment variable (set by Vault or admin)
    index_name = os.getenv("ENVECTOR_INDEX_NAME")
    if not index_name:
        logger.warning("ENVECTOR_INDEX_NAME not set — record storage will be unavailable")

    # Fetch FHE public keys from Vault (provides key_id)
    vault_key_id = None
    if config.vault.endpoint and config.vault.token:
        logger.info("[keys] Fetching from Vault: %s", config.vault.endpoint)
        try:
            from adapter.vault_client import VaultClient as _VaultClient

            _vault = _VaultClient(vault_endpoint=config.vault.endpoint, vault_token=config.vault.token)
            try:
                bundle = await _vault.get_public_key()
                logger.info("[keys] Vault response keys: %s", list(bundle.keys()))
                vault_key_id = bundle.pop("key_id", None)
                vault_index = bundle.pop("index_name", None)
                bundle.pop("agent_id", None)
                bundle.pop("agent_dek", None)
                logger.info("[keys] key_id=%s  index_name=%s  files_in_bundle=%s", vault_key_id, vault_index, list(bundle.keys()))

                if vault_key_id:
                    key_dir = os.path.join(os.path.expanduser("~/.rune/keys"), vault_key_id)
                    os.makedirs(key_dir, mode=0o700, exist_ok=True)
                    for filename, content in bundle.items():
                        filepath = os.path.join(key_dir, filename)
                        fd = os.open(filepath, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
                        with os.fdopen(fd, "w") as f:
                            f.write(content)
                        logger.info("[keys] Wrote %s (%d bytes)", filepath, len(content))
                    # Log final contents of the key directory
                    saved = os.listdir(key_dir)
                    logger.info("[keys] ~/.rune/keys/%s/ contains: %s", vault_key_id, saved)
                    if vault_index and not index_name:
                        index_name = vault_index
                        logger.info("[keys] Using index_name from Vault: %s", index_name)
                else:
                    logger.warning("[keys] Vault did not return a key_id — storage will fail")
            finally:
                await _vault.close()
        except Exception as e:
            logger.error("[keys] Failed to fetch from Vault: %s", e)
            # Fall back to cached keys in ~/.rune/keys/
            keys_dir = os.path.expanduser("~/.rune/keys")
            logger.info("[keys] Checking cache at %s", keys_dir)
            if os.path.isdir(keys_dir):
                cached = [
                    d for d in os.listdir(keys_dir)
                    if os.path.isfile(os.path.join(keys_dir, d, "EncKey.json"))
                ]
                logger.info("[keys] Cached key dirs found: %s", cached)
                if cached:
                    vault_key_id = cached[0]
                    logger.info("[keys] Using cached key_id=%s", vault_key_id)
                else:
                    logger.error("[keys] No cached keys found — storage unavailable")
    else:
        logger.info("[keys] Vault not configured — checking cache")
        keys_dir = os.path.expanduser("~/.rune/keys")
        if os.path.isdir(keys_dir):
            cached = [
                d for d in os.listdir(keys_dir)
                if os.path.isfile(os.path.join(keys_dir, d, "EncKey.json"))
            ]
            logger.info("[keys] Cached key dirs found: %s", cached)
            if cached:
                vault_key_id = cached[0]
                logger.info("[keys] Using cached key_id=%s", vault_key_id)

    logger.info("[keys] Final vault_key_id=%s", vault_key_id)

    # Initialize embedding service
    logger.info("Initializing embedding service...")
    embedding_service = get_embedding_service(
        mode=config.embedding.mode,
        model=config.embedding.model
    )

    # Load and embed patterns (including multilingual)
    logger.info("Loading patterns...")
    patterns = load_all_language_patterns()
    logger.info("Found %d patterns", len(patterns))

    pattern_cache = PatternCache(embedding_service)
    pattern_cache.load_patterns(patterns)

    # Initialize detector
    detector = DecisionDetector(
        pattern_cache=pattern_cache,
        threshold=config.scribe.similarity_threshold,
        high_confidence_threshold=config.scribe.auto_capture_threshold
    )
    logger.info("Detector ready (threshold: %s)", config.scribe.similarity_threshold)

    # LLM configuration
    llm_cfg = config.llm
    llm_provider = (llm_cfg.provider or os.getenv("RUNE_LLM_PROVIDER", "anthropic")).lower()
    tier2_provider = (llm_cfg.tier2_provider or os.getenv("RUNE_TIER2_LLM_PROVIDER", llm_provider)).lower()
    anthropic_key = llm_cfg.anthropic_api_key or os.getenv("ANTHROPIC_API_KEY") or None
    openai_key = llm_cfg.openai_api_key or os.getenv("OPENAI_API_KEY") or None
    google_key = llm_cfg.google_api_key or os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY") or None

    def _provider_key(provider: str):
        if provider == "openai":
            return openai_key
        if provider == "google":
            return google_key
        return anthropic_key

    def _provider_model(provider: str, role: str) -> str:
        if provider == "openai":
            if role == "tier2" and llm_cfg.openai_tier2_model:
                return llm_cfg.openai_tier2_model
            return llm_cfg.openai_model
        if provider == "google":
            if role == "tier2" and llm_cfg.google_tier2_model:
                return llm_cfg.google_tier2_model
            return llm_cfg.google_model
        if role == "tier2":
            return config.scribe.tier2_model
        return llm_cfg.anthropic_model

    # Initialize Tier 2 LLM filter
    if config.scribe.tier2_enabled and _provider_key(tier2_provider):
        tier2_filter = Tier2Filter(
            llm_provider=tier2_provider,
            anthropic_api_key=anthropic_key,
            openai_api_key=openai_key,
            google_api_key=google_key,
            model=_provider_model(tier2_provider, "tier2"),
        )
        if tier2_filter.is_available:
            logger.info("Tier 2 LLM filter ready (%s/%s)", tier2_provider, tier2_filter._model)
        else:
            logger.warning("Tier 2 LLM filter init failed (Tier 1 only)")
    else:
        tier2_filter = None
        logger.info("Tier 2 LLM filter disabled" if not config.scribe.tier2_enabled else "Tier 2 skipped (no API key)")

    # Initialize Tier 3 LLM extractor (for record building)
    llm_extractor = LLMExtractor(
        llm_provider=llm_provider,
        anthropic_api_key=anthropic_key,
        openai_api_key=openai_key,
        google_api_key=google_key,
        model=_provider_model(llm_provider, "extract"),
    )
    if llm_extractor.is_available:
        logger.info("Tier 3 LLM extractor ready (Sonnet)")
    else:
        logger.info("Tier 3 LLM extractor not available (regex fallback)")

    # Initialize record builder
    record_builder = RecordBuilder(llm_extractor=llm_extractor)

    # Initialize review queue
    review_queue = ReviewQueue()
    stats = review_queue.get_stats()
    logger.info("Review queue: %d pending", stats['pending'])

    # Initialize handlers
    slack_handler = SlackHandler(signing_secret=config.scribe.slack_signing_secret)
    notion_handler = NotionHandler(signing_secret=config.scribe.notion_signing_secret)

    # Initialize enVector client (if configured)
    if config.envector.endpoint:
        logger.info("[envector] Initializing client (address=%s, key_id=%s)", config.envector.endpoint, vault_key_id)
        try:
            envector_client = EnVectorClient(
                address=config.envector.endpoint,
                access_token=config.envector.api_key or None,
                key_id=vault_key_id,
                auto_key_setup=False
            )
            logger.info("EnVector client ready (%s)", config.envector.endpoint)
        except Exception as e:
            logger.warning("EnVector client failed: %s", e)
            envector_client = None

    logger.info("Ready to receive events")

    yield

    # Cleanup
    logger.info("Shutting down...")


app = FastAPI(
    title="Rune Scribe Agent",
    description="Organizational context capture via webhooks",
    version="0.1.0",
    lifespan=lifespan
)


# =============================================================================
# Request/Response Models
# =============================================================================

class ReviewSubmission(BaseModel):
    """Review submission request"""
    q1_worth_saving: str  # "capture" or "ignore"
    q2_evidence_supported: str  # "supported", "partially_supported", "unknown"
    q3_sensitivity: str  # "public", "internal", "restricted"
    q4_status: Optional[str] = None  # "proposed", "accepted", etc.
    reviewer_notes: Optional[str] = None
    reviewer: Optional[str] = None


# =============================================================================
# Background Tasks
# =============================================================================

def _parse_slack_command(text: str) -> tuple[str, str]:
    """
    Extract command and payload from an @Rune mention.

    "@Rune remember we chose PostgreSQL" → ("remember", "we chose PostgreSQL")
    "@Rune something else"               → ("something", "else")
    """
    import re as _re
    clean = _re.sub(r"<@U[A-Z0-9]+>\s*", "", text).strip()
    parts = clean.split(None, 1)
    command = parts[0].lower() if parts else ""
    payload = parts[1] if len(parts) > 1 else ""
    return command, payload


async def _post_to_slack(channel: str, text: str, thread_ts: Optional[str] = None) -> None:
    """Post a message back to Slack using chat.postMessage."""
    bot_token = os.getenv("SLACK_BOT_TOKEN")
    if not bot_token:
        logger.warning("[slack_post] SLACK_BOT_TOKEN not set — cannot reply to Slack")
        return

    import httpx
    payload = {"channel": channel, "text": text}
    if thread_ts:
        payload["thread_ts"] = thread_ts

    try:
        async with httpx.AsyncClient() as client:
            resp = await client.post(
                "https://slack.com/api/chat.postMessage",
                headers={"Authorization": f"Bearer {bot_token}", "Content-Type": "application/json"},
                json=payload,
                timeout=10,
            )
        data = resp.json()
        if not data.get("ok"):
            logger.warning("[slack_post] API error: %s", data.get("error"))
        else:
            logger.info("[slack_post] Message posted to %s", channel)
    except Exception as e:
        logger.error("[slack_post] Failed to post: %s", e)


async def _synthesize_results(query: str, results: list) -> Optional[str]:
    """
    Call LLM to synthesize search results into one coherent answer — mirrors retriever.md behavior.
    Returns None if LLM is unavailable (caller falls back to raw list).
    """
    import asyncio
    import re as _re

    llm_cfg = config.llm if config else None
    anthropic_key = (llm_cfg.anthropic_api_key if llm_cfg else None) or os.getenv("ANTHROPIC_API_KEY")
    openai_key = (llm_cfg.openai_api_key if llm_cfg else None) or os.getenv("OPENAI_API_KEY")
    google_key = (llm_cfg.google_api_key if llm_cfg else None) or os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY")

    llm_provider = (
        (llm_cfg.tier2_provider or llm_cfg.provider) if llm_cfg else None
    ) or os.getenv("RUNE_TIER2_LLM_PROVIDER") or os.getenv("RUNE_LLM_PROVIDER", "anthropic")
    llm_provider = llm_provider.lower()

    # Use the fast tier2 model (Haiku) — synthesis is cheaper than extraction
    model = (config.scribe.tier2_model if config else None) or "claude-haiku-4-5-20251001"

    llm = LLMClient(
        provider=llm_provider,
        model=model,
        anthropic_api_key=anthropic_key,
        openai_api_key=openai_key,
        google_api_key=google_key,
    )

    if not llm.is_available:
        logger.info("[synthesize] LLM unavailable — skipping synthesis")
        return None

    # Build results block for the prompt
    records_text = []
    for r in results:
        clean = _re.sub(r"# Decision Record:.*?(?=\n\n|\Z)", "", r.payload_text or "", flags=_re.DOTALL).strip()
        snippet = (clean or r.payload_text or "")[:500]
        records_text.append(
            f"[{r.record_id}]  Title: {r.title}  Domain: {r.domain}  "
            f"Certainty: {r.certainty}  Score: {r.score:.2f}\n{snippet}"
        )

    prompt = (
        f'A user asked: "{query}"\n\n'
        f"Relevant records from organizational memory:\n\n"
        + "\n\n---\n\n".join(records_text)
        + "\n\nSynthesize these records into a clear, concise answer (2-4 sentences max).\n"
        "Rules:\n"
        "- Cite record IDs in brackets, e.g. [dec_2024-01-15_arch_postgres]\n"
        "- Match tone to certainty: 'supported' → confident, 'partially_supported' → hedged, 'unknown' → caveated\n"
        "- If multiple records are phases of the same decision, consolidate into one mention\n"
        "- Do NOT fabricate — only use information present in the records\n"
        "- If records are not relevant to the query, say so briefly\n\n"
        "Answer:"
    )

    loop = asyncio.get_event_loop()
    try:
        answer = await loop.run_in_executor(
            None, lambda: llm.generate(prompt, max_tokens=350, timeout=20.0)
        )
        logger.info("[synthesize] Synthesis complete (%d chars)", len(answer))
        return answer
    except Exception as e:
        logger.warning("[synthesize] LLM synthesis failed: %s", e)
        return None


async def _force_recall(query: str, user: str, channel: str, thread_ts: Optional[str] = None) -> None:
    """
    Search enVector via the Vault-secured pipeline and return results to Slack.
    Called when user types @Rune recall <query>.
    """
    global envector_client, embedding_service, index_name, config

    if not envector_client or not embedding_service:
        logger.error("[force_recall] enVector client not initialized")
        await _post_to_slack(channel, "Error: enVector not initialized.", thread_ts)
        return

    if not index_name:
        logger.error("[force_recall] index_name not configured")
        await _post_to_slack(channel, "Error: index not configured.", thread_ts)
        return

    logger.info("[force_recall] query=%r  user=%s  index=%s", query, user, index_name)

    vault_client = None
    try:
        from ..retriever.searcher import Searcher
        from adapter.vault_client import VaultClient

        if config and config.vault.endpoint and config.vault.token:
            vault_client = VaultClient(
                vault_endpoint=config.vault.endpoint,
                vault_token=config.vault.token,
            )
            logger.info("[force_recall] Vault client ready for decryption")

        searcher = Searcher(
            envector_client=envector_client,
            embedding_service=embedding_service,
            index_name=index_name,
            vault_client=vault_client,
        )

        results = await searcher._search_single(query, topk=10)

        _results_json = json.dumps([r.__dict__ for r in results], indent=2, default=str)
        logger.info("RESULTS...................................`:\n%s", _results_json)
        _export_path = os.path.join(os.path.dirname(__file__), "rune_results.json")
        with open(_export_path, "w") as _f:
            _f.write(_results_json)
        logger.info("[force_recall] Results exported to %s", _export_path)
        if not results:
            logger.info("[force_recall] No results found for query: %r", query)
            await _post_to_slack(channel, f"No results found for: _{query}_", thread_ts)
            return

        logger.info("[force_recall] Raw results (%d):", len(results))
        for i, r in enumerate(results, 1):
            logger.info("[force_recall]  %d. [%.2f] %s", i, r.score, r.title)

        # Filter out anomalous scores (>1.0 means bad vector) and low relevance
        MIN_SCORE = 0.4
        filtered = [r for r in results if 0.0 < r.score <= 1.0 and r.score >= MIN_SCORE]
        filtered = filtered[:5]

        logger.info("[force_recall] After filtering (score 0.4–1.0): %d results", len(filtered))

        if not filtered:
            await _post_to_slack(channel, f"No relevant results found for: _{query}_", thread_ts)
            return

        # Try LLM synthesis first (mirrors retriever.md behavior)
        answer = await _synthesize_results(query, filtered)

        if answer:
            await _post_to_slack(channel, f"*Recall:* _{query}_\n\n{answer}", thread_ts)
        else:
            # Fallback: deterministic formatter
            from .format_results import format_results_from_records
            formatted = format_results_from_records(query, filtered)
            await _post_to_slack(channel, formatted, thread_ts)

    except Exception as e:
        logger.error("[force_recall] Error during search: %s", e)
        await _post_to_slack(channel, f"Error during recall: {e}", thread_ts)
    finally:
        if vault_client:
            await vault_client.close()


async def _force_capture(text: str, user: str, channel: str) -> None:
    """
    Store to enVector after passing Tier 1 + Tier 2 checks — same pipeline as /rune:capture.
    Called when user explicitly types @Rune remember <text>.
    """
    global record_builder, detector, tier2_filter

    if not record_builder:
        logger.error("[force_capture] record_builder not initialized")
        return

    # Tier 1: embedding similarity check
    if detector:
        detection = detector.detect(text)
        if not detection.is_significant:
            logger.info(
                "[force_capture] Tier 1 REJECT — confidence=%.2f threshold=%.2f text=%r",
                detection.confidence, detector.threshold, text,
            )
            await _post_to_slack(
                channel,
                f"Not stored — _{text[:80]}_ doesn't look like a decision "
                f"(confidence: {detection.confidence:.2f}, threshold: {detector.threshold}).\n"
                f"Try: _@Rune remember we chose X because Y_",
            )
            return
        logger.info("[force_capture] Tier 1 PASS — confidence=%.2f pattern=%r", detection.confidence, detection.matched_pattern)
        result = detection
    else:
        result = DetectionResult(is_significant=True, confidence=1.0, matched_pattern="manual capture")

    # Tier 2: LLM policy filter (Haiku) — same gate as /rune:capture
    if tier2_filter and tier2_filter.is_available:
        filter_result = tier2_filter.evaluate(
            text=text,
            tier1_score=result.confidence,
            tier1_pattern=result.matched_pattern or "",
        )
        if not filter_result.should_capture:
            logger.info("[force_capture] Tier 2 REJECT: %s", filter_result.reason)
            await _post_to_slack(
                channel,
                f"Not stored — _{text[:80]}_\nReason: {filter_result.reason}\n"
                f"Try including a decision with reasoning: _@Rune remember we chose X because Y_",
            )
            return
        logger.info("[force_capture] Tier 2 PASS: %s", filter_result.reason)
        if filter_result.domain and filter_result.domain != "general":
            from dataclasses import replace as _replace
            result = _replace(result, domain=filter_result.domain)
    else:
        logger.info("[force_capture] Tier 2 skipped (filter unavailable)")

    import time as _time
    raw_event = RawEvent(
        text=text,
        user=user,
        channel=channel,
        timestamp=str(_time.time()),
        source="slack_mention",
    )

    language = detect_language(text)
    records = record_builder.build_phases(raw_event, result, language=language)
    logger.info("[force_capture] Built %d record(s)", len(records))

    for record in records:
        logger.info(
            "[force_capture] ┌─ Decision Record ──────────────────────────\n"
            "                │  id:          %s\n"
            "                │  title:       %s\n"
            "                │  domain:      %s\n"
            "                │  status:      %s\n"
            "                │  certainty:   %s\n"
            "                │  rationale:   %s\n"
            "                │  tags:        %s\n"
            "                │  payload:     %s\n"
            "                └────────────────────────────────────────────",
            record.id,
            record.title,
            record.domain.value if hasattr(record.domain, "value") else record.domain,
            record.status.value if hasattr(record.status, "value") else record.status,
            record.why.certainty.value if hasattr(record.why.certainty, "value") else record.why.certainty,
            (record.why.rationale_summary or "")[:120],
            record.tags,
            record.payload.text[:200].replace("\n", " "),
        )
        await store_record(record)

    logger.info("[force_capture] ✓ Done: %d record(s) force-pushed to enVector by user=%s", len(records), user)


async def process_message(message: Message):
    """
    Process a message through the 3-tier capture pipeline.

    Tier 1: Embedding similarity (local, zero tokens) — wide net
    Tier 2: LLM policy filter (Haiku, ~200 tokens) — false positive removal
    Tier 3: LLM extraction (Sonnet, ~500 tokens) — Decision Record building
    """
    global detector, tier2_filter, record_builder, envector_client, review_queue, embedding_service

    if not detector or not record_builder:
        logger.warning("Not initialized, skipping message")
        return

    # === Tier 1: Embedding similarity (local, free) ===
    result = detector.detect(message.text)

    if not result.is_significant:
        return  # Not significant, ignore

    logger.info("Tier 1 PASS (score: %.2f, pattern: \"%.50s...\")", result.confidence, result.matched_pattern)

    # === Tier 2: LLM policy filter (Haiku, cheap) ===
    if tier2_filter and tier2_filter.is_available:
        filter_result = tier2_filter.evaluate(
            text=message.text,
            tier1_score=result.confidence,
            tier1_pattern=result.matched_pattern or "",
        )

        if not filter_result.should_capture:
            logger.info("Tier 2 REJECT: %s", filter_result.reason)
            return

        logger.info("Tier 2 PASS: %s", filter_result.reason)

        # Use Tier 2's domain hint if Tier 1's is generic
        if filter_result.domain != "general" and result.domain in (None, "general"):
            result.domain = filter_result.domain
    else:
        logger.info("Tier 2 skipped (filter unavailable)")

    # === Tier 3: LLM extraction + Decision Record building (Sonnet) ===
    raw_event = RawEvent(
        text=message.text,
        user=message.user,
        channel=message.channel,
        timestamp=message.timestamp,
        source=message.source,
        thread_ts=message.thread_ts,
        url=message.url,
    )

    language = detect_language(message.text)
    record = record_builder.build(raw_event, result, language=language)

    logger.info("Tier 3 built record: %s (certainty: %s)", record.id, record.why.certainty.value)

    # Decide: auto-capture or review queue
    if detector.should_auto_capture(result):
        await store_record(record)
    else:
        review_queue.add(record, result.confidence)
        logger.info("Added to review queue: %s", record.id)


async def store_record(record):
    """Store a Decision Record to enVector"""
    global envector_client, embedding_service, index_name

    if not envector_client or not embedding_service:
        logger.warning("Cannot store (no enVector client): %s", record.id)
        return

    if not index_name:
        logger.warning("Cannot store (no index name configured): %s", record.id)
        return

    try:
        text = record.payload.text
        metadata = record.model_dump(mode='json')
        logger.info("[store_record] Inserting %s into index=%s (%d chars)", record.id, index_name, len(text))

        result = envector_client.insert_with_text(
            index_name=index_name,
            texts=[text],
            embedding_service=embedding_service,
            metadata=[metadata]
        )

        if result.get("ok"):
            logger.info("[store_record] ✓ Successfully stored %s", record.id)
        else:
            logger.error("[store_record] ✗ Failed to store %s: %s", record.id, result.get("error"))

    except Exception as e:
        logger.error("[store_record] ✗ Exception storing %s: %s", record.id, e)


# =============================================================================
# Endpoints
# =============================================================================

@app.get("/health")
async def health():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "service": "scribe",
        "initialized": detector is not None,
        "pipeline": "3-tier" if (tier2_filter and tier2_filter.is_available) else "1-tier",
        "tier1_patterns": detector._cache.pattern_count if detector else 0,
        "tier2_available": tier2_filter.is_available if tier2_filter else False,
        "tier3_available": record_builder._llm_extractor.is_available if record_builder and record_builder._llm_extractor else False,
        "pending_reviews": review_queue.get_stats()["pending"] if review_queue else 0,
    }


@app.post("/slack/events")
async def slack_events(
    request: Request,
    background_tasks: BackgroundTasks,
    x_slack_signature: Optional[str] = Header(None),
    x_slack_request_timestamp: Optional[str] = Header(None)
):
    """
    Handle Slack webhook events.

    This is the main entry point for Slack integration.
    """
    global slack_handler

    if not slack_handler:
        raise HTTPException(status_code=503, detail="Handler not initialized")

    # Read body
    body = await request.body()

    # Verify signature
    if not slack_handler.verify_signature(
        body,
        x_slack_signature or "",
        x_slack_request_timestamp or ""
    ):
        raise HTTPException(status_code=401, detail="Invalid signature")

    # Parse JSON
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        raise HTTPException(status_code=400, detail="Invalid JSON")

    # Handle URL verification challenge
    if data.get("type") == "url_verification":
        return JSONResponse({"challenge": data.get("challenge")})

    # Parse event to message
    message = await slack_handler.parse_event(data)

    if message:
        command, payload = _parse_slack_command(message.text)
        logger.info("[slack/events] command=%r payload=%r", command, payload)

        if command == "recall" and payload:
            # @Rune recall <query> → search enVector and return results to Slack
            logger.info("[slack/events] recall triggered by user=%s query=%r", message.user, payload)
            background_tasks.add_task(_force_recall, payload, message.user, message.channel, message.thread_ts or message.timestamp)
        elif command and command != "recall":
            # Unknown command → help message
            background_tasks.add_task(
                _post_to_slack,
                message.channel,
                f"Unknown command `{command}`. Try: `@Rune recall <question>`",
                message.thread_ts or message.timestamp,
            )

    # Acknowledge receipt
    return JSONResponse({"ok": True})


@app.post("/notion/events")
async def notion_events(
    request: Request,
    background_tasks: BackgroundTasks,
    x_notion_signature: Optional[str] = Header(None),
):
    """
    Handle Notion webhook events.

    Receives page.created, page.updated, and database.updated events.
    """
    global notion_handler

    if not notion_handler:
        raise HTTPException(status_code=503, detail="Handler not initialized")

    # Read body
    body = await request.body()

    # Verify signature
    if not notion_handler.verify_signature(body, x_notion_signature or "", ""):
        raise HTTPException(status_code=401, detail="Invalid signature")

    # Parse JSON
    try:
        data = json.loads(body)
    except json.JSONDecodeError:
        raise HTTPException(status_code=400, detail="Invalid JSON")

    # Parse event to message
    message = await notion_handler.parse_event(data)

    if message and notion_handler.should_process(message):
        background_tasks.add_task(process_message, message)

    # Acknowledge receipt
    return JSONResponse({"ok": True})


@app.get("/review")
async def get_reviews():
    """Get pending reviews"""
    global review_queue

    if not review_queue:
        raise HTTPException(status_code=503, detail="Review queue not initialized")

    pending = review_queue.get_pending()

    return {
        "pending_count": len(pending),
        "items": [
            {
                "record_id": item.record_id,
                "confidence": item.detection_confidence,
                "created_at": item.created_at,
                "title": item.record_json.get("title", "N/A"),
                "domain": item.record_json.get("domain", "N/A"),
            }
            for item in pending
        ]
    }


@app.get("/review/{record_id}")
async def get_review_item(record_id: str):
    """Get a specific review item"""
    global review_queue

    if not review_queue:
        raise HTTPException(status_code=503, detail="Review queue not initialized")

    item = review_queue.get_item(record_id)
    if not item:
        raise HTTPException(status_code=404, detail="Item not found")

    return {
        "record_id": item.record_id,
        "confidence": item.detection_confidence,
        "created_at": item.created_at,
        "status": item.status,
        "questions": item.questions,
        "record": item.record_json,
        "formatted": review_queue.format_for_review(item),
    }


@app.post("/review/{record_id}")
async def submit_review(record_id: str, submission: ReviewSubmission):
    """Submit review for an item"""
    global review_queue

    if not review_queue:
        raise HTTPException(status_code=503, detail="Review queue not initialized")

    # Convert submission to ReviewAnswers
    answers = ReviewAnswers(
        q1_worth_saving=ReviewAnswer(submission.q1_worth_saving),
        q2_evidence_supported=ReviewAnswer(submission.q2_evidence_supported),
        q3_sensitivity=ReviewAnswer(submission.q3_sensitivity),
        q4_status=ReviewAnswer(submission.q4_status) if submission.q4_status else None,
        reviewer_notes=submission.reviewer_notes,
    )

    # Submit review
    record = review_queue.submit_review(
        record_id=record_id,
        answers=answers,
        reviewer=submission.reviewer
    )

    if record is None:
        # Item was rejected
        return {"status": "rejected", "record_id": record_id}

    # Store approved record
    await store_record(record)

    return {
        "status": "approved",
        "record_id": record_id,
        "stored": True,
    }


@app.delete("/review/{record_id}")
async def delete_review(record_id: str):
    """Delete a review item"""
    global review_queue

    if not review_queue:
        raise HTTPException(status_code=503, detail="Review queue not initialized")

    removed = review_queue.remove(record_id)
    if not removed:
        raise HTTPException(status_code=404, detail="Item not found")

    return {"status": "deleted", "record_id": record_id}


# @app.post("/slack/remember")
# async def force_remember(
#     request: Request,
#     background_tasks: BackgroundTasks,
#     x_slack_signature: Optional[str] = Header(None),
#     x_slack_request_timestamp: Optional[str] = Header(None),
# ):

#     global record_builder, slack_handler

#     logger.info("[/slack/remember] Request received")

#     if not record_builder:
#         logger.error("[/slack/remember] record_builder not initialized")
#         return JSONResponse({"response_type": "ephemeral", "text": "Error: server not initialized."})

#     body = await request.body()
#     logger.info("[/slack/remember] Raw body: %s", body.decode("utf-8", errors="replace"))

#     if slack_handler and not slack_handler.verify_signature(
#         body,
#         x_slack_signature or "",
#         x_slack_request_timestamp or "",
#     ):
#         logger.warning("[/slack/remember] Signature verification failed")
#         return JSONResponse({"response_type": "ephemeral", "text": "Error: invalid signature."})

#     logger.info("[/slack/remember] Signature OK")

#     # Slack slash commands send form-encoded data, not JSON
#     from urllib.parse import parse_qs
#     params = parse_qs(body.decode("utf-8"))
#     text = params.get("text", [""])[0].strip()
#     user = params.get("user_id", ["unknown"])[0]
#     channel = params.get("channel_id", ["unknown"])[0]
#     source = "slack_slash_command"

#     logger.info("[/slack/remember] Parsed — user=%s channel=%s text=%r", user, channel, text)

#     if not text:
#         return JSONResponse({"response_type": "ephemeral", "text": "Usage: /remember <text to save to Rune>"})

#     from .detector import DetectionResult
#     import time as _time

#     result = DetectionResult(is_significant=True, confidence=1.0, matched_pattern="manual capture")
#     raw_event = RawEvent(
#         text=text,
#         user=user,
#         channel=channel,
#         timestamp=str(_time.time()),
#         source=source,
#     )

#     logger.info("[/slack/remember] Building record...")
#     language = detect_language(text)
#     record = record_builder.build(raw_event, result, language=language)
#     logger.info("[/slack/remember] Record built: %s — queuing background store", record.id)

#     background_tasks.add_task(store_record, record)
#     return JSONResponse({"response_type": "ephemeral", "text": "Captured to Rune memory."})

@app.get("/stats")
async def get_stats():
    """Get Scribe statistics"""
    global review_queue, detector

    stats = {
        "service": "scribe",
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }

    if review_queue:
        stats["review_queue"] = review_queue.get_stats()

    if detector:
        stats["pipeline"] = {
            "tier1_threshold": detector.threshold,
            "tier1_patterns": detector._cache.pattern_count,
            "tier2_enabled": tier2_filter.is_available if tier2_filter else False,
            "tier3_enabled": record_builder._llm_extractor.is_available if record_builder and record_builder._llm_extractor else False,
            "auto_capture_threshold": detector.high_confidence_threshold,
        }

    return stats


# =============================================================================
# CLI Entry Point
# =============================================================================

def run_server():
    """Run the Scribe server"""
    import uvicorn

    config = load_config()
    port = config.scribe.slack_webhook_port

    logger.info("Starting server on port %d", port)
    uvicorn.run(
        "agents.scribe.server:app",
        host="0.0.0.0",
        port=port,
        reload=False,
    )


if __name__ == "__main__":
    run_server()
