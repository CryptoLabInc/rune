# Cross-Agent Architecture Fixes — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all cross-agent architecture issues: remove provider-specific shims, restructure config, fill Google gaps, consolidate utilities, harden LLMClient.

**Architecture:** Top-level `LLMConfig` owns all provider keys/models. `LLMClient` is the sole LLM interface — no module accesses raw SDK clients. Shared `parse_llm_json()` utility replaces 3 duplicate implementations.

**Tech Stack:** Python 3.11+, TypeScript/ESM, pytest, FastAPI, FastMCP

---

### Task 1: Create shared JSON parsing utility

**Files:**
- Create: `agents/common/llm_utils.py`
- Test: `agents/tests/test_llm_utils.py`

**Step 1: Write the failing test**

Create `agents/tests/test_llm_utils.py`:

```python
"""Tests for shared LLM response parsing utilities."""

import pytest
from agents.common.llm_utils import parse_llm_json


class TestParseLlmJson:
    def test_valid_json(self):
        assert parse_llm_json('{"key": "value"}') == {"key": "value"}

    def test_json_with_markdown_fences(self):
        raw = '```json\n{"capture": true, "reason": "test"}\n```'
        result = parse_llm_json(raw)
        assert result == {"capture": True, "reason": "test"}

    def test_json_with_plain_fences(self):
        raw = '```\n{"a": 1}\n```'
        assert parse_llm_json(raw) == {"a": 1}

    def test_json_embedded_in_text(self):
        raw = 'Here is the result: {"key": "value"} and some trailing text.'
        assert parse_llm_json(raw) == {"key": "value"}

    def test_no_json_returns_empty_dict(self):
        assert parse_llm_json("This is not JSON at all") == {}

    def test_empty_string_returns_empty_dict(self):
        assert parse_llm_json("") == {}

    def test_nested_json(self):
        raw = '{"phases": [{"title": "A"}, {"title": "B"}]}'
        result = parse_llm_json(raw)
        assert len(result["phases"]) == 2

    def test_invalid_json_with_braces_returns_empty(self):
        raw = '{"broken: json'
        assert parse_llm_json(raw) == {}
```

**Step 2: Run test to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_llm_utils.py -v`
Expected: FAIL with `ModuleNotFoundError`

**Step 3: Write minimal implementation**

Create `agents/common/llm_utils.py`:

```python
"""Shared utilities for parsing LLM responses."""

from __future__ import annotations

import json


def parse_llm_json(raw: str) -> dict:
    """Parse JSON from an LLM response, handling code fences and preamble text.

    Tries in order:
    1. Strip markdown code fences, then json.loads
    2. Direct json.loads on the raw string
    3. Extract substring between first '{' and last '}', then json.loads
    4. Return empty dict
    """
    if not raw:
        return {}

    text = raw
    if text.startswith("```"):
        lines = text.split("\n")
        lines = [l for l in lines if not l.strip().startswith("```")]
        text = "\n".join(lines)

    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass

    start = raw.find("{")
    end = raw.rfind("}") + 1
    if start >= 0 and end > start:
        try:
            return json.loads(raw[start:end])
        except json.JSONDecodeError:
            pass

    return {}
```

**Step 4: Run test to verify it passes**

Run: `.venv/bin/python -m pytest agents/tests/test_llm_utils.py -v`
Expected: 8 PASSED

**Step 5: Commit**

```
feat: add shared parse_llm_json utility
```

---

### Task 2: Add LLMConfig dataclass and restructure config

**Files:**
- Modify: `agents/common/config.py`
- Test: `agents/tests/test_config.py` (create)

**Step 1: Write the failing test**

Create `agents/tests/test_config.py`:

```python
"""Tests for config restructuring — LLMConfig and migration."""

import json
import os
import tempfile
import pytest
from unittest.mock import patch


