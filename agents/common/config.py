"""
Configuration Management for Rune Agents

Loads configuration from ~/.rune/config.json and environment variables.
"""

import os
import json
from pathlib import Path
from dataclasses import dataclass, field

# Default config paths
CONFIG_DIR = Path.home() / ".rune"
CONFIG_PATH = CONFIG_DIR / "config.json"
LOGS_DIR = CONFIG_DIR / "logs"
KEYS_DIR = CONFIG_DIR / "keys"
REVIEW_QUEUE_PATH = CONFIG_DIR / "review_queue.json"

# Project paths (relative to this file)
PROJECT_ROOT = Path(__file__).parent.parent.parent  # rune/
PATTERNS_DIR = PROJECT_ROOT / "patterns"
MCP_SERVER_DIR = PROJECT_ROOT / "mcp" / "server"


@dataclass
class VaultConfig:
    """Rune-Vault configuration"""
    endpoint: str = ""
    token: str = ""


@dataclass
class EnVectorConfig:
    """enVector Cloud configuration"""
    endpoint: str = "localhost:50050"
    api_key: str = ""


@dataclass
class EmbeddingConfig:
    """Embedding model configuration"""
    mode: str = "femb"  # fastembed (on-device)
    model: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"


@dataclass
class LLMConfig:
    """Shared LLM provider configuration across all agents"""
    provider: str = "anthropic"
    tier2_provider: str = "anthropic"
    anthropic_api_key: str = ""
    anthropic_model: str = "claude-sonnet-4-20250514"
    openai_api_key: str = ""
    openai_model: str = "gpt-4o-mini"
    openai_tier2_model: str = ""
    google_api_key: str = ""
    google_model: str = "gemini-2.0-flash-exp"
    google_tier2_model: str = ""


@dataclass
class ScribeConfig:
    """Scribe agent configuration"""
    slack_webhook_port: int = 8080
    similarity_threshold: float = 0.35  # Tier 1: wider net (Tier 2 LLM handles precision)
    auto_capture_threshold: float = 0.7
    tier2_enabled: bool = True  # Tier 2: Haiku-based policy filter
    tier2_model: str = "claude-haiku-4-5-20251001"
    patterns_path: str = str(PATTERNS_DIR / "capture-triggers.md")
    slack_signing_secret: str = ""
    notion_signing_secret: str = ""


@dataclass
class RetrieverConfig:
    """Retriever agent configuration"""
    topk: int = 10
    confidence_threshold: float = 0.5


@dataclass
class RuneConfig:
    """Main Rune configuration"""
    vault: VaultConfig = field(default_factory=VaultConfig)
    envector: EnVectorConfig = field(default_factory=EnVectorConfig)
    embedding: EmbeddingConfig = field(default_factory=EmbeddingConfig)
    llm: LLMConfig = field(default_factory=LLMConfig)
    scribe: ScribeConfig = field(default_factory=ScribeConfig)
    retriever: RetrieverConfig = field(default_factory=RetrieverConfig)
    state: str = "dormant"  # "active" or "dormant"
    _env_sourced_keys: set = field(default_factory=set, repr=False)


def _parse_vault_config(data: dict) -> VaultConfig:
    """Parse vault section from config dict"""
    vault_data = data.get("vault", {})
    return VaultConfig(
        endpoint=vault_data.get("endpoint") or vault_data.get("url", ""),
        token=vault_data.get("token", ""),
    )


def _parse_envector_config(data: dict) -> EnVectorConfig:
    """Parse envector section from config dict"""
    envector_data = data.get("envector", {})
    return EnVectorConfig(
        endpoint=envector_data.get("endpoint", "localhost:50050"),
        api_key=envector_data.get("api_key", ""),
    )


def _parse_embedding_config(data: dict) -> EmbeddingConfig:
    """Parse embedding section from config dict"""
    embedding_data = data.get("embedding", {})
    return EmbeddingConfig(
        mode=embedding_data.get("mode", "femb"),
        model=embedding_data.get("model", "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"),
    )


