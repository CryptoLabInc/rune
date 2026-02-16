"""
Multi-Day Team Scenario Test

Simulates a realistic 3-day workflow for a 3-person team (Alice, Bob, Charlie)
where conversations happen sequentially across channels, decisions emerge from
discussion threads, and cross-member recall validates organizational memory.

Day 1: Architecture Sprint Planning
  - Alice proposes database migration
  - Bob raises performance concerns
  - Charlie flags security requirements
  - Team decides on PostgreSQL + encryption at rest
  - Casual chat and status updates mixed in

Day 2: Implementation Decisions
  - Alice finalizes API design (gRPC)
  - Bob handles feature flag rollout (LaunchDarkly)
  - Charlie establishes security review policy
  - Cross-references to Day 1 decisions

Day 3: Incident Response + Recall
  - Production incident triggers policy review
  - Team recalls previous decisions to inform response
  - Post-mortem generates new learnings
  - Retriever validates cross-member recall

Each day tests the full 3-tier pipeline:
  Tier 1: Embedding similarity → wide net
  Tier 2: Haiku policy filter → false positive removal
  Tier 3: Sonnet extraction → Decision Record building
"""

import json
import pytest
from datetime import datetime, timedelta
from unittest.mock import Mock
from typing import List, Dict, Any, Optional
from dataclasses import dataclass, field


# ============================================================================
# Day Scenario Data Structures
# ============================================================================

@dataclass
class ChannelMessage:
    """A message in a Slack channel with metadata"""
    timestamp: str  # Simulated Slack ts
    user: str
    channel: str
    text: str
    thread_ts: Optional[str] = None  # If thread reply
    expected_capture: bool = False
    expected_domain: Optional[str] = None
    note: str = ""


@dataclass
class DayScenario:
    """A day's worth of team conversations"""
    day: int
    title: str
    messages: List[ChannelMessage]
    expected_captures: int = 0  # How many should be captured
    expected_rejects: int = 0   # How many should be filtered out


# ============================================================================
# Tier 2 Simulation (same as test_pipeline_scenario.py)
# ============================================================================

def simulate_tier2(text: str) -> dict:
    """Simulate Tier 2 Haiku judgment"""
    text_lower = text.lower()

    casual_signals = [
        "good morning", "hope everyone", "heading to", "sounds good",
        "standup in", "hey everyone", "thanks for", "happy hour",
        "pizza", "team lunch", "conference room", "offsite",
        "coffee", "grab lunch", "see you", "have a great",
        "sounds like a plan", "no worries", "let me check",
        "sure thing", "will do", "on it",
        "sushi", "ramen", "thai food", "team dinner",
    ]
    for s in casual_signals:
        if s in text_lower:
            return {"capture": False, "reason": "Casual/social", "domain": "general"}

    status_signals = [
        "still working on", "looking into it", "updated the",
        "merged the pr", "tests are green", "running the",
        "was patched", "no action needed", "please update",
        "should have it by", "currently broken", "deploying now",
        "build passed", "build failed", "restarting the",
        "checking the logs", "monitoring the", "seems fine now",
        "root cause identified", "incident resolved", "error rate back to",
        "deploying the", "will monitor",
    ]
    for s in status_signals:
        if s in text_lower:
            return {"capture": False, "reason": "Status update", "domain": "general"}

    vague_signals = [
        "maybe we should", "might be", "sometime",
        "should look into", "just my personal", "kind of",
        "interesting approach", "i wonder if", "not sure yet",
        "could potentially", "thinking about",
    ]
    for s in vague_signals:
        if s in text_lower:
            return {"capture": False, "reason": "Vague opinion", "domain": "general"}

    question_only = "?" in text and not any(
        w in text_lower for w in ["decided", "agreed", "going with", "policy", "mandate", "chose"]
    )
    if question_only and len(text) < 150:
        return {"capture": False, "reason": "Question without decision", "domain": "general"}

    # Determine domain for captures
    domain = "general"
    if any(w in text_lower for w in ["security", "mtls", "encryption", "auth", "compliance", "vulnerability", "certificate", "audit"]):
        domain = "security"
    elif any(w in text_lower for w in ["architecture", "postgresql", "grpc", "kafka", "microservice", "typescript", "database", "schema"]):
        domain = "architecture"
    elif any(w in text_lower for w in ["deployment", "devops", "ci/cd", "blue-green", "rollback", "canary"]):
        domain = "ops"
    elif any(w in text_lower for w in ["sprint", "feature flag", "prioritiz", "customer", "product", "roadmap"]):
        domain = "product"
    elif any(w in text_lower for w in ["performance", "bottleneck", "latency", "benchmark", "cache", "n+1"]):
        domain = "architecture"
    elif any(w in text_lower for w in ["postmortem", "outage", "incident", "circuit breaker", "retry", "lesson"]):
        domain = "ops"

    return {"capture": True, "reason": "Organizational decision with rationale", "domain": domain}