class TestLLMConfig:
    def test_llm_config_defaults(self):
        from agents.common.config import LLMConfig
        cfg = LLMConfig()
        assert cfg.provider == "anthropic"
        assert cfg.tier2_provider == "anthropic"
        assert cfg.anthropic_api_key == ""
        assert cfg.openai_tier2_model == ""
        assert cfg.google_tier2_model == ""

    def test_load_config_new_llm_section(self, tmp_path):
        from agents.common.config import load_config, CONFIG_PATH
        config_data = {
            "llm": {
                "provider": "openai",
                "openai_api_key": "sk-test",
                "openai_model": "gpt-4o",
            },
            "state": "active",
        }
        config_file = tmp_path / "config.json"
        config_file.write_text(json.dumps(config_data))

        with patch("agents.common.config.CONFIG_PATH", config_file):
            cfg = load_config()

        assert cfg.llm.provider == "openai"
        assert cfg.llm.openai_api_key == "sk-test"

    def test_load_config_migrates_from_retriever(self, tmp_path):
        """Old configs with keys in retriever section should still work."""
        from agents.common.config import load_config
        config_data = {
            "retriever": {
                "llm_provider": "openai",
                "openai_api_key": "sk-old",
                "openai_model": "gpt-4o-mini",
                "topk": 10,
            },
            "scribe": {
                "tier2_provider": "openai",
            },
            "state": "active",
        }
        config_file = tmp_path / "config.json"
        config_file.write_text(json.dumps(config_data))

        with patch("agents.common.config.CONFIG_PATH", config_file):
            cfg = load_config()

        assert cfg.llm.provider == "openai"
        assert cfg.llm.openai_api_key == "sk-old"
        assert cfg.llm.tier2_provider == "openai"
        # retriever should retain non-LLM fields
        assert cfg.retriever.topk == 10

    def test_env_var_overrides_llm_config(self, tmp_path):
        from agents.common.config import load_config
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        env = {"OPENAI_API_KEY": "sk-env", "RUNE_LLM_PROVIDER": "openai"}
        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch.dict(os.environ, env, clear=False):
            cfg = load_config()

        assert cfg.llm.openai_api_key == "sk-env"
        assert cfg.llm.provider == "openai"

    def test_save_config_omits_env_keys(self, tmp_path):
        from agents.common.config import load_config, save_config
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        env = {"ANTHROPIC_API_KEY": "sk-from-env"}
        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch("agents.common.config.CONFIG_DIR", tmp_path), \
             patch.dict(os.environ, env, clear=False):
            cfg = load_config()
            save_config(cfg)

        saved = json.loads(config_file.read_text())
        # Keys loaded from env should not be persisted
        assert saved.get("llm", {}).get("anthropic_api_key", "") == ""
```

**Step 2: Run test to verify it fails**

Run: `.venv/bin/python -m pytest agents/tests/test_config.py -v`
Expected: FAIL — `LLMConfig` does not exist

**Step 3: Implement config restructuring**

Modify `agents/common/config.py`:

- Add `LLMConfig` dataclass
- Add `_parse_llm_config(data)` parser with migration logic
- Remove LLM fields from `RetrieverConfig` and `ScribeConfig`
- Add `RuneConfig.llm` field
- Track which keys came from env vars (set `_env_keys: set` on config)
- Update `save_config()` to skip env-sourced keys
- Update `load_config()` env var section for LLM keys

**Step 4: Run test to verify it passes**

Run: `.venv/bin/python -m pytest agents/tests/test_config.py -v`
Expected: PASS

**Step 5: Run full test suite**

Run: `.venv/bin/python -m pytest agents/tests/ -x -q`
Expected: Some tests may fail due to config access changes — note them for fixing in later tasks.

**Step 6: Commit**

```
feat: restructure config with top-level LLMConfig
```

---

### Task 3: Improve LLMClient — logging, Google system_instruction, timeout, "auto" rejection

**Files:**
- Modify: `agents/common/llm_client.py`
- Test: `agents/tests/test_llm_client.py` (create)

**Step 1: Write the failing test**

Create `agents/tests/test_llm_client.py`:

```python
"""Tests for LLMClient provider abstraction."""

