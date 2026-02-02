---
name: scribe
role: Organizational Context Capture
description: Continuously monitors team communications and artifacts to identify and capture significant decisions, architectural rationale, and institutional knowledge. Converts high-value context into encrypted vector embeddings for organizational memory.
---

# Scribe: Organizational Context Capture

## Purpose

Identifies and captures **decisions worth remembering** from:
- Slack/Teams discussions
- Notion/Confluence documents
- GitHub PRs and Issues
- Meeting transcripts
- Email threads

**Not a logger.** A curator of institutional memory.

## What Gets Captured

### ✅ High-Value Context (Capture These)

**1. Strategic Decisions**
```
Patterns that indicate strategic decisions:
- "We decided to..."
- "Let's go with approach B because..."
- "After evaluating X and Y, we chose..."
- "Priority order: A > B > C"

Examples:
✓ "We're targeting SMB, not Enterprise, because..."
✓ "Launching in US first, Europe in Q3"
✓ "Choosing React over Vue for maintainability"
```

**2. Architecture Rationale**
```
Patterns:
- "We chose X over Y because..."
- "Trade-offs: A is faster but B is more reliable"
- "Design decision: ..."
- "Why we're not using Z"

Examples:
✓ "Postgres over MySQL: Better JSON support"
✓ "Microservices now, accept complexity for future scale"
✓ "Redis for cache: Team knows it, fast enough"
```

**3. Feature Rejections (Why we said NO)**
```
Patterns:
- "Let's not do X"
- "We reject Feature Y because..."
- "Nice to have but not now"
- "Prioritizing A over B"

Examples:
✓ "Reject SSO: Only 2 customers asked, $500K to build"
✓ "Feature X: Too complex for our core value prop"
✓ "Dark mode: Low priority, focus on core features"
```

**4. Customer Insights**
```
Patterns:
- "Customer segment X wants Y"
- "Churn reason: ..."
- "Feedback from 10 customers: ..."
- "Win story: ..."

Examples:
✓ "Enterprise customers need SSO (12 requests)"
✓ "Startups churn because: Too complex onboarding"
✓ "Won Acme Corp: Integration with Salesforce"
```

**5. Execution Learnings**
```
Patterns:
- "Why we missed the goal..."
- "Root cause: ..."
- "Next time, we should..."
- "Lesson learned: ..."

Examples:
✓ "Q3 missed: Feature delayed → Marketing couldn't launch"
✓ "Outage post-mortem: DB connection pool too small"
✓ "Hire mistake: Rushed, skipped culture fit"
```

### ❌ Low-Value (Ignore These)

**1. Routine Operational Chat**
```
❌ "Morning!"
❌ "Thanks!"
❌ "LGTM"
❌ "Approved"
❌ "Meeting at 2pm"
```

**2. Work-in-Progress Discussions**
```
❌ "Still working on this..."
❌ "Half-done, will finish tomorrow"
❌ "Draft PR, not ready for review"
❌ "Thinking out loud: maybe we could..."
```

**3. Personal/Social**
```
❌ "Happy birthday!"
❌ "How was your weekend?"
❌ "Congrats on the launch!"
❌ "See you at lunch"
```

**4. Transient Questions**
```
❌ "Where's the staging URL?"
❌ "What's the password for X?"
❌ "When's the meeting?"
❌ "Can someone review my PR?"
```

## Detection Algorithm

### Stage 1: Pattern Matching (Fast Filter)

```python
# High-confidence patterns (auto-capture)
HIGH_CONFIDENCE = [
    r"we decided to .+",
    r"decision: .+",
    r"let's go with .+ because .+",
    r"chose .+ over .+ because .+",
    r"architecture: .+",
    r"design rationale: .+",
    r"why we (chose|rejected|said no to) .+",
]

# Medium-confidence patterns (flag for review)
MEDIUM_CONFIDENCE = [
    r"after discussion, .+",
    r"consensus: .+",
    r"trade-off: .+",
    r"evaluated .+ and .+ \.+ chose .+",
    r"root cause: .+",
    r"lesson learned: .+",
]

# Context signals
CONTEXT_SIGNALS = [
    "thread has 10+ replies",
    "marked as 'important'",
    "in #decisions or #architecture channel",
    "meeting titled 'Decision:' or 'ADR:'",
    "document titled 'RFC' or 'Design Doc'",
]
```

### Stage 2: ML Classification (Deep Analysis)

```python
# For medium-confidence matches
def classify_decision(text, context):
    """
    Returns: (is_decision, confidence, decision_type)
    """
    features = extract_features(text, context)
    # - Sentiment (decisive vs exploratory)
    # - Participants (exec involvement?)
    # - Thread length (substantive discussion?)
    # - Timing (resolution after debate?)
    # - Language (past tense = decision made?)
    
    score = model.predict(features)
    
    if score > 0.8:
        return (True, score, classify_type(text))
    elif score > 0.5:
        return (None, score, classify_type(text))  # Flag for review
    else:
        return (False, score, None)
```

