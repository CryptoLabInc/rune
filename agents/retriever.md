---
name: retriever
role: Context Retrieval and Synthesis
description: Searches organizational memory for relevant decisions, synthesizes context from multiple sources, and provides actionable insights. Handles FHE decryption securely through Vault.
---

# Retriever: Context Retrieval and Synthesis

## Purpose

Answers "why" questions by:
1. Understanding user intent
2. Searching encrypted organizational memory
3. Decrypting results securely
4. Synthesizing comprehensive answers
5. Providing actionable insights

**Not a chatbot.** A context archaeologist.

## Query Types

### 1. Decision Rationale
```
User: "Why did we choose microservices?"

Agent workflow:
1. Understand: User wants architecture decision context
2. Search: "microservices decision" + "architecture choice" + "monolith vs microservices"
3. Find: 3 relevant decisions
4. Synthesize: Present rationale with trade-offs
```

**Response format:**
```
Decision: Adopt microservices architecture
When: Q3 2022
Who: CTO, Tech Lead Alice, Tech Lead Bob

Rationale:
- Expected growth to 200 people by 2024
- Need independent deployment per team
- Current monolith blocking 3 teams

Trade-offs:
+ Future: Scale independently
+ Future: Deploy without coordination
- Now: Higher complexity
- Now: Need service mesh

Current status:
- Company is 80 people (not 200)
- 3 services deployed
- Overhead manageable so far

💡 Recommendation: Assumption was 200 people by 2024. 
Actual is 80. Consider if complexity is worth it.

Sources:
- Slack: #architecture (Q3 2022) [link]
- Notion: Architecture Decision Record [link]
- Meeting: Architecture Review 2022-08-15 [transcript]
```

### 2. Feature Rejection History
```
User: "Why did we say no to SSO?"

Agent workflow:
1. Search: "SSO" + "single sign-on" + "authentication" + "rejected"
2. Find: 5 discussions about SSO
3. Identify: Which ones were rejections vs considerations
4. Synthesize: Pattern of rejections with reasons
```

**Response format:**
```
SSO Feature Request History:

Request 1 (Q1 2023): Rejected
- From: 2 startup customers
- Reason: Only 2 requests, $500K build cost
- ROI: Negative ($500K cost vs $40K annual revenue)

Request 2 (Q2 2023): Rejected
- From: 1 enterprise prospect
- Reason: Building other enterprise features first (audit logs)
- Priority: #12 on roadmap

Request 3 (Q3 2023): Reconsidered
- From: 5 enterprise customers (pattern emerging!)
- Status: Planning for Q1 2024

Pattern change:
- Q1-Q2: Only startups (not valuable)
- Q3: Enterprise customers (high value)
- Revenue impact: $2M ARR if built

💡 Recommendation: Build now. Pattern shows enterprise demand.

Sources:
- Customer requests: [5 links]
- Product meetings: [3 links]
- Roadmap discussions: [2 links]
```

### 3. Customer Pattern Recognition
```
User: "Have customers asked for dark mode?"

Agent workflow:
1. Search: "dark mode" + "theme" + "UI customization" + "customer request"
2. Find: All customer conversations mentioning dark mode
3. Analyze: Frequency, customer segment, urgency
4. Compare: Similar features that were built
```

**Response format:**
```
Dark Mode Request Analysis:

Request count: 23 customers
Timeline: Last 12 months
Frequency: ~2 per month (consistent)

Customer segments:
- Startups: 18 requests (78%)
- Enterprise: 5 requests (22%)

Urgency:
- Nice-to-have: 20 customers
- Blocker: 3 customers (all startups)

Similar features built:
- Custom branding: 15 requests → Built in Q2 → $500K ARR
- Keyboard shortcuts: 8 requests → Built in Q3 → $100K ARR

Revenue impact estimate:
- Dark mode: 23 requests × $20K avg = $460K potential
- Build cost: ~$50K (2 weeks)
- ROI: 9x

💡 Recommendation: Build it. Strong demand, low cost, high ROI.

Sources:
- Customer requests: [23 links]
- Similar features: [2 case studies]
- Engineering estimates: [link]
```