import pytest
from unittest.mock import patch, MagicMock
from agents.common.llm_client import LLMClient


class TestLLMClientInit:
    def test_missing_anthropic_key_logs_info(self, caplog):
        import logging
        with caplog.at_level(logging.INFO, logger="rune.common.llm_client"):
            client = LLMClient(provider="anthropic")
        assert not client.is_available
        assert "API key not provided" in caplog.text

    def test_missing_openai_key_logs_info(self, caplog):
        import logging
        with caplog.at_level(logging.INFO, logger="rune.common.llm_client"):
            client = LLMClient(provider="openai")
        assert not client.is_available
        assert "API key not provided" in caplog.text

    def test_missing_google_key_logs_info(self, caplog):
        import logging
        with caplog.at_level(logging.INFO, logger="rune.common.llm_client"):
            client = LLMClient(provider="google")
        assert not client.is_available
        assert "API key not provided" in caplog.text

    def test_auto_provider_raises(self):
        with pytest.raises(ValueError, match="auto"):
            LLMClient(provider="auto")

    def test_unsupported_provider_logs_warning(self, caplog):
        import logging
        with caplog.at_level(logging.WARNING, logger="rune.common.llm_client"):
            client = LLMClient(provider="unsupported")
        assert not client.is_available
        assert "Unsupported" in caplog.text


class TestLLMClientGenerate:
    def test_generate_raises_when_unavailable(self):
        client = LLMClient(provider="anthropic")
        with pytest.raises(RuntimeError, match="not available"):
            client.generate("test")