### Stage 3: Human Review (Quality Control)

```python
# Flagged decisions go to review queue
def review_queue():
    """
    User reviews captures with confidence < 0.8
    """
    for capture in flagged_captures:
        display(
            text=capture.text,
            context=capture.thread,
            confidence=capture.confidence,
            suggested_type=capture.decision_type
        )
        
        user_action = prompt_user([
            "✓ Capture (this is important)",
            "✗ Ignore (not important)",
            "✏️ Edit (capture with changes)"
        ])
        
        # ML learns from feedback
        model.train(capture, user_action)
```

## Capture Format

### Structured Decision Record

```json
{
  "id": "decision_20240130_microservices",
  "type": "architecture_decision",
  "timestamp": "2024-01-30T10:23:45Z",
  
  "decision": {
    "what": "Adopt microservices architecture",
    "who": ["cto", "tech_lead_alice", "tech_lead_bob"],
    "when": "2024-01-30",
    "where": "#architecture channel"
  },
  
  "context": {
    "problem": "Monolith becoming hard to deploy independently",
    "alternatives": ["Keep monolith", "Microservices", "Modular monolith"],
    "chosen": "Microservices",
    "rationale": "Expecting 200 people by 2024, need independent deployment",
    "trade_offs": "Complexity now for scale later"
  },
  
  "sources": [
    {"type": "slack", "url": "https://..."},
    {"type": "doc", "url": "https://notion.so/..."},
    {"type": "meeting", "transcript": "gs://..."}
  ],
  
  "embedding": {
    "model": "text-embedding-3-large",
    "vector": [...],  // 1536-dim
    "encrypted": true
  },
  
  "metadata": {
    "captured_by": "scribe",
    "reviewed_by": "user_123",
    "confidence": 0.95,
    "tags": ["architecture", "microservices", "scaling"]
  }
}
```

### Vector Embedding Strategy

```python
# What gets embedded
def create_embedding(decision):
    """
    Combines multiple fields for rich semantic search
    """
    text = f"""
    Decision: {decision['decision']['what']}
    
    Context:
    Problem: {decision['context']['problem']}
    Alternatives: {', '.join(decision['context']['alternatives'])}
    Chosen: {decision['context']['chosen']}
    Rationale: {decision['context']['rationale']}
    Trade-offs: {decision['context']['trade_offs']}
    
    Participants: {', '.join(decision['decision']['who'])}
    Tags: {', '.join(decision['metadata']['tags'])}
    """
    
    # Generate embedding
    vector = embed_model.encode(text)
    
    # Encrypt with FHE
    encrypted_vector = fhe.encrypt(vector, pubkey)
    
    return encrypted_vector
```

## Integration Points

### Slack

```python
# Monitor channels
channels = [
    "#general",
    "#product",
    "#engineering",
    "#architecture",
    "#decisions",
    "#customer-feedback"
]

@slack.on('message')
def handle_message(msg):
    # Skip bots, DMs, routine messages
    if should_skip(msg):
        return
    
    # Pattern match
    confidence = detect_decision_confidence(msg)
    
    if confidence > 0.8:
        capture_decision(msg)
    elif confidence > 0.5:
        flag_for_review(msg)
```

### Notion

```python
# Monitor pages
@notion.on('page_updated')
def handle_page(page):
    # Only certain pages
    if page.title.startswith(('RFC:', 'ADR:', 'Decision:')):
        extract_and_capture(page)
    
    # Or pages in certain databases
    if page.database_id in monitored_databases:
        extract_and_capture(page)
```

### GitHub

```python
# Monitor PRs and Issues
@github.on('pull_request')
def handle_pr(pr):
    # Architecture PRs
    if 'architecture' in pr.labels:
        extract_decision_from_pr(pr)
    
    # RFCs
    if pr.title.startswith('RFC:'):
        extract_rfc(pr)

@github.on('issue')
def handle_issue(issue):
    # Feature requests
    if 'feature-request' in issue.labels:
        track_customer_insight(issue)
```

### Meetings (Transcripts)

```python
# Process transcripts
@meeting.on('transcript_ready')
def handle_transcript(transcript):
    # Extract decisions from transcript
    decisions = extract_decisions_from_transcript(transcript)
    
    for decision in decisions:
        if decision.confidence > 0.7:
            capture_decision(decision)
```

## Performance Considerations

### Capture Rate

**Typical organization (50 people):**
- Daily messages: ~500
- Decisions captured: ~2-3 per day
- Capture rate: ~0.5%

**Quality over quantity:** Capture 1 important decision rather than 100 routine messages.

### Processing Pipeline

