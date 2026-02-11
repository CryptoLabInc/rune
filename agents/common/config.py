"""
Configuration Management for Rune Agents

Loads configuration from ~/.rune/config.json and environment variables.
"""

import os
import json
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional


# Default config paths
CONFIG_DIR = Path.home() / ".rune"
CONFIG_PATH = CONFIG_DIR / "config.json"
LOGS_DIR = CONFIG_DIR / "logs"
KEYS_DIR = CONFIG_DIR / "keys"
REVIEW_QUEUE_PATH = CONFIG_DIR / "review_queue.json"

# Project paths (relative to this file)
PROJECT_ROOT = Path(__file__).parent.parent.parent  # rune/
PATTERNS_DIR = PROJECT_ROOT / "patterns"
MCP_SERVER_DIR = PROJECT_ROOT / "mcp" / "envector-mcp-server" / "srcs"


@dataclass
class VaultConfig:
    """Rune-Vault configuration"""
    url: str = ""
    token: str = ""


@dataclass
class EnVectorConfig:
    """enVector Cloud configuration"""
    endpoint: str = "localhost:50050"
    api_key: str = ""
    collection: str = "rune-context"


@dataclass
class EmbeddingConfig:
    """Embedding model configuration"""
    mode: str = "femb"  # fastembed (on-device)
    model: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"


@dataclass
class ScribeConfig:
    """Scribe agent configuration"""
    slack_webhook_port: int = 8080
    similarity_threshold: float = 0.7
    auto_capture_threshold: float = 0.8
    patterns_path: str = str(PATTERNS_DIR / "capture-triggers.md")
    slack_signing_secret: str = ""


@dataclass
class RetrieverConfig:
    """Retriever agent configuration"""
    topk: int = 10
    confidence_threshold: float = 0.5
    anthropic_api_key: str = ""
    anthropic_model: str = "claude-sonnet-4-20250514"


@dataclass
class RuneConfig:
    """Main Rune configuration"""
    vault: VaultConfig = field(default_factory=VaultConfig)
    envector: EnVectorConfig = field(default_factory=EnVectorConfig)
    embedding: EmbeddingConfig = field(default_factory=EmbeddingConfig)
    scribe: ScribeConfig = field(default_factory=ScribeConfig)
    retriever: RetrieverConfig = field(default_factory=RetrieverConfig)
    state: str = "dormant"  # "active" or "dormant"


def _parse_vault_config(data: dict) -> VaultConfig:
    """Parse vault section from config dict"""
    vault_data = data.get("vault", {})
    return VaultConfig(
        url=vault_data.get("url", ""),
        token=vault_data.get("token", ""),
    )


def _parse_envector_config(data: dict) -> EnVectorConfig:
    """Parse envector section from config dict"""
    envector_data = data.get("envector", {})
    return EnVectorConfig(
        endpoint=envector_data.get("endpoint", "localhost:50050"),
        api_key=envector_data.get("api_key", ""),
        collection=envector_data.get("collection", "rune-context"),
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
        similarity_threshold=scribe_data.get("similarity_threshold", 0.7),
        auto_capture_threshold=scribe_data.get("auto_capture_threshold", 0.8),
        patterns_path=scribe_data.get("patterns_path", str(PATTERNS_DIR / "capture-triggers.md")),
        slack_signing_secret=scribe_data.get("slack_signing_secret", ""),
    )


def _parse_retriever_config(data: dict) -> RetrieverConfig:
    """Parse retriever section from config dict"""
    retriever_data = data.get("retriever", {})
    return RetrieverConfig(
        topk=retriever_data.get("topk", 10),
        confidence_threshold=retriever_data.get("confidence_threshold", 0.5),
        anthropic_api_key=retriever_data.get("anthropic_api_key", ""),
        anthropic_model=retriever_data.get("anthropic_model", "claude-sonnet-4-20250514"),
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
            config.scribe = _parse_scribe_config(data)
            config.retriever = _parse_retriever_config(data)
            config.state = data.get("state", "dormant")
        except (json.JSONDecodeError, IOError) as e:
            print(f"[Config] Warning: Failed to load config file: {e}")

    # Environment variable overrides
    if os.getenv("VAULT_URL"):
        config.vault.url = os.getenv("VAULT_URL")
    if os.getenv("VAULT_TOKEN"):
        config.vault.token = os.getenv("VAULT_TOKEN")

    if os.getenv("ENVECTOR_ENDPOINT"):
        config.envector.endpoint = os.getenv("ENVECTOR_ENDPOINT")
    if os.getenv("ENVECTOR_API_KEY"):
        config.envector.api_key = os.getenv("ENVECTOR_API_KEY")
    if os.getenv("ENVECTOR_COLLECTION"):
        config.envector.collection = os.getenv("ENVECTOR_COLLECTION")

    if os.getenv("EMBEDDING_MODE"):
        config.embedding.mode = os.getenv("EMBEDDING_MODE")
    if os.getenv("EMBEDDING_MODEL"):
        config.embedding.model = os.getenv("EMBEDDING_MODEL")

    if os.getenv("SCRIBE_PORT"):
        config.scribe.slack_webhook_port = int(os.getenv("SCRIBE_PORT"))
    if os.getenv("SCRIBE_THRESHOLD"):
        config.scribe.similarity_threshold = float(os.getenv("SCRIBE_THRESHOLD"))
    if os.getenv("SLACK_SIGNING_SECRET"):
        config.scribe.slack_signing_secret = os.getenv("SLACK_SIGNING_SECRET")

    if os.getenv("ANTHROPIC_API_KEY"):
        config.retriever.anthropic_api_key = os.getenv("ANTHROPIC_API_KEY")
    if os.getenv("ANTHROPIC_MODEL"):
        config.retriever.anthropic_model = os.getenv("ANTHROPIC_MODEL")

    if os.getenv("RUNE_STATE"):
        config.state = os.getenv("RUNE_STATE")

    return config


def save_config(config: RuneConfig) -> None:
    """Save configuration to file"""
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)

    data = {
        "vault": {
            "url": config.vault.url,
            "token": config.vault.token,
        },
        "envector": {
            "endpoint": config.envector.endpoint,
            "api_key": config.envector.api_key,
            "collection": config.envector.collection,
        },
        "embedding": {
            "mode": config.embedding.mode,
            "model": config.embedding.model,
        },
        "scribe": {
            "slack_webhook_port": config.scribe.slack_webhook_port,
            "similarity_threshold": config.scribe.similarity_threshold,
            "auto_capture_threshold": config.scribe.auto_capture_threshold,
            "patterns_path": config.scribe.patterns_path,
            "slack_signing_secret": config.scribe.slack_signing_secret,
        },
        "retriever": {
            "topk": config.retriever.topk,
            "confidence_threshold": config.retriever.confidence_threshold,
            "anthropic_api_key": config.retriever.anthropic_api_key,
            "anthropic_model": config.retriever.anthropic_model,
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