# ============================================================================
# Day 1: Architecture Sprint Planning
# ============================================================================

DAY1 = DayScenario(
    day=1,
    title="Architecture Sprint Planning",
    messages=[
        # 09:00 — Morning casual
        ChannelMessage(
            timestamp="1707897600.000100",
            user="alice", channel="#general",
            text="Good morning team! Hope everyone had a great weekend. Ready for sprint planning?",
            expected_capture=False, note="Casual greeting",
        ),
        ChannelMessage(
            timestamp="1707897660.000200",
            user="bob", channel="#general",
            text="Hey everyone! Coffee is on. Let me check the sprint board real quick.",
            expected_capture=False, note="Casual + status",
        ),
        ChannelMessage(
            timestamp="1707897720.000300",
            user="charlie", channel="#general",
            text="Good morning! I have the security review results to share later.",
            expected_capture=False, note="Casual greeting with mild status",
        ),

        # 09:30 — Sprint planning begins
        ChannelMessage(
            timestamp="1707899400.000400",
            user="alice", channel="#architecture",
            text='I want to kick off the database migration discussion. Our current MySQL setup is hitting scaling limits — 50ms p99 on read queries is too high for the payment service. We need to evaluate PostgreSQL vs CockroachDB for the primary datastore.',
            expected_capture=True, expected_domain="architecture",
            note="Problem statement with concrete metrics",
        ),
        ChannelMessage(
            timestamp="1707899460.000500",
            user="bob", channel="#architecture",
            text="What are the actual numbers? Is 50ms really the bottleneck or is it the application layer?",
            thread_ts="1707899400.000400",
            expected_capture=False, note="Question without decision",
        ),
        ChannelMessage(
            timestamp="1707899520.000600",
            user="alice", channel="#architecture",
            text='I ran benchmarks last week. MySQL p99 was 48ms, PostgreSQL on the same dataset was 12ms, CockroachDB was 18ms. "The difference is PostgreSQL\'s query planner handles our join-heavy workload much better" according to the benchmark report.',
            thread_ts="1707899400.000400",
            expected_capture=True, expected_domain="architecture",
            note="Benchmark data with quoted evidence",
        ),
        ChannelMessage(
            timestamp="1707899580.000700",
            user="charlie", channel="#architecture",
            text='Before we decide, I need to flag that any database migration must maintain encryption at rest. "Our SOC2 auditor specifically requires AES-256 for all PII data stores" — this is non-negotiable.',
            thread_ts="1707899400.000400",
            expected_capture=True, expected_domain="security",
            note="Security requirement with quoted compliance mandate",
        ),
        ChannelMessage(
            timestamp="1707899640.000800",
            user="alice", channel="#architecture",
            text='Both PostgreSQL and CockroachDB support TDE (Transparent Data Encryption). PostgreSQL uses pgcrypto extension for AES-256. Given the benchmark results and compliance requirements, I propose we go with PostgreSQL. The trade-offs are: we lose CockroachDB\'s automatic sharding, but we gain better query performance and a more mature ecosystem.',
            thread_ts="1707899400.000400",
            expected_capture=True, expected_domain="architecture",
            note="Final decision with trade-off analysis",
        ),
        ChannelMessage(
            timestamp="1707899700.000900",
            user="bob", channel="#architecture",
            text="Sounds like a plan. I'll update the sprint board.",
            thread_ts="1707899400.000400",
            expected_capture=False, note="Casual agreement, no additional decision",
        ),

        # 10:30 — Bob's sprint priorities
        ChannelMessage(
            timestamp="1707903000.001000",
            user="bob", channel="#sprint-planning",
            text='Sprint priorities for Q1-W3: We are prioritizing the auth refactor over the mobile redesign. Three features are blocked on the OAuth PKCE migration. "The current implementation has a known token replay vulnerability" per the security audit. Allocating 2 engineers full-time for 2 weeks.',
            expected_capture=True, expected_domain="product",
            note="Sprint prioritization with security rationale and resource allocation",
        ),
        ChannelMessage(
            timestamp="1707903060.001100",
            user="bob", channel="#sprint-planning",
            text="Still working on the velocity calculations from last sprint. Should have the report by EOD.",
            expected_capture=False, note="Status update",
        ),

        # 11:00 — Charlie's security policies
        ChannelMessage(
            timestamp="1707904800.001200",
            user="charlie", channel="#security",
            text='New policy effective immediately: All inter-service communication must use mTLS with certificates issued by our internal CA. "The penetration test found 3 services communicating over plaintext HTTP internally" which is a critical finding. Deadline: all services must be migrated by March 15.',
            expected_capture=True, expected_domain="security",
            note="Security policy with evidence from pentest and deadline",
        ),
        ChannelMessage(
            timestamp="1707904860.001300",
            user="charlie", channel="#security",
            text="Running the weekly vulnerability scan now. Will share results when done.",
            expected_capture=False, note="Routine operational task",
        ),
        ChannelMessage(
            timestamp="1707904920.001400",
            user="alice", channel="#security",
            text="Charlie, will the mTLS requirement affect our gRPC setup? We're already using TLS for gRPC.",
            thread_ts="1707904800.001200",
            expected_capture=False, note="Question without decision",
        ),
        ChannelMessage(
            timestamp="1707904980.001500",
            user="charlie", channel="#security",
            text='gRPC already uses TLS, but we need mutual TLS — both client and server must present certificates. The policy is: "Every service must present a valid x509 certificate, and every service must verify the peer certificate against our CA." This applies to gRPC, HTTP, and any custom TCP services.',
            thread_ts="1707904800.001200",
            expected_capture=True, expected_domain="security",
            note="Clarification that adds concrete technical detail to the mTLS policy",
        ),

        # 12:00 — Lunch break casual
        ChannelMessage(
            timestamp="1707908400.001600",
            user="bob", channel="#random",
            text="Anyone want to grab lunch at the new ramen place?",
            expected_capture=False, note="Casual",
        ),
        ChannelMessage(
            timestamp="1707908460.001700",
            user="alice", channel="#random",
            text="Sure thing! I'll be ready in 5.",
            expected_capture=False, note="Casual",
        ),

        # 14:00 — Afternoon decisions
        ChannelMessage(
            timestamp="1707915600.001800",
            user="alice", channel="#engineering",
            text='We standardized on TypeScript strict mode for all frontend code, effective today. The policy: no new JavaScript files in the monorepo. Existing JS files will be migrated over the next 3 sprints. We chose TypeScript over Flow because "TypeScript has 10x the community ecosystem and better IDE support."',
            expected_capture=True, expected_domain="architecture",
            note="Technology standardization with timeline and quoted rationale",
        ),
        ChannelMessage(
            timestamp="1707915660.001900",
            user="bob", channel="#engineering",
            text="Merged the PR for the login page fix. Tests are green.",
            expected_capture=False, note="Routine PR notification",
        ),

        # 15:00 — Vague/borderline messages
        ChannelMessage(
            timestamp="1707919200.002000",
            user="bob", channel="#engineering",
            text="I read an article about how Spotify uses feature flags at scale. Interesting approach, maybe we should look into that someday.",
            expected_capture=False, note="Information sharing + vague suggestion",
        ),
        ChannelMessage(
            timestamp="1707919260.002100",
            user="alice", channel="#random",
            text="We decided to order sushi for the team dinner tonight. The Thai place was closed.",
            expected_capture=False, note="Borderline: contains 'We decided' but is food, not org decision",
        ),

        # 16:00 — Deployment policy
        ChannelMessage(
            timestamp="1707922800.002200",
            user="bob", channel="#devops",
            text='New deployment policy: All production deployments must use blue-green strategy. "Deploy to staging first, run automated smoke tests, then switch traffic" is the new standard. We chose blue-green over canary because our test coverage is high enough to catch issues pre-switch. Rollback time drops from ~30 minutes to under 10 seconds.',
            expected_capture=True, expected_domain="ops",
            note="Deployment policy with build-vs-buy rationale and quantified improvement",
        ),
    ],
    expected_captures=9,
    expected_rejects=13,
)


