"""
enVector MCP Server for Rune plugin.

Transport: stdio only (launched by Claude Code plugin system).

Expected MCP Tool Return Format:
{
    "ok": bool,
    "results": Any,          # Present if ok is True
    "error": str            # Present if ok is False
}
"""

import argparse
import logging
from typing import Union, List, Dict, Any, Optional, Annotated
from datetime import datetime, timezone
import numpy as np
import os, sys, signal
import json

logger = logging.getLogger("rune.mcp")


class _SensitiveFilter(logging.Filter):
    """Sanitize potential secrets from log messages."""
    import re
    _PATTERNS = [
        re.compile(r'(sk-|pk-|api_|envector_|evt_)[a-zA-Z0-9_-]{10,}'),
        re.compile(r'(token|key|secret|password)["\s:=]+[a-zA-Z0-9_-]{20,}', re.IGNORECASE),
    ]

    def filter(self, record):
        import re
        msg = record.getMessage()
        for pat in self._PATTERNS:
            msg = pat.sub(lambda m: m.group()[:8] + '***', msg)
        record.msg = msg
        record.args = ()
        return True


logger.addFilter(_SensitiveFilter())
from pydantic import Field

# Add parent directory (rune/mcp/) to sys.path so `from adapter import ...` works
CURRENT_DIR = os.path.dirname(os.path.abspath(__file__))
MCP_ROOT = os.path.dirname(CURRENT_DIR)
PLUGIN_ROOT = os.path.dirname(MCP_ROOT)  # rune/ root for `from agents import ...`
# Re-insert paths to take precedence over the script dir
for _p in (MCP_ROOT, PLUGIN_ROOT):
    try:
        sys.path.remove(_p)
    except ValueError:
        pass
sys.path[0:0] = [PLUGIN_ROOT, MCP_ROOT]
del _p

from fastmcp import FastMCP, Context  # pip install fastmcp
from mcp.types import ToolAnnotations
from adapter import EnVectorSDKAdapter
from adapter.vault_client import VaultClient, VaultError
from server.errors import (
    RuneError, VaultConnectionError, VaultDecryptionError,
    EnvectorConnectionError, EnvectorInsertError,
    PipelineNotReadyError, InvalidInputError, make_error,
)


def _detection_from_agent_data(
    domain: str = "general",
    confidence: float = 0.0,
    category: str = "",
) -> "DetectionResult":
    """Build DetectionResult from agent-provided metadata.

    In agent-delegated mode the calling agent has already evaluated
    significance.  We construct a minimal DetectionResult so that
    RecordBuilder can consume it without running the pattern detector.
    """
    from agents.scribe.detector import DetectionResult
    return DetectionResult(
        is_significant=True,  # Agent said capture=true
        confidence=confidence,
        domain=domain,
        category=category or domain,
    )


def _embedding_text_for_record(record) -> str:
    """Select the text to embed in enVector.

    Schema 2.1+: use reusable_insight (dense NL gist).
    Schema 2.0 fallback: use payload.text (verbose markdown).
    """
    from agents.common.schemas.embedding import embedding_text_for_record
    return embedding_text_for_record(record)


def _classify_novelty(
    max_similarity: float,
    threshold_novel: float = 0.3,
    threshold_related: float = 0.7,
    threshold_near_duplicate: float = 0.95,
) -> dict:
    """Classify capture novelty based on similarity to existing memory."""
    from agents.common.schemas.embedding import classify_novelty
    return classify_novelty(max_similarity, threshold_novel, threshold_related, threshold_near_duplicate)


# ---------- Capture Log ---------- #
CAPTURE_LOG_PATH = os.path.join(os.path.expanduser("~"), ".rune", "capture_log.jsonl")