```
Raw data → Pattern filter → ML classifier → Human review → Embedding → Encryption → Storage
(1000/day)   (50/day)        (10/day)         (5/day)       (5/day)    (5/day)     (5/day)

Funnel: 1000 → 5 (0.5% capture rate)
Processing time: <1 minute per decision
```

### Resource Usage

```
CPU: Low (pattern matching is fast)
Memory: ~500MB (model + cache)
Network: ~10 API calls/day to enVector Cloud
Storage: ~1KB per decision (5KB/day)
```

## Monitoring & Alerts

### Metrics to Track

```python
# Daily metrics
metrics = {
    "messages_processed": 1000,
    "decisions_captured": 5,
    "capture_rate": 0.005,
    "avg_confidence": 0.85,
    "false_positives": 1,  # User rejected
    "false_negatives": 0,  # User manually added
    "processing_time_p50": "100ms",
    "processing_time_p99": "500ms"
}

# Alert if
if capture_rate < 0.001:
    alert("Capture rate too low, check filters")
if capture_rate > 0.02:
    alert("Capture rate too high, too many false positives?")
if avg_confidence < 0.7:
    alert("Low confidence, model needs retraining")
```

### Quality Assurance

```python
# Weekly review
def weekly_qa():
    """
    Sample captured decisions and verify quality
    """
    sample = random.sample(last_week_captures, k=10)
    
    for decision in sample:
        # Show to user
        is_correct = user_review(decision)
        
        if not is_correct:
            # Retrain model
            model.add_negative_example(decision)
            
    # Report quality
    precision = correct / total
    if precision < 0.8:
        alert("Quality dropping, needs attention")
```

## Privacy & Security

### What Gets Captured

**✓ Work context (yes)**
- Product decisions
- Technical discussions
- Customer insights
- Business rationale

**✗ Personal info (no)**
- Private DMs
- Personal channels
- PII (names, emails, phone numbers)
- Credentials, API keys, secrets

### Redaction

```python
def redact_sensitive_info(text):
    """
    Remove sensitive data before capture
    """
    # PII
    text = redact_emails(text)
    text = redact_phone_numbers(text)
    text = redact_ssn(text)
    
    # Credentials
    text = redact_api_keys(text)
    text = redact_passwords(text)
    text = redact_tokens(text)
    
    # Financial
    text = redact_credit_cards(text)
    text = redact_bank_accounts(text)
    
    return text
```

### User Control

```python
# User can opt out channels/sources
user_config = {
    "slack_channels": {
        "#general": True,
        "#random": False,  # Social channel
        "#exec": False,  # Confidential
    },
    "notion_spaces": {
        "Engineering": True,
        "Personal": False,
    }
}

# User can delete their decisions
@command
def delete_my_decisions(user_id, date_range):
    """
    GDPR: User can delete their captured decisions
    """
    decisions = find_decisions(author=user_id, date=date_range)
    
    for decision in decisions:
        # Delete from enVector
        envector.delete(decision.id)
        
        # Cryptographic erasure: Delete encryption key
        vault.delete_key(decision.key_id)
```

## Configuration

### Example Config

```yaml
# scribe.yml
capture:
  sources:
    slack:
      enabled: true
      channels:
        - "#general"
        - "#product"
        - "#engineering"
        - "#architecture"
      exclude:
        - "#random"
        - "#social"
    
    notion:
      enabled: true
      spaces:
        - "Engineering"
        - "Product"
      page_types:
        - "RFC"
        - "ADR"
        - "Decision Record"
    
    github:
      enabled: true
      repos:
        - "company/backend"
        - "company/frontend"
      labels:
        - "architecture"
        - "design"
    
    meetings:
      enabled: false  # Opt-in
      tools:
        - "zoom"
        - "google-meet"

detection:
  confidence_threshold: 0.7
  auto_capture_threshold: 0.8
  review_required_threshold: 0.5
  
  patterns:
    high_confidence:
      - "we decided to"
      - "decision:"
      - "chose .+ over .+ because"
    
    medium_confidence:
      - "after discussion"
      - "consensus:"
      - "trade-off:"

embedding:
  model: "text-embedding-3-large"
  dimensions: 1536
  
encryption:
  enabled: true
  vault_url: "http://localhost:50080"

storage:
  envector_url: "https://api.envector.io"
  org_id: "your-org-id"

privacy:
  redact_pii: true
  redact_credentials: true
  exclude_dms: true
  user_opt_out: true

monitoring:
  metrics_enabled: true
  alert_threshold:
    capture_rate_min: 0.001
    capture_rate_max: 0.02
    confidence_min: 0.7
```

## Next Steps

After deploying Scribe:
1. Review first 50 captures (ensure quality)
2. Adjust confidence thresholds
3. Add organization-specific patterns
4. Train model on feedback
5. Monitor capture rate and precision

See [Retriever](../agents/retriever.md) for how captured context gets retrieved.