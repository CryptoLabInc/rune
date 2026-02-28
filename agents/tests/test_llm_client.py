"""Tests for LLMClient provider abstraction."""

import pytest
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
            client = LLMClient(provider="unsupported_xyz")
        assert not client.is_available
        assert "Unsupported" in caplog.text


class TestLLMClientGenerate:
    def test_generate_raises_when_unavailable(self):
        client = LLMClient(provider="anthropic")
        with pytest.raises(RuntimeError, match="not available"):
            client.generate("test")