```

**Step 2: Run test to verify failures**

Run: `.venv/bin/python -m pytest agents/tests/test_llm_client.py -v`
Expected: Some tests fail (missing log, "auto" not rejected)

**Step 3: Implement LLMClient improvements**

Modify `agents/common/llm_client.py`:

- Add `logger.info("%s API key not provided, LLM client unavailable", provider)` for each missing key
- Add `if self.provider == "auto": raise ValueError("...")` at top of `__init__`
- Google `generate()`: create `GenerativeModel` with `system_instruction` when system prompt given, cache by hash
- Google `generate()`: pass `request_options={"timeout": timeout}` to `generate_content()`
- Update module docstring to include Google

**Step 4: Run test to verify passes**

Run: `.venv/bin/python -m pytest agents/tests/test_llm_client.py -v`
Expected: ALL PASSED

**Step 5: Commit**

```
fix: LLMClient — logging, Google system_instruction, reject "auto"
```

---

### Task 4: Remove _client shim from query_processor and synthesizer

**Files:**
- Modify: `agents/retriever/query_processor.py`
- Modify: `agents/retriever/synthesizer.py`
- Modify: `agents/tests/test_retriever.py`

**Step 1: Update query_processor.py**

- Remove line 186: `self._llm_client = getattr(self._llm, "_client", None)`
- Line 200: change condition to `if language.is_english or not self._llm or not self._llm.is_available:`
- Lines 248-255: remove the `else` branch (`self._llm_client.messages.create(...)`)
- Replace `self._parse_llm_query_response` call with `from ..common.llm_utils import parse_llm_json` and use `parse_llm_json(raw)`

**Step 2: Update synthesizer.py**

- Remove line 137: `self._client = getattr(self._llm, "_client", None)`
- Line 142: change `has_llm` to return `self._llm.is_available`
- Lines 212-219: remove the `else` branch (`self._client.messages.create(...)`)

**Step 3: Update tests**

In `agents/tests/test_retriever.py`:

- `TestQueryProcessorMultilingual.mock_llm_processor`: replace `_llm_client` mock with `_llm` mock that has `is_available=True` and `generate()` returning the JSON string
- All assertions using `_llm_client.messages.create` → `_llm.generate`

**Step 4: Run tests**

Run: `.venv/bin/python -m pytest agents/tests/test_retriever.py -v`
Expected: ALL PASSED

**Step 5: Commit**

```
refactor: remove _client shim from retriever modules
```

---

### Task 5: Remove _client shim from llm_extractor and tier2_filter

**Files:**
- Modify: `agents/scribe/llm_extractor.py`
- Modify: `agents/scribe/tier2_filter.py`
- Modify: `agents/tests/test_tier2_filter.py`

**Step 1: Update llm_extractor.py**

- Remove line 210: `self._client = getattr(self._llm, "_client", None)`
- Line 215: `is_available` → return `self._llm.is_available`
- Lines 217-225: `_generate()` → only `self._llm.generate()`, remove `self._client` fallback
- Replace `self._parse_json` with `from ..common.llm_utils import parse_llm_json`

**Step 2: Update tier2_filter.py**

- Remove line 85: `self._client = getattr(self._llm, "_client", None)`
- Lines 87-91: `is_available` → `return self._llm.is_available`
- Lines 117-133: `evaluate()` → only `self._llm.generate()`, remove `self._client` fallback
- Replace `_parse_response` internal JSON parsing with `parse_llm_json`

**Step 3: Update tests**

In `agents/tests/test_tier2_filter.py`:

- `filter_with_mock` fixture: replace `_client` mock with `_llm` mock that has `is_available=True` and `generate()` return
- `test_evaluate_fallback_on_unavailable`: set `_llm` with `is_available=False`
- All `_client.messages.create` assertions → `_llm.generate`

**Step 4: Run tests**

Run: `.venv/bin/python -m pytest agents/tests/test_tier2_filter.py agents/tests/test_pipeline_scenario.py agents/tests/test_record_builder.py -v`
Expected: ALL PASSED

**Step 5: Commit**

```
refactor: remove _client shim from scribe modules
```

---

### Task 6: Fix scribe server — Google provider gap + use LLMConfig

**Files:**
- Modify: `agents/scribe/server.py`

**Step 1: Update server.py lifespan()**

Lines 105-134: replace with:

```python
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
        return llm_cfg.openai_tier2_model if role == "tier2" and llm_cfg.openai_tier2_model else llm_cfg.openai_model
    if provider == "google":
        return llm_cfg.google_tier2_model if role == "tier2" and llm_cfg.google_tier2_model else llm_cfg.google_model
    if role == "tier2":
        return config.scribe.tier2_model
    return llm_cfg.anthropic_model
```

- Pass `google_api_key=google_key` to both `Tier2Filter` and `LLMExtractor` constructors
- Use `_provider_model()` for model selection

**Step 2: Run existing tests**

Run: `.venv/bin/python -m pytest agents/tests/ -x -q`
Expected: ALL PASSED

**Step 3: Commit**

```
fix: scribe server — Google provider gap, use LLMConfig
```

---

### Task 7: Update MCP server — use LLMConfig, tier2 model per provider

**Files:**
- Modify: `mcp/server/server.py`

**Step 1: Update _init_pipelines()**

Lines 728-766: update to read from `rune_config.llm` instead of `rune_config.retriever`:

```python
llm_cfg = rune_config.llm
configured_llm_provider = (llm_cfg.provider or os.getenv("RUNE_LLM_PROVIDER", "anthropic")).lower()
configured_tier2_provider = (llm_cfg.tier2_provider or os.getenv("RUNE_TIER2_LLM_PROVIDER", configured_llm_provider)).lower()
anthropic_key = llm_cfg.anthropic_api_key or os.getenv("ANTHROPIC_API_KEY", "")
openai_key = llm_cfg.openai_api_key or os.getenv("OPENAI_API_KEY", "")
google_key = llm_cfg.google_api_key or os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY") or ""
```

Update `_provider_model()`:
```python
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
        return rune_config.scribe.tier2_model
    return llm_cfg.anthropic_model
