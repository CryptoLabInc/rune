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

### Stage 2: Similarity based classification (Deep Analysis)

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
    "model": "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2",
    "vector": [...],  // 384-dim
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

## Privacy & Security

- All data is FHE-encrypted before storage on enVector Cloud
- Sensitive data (API keys, passwords, tokens, PII) should be redacted before capture
- See [patterns/capture-triggers.md](../patterns/capture-triggers.md) for redaction rules

## Next Steps

See [Retriever](../agents/retriever.md) for how captured context gets retrieved.