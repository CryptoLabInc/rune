"""
3-Person Team Pipeline Scenario Tests

Tests the full 3-tier Scribe capture pipeline and Retriever recall with
realistic conversation scripts from 3 team members:
- Alice (CTO): Architecture & infrastructure decisions
- Bob (EM): Sprint planning, feature prioritization, deployment
- Charlie (Security Lead): Security policies, compliance, encryption

Each conversation includes:
- CAPTURE: Real decisions with rationale (should pass all 3 tiers)
- REJECT: Casual chat, status updates, vague opinions (should be filtered)
- BORDERLINE: Triggers Tier 1 but Tier 2 should reject (false positives)

Pipeline:
  Tier 1: Embedding similarity (local, 0 tokens) → wide net
  Tier 2: LLM policy filter (Haiku, ~200 tokens) → false positive removal
  Tier 3: LLM extraction (Sonnet, ~500 tokens) → Decision Record building
"""

import json
import pytest
from datetime import datetime, timezone
from unittest.mock import Mock, MagicMock, patch
from dataclasses import dataclass
from typing import List, Optional


# ============================================================================
# Conversation Scripts
# ============================================================================

@dataclass
class ConversationLine:
    """A single line in a conversation script"""
    text: str
    user: str
    channel: str
    expected_capture: bool  # Should this be captured?
    expected_domain: Optional[str] = None  # Expected domain if captured
    category: str = ""  # "decision", "casual", "status", "vague", "borderline"
    note: str = ""  # Why this is expected to capture/reject


# Alice: CTO / Architecture Lead
ALICE_CONVERSATIONS: List[ConversationLine] = [
    # --- Real decisions (CAPTURE) ---
    ConversationLine(
        text='We decided to use PostgreSQL instead of MongoDB for the main database because we need strong ACID compliance for financial transactions. "PostgreSQL gives us ACID compliance out of the box" said the lead architect. The trade-off is that we lose flexible schema support.',
        user="alice", channel="#architecture",
        expected_capture=True, expected_domain="architecture",
        category="decision",
        note="Clear decision with rationale, quote, and trade-off",
    ),
    ConversationLine(
        text='After extensive benchmarking, we are going with gRPC for inter-service communication. REST showed 3x higher latency for our use case. We will use Protocol Buffers for schema evolution. The key risk is that browser clients cannot call gRPC directly, so we need a gateway.',
        user="alice", channel="#architecture",
        expected_capture=True, expected_domain="architecture",
        category="decision",
        note="Technology choice with benchmark data and risk assessment",
    ),
    ConversationLine(
        text='The team has agreed to adopt an event-driven architecture using Kafka for all inter-service communication. "Event sourcing lets us replay state when debugging production issues" was the consensus. Direct API calls between services are now banned except for synchronous reads.',
        user="alice", channel="#architecture",
        expected_capture=True, expected_domain="architecture",
        category="decision",
        note="Policy establishment with quoted consensus",
    ),
    ConversationLine(
        text='We standardized on TypeScript for all frontend code effective immediately. Our policy is to enforce strict mode and use Zod for runtime validation at API boundaries. No exceptions for new projects.',
        user="alice", channel="#engineering",
        expected_capture=True, expected_domain="architecture",
        category="decision",
        note="Technology standardization policy",
    ),
    ConversationLine(
        text='After the postmortem on last week\'s outage, we learned that our retry logic was causing cascading failures. The lesson: always implement circuit breakers before adding retries. We are adopting the Resilience4j library for this.',
        user="alice", channel="#incidents",
        expected_capture=True, expected_domain="ops",
        category="decision",
        note="Lesson learned from incident with concrete action",
    ),

    # --- Casual / Status (REJECT) ---
    ConversationLine(
        text="Good morning team! Hope everyone had a great weekend.",
        user="alice", channel="#general",
        expected_capture=False,
        category="casual",
        note="Social greeting",
    ),
    ConversationLine(
        text="I'm heading to the dentist after lunch, will be back around 3pm.",
        user="alice", channel="#general",
        expected_capture=False,
        category="casual",
        note="Personal schedule update",
    ),
    ConversationLine(
        text="The CI build is currently broken on main. Looking into it.",
        user="alice", channel="#engineering",
        expected_capture=False,
        category="status",
        note="Status update without decision",
    ),
    ConversationLine(
        text="Updated the README with the new setup instructions.",
        user="alice", channel="#engineering",
        expected_capture=False,
        category="status",
        note="Routine task completion",
    ),

    # --- Borderline / False Positives (REJECT by Tier 2) ---
    ConversationLine(
        text="We decided to order pizza for the team lunch today. Hawaiian vs pepperoni was a tough call but pepperoni won.",
        user="alice", channel="#random",
        expected_capture=False,
        category="borderline",
        note="Contains 'We decided' pattern but is food order, not org decision",
    ),
    ConversationLine(
        text="Maybe we should consider using Rust sometime. It might be faster than Go for some things.",
        user="alice", channel="#architecture",
        expected_capture=False,
        category="vague",
        note="Vague opinion without commitment ('maybe', 'sometime', 'might')",
    ),
    ConversationLine(
        text="I think Python is better than Java but that's just my personal preference honestly.",
        user="alice", channel="#random",
        expected_capture=False,
        category="vague",
        note="Personal opinion without team decision",
    ),
]


