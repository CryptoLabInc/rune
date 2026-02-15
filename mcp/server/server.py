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
from dotenv import load_dotenv
load_dotenv()

# Add parent directory (rune/mcp/) to sys.path so `from adapter import ...` works
CURRENT_DIR = os.path.dirname(os.path.abspath(__file__))
MCP_ROOT = os.path.dirname(CURRENT_DIR)
if MCP_ROOT not in sys.path:
    sys.path.insert(0, MCP_ROOT)

from fastmcp import FastMCP  # pip install fastmcp
from fastmcp.exceptions import ToolError
from mcp.types import ToolAnnotations
from adapter import EnVectorSDKAdapter, EmbeddingAdapter, DocumentPreprocessingAdapter
from adapter.vault_client import VaultClient, create_vault_client, DecryptResult, VaultError


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
            envector_adapter: EnVectorSDKAdapter,
            mcp_server_name: str = "envector_mcp_server",
            embedding_adapter: "EmbeddingAdapter" = None,
            document_preprocessor: DocumentPreprocessingAdapter = None,
            vault_client: Optional[VaultClient] = None,
            vault_configured: bool = False,
            vault_keys_loaded: bool = False,
            vault_index_name: Optional[str] = None,
        ) -> None:
        """
        Initializes the MCPServerApp with the given adapter and server name.
        Args:
            adapter (EnVectorSDKAdapter): The enVector SDK adapter instance.
            mcp_server_name (str): The name of the MCP server.
            vault_client (VaultClient): Optional Vault client for secure decryption.
            vault_configured (bool): Whether Vault credentials are present in config.
            vault_keys_loaded (bool): Whether public keys were successfully fetched from Vault.
            vault_index_name (str): Team index name provisioned by Vault admin (optional).
        """
        # adapters
        self.envector = envector_adapter
        self.embedding = embedding_adapter
        self.preprocessor = document_preprocessor
        self.vault = vault_client
        self._vault_configured = vault_configured
        self._vault_keys_loaded = vault_keys_loaded
        self._vault_index_name = vault_index_name
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

        # ---------- MCP Tools: Create Index ---------- #
        @self.mcp.tool(
            name="create_index",
            description="Create an index in enVector.",
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_create_index(
            index_name: Annotated[str, Field(description="index name to create")],
            dim: Annotated[int, Field(description="dimensionality of the index")],
            index_params: Annotated[Dict[str, Any], Field(description="indexing parameters including FLAT and IVF_FLAT. The default is FLAT, or set index_params as {'index_type': 'IVF_FLAT', 'nlist': <int>, 'default_nprobe': <int>} for IVF.")]
        ) -> Dict[str, Any]:
            """
            MCP tool to create an index using the enVector SDK adapter.
            Calls self.envector.call_create_index(...).

            Args:
                index_name (str): The name of the index to create.
                dim (int): The dimensionality of the index.
                index_params (Dict[str, Any]): The parameters for the index.

            Returns:
                Dict[str, Any]: The create index results from the enVector SDK adapter.
            """
            # Guard: If Vault is configured, keys must have been loaded from Vault
            if self._vault_configured and not self._vault_keys_loaded:
                return {
                    "ok": False,
                    "error": "Cannot create index: Vault is configured but public keys were not loaded. "
                             "Check Vault connectivity and restart the MCP server."
                }
            return self.envector.call_create_index(index_name=index_name, dim=dim, index_params=index_params)

        # ---------- MCP Tools: Get Index List ---------- #
        @self.mcp.tool(
            name="get_index_list",
            description="Get the list of indexes from the enVector SDK.",
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_get_index_list() -> Dict[str, Any]:
            """
            MCP tool to get the list of indexes using the enVector SDK adapter.
            Call the adapter's call_get_index_list method.

            Returns:
                Dict[str, Any]: The index list from the enVector SDK adapter.
            """
            return self.envector.call_get_index_list()

        # ---------- MCP Tools: Get Index Info ---------- #
        @self.mcp.tool(
            name="get_index_info",
            description="Get information about a specific index from the enVector SDK.",
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_get_index_info(
            index_name: Annotated[str, Field(description="index name to get information for")],
        ) -> Dict[str, Any]:
            """
            MCP tool to get information about a specific index using the enVector SDK adapter.
            Call the adapter's call_get_index_info method.

            Args:
                index_name (str): The name of the index to retrieve information for.

            Returns:
                Dict[str, Any]: The index information from the enVector SDK adapter.
            """
            return self.envector.call_get_index_info(index_name=index_name)

        # ---------- MCP Tools: Insert ---------- #
        @self.mcp.tool(
            name="insert",
            description=(
                "Insert vectors or metadata using enVector SDK. "
                "Allowing to insert metadata as text only as supporting embedding the metadata as vectors. "
                "Allowing one or more vectors, but insert 'batch_size' vectors in once would be more efficient. "
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_insert(
            index_name: Annotated[str, Field(description="index name to insert data into")],
            vectors: Annotated[Union[List[float], List[List[float]]], Field(description="vectors to insert")] = None,
            metadata: Annotated[Union[Any, List[Any]], Field(description="the corresponding metadata of the vectors to insert for retrieval")] = None
        ) -> Dict[str, Any]:
            """
            MCP tool to perform insert using the enVector SDK adapter.
            Call the adapter's call_insert method.

            Args:
                index_name (str): The name of the index to insert into.
                vectors (Union[List[float], List[List[float]]]): The vector(s) to insert.
                metadata (Union[Any, List[Any]]): The list of metadata associated with the vectors.

            Returns:
                Dict[str, Any]: The insert results from the enVector SDK adapter.
            """
            if self._vault_configured and not self._vault_keys_loaded:
                return {
                    "ok": False,
                    "error": "Cannot insert: Vault is configured but public keys were not loaded. "
                             "Check Vault connectivity and restart the MCP server."
                }
            if vectors is None and metadata is None:
                raise ValueError("`vectors` or `metadata` parameter must be provided.")

            if vectors is not None:
                # Instance normalization for vectors
                if isinstance(vectors, np.ndarray):
                    vectors = [vectors.tolist()]
                elif isinstance(vectors, list) and all(isinstance(v, np.ndarray) for v in vectors):
                    vectors = [v.tolist() for v in vectors]
                elif isinstance(vectors, list) and all(isinstance(v, float) for v in vectors):
                    vectors = [vectors]
                elif isinstance(vectors, str):
                    # If `vectors` is passed as a string, try to parse it as JSON
                    try:
                        vectors = json.loads(vectors)
                    except json.JSONDecodeError:
                        # If parsing fails, raise an error
                        raise ValueError("Invalid format has used or failed to parse JSON for `vectors` parameter. Caused by: " + vectors)

            elif metadata is not None:
                # Instance normalization for metadata
                if not isinstance(metadata, list):
                    if isinstance(metadata, str):
                        # If `metadata` is passed as a string, try to parse it as JSON
                        try:
                            metadata = json.loads(metadata)
                        except json.JSONDecodeError:
                            # If parsing fails, wrap the string in a list
                            metadata = [metadata]
                    else:
                        # If `metadata` is not a list or string, wrap it in a list
                        metadata = [metadata]

                if vectors is None and self.embedding is not None:
                    vectors = self.embedding.get_embedding(metadata)

            return self.envector.call_insert(index_name=index_name, vectors=vectors, metadata=metadata)

        # ---------- MCP Tools: Insert Documents from Path ---------- #
        @self.mcp.tool(
            name="insert_documents_from_path",
            description=(
                "Insert long documents from the given path using enVector SDK. "
                "This tool read document in a directory, preprocess and chunk them, then embed and insert into enVector. "
                "This tool requires a path of documents to read and insert"
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_insert_documents_from_path(
            index_name: Annotated[str, Field(description="index name to insert data into")],
            document_path: Annotated[Union[Any, List[Any]], Field(description="documents path to insert")] = None,
            language: Annotated[Optional[str], Field(description="language of the documents for preprocessing and chunking")] = "DOCUMENT",
        ) -> Dict[str, Any]:
            """
            MCP tool to perform insert of documents using the enVector SDK adapter.
            """
            chunk_docs = self.preprocessor.preprocess_documents_from_path(path=document_path, language=language)
            text = [chunk["text"] for chunk in chunk_docs]
            metadata = [json.dumps(chunk) for chunk in chunk_docs]
            vectors = self.embedding.get_embedding(text)
            return self.envector.call_insert(index_name=index_name, vectors=vectors, metadata=metadata)

        # ---------- MCP Tools: Insert Documents from Texts ---------- #
        @self.mcp.tool(
            name="insert_documents_from_text",
            description=(
                "Insert long documents from the given texts using enVector SDK. "
                "This tool read document in a directory, preprocess and chunk them, then embed and insert into enVector. "
                "This tool requires a list of text documents loaded by LLMs to read and insert"
            ),
            annotations=ToolAnnotations(readOnlyHint=False, destructiveHint=False)
        )
        async def tool_insert_documents_from_text(
            index_name: Annotated[str, Field(description="index name to insert data into")],
            texts: Annotated[Union[Any, List[Any]], Field(description="document text to insert")] = None,
        ) -> Dict[str, Any]:
            """
            MCP tool to perform insert of documents using the enVector SDK adapter.

            """
            chunk_docs = self.preprocessor.preprocess_document_from_text(texts=texts)
            text = [chunk["text"] for chunk in chunk_docs]
            metadata = [json.dumps(chunk) for chunk in chunk_docs]
            vectors = self.embedding.get_embedding(text)
            return self.envector.call_insert(index_name=index_name, vectors=vectors, metadata=metadata)

        # ---------- MCP Tools: Search ---------- #
        @self.mcp.tool(
            name="search",
            description=(
                "Search your own encrypted vector data on enVector Cloud. "
                "The decryption key (secret key) is held locally by this MCP server runtime, "
                "so this tool is for indexes where the data owner is the current operator. "
                "Use 'remember' instead when accessing shared team memory where the "
                "decryption key is managed by a separate Vault server. "
                "Accepts text queries (auto-embedded), vector arrays, or JSON-encoded vectors."
            ),
            annotations=ToolAnnotations(readOnlyHint=True, destructiveHint=False)
        )
        async def tool_search(
            index_name: Annotated[str, Field(description="index name to search from")],
            query: Annotated[Any, Field(description="search query: natural language text, vector (list of floats), or JSON-encoded vector")],
            topk: Annotated[int, Field(description="number of top-k results to return")],
        ) -> Dict[str, Any]:
            """
            MCP tool to perform search using the enVector SDK adapter.
            Call the adapter's call_search method.

            Args:
                index_name (str): The name of the index to search.
                query (Union[List[float], List[List[float]]]): The search query.
                topk (int): The number of top results to return.

            Returns:
                Dict[str, Any]: The search results from the enVector SDK adapter.
            """
            try:
                preprocessed_query = _preprocess(query)
            except ValueError as exc:
                raise ToolError(f"Invalid query parameter: {exc}") from exc
            return self.envector.call_search(index_name=index_name, query=preprocessed_query, topk=topk)

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
        "--envector-address",
        default=os.getenv("ENVECTOR_ADDRESS", None),
        help="enVector endpoint address (host:port or URL).",
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
    ENVECTOR_ADDRESS = args.envector_address or ""
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
                if not ENVECTOR_ADDRESS and _ev_cfg.get("endpoint"):
                    ENVECTOR_ADDRESS = _ev_cfg["endpoint"]
                    logger.info(f"Loaded ENVECTOR_ADDRESS from config: {ENVECTOR_ADDRESS}")
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
        success, vault_index = fetch_keys_from_vault(RUNEVAULT_ENDPOINT, RUNEVAULT_TOKEN, ENVECTOR_KEY_PATH)
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

    envector_adapter = EnVectorSDKAdapter(
        address=ENVECTOR_ADDRESS,
        key_id=ENVECTOR_KEY_ID,
        key_path=ENVECTOR_KEY_PATH,
        eval_mode=ENVECTOR_EVAL_MODE,
        query_encryption=ENCRYPTED_QUERY,
        access_token=ENVECTOR_API_KEY,
        auto_key_setup=AUTO_KEY_SETUP,
    )

    if args.embedding_model is not None:
        from adapter.embeddings import EmbeddingAdapter
        embedding_adapter = EmbeddingAdapter(
            mode=args.embedding_mode,
            model_name=args.embedding_model
        )
    else:
        embedding_adapter = None

    document_preprocessor = DocumentPreprocessingAdapter()

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

    app = MCPServerApp(
        mcp_server_name=MCP_SERVER_NAME,
        envector_adapter=envector_adapter,
        embedding_adapter=embedding_adapter,
        document_preprocessor=document_preprocessor,
        vault_client=vault_client,
        vault_configured=VAULT_CONFIGURED,
        vault_keys_loaded=VAULT_KEYS_LOADED,
        vault_index_name=VAULT_INDEX_NAME,
    )

    def _handle_shutdown(signum, frame):
        raise SystemExit(0)
    for sig in (signal.SIGINT, getattr(signal, "SIGTERM", None)):
        if sig is not None:
            signal.signal(sig, _handle_shutdown)

    app.run()