# ============================================================================
# Day 2: Implementation Decisions
# ============================================================================

DAY2 = DayScenario(
    day=2,
    title="Implementation Decisions",
    messages=[
        # 09:00 — Morning standup
        ChannelMessage(
            timestamp="1707984000.003000",
            user="bob", channel="#engineering",
            text="Standup in 5 minutes! Please have your updates ready.",
            expected_capture=False, note="Meeting reminder",
        ),
        ChannelMessage(
            timestamp="1707984060.003100",
            user="alice", channel="#engineering",
            text="Deploying the database migration script to staging now. Will monitor for the next hour.",
            expected_capture=False, note="Status update",
        ),

        # 09:30 — API design finalization
        ChannelMessage(
            timestamp="1707985800.003200",
            user="alice", channel="#architecture",
            text='Finalizing the API design: we will use gRPC with Protocol Buffers for all internal service-to-service communication. REST will be reserved only for the public API gateway. The key reasons: our benchmarks show gRPC is 3x faster than REST for our payload sizes, and protobuf gives us backward-compatible schema evolution. The trade-off is that browser clients need a gRPC-Web proxy, which adds one hop.',
            expected_capture=True, expected_domain="architecture",
            note="API design decision with benchmark rationale and trade-off",
        ),
        ChannelMessage(
            timestamp="1707985860.003300",
            user="charlie", channel="#architecture",
            text='I want to add a requirement to the gRPC decision: all gRPC services must implement health checking per the gRPC Health Checking Protocol. This is needed for our Kubernetes readiness probes and for the mTLS certificate rotation without downtime.',
            thread_ts="1707985800.003200",
            expected_capture=True, expected_domain="architecture",
            note="Additional requirement tied to Day 1 mTLS decision",
        ),

        # 10:00 — Feature flags
        ChannelMessage(
            timestamp="1707987600.003400",
            user="bob", channel="#engineering",
            text='We decided to adopt LaunchDarkly for feature flags instead of building in-house. Build-vs-buy analysis showed: building our own would take ~6 months of engineering time (2 engineers) plus ongoing maintenance. LaunchDarkly gives us targeting rules, gradual rollouts, and an audit trail out of the box. "The ROI is clear — we save $300K in engineering cost over 2 years" per our analysis.',
            expected_capture=True, expected_domain="product",
            note="Build-vs-buy with quantified ROI",
        ),
        ChannelMessage(
            timestamp="1707987660.003500",
            user="alice", channel="#engineering",
            text="Not sure yet about the pricing tier. Let me check with finance and get back.",
            thread_ts="1707987600.003400",
            expected_capture=False, note="Vague / pending decision",
        ),

        # 11:00 — Security review process
        ChannelMessage(
            timestamp="1707991200.003600",
            user="charlie", channel="#security",
            text='After the vulnerability disclosure on the payment API last week, we are implementing a mandatory security review process. Policy: every PR that touches authentication, payment processing, or PII handling code must be reviewed by a security-certified engineer before merge. No exceptions. We chose to enforce this via GitHub CODEOWNERS rather than honor system because "automated enforcement is the only reliable enforcement" — lesson from the incident.',
            expected_capture=True, expected_domain="security",
            note="Security review policy triggered by incident, with enforcement mechanism and lesson learned",
        ),
        ChannelMessage(
            timestamp="1707991260.003700",
            user="charlie", channel="#security",
            text="CVE-2024-1234 was patched in Node.js 20.11.1. We're already on 20.12.0 so no action needed.",
            expected_capture=False, note="Informational CVE update, no decision",
        ),

        # 12:00 — Casual
        ChannelMessage(
            timestamp="1707994800.003800",
            user="alice", channel="#random",
            text="The sushi from last night was amazing! We should go there again.",
            expected_capture=False, note="Casual",
        ),

        # 13:00 — Integration patterns
        ChannelMessage(
            timestamp="1707998400.003900",
            user="alice", channel="#architecture",
            text='For the event-driven architecture: Kafka is our standard message bus for all async communication between services. We chose Kafka over RabbitMQ because "Kafka\'s log-based architecture lets us replay events for debugging and rebuilding state after failures." The team also agreed that all events must follow a common envelope schema with correlation IDs for distributed tracing.',
            expected_capture=True, expected_domain="architecture",
            note="Integration pattern decision with technical rationale and team agreement",
        ),

        # 14:00 — Performance standards
        ChannelMessage(
            timestamp="1708002000.004000",
            user="bob", channel="#performance",
            text='New performance standards: All API endpoints must respond within 200ms at p95. We identified the user dashboard as the worst offender (3.2s) due to N+1 query patterns. The fix: mandatory use of DataLoader for all list/collection endpoints. "We measured a 16x improvement from 3.2s to 195ms after batching" in the dashboard endpoint. This pattern is now required for all new endpoints.',
            expected_capture=True, expected_domain="architecture",
            note="Performance standard with measurement data and mandatory pattern",
        ),

        # 15:00 — Encryption details
        ChannelMessage(
            timestamp="1708005600.004100",
            user="charlie", channel="#security",
            text='Encryption implementation details finalized: AES-256-GCM for data at rest (PostgreSQL TDE via pgcrypto), TLS 1.3 for data in transit. Key management: encryption keys stored in HashiCorp Vault with automatic rotation every 90 days. We chose AES-256-GCM over AES-256-CBC because GCM provides authenticated encryption — "it prevents both tampering and confidentiality breaches in a single primitive."',
            expected_capture=True, expected_domain="security",
            note="Encryption standard with technical rationale for algorithm choice",
        ),

        # 16:00 — Status and borderline
        ChannelMessage(
            timestamp="1708009200.004200",
            user="bob", channel="#engineering",
            text="Reminder: please update your Jira tickets before end of sprint tomorrow.",
            expected_capture=False, note="Administrative reminder",
        ),
        ChannelMessage(
            timestamp="1708009260.004300",
            user="alice", channel="#architecture",
            text="I wonder if we should consider GraphQL for the public API someday. Could be interesting.",
            expected_capture=False, note="Vague musing without commitment",
        ),

        # 17:00 — Rate limiting decision
        ChannelMessage(
            timestamp="1708012800.004400",
            user="charlie", channel="#security",
            text='Rate limiting strategy decided: we will implement rate limiting at the API gateway using Kong. We chose Kong over building custom middleware because Kong provides built-in rate limiting with Redis backend, IP allowlisting, and integrates with our existing Prometheus monitoring. Configuration: 100 requests/minute for unauthenticated, 1000/minute for authenticated, 10000/minute for internal service-to-service.',
            expected_capture=True, expected_domain="security",
            note="Rate limiting decision with tool choice rationale and specific configuration",
        ),
    ],
    expected_captures=8,
    expected_rejects=7,
)


