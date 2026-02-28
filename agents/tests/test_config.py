"""Tests for config restructuring -- LLMConfig and migration."""

import json
import os
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
        from agents.common.config import load_config
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
        assert saved.get("llm", {}).get("anthropic_api_key", "") == ""

    def test_save_config_writes_llm_section(self, tmp_path):
        """save_config should write an 'llm' section, not embed keys in retriever."""
        from agents.common.config import load_config, save_config
        config_data = {
            "llm": {
                "provider": "openai",
                "openai_api_key": "sk-file",
                "openai_model": "gpt-4o",
            },
        }
        config_file = tmp_path / "config.json"
        config_file.write_text(json.dumps(config_data))

        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch("agents.common.config.CONFIG_DIR", tmp_path):
            cfg = load_config()
            save_config(cfg)

        saved = json.loads(config_file.read_text())
        assert "llm" in saved
        assert saved["llm"]["provider"] == "openai"
        assert saved["llm"]["openai_api_key"] == "sk-file"
        # retriever section should NOT contain LLM keys
        retriever_section = saved.get("retriever", {})
        assert "llm_provider" not in retriever_section
        assert "anthropic_api_key" not in retriever_section
        assert "openai_api_key" not in retriever_section

    def test_save_config_no_retriever_llm_keys(self, tmp_path):
        """Retriever section in saved output must not contain LLM fields."""
        from agents.common.config import load_config, save_config
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch("agents.common.config.CONFIG_DIR", tmp_path):
            cfg = load_config()
            save_config(cfg)

        saved = json.loads(config_file.read_text())
        retriever_section = saved.get("retriever", {})
        for key in ["llm_provider", "anthropic_api_key", "anthropic_model",
                     "openai_api_key", "openai_model", "google_api_key", "google_model"]:
            assert key not in retriever_section, f"{key} should not be in retriever section"

    def test_save_config_no_scribe_tier2_provider(self, tmp_path):
        """Scribe section in saved output must not contain tier2_provider."""
        from agents.common.config import load_config, save_config
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch("agents.common.config.CONFIG_DIR", tmp_path):
            cfg = load_config()
            save_config(cfg)

        saved = json.loads(config_file.read_text())
        scribe_section = saved.get("scribe", {})
        assert "tier2_provider" not in scribe_section

    def test_rune_config_has_llm_field(self):
        """RuneConfig should have an llm field of type LLMConfig."""
        from agents.common.config import RuneConfig, LLMConfig
        cfg = RuneConfig()
        assert isinstance(cfg.llm, LLMConfig)

    def test_retriever_config_no_llm_fields(self):
        """RetrieverConfig should not have LLM-specific fields."""
        from agents.common.config import RetrieverConfig
        cfg = RetrieverConfig()
        assert not hasattr(cfg, "llm_provider")
        assert not hasattr(cfg, "anthropic_api_key")
        assert not hasattr(cfg, "openai_api_key")
        assert not hasattr(cfg, "google_api_key")

    def test_scribe_config_no_tier2_provider(self):
        """ScribeConfig should not have tier2_provider."""
        from agents.common.config import ScribeConfig
        cfg = ScribeConfig()
        assert not hasattr(cfg, "tier2_provider")

    def test_gemini_api_key_env_var(self, tmp_path):
        """GEMINI_API_KEY should also set google_api_key."""
        from agents.common.config import load_config
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        env = {"GEMINI_API_KEY": "gem-key"}
        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch.dict(os.environ, env, clear=False):
            cfg = load_config()

        assert cfg.llm.google_api_key == "gem-key"

    def test_tier2_env_var_override(self, tmp_path):
        """RUNE_TIER2_LLM_PROVIDER env var should set llm.tier2_provider."""
        from agents.common.config import load_config
        config_file = tmp_path / "config.json"
        config_file.write_text("{}")

        env = {"RUNE_TIER2_LLM_PROVIDER": "openai"}
        with patch("agents.common.config.CONFIG_PATH", config_file), \
             patch.dict(os.environ, env, clear=False):
            cfg = load_config()

        assert cfg.llm.tier2_provider == "openai"

    def test_parse_llm_config_new_section(self):
        """_parse_llm_config reads from data['llm'] when present."""
        from agents.common.config import _parse_llm_config
        data = {
            "llm": {
                "provider": "google",
                "google_api_key": "gk-123",
                "google_model": "gemini-pro",
                "tier2_provider": "openai",
            }
        }
        llm = _parse_llm_config(data)
        assert llm.provider == "google"
        assert llm.google_api_key == "gk-123"
        assert llm.tier2_provider == "openai"

    def test_parse_llm_config_fallback_to_retriever(self):
        """_parse_llm_config falls back to retriever fields for migration."""
        from agents.common.config import _parse_llm_config
        data = {
            "retriever": {
                "llm_provider": "anthropic",
                "anthropic_api_key": "ak-old",
                "anthropic_model": "claude-3-haiku",
                "openai_api_key": "ok-old",
                "openai_model": "gpt-3.5-turbo",
                "google_api_key": "gk-old",
                "google_model": "gemini-1.0",
            },
            "scribe": {
                "tier2_provider": "google",
            },
        }
        llm = _parse_llm_config(data)
        assert llm.provider == "anthropic"
        assert llm.anthropic_api_key == "ak-old"
        assert llm.anthropic_model == "claude-3-haiku"
        assert llm.openai_api_key == "ok-old"
        assert llm.openai_model == "gpt-3.5-turbo"
        assert llm.google_api_key == "gk-old"
        assert llm.google_model == "gemini-1.0"
        assert llm.tier2_provider == "google"