def _parse_scribe_config(data: dict) -> ScribeConfig:
    """Parse scribe section from config dict"""
    scribe_data = data.get("scribe", {})
    return ScribeConfig(
        slack_webhook_port=scribe_data.get("slack_webhook_port", 8080),
        similarity_threshold=scribe_data.get("similarity_threshold", 0.35),
        auto_capture_threshold=scribe_data.get("auto_capture_threshold", 0.7),
        tier2_enabled=scribe_data.get("tier2_enabled", True),
        tier2_model=scribe_data.get("tier2_model", "claude-haiku-4-5-20251001"),
        patterns_path=scribe_data.get("patterns_path", str(PATTERNS_DIR / "capture-triggers.md")),
        slack_signing_secret=scribe_data.get("slack_signing_secret", ""),
        notion_signing_secret=scribe_data.get("notion_signing_secret", ""),
    )


def _parse_retriever_config(data: dict) -> RetrieverConfig:
    """Parse retriever section from config dict (non-LLM fields only)"""
    retriever_data = data.get("retriever", {})
    return RetrieverConfig(
        topk=retriever_data.get("topk", 10),
        confidence_threshold=retriever_data.get("confidence_threshold", 0.5),
    )


def _parse_llm_config(data: dict) -> LLMConfig:
    """Parse LLM configuration with backward-compatible migration.

    Reads from ``data["llm"]`` first. If that section is absent, falls back
    to reading LLM-specific keys from ``data["retriever"]`` and
    ``data["scribe"]["tier2_provider"]`` for backward compatibility with
    configs written before the ``llm`` section existed.
    """
    llm_data = data.get("llm")

    if llm_data is not None:
        # New-style config: read directly from llm section
        return LLMConfig(
            provider=llm_data.get("provider", "anthropic"),
            tier2_provider=llm_data.get("tier2_provider", "anthropic"),
            anthropic_api_key=llm_data.get("anthropic_api_key", ""),
            anthropic_model=llm_data.get("anthropic_model", "claude-sonnet-4-20250514"),
            openai_api_key=llm_data.get("openai_api_key", ""),
            openai_model=llm_data.get("openai_model", "gpt-4o-mini"),
            openai_tier2_model=llm_data.get("openai_tier2_model", ""),
            google_api_key=llm_data.get("google_api_key", ""),
            google_model=llm_data.get("google_model", "gemini-2.0-flash-exp"),
            google_tier2_model=llm_data.get("google_tier2_model", ""),
        )

    # Migration: fall back to retriever + scribe fields
    retriever_data = data.get("retriever", {})
    scribe_data = data.get("scribe", {})

    return LLMConfig(
        provider=retriever_data.get("llm_provider", "anthropic"),
        tier2_provider=scribe_data.get("tier2_provider", "anthropic"),
        anthropic_api_key=retriever_data.get("anthropic_api_key", ""),
        anthropic_model=retriever_data.get("anthropic_model", "claude-sonnet-4-20250514"),
        openai_api_key=retriever_data.get("openai_api_key", ""),
        openai_model=retriever_data.get("openai_model", "gpt-4o-mini"),
        openai_tier2_model="",
        google_api_key=retriever_data.get("google_api_key", ""),
        google_model=retriever_data.get("google_model", "gemini-2.0-flash-exp"),
        google_tier2_model="",
    )