# ============================================================================
# Day 3: Incident Response + Recall
# ============================================================================

DAY3 = DayScenario(
    day=3,
    title="Incident Response + Recall",
    messages=[
        # 02:00 — Incident alert
        ChannelMessage(
            timestamp="1708070400.005000",
            user="bob", channel="#incidents",
            text="ALERT: Payment service is returning 500 errors. Error rate at 15%. Checking the logs now.",
            expected_capture=False, note="Alert notification + status",
        ),
        ChannelMessage(
            timestamp="1708070460.005100",
            user="bob", channel="#incidents",
            text="Root cause identified: the retry logic in the payment service is causing cascading failures. Each failed request spawns 3 retries, which overwhelm the database connection pool.",
            expected_capture=False, note="Investigation finding — status, not decision yet",
        ),

        # 02:30 — Incident decision
        ChannelMessage(
            timestamp="1708072200.005200",
            user="alice", channel="#incidents",
            text='Immediate fix: we are disabling retries on the payment service and implementing a circuit breaker pattern. "When error rate exceeds 50%, stop sending requests and return a cached response." We are using Resilience4j for the circuit breaker implementation. This is a permanent architectural decision, not just an incident patch.',
            expected_capture=True, expected_domain="architecture",
            note="Incident response decision with permanent architectural impact",
        ),
        ChannelMessage(
            timestamp="1708072260.005300",
            user="charlie", channel="#incidents",
            text='Adding a security note to the incident: the retry storm exposed that our rate limiter was not applied to internal service-to-service calls. Updating the Kong configuration to enforce the 10000/min limit on internal traffic too. "This is consistent with our defense-in-depth policy."',
            thread_ts="1708072200.005200",
            expected_capture=True, expected_domain="security",
            note="Security policy update triggered by incident",
        ),

        # 03:00 — Incident resolved
        ChannelMessage(
            timestamp="1708074000.005400",
            user="bob", channel="#incidents",
            text="Incident resolved. Payment service error rate back to 0%. Deploying the circuit breaker to all services now.",
            expected_capture=False, note="Status update — incident resolution",
        ),

        # 09:00 — Morning after
        ChannelMessage(
            timestamp="1708095600.005500",
            user="alice", channel="#general",
            text="Good morning. Rough night with the incident. Let's make sure we do a proper postmortem.",
            expected_capture=False, note="Casual reference to incident",
        ),

        # 10:00 — Post-mortem
        ChannelMessage(
            timestamp="1708099200.005600",
            user="alice", channel="#incidents",
            text='Post-mortem conclusion: The retry storm incident taught us three key lessons. First, never implement retries without circuit breakers — this is now a mandatory architectural pattern. Second, all retry configurations must use exponential backoff with jitter, not fixed intervals. Third, our load testing must include failure scenarios, not just happy path. "We will add chaos engineering tests to the CI pipeline" was the team decision.',
            expected_capture=True, expected_domain="ops",
            note="Post-mortem with 3 lessons learned and concrete action items",
        ),
        ChannelMessage(
            timestamp="1708099260.005700",
            user="bob", channel="#incidents",
            text='From the product side: we are adding a circuit breaker health dashboard to the monitoring system. The existing Grafana dashboards will be extended with Resilience4j metrics. "Every team must be able to see their service\'s circuit breaker state in real-time." This ties into our Q1 observability goals.',
            thread_ts="1708099200.005600",
            expected_capture=True, expected_domain="ops",
            note="Observability decision tied to incident response",
        ),
        ChannelMessage(
            timestamp="1708099320.005800",
            user="charlie", channel="#incidents",
            text='Security post-mortem addition: the incident revealed that we had no alerting on anomalous internal traffic patterns. New policy: implement anomaly detection on internal service call volumes. When any service exceeds 3x its baseline call rate, trigger an automatic alert. "Anomalous traffic patterns are often the first sign of both failures and attacks."',
            thread_ts="1708099200.005600",
            expected_capture=True, expected_domain="security",
            note="New monitoring policy from security perspective",
        ),

        # 11:00 — Process improvement
        ChannelMessage(
            timestamp="1708102800.005900",
            user="bob", channel="#engineering",
            text='Based on the incident, we are updating our deployment checklist. New requirement: every service deployment must include a rollback test. "We found that 40% of our services had untested rollback procedures" during the incident audit. The blue-green deployment policy from Day 1 helped us recover the payment service in under 10 seconds, validating that decision.',
            expected_capture=True, expected_domain="ops",
            note="Process improvement with quantified finding and reference to earlier decision",
        ),

        # 12:00 — Casual
        ChannelMessage(
            timestamp="1708106400.006000",
            user="charlie", channel="#random",
            text="After last night, I think we all deserve a long lunch. Anyone want Thai food?",
            expected_capture=False, note="Casual",
        ),
        ChannelMessage(
            timestamp="1708106460.006100",
            user="bob", channel="#random",
            text="Will do! Let me grab my jacket.",
            expected_capture=False, note="Casual",
        ),

        # 14:00 — Dependency policy (emerged from incident)
        ChannelMessage(
            timestamp="1708113600.006200",
            user="alice", channel="#architecture",
            text='New architectural policy from the incident learnings: all service-to-service dependencies must declare timeout, retry, and circuit breaker configurations in a standardized config file (circuit-breaker.yaml). "No service may call another service without explicit failure handling configuration." We chose YAML over environment variables because it is version-controlled and reviewable in PRs.',
            expected_capture=True, expected_domain="architecture",
            note="New architectural standard emerged from incident",
        ),

        # 15:00 — Status updates
        ChannelMessage(
            timestamp="1708117200.006300",
            user="bob", channel="#engineering",
            text="Build passed on the circuit breaker integration. Deploying to staging for smoke tests.",
            expected_capture=False, note="Status update",
        ),
        ChannelMessage(
            timestamp="1708117260.006400",
            user="alice", channel="#engineering",
            text="Monitoring the staging deployment. Seems fine now.",
            expected_capture=False, note="Status update",
        ),

        # 16:00 — Borderline
        ChannelMessage(
            timestamp="1708120800.006500",
            user="bob", channel="#engineering",
            text="Maybe we should think about adopting Rust for the performance-critical services. Not sure yet though.",
            expected_capture=False, note="Vague suggestion without commitment",
        ),
    ],
    expected_captures=7,
    expected_rejects=9,
)