def _append_capture_log(
    record_id: str, title: str, domain: str, mode: str,
    action: str = "captured", novelty_class: str = "", novelty_score: float = 0.0,
):
    """Append a capture event to the local JSONL log (atomic, secure permissions)."""
    try:
        entry_dict = {
            "ts": datetime.now(timezone.utc).isoformat(),
            "action": action,
            "id": record_id,
            "title": title,
            "domain": domain,
            "mode": mode,
        }
        if novelty_class:
            entry_dict["novelty_class"] = novelty_class
            entry_dict["novelty_score"] = novelty_score
        entry = json.dumps(entry_dict, ensure_ascii=False)
        fd = os.open(CAPTURE_LOG_PATH, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        with os.fdopen(fd, "a") as f:
            f.write(entry + "\n")
    except Exception as e:
        logger.debug("Capture log write failed: %s", e)


def _read_capture_log(limit: int = 20, domain: str = None, since: str = None) -> list:
    """Read capture log entries in reverse chronological order."""
    if not os.path.exists(CAPTURE_LOG_PATH):
        return []
    try:
        with open(CAPTURE_LOG_PATH, "r") as f:
            lines = f.readlines()
    except Exception:
        return []

    entries = []
    for line in reversed(lines):
        line = line.strip()
        if not line:
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if domain and entry.get("domain") != domain:
            continue
        if since:
            entry_ts = entry.get("ts", "")
            if entry_ts < since:
                continue
        entries.append(entry)
        if len(entries) >= limit:
            break
    return entries


def _set_dormant_with_reason(reason: str):
    """Update config.json to dormant state with a reason and timestamp"""
    config_path = os.path.join(os.path.expanduser("~"), ".rune", "config.json")
    try:
        if not os.path.exists(config_path):
            return
        with open(config_path) as f:
            data = json.load(f)
        if data.get("state") == "dormant" and data.get("dormant_reason") == reason:
            return  # already set to this reason — no change needed
        data["state"] = "dormant"
        data["dormant_reason"] = reason
        data["dormant_since"] = datetime.now(timezone.utc).isoformat()
        fd = os.open(config_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as f:
            json.dump(data, f, indent=2)
        logger.warning("Switched to dormant state: %s", reason)
    except Exception as e:
        logger.debug("Failed to update config dormant state: %s", e)


async def _async_fetch_keys_from_vault(
    vault_endpoint: str,
    vault_token: str,
    key_base_path: str,
    ca_cert: str = None,
    tls_disable: bool = False,
) -> tuple:
    """
    Async core: fetches public keys (EncKey, EvalKey) and per-agent metadata
    DEK from Rune-Vault via gRPC.

    The Vault bundle includes key_id, index_name, agent_id, and agent_dek
    so the client discovers them dynamically.

    Args:
        vault_endpoint: Rune-Vault gRPC endpoint
        vault_token: Authentication token
        key_base_path: Root key directory (e.g. ~/.rune/keys).
            Keys are saved under key_base_path/<key_id>/.
        ca_cert: Path to CA certificate PEM for self-signed certs.
        tls_disable: If True, use insecure plaintext channel.

    Returns:
        tuple: (success, index_name, key_id, agent_id, agent_dek_bytes, envector_endpoint, envector_api_key)
    """
    client = VaultClient(
        vault_endpoint=vault_endpoint,
        vault_token=vault_token,
        ca_cert=ca_cert,
        tls_disable=tls_disable,
    )
    try:
        bundle = await client.get_public_key()

        # Extract metadata before saving key files
        vault_index_name = bundle.pop("index_name", None)
        vault_key_id = bundle.pop("key_id", None)
        vault_agent_id = bundle.pop("agent_id", None)
        vault_agent_dek_b64 = bundle.pop("agent_dek", None)
        vault_envector_endpoint = bundle.pop("envector_endpoint", None)
        vault_envector_api_key = bundle.pop("envector_api_key", None)

        if vault_index_name:
            logger.info(f"Vault provided index_name: {vault_index_name}")
        if vault_key_id:
            logger.info(f"Vault provided key_id: {vault_key_id}")
        else:
            logger.warning("Vault did not provide key_id — key directory cannot be determined")
            return False, vault_index_name, None, None, None, None, None
        if vault_agent_id:
            logger.info(f"Vault provided agent_id: {vault_agent_id}")

        # Decode agent DEK from base64
        agent_dek_bytes = None
        if vault_agent_dek_b64:
            import base64
            try:
                agent_dek_bytes = base64.b64decode(vault_agent_dek_b64)
            except (base64.binascii.Error, ValueError) as e:
                logger.error(f"Failed to decode agent_dek from Vault (invalid base64): {e}")
                return False, vault_index_name, vault_key_id, vault_agent_id, None, None, None
            if len(agent_dek_bytes) != 32:
                logger.error(f"agent_dek has invalid length {len(agent_dek_bytes)} bytes (expected 32 for AES-256)")
                return False, vault_index_name, vault_key_id, vault_agent_id, None, None, None

        # Save keys under key_base_path/<key_id>/ with restrictive permissions
        key_dir = os.path.join(key_base_path, vault_key_id)
        os.makedirs(key_dir, mode=0o700, exist_ok=True)

        for filename, key_content in bundle.items():
            filepath = os.path.join(key_dir, filename)
            fd = os.open(filepath, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, 'w') as f:
                f.write(key_content)
            logger.info(f"Saved {filename} to {filepath}")

        return True, vault_index_name, vault_key_id, vault_agent_id, agent_dek_bytes, vault_envector_endpoint, vault_envector_api_key

    except Exception as e:
        logger.error(f"Failed to fetch keys from Vault: {e}")
        return False, None, None, None, None, None, None
    finally:
        await client.close()


def fetch_keys_from_vault(
    vault_endpoint: str,
    vault_token: str,
    key_base_path: str,
    ca_cert: str = None,
    tls_disable: bool = False,
) -> tuple:
    """
    Fetches public keys from Rune-Vault. Safe to call from both sync (main)
    and async (reload_pipelines) contexts.

    Args:
        vault_endpoint: Rune-Vault endpoint URL
        vault_token: Authentication token for Vault
        key_base_path: Root key directory (e.g. ~/.rune/keys)
        ca_cert: Path to CA certificate PEM for self-signed certs.
        tls_disable: If True, use insecure plaintext channel.

    Returns:
        tuple: (success, index_name, key_id, agent_id, agent_dek_bytes, envector_endpoint, envector_api_key)
    """
    import asyncio
    _fail = (False, None, None, None, None, None, None)

    try:
        asyncio.get_running_loop()
        # Already inside an event loop (e.g. FastMCP startup) —
        # run the async fetch in a separate thread with its own loop.
        import concurrent.futures
        with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
            future = pool.submit(
                asyncio.run,
                _async_fetch_keys_from_vault(vault_endpoint, vault_token, key_base_path, ca_cert, tls_disable),
            )
            try:
                return future.result(timeout=30)
            except concurrent.futures.TimeoutError:
                logger.error("Vault key fetch timed out after 30 seconds")
                return _fail
            except Exception as e:
                logger.error(f"Vault key fetch failed in thread: {e}")
                return _fail
    except RuntimeError:
        # No running event loop — safe to use asyncio.run() directly.
        try:
            return asyncio.run(
                _async_fetch_keys_from_vault(vault_endpoint, vault_token, key_base_path, ca_cert, tls_disable)
            )
        except Exception as e:
            logger.error(f"Vault key fetch failed: {e}")
            return _fail

class MCPServerApp:
    """
    Main application class for the MCP server.

    Security Model (with Rune-Vault):
    - MCP Server handles embeddings, query encryption, and orchestration
    - Rune-Vault holds secret key and performs all decryption
    - Agent never has access to secret key
    """
    # Canonical key path (key_id is discovered from Vault at runtime)
    DEFAULT_KEY_PATH = os.path.expanduser("~/.rune/keys")

    def __init__(
            self,
            envector_adapter: Optional[EnVectorSDKAdapter] = None,
            mcp_server_name: str = "envector_mcp_server",
            vault_client: Optional[VaultClient] = None,
            vault_index_name: Optional[str] = None,
            key_path: Optional[str] = None,
            key_id: Optional[str] = None,
            agent_id: Optional[str] = None,
            agent_dek: Optional[bytes] = None,
            scribe_pipeline: Optional[Dict[str, Any]] = None,
            retriever_pipeline: Optional[Dict[str, Any]] = None,
        ) -> None:
        """
        Initializes the MCPServerApp with the given adapter and server name.
        Args:
            envector_adapter (EnVectorSDKAdapter): The enVector SDK adapter instance.
            mcp_server_name (str): The name of the MCP server.
            vault_client (VaultClient): Optional Vault client for secure decryption.
                Embedding is initialized from config.json via _init_pipelines (no CLI override).
            vault_index_name (str): Team index name provisioned by Vault admin (optional).
            key_path (str): Root directory for encryption keys.
            key_id (str): Key identifier (subdirectory under key_path).
                Discovered from Vault at runtime; no hardcoded default.
            agent_id (str): Per-agent identifier for metadata encryption (from Vault).
            agent_dek (bytes): Per-agent AES-256 DEK for app-layer metadata encryption.
            scribe_pipeline (dict): Pre-initialized scribe pipeline components.
            retriever_pipeline (dict): Pre-initialized retriever pipeline components.
        """
        # adapters
        self.envector = envector_adapter
        self.embedding = None  # set by _init_pipelines from config
        self.vault = vault_client
        self._vault_index_name = vault_index_name
        self._key_path = key_path or self.DEFAULT_KEY_PATH
        self._key_id = key_id  # Vault-provided, no hardcoded fallback
        self._agent_id = agent_id
        self._agent_dek = agent_dek
        self._scribe = scribe_pipeline
        self._retriever = retriever_pipeline
        self._envector_endpoint: Optional[str] = None
        self._envector_api_key: Optional[str] = None
        self._client_provider_override: Optional[str] = None
        self._active_llm_provider: Optional[str] = None
        self._active_tier2_provider: Optional[str] = None
        # mcp
        self.mcp = FastMCP(name=mcp_server_name)

        # ---------- Confidence Calculation (inlined from Synthesizer) ---------- #
        def _calculate_confidence(results) -> float:
            """Calculate overall confidence from search results (pure math, no LLM)."""
            if not results:
                return 0.0
            certainty_weights = {
                "supported": 1.0,
                "partially_supported": 0.6,
                "unknown": 0.3,
            }
            total_weight = 0.0
            total_score = 0.0
            for i, r in enumerate(results[:5]):
                position_weight = 1.0 / (i + 1)
                cert_weight = certainty_weights.get(r.certainty, 0.3)
                weight = position_weight * cert_weight * r.score
                total_weight += weight
                total_score += weight
            if total_weight == 0:
                return 0.0
            return round(min(1.0, total_score / 2.0), 2)

        # ---------- Common Query Preprocessing ---------- #
        def _preprocess(raw_query: Any) -> Union[List[float], List[List[float]]]:
            """Convert raw query input (string, ndarray, list) into a valid vector or batch of vectors."""
            if isinstance(raw_query, str):
                raw_query = raw_query.strip()

                if self.embedding is not None:
                    return self.embedding.embed([raw_query])[0]

                if not raw_query:
                    raise ValueError("`query` string is empty. Provide a JSON array of floats or precomputed embedding.")
                try:
                    raw_query = json.loads(raw_query)
                except json.JSONDecodeError as exc:
                    raise ValueError(
                        "Plain text is not supported for `query`. Convert the text into an embedding vector "
                        "and pass it as a JSON array (e.g., [[0.1, 0.2], ...])."
                    ) from exc

            if isinstance(raw_query, np.ndarray):
                raw_query = raw_query.tolist()
            elif isinstance(raw_query, list) and all(isinstance(q, np.ndarray) for q in raw_query):
                raw_query = [q.tolist() for q in raw_query]

            def _is_vector(value: Any) -> bool:
                return isinstance(value, list) and all(isinstance(v, (int, float)) for v in value)

            if _is_vector(raw_query):
                return raw_query
            if isinstance(raw_query, list) and all(_is_vector(item) for item in raw_query):
                return raw_query

            raise ValueError(
                "`query` must be a list of floats or a list of float lists. "
                f"Received type: {type(raw_query).__name__}"
            )

        def _infer_provider_from_context(ctx: Optional[Context]) -> Optional[str]:
            """
            Infer LLM provider from MCP initialize clientInfo.name.
            This is best-effort and only used when config provider is set to "auto".
            """
            if ctx is None or ctx.request_context is None:
                return None

            try:
                session = getattr(ctx.request_context, "session", None)
                params = getattr(session, "client_params", None)
                client_info = getattr(params, "clientInfo", None) or getattr(params, "client_info", None)
                client_name = (getattr(client_info, "name", "") or "").lower()
            except Exception:
                return None

            if not client_name:
                return None
            if any(token in client_name for token in ("claude", "anthropic")):
                return "anthropic"
            if any(token in client_name for token in ("openai", "codex", "chatgpt")):
                return "openai"
            if any(token in client_name for token in ("gemini", "google", "antigravity", "openclaw")):
                return "google"
            return None

        def _maybe_reload_for_auto_provider(ctx: Optional[Context]) -> None:
            inferred = _infer_provider_from_context(ctx)
            if not inferred:
                return
            if inferred == self._client_provider_override:
                return

            self._client_provider_override = inferred
            logger.info("Auto provider inferred from MCP clientInfo: %s", inferred)
            refresh = self._init_pipelines()
            if refresh.get("errors"):
                logger.warning("Auto provider reload had warnings: %s", refresh["errors"])

        # ---------- MCP Tools: Vault Health Check ---------- #
        @self.mcp.tool(
            name="vault_status",
            description="Check Rune-Vault connection status and security mode.",
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_vault_status() -> Dict[str, Any]:
            """
            Returns the current Vault integration status.

            Returns:
                Dict with Vault connection status and security mode information.
            """
            if self.vault is None:
                return {
                    "ok": True,
                    "vault_configured": False,
                    "secure_search_available": False,
                    "mode": "standard (no Vault)",
                    "team_index_name": self._vault_index_name,
                    "warning": "secret key may be accessible locally. Configure Vault for secure mode."
                }

            # Check Vault health via /health endpoint
            try:
                vault_healthy = await self.vault.health_check()
                return {
                    "ok": True,
                    "vault_configured": True,
                    "vault_endpoint": getattr(self.vault, 'vault_endpoint', 'unknown'),
                    "secure_search_available": vault_healthy,
                    "mode": "secure (Vault-backed)",
                    "vault_healthy": vault_healthy,
                    "team_index_name": self._vault_index_name,
                }
            except Exception as e:
                err = make_error(VaultConnectionError(f"Vault health check failed: {e}"))
                err["vault_configured"] = True
                return err

        # ---------- MCP Tools: Diagnostics ---------- #
        @self.mcp.tool(
            name="diagnostics",
            description=(
                "System health check tool for the Rune. "
                "Reports status of Vault connection, encryption keys, "
                "pipeline initialization, and enVector cloud reachability."
            ),
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_diagnostics() -> Dict[str, Any]:
            """
            Returns diagnostic reports about Rune subsystems"

            Returns:
                Dict with subsystem health information
            """
            import time
            import sys

            report: Dict[str, Any] = {"ok": True}

            # Environment Info
            try:
                report["environment"] = {
                    "os": sys.platform,
                    "python_version": sys.version.split(" ")[0],
                    "cwd": os.getcwd(),
                }
            except Exception as e:
                report["environment"] = {"error": str(e)}

            # Dormant state info
            config_path = os.path.join(os.path.expanduser("~"), ".rune", "config.json")
            if os.path.exists(config_path):
                try:
                    with open(config_path) as _cf:
                        _cfg_data = json.load(_cf)
                    report["state"] = _cfg_data.get("state", "unknown")
                    if _cfg_data.get("dormant_reason"):
                        report["dormant_reason"] = _cfg_data["dormant_reason"]
                    if _cfg_data.get("dormant_since"):
                        report["dormant_since"] = _cfg_data["dormant_since"]
                except Exception:
                    pass

            # Vault connection
            vault_info: Dict[str, Any] = {
                "configured": self.vault is not None,
                "healthy": False,
                "endpoint": None,
            }

            if self.vault is not None:
                vault_info["endpoint"] = getattr(self.vault, "vault_endpoint", "unknown")
                try:
                    vault_info["healthy"] = await self.vault.health_check()
                except Exception as e:
                    vault_info["healthy"] = False
                    vault_info["error"] = str(e)
            report["vault"] = vault_info

            # Encryption Keys
            key_id = self._key_id
            enc_key_loaded = False
            if key_id and self._key_path:
                enc_key_file = os.path.join(self._key_path, key_id, "EncKey.json")
                enc_key_loaded = os.path.exists(enc_key_file)

            keys_info: Dict[str, Any] = {
                "enc_key_loaded": enc_key_loaded,
                "key_id": key_id,
                "agent_dek_loaded": self._agent_dek is not None,
            }
            report["keys"] = keys_info

            # Pipelines
            pipelines_info: Dict[str, Any] = {
                "scribe": self._scribe is not None,
                "retriever": self._retriever is not None,
                "llm_provider": self._active_llm_provider,
            }
            report["pipelines"] = pipelines_info

            # Embedding model
            embedding_info: Dict[str, Any] = {
                "model": None,
                "mode": None,
            }
            if self._scribe and self._scribe.get("embedding_service"):
                svc = self._scribe["embedding_service"]
                embedding_info["model"] = getattr(svc, "_model", "unknown")
                embedding_info["mode"] = getattr(svc, "_mode", "unknown")
            report["embedding"] = embedding_info

            # enVector Cloud
            envector_info: Dict[str, Any] = {
                "reachable": False,
                "latency_ms": None,
            }

            if self.envector is not None:
                import concurrent.futures as _cf
                ENVECTOR_DIAGNOSIS_TIMEOUT = 5.0  # seconds
                _pool = _cf.ThreadPoolExecutor(max_workers=1)
                try:
                    t0 = time.monotonic()
                    _future = _pool.submit(self.envector.invoke_get_index_list)
                    try:
                        _future.result(timeout=ENVECTOR_DIAGNOSIS_TIMEOUT)
                        latency = (time.monotonic() - t0) * 1000
                        envector_info["reachable"] = True
                        envector_info["latency_ms"] = round(latency, 1)
                    except _cf.TimeoutError:
                        elapsed = round((time.monotonic() - t0) * 1000, 1)
                        envector_info["error"] = (
                            f"Health check timed out after {ENVECTOR_DIAGNOSIS_TIMEOUT:.0f}s "
                            f"(elapsed: {elapsed}ms). "
                            "Run /rune:activate to pre-warm the connection, then retry /rune:status."
                        )
                        envector_info["error_type"] = "timeout"
                        envector_info["elapsed_ms"] = elapsed
                except Exception as e:
                    err_str = str(e)
                    # Classify errors for more hints to users
                    if "UNAVAILABLE" in err_str or "Connection refused" in err_str:
                        error_type = "connection_refused"
                        hint = "Check that the enVector endpoint is correct and reachable from this machine"
                    elif "UNAUTHENTICATED" in err_str or "401" in err_str:
                        error_type = "auth_failure"
                        hint = "enVector API key may be invalid or expired"
                    elif "DEADLINE_EXCEEDED" in err_str:
                        error_type = "deadline_exceeded"
                        hint = (
                            "The enVector gRPC deadline was exceeded. "
                            "Run /rune:activate to pre-warm, then retry /rune:status"
                        )
                    else:
                        error_type = "unknown"
                        hint = "Run /rune:activate to reinitialize the connection, or check network connectivity"
                    envector_info["error"] = err_str
                    envector_info["error_type"] = error_type
                    envector_info["hint"] = hint
                finally:
                    # Return immediately without waiting on timeout
                    _pool.shutdown(wait=False)
            report["envector"] = envector_info

            # Result
            if self.vault is not None and not vault_info["healthy"]:
                report["ok"] = False
            if not enc_key_loaded:
                report["ok"] = False

            return report

        # ---------- MCP Tools: Capture (Scribe Pipeline) ---------- #
        @self.mcp.tool(
            name="capture",
            description=(
                "Capture a significant organizational decision into FHE-encrypted team memory. "
                "PRIMARY: Agent-delegated mode — pass `extracted` JSON with the agent's own "
                "evaluation and extraction. The MCP server stores it without additional LLM calls. "
                "LEGACY: If `extracted` is omitted and API keys are configured, falls back to "
                "a 3-tier server-side pipeline (pattern detection → LLM filter → LLM extraction)."
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_capture(
            text: Annotated[str, Field(description="The text containing a potential decision or significant context to capture")],
            source: Annotated[str, Field(description="Source of the text (e.g., 'claude_agent', 'slack', 'github')")] = "claude_agent",
            user: Annotated[Optional[str], Field(description="User who authored the text")] = None,
            channel: Annotated[Optional[str], Field(description="Channel or location where the text originated")] = None,
            extracted: Annotated[Optional[str], Field(description="Pre-extracted JSON from calling agent (agent-delegated mode). When provided, Tier 2/3 are skipped.")] = None,
            ctx: Optional[Context] = None,
        ) -> Dict[str, Any]:
            _maybe_reload_for_auto_provider(ctx)

            if self._scribe is None:
                return make_error(PipelineNotReadyError(
                    "Scribe pipeline not initialized.",
                    recovery_hint="Run /rune:activate to reinitialize pipelines, or restart Claude Code if the problem persists.",
                ))

            if not self._vault_index_name:
                return make_error(PipelineNotReadyError(
                    "No index name available. Vault must provide a team index name."
                ))

            try:
                from datetime import datetime, timezone
                from agents.scribe.record_builder import RawEvent
                from agents.scribe.llm_extractor import (
                    ExtractionResult, ExtractedFields, PhaseExtractedFields,
                )
                from agents.common.llm_utils import parse_llm_json

                record_builder = self._scribe["record_builder"]
                envector_client = self._scribe["envector_client"]
                embedding_service = self._scribe["embedding_service"]
                detector = self._scribe.get("detector")
                tier2_filter = self._scribe.get("tier2_filter")

                # ===== PRIMARY: Agent-delegated mode =====
                # The calling agent (Claude/Gemini/Codex) has already evaluated and
                # extracted the decision.  We just validate, build records, and store.
                if extracted is not None:
                    return await self._capture_single(
                        text=text,
                        source=source,
                        user=user,
                        channel=channel,
                        extracted=extracted,
                    )

                # ===== FALLBACK: Legacy 3-tier pipeline (requires API keys) =====
                # Retained for backward compatibility.  New integrations should use
                # agent-delegated mode above.
                if detector is None:
                    return {
                        "ok": True,
                        "captured": False,
                        "reason": "No `extracted` JSON provided and legacy pipeline not available "
                                  "(no API keys configured). Use agent-delegated mode by passing "
                                  "the `extracted` parameter.",
                    }
                raw_event = RawEvent(
                    text=text,
                    user=user or "unknown",
                    channel=channel or "claude_session",
                    timestamp=str(datetime.now(timezone.utc).timestamp()),
                    source=source,
                )
                return await self._legacy_standard_capture(
                    text=text,
                    raw_event=raw_event,
                    detector=detector,
                    tier2_filter=tier2_filter,
                    record_builder=record_builder,
                    envector_client=envector_client,
                    embedding_service=embedding_service,
                )

            except VaultError as e:
                logger.error(f"Capture failed (Vault): {e}", exc_info=True)
                _set_dormant_with_reason("vault_unreachable")
                return make_error(VaultConnectionError(
                    str(e),
                    recovery_hint=(
                        "Vault error during capture. Check: "
                        "(1) Is the Vault server running? "
                        "(2) Is your token valid? "
                        "Run /rune:status for diagnostics."
                    ),
                ))
            except (ConnectionError, OSError) as e:
                logger.error(f"Capture failed (network): {e}", exc_info=True)
                _set_dormant_with_reason("envector_unreachable")
                return make_error(EnvectorConnectionError(
                    str(e),
                    recovery_hint=(
                        "Network error during capture. Check: "
                        "(1) Is the enVector endpoint reachable? "
                        "(2) Is your API key valid? "
                        "Run /rune:status for diagnostics."
                    ),
                ))
            except ValueError as e:
                logger.error(f"Capture failed (input): {e}", exc_info=True)
                return make_error(InvalidInputError(str(e)))
            except Exception as e:
                logger.error(f"Capture failed: {e}", exc_info=True)
                return make_error(e)

        # ---------- MCP Tools: Batch Capture (Session-End Sweep) ---------- #
        @self.mcp.tool(
            name="batch_capture",
            description=(
                "Batch-capture multiple decisions at once (session-end sweep). "
                "Each item uses the same format as the `capture` tool's `extracted` parameter. "
                "Items are processed independently — one failure does not abort others. "
                "Novelty check runs per item; near-duplicates are skipped."
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_batch_capture(
            items: Annotated[str, Field(description="JSON array of extracted decision objects (same format as capture's extracted parameter)")],
            source: Annotated[str, Field(description="Source (e.g., 'claude_agent')")] = "claude_agent",
            user: Annotated[Optional[str], Field(description="User who authored the decisions")] = None,
            channel: Annotated[Optional[str], Field(description="Channel or context")] = None,
            ctx: Optional[Context] = None,
        ) -> Dict[str, Any]:
            _maybe_reload_for_auto_provider(ctx)

            if self._scribe is None:
                return make_error(PipelineNotReadyError("Scribe pipeline not initialized."))
            if not self._vault_index_name:
                return make_error(PipelineNotReadyError("No index name available."))

            try:
                items_list = json.loads(items)
            except json.JSONDecodeError as e:
                return {"ok": False, "error": f"Invalid JSON: {e}"}

            if not isinstance(items_list, list):
                return {"ok": False, "error": "items must be a JSON array"}

            if len(items_list) == 0:
                return {"ok": True, "total": 0, "results": [], "captured": 0, "skipped": 0, "errors": 0}

            results = []
            for i, item in enumerate(items_list):
                title = ""
                try:
                    title = item.get("title", "") if isinstance(item, dict) else ""
                    item_text = item.get("reusable_insight") or item.get("title") or "[batch_capture]" if isinstance(item, dict) else "[batch_capture]"
                    result = await self._capture_single(
                        text=item_text,
                        source=source,
                        user=user,
                        channel=channel,
                        extracted=json.dumps(item),
                    )
                    if result.get("captured"):
                        status = "captured"
                        novelty_class = result.get("novelty", {}).get("class", "novel")
                    elif result.get("novelty", {}).get("class") == "near_duplicate":
                        status = "near_duplicate"
                        novelty_class = "near_duplicate"
                    else:
                        status = "skipped"
                        novelty_class = result.get("novelty", {}).get("class", "")
                    results.append({
                        "index": i,
                        "title": title,
                        "status": status,
                        "novelty": novelty_class,
                    })
                except Exception as e:
                    logger.warning("batch_capture item %d failed: %s", i, e)
                    results.append({
                        "index": i,
                        "title": title,
                        "status": "error",
                        "error": str(e),
                    })

            captured = sum(1 for r in results if r["status"] == "captured")
            skipped = sum(1 for r in results if r["status"] in ("skipped", "near_duplicate"))
            errors = sum(1 for r in results if r["status"] == "error")

            return {
                "ok": True,
                "total": len(results),
                "results": results,
                "captured": captured,
                "skipped": skipped,
                "errors": errors,
            }

        # ---------- MCP Tools: Recall (Retriever Pipeline) ---------- #
        @self.mcp.tool(
            name="recall",
            description=(
                "Search and synthesize answers from FHE-encrypted team memory via Vault-secured pipeline. "
                "Pipeline: (1) query expansion and intent detection, "
                "(2) encrypted similarity scoring on enVector Cloud, "
                "(3) Rune-Vault decrypts result ciphertext (secret key never leaves Vault). "
                "Use for questions about past decisions, trade-offs, and organizational knowledge."
            ),
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_recall(
            query: Annotated[str, Field(description="Natural language question about past decisions or organizational context")],
            topk: Annotated[int, Field(description="Number of results to consider for synthesis")] = 5,
            domain: Annotated[Optional[str], Field(description="Filter by domain (e.g. 'architecture', 'security')")] = None,
            status: Annotated[Optional[str], Field(description="Filter by status (e.g. 'accepted', 'proposed')")] = None,
            since: Annotated[Optional[str], Field(description="Filter records after this ISO date (e.g. '2026-01-01')")] = None,
            ctx: Optional[Context] = None,
        ) -> Dict[str, Any]:
            _maybe_reload_for_auto_provider(ctx)

            if self._retriever is None:
                return make_error(PipelineNotReadyError(
                    "Retriever pipeline not initialized.",
                    recovery_hint="Run /rune:activate to reinitialize pipelines, or restart Claude Code if the problem persists.",
                ))

            if topk > 10:
                return make_error(InvalidInputError("topk must be 10 or less."))

            try:
                query_processor = self._retriever["query_processor"]
                searcher = self._retriever["searcher"]
                synthesizer = self._retriever.get("synthesizer")

                # Step 1: Parse query (intent detection, entity extraction, query expansion)
                parsed_query = query_processor.parse(query)

                # Step 2: Search enVector (over-fetch, post-filter, recency weighting)
                filters = {}
                if domain:
                    filters["domain"] = domain
                if status:
                    filters["status"] = status
                if since:
                    filters["since"] = since
                results = await searcher.search(parsed_query, topk=topk, filters=filters or None)

                # Step 3: Return results (agent synthesizes) or use server-side synthesizer
                # Primary path: raw results for agent-side synthesis (no LLM key needed)
                if synthesizer is None or not synthesizer.has_llm:
                    confidence = _calculate_confidence(results)
                    formatted_results = []
                    for r in results:
                        entry = {
                            "record_id": r.record_id,
                            "title": r.title,
                            "content": r.payload_text,
                            "domain": r.domain,
                            "certainty": r.certainty,
                            "score": r.score,
                        }
                        if r.group_id:
                            entry["group_id"] = r.group_id
                            entry["group_type"] = r.group_type
                            entry["phase_seq"] = r.phase_seq
                            entry["phase_total"] = r.phase_total
                        formatted_results.append(entry)

                    sources = [
                        {
                            "record_id": r.record_id,
                            "title": r.title,
                            "domain": r.domain,
                            "certainty": r.certainty,
                            "score": r.score,
                        }
                        for r in results[:5]
                    ]

                    return {
                        "ok": True,
                        "found": len(results),
                        "results": formatted_results,
                        "confidence": confidence,
                        "sources": sources,
                        "synthesized": False,
                    }

                # Fallback: server-side synthesis when LLM key is available
                answer = synthesizer.synthesize(parsed_query, results)
                return {
                    "ok": True,
                    "found": len(results),
                    "answer": answer.answer,
                    "confidence": answer.confidence,
                    "sources": answer.sources,
                    "warnings": answer.warnings if answer.warnings else None,
                    "related_queries": answer.related_queries if answer.related_queries else None,
                    "synthesized": True,
                }

            except VaultError as e:
                logger.error(f"Recall failed (Vault): {e}", exc_info=True)
                _set_dormant_with_reason("vault_unreachable")
                return make_error(VaultDecryptionError(
                    str(e),
                    recovery_hint=(
                        "Vault decryption failed during recall. Check: "
                        "(1) Is your Vault token valid? "
                        "(2) Does the token have permission for this team index? "
                        "Run /rune:status for diagnostics or /rune:configure to update credentials."
                    ),
                ))
            except (ConnectionError, OSError) as e:
                logger.error(f"Recall failed (network): {e}", exc_info=True)
                _set_dormant_with_reason("envector_unreachable")
                return make_error(EnvectorConnectionError(
                    str(e),
                    recovery_hint=(
                        "Network error during recall. Check: "
                        "(1) Is the enVector endpoint reachable? "
                        "(2) Is your API key still valid? "
                        "Run /rune:status for diagnostics."
                    ),
                ))
            except ValueError as e:
                logger.error(f"Recall failed (input): {e}", exc_info=True)
                return make_error(InvalidInputError(str(e)))
            except Exception as e:
                logger.error(f"Recall failed: {e}", exc_info=True)
                return make_error(e)

        # ---------- MCP Tools: Reload Pipelines ---------- #
        @self.mcp.tool(
            name="reload_pipelines",
            description=(
                "Re-read ~/.rune/config.json and reinitialize scribe/retriever pipelines. "
                "Call this after the Rune activate command changes state to 'active' "
                "to avoid restarting the current agent session."
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_reload_pipelines() -> Dict[str, Any]:
            result = self._init_pipelines()

            # Pre-warm the enVector connection (blocking) immediately after pipeline init
            #
            # Prevent subsequent '/rune:status' diagnostics check is timed out during RegisterKey and
            # reported enVector as "unreachable"
            envector_warmup: Dict[str, Any] = {}
            if result["scribe"] and self.envector is not None:
                import time as _time
                import concurrent.futures as _cf
                WARMUP_TIMEOUT = 60.0  # seconds; RegisterKey can take tens of seconds
                _pool = _cf.ThreadPoolExecutor(max_workers=1)
                try:
                    _t0 = _time.monotonic()
                    _future = _pool.submit(self.envector.invoke_get_index_list)
                    _future.result(timeout=WARMUP_TIMEOUT)
                    envector_warmup = {
                        "ok": True,
                        "latency_ms": round((_time.monotonic() - _t0) * 1000, 1),
                    }
                    logger.info("enVector pre-warm completed in %.0fms", envector_warmup["latency_ms"])
                except _cf.TimeoutError:
                    envector_warmup = {
                        "ok": False,
                        "error": f"Pre-warm timed out after {WARMUP_TIMEOUT:.0f}s",
                    }
                    logger.warning("enVector pre-warm timed out after %.0fs", WARMUP_TIMEOUT)
                except Exception as _e:
                    envector_warmup = {"ok": False, "error": str(_e)}
                    logger.warning("enVector pre-warm failed: %s", _e)
                finally:
                    _pool.shutdown(wait=False)

            return {
                "ok": not result["errors"],
                "state": result["state"],
                "scribe_initialized": result["scribe"],
                "retriever_initialized": result["retriever"],
                "errors": result["errors"] if result["errors"] else None,
                "envector_warmup": envector_warmup or None,
            }

        # ---------- MCP Tools: Capture History ---------- #
        @self.mcp.tool(
            name="capture_history",
            description=(
                "View recent capture history from the local log. "
                "Returns captured decision records in reverse chronological order. "
                "Use to check what has been captured, verify captures, or find record IDs for deletion."
            ),
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_capture_history(
            limit: Annotated[int, Field(description="Number of recent captures to return")] = 20,
            domain: Annotated[Optional[str], Field(description="Filter by domain (e.g. 'architecture', 'security')")] = None,
            since: Annotated[Optional[str], Field(description="Filter captures after this ISO date (e.g. '2026-03-01')")] = None,
        ) -> Dict[str, Any]:
            entries = _read_capture_log(limit=min(limit, 100), domain=domain, since=since)
            return {
                "ok": True,
                "count": len(entries),
                "entries": entries,
            }

        # ---------- MCP Tools: Delete Capture (Soft-Delete) ---------- #
        @self.mcp.tool(
            name="delete_capture",
            description=(
                "Soft-delete a captured decision record by marking its status as 'reverted'. "
                "The record remains in storage but is heavily demoted in search results (0.3x score). "
                "Use capture_history to find record IDs."
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=True)
        )
        async def tool_delete_capture(
            record_id: Annotated[str, Field(description="The record ID to soft-delete (e.g. dec_20260316_arch_abc)")],
        ) -> Dict[str, Any]:
            if self._retriever is None or self._scribe is None:
                return make_error(PipelineNotReadyError(
                    "Pipelines not initialized.",
                    recovery_hint="Run /rune:activate to reinitialize pipelines, or restart Claude Code if the problem persists.",
                ))

            try:
                searcher = self._retriever["searcher"]

                # Search for the record by ID
                from agents.retriever.query_processor import ParsedQuery, TimeScope
                target = await searcher.search_by_id(record_id)
                if not target:
                    return make_error(InvalidInputError(
                        f"Record '{record_id}' not found in search results. "
                        "Use capture_history to find valid record IDs."
                    ))

                # Update status to reverted in metadata
                metadata = target.metadata
                metadata["status"] = "reverted"

                # Re-insert with updated metadata
                envector_client = self._scribe["envector_client"]
                embedding_service = self._scribe["embedding_service"]

                # Use reusable_insight for embedding if available (schema 2.1+)
                ri = metadata.get("reusable_insight", "")
                embedding_text = ri.strip() if ri and ri.strip() else target.payload_text
                insert_result = envector_client.insert_with_text(
                    index_name=self._vault_index_name,
                    texts=[embedding_text],
                    embedding_service=embedding_service,
                    metadata=[metadata],
                )

                if not insert_result.get("ok"):
                    return make_error(EnvectorInsertError(
                        f"Re-insert failed: {insert_result.get('error')}"
                    ))

                _append_capture_log(record_id, target.title, target.domain, "soft-delete", action="deleted")
                return {
                    "ok": True,
                    "deleted": True,
                    "record_id": record_id,
                    "title": target.title,
                    "method": "soft-delete (status=reverted)",
                }

            except VaultError as e:
                logger.error(f"Delete failed (Vault): {e}", exc_info=True)
                _set_dormant_with_reason("vault_unreachable")
                return make_error(VaultConnectionError(
                    str(e),
                    recovery_hint=(
                        "Vault error during delete. Check: "
                        "(1) Is the Vault server running? "
                        "(2) Is your token valid? "
                        "Run /rune:status for diagnostics."
                    ),
                ))
            except (ConnectionError, OSError) as e:
                logger.error(f"Delete failed (network): {e}", exc_info=True)
                _set_dormant_with_reason("envector_unreachable")
                return make_error(EnvectorConnectionError(
                    str(e),
                    recovery_hint=(
                        "Network error during delete. Check: "
                        "(1) Is the enVector endpoint reachable? "
                        "(2) Is your API key valid? "
                        "Run /rune:status for diagnostics."
                    ),
                ))
            except Exception as e:
                logger.error(f"Delete failed: {e}", exc_info=True)
                return make_error(e)

    async def _capture_single(
        self,
        text: str,
        source: str,
        user: Optional[str],
        channel: Optional[str],
        extracted: str,
    ) -> Dict[str, Any]:
        """Execute a single agent-delegated capture.

        Extracted from tool_capture() so it can be reused by batch_capture
        and session-end sweep without duplicating logic.

        The caller is responsible for error handling (try/except); this
        method raises on failure rather than returning error dicts.
        """
        from datetime import datetime, timezone
        from agents.scribe.record_builder import RawEvent
        from agents.scribe.llm_extractor import (
            ExtractionResult, ExtractedFields, PhaseExtractedFields,
        )
        from agents.common.llm_utils import parse_llm_json

        if self._scribe is None:
            return {"ok": False, "error": "Scribe pipeline not initialized."}
        if not self._vault_index_name:
            return {"ok": False, "error": "No index name available."}

        record_builder = self._scribe["record_builder"]
        envector_client = self._scribe["envector_client"]
        embedding_service = self._scribe["embedding_service"]

        data = parse_llm_json(extracted)
        if not data:
            return {"ok": False, "error": "Invalid extracted JSON — could not parse."}

        # Tier 2 check: agent already evaluated
        tier2 = data.get("tier2", {})
        if not tier2.get("capture", True):
            return {
                "ok": True,
                "captured": False,
                "reason": f"Agent rejected: {tier2.get('reason', 'no reason')}",
            }

        # Domain from agent's tier2 evaluation
        agent_domain = tier2.get("domain", "general")

        # Parse confidence from agent JSON
        agent_confidence = data.get("confidence")
        if isinstance(agent_confidence, (int, float)):
            agent_confidence = max(0.0, min(1.0, float(agent_confidence)))
        else:
            agent_confidence = None

        # Build detection from agent data — no detector needed
        detection = _detection_from_agent_data(
            domain=agent_domain,
            confidence=float(agent_confidence) if agent_confidence is not None else 0.0,
        )

        # Build ExtractionResult from agent JSON

        phases_data = data.get("phases")
        if phases_data and len(phases_data) > 1:
            # Multi-phase or bundle
            phases = []
            for p in phases_data[:7]:
                phases.append(PhaseExtractedFields(
                    phase_title=str(p.get("phase_title", ""))[:60],
                    phase_decision=str(p.get("phase_decision", "")),
                    phase_rationale=str(p.get("phase_rationale", "")),
                    phase_problem=str(p.get("phase_problem", "")),
                    alternatives=[str(a) for a in p.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                    tags=[str(t).lower() for t in p.get("tags", []) if t],
                ))
            pre_extraction = ExtractionResult(
                group_title=str(data.get("group_title", ""))[:60],
                group_type=str(data.get("group_type", "phase_chain")),
                group_summary=str(data.get("reusable_insight", "") or data.get("group_title", "")),
                status_hint=str(data.get("status_hint", "")).lower(),
                tags=[str(t).lower() for t in data.get("tags", []) if t],
                confidence=agent_confidence,
                phases=phases,
            )
        else:
            # Single record (may have phases with 0-1 entries, or flat fields)
            if phases_data and len(phases_data) == 1:
                p = phases_data[0]
                single = ExtractedFields(
                    title=str(p.get("phase_title", data.get("title", "")))[:60],
                    rationale=str(p.get("phase_rationale", data.get("rationale", ""))),
                    problem=str(p.get("phase_problem", data.get("problem", ""))),
                    alternatives=[str(a) for a in p.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in p.get("trade_offs", []) if t],
                    status_hint=str(data.get("status_hint", "")).lower(),
                    tags=[str(t).lower() for t in p.get("tags", data.get("tags", [])) if t],
                )
            else:
                single = ExtractedFields(
                    title=str(data.get("title", ""))[:60],
                    rationale=str(data.get("rationale", "")),
                    problem=str(data.get("problem", "")),
                    alternatives=[str(a) for a in data.get("alternatives", []) if a],
                    trade_offs=[str(t) for t in data.get("trade_offs", []) if t],
                    status_hint=str(data.get("status_hint", "")).lower(),
                    tags=[str(t).lower() for t in data.get("tags", []) if t],
                )
            pre_extraction = ExtractionResult(
                group_title=single.title,
                group_summary=str(data.get("reusable_insight", "")) or "",
                status_hint=single.status_hint,
                tags=single.tags,
                confidence=agent_confidence,
                single=single,
            )

        raw_event = RawEvent(
            text=text,
            user=user or "unknown",
            channel=channel or "claude_session",
            timestamp=str(datetime.now(timezone.utc).timestamp()),
            source=source,
        )
        records = record_builder.build_phases(raw_event, detection, pre_extraction=pre_extraction)

        # ===== Novelty check (Memory-as-Filter) =====
        # Compare reusable_insight against existing memory
        embedding_text = _embedding_text_for_record(records[0])
        novelty_info = {"score": 1.0, "class": "novel", "related": []}

        try:
            search_result = envector_client.search_with_text(
                index_name=self._vault_index_name,
                query_text=embedding_text,
                embedding_service=embedding_service,
                topk=3,
            )
            parsed = envector_client.parse_search_results(search_result)
            if parsed:
                max_sim = max(r.get("score", 0.0) for r in parsed)
                novelty_info = _classify_novelty(max_sim)
                novelty_info["related"] = [
                    {
                        "id": r.get("metadata", {}).get("id", ""),
                        "title": r.get("metadata", {}).get("title", ""),
                        "similarity": round(r.get("score", 0.0), 3),
                    }
                    for r in parsed[:3]
                ]

                # NEAR-DUPLICATE -> skip capture (only blocking case)
                if novelty_info["class"] == "near_duplicate":
                    return {
                        "ok": True,
                        "captured": False,
                        "reason": "Near-duplicate — virtually identical insight already stored",
                        "novelty": novelty_info,
                    }
        except Exception as e:
            # Novelty check failure is non-fatal — proceed with capture
            logger.warning("Novelty check failed: %s", e)

        # Embed reusable_insight (schema 2.1) or payload.text (fallback)
        texts = [_embedding_text_for_record(r) for r in records]
        metadata = [r.model_dump(mode="json") for r in records]
        insert_result = envector_client.insert_with_text(
            index_name=self._vault_index_name,
            texts=texts,
            embedding_service=embedding_service,
            metadata=metadata,
        )

        if not insert_result.get("ok"):
            return {"ok": False, "error": f"Insert failed: {insert_result.get('error')}"}

        first = records[0]
        result = {
            "ok": True,
            "captured": True,
            "record_id": first.id,
            "summary": first.title,
            "domain": first.domain.value,
            "certainty": first.why.certainty.value,
            "mode": "agent-delegated",
            "novelty": novelty_info,
        }
        if len(records) > 1:
            result["record_count"] = len(records)
            result["group_id"] = first.group_id
            result["group_type"] = first.group_type or "phase_chain"
        _append_capture_log(
            first.id, first.title, first.domain.value, "agent-delegated",
            novelty_class=novelty_info.get("class", ""),
            novelty_score=novelty_info.get("score", 0.0),
        )
        return result

    async def _legacy_standard_capture(
        self,
        text: str,
        raw_event,
        detector,
        tier2_filter,
        record_builder,
        envector_client,
        embedding_service,
    ) -> Dict[str, Any]:
        """Standard 3-tier capture pipeline (legacy).

        Requires API keys for Tier 2 (LLM filter) and Tier 3 (LLM extraction).
        Retained for backward compatibility with deployments that have
        ANTHROPIC_API_KEY configured and prefer server-side evaluation.

        Most deployments should use agent-delegated mode instead — pass
        the ``extracted`` parameter to let the calling agent handle
        evaluation and extraction.
        """
        # Tier 1: Embedding similarity detection (0 LLM tokens)
        detection = detector.detect(text)
        if not detection.is_significant:
            return {
                "ok": True,
                "captured": False,
                "reason": f"Not significant (confidence: {detection.confidence:.2f}, threshold: {detector.threshold})",
            }

        # Tier 2: LLM policy filter (~200 tokens)
        if tier2_filter and tier2_filter.is_available:
            filter_result = tier2_filter.evaluate(
                text,
                tier1_score=detection.confidence,
                tier1_pattern=detection.matched_pattern or "",
            )
            if not filter_result.should_capture:
                return {
                    "ok": True,
                    "captured": False,
                    "reason": f"Tier 2 rejected: {filter_result.reason}",
                }
            # Update domain from Tier 2 if available
            if filter_result.domain and filter_result.domain != "general":
                from dataclasses import replace
                detection = replace(detection, domain=filter_result.domain)

        # Tier 3: Structured extraction + record building (~500 tokens)
        records = record_builder.build_phases(raw_event, detection)

        # Store in enVector with FHE encryption
        texts = [_embedding_text_for_record(r) for r in records]
        metadata = [r.model_dump(mode="json") for r in records]
        insert_result = envector_client.insert_with_text(
            index_name=self._vault_index_name,
            texts=texts,
            embedding_service=embedding_service,
            metadata=metadata,
        )

        if not insert_result.get("ok"):
            return {"ok": False, "error": f"Insert failed: {insert_result.get('error')}"}

        first = records[0]
        result = {
            "ok": True,
            "captured": True,
            "record_id": first.id,
            "summary": first.title,
            "domain": first.domain.value,
            "certainty": first.why.certainty.value,
        }
        if len(records) > 1:
            result["record_count"] = len(records)
            result["group_id"] = first.group_id
            result["group_type"] = first.group_type or "phase_chain"
        _append_capture_log(first.id, first.title, first.domain.value, "standard")
        return result

    def _init_pipelines(self) -> Dict[str, Any]:
        """
        (Re-)initialize scribe and retriever pipelines by reading fresh config.
        Called at startup from main() and at runtime from reload_pipelines tool.
        """
        result = {"scribe": False, "retriever": False, "state": "unknown", "errors": []}

        try:
            from agents.common.config import load_config as load_rune_config
            from agents.common.embedding_service import EmbeddingService
            from agents.common.envector_client import EnVectorClient
            from agents.common.pattern_cache import PatternCache
            from agents.scribe.pattern_parser import load_all_language_patterns
            from agents.scribe.detector import DecisionDetector
            from agents.scribe.tier2_filter import Tier2Filter
            from agents.scribe.llm_extractor import LLMExtractor
            from agents.scribe.record_builder import RecordBuilder
            from agents.retriever.query_processor import QueryProcessor
            from agents.retriever.searcher import Searcher
            from agents.retriever.synthesizer import Synthesizer

            rune_config = load_rune_config()
            result["state"] = rune_config.state

            if rune_config.state != "active":
                self._scribe = None
                self._retriever = None
                return result

            embedding_svc = EmbeddingService(
                mode=rune_config.embedding.mode,
                model=rune_config.embedding.model,
            )

            # Resolve key_id: prefer Vault-provided, then instance, then fetch from Vault
            key_path = self._key_path
            key_id = self._key_id

            # Always re-fetch from Vault on reload to pick up endpoint/index changes
            if rune_config.vault.endpoint and rune_config.vault.token:
                logger.info("Fetching keys from Vault...")
                success, vault_index, vault_key_id, vault_agent_id, vault_agent_dek, vault_ev_endpoint, vault_ev_api_key = fetch_keys_from_vault(
                    rune_config.vault.endpoint,
                    rune_config.vault.token,
                    key_path,
                    ca_cert=rune_config.vault.ca_cert or None,
                    tls_disable=rune_config.vault.tls_disable,
                )
                if success and vault_key_id:
                    key_id = vault_key_id
                    self._key_id = key_id
                    logger.info(f"Vault provided key_id: {key_id}")
                    if vault_index:
                        self._vault_index_name = vault_index
                    if vault_agent_id:
                        self._agent_id = vault_agent_id
                    if vault_agent_dek:
                        self._agent_dek = vault_agent_dek
                    if vault_ev_endpoint:
                        self._envector_endpoint = vault_ev_endpoint
                    if vault_ev_api_key:
                        self._envector_api_key = vault_ev_api_key
                else:
                    result["errors"].append("Failed to fetch keys from Vault")
                    logger.error("Failed to fetch keys from Vault — capture/search will fail")
                    _set_dormant_with_reason("vault_unreachable")

            if not key_id:
                result["errors"].append("key_id not available. Vault must provide key_id.")
                logger.error("key_id unknown — aborting pipeline init")
                return result

            key_dir = os.path.join(key_path, key_id)
            enc_key_path = os.path.join(key_dir, "EncKey.json")

            # Early return if EncKey still missing after fetch attempt
            if not os.path.exists(enc_key_path):
                result["errors"].append(
                    f"EncKey.json not found at {enc_key_path}. "
                    "Cannot initialize pipelines without encryption keys."
                )
                logger.error(f"EncKey.json missing at {enc_key_path} — aborting pipeline init")
                return result

            envector_client = EnVectorClient(
                address=self._envector_endpoint or "",
                key_path=key_path,
                key_id=key_id,
                access_token=self._envector_api_key or "",
                auto_key_setup=False,
                agent_id=self._agent_id,
                agent_dek=self._agent_dek,
            )

            # Refresh 'self.envector' to reflect updated endpoint/API key from Vault
            try:
                self.envector = EnVectorSDKAdapter(
                    address=self._envector_endpoint or "",
                    key_id=key_id,
                    key_path=key_path,
                    eval_mode="rmp",
                    query_encryption=False,
                    access_token=self._envector_api_key or "",
                    auto_key_setup=False,
                    agent_id=self._agent_id,
                    agent_dek=self._agent_dek,
                )
            except Exception as e:
                logger.warning("enVector adapter refresh failed: %s", e)

            llm_cfg = rune_config.llm
            configured_llm_provider = (llm_cfg.provider or os.getenv("RUNE_LLM_PROVIDER", "anthropic")).lower()
            configured_tier2_provider = (llm_cfg.tier2_provider or os.getenv("RUNE_TIER2_LLM_PROVIDER", configured_llm_provider)).lower()
            anthropic_key = llm_cfg.anthropic_api_key or os.getenv("ANTHROPIC_API_KEY", "")
            openai_key = llm_cfg.openai_api_key or os.getenv("OPENAI_API_KEY", "")
            google_key = llm_cfg.google_api_key or os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY") or ""

            def _resolve_provider(configured: str, fallback: str) -> str:
                if configured == "auto":
                    if self._client_provider_override in ("anthropic", "openai", "google"):
                        return self._client_provider_override
                    env_auto = os.getenv("RUNE_AUTO_LLM_PROVIDER", "").lower()
                    if env_auto in ("anthropic", "openai", "google"):
                        return env_auto
                    return fallback
                if configured in ("anthropic", "openai", "google"):
                    return configured
                return fallback

            llm_provider = _resolve_provider(configured_llm_provider, "anthropic")
            tier2_provider = _resolve_provider(configured_tier2_provider, llm_provider)
            self._active_llm_provider = llm_provider
            self._active_tier2_provider = tier2_provider

            def _provider_key(provider: str) -> str:
                if provider == "openai":
                    return openai_key
                if provider == "google":
                    return google_key
                return anthropic_key

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

            # Phase 1: Core infrastructure (always needed)
            has_llm_key = bool(_provider_key(llm_provider))

            llm_extractor = None
            if has_llm_key:
                llm_extractor = LLMExtractor(
                    llm_provider=llm_provider,
                    anthropic_api_key=anthropic_key,
                    openai_api_key=openai_key,
                    google_api_key=google_key,
                    model=_provider_model(llm_provider, "extract"),
                )
            record_builder = RecordBuilder(llm_extractor=llm_extractor)

            # Phase 2: Legacy pipeline components (only if API keys present)
            detector = None
            tier2_filter = None
            if has_llm_key:
                pattern_cache = PatternCache(embedding_svc)
                patterns = load_all_language_patterns()
                loaded = pattern_cache.load_patterns(patterns)
                logger.info(f"Scribe Tier 1: loaded {loaded} patterns into cache")

                detector = DecisionDetector(
                    pattern_cache,
                    threshold=rune_config.scribe.similarity_threshold,
                    high_confidence_threshold=rune_config.scribe.auto_capture_threshold,
                )

                if rune_config.scribe.tier2_enabled and _provider_key(tier2_provider):
                    tier2_filter = Tier2Filter(
                        llm_provider=tier2_provider,
                        anthropic_api_key=anthropic_key,
                        openai_api_key=openai_key,
                        google_api_key=google_key,
                        model=_provider_model(tier2_provider, "tier2"),
                    )

            self._scribe = {
                "record_builder": record_builder,
                "envector_client": envector_client,
                "embedding_service": embedding_svc,
                # Legacy pipeline components (None if no API keys)
                "detector": detector,
                "tier2_filter": tier2_filter,
            }
            # Unify embedding: pipeline's EmbeddingService is the single source
            self.embedding = embedding_svc
            result["scribe"] = True
            if has_llm_key:
                logger.info("Scribe pipeline initialized (server-side Tier 2/3)")
            else:
                logger.info("Scribe pipeline initialized (agent-delegated mode — no LLM API key)")

            # Retriever pipeline
            if not self._vault_index_name:
                result["errors"].append("Vault index name not available — retriever pipeline skipped.")
                logger.warning("No vault index name — skipping retriever pipeline init")
            else:
                query_processor = QueryProcessor(
                    llm_provider=llm_provider,
                    anthropic_api_key=anthropic_key,
                    openai_api_key=openai_key,
                    google_api_key=google_key,
                    model=_provider_model(llm_provider, "query"),
                )
                searcher = Searcher(envector_client, embedding_svc, self._vault_index_name, vault_client=self.vault)

                synthesizer = None
                if has_llm_key:
                    synthesizer = Synthesizer(
                        llm_provider=llm_provider,
                        anthropic_api_key=anthropic_key,
                        openai_api_key=openai_key,
                        google_api_key=google_key,
                        model=_provider_model(llm_provider, "query"),
                    )

                self._retriever = {
                    "query_processor": query_processor,
                    "searcher": searcher,
                    "synthesizer": synthesizer,
                }
                result["retriever"] = True
                if has_llm_key:
                    logger.info("Retriever pipeline initialized (server-side synthesis)")
                else:
                    logger.info("Retriever pipeline initialized (agent-delegated mode — raw results returned)")

        except VaultError as e:
            result["errors"].append({
                "code": "VAULT_CONNECTION_ERROR",
                "message": str(e),
                "retryable": True,
                "recovery_hint": "Vault connection failed during pipeline initialization. Check Vault endpoint and token via /rune:status.",
            })
            _set_dormant_with_reason("vault_unreachable")
            logger.warning(f"Pipeline init failed (Vault): {e}")
        except Exception as e:
            result["errors"].append({
                "code": "INTERNAL_ERROR",
                "message": str(e),
                "retryable": False,
                "recovery_hint": "Unexpected error during pipeline initialization. Try /rune:activate or restart Claude Code.",
            })
            _set_dormant_with_reason("pipeline_init_failed")
            logger.warning(f"Pipeline init failed: {e}")

        return result

    def run(self) -> None:
        """Runs the MCP server using stdio transport."""
        self.mcp.run(transport="stdio")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Run the enVector MCP server (stdio).")
    parser.add_argument(
        "--mode", default="stdio", help=argparse.SUPPRESS,  # kept for backwards compat
    )
    parser.add_argument(
        "--server-name",
        default=os.getenv("MCP_SERVER_NAME", "envector_mcp_server"),
        help="Advertised MCP server name.",
    )
    parser.add_argument(
        "--envector-endpoint", "--envector-address",
        dest="envector_endpoint",
        default=os.getenv("ENVECTOR_ENDPOINT") or os.getenv("ENVECTOR_ADDRESS"),
        help="enVector endpoint (host:port or URL).",
    )
    parser.add_argument(
        "--envector-key-id",
        default=os.getenv("ENVECTOR_KEY_ID", "vault-key"),
        help="enVector key identifier.",
    )
    parser.add_argument(
        "--envector-key-path",
        default=os.getenv("ENVECTOR_KEY_PATH", os.path.join(CURRENT_DIR, "keys")),
        help="Path to the enVector key directory.",
    )
    parser.add_argument(
        "--envector-eval-mode",
        default=os.getenv("ENVECTOR_EVAL_MODE", "rmp"),
        help="enVector evaluation mode (e.g., 'rmp', 'mm').",
    )
    parser.add_argument(
        "--encrypted-query",
        action="store_true",
        default=os.getenv("ENVECTOR_ENCRYPTED_QUERY", "false").lower() in ("true", "1", "yes"),
        help="Encrypt the query vectors."
    )
    parser.add_argument(
        "--no-auto-key-setup",
        action="store_true",
        help="Disable automatic key generation. Use when keys are provided externally (e.g., from Rune-Vault).",
    )
    args = parser.parse_args()

    MCP_SERVER_NAME = args.server_name
    ENVECTOR_ENDPOINT = args.envector_endpoint or ""
    ENVECTOR_API_KEY = os.getenv("ENVECTOR_API_KEY", None)
    ENVECTOR_KEY_ID = args.envector_key_id
    ENVECTOR_KEY_PATH = args.envector_key_path
    ENVECTOR_EVAL_MODE = args.envector_eval_mode
    ENCRYPTED_QUERY = args.encrypted_query

    # ── Load ~/.rune/config.json if ENVECTOR_CONFIG is set ──
    _vault_cfg = {}  # populated from config file if available
    _config_path = os.getenv("ENVECTOR_CONFIG")
    if _config_path:
        _config_path = os.path.expanduser(_config_path)
        if os.path.exists(_config_path):
            try:
                with open(_config_path) as _cf:
                    _rune_config = json.load(_cf)
                _vault_cfg = _rune_config.get("vault", {})
                if not os.getenv("RUNEVAULT_ENDPOINT") and (_vault_cfg.get("endpoint") or _vault_cfg.get("url")):
                    os.environ["RUNEVAULT_ENDPOINT"] = _vault_cfg.get("endpoint") or _vault_cfg["url"]
                if not os.getenv("RUNEVAULT_TOKEN") and _vault_cfg.get("token"):
                    os.environ["RUNEVAULT_TOKEN"] = _vault_cfg["token"]
                logger.info(f"Loaded Rune config from {_config_path}")
            except Exception as _e:
                logger.warning(f"Failed to read Rune config {_config_path}: {_e}")
        else:
            logger.info(f"Rune config not found at {_config_path}, using env vars only")

    # Rune-Vault Integration
    _env_var = os.getenv("ENVECTOR_AUTO_KEY_SETUP", "true").lower() in ("true", "1", "yes")
    AUTO_KEY_SETUP = _env_var and not args.no_auto_key_setup
    RUNEVAULT_ENDPOINT = os.getenv("RUNEVAULT_ENDPOINT", None)
    RUNEVAULT_TOKEN = os.getenv("RUNEVAULT_TOKEN", None)

    VAULT_CA_CERT = os.getenv("VAULT_CA_CERT") or _vault_cfg.get("ca_cert", "") or None
    VAULT_TLS_DISABLE = os.getenv("VAULT_TLS_DISABLE", "").lower() == "true"
    if not VAULT_TLS_DISABLE:
        VAULT_TLS_DISABLE = bool(_vault_cfg.get("tls_disable", False))

    VAULT_CONFIGURED = bool(RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN)
    VAULT_KEYS_LOADED = False
    VAULT_INDEX_NAME = None
    AGENT_ID = None
    AGENT_DEK = None

    if RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN:
        # When Vault is configured (Rune plugin mode), use canonical key path.
        # key_id is discovered from Vault — no hardcoded default.
        ENVECTOR_KEY_PATH = MCPServerApp.DEFAULT_KEY_PATH

        logger.info(f"Vault configured — fetching public keys from: {RUNEVAULT_ENDPOINT}")
        success, vault_index, vault_key_id, vault_agent_id, vault_agent_dek, vault_ev_endpoint, vault_ev_api_key = fetch_keys_from_vault(
            RUNEVAULT_ENDPOINT, RUNEVAULT_TOKEN,
            ENVECTOR_KEY_PATH,
            ca_cert=VAULT_CA_CERT,
            tls_disable=VAULT_TLS_DISABLE,
        )
        if success and vault_key_id:
            ENVECTOR_KEY_ID = vault_key_id
            logger.info(f"Vault provided key_id: {ENVECTOR_KEY_ID}")
            AUTO_KEY_SETUP = False
            VAULT_KEYS_LOADED = True
            VAULT_INDEX_NAME = vault_index
            AGENT_ID = vault_agent_id
            AGENT_DEK = vault_agent_dek
            if vault_ev_endpoint:
                ENVECTOR_ENDPOINT = vault_ev_endpoint
                logger.info("Using enVector endpoint from Vault bundle")
            if vault_ev_api_key:
                ENVECTOR_API_KEY = vault_ev_api_key
                logger.info("Using enVector API key from Vault bundle")
            if not vault_ev_endpoint or not vault_ev_api_key:
                logger.error("Vault bundle missing enVector credentials. Contact your Vault administrator.")
                _set_dormant_with_reason("envector_not_provisioned")
        else:
            logger.error("Failed to fetch keys/key_id from Vault. Operations requiring encryption will fail.")
            _set_dormant_with_reason("vault_unreachable")
            AUTO_KEY_SETUP = False
    elif RUNEVAULT_ENDPOINT and not RUNEVAULT_TOKEN:
        logger.warning("Vault endpoint provided but no token specified. Skipping Vault integration.")
        VAULT_CONFIGURED = True
        AUTO_KEY_SETUP = False
    elif not AUTO_KEY_SETUP:
        logger.info(f"Using externally provided keys from: {ENVECTOR_KEY_PATH}")

    envector_adapter = None
    try:
        envector_adapter = EnVectorSDKAdapter(
            address=ENVECTOR_ENDPOINT,
            key_id=ENVECTOR_KEY_ID,
            key_path=ENVECTOR_KEY_PATH,
            eval_mode=ENVECTOR_EVAL_MODE,
            query_encryption=ENCRYPTED_QUERY,
            access_token=ENVECTOR_API_KEY,
            auto_key_setup=AUTO_KEY_SETUP,
        )
    except Exception as e:
        logger.warning(f"enVector adapter init failed (server will start in degraded mode): {e}")

    vault_client = None
    if RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN:
        logger.info(f"Initializing Vault client: {RUNEVAULT_ENDPOINT}")
        vault_client = VaultClient(
            vault_endpoint=RUNEVAULT_ENDPOINT,
            vault_token=RUNEVAULT_TOKEN,
            ca_cert=VAULT_CA_CERT,
            tls_disable=VAULT_TLS_DISABLE,
        )
        logger.info("Vault client initialized - recall tool available")
    else:
        logger.info("Vault not configured - recall tool will be unavailable")

    # ── Create MCP app (pipelines initialized via _init_pipelines) ──
    app = MCPServerApp(
        mcp_server_name=MCP_SERVER_NAME,
        envector_adapter=envector_adapter,
        vault_client=vault_client,
        vault_index_name=VAULT_INDEX_NAME,
        key_path=ENVECTOR_KEY_PATH,
        key_id=ENVECTOR_KEY_ID,
        agent_id=AGENT_ID,
        agent_dek=AGENT_DEK,
    )

    # Set enVector credentials from Vault bundle on app instance
    if ENVECTOR_ENDPOINT:
        app._envector_endpoint = ENVECTOR_ENDPOINT
    if ENVECTOR_API_KEY:
        app._envector_api_key = ENVECTOR_API_KEY

    # Initialize pipelines (reads ~/.rune/config.json state)
    _pipeline_result = app._init_pipelines()
    if _pipeline_result["scribe"]:
        logger.info("Scribe pipeline ready (capture tool available)")
    if _pipeline_result["retriever"]:
        logger.info("Retriever pipeline ready (recall tool available)")
    if _pipeline_result["errors"]:
        logger.warning(f"Pipeline init issues: {_pipeline_result['errors']}")

    def _handle_shutdown(signum, frame):
        raise SystemExit(0)
    for sig in (signal.SIGINT, getattr(signal, "SIGTERM", None)):
        if sig is not None:
            signal.signal(sig, _handle_shutdown)

    app.run()