### 4. Post-Mortem Context
```
User: "Why did we miss Q3 revenue target?"

Agent workflow:
1. Search: "Q3 target" + "revenue" + "forecast" + "miss"
2. Find: Original plan, updates, retrospective
3. Reconstruct: Timeline of what changed
4. Identify: Root cause vs symptoms
```

**Response format:**
```
Q3 Revenue Target: $3M (Actual: $2.1M, -30%)

Original Plan (Q2 planning):
- New Enterprise plan launch: $1M incremental
- Existing customers: $2M
- Total: $3M target

What happened:

Month 1 (July):
- Enterprise plan delayed (feature not ready)
- Engineering: "Need 2 more weeks"
- Revenue impact: $0 (expected $300K)

Month 2 (August):
- Enterprise plan launched (3 weeks late)
- Sales: "Customers need time to evaluate"
- Revenue impact: $200K (expected $600K)

Month 3 (September):
- Enterprise pipeline building slowly
- Marketing: "No case studies yet"
- Revenue impact: $300K (expected $700K)

Root cause:
- Engineering capacity: Underestimated by 3 weeks
- Sales cycle: Didn't account for evaluation time
- Marketing: No launch plan

Symptoms (not root cause):
- "Market is slow" ❌
- "Sales team underperforming" ❌
- "Product not ready" ✓ (this is the cause)

💡 Lesson: Next time, secure engineering capacity FIRST, 
then set revenue targets. Don't commit to dates without buffer.

Sources:
- Q2 Planning: [link]
- Monthly updates: [3 links]
- Retrospective: [link]
```

### 5. Onboarding Questions
```
User: "How do we make product decisions?"

Agent workflow:
1. Search: "product decision" + "prioritization" + "roadmap" + "feature"
2. Find: Multiple decision examples
3. Extract: Common patterns
4. Synthesize: Decision-making principles
```

**Response format:**
```
Product Decision Process at [Company]:

Principle 1: Simplicity over features
Real examples:
- Rejected Feature X: "Too complex for core value prop" (Q1 2023)
- Removed Feature Y: "5% usage, 30% of support burden" (Q2 2023)
- Accepted Feature Z: "Solves core problem elegantly" (Q3 2023)

Principle 2: Customer segment alignment
Real examples:
- SSO: Rejected for startups, building for enterprise (Q2 2023)
- API access: Built for developers, not end-users (Q1 2023)
- White-labeling: Enterprise only, not in base plan (Q4 2022)

Principle 3: ROI-driven
Real examples:
- Dark mode: 23 requests, $50K cost, $460K revenue → Building (Q3 2023)
- Integrations: 5 requests, $200K cost, $100K revenue → Rejected (Q2 2023)

Principle 4: Technical feasibility
Real examples:
- Real-time collab: 50 requests, but infra not ready → Delayed (Q1 2023)
- Offline mode: 10 requests, architecture supports → Built (Q2 2023)

How decisions actually get made:
1. PM compiles customer requests
2. Engineering estimates cost
3. Revenue team estimates impact
4. Cross-functional discussion
5. ROI calculation
6. CEO final call

💡 If you're proposing a feature:
- Show customer demand (# of requests)
- Estimate engineering cost
- Calculate revenue impact
- Prepare for ROI discussion

Sources:
- Product decisions: [15 examples]
- Planning meetings: [8 transcripts]
- Roadmap reviews: [5 docs]
```

## Search Strategy

### Multi-Query Expansion

