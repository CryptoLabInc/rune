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
import numpy as np
import os, sys, signal
import json

logger = logging.getLogger("rune.mcp")
from pydantic import Field

# Add parent directory (rune/mcp/) to sys.path so `from adapter import ...` works
CURRENT_DIR = os.path.dirname(os.path.abspath(__file__))
MCP_ROOT = os.path.dirname(CURRENT_DIR)
PLUGIN_ROOT = os.path.dirname(MCP_ROOT)  # rune/ root for `from agents import ...`
if MCP_ROOT not in sys.path:
    sys.path.insert(0, MCP_ROOT)
if PLUGIN_ROOT not in sys.path:
    sys.path.insert(0, PLUGIN_ROOT)

from fastmcp import FastMCP  # pip install fastmcp
from mcp.types import ToolAnnotations
from adapter import EnVectorSDKAdapter, EmbeddingAdapter
from adapter.vault_client import VaultClient, VaultError


def fetch_keys_from_vault(vault_endpoint: str, vault_token: str, key_path: str) -> tuple:
    """
    Fetches public keys (EncKey, EvalKey) from Rune-Vault via gRPC.

    Args:
        vault_endpoint: Rune-Vault endpoint URL (e.g., http://vault-mcp:50080/mcp)
        vault_token: Authentication token for Vault
        key_path: Local directory to save the fetched keys

    Returns:
        tuple: (success: bool, index_name: Optional[str])
    """
    import asyncio

    async def _fetch():
        client = VaultClient(vault_endpoint=vault_endpoint, vault_token=vault_token)
        try:
            bundle = await client.get_public_key()

            # Extract team index name before saving key files
            vault_index_name = bundle.pop("index_name", None)
            if vault_index_name:
                logger.info(f"Vault provided team index name: {vault_index_name}")

            # Ensure key directory exists
            os.makedirs(key_path, exist_ok=True)

            # Save each key file
            for filename, key_content in bundle.items():
                filepath = os.path.join(key_path, filename)
                with open(filepath, 'w') as f:
                    f.write(key_content)
                logger.info(f"Saved {filename} to {filepath}")

            return True, vault_index_name

        except Exception as e:
            logger.error(f"Failed to fetch keys from Vault: {e}")
            return False, None
        finally:
            await client.close()

    return asyncio.run(_fetch())