ALL_DAYS = [DAY1, DAY2, DAY3]


# ============================================================================
# Tests
# ============================================================================

class TestDayScenarioCompleteness:
    """Verify test data is comprehensive and balanced"""

    def test_day1_message_count(self):
        assert len(DAY1.messages) == 22
        captures = [m for m in DAY1.messages if m.expected_capture]
        rejects = [m for m in DAY1.messages if not m.expected_capture]
        assert len(captures) == DAY1.expected_captures
        assert len(rejects) == DAY1.expected_rejects

    def test_day2_message_count(self):
        assert len(DAY2.messages) == 15
        captures = [m for m in DAY2.messages if m.expected_capture]
        rejects = [m for m in DAY2.messages if not m.expected_capture]
        assert len(captures) == DAY2.expected_captures
        assert len(rejects) == DAY2.expected_rejects

    def test_day3_message_count(self):
        assert len(DAY3.messages) == 16
        captures = [m for m in DAY3.messages if m.expected_capture]
        rejects = [m for m in DAY3.messages if not m.expected_capture]
        assert len(captures) == DAY3.expected_captures
        assert len(rejects) == DAY3.expected_rejects

    def test_total_messages(self):
        total = sum(len(d.messages) for d in ALL_DAYS)
        assert total >= 50, f"Should have >=50 total messages, got {total}"

    def test_all_members_active_each_day(self):
        for day in ALL_DAYS:
            users = set(m.user for m in day.messages)
            assert "alice" in users, f"Alice missing from Day {day.day}"
            assert "bob" in users, f"Bob missing from Day {day.day}"
            assert "charlie" in users, f"Charlie missing from Day {day.day}"

    def test_thread_discussions_present(self):
        """At least some messages should be thread replies"""
        thread_replies = [m for d in ALL_DAYS for m in d.messages if m.thread_ts]
        assert len(thread_replies) >= 5, f"Should have >=5 thread replies, got {len(thread_replies)}"

    def test_channels_diverse(self):
        channels = set(m.channel for d in ALL_DAYS for m in d.messages)
        assert len(channels) >= 5, f"Should use >=5 channels, got {channels}"