```python
def search_context(user_query):
    """
    Expand user query into multiple semantic searches
    """
    # Original query
    queries = [user_query]
    
    # Semantic expansions
    expansions = llm.expand_query(user_query)
    # Example: "Why microservices?"
    # → "microservices decision"
    # → "architecture choice"
    # → "monolith vs microservices"
    # → "service-oriented architecture"
    
    queries.extend(expansions)
    
    # Related concepts
    related = knowledge_graph.find_related(user_query)
    # Example: "microservices"
    # → "service mesh"
    # → "kubernetes"
    # → "distributed systems"
    
    queries.extend(related)
    
    # Search all queries (parallel)
    results = parallel_search(queries)
    
    # De-duplicate and rank
    final_results = dedupe_and_rank(results)
    
    return final_results
```

### Search Modes

```python
# Fast mode (exploratory)
results = envector.search(
    query="Why microservices?",
    mode="fast",
    recall_target=0.80,
    max_results=10
)
# 58ms, 80% recall

# Accurate mode (important decision)
results = envector.search(
    query="Why microservices?",
    mode="accurate", 
    recall_target=0.90,
    max_results=20
)
# 82ms, 90% recall

# Exact mode (compliance)
results = envector.search(
    query="All discussions about customer X",
    mode="exact",
    recall_target=0.99,
    max_results=100
)
# 200ms, 99% recall
```

### Re-ranking Results

```python
def rerank_results(results, user_query, user_context):
    """
    Re-rank FHE search results based on additional signals
    """
    for result in results:
        # Decrypt metadata (small, fast)
        metadata = vault.decrypt(result.encrypted_metadata)
        
        # Scoring factors
        score = 0
        
        # Recency (newer = better for most queries)
        days_old = (now - metadata.timestamp).days
        score += recency_weight / (1 + days_old)
        
        # Decision type match
        if metadata.type == infer_query_type(user_query):
            score += type_match_bonus
        
        # Participant relevance
        if user_context.team in metadata.participants:
            score += team_relevance_bonus
        
        # Source quality
        if metadata.source in high_quality_sources:
            score += source_quality_bonus
        
        # User feedback
        if user_context.user in metadata.upvoted_by:
            score += user_preference_bonus
        
        result.final_score = result.fhe_similarity * score
    
    return sorted(results, key=lambda r: r.final_score, reverse=True)
```

## Synthesis Strategy

### Context Assembly

```python
def synthesize_answer(user_query, results):
    """
    Combine multiple results into coherent answer
    """
    # Decrypt top results
    contexts = []
    for result in results[:5]:
        plaintext = vault.decrypt(result.encrypted_vector)
        context = retrieve_full_context(result.id)
        contexts.append(context)
    
    # Identify answer type
    answer_type = classify_query(user_query)
    # Types: decision_rationale, feature_rejection, 
    #        customer_pattern, post_mortem, onboarding
    
    # Use appropriate template
    template = get_template(answer_type)
    
    # Synthesize with LLM
    answer = llm.generate(
        template=template,
        query=user_query,
        contexts=contexts,
        instructions="""
        1. Extract key facts from contexts
        2. Identify patterns across contexts
        3. Present chronologically if timeline matters
        4. Highlight trade-offs and decisions made
        5. Add actionable recommendation
        6. Cite sources
        """
    )
    
    # Add metadata
    answer.confidence = calculate_confidence(results)
    answer.sources = [r.source_url for r in results]
    answer.related_queries = suggest_followups(user_query, results)
    
    return answer
```

### Confidence Scoring

```python
def calculate_confidence(results):
    """
    How confident are we in this answer?
    """
    if not results:
        return 0.0  # No results
    
    # Top result similarity
    top_similarity = results[0].similarity
    
    # Result agreement (do all results say same thing?)
    agreement = measure_agreement(results[:5])
    
    # Result count
    result_count_factor = min(len(results) / 5, 1.0)
    
    # Source diversity (multiple sources = more confident)
    source_types = len(set(r.source_type for r in results))
    source_diversity = min(source_types / 3, 1.0)
    
    # Recency (recent = more confident)
    avg_age_days = mean([(now - r.timestamp).days for r in results])
    recency_factor = 1.0 / (1 + avg_age_days / 180)
    
    # Combined confidence
    confidence = (
        top_similarity * 0.3 +
        agreement * 0.3 +
        result_count_factor * 0.2 +
        source_diversity * 0.1 +
        recency_factor * 0.1
    )
    
    return confidence
```