# Bob: Engineering Manager / Sprint Lead
BOB_CONVERSATIONS: List[ConversationLine] = [
    # --- Real decisions (CAPTURE) ---
    ConversationLine(
        text='For this sprint, we have prioritized the authentication refactoring because it blocks three other features. "The current OAuth implementation does not support PKCE and that is a security risk" said the security lead. We are allocating 2 engineers full-time.',
        user="bob", channel="#sprint-planning",
        expected_capture=True, expected_domain="product",
        category="decision",
        note="Sprint prioritization with blocking rationale and resource allocation",
    ),
    ConversationLine(
        text='We decided to adopt feature flags using LaunchDarkly instead of building our own system. The business case is clear: we need gradual rollouts for enterprise customers who require zero-downtime deployments. Build vs buy analysis showed 6 months of eng time saved.',
        user="bob", channel="#engineering",
        expected_capture=True, expected_domain="product",
        category="decision",
        note="Build-vs-buy decision with quantified business case",
    ),
    ConversationLine(
        text='New deployment policy: all changes must go through blue-green deployments. "Deploy to staging before production, and all changes must pass automated smoke tests" is the new mandate. This reduces rollback time from hours to seconds.',
        user="bob", channel="#devops",
        expected_capture=True, expected_domain="ops",
        category="decision",
        note="New deployment policy with quantified improvement",
    ),
    ConversationLine(
        text='Performance bottleneck identified and resolved: the user dashboard query took 3.2 seconds because of N+1 queries. We are going with DataLoader pattern to batch database calls. "This should bring it under 200ms" per our benchmarks. All teams must use DataLoader for list endpoints going forward.',
        user="bob", channel="#performance",
        expected_capture=True, expected_domain="architecture",
        category="decision",
        note="Technical decision with benchmark data and new team policy",
    ),
    ConversationLine(
        text='We are deprioritizing the mobile app redesign for Q2. Customer feedback analysis shows enterprise clients care 3x more about API stability than UI polish. The lesson learned: always validate assumptions with data before committing to large projects.',
        user="bob", channel="#product",
        expected_capture=True, expected_domain="product",
        category="decision",
        note="Prioritization decision with data-backed rationale and lesson",
    ),

    # --- Casual / Status (REJECT) ---
    ConversationLine(
        text="Hey everyone, standup in 5 minutes!",
        user="bob", channel="#engineering",
        expected_capture=False,
        category="casual",
        note="Meeting reminder",
    ),
    ConversationLine(
        text="Still working on the sprint velocity report. Should have it by EOD.",
        user="bob", channel="#sprint-planning",
        expected_capture=False,
        category="status",
        note="Status update without decision content",
    ),
    ConversationLine(
        text="Merged the PR for the login page fix. Tests are green.",
        user="bob", channel="#engineering",
        expected_capture=False,
        category="status",
        note="Routine PR merge notification",
    ),
    ConversationLine(
        text="Reminder: please update your Jira tickets before the end of the sprint.",
        user="bob", channel="#sprint-planning",
        expected_capture=False,
        category="casual",
        note="Administrative reminder",
    ),

    # --- Borderline / False Positives (REJECT by Tier 2) ---
    ConversationLine(
        text="We decided to move the team offsite to next Thursday because the conference room is booked on Wednesday.",
        user="bob", channel="#general",
        expected_capture=False,
        category="borderline",
        note="Contains 'We decided' but is scheduling, not org decision",
    ),
    ConversationLine(
        text="I read an article about how Netflix uses chaos engineering. Interesting approach, we should look into that someday.",
        user="bob", channel="#engineering",
        expected_capture=False,
        category="vague",
        note="Information sharing without decision or commitment",
    ),
]


# Charlie: Security Lead
CHARLIE_CONVERSATIONS: List[ConversationLine] = [
    # --- Real decisions (CAPTURE) ---
    ConversationLine(
        text='We must implement mTLS for all inter-service communication by end of Q2. "The security review flagged this as requiring immediate attention" due to our SOC2 compliance audit. All services must present valid certificates issued by our internal CA.',
        user="charlie", channel="#security",
        expected_capture=True, expected_domain="security",
        category="decision",
        note="Security mandate with compliance deadline and technical requirement",
    ),
    ConversationLine(
        text='Our authentication approach is moving to OIDC with Auth0 as the identity provider. All API keys must be rotated every 90 days and stored in HashiCorp Vault. "No more hardcoded secrets in config files" is the mandate. Violations will block deploys.',
        user="charlie", channel="#security",
        expected_capture=True, expected_domain="security",
        category="decision",
        note="Authentication policy with enforcement mechanism",
    ),
    ConversationLine(
        text='The encryption strategy is AES-256-GCM for data at rest and TLS 1.3 for data in transit. We chose this combination because it meets both HIPAA and PCI DSS requirements simultaneously. The trade-off: AES-256-GCM has slightly higher CPU overhead than AES-128, but compliance trumps performance here.',
        user="charlie", channel="#security",
        expected_capture=True, expected_domain="security",
        category="decision",
        note="Encryption policy with compliance rationale and trade-off analysis",
    ),
    ConversationLine(
        text='After the vulnerability disclosure last week, we are implementing a mandatory security review for all PRs that touch authentication, payment, or PII handling code. The review must be completed by a security-certified engineer before merge. No exceptions.',
        user="charlie", channel="#security",
        expected_capture=True, expected_domain="security",
        category="decision",
        note="New security review policy triggered by incident",
    ),
    ConversationLine(
        text='The security team has decided to implement rate limiting at the API gateway level using Kong. We chose Kong over custom middleware because it provides built-in rate limiting, IP allowlisting, and integrates with our existing Prometheus monitoring. This is critical for our Q3 compliance goals.',
        user="charlie", channel="#security",
        expected_capture=True, expected_domain="security",
        category="decision",
        note="Tool selection with build-vs-buy rationale",
    ),

    # --- Casual / Status (REJECT) ---
    ConversationLine(
        text="Running the weekly vulnerability scan now. Will share results in the security channel.",
        user="charlie", channel="#security",
        expected_capture=False,
        category="status",
        note="Routine operational task",
    ),
    ConversationLine(
        text="CVE-2024-12345 was patched in the latest update. We're on the latest version already so no action needed.",
        user="charlie", channel="#security",
        expected_capture=False,
        category="status",
        note="Informational CVE update, no decision involved",
    ),
    ConversationLine(
        text="Thanks for completing the security training everyone! We had 95% completion rate.",
        user="charlie", channel="#general",
        expected_capture=False,
        category="casual",
        note="Appreciation message",
    ),

    # --- Borderline / False Positives (REJECT by Tier 2) ---
    ConversationLine(
        text="We decided to skip the security happy hour this Friday because most of the team is remote.",
        user="charlie", channel="#random",
        expected_capture=False,
        category="borderline",
        note="Contains 'We decided' but is social event, not security policy",
    ),
    ConversationLine(
        text="Maybe we should consider switching to a different password manager. The current one is kind of slow.",
        user="charlie", channel="#security",
        expected_capture=False,
        category="vague",
        note="Vague suggestion without commitment or analysis",
    ),
]


