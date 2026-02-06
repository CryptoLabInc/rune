# Agents

This directory contains agent specifications and implementation guides.

## Agent Types

### Scribe

**Role:** Context capture

**Purpose:** Continuously monitors team communications and artifacts to identify and capture significant decisions, architectural rationale, and institutional knowledge.

**Specification:** [scribe.md](scribe.md)

**Key Features:**
- Watches multiple sources (Slack, Notion, GitHub, meetings)
- Detects significant decisions (pattern + ML)
- Extracts context and metadata
- Encrypts and stores in organizational memory

### Retriever

**Role:** Context retrieval and synthesis

**Purpose:** Searches organizational memory for relevant decisions, synthesizes context from multiple sources, and provides actionable insights.

**Specification:** [retriever.md](retriever.md)

**Key Features:**
- Understands user intent
- Searches encrypted organizational memory
- Decrypts results securely
- Synthesizes comprehensive answers
- Provides actionable insights

## Agent Integration

### How Agents Work with Rune

```
┌──────────────────────────────────────┐
│         Your AI Agent                │
│      (Claude/Gemini/Codex)           │
├──────────────────────────────────────┤
│  1. Load skills from skills/         │
│  2. Connect to MCP servers           │
│  3. Execute agent behaviors          │
│     - Scribe: Capture context        │
│     - Retriever: Search & synthesize │
└──────────────────────────────────────┘
```

### Agent Workflow

**Scribe Workflow:**

```
1. Watch configured sources
   ├─ Slack channels
   ├─ Notion pages
   ├─ GitHub PRs/Issues
   └─ Meeting transcripts

2. Detect significant events
   ├─ Pattern matching
   ├─ Similarity based classification
   └─ Human review (optional)

3. Extract context
   ├─ Decision content
   ├─ Participants
   ├─ Timestamp
   └─ Related artifacts

4. Encrypt and store
   ├─ Generate embedding
   ├─ Encrypt via Vault MCP
   └─ Store in enVector Cloud
```

**Retriever Workflow:**

```
1. Parse user query
   └─ Understand intent

2. Generate search queries
   └─ Expand with synonyms

3. Search memory
   ├─ Encrypt query (Vault MCP)
   ├─ FHE search (enVector Cloud)
   └─ Decrypt results (Vault MCP)

4. Synthesize answer
   ├─ Rank results by relevance
   ├─ Combine multiple sources
   └─ Provide actionable insights
```

## Implementing Agent Behaviors

### Python Example

```python
from rune import ContextMemory, Scribe, Retriever

# Initialize memory
memory = ContextMemory(
    vault_url="https://vault-your-team.oci.envector.io",
    vault_token="evt_xxx"
)

# Scribe
scribe = Scribe(memory)
scribe.watch_source("slack", channels=["#engineering"])
scribe.watch_source("github", repos=["owner/repo"])
scribe.start()

# Retriever
retriever = Retriever(memory)
answer = retriever.answer_query("Why did we choose Postgres?")
print(answer)
```

### JavaScript/TypeScript Example

```typescript
import { ContextMemory, Scribe, Retriever } from 'rune';

// Initialize memory
const memory = new ContextMemory({
  vaultUrl: 'https://vault-your-team.oci.envector.io',
  vaultToken: 'evt_xxx'
});

// Scribe
const scribe = new Scribe(memory);
scribe.watchSource('slack', { channels: ['#engineering'] });
scribe.watchSource('github', { repos: ['owner/repo'] });
await scribe.start();

// Retriever
const retriever = new Retriever(memory);
const answer = await retriever.answerQuery('Why did we choose Postgres?');
console.log(answer);
```

## Custom Agent Behaviors

You can implement custom agent behaviors by extending base classes:

```python
from rune import BaseAgent

class CustomAgent(BaseAgent):
    def __init__(self, memory):
        super().__init__(memory)
    
    async def custom_behavior(self):
        # Your custom logic
        results = await self.memory.search("custom query")
        return self.process(results)
    
    def process(self, results):
        # Custom processing logic
        return {"processed": results}

# Use custom agent
agent = CustomAgent(memory)
result = await agent.custom_behavior()
```

## Agent Configuration

### Environment Variables

```bash
# Vault configuration
export VAULT_URL="https://vault-your-team.oci.envector.io"
export VAULT_TOKEN="evt_xxx"

# Cloud configuration (optional)
export CLOUD_URL="https://api.envector.io"

# Agent-specific settings
export AGENT_LOG_LEVEL="INFO"
export AGENT_CAPTURE_THRESHOLD=0.8  # ML confidence threshold
export AGENT_SEARCH_LIMIT=10        # Max results per search
```

### Configuration Files

**~/.claude/agent-config.yaml** (example for Claude):

```yaml
scribe:
  sources:
    slack:
      enabled: true
      channels: ["#engineering", "#product"]
      patterns:
        - "decision:"
        - "we chose"
        - "architecture:"
    github:
      enabled: true
      repos: ["owner/repo"]
      events: ["pull_request", "issue_comment"]
  
  capture:
    auto_capture: true
    confidence_threshold: 0.8
    human_review: false

retriever:
  search:
    mode: "accurate"  # fast, accurate, exact
    limit: 10
    timeout_ms: 5000
  
  synthesis:
    max_sources: 5
    include_metadata: true
```

## Testing Agents

### Unit Tests

```python
# tests/test_scribe.py
import pytest
from rune import Scribe, ContextMemory

@pytest.fixture
def mock_memory():
    return MockContextMemory()

def test_scribe_detects_decision(mock_memory):
    scribe = Scribe(mock_memory)
    
    text = "Decision: We chose Postgres for JSON support"
    is_significant = scribe.is_significant(text)
    
    assert is_significant == True

def test_scribe_captures_context(mock_memory):
    scribe = Scribe(mock_memory)
    
    result = scribe.capture(
        text="We chose Postgres",
        source="slack",
        metadata={"channel": "#eng"}
    )
    
    assert result.success == True
    assert mock_memory.captured_count == 1
```

### Integration Tests

```python
# tests/test_integration.py
import pytest
from rune import ContextMemory, Scribe, Retriever

@pytest.mark.integration
async def test_capture_and_retrieve():
    # Use test Vault
    memory = ContextMemory(
        vault_url="http://localhost:50080",
        vault_token="test_token"
    )
    
    # Capture
    scribe = Scribe(memory)
    await scribe.capture(
        text="Test decision: chose option A",
        source="test"
    )
    
    # Retrieve
    retriever = Retriever(memory)
    results = await retriever.search("test decision")
    
    assert len(results) > 0
    assert "option A" in results[0].content
```

## Best Practices

1. **Error Handling**: Always handle MCP connection failures
2. **Rate Limiting**: Respect source API rate limits
3. **Logging**: Log all significant events for debugging
4. **Testing**: Write tests for both capture and retrieval
5. **Configuration**: Make agents configurable via environment
6. **Documentation**: Document custom behaviors clearly

## Troubleshooting

### Agent not capturing context

**Check:**
1. Sources configured correctly?
2. Patterns matching content?
3. Confidence threshold too high?
4. Vault connection working?

**Debug:**
```python
agent.set_log_level("DEBUG")
agent.test_source_connection("slack")
agent.test_pattern_matching("sample text")
```

### Agent not finding context

**Check:**
1. Context actually captured?
2. Query embedding correct?
3. Search mode appropriate?
4. Vault decryption working?

**Debug:**
```python
retriever.set_log_level("DEBUG")
results = retriever.search("query", debug=True)
print(results.debug_info)
```

## Next Steps

- Read agent specifications: [scribe.md](scribe.md), [retriever.md](retriever.md)
- Try example workflows: [../examples/](../examples/)
- Integrate with your agent: [../docs/AGENT-INTEGRATION.md](../docs/AGENT-INTEGRATION.md)
- Join community discussions