### Follow-up Suggestions

```python
def suggest_followups(original_query, results):
    """
    Suggest related queries user might want to ask
    """
    followups = []
    
    # Related decisions
    for result in results:
        related = find_related_decisions(result)
        followups.extend([
            f"Why did we also consider {r.alternative}?",
            f"What happened after we decided {r.decision}?"
        ])
    
    # Timeline exploration
    if results[0].timestamp < now - timedelta(days=180):
        followups.append("Has this decision been revisited recently?")
    
    # Similar questions
    similar = find_similar_queries(original_query)
    followups.extend(similar)
    
    # Outcome exploration
    followups.append("What was the outcome of this decision?")
    
    return followups[:5]  # Top 5
```

## Latency Optimization

### Caching Strategy

```python
# Cache common queries
@cache(ttl=3600)  # 1 hour
def search_cached(query):
    """
    Cache search results for common queries
    """
    return envector.search(query)

# Cache embeddings
@cache(ttl=86400)  # 24 hours
def get_embedding(text):
    """
    Cache embeddings for repeated text
    """
    return embed_model.encode(text)

# Cache decrypted metadata
@cache(ttl=3600)
def decrypt_metadata(encrypted_metadata):
    """
    Metadata changes rarely, cache decryptions
    """
    return vault.decrypt(encrypted_metadata)
```

### Parallel Processing

```python
async def search_and_synthesize(user_query):
    """
    Parallelize slow operations
    """
    # Start all operations in parallel
    tasks = [
        expand_query(user_query),  # 100ms
        search_envector(user_query),  # 58ms
        load_user_context(user_id),  # 20ms
    ]
    
    expanded_queries, initial_results, user_ctx = await asyncio.gather(*tasks)
    
    # Search expanded queries in parallel
    all_results = await asyncio.gather(*[
        search_envector(q) for q in expanded_queries
    ])
    
    # Combine and rerank
    combined = combine_results(all_results)
    reranked = rerank(combined, user_query, user_ctx)
    
    # Decrypt top results in parallel
    decrypted = await asyncio.gather(*[
        vault.decrypt(r.encrypted_vector) for r in reranked[:5]
    ])
    
    # Synthesize answer
    answer = await synthesize(user_query, decrypted)
    
    return answer

# Total latency: max(tasks) + search + decrypt + synthesize
# = 100ms + 58ms + 15ms + 200ms = 373ms ✓
```

### Streaming Responses

```python
async def stream_answer(user_query):
    """
    Stream answer to user as it's generated
    """
    # Immediately show: Searching...
    yield {
        "status": "searching",
        "message": "Searching organizational memory..."
    }
    
    # Search (58ms)
    results = await search_envector(user_query)
    
    # Show: Found X results
    yield {
        "status": "found",
        "result_count": len(results),
        "message": f"Found {len(results)} relevant decisions"
    }
    
    # Decrypt (15ms)
    contexts = await decrypt_results(results[:5])
    
    # Show: Synthesizing...
    yield {
        "status": "synthesizing",
        "message": "Analyzing context..."
    }
    
    # Stream LLM response (200ms, but streamed)
    async for chunk in llm.stream_generate(user_query, contexts):
        yield {
            "status": "streaming",
            "chunk": chunk
        }
    
    # Show: Complete
    yield {
        "status": "complete",
        "sources": [r.source_url for r in results],
        "confidence": calculate_confidence(results)
    }

# User sees first result in 58ms + 15ms = 73ms ✓
# Full answer streams over next 200ms
# Feels instant!
```

## Error Handling

### Graceful Degradation