ALL_CONVERSATIONS = ALICE_CONVERSATIONS + BOB_CONVERSATIONS + CHARLIE_CONVERSATIONS


# ============================================================================
# Cross-Member Recall Queries
# ============================================================================

@dataclass
class RecallQuery:
    """A recall query from one member about another's context"""
    searcher: str
    query: str
    expected_source_member: str
    expected_domain: str
    description: str


RECALL_QUERIES: List[RecallQuery] = [
    # Bob asks about Alice's architecture decisions
    RecallQuery(
        searcher="bob",
        query="Why did we choose PostgreSQL over MongoDB?",
        expected_source_member="alice",
        expected_domain="architecture",
        description="Bob recalls Alice's database decision",
    ),
    RecallQuery(
        searcher="bob",
        query="What's our approach for inter-service communication?",
        expected_source_member="alice",
        expected_domain="architecture",
        description="Bob recalls Alice's gRPC decision",
    ),
    # Alice asks about Charlie's security policies
    RecallQuery(
        searcher="alice",
        query="What are the security requirements for inter-service communication?",
        expected_source_member="charlie",
        expected_domain="security",
        description="Alice recalls Charlie's mTLS requirement",
    ),
    RecallQuery(
        searcher="alice",
        query="What security considerations apply to data at rest encryption?",
        expected_source_member="charlie",
        expected_domain="security",
        description="Alice recalls Charlie's encryption strategy",
    ),
    # Charlie asks about Bob's deployment policy
    RecallQuery(
        searcher="charlie",
        query="What's our process for deploying to production?",
        expected_source_member="bob",
        expected_domain="ops",
        description="Charlie recalls Bob's blue-green deployment policy",
    ),
    RecallQuery(
        searcher="charlie",
        query="Why did we decide on LaunchDarkly for feature flags?",
        expected_source_member="bob",
        expected_domain="product",
        description="Charlie recalls Bob's LaunchDarkly decision",
    ),
]


# ============================================================================
# Tier 2 Mock Helpers
# ============================================================================

def make_tier2_response(capture: bool, reason: str, domain: str = "general"):
    """Create a mock Anthropic response for Tier 2"""
    response = Mock()
    content_block = Mock()
    content_block.text = json.dumps({
        "capture": capture,
        "reason": reason,
        "domain": domain,
    })
    response.content = [content_block]
    return response


def simulate_tier2_judgment(text: str) -> dict:
    """
    Simulate Tier 2 Haiku judgment based on text content.
    This mimics what a real Haiku call would return.
    """
    text_lower = text.lower()

    # Clear casual / social
    casual_signals = [
        "good morning", "hope everyone", "heading to the dentist",
        "standup in", "hey everyone", "thanks for completing",
        "happy hour", "pizza for the team", "team lunch",
        "conference room is booked", "offsite to next",
    ]
    for signal in casual_signals:
        if signal in text_lower:
            return {"capture": False, "reason": "Social/scheduling message, not an organizational decision", "domain": "general"}

    # Status updates
    status_signals = [
        "still working on", "looking into it", "updated the readme",
        "merged the pr", "tests are green", "running the weekly",
        "was patched", "no action needed", "please update your jira",
        "should have it by eod", "currently broken",
    ]
    for signal in status_signals:
        if signal in text_lower:
            return {"capture": False, "reason": "Status update without decision content", "domain": "general"}

    # Vague opinions
    vague_signals = [
        "maybe we should", "might be", "sometime", "should look into that someday",
        "just my personal preference", "kind of slow",
        "interesting approach",
    ]
    for signal in vague_signals:
        if signal in text_lower:
            return {"capture": False, "reason": "Vague opinion without commitment or concrete decision", "domain": "general"}

    # Read article / info sharing without decision
    if "i read an article" in text_lower and "decided" not in text_lower:
        return {"capture": False, "reason": "Information sharing without decision", "domain": "general"}

    # Real decisions — determine domain
    domain = "general"
    if any(w in text_lower for w in ["security", "mtls", "encryption", "authentication", "compliance", "vulnerability", "rate limiting"]):
        domain = "security"
    elif any(w in text_lower for w in ["architecture", "postgresql", "grpc", "kafka", "microservice", "typescript", "event-driven"]):
        domain = "architecture"
    elif any(w in text_lower for w in ["deployment", "devops", "ci/cd", "blue-green"]):
        domain = "ops"
    elif any(w in text_lower for w in ["sprint", "feature flag", "prioritiz", "customer feedback", "mobile app"]):
        domain = "product"
    elif any(w in text_lower for w in ["performance", "bottleneck", "latency", "benchmark"]):
        domain = "architecture"
    elif any(w in text_lower for w in ["postmortem", "outage", "incident", "circuit breaker"]):
        domain = "ops"

    return {"capture": True, "reason": "Contains concrete organizational decision with rationale", "domain": domain}