class TestDayScenarioPipeline:
    """Run each day's messages through the full 3-tier pipeline"""

    @pytest.fixture
    def pipeline(self):
        from agents.scribe.tier2_filter import Tier2Filter
        from agents.scribe.record_builder import RecordBuilder, RawEvent
        from agents.scribe.detector import DecisionDetector
        from agents.common.pattern_cache import PatternCache, PatternEntry

        # Tier 1 mock
        cache = Mock(spec=PatternCache)
        cache.pattern_count = 10

        def mock_match(text, threshold=0.0):
            if len(text.strip()) < 20:
                return (None, 0.1)
            entry = PatternEntry(
                text="decision pattern", category="architecture",
                priority="high", embedding=[0.1] * 384, domain="architecture",
            )
            return (entry, 0.75)

        cache.find_best_match.side_effect = mock_match
        detector = DecisionDetector(cache, threshold=0.5, high_confidence_threshold=0.8)

        # Tier 2 mock
        tier2 = Tier2Filter.__new__(Tier2Filter)
        tier2._api_key = "test"
        tier2._model = "test"
        tier2._client = Mock()

        def tier2_call(**kwargs):
            msgs = kwargs.get("messages", [])
            if msgs:
                text = msgs[0]["content"].replace("Message: ", "").split("\n(Tier 1")[0]
                j = simulate_tier2(text)
                resp = Mock()
                blk = Mock()
                blk.text = json.dumps(j)
                resp.content = [blk]
                return resp
            resp = Mock()
            blk = Mock()
            blk.text = json.dumps({"capture": True, "reason": "default", "domain": "general"})
            resp.content = [blk]
            return resp

        tier2._client.messages.create.side_effect = tier2_call

        # Tier 3
        builder = RecordBuilder()

        return {"detector": detector, "tier2": tier2, "builder": builder}

    def _process(self, pipeline, msg: ChannelMessage):
        from agents.scribe.record_builder import RawEvent
        from agents.common.language import detect_language

        det = pipeline["detector"]
        t2 = pipeline["tier2"]
        bld = pipeline["builder"]

        result = det.detect(msg.text)
        if not result.is_significant:
            return None

        fr = t2.evaluate(msg.text, tier1_score=result.confidence, tier1_pattern=result.matched_pattern or "")
        if not fr.should_capture:
            return None

        if fr.domain != "general" and result.domain in (None, "general"):
            result.domain = fr.domain

        raw = RawEvent(
            text=msg.text, user=msg.user, channel=msg.channel,
            timestamp=msg.timestamp, source="slack", thread_ts=msg.thread_ts,
        )
        lang = detect_language(msg.text)
        return bld.build(raw, result, language=lang)

    def test_day1_pipeline(self, pipeline):
        """Day 1 messages processed correctly"""
        for msg in DAY1.messages:
            record = self._process(pipeline, msg)
            if msg.expected_capture:
                assert record is not None, f"Day 1 SHOULD CAPTURE: {msg.text[:60]}... ({msg.note})"
                assert len(record.payload.text) > 50
            else:
                assert record is None, f"Day 1 SHOULD REJECT: {msg.text[:60]}... ({msg.note})"

    def test_day2_pipeline(self, pipeline):
        """Day 2 messages processed correctly"""
        for msg in DAY2.messages:
            record = self._process(pipeline, msg)
            if msg.expected_capture:
                assert record is not None, f"Day 2 SHOULD CAPTURE: {msg.text[:60]}... ({msg.note})"
                assert len(record.payload.text) > 50
            else:
                assert record is None, f"Day 2 SHOULD REJECT: {msg.text[:60]}... ({msg.note})"

    def test_day3_pipeline(self, pipeline):
        """Day 3 messages processed correctly"""
        for msg in DAY3.messages:
            record = self._process(pipeline, msg)
            if msg.expected_capture:
                assert record is not None, f"Day 3 SHOULD CAPTURE: {msg.text[:60]}... ({msg.note})"
                assert len(record.payload.text) > 50
            else:
                assert record is None, f"Day 3 SHOULD REJECT: {msg.text[:60]}... ({msg.note})"

    def test_overall_capture_statistics(self, pipeline):
        """Verify overall stats across all 3 days"""
        total = 0
        captured = 0
        rejected = 0

        for day in ALL_DAYS:
            for msg in day.messages:
                total += 1
                record = self._process(pipeline, msg)
                if record:
                    captured += 1
                else:
                    rejected += 1

        expected_total_captures = sum(d.expected_captures for d in ALL_DAYS)
        assert captured == expected_total_captures, (
            f"Expected {expected_total_captures} total captures, got {captured}"
        )

        # Capture rate should be 30-55%
        rate = captured / total
        assert 0.30 <= rate <= 0.55, f"Capture rate {rate:.1%} out of expected range"

    def test_captured_records_have_valid_structure(self, pipeline):
        """All captured records should have proper Decision Record structure"""
        for day in ALL_DAYS:
            for msg in day.messages:
                if not msg.expected_capture:
                    continue
                record = self._process(pipeline, msg)
                assert record is not None

                # Structural validation
                assert record.id, f"Missing ID: {msg.text[:40]}"
                assert record.title, f"Missing title: {msg.text[:40]}"
                assert record.payload.text, f"Missing payload.text: {msg.text[:40]}"
                assert record.payload.format == "markdown"
                assert record.evidence, f"Missing evidence: {msg.text[:40]}"
                assert record.why.certainty, f"Missing certainty: {msg.text[:40]}"
                assert record.domain, f"Missing domain: {msg.text[:40]}"

                # payload.text should be markdown, not JSON
                assert not record.payload.text.strip().startswith("{")
                assert "# " in record.payload.text or "## " in record.payload.text


class TestCrossTeamRecallDay:
    """Test that Day 1-2 decisions can be recalled on Day 3"""

    def test_postmortem_references_earlier_decisions(self):
        """Post-mortem messages should reference earlier architecture/deployment decisions"""
        # Day 3 post-mortem mentions circuit breakers (new) and
        # references blue-green deployment policy (Day 1)
        postmortem = DAY3.messages[9]  # Bob's deployment checklist update
        assert "blue-green" in postmortem.text.lower() or "rollback" in postmortem.text.lower()
        assert postmortem.expected_capture is True

    def test_incident_triggers_policy_updates(self):
        """Day 3 incident should generate new policies (not just status)"""
        day3_captures = [m for m in DAY3.messages if m.expected_capture]
        # Should have multiple capture-worthy messages from the incident
        assert len(day3_captures) >= 5, (
            f"Incident response should generate >=5 captured decisions/learnings, got {len(day3_captures)}"
        )

        # Different members should contribute to incident response
        incident_users = set(m.user for m in day3_captures)
        assert len(incident_users) >= 2, "Multiple team members should contribute to incident response"

    def test_day3_recalls_day1_encryption(self):
        """Day 3 security decisions should be consistent with Day 1 encryption requirements"""
        day1_security = [m for m in DAY1.messages if m.expected_capture and m.expected_domain == "security"]
        day3_security = [m for m in DAY3.messages if m.expected_capture and m.expected_domain == "security"]

        assert len(day1_security) >= 2, "Day 1 should establish security baseline"
        assert len(day3_security) >= 1, "Day 3 should have security updates"

    def test_query_processor_handles_incident_queries(self):
        """Common incident-related recall queries should parse correctly"""
        from agents.retriever.query_processor import QueryProcessor, QueryIntent

        qp = QueryProcessor()

        incident_queries = [
            ("Why did we choose the circuit breaker pattern?", QueryIntent.DECISION_RATIONALE),
            ("What's our process for deploying to production?", QueryIntent.PATTERN_LOOKUP),
            ("What are the security requirements for internal traffic?", QueryIntent.SECURITY_COMPLIANCE),
        ]

        for query_text, expected_intent in incident_queries:
            parsed = qp.parse(query_text)
            assert parsed.intent == expected_intent, (
                f"Query '{query_text}' should have intent {expected_intent.value}, got {parsed.intent.value}"
            )

    def test_synthesizer_handles_multi_source_recall(self):
        """Synthesizer should handle results from multiple team members"""
        from agents.retriever.synthesizer import Synthesizer
        from agents.retriever.searcher import SearchResult
        from agents.retriever.query_processor import QueryProcessor

        synth = Synthesizer()
        qp = QueryProcessor()

        parsed = qp.parse("What's our approach for handling service failures?")

        # Results from different team members across different days
        results = [
            SearchResult(
                record_id="dec_day1_arch_circuit_breaker",
                title="Circuit breaker pattern for service failures",
                payload_text="# Decision Record: Circuit Breaker Pattern\n\n## Decision\nImplement circuit breakers before retries...",
                domain="architecture", certainty="supported", status="accepted",
                score=0.91, metadata={"member": "alice"},
            ),
            SearchResult(
                record_id="dec_day3_ops_postmortem",
                title="Payment service incident postmortem",
                payload_text="# Decision Record: Incident Postmortem Learnings\n\n## Decision\nMandatory circuit breakers, exponential backoff...",
                domain="ops", certainty="supported", status="accepted",
                score=0.87, metadata={"member": "alice"},
            ),
            SearchResult(
                record_id="dec_day3_sec_internal_traffic",
                title="Internal traffic anomaly detection",
                payload_text="# Decision Record: Internal Traffic Monitoring\n\n## Decision\nImplement anomaly detection on internal call volumes...",
                domain="security", certainty="partially_supported", status="proposed",
                score=0.72, metadata={"member": "charlie"},
            ),
        ]

        answer = synth.synthesize(parsed, results)

        assert len(answer.answer) > 0
        assert len(answer.sources) == 3
        assert answer.confidence > 0

        # Should reference multiple domains
        domains = set(s["domain"] for s in answer.sources)
        assert len(domains) >= 2, f"Should span multiple domains, got {domains}"