def load_config() -> RuneConfig:
    """
    Load configuration from file and environment variables.

    Priority (highest to lowest):
    1. Environment variables
    2. Config file (~/.rune/config.json)
    3. Default values
    """
    config = RuneConfig()

    # Load from config file if exists
    if CONFIG_PATH.exists():
        try:
            with open(CONFIG_PATH) as f:
                data = json.load(f)

            config.vault = _parse_vault_config(data)
            config.envector = _parse_envector_config(data)
            config.embedding = _parse_embedding_config(data)
            config.llm = _parse_llm_config(data)
            config.scribe = _parse_scribe_config(data)
            config.retriever = _parse_retriever_config(data)
            config.state = data.get("state", "dormant")
        except (json.JSONDecodeError, IOError) as e:
            print(f"[Config] Warning: Failed to load config file: {e}")

    # Environment variable overrides
    if os.getenv("RUNEVAULT_ENDPOINT"):
        config.vault.endpoint = os.getenv("RUNEVAULT_ENDPOINT")
    if os.getenv("RUNEVAULT_TOKEN"):
        config.vault.token = os.getenv("RUNEVAULT_TOKEN")

    if os.getenv("ENVECTOR_ENDPOINT"):
        config.envector.endpoint = os.getenv("ENVECTOR_ENDPOINT")
    if os.getenv("ENVECTOR_API_KEY"):
        config.envector.api_key = os.getenv("ENVECTOR_API_KEY")
    if os.getenv("EMBEDDING_MODE"):
        config.embedding.mode = os.getenv("EMBEDDING_MODE")
    if os.getenv("EMBEDDING_MODEL"):
        config.embedding.model = os.getenv("EMBEDDING_MODEL")

    if os.getenv("SCRIBE_PORT"):
        config.scribe.slack_webhook_port = int(os.getenv("SCRIBE_PORT"))
    if os.getenv("SCRIBE_THRESHOLD"):
        config.scribe.similarity_threshold = float(os.getenv("SCRIBE_THRESHOLD"))
    if os.getenv("SCRIBE_AUTO_THRESHOLD"):
        config.scribe.auto_capture_threshold = float(os.getenv("SCRIBE_AUTO_THRESHOLD"))
    if os.getenv("SLACK_SIGNING_SECRET"):
        config.scribe.slack_signing_secret = os.getenv("SLACK_SIGNING_SECRET")
    if os.getenv("NOTION_SIGNING_SECRET"):
        config.scribe.notion_signing_secret = os.getenv("NOTION_SIGNING_SECRET")

    # LLM env var overrides (target config.llm, track env-sourced keys)
    _env_llm_map = {
        "ANTHROPIC_API_KEY": "anthropic_api_key",
        "ANTHROPIC_MODEL": "anthropic_model",
        "OPENAI_API_KEY": "openai_api_key",
        "OPENAI_MODEL": "openai_model",
        "GOOGLE_API_KEY": "google_api_key",
        "GEMINI_API_KEY": "google_api_key",
        "GOOGLE_MODEL": "google_model",
        "RUNE_LLM_PROVIDER": "provider",
        "RUNE_TIER2_LLM_PROVIDER": "tier2_provider",
    }
    for env_var, attr in _env_llm_map.items():
        val = os.getenv(env_var)
        if val:
            setattr(config.llm, attr, val)
            config._env_sourced_keys.add(attr)

    if os.getenv("RUNE_STATE"):
        config.state = os.getenv("RUNE_STATE")

    return config


def save_config(config: RuneConfig) -> None:
    """Save configuration to file.

    API key fields that were sourced from environment variables are written
    as empty strings so that secrets are not persisted to disk.
    """
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)

    env_sourced = getattr(config, "_env_sourced_keys", set())

    # Build llm section, blanking out env-sourced API key fields
    _llm_api_key_fields = {
        "anthropic_api_key", "openai_api_key", "google_api_key",
    }
    llm_section = {
        "provider": config.llm.provider,
        "tier2_provider": config.llm.tier2_provider,
        "anthropic_api_key": config.llm.anthropic_api_key,
        "anthropic_model": config.llm.anthropic_model,
        "openai_api_key": config.llm.openai_api_key,
        "openai_model": config.llm.openai_model,
        "openai_tier2_model": config.llm.openai_tier2_model,
        "google_api_key": config.llm.google_api_key,
        "google_model": config.llm.google_model,
        "google_tier2_model": config.llm.google_tier2_model,
    }
    for key in _llm_api_key_fields:
        if key in env_sourced:
            llm_section[key] = ""

    data = {
        "vault": {
            "endpoint": config.vault.endpoint,
            "token": config.vault.token,
        },
        "envector": {
            "endpoint": config.envector.endpoint,
            "api_key": config.envector.api_key,
        },
        "embedding": {
            "mode": config.embedding.mode,
            "model": config.embedding.model,
        },
        "llm": llm_section,
        "scribe": {
            "slack_webhook_port": config.scribe.slack_webhook_port,
            "similarity_threshold": config.scribe.similarity_threshold,
            "auto_capture_threshold": config.scribe.auto_capture_threshold,
            "tier2_enabled": config.scribe.tier2_enabled,
            "tier2_model": config.scribe.tier2_model,
            "patterns_path": config.scribe.patterns_path,
            "slack_signing_secret": config.scribe.slack_signing_secret,
            "notion_signing_secret": config.scribe.notion_signing_secret,
        },
        "retriever": {
            "topk": config.retriever.topk,
            "confidence_threshold": config.retriever.confidence_threshold,
        },
        "state": config.state,
    }

    with open(CONFIG_PATH, "w") as f:
        json.dump(data, f, indent=2)

    # Set secure permissions
    CONFIG_PATH.chmod(0o600)


def ensure_directories() -> None:
    """Ensure required directories exist"""
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)
    LOGS_DIR.mkdir(parents=True, exist_ok=True)
    KEYS_DIR.mkdir(parents=True, exist_ok=True)
