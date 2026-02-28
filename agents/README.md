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

## Shared Modules (`common/`)

| Module | Purpose |
|--------|---------|
| `config.py` | `LLMConfig` dataclass, `load_config()` / `save_config()` with env-var override |
| `llm_client.py` | `LLMClient` — unified `generate()` interface for Anthropic, OpenAI, and Google |
| `llm_utils.py` | `parse_llm_json()` — shared JSON extraction from LLM responses |
| `embedding_service.py` | Local embedding via sentence-transformers |
| `schemas.py` | Shared data schemas |

Both Scribe and Retriever use `LLMClient` for all LLM calls. The provider is configured via the top-level `llm` section in `~/.rune/config.json` or environment variables.

## How Agents Work with Rune

Agents interact with organizational memory through MCP tools exposed by envector-mcp-server:

- **`capture`**: Store organizational decisions via 3-tier pipeline (embedding similarity → LLM filter → LLM extraction → FHE-encrypted storage).
- **`recall`**: Search and synthesize answers from shared team memory. Uses Vault-secured pipeline — secret key never leaves Vault. Returns LLM-synthesized answers with source citations.
- **`search`**: Search the operator's own encrypted vector data. Secret key is held locally by the MCP server runtime.

### Capture Workflow (Scribe)

1. Detect significant context (pattern matching on conversation)
2. Call `capture` tool → 3-tier pipeline → encrypt and store in enVector Cloud

### Retrieval Workflow (Retriever)

1. Parse user query (intent detection, entity extraction, query expansion)
2. Call `recall` tool, which orchestrates:
   - Multi-query encrypted similarity scoring on enVector Cloud → result ciphertext
   - Rune-Vault decrypts result ciphertext with secret key, selects top-k
   - Retrieve and decrypt metadata for top-k indices
   - LLM synthesis with certainty respect
3. Return synthesized answer with source citations

## Next Steps

- Read agent specifications: [scribe.md](scribe.md), [retriever.md](retriever.md)
- Try example workflows: [../examples/](../examples/)