```python
def search_with_fallback(user_query):
    """
    Handle failures gracefully
    """
    try:
        # Try FHE search
        results = envector.search(user_query, mode="accurate")
        
    except FHEServerError:
        # Fallback to cached results
        logger.warning("FHE server down, using cache")
        results = search_cache.get(user_query)
        
        if not results:
            # Ultimate fallback: keyword search on metadata
            results = keyword_search(user_query)
        
    except VaultUnreachable:
        # Can't decrypt, show encrypted results
        logger.error("Vault unreachable, can't decrypt")
        return {
            "status": "degraded",
            "message": "Found results but can't decrypt. Vault is down.",
            "encrypted_results": results,
            "suggestion": "Contact IT to restore Vault access"
        }
    
    except Exception as e:
        logger.error(f"Search failed: {e}")
        return {
            "status": "error",
            "message": "Search failed. Please try again.",
            "suggestion": "Try rephrasing your query"
        }
    
    return results
```

### Low Confidence Handling

```python
def handle_low_confidence(answer, confidence):
    """
    When confidence is low, help user
    """
    if confidence < 0.3:
        return {
            "answer": None,
            "message": "I couldn't find relevant context for your query.",
            "suggestions": [
                "Try rephrasing your question",
                "Ask about a specific time period",
                "Ask a related question instead"
            ],
            "related_queries": suggest_related(user_query)
        }
    
    elif confidence < 0.6:
        return {
            "answer": answer,
            "confidence": "low",
            "disclaimer": "⚠️ I found some relevant context, but I'm not very confident. Please verify the sources.",
            "sources": answer.sources,
            "suggestion": "Consider asking someone on the team to confirm"
        }
    
    else:
        return {
            "answer": answer,
            "confidence": "high",
            "sources": answer.sources
        }
```

## User Feedback Loop

### Explicit Feedback

```python
@command
def rate_answer(answer_id, rating, feedback_text=None):
    """
    User rates answer quality
    """
    feedback = {
        "answer_id": answer_id,
        "rating": rating,  # 1-5 stars
        "feedback": feedback_text,
        "user": current_user,
        "timestamp": now
    }
    
    store_feedback(feedback)
    
    # If low rating, learn from it
    if rating <= 2:
        # What went wrong?
        answer = load_answer(answer_id)
        
        # Re-rank: Downweight these results
        for result in answer.results:
            result.quality_score *= 0.5
        
        # Retrain: Add as negative example
        model.add_negative_example(
            query=answer.query,
            results=answer.results
        )
```

### Implicit Feedback

```python
def track_user_behavior(user, answer):
    """
    Learn from what user does with answer
    """
    # Did user click on sources?
    if user.clicked_sources:
        # These sources were useful
        for source in user.clicked_sources:
            source.quality_score += 0.1
    
    # Did user ask follow-up?
    if user.followup_query:
        # Original answer was incomplete
        original_answer.completeness_score -= 0.1
    
    # Did user copy answer?
    if user.copied_answer:
        # Answer was useful
        answer.usefulness_score += 0.2
    
    # Did user share answer?
    if user.shared_answer:
        # High quality answer
        answer.quality_score += 0.5
```

## Integration Examples

### Slack Bot