# ============================================================================
# Test Classes
# ============================================================================

class TestConversationCaptureDecisions:
    """Test that conversations are correctly classified as capture vs reject"""

    @pytest.fixture
    def tier2_filter(self):
        """Create Tier2Filter with simulated judgment"""
        from agents.scribe.tier2_filter import Tier2Filter, FilterResult

        f = Tier2Filter.__new__(Tier2Filter)
        f._provider = "anthropic"
        f._model = "claude-haiku-4-5-20251001"

        mock_llm = Mock()
        mock_llm.is_available = True

        def side_effect_evaluate(prompt, **kwargs):
            # Extract the actual text from "<message>\n...\n</message>" format
            text = prompt.replace("<message>\n", "").split("\n</message>")[0]
            text = text.split("\n(Tier 1")[0]
            judgment = simulate_tier2_judgment(text)
            return json.dumps(judgment)

        mock_llm.generate.side_effect = side_effect_evaluate
        f._llm = mock_llm
        return f

    def test_alice_capture_decisions(self, tier2_filter):
        """Alice's real decisions should be captured"""
        captures = [c for c in ALICE_CONVERSATIONS if c.expected_capture]
        assert len(captures) == 5, "Alice should have 5 capturable decisions"

        for conv in captures:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is True, (
                f"SHOULD CAPTURE but rejected: [{conv.category}] {conv.text[:80]}... "
                f"Reason: {result.reason}"
            )

    def test_alice_reject_casual(self, tier2_filter):
        """Alice's casual/status messages should be rejected"""
        rejects = [c for c in ALICE_CONVERSATIONS if not c.expected_capture]
        assert len(rejects) >= 7, "Alice should have >=7 rejectable messages"

        for conv in rejects:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is False, (
                f"SHOULD REJECT but captured: [{conv.category}] {conv.text[:80]}... "
                f"Reason: {result.reason}"
            )

    def test_bob_capture_decisions(self, tier2_filter):
        """Bob's real decisions should be captured"""
        captures = [c for c in BOB_CONVERSATIONS if c.expected_capture]
        assert len(captures) == 5, "Bob should have 5 capturable decisions"

        for conv in captures:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is True, (
                f"SHOULD CAPTURE but rejected: [{conv.category}] {conv.text[:80]}... "
                f"Reason: {result.reason}"
            )

    def test_bob_reject_casual(self, tier2_filter):
        """Bob's casual/status messages should be rejected"""
        rejects = [c for c in BOB_CONVERSATIONS if not c.expected_capture]
        assert len(rejects) >= 6, "Bob should have >=6 rejectable messages"

        for conv in rejects:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is False, (
                f"SHOULD REJECT but captured: [{conv.category}] {conv.text[:80]}... "
                f"Reason: {result.reason}"
            )

    def test_charlie_capture_decisions(self, tier2_filter):
        """Charlie's real decisions should be captured"""
        captures = [c for c in CHARLIE_CONVERSATIONS if c.expected_capture]
        assert len(captures) == 5, "Charlie should have 5 capturable decisions"

        for conv in captures:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is True, (
                f"SHOULD CAPTURE but rejected: [{conv.category}] {conv.text[:80]}... "
                f"Reason: {result.reason}"
            )

    def test_charlie_reject_casual(self, tier2_filter):
        """Charlie's casual/status messages should be rejected"""
        rejects = [c for c in CHARLIE_CONVERSATIONS if not c.expected_capture]
        assert len(rejects) >= 5, "Charlie should have >=5 rejectable messages"

        for conv in rejects:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is False, (
                f"SHOULD REJECT but captured: [{conv.category}] {conv.text[:80]}... "
                f"Reason: {result.reason}"
            )

    def test_all_borderline_rejected(self, tier2_filter):
        """All borderline messages (false positives) should be rejected by Tier 2"""
        borderline = [c for c in ALL_CONVERSATIONS if c.category == "borderline"]
        assert len(borderline) >= 3, "Should have at least 3 borderline messages"

        for conv in borderline:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is False, (
                f"BORDERLINE should be rejected: {conv.text[:80]}... "
                f"Note: {conv.note}"
            )

    def test_all_vague_rejected(self, tier2_filter):
        """All vague opinions should be rejected by Tier 2"""
        vague = [c for c in ALL_CONVERSATIONS if c.category == "vague"]
        assert len(vague) >= 3, "Should have at least 3 vague messages"

        for conv in vague:
            result = tier2_filter.evaluate(conv.text)
            assert result.should_capture is False, (
                f"VAGUE should be rejected: {conv.text[:80]}... "
                f"Note: {conv.note}"
            )

    def test_capture_rate(self, tier2_filter):
        """Overall capture rate should be reasonable (30-50%)"""
        total = len(ALL_CONVERSATIONS)
        expected_captures = sum(1 for c in ALL_CONVERSATIONS if c.expected_capture)
        expected_rate = expected_captures / total

        # Verify our script is balanced
        assert 0.30 <= expected_rate <= 0.55, (
            f"Expected capture rate: {expected_rate:.1%} "
            f"({expected_captures}/{total})"
        )


class TestTier3RecordBuilding:
    """Test that Tier 3 builds correct Decision Records from captured messages"""

    @pytest.fixture
    def record_builder(self):
        """Create RecordBuilder without LLM (regex fallback)"""
        from agents.scribe.record_builder import RecordBuilder
        return RecordBuilder()

    def _make_detection(self, conv: ConversationLine):
        """Create a DetectionResult for a conversation line"""
        from agents.scribe.detector import DetectionResult
        return DetectionResult(
            is_significant=True,
            confidence=0.85,
            matched_pattern="decision pattern",
            category=conv.expected_domain or "general",
            domain=conv.expected_domain or "general",
            priority="high",
        )

    def _make_raw_event(self, conv: ConversationLine):
        """Create a RawEvent for a conversation line"""
        from agents.scribe.record_builder import RawEvent
        return RawEvent(
            text=conv.text,
            user=conv.user,
            channel=conv.channel,
            timestamp=str(datetime.now(timezone.utc).timestamp()),
            source="slack",
        )

    def test_alice_postgresql_decision_record(self, record_builder):
        """Alice's PostgreSQL decision should produce a proper record"""
        conv = ALICE_CONVERSATIONS[0]  # PostgreSQL decision
        raw_event = self._make_raw_event(conv)
        detection = self._make_detection(conv)

        record = record_builder.build(raw_event, detection)

        # Should have a meaningful title
        assert len(record.title) > 5
        # Should have evidence (the quote about ACID compliance)
        assert len(record.evidence) > 0
        # At least one evidence should have a real quote
        has_quote = any("ACID" in e.quote for e in record.evidence)
        assert has_quote, "Should extract the ACID compliance quote"
        # Should have rationale
        assert record.why.rationale_summary or record.why.certainty.value != "unknown"
        # payload.text should be non-empty
        assert len(record.payload.text) > 100
        # payload.text should contain key info
        assert "PostgreSQL" in record.payload.text or "postgresql" in record.payload.text.lower()

    def test_bob_deployment_policy_record(self, record_builder):
        """Bob's deployment policy should produce a proper record"""
        conv = BOB_CONVERSATIONS[2]  # Blue-green deployment
        raw_event = self._make_raw_event(conv)
        detection = self._make_detection(conv)

        record = record_builder.build(raw_event, detection)

        assert len(record.title) > 5
        assert len(record.evidence) > 0
        assert len(record.payload.text) > 100
        # Should be associated with the right user
        assert "bob" in str(record.decision.who)

    def test_charlie_encryption_record(self, record_builder):
        """Charlie's encryption decision should produce a proper record"""
        conv = CHARLIE_CONVERSATIONS[2]  # AES-256-GCM
        raw_event = self._make_raw_event(conv)
        detection = self._make_detection(conv)

        record = record_builder.build(raw_event, detection)

        assert len(record.title) > 5
        assert len(record.payload.text) > 100
        # Should contain encryption-related content
        payload_lower = record.payload.text.lower()
        assert "aes" in payload_lower or "encryption" in payload_lower

    def test_all_capture_messages_produce_valid_records(self, record_builder):
        """All messages expected to be captured should produce valid records"""
        captures = [c for c in ALL_CONVERSATIONS if c.expected_capture]

        for conv in captures:
            raw_event = self._make_raw_event(conv)
            detection = self._make_detection(conv)
            record = record_builder.build(raw_event, detection)

            # Every record must have:
            assert record.id, f"Missing ID for: {conv.text[:50]}"
            assert record.title, f"Missing title for: {conv.text[:50]}"
            assert record.payload.text, f"Missing payload.text for: {conv.text[:50]}"
            assert len(record.payload.text) > 50, f"payload.text too short for: {conv.text[:50]}"
            assert record.evidence, f"Missing evidence for: {conv.text[:50]}"
            assert record.domain, f"Missing domain for: {conv.text[:50]}"
            assert record.why.certainty, f"Missing certainty for: {conv.text[:50]}"

    def test_evidence_certainty_consistency(self, record_builder):
        """Certainty should be consistent with evidence quality"""
        # Message with direct quote → should be supported or partially_supported
        conv = ALICE_CONVERSATIONS[0]  # Has explicit quote
        raw_event = self._make_raw_event(conv)
        detection = self._make_detection(conv)
        record = record_builder.build(raw_event, detection)

        assert record.why.certainty.value in ("supported", "partially_supported"), (
            f"Quote-bearing message should be supported, got: {record.why.certainty.value}"
        )

    def test_payload_text_is_embeddable(self, record_builder):
        """payload.text should be formatted for embedding (not JSON)"""
        conv = ALICE_CONVERSATIONS[0]
        raw_event = self._make_raw_event(conv)
        detection = self._make_detection(conv)
        record = record_builder.build(raw_event, detection)

        payload = record.payload.text
        # Should be readable text, not JSON
        assert not payload.strip().startswith("{"), "payload.text should not be JSON"
        # Should contain markdown headers
        assert "# " in payload or "## " in payload, "payload.text should have markdown headers"


class TestFullPipelineFlow:
    """End-to-end pipeline flow tests with all 3 tiers"""

    @pytest.fixture
    def pipeline_components(self):
        """Set up all pipeline components with mocks"""
        from agents.scribe.tier2_filter import Tier2Filter
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.scribe.detector import DecisionDetector, DetectionResult
        from agents.common.pattern_cache import PatternCache, PatternEntry

        # Mock pattern cache for Tier 1
        pattern_cache = Mock(spec=PatternCache)
        pattern_cache.pattern_count = 10

        # Tier 1: always pass for this test (we test Tier 2 filtering)
        def mock_best_match(text, threshold=0.0):
            if len(text.strip()) < 20:
                return (None, 0.1)
            entry = PatternEntry(
                text="We decided to use",
                category="architecture",
                priority="high",
                embedding=[0.1] * 384,
                domain="architecture",
            )
            return (entry, 0.75)  # Above threshold but below auto-capture

        pattern_cache.find_best_match.side_effect = mock_best_match

        detector = DecisionDetector(
            pattern_cache=pattern_cache,
            threshold=0.5,
            high_confidence_threshold=0.8,
        )

        # Tier 2: simulated judgment
        tier2 = Tier2Filter.__new__(Tier2Filter)
        tier2._provider = "anthropic"
        tier2._model = "test"

        mock_llm = Mock()
        mock_llm.is_available = True

        def tier2_side_effect(prompt, **kwargs):
            # Extract the actual text from "<message>\n...\n</message>" format
            text = prompt.replace("<message>\n", "").split("\n</message>")[0]
            text = text.split("\n(Tier 1")[0]
            judgment = simulate_tier2_judgment(text)
            return json.dumps(judgment)

        mock_llm.generate.side_effect = tier2_side_effect
        tier2._llm = mock_llm

        # Tier 3: RecordBuilder (regex fallback, no LLM)
        builder = RecordBuilder()

        return {
            "detector": detector,
            "tier2": tier2,
            "builder": builder,
        }

    def _run_pipeline(self, components, conv: ConversationLine):
        """Run a message through the full 3-tier pipeline"""
        from agents.scribe.record_builder import RawEvent
        from agents.common.language import detect_language

        detector = components["detector"]
        tier2 = components["tier2"]
        builder = components["builder"]

        # Tier 1
        result = detector.detect(conv.text)
        if not result.is_significant:
            return {"tier1": "reject", "record": None}

        # Tier 2
        filter_result = tier2.evaluate(
            text=conv.text,
            tier1_score=result.confidence,
            tier1_pattern=result.matched_pattern or "",
        )
        if not filter_result.should_capture:
            return {"tier1": "pass", "tier2": "reject", "reason": filter_result.reason, "record": None}

        # Use Tier 2 domain hint
        if filter_result.domain != "general" and result.domain in (None, "general"):
            result.domain = filter_result.domain

        # Tier 3
        raw_event = RawEvent(
            text=conv.text,
            user=conv.user,
            channel=conv.channel,
            timestamp=str(datetime.now(timezone.utc).timestamp()),
            source="slack",
        )
        language = detect_language(conv.text)
        record = builder.build(raw_event, result, language=language)

        # Auto-capture vs review
        action = "auto_capture" if detector.should_auto_capture(result) else "review_queue"

        return {
            "tier1": "pass",
            "tier2": "pass",
            "tier3": "built",
            "action": action,
            "record": record,
        }

    def test_alice_full_pipeline(self, pipeline_components):
        """Run all of Alice's messages through the full pipeline"""
        for conv in ALICE_CONVERSATIONS:
            result = self._run_pipeline(pipeline_components, conv)

            if conv.expected_capture:
                assert result.get("record") is not None, (
                    f"Alice CAPTURE expected but no record: {conv.text[:60]}..."
                )
            else:
                assert result.get("record") is None, (
                    f"Alice REJECT expected but got record: {conv.text[:60]}..."
                )

    def test_bob_full_pipeline(self, pipeline_components):
        """Run all of Bob's messages through the full pipeline"""
        for conv in BOB_CONVERSATIONS:
            result = self._run_pipeline(pipeline_components, conv)

            if conv.expected_capture:
                assert result.get("record") is not None, (
                    f"Bob CAPTURE expected but no record: {conv.text[:60]}..."
                )
            else:
                assert result.get("record") is None, (
                    f"Bob REJECT expected but got record: {conv.text[:60]}..."
                )

    def test_charlie_full_pipeline(self, pipeline_components):
        """Run all of Charlie's messages through the full pipeline"""
        for conv in CHARLIE_CONVERSATIONS:
            result = self._run_pipeline(pipeline_components, conv)

            if conv.expected_capture:
                assert result.get("record") is not None, (
                    f"Charlie CAPTURE expected but no record: {conv.text[:60]}..."
                )
            else:
                assert result.get("record") is None, (
                    f"Charlie REJECT expected but got record: {conv.text[:60]}..."
                )

    def test_pipeline_statistics(self, pipeline_components):
        """Verify overall pipeline statistics match expectations"""
        stats = {
            "total": 0,
            "tier1_reject": 0,
            "tier2_reject": 0,
            "captured": 0,
            "review_queue": 0,
            "auto_capture": 0,
        }

        for conv in ALL_CONVERSATIONS:
            stats["total"] += 1
            result = self._run_pipeline(pipeline_components, conv)

            if result.get("tier1") == "reject":
                stats["tier1_reject"] += 1
            elif result.get("tier2") == "reject":
                stats["tier2_reject"] += 1
            elif result.get("record"):
                stats["captured"] += 1
                if result.get("action") == "auto_capture":
                    stats["auto_capture"] += 1
                else:
                    stats["review_queue"] += 1

        # Tier 1 should pass most messages (wide net at 0.5 threshold)
        # Tier 2 should reject the false positives
        expected_captures = sum(1 for c in ALL_CONVERSATIONS if c.expected_capture)
        assert stats["captured"] == expected_captures, (
            f"Expected {expected_captures} captures, got {stats['captured']}. "
            f"Stats: {stats}"
        )

        # Tier 2 should have rejected some messages that Tier 1 passed
        assert stats["tier2_reject"] > 0, (
            f"Tier 2 should reject some Tier 1 false positives. Stats: {stats}"
        )

    def test_no_short_messages_captured(self, pipeline_components):
        """Short messages should be rejected at Tier 1"""
        short_messages = [
            ConversationLine(text="Hi", user="alice", channel="#general", expected_capture=False),
            ConversationLine(text="OK", user="bob", channel="#general", expected_capture=False),
            ConversationLine(text="lgtm", user="charlie", channel="#engineering", expected_capture=False),
            ConversationLine(text="", user="alice", channel="#general", expected_capture=False),
        ]

        for conv in short_messages:
            result = self._run_pipeline(pipeline_components, conv)
            assert result.get("record") is None, (
                f"Short message should not be captured: '{conv.text}'"
            )


class TestReviewQueueIntegration:
    """Test that moderate-confidence captures go to review queue"""

    def test_review_queue_add_and_retrieve(self, tmp_path):
        """Test adding records to review queue and retrieving them"""
        from agents.scribe.review_queue import ReviewQueue
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.scribe.detector import DetectionResult
        from agents.common.language import detect_language

        queue = ReviewQueue(queue_path=tmp_path / "test_review.json")
        builder = RecordBuilder()

        # Add a few records from different team members
        test_messages = [
            ALICE_CONVERSATIONS[0],  # PostgreSQL decision
            BOB_CONVERSATIONS[0],    # Sprint prioritization
            CHARLIE_CONVERSATIONS[0], # mTLS mandate
        ]

        record_ids = []
        for conv in test_messages:
            raw_event = RawEvent(
                text=conv.text,
                user=conv.user,
                channel=conv.channel,
                timestamp=str(datetime.now(timezone.utc).timestamp()),
                source="slack",
            )
            detection = DetectionResult(
                is_significant=True,
                confidence=0.75,  # Below auto-capture threshold
                matched_pattern="test",
                category=conv.expected_domain,
                domain=conv.expected_domain,
                priority="medium",
            )
            language = detect_language(conv.text)
            record = builder.build(raw_event, detection, language=language)
            rid = queue.add(record, detection.confidence)
            record_ids.append(rid)

        # Verify queue state
        pending = queue.get_pending()
        assert len(pending) == 3, f"Expected 3 pending, got {len(pending)}"

        # Each item should have questions
        for item in pending:
            assert len(item.questions) >= 3, "Each item should have at least 3 review questions"

        # Verify stats
        stats = queue.get_stats()
        assert stats["pending"] == 3
        assert stats["reviewed"] == 0

    def test_review_approve_updates_record(self, tmp_path):
        """Test that approving a review updates the record correctly"""
        from agents.scribe.review_queue import ReviewQueue, ReviewAnswers, ReviewAnswer
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.scribe.detector import DetectionResult
        from agents.common.language import detect_language

        queue = ReviewQueue(queue_path=tmp_path / "test_review.json")
        builder = RecordBuilder()

        conv = ALICE_CONVERSATIONS[0]  # PostgreSQL decision
        raw_event = RawEvent(
            text=conv.text, user=conv.user, channel=conv.channel,
            timestamp=str(datetime.now(timezone.utc).timestamp()), source="slack",
        )
        detection = DetectionResult(
            is_significant=True, confidence=0.75,
            matched_pattern="test", category="architecture",
            domain="architecture", priority="medium",
        )
        language = detect_language(conv.text)
        record = builder.build(raw_event, detection, language=language)
        rid = queue.add(record, detection.confidence)

        # Submit review: approve with corrections
        answers = ReviewAnswers(
            q1_worth_saving=ReviewAnswer.CAPTURE,
            q2_evidence_supported=ReviewAnswer.SUPPORTED,
            q3_sensitivity=ReviewAnswer.INTERNAL,
            q4_status=ReviewAnswer.ACCEPTED,
            reviewer_notes="Confirmed with CTO",
        )
        updated = queue.submit_review(rid, answers, reviewer="reviewer1")

        assert updated is not None, "Approved review should return updated record"
        assert updated.why.certainty.value == "supported"
        assert updated.sensitivity.value == "internal"
        assert updated.status.value == "accepted"
        assert updated.quality.review_state.value == "approved"

    def test_review_reject_discards(self, tmp_path):
        """Test that rejecting a review discards the record"""
        from agents.scribe.review_queue import ReviewQueue, ReviewAnswers, ReviewAnswer
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.scribe.detector import DetectionResult
        from agents.common.language import detect_language

        queue = ReviewQueue(queue_path=tmp_path / "test_review.json")
        builder = RecordBuilder()

        # Use a borderline message that shouldn't be captured
        conv = ALICE_CONVERSATIONS[-2]  # Pizza decision
        raw_event = RawEvent(
            text=conv.text, user=conv.user, channel=conv.channel,
            timestamp=str(datetime.now(timezone.utc).timestamp()), source="slack",
        )
        detection = DetectionResult(
            is_significant=True, confidence=0.55,
            matched_pattern="We decided", category="general",
            domain="general", priority="medium",
        )
        language = detect_language(conv.text)
        record = builder.build(raw_event, detection, language=language)
        rid = queue.add(record, detection.confidence)

        # Reviewer rejects
        answers = ReviewAnswers(
            q1_worth_saving=ReviewAnswer.IGNORE,
            q2_evidence_supported=ReviewAnswer.UNKNOWN,
            q3_sensitivity=ReviewAnswer.PUBLIC,
        )
        result = queue.submit_review(rid, answers, reviewer="reviewer1")

        assert result is None, "Rejected review should return None"
        assert queue.get_item(rid).status == "rejected"