class MCPServerApp:
    """
    Main application class for the MCP server.

    Security Model (with Rune-Vault):
    - MCP Server handles embeddings, query encryption, and orchestration
    - Rune-Vault holds secret key and performs all decryption
    - Agent never has access to secret key
    """
    def __init__(
            self,
            envector_adapter: Optional[EnVectorSDKAdapter] = None,
            mcp_server_name: str = "envector_mcp_server",
            embedding_adapter: "EmbeddingAdapter" = None,
            vault_client: Optional[VaultClient] = None,
            vault_index_name: Optional[str] = None,
            scribe_pipeline: Optional[Dict[str, Any]] = None,
            retriever_pipeline: Optional[Dict[str, Any]] = None,
        ) -> None:
        """
        Initializes the MCPServerApp with the given adapter and server name.
        Args:
            envector_adapter (EnVectorSDKAdapter): The enVector SDK adapter instance.
            mcp_server_name (str): The name of the MCP server.
            vault_client (VaultClient): Optional Vault client for secure decryption.
            vault_index_name (str): Team index name provisioned by Vault admin (optional).
            scribe_pipeline (dict): Pre-initialized scribe pipeline components.
            retriever_pipeline (dict): Pre-initialized retriever pipeline components.
        """
        # adapters
        self.envector = envector_adapter
        self.embedding = embedding_adapter
        self.vault = vault_client
        self._vault_index_name = vault_index_name
        self._scribe = scribe_pipeline
        self._retriever = retriever_pipeline
        # mcp
        self.mcp = FastMCP(name=mcp_server_name)

        # ---------- Common Query Preprocessing ---------- #
        def _preprocess(raw_query: Any) -> Union[List[float], List[List[float]]]:
            """Convert raw query input (string, ndarray, list) into a valid vector or batch of vectors."""
            if isinstance(raw_query, str):
                raw_query = raw_query.strip()

                if self.embedding is not None:
                    return self.embedding.get_embedding([raw_query])[0]

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

        # ---------- MCP Tools: Remember (Vault-Secured Retrieval) ---------- #
        @self.mcp.tool(
            name="remember",
            description=(
                "Recall from shared team memory stored on enVector Cloud. "
                "Unlike 'search' where the data owner is the local operator, "
                "'remember' accesses indexes whose decryption key (secret key) is held "
                "exclusively by a team-shared Rune-Vault server — never loaded into "
                "this MCP server runtime. This isolation prevents agent tampering "
                "attacks from indiscriminately decrypting shared vectors. "
                "Vault enforces access policy (max 10 results per query, audit trail). "
                "Use this when recalling shared team knowledge: past decisions, "
                "institutional context, onboarding material, or any collectively "
                "owned memory. "
                "Accepts text queries (auto-embedded), vector arrays, or JSON-encoded vectors."
            ),
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_remember(
            index_name: Annotated[Optional[str], Field(
                description="index name to remember from. If omitted, the admin-provisioned team index is used."
            )] = None,
            query: Annotated[Optional[Union[str, List[float]]], Field(
                description="single recall query: natural language text or vector (list of floats)"
            )] = None,
            topk: Annotated[int, Field(description="number of results to recall (1-10, enforced by Vault policy)")] = 5,
            request_id: Annotated[str, Field(description="optional correlation ID for audit trail")] = "",
        ) -> Dict[str, Any]:
            """
            Recall organizational decisions and context from encrypted memory.
            This tool accepts a SINGLE query only. For batch queries, use the search tool instead.

            Pipeline:
            1. Embed query, run encrypted similarity scoring on enVector Cloud → result ciphertext
            2. Rune-Vault decrypts result ciphertext with secret key, selects top-k (secret key never leaves Vault)
            3. Retrieve metadata for top-k indices from enVector Cloud

            Args:
                index_name (str): The name of index to recall from. If omitted, uses the admin-provisioned team index.
                query (Union[str, List[float]]): Single recall query (text or vector).
                topk (int): Number of top results (max 10, enforced by Vault).
                request_id (str): Optional correlation ID for audit trail.

            Returns:
                Dict[str, Any]: The recalled results and audit information,
            """
            # Resolve index_name: explicit > vault default > error
            if index_name is None:
                if self._vault_index_name:
                    index_name = self._vault_index_name
                else:
                    return {
                        "ok": False,
                        "error": "index_name is required. No team index configured by Vault admin.",
                    }

            if query is None:
                return {"ok": False, "error": "query parameter is required."}

            if self.envector is None:
                return {
                    "ok": False,
                    "error": "enVector adapter not available. MCP server started without enVector connection.",
                }

            if self.vault is None:
                return {
                    "ok": False,
                    "error": "Vault not configured. Set RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN environment variables.",
                }


            try:
                preprocessed_query = _preprocess(query)
            except ValueError as exc:
                return {"ok": False, "error": f"Query preprocessing failed: {exc}"}

            if isinstance(preprocessed_query, list) and len(preprocessed_query) > 0 and isinstance(preprocessed_query[0], list):
                return {
                    "ok": False,
                    "error": "Remember tool accepts single query only. Use search tool for batch queries."
                }

            if topk > 10:
                return {"ok": False, "error": "Policy: max top_k is 10."}

            try:
                # Step 1: encrypted search → result ciphertext
                scoring_result = self.envector.call_score(
                    index_name=index_name,
                    query=[preprocessed_query]
                )
                if not scoring_result.get("ok"):
                    return {"ok": False, "error": scoring_result.get("error"), "request_id": request_id or "N/A"}

                blobs = scoring_result["encrypted_blobs"]
                if not blobs:
                    return {"ok": True, "results": [], "request_id": request_id or "N/A"}

                # Step 2: Vault decrypts + top-k
                vault_result = await self.vault.decrypt_search_results(
                    encrypted_blob_b64=blobs[0],
                    top_k=topk,
                )
                if not vault_result.ok:
                    return {"ok": False, "error": f"Vault decryption failed: {vault_result.error}", "request_id": request_id or "N/A"}

                # Step 3: Retrieve encrypted metadata
                metadata_result = self.envector.call_remind(
                    index_name=index_name,
                    indices=vault_result.results,
                    output_fields=["metadata"]
                )
                if not metadata_result.get("ok"):
                    return {"ok": False, "error": metadata_result.get("error"), "request_id": request_id or "N/A"}

                # Step 4: Decrypt metadata via Vault (MetadataKey never leaves Vault)
                encrypted_entries = metadata_result.get("results", [])
                encrypted_blobs = [
                    entry.get("data", "") for entry in encrypted_entries
                ]
                if encrypted_blobs and any(encrypted_blobs):
                    decrypted_metadata = await self.vault.decrypt_metadata(
                        encrypted_metadata_list=[b for b in encrypted_blobs if b is not None]
                    )
                    # Merge decrypted metadata with scores
                    for i, entry in enumerate(encrypted_entries):
                        if i < len(decrypted_metadata):
                            entry["metadata"] = decrypted_metadata[i]
                        entry.pop("data", None)

                return {
                    "ok": True,
                    "results": encrypted_entries,
                    "request_id": request_id or "N/A",
                }

            except VaultError as e:
                return {"ok": False, "error": f"Vault error: {e}", "request_id": request_id or "N/A"}
            except Exception as e:
                return {"ok": False, "error": str(e), "request_id": request_id or "N/A"}

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
                return {
                    "ok": False,
                    "vault_configured": True,
                    "error": f"Vault health check failed: {e}"
                }

        # ---------- MCP Tools: Capture (Scribe Pipeline) ---------- #
        @self.mcp.tool(
            name="capture",
            description=(
                "Capture a significant organizational decision into encrypted memory. "
                "Runs the 3-tier pipeline: Tier 1 embedding similarity detection, "
                "Tier 2 LLM policy filter (Haiku), Tier 3 structured extraction (Sonnet). "
                "Only captures text that passes all tiers as significant."
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_capture(
            text: Annotated[str, Field(description="The text containing a potential decision or significant context to capture")],
            source: Annotated[str, Field(description="Source of the text (e.g., 'claude_agent', 'slack', 'github')")] = "claude_agent",
            user: Annotated[Optional[str], Field(description="User who authored the text")] = None,
            channel: Annotated[Optional[str], Field(description="Channel or location where the text originated")] = None,
        ) -> Dict[str, Any]:
            if self._scribe is None:
                return {"ok": False, "error": "Scribe pipeline not initialized. Check Rune configuration."}

            if not self._vault_index_name:
                return {"ok": False, "error": "No index name available. Vault must provide a team index name."}

            try:
                from datetime import datetime, timezone
                from agents.scribe.record_builder import RawEvent

                detector = self._scribe["detector"]
                tier2_filter = self._scribe.get("tier2_filter")
                record_builder = self._scribe["record_builder"]
                envector_client = self._scribe["envector_client"]
                embedding_service = self._scribe["embedding_service"]

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
                raw_event = RawEvent(
                    text=text,
                    user=user or "unknown",
                    channel=channel or "claude_session",
                    timestamp=str(datetime.now(timezone.utc).timestamp()),
                    source=source,
                )
                record = record_builder.build(raw_event, detection)

                # Store in enVector with FHE encryption
                insert_result = envector_client.insert_with_text(
                    index_name=self._vault_index_name,
                    texts=[record.payload.text],
                    embedding_service=embedding_service,
                    metadata=[record.model_dump(mode="json")],
                )

                if not insert_result.get("ok"):
                    return {"ok": False, "error": f"Insert failed: {insert_result.get('error')}"}

                return {
                    "ok": True,
                    "captured": True,
                    "record_id": record.id,
                    "summary": record.title,
                    "domain": record.domain.value,
                    "certainty": record.why.certainty.value,
                }

            except Exception as e:
                logger.error(f"Capture failed: {e}", exc_info=True)
                return {"ok": False, "error": str(e)}

        # ---------- MCP Tools: Recall (Retriever Pipeline) ---------- #
        @self.mcp.tool(
            name="recall",
            description=(
                "Search organizational memory for past decisions, context, and insights. "
                "Parses the query to understand intent, searches encrypted vector memory "
                "with multi-query expansion, and synthesizes a coherent answer. "
                "Respects evidence certainty levels in the response."
            ),
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_recall(
            query: Annotated[str, Field(description="Natural language question about past decisions or organizational context")],
            topk: Annotated[int, Field(description="Number of results to consider for synthesis")] = 5,
        ) -> Dict[str, Any]:
            if self._retriever is None:
                return {"ok": False, "error": "Retriever pipeline not initialized. Check Rune configuration."}

            if topk > 50:
                return {"ok": False, "error": "topk must be 50 or less."}

            try:
                query_processor = self._retriever["query_processor"]
                searcher = self._retriever["searcher"]
                synthesizer = self._retriever["synthesizer"]

                # Step 1: Parse query (intent detection, entity extraction, query expansion)
                parsed_query = query_processor.parse(query)

                # Step 2: Search enVector (multi-query expansion, dedup, ranking)
                results = searcher.search(parsed_query, topk=topk)

                # Step 3: Synthesize answer (LLM synthesis with certainty respect)
                answer = synthesizer.synthesize(parsed_query, results)

                return {
                    "ok": True,
                    "found": len(results),
                    "answer": answer.answer,
                    "confidence": answer.confidence,
                    "sources": answer.sources,
                    "warnings": answer.warnings,
                    "related_queries": answer.related_queries,
                }

            except Exception as e:
                logger.error(f"Recall failed: {e}", exc_info=True)
                return {"ok": False, "error": str(e)}

        # ---------- MCP Tools: Reload Pipelines ---------- #
        @self.mcp.tool(
            name="reload_pipelines",
            description=(
                "Re-read ~/.rune/config.json and reinitialize scribe/retriever pipelines. "
                "Call this after /rune:activate changes state to 'active' to avoid "
                "restarting Claude Code."
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_reload_pipelines() -> Dict[str, Any]:
            result = self._init_pipelines()
            return {
                "ok": not result["errors"],
                "state": result["state"],
                "scribe_initialized": result["scribe"],
                "retriever_initialized": result["retriever"],
                "errors": result["errors"] if result["errors"] else None,
            }

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

            envector_client = EnVectorClient(
                address=rune_config.envector.endpoint,
                key_path=str(os.path.expanduser("~/.rune/keys")),
                key_id="rune_key",
                access_token=rune_config.envector.api_key,
                auto_key_setup=False,
            )

            anthropic_key = rune_config.retriever.anthropic_api_key or os.getenv("ANTHROPIC_API_KEY", "")

            # Scribe pipeline
            pattern_cache = PatternCache(embedding_svc)
            patterns = load_all_language_patterns()
            loaded = pattern_cache.load_patterns(patterns)
            logger.info(f"Scribe Tier 1: loaded {loaded} patterns into cache")

            detector = DecisionDetector(
                pattern_cache,
                threshold=rune_config.scribe.similarity_threshold,
                high_confidence_threshold=rune_config.scribe.auto_capture_threshold,
            )

            tier2_filter = None
            if rune_config.scribe.tier2_enabled and anthropic_key:
                tier2_filter = Tier2Filter(
                    anthropic_api_key=anthropic_key,
                    model=rune_config.scribe.tier2_model,
                )

            llm_extractor = None
            if anthropic_key:
                llm_extractor = LLMExtractor(anthropic_api_key=anthropic_key)
            record_builder = RecordBuilder(llm_extractor=llm_extractor)

            self._scribe = {
                "detector": detector,
                "tier2_filter": tier2_filter,
                "record_builder": record_builder,
                "envector_client": envector_client,
                "embedding_service": embedding_svc,
            }
            result["scribe"] = True
            logger.info("Scribe pipeline initialized")

            # Retriever pipeline
            if not self._vault_index_name:
                result["errors"].append("Vault index name not available — retriever pipeline skipped.")
                logger.warning("No vault index name — skipping retriever pipeline init")
            else:
                query_processor = QueryProcessor(anthropic_api_key=anthropic_key)
                searcher = Searcher(envector_client, embedding_svc, self._vault_index_name)
                synthesizer = Synthesizer(anthropic_api_key=anthropic_key)

                self._retriever = {
                    "query_processor": query_processor,
                    "searcher": searcher,
                    "synthesizer": synthesizer,
                }
                result["retriever"] = True
                logger.info("Retriever pipeline initialized")

        except Exception as e:
            result["errors"].append(str(e))
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
        default=os.getenv("ENVECTOR_KEY_ID", "mcp_key"),
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
        "--embedding-mode",
        default=os.getenv("EMBEDDING_MODE", "femb"),
        choices=("femb", "sbert", "hf", "openai"),
        help="Embedding backend.",
    )
    parser.add_argument(
        "--embedding-model",
        default=os.getenv("EMBEDDING_MODEL", "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"),
        help="Embedding model name.",
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
    _config_path = os.getenv("ENVECTOR_CONFIG")
    if _config_path:
        _config_path = os.path.expanduser(_config_path)
        if os.path.exists(_config_path):
            try:
                with open(_config_path) as _cf:
                    _rune_config = json.load(_cf)
                _ev_cfg = _rune_config.get("envector", {})
                _vault_cfg = _rune_config.get("vault", {})
                if not ENVECTOR_ENDPOINT and _ev_cfg.get("endpoint"):
                    ENVECTOR_ENDPOINT = _ev_cfg["endpoint"]
                    logger.info(f"Loaded ENVECTOR_ENDPOINT from config: {ENVECTOR_ENDPOINT}")
                if not ENVECTOR_API_KEY and _ev_cfg.get("api_key"):
                    ENVECTOR_API_KEY = _ev_cfg["api_key"]
                    logger.info("Loaded ENVECTOR_API_KEY from config")
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

    VAULT_CONFIGURED = bool(RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN)
    VAULT_KEYS_LOADED = False
    VAULT_INDEX_NAME = None

    if RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN:
        logger.info(f"Vault configured — fetching public keys from: {RUNEVAULT_ENDPOINT}")
        success, vault_index = fetch_keys_from_vault(RUNEVAULT_ENDPOINT, RUNEVAULT_TOKEN, os.path.join(ENVECTOR_KEY_PATH, ENVECTOR_KEY_ID))
        if success:
            logger.info("Successfully fetched keys from Vault")
            AUTO_KEY_SETUP = False
            VAULT_KEYS_LOADED = True
            VAULT_INDEX_NAME = vault_index
        else:
            logger.error("Failed to fetch keys from Vault. Operations requiring encryption will fail.")
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

    if args.embedding_model is not None:
        from adapter.embeddings import EmbeddingAdapter
        embedding_adapter = EmbeddingAdapter(
            mode=args.embedding_mode,
            model_name=args.embedding_model
        )
    else:
        embedding_adapter = None

    vault_client = None
    if RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN:
        logger.info(f"Initializing Vault client: {RUNEVAULT_ENDPOINT}")
        vault_client = VaultClient(
            vault_endpoint=RUNEVAULT_ENDPOINT,
            vault_token=RUNEVAULT_TOKEN,
        )
        logger.info("Vault client initialized - remember tool available")
    else:
        logger.info("Vault not configured - remember tool will be unavailable")

    # ── Create MCP app (pipelines initialized via _init_pipelines) ──
    app = MCPServerApp(
        mcp_server_name=MCP_SERVER_NAME,
        envector_adapter=envector_adapter,
        embedding_adapter=embedding_adapter,
        vault_client=vault_client,
        vault_index_name=VAULT_INDEX_NAME,
    )

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