```

**Step 2: Verify server starts**

Run: `.venv/bin/python -c "from mcp.server.server import MCPServerApp; print('OK')"`
Expected: `OK`

**Step 3: Commit**

```
refactor: MCP server uses LLMConfig, tier2 model per provider
```

---

### Task 8: TypeScript isActive() mtime caching

**Files:**
- Modify: `src/config.ts`

**Step 1: Implement mtime caching**

Add at module level in `src/config.ts`:

```typescript
let _cachedActive: boolean | null = null;
let _cachedMtimeMs: number = 0;

export function isActive(): boolean {
  try {
    const stat = fs.statSync(CONFIG_PATH);
    if (_cachedActive !== null && stat.mtimeMs === _cachedMtimeMs) {
      return _cachedActive;
    }
    _cachedMtimeMs = stat.mtimeMs;
    const config = loadRuneConfig();
    _cachedActive = config?.state === "active";
    return _cachedActive;
  } catch {
    _cachedActive = null;
    return false;
  }
}

export function isDormant(): boolean {
  return !isActive();
}
```

**Step 2: Commit**

```
perf: cache isActive() by config file mtime
```

---

### Task 9: Update config template, README, versions

**Files:**
- Modify: `config/config.template.json`
- Modify: `config/README.md`
- Modify: `package.json`
- Modify: `openclaw.plugin.json`

**Step 1: Update config.template.json**

```json
{
  "vault": {
    "endpoint": "VAULT_GRPC_ENDPOINT",
    "token": "VAULT_TOKEN"
  },
  "envector": {
    "endpoint": "ENVECTOR_ENDPOINT",
    "api_key": "ENVECTOR_API_KEY"
  },
  "llm": {
    "provider": "anthropic",
    "tier2_provider": "anthropic",
    "anthropic_api_key": "",
    "anthropic_model": "claude-sonnet-4-20250514",
    "openai_api_key": "",
    "openai_model": "gpt-4o-mini",
    "openai_tier2_model": "",
    "google_api_key": "",
    "google_model": "gemini-2.0-flash-exp",
    "google_tier2_model": ""
  },
  "scribe": {
    "tier2_enabled": true,
    "tier2_model": "claude-haiku-4-5-20251001"
  },
  "retriever": {
    "topk": 10,
    "confidence_threshold": 0.5
  },
  "state": "dormant",
  "metadata": {
    "configVersion": "2.0",
    "lastUpdated": null,
    "teamId": null
  }
}
```

**Step 2: Update config/README.md**

- Replace Optional Sections example to show new `llm` section
- Update `llm_provider` / `tier2_provider` docs to reference `llm.provider`
- Add `google_api_key` / `google_model` to environment variables section
- Add `"google"` to provider list

**Step 3: Update versions**

- `package.json` line 3: `"version": "0.2.0"`
- `openclaw.plugin.json` line 5: `"version": "0.2.0"`

**Step 4: Run full test suite**

Run: `.venv/bin/python -m pytest agents/tests/ -x -q`
Expected: ALL PASSED (187+ tests)

**Step 5: Commit**

```
chore: bump version to 0.2.0, update config template and docs
```

---

### Task 10: Final verification

**Step 1: Run full test suite**

Run: `.venv/bin/python -m pytest agents/tests/ -v`
Expected: ALL PASSED

**Step 2: Verify no remaining _client references**

Run: `grep -rn "self\._client" agents/ --include="*.py" | grep -v test | grep -v __pycache__`
Expected: No matches (or only in `envector_client.py` which is unrelated)

**Step 3: Verify no remaining retriever LLM key access**

Run: `grep -rn "config\.retriever\.anthropic_api_key\|config\.retriever\.openai_api_key\|config\.retriever\.google_api_key\|config\.retriever\.llm_provider" agents/ mcp/ --include="*.py" | grep -v __pycache__`
Expected: No matches

**Step 4: Verify TypeScript compiles**

Run: `npx tsc --noEmit 2>&1 || echo "TS check done"`

**Step 5: Commit any remaining fixes**

```
fix: final verification cleanup
```