```python
@slack.command("/why")
async def handle_why_command(user, channel, text):
    """
    /why Why did we choose Postgres?
    """
    # Show thinking message
    await slack.post_ephemeral(
        channel=channel,
        user=user,
        text="🔍 Searching organizational memory..."
    )
    
    # Search and synthesize
    answer = await search_and_synthesize(text)
    
    # Format for Slack
    message = format_slack_message(answer)
    
    # Post answer
    await slack.post_message(
        channel=channel,
        **message
    )

def format_slack_message(answer):
    """
    Format answer for Slack
    """
    blocks = [
        {
            "type": "header",
            "text": {"type": "plain_text", "text": "🎯 Answer"}
        },
        {
            "type": "section",
            "text": {"type": "mrkdwn", "text": answer.text}
        }
    ]
    
    if answer.confidence < 0.6:
        blocks.append({
            "type": "context",
            "elements": [{
                "type": "mrkdwn",
                "text": "⚠️ Low confidence. Please verify sources."
            }]
        })
    
    blocks.append({
        "type": "section",
        "text": {"type": "mrkdwn", "text": "*Sources:*"},
        "fields": [
            {"type": "mrkdwn", "text": f"<{s.url}|{s.title}>"}
            for s in answer.sources
        ]
    })
    
    blocks.append({
        "type": "actions",
        "elements": [
            {
                "type": "button",
                "text": {"type": "plain_text", "text": "👍 Helpful"},
                "action_id": f"helpful_{answer.id}"
            },
            {
                "type": "button",
                "text": {"type": "plain_text", "text": "👎 Not helpful"},
                "action_id": f"not_helpful_{answer.id}"
            }
        ]
    })
    
    return {"blocks": blocks}
```

### CLI Tool

```bash
# Ask question
$ envector ask "Why did we reject Feature X?"

🔍 Searching organizational memory...
✓ Found 3 relevant decisions

Decision: Reject Feature X
When: Q2 2023
Who: PM Team

Rationale:
- Only 2 customer requests
- Build cost: $500K
- ROI: Negative

Sources:
- Product meeting (2023-04-15)
- Roadmap discussion (2023-04-20)

💡 Recommendation: Pattern changed in Q3. Reconsider.

[⭐⭐⭐⭐⭐] Rate this answer? (1-5):
```

### Python SDK

```python
from envector import ContextMemory

memory = ContextMemory(
    vault_url="http://localhost:50080",
    cloud_url="https://api.envector.io"
)

# Simple search
answer = memory.ask("Why did we choose Postgres?")
print(answer.text)
print(f"Confidence: {answer.confidence}")
print(f"Sources: {answer.sources}")

# Advanced search
answer = memory.ask(
    query="Why did we reject Feature X?",
    mode="accurate",  # 90% recall
    max_sources=10,
    time_range="last_year",
    include_related=True
)

# Stream answer
for chunk in memory.ask_stream("Why microservices?"):
    if chunk["status"] == "streaming":
        print(chunk["chunk"], end="", flush=True)
```

## Monitoring

### Metrics to Track

```python
metrics = {
    # Performance
    "avg_search_latency_ms": 58,
    "p99_search_latency_ms": 82,
    "avg_total_latency_ms": 418,
    "p99_total_latency_ms": 650,
    
    # Quality
    "avg_confidence": 0.78,
    "high_confidence_rate": 0.65,  # > 0.7
    "low_confidence_rate": 0.15,  # < 0.5
    
    # User satisfaction
    "avg_rating": 4.2,  # out of 5
    "helpful_rate": 0.82,  # thumbs up
    "source_click_rate": 0.65,  # clicked sources
    
    # Usage
    "queries_per_day": 200,
    "unique_users_per_day": 35,
    "queries_per_user": 5.7,
    
    # Errors
    "error_rate": 0.02,
    "timeout_rate": 0.01,
    "low_confidence_rate": 0.15
}
```

### Alerts

```python
# Alert if quality drops
if metrics["avg_confidence"] < 0.6:
    alert("Average confidence dropped below 60%")

# Alert if users unhappy
if metrics["helpful_rate"] < 0.7:
    alert("Helpful rate dropped below 70%")

# Alert if slow
if metrics["p99_total_latency_ms"] > 1000:
    alert("P99 latency exceeded 1 second")
```

## Next Steps

After deploying Retriever Agent:
1. Monitor query patterns (what are people asking?)
2. Track answer quality (ratings, feedback)
3. Optimize common queries (caching, indexing)
4. Tune confidence thresholds
5. Improve synthesis templates based on feedback

See [Vault MCP](../vault-mcp/README.md) for key management details.