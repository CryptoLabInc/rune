# Cross-Agent Architecture Fixes — Design Document

Date: 2026-02-28
Version: 0.2.0

## Summary

Fix all issues found in the cross-agent architecture review: remove provider-specific backward compatibility shims, restructure config to a top-level `llm` section, fill Google provider gaps, consolidate duplicated utilities, and improve LLMClient robustness.

## Changes

### 1. Config restructure — top-level `llm` section

**New `LLMConfig` dataclass** with all LLM-related fields:
- `provider`, `tier2_provider`
- `anthropic_api_key`, `anthropic_model`
- `openai_api_key`, `openai_model`, `openai_tier2_model`
- `google_api_key`, `google_model`, `google_tier2_model`

**Remove from `RetrieverConfig`**: `llm_provider`, `anthropic_api_key`, `anthropic_model`, `openai_api_key`, `openai_model`, `google_api_key`, `google_model`

**Remove from `ScribeConfig`**: `tier2_provider`

**Migration**: `load_config()` reads from `llm` section first; falls back to `retriever` section for backward compat.

**`save_config()`**: Skip writing API keys that were loaded from environment variables (track origin).

### 2. Remove `_client` backward compat shim

All 4 modules (`query_processor`, `synthesizer`, `llm_extractor`, `tier2_filter`):
- Remove `self._client = getattr(self._llm, "_client", None)`
- Remove all `self._client.messages.create(...)` fallback paths
- Use `self._llm.generate()` exclusively
- Simplify `is_available` to `self._llm.is_available`

Update tests to mock `LLMClient.generate()` instead of `_client.messages.create()`.

### 3. LLMClient improvements

- Log `logger.info()` when API key is missing (not silent return)
- Google: use `system_instruction` parameter in model constructor
- Google: cache `GenerativeModel` instances by system prompt hash
- Google: apply timeout via `request_options`
- Reject `"auto"` as provider with `ValueError`

### 4. Scribe server Google provider gap

- Add `google_key` resolution
- Update `_provider_key()` to handle all 3 providers
- Pass `google_api_key` to `Tier2Filter` and `LLMExtractor` constructors

### 5. JSON parsing utility consolidation

New `agents/common/llm_utils.py`:
- `parse_llm_json(raw: str) -> dict` — strip code fences, try json.loads, fallback to brace extraction

Replace duplicated logic in `query_processor._parse_llm_query_response()`, `llm_extractor._parse_json()`, `tier2_filter._parse_response()`.

### 6. TypeScript `isActive()` caching

- Track file mtime; skip re-read if unchanged
- Eliminates disk IO on every hook invocation

### 7. Version and docs

- `package.json` and `openclaw.plugin.json` → `0.2.0`
- `config.template.json` → new `llm` section with all fields including google
- `config/README.md` → updated schema docs
- OpenAI/Google tier2 model fields in config

## File Change Map

| File | Change |
|------|--------|
| `agents/common/config.py` | Add `LLMConfig`, restructure, migration, save_config fix |
| `agents/common/llm_client.py` | Logging, Google system_instruction, timeout, reject "auto" |
| `agents/common/llm_utils.py` | NEW — shared `parse_llm_json()` |
| `agents/retriever/query_processor.py` | Remove _client shim, use llm_utils |
| `agents/retriever/synthesizer.py` | Remove _client shim |
| `agents/scribe/llm_extractor.py` | Remove _client shim, use llm_utils |
| `agents/scribe/tier2_filter.py` | Remove _client shim, use llm_utils |
| `agents/scribe/server.py` | Google key, _provider_key fix, use LLMConfig |
| `mcp/server/server.py` | Use LLMConfig, tier2 model per provider |
| `agents/tests/test_tier2_filter.py` | Mock LLMClient.generate |
| `agents/tests/test_retriever.py` | Mock LLMClient.generate |
| `config/config.template.json` | New llm section |
| `config/README.md` | Updated docs |
| `src/config.ts` | isActive mtime caching |
| `package.json` | Version 0.2.0 |
| `openclaw.plugin.json` | Version 0.2.0 |
