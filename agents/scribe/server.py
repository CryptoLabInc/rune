"""
Scribe Server

FastAPI server for receiving webhooks and capturing organizational context.

Endpoints:
- POST /slack/events: Slack webhook endpoint
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
from datetime import datetime
from typing import Optional
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request, HTTPException, Header, BackgroundTasks
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from ..common.config import load_config, RuneConfig, ensure_directories
from ..common.embedding_service import EmbeddingService, get_embedding_service
from ..common.envector_client import EnVectorClient
from ..common.pattern_cache import PatternCache
from ..common.language import detect_language
from .pattern_parser import load_default_patterns, load_all_language_patterns
from .detector import DecisionDetector
from .record_builder import RecordBuilder, RawEvent
from .llm_extractor import LLMExtractor
from .review_queue import ReviewQueue, ReviewAnswers, ReviewAnswer
from .tier2_filter import Tier2Filter
from .handlers import SlackHandler, Message


# Global state
config: Optional[RuneConfig] = None
detector: Optional[DecisionDetector] = None
tier2_filter: Optional[Tier2Filter] = None
record_builder: Optional[RecordBuilder] = None
envector_client: Optional[EnVectorClient] = None
review_queue: Optional[ReviewQueue] = None
slack_handler: Optional[SlackHandler] = None
embedding_service: Optional[EmbeddingService] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize components on startup"""
    global config, detector, tier2_filter, record_builder, envector_client, review_queue
    global slack_handler, embedding_service

    print("[Scribe] Starting up...")

    # Ensure directories exist
    ensure_directories()

    # Load config
    config = load_config()
    print(f"[Scribe] Loaded config (state: {config.state})")

    # Initialize embedding service
    print("[Scribe] Initializing embedding service...")
    embedding_service = get_embedding_service(
        mode=config.embedding.mode,
        model=config.embedding.model
    )

    # Load and embed patterns (including multilingual)
    print("[Scribe] Loading patterns...")
    patterns = load_all_language_patterns()
    print(f"[Scribe] Found {len(patterns)} patterns")

    pattern_cache = PatternCache(embedding_service)
    pattern_cache.load_patterns(patterns)

    # Initialize detector
    detector = DecisionDetector(
        pattern_cache=pattern_cache,
        threshold=config.scribe.similarity_threshold,
        high_confidence_threshold=config.scribe.auto_capture_threshold
    )
    print(f"[Scribe] Detector ready (threshold: {config.scribe.similarity_threshold})")

    # Initialize Tier 2 LLM filter (Haiku — cheap, fast)
    api_key = config.retriever.anthropic_api_key or None
    if config.scribe.tier2_enabled and api_key:
        tier2_filter = Tier2Filter(
            anthropic_api_key=api_key,
            model=config.scribe.tier2_model,
        )
        if tier2_filter.is_available:
            print(f"[Scribe] Tier 2 LLM filter ready ({config.scribe.tier2_model})")
        else:
            print("[Scribe] Tier 2 LLM filter init failed (Tier 1 only)")
    else:
        tier2_filter = None
        print("[Scribe] Tier 2 LLM filter disabled" if not config.scribe.tier2_enabled else "[Scribe] Tier 2 skipped (no API key)")

    # Initialize Tier 3 LLM extractor (Sonnet — for record building)
    llm_extractor = LLMExtractor(
        anthropic_api_key=api_key,
        model=config.retriever.anthropic_model,
    )
    if llm_extractor.is_available:
        print("[Scribe] Tier 3 LLM extractor ready (Sonnet)")
    else:
        print("[Scribe] Tier 3 LLM extractor not available (regex fallback)")

    # Initialize record builder
    record_builder = RecordBuilder(llm_extractor=llm_extractor)

    # Initialize review queue
    review_queue = ReviewQueue()
    stats = review_queue.get_stats()
    print(f"[Scribe] Review queue: {stats['pending']} pending")

    # Initialize handlers
    slack_handler = SlackHandler(signing_secret=config.scribe.slack_signing_secret)

    # Initialize enVector client (if configured)
    if config.envector.endpoint:
        try:
            envector_client = EnVectorClient(
                address=config.envector.endpoint,
                access_token=config.envector.api_key or None,
            )
            print(f"[Scribe] EnVector client ready ({config.envector.endpoint})")
        except Exception as e:
            print(f"[Scribe] Warning: EnVector client failed: {e}")
            envector_client = None

    print("[Scribe] Ready to receive events")

    yield

    # Cleanup
    print("[Scribe] Shutting down...")


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

async def process_message(message: Message):
    """
    Process a message through the 3-tier capture pipeline.

    Tier 1: Embedding similarity (local, zero tokens) — wide net
    Tier 2: LLM policy filter (Haiku, ~200 tokens) — false positive removal
    Tier 3: LLM extraction (Sonnet, ~500 tokens) — Decision Record building
    """
    global detector, tier2_filter, record_builder, envector_client, review_queue, embedding_service

    if not detector or not record_builder:
        print("[Scribe] Not initialized, skipping message")
        return

    # === Tier 1: Embedding similarity (local, free) ===
    result = detector.detect(message.text)

    if not result.is_significant:
        return  # Not significant, ignore

    print(f"[Scribe] Tier 1 PASS (score: {result.confidence:.2f}, pattern: \"{result.matched_pattern[:50]}...\")")

    # === Tier 2: LLM policy filter (Haiku, cheap) ===
    if tier2_filter and tier2_filter.is_available:
        filter_result = tier2_filter.evaluate(
            text=message.text,
            tier1_score=result.confidence,
            tier1_pattern=result.matched_pattern or "",
        )

        if not filter_result.should_capture:
            print(f"[Scribe] Tier 2 REJECT: {filter_result.reason}")
            return

        print(f"[Scribe] Tier 2 PASS: {filter_result.reason}")

        # Use Tier 2's domain hint if Tier 1's is generic
        if filter_result.domain != "general" and result.domain in (None, "general"):
            result.domain = filter_result.domain
    else:
        print("[Scribe] Tier 2 skipped (filter unavailable)")

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

    print(f"[Scribe] Tier 3 built record: {record.id} (certainty: {record.why.certainty.value})")

    # Decide: auto-capture or review queue
    if detector.should_auto_capture(result):
        await store_record(record)
    else:
        review_queue.add(record, result.confidence)
        print(f"[Scribe] Added to review queue: {record.id}")


async def store_record(record):
    """Store a Decision Record to enVector"""
    global envector_client, embedding_service, config

    if not envector_client or not embedding_service:
        print(f"[Scribe] Cannot store (no enVector client): {record.id}")
        return

    try:
        # The payload.text is what we embed
        text = record.payload.text
        metadata = record.model_dump(mode='json')

        # Insert to enVector
        result = envector_client.insert_with_text(
            index_name=config.envector.collection,
            texts=[text],
            embedding_service=embedding_service,
            metadata=[metadata]
        )

        if result.get("ok"):
            print(f"[Scribe] Stored: {record.id}")
        else:
            print(f"[Scribe] Failed to store {record.id}: {result.get('error')}")

    except Exception as e:
        print(f"[Scribe] Error storing {record.id}: {e}")


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
    if slack_handler.is_url_verification(data):
        challenge = slack_handler.get_challenge(data)
        return JSONResponse({"challenge": challenge})

    # Parse event to message
    message = await slack_handler.parse_event(data)

    if message and slack_handler.should_process(message):
        # Process in background (don't block response)
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


@app.get("/stats")
async def get_stats():
    """Get Scribe statistics"""
    global review_queue, detector

    stats = {
        "service": "scribe",
        "timestamp": datetime.utcnow().isoformat(),
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

    print(f"[Scribe] Starting server on port {port}")
    uvicorn.run(
        "agents.scribe.server:app",
        host="0.0.0.0",
        port=port,
        reload=False,
    )


if __name__ == "__main__":
    run_server()