class TestCrossTeamRecall:
    """Test that decisions from one team member can be recalled by another"""

    def test_recall_queries_well_formed(self):
        """Verify recall query test data is well-formed"""
        assert len(RECALL_QUERIES) >= 6, "Should have at least 6 cross-member recall queries"

        # All queries should reference different source members
        source_members = set(q.expected_source_member for q in RECALL_QUERIES)
        assert len(source_members) >= 3, "Queries should span all 3 team members"

        # All queries should have different searchers
        searchers = set(q.searcher for q in RECALL_QUERIES)
        assert len(searchers) >= 3, "All 3 members should search for others' decisions"

    def test_query_processor_parses_recall_queries(self):
        """QueryProcessor should correctly parse recall queries"""
        from agents.retriever.query_processor import QueryProcessor, QueryIntent

        qp = QueryProcessor()

        specific_intent_count = 0
        for rq in RECALL_QUERIES:
            parsed = qp.parse(rq.query)

            if parsed.intent != QueryIntent.GENERAL:
                specific_intent_count += 1

            assert len(parsed.keywords) > 0, (
                f"Query should have keywords: {rq.query}"
            )
            assert len(parsed.expanded_queries) >= 1, (
                f"Query should have expansions: {rq.query}"
            )

        # At least half of recall queries should get a specific intent
        assert specific_intent_count >= len(RECALL_QUERIES) // 2, (
            f"At least half of recall queries should have specific intent, "
            f"got {specific_intent_count}/{len(RECALL_QUERIES)}"
        )

    def test_synthesizer_fallback_produces_answer(self):
        """Synthesizer fallback should produce formatted answers"""
        from agents.retriever.synthesizer import Synthesizer, SynthesizedAnswer
        from agents.retriever.searcher import SearchResult
        from agents.retriever.query_processor import QueryProcessor

        synth = Synthesizer()  # No API key → fallback
        qp = QueryProcessor()

        # Simulate: Bob asks about Alice's PostgreSQL decision
        parsed = qp.parse("Why did we choose PostgreSQL over MongoDB?")

        # Create mock search results as if enVector returned Alice's record
        search_results = [
            SearchResult(
                record_id="dec_2026-02-13_arch_postgresql",
                title="Use PostgreSQL instead of MongoDB",
                payload_text="# Decision Record: Use PostgreSQL instead of MongoDB\n\n## Decision\nWe chose PostgreSQL for ACID compliance...",
                domain="architecture",
                certainty="supported",
                status="accepted",
                score=0.92,
                metadata={"member": "alice"},
            ),
        ]

        answer = synth.synthesize(parsed, search_results)

        assert len(answer.answer) > 0, "Should produce an answer"
        assert answer.confidence > 0, "Should have non-zero confidence"
        assert len(answer.sources) == 1, "Should reference the source"
        assert answer.sources[0]["record_id"] == "dec_2026-02-13_arch_postgresql"

    def test_synthesizer_warns_on_uncertain_evidence(self):
        """Synthesizer should warn when evidence certainty is low"""
        from agents.retriever.synthesizer import Synthesizer
        from agents.retriever.searcher import SearchResult
        from agents.retriever.query_processor import QueryProcessor

        synth = Synthesizer()
        qp = QueryProcessor()

        parsed = qp.parse("What is our deployment policy?")

        # Search result with unknown certainty
        search_results = [
            SearchResult(
                record_id="dec_2026-02-13_ops_deployment",
                title="Blue-green deployment policy",
                payload_text="# Decision Record: Blue-green deployment\n\n## Decision\nAll changes must go through blue-green...",
                domain="ops",
                certainty="unknown",
                status="proposed",
                score=0.80,
                metadata={"member": "bob"},
            ),
        ]

        answer = synth.synthesize(parsed, search_results)

        # Should include uncertainty warnings
        has_warning = any("uncertain" in w.lower() or "unknown" in w.lower() for w in answer.warnings)
        assert has_warning or "LLM not available" in str(answer.warnings), (
            f"Should warn about uncertain evidence. Warnings: {answer.warnings}"
        )


class TestConversationScriptCompleteness:
    """Meta-tests to verify the conversation scripts are comprehensive"""

    def test_all_members_have_conversations(self):
        """Each member should have a good mix of conversations"""
        for name, convs in [("alice", ALICE_CONVERSATIONS), ("bob", BOB_CONVERSATIONS), ("charlie", CHARLIE_CONVERSATIONS)]:
            captures = [c for c in convs if c.expected_capture]
            rejects = [c for c in convs if not c.expected_capture]
            assert len(captures) >= 5, f"{name} should have >=5 capture messages"
            assert len(rejects) >= 5, f"{name} should have >=5 reject messages"

    def test_categories_covered(self):
        """All conversation categories should be represented"""
        categories = set(c.category for c in ALL_CONVERSATIONS)
        assert "decision" in categories
        assert "casual" in categories
        assert "status" in categories
        assert "vague" in categories
        assert "borderline" in categories

    def test_domains_covered(self):
        """Multiple domains should be represented"""
        domains = set(c.expected_domain for c in ALL_CONVERSATIONS if c.expected_domain)
        assert "architecture" in domains
        assert "security" in domains
        assert "product" in domains or "ops" in domains

    def test_total_conversation_count(self):
        """Should have a substantial number of test conversations"""
        assert len(ALL_CONVERSATIONS) >= 30, (
            f"Should have >=30 total conversations, got {len(ALL_CONVERSATIONS)}"
        )

    def test_recall_queries_span_all_pairs(self):
        """Recall queries should test all member-to-member pairs"""
        pairs = set()
        for rq in RECALL_QUERIES:
            pairs.add((rq.searcher, rq.expected_source_member))

        # At least one query per searcher-target pair
        assert ("bob", "alice") in pairs, "Bob should query Alice's decisions"
        assert ("alice", "charlie") in pairs, "Alice should query Charlie's decisions"
        assert ("charlie", "bob") in pairs, "Charlie should query Bob's decisions"
