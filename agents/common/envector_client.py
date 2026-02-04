"""
EnVector Client

Wraps EnVectorSDKAdapter for direct access to enVector operations.
Avoids MCP protocol overhead by importing adapters directly.
"""

import sys
import json
from pathlib import Path
from typing import List, Dict, Any, Optional

# Add envector-mcp-server to path
MCP_SERVER_PATH = Path(__file__).parent.parent.parent / "mcp" / "envector-mcp-server" / "srcs"
if str(MCP_SERVER_PATH) not in sys.path:
    sys.path.insert(0, str(MCP_SERVER_PATH))


class EnVectorClient:
    """
    Direct client to enVector operations.

    Uses direct import of EnVectorSDKAdapter instead of MCP protocol
    for lower overhead when running on the same machine.
    """

    def __init__(
        self,
        address: str = "localhost:50050",
        key_path: str = "~/.rune/keys",
        key_id: str = "rune_key",
        access_token: Optional[str] = None,
        auto_key_setup: bool = True,
    ):
        """
        Initialize EnVector client.

        Args:
            address: enVector server address (host:port or cloud URL)
            key_path: Path to store/load encryption keys
            key_id: Key identifier
            access_token: Cloud access token (for enVector Cloud)
            auto_key_setup: Auto-generate keys if not found
        """
        self._address = address
        self._key_path = Path(key_path).expanduser()
        self._key_id = key_id
        self._access_token = access_token
        self._auto_key_setup = auto_key_setup
        self._adapter = None
        self._initialized = False

    def _ensure_initialized(self) -> None:
        """Lazily initialize the adapter"""
        if self._initialized:
            return

        try:
            from adapter.envector_sdk import EnVectorSDKAdapter

            # Ensure key directory exists
            self._key_path.mkdir(parents=True, exist_ok=True)

            self._adapter = EnVectorSDKAdapter(
                address=self._address,
                key_id=self._key_id,
                key_path=str(self._key_path),
                eval_mode="rmp",
                query_encryption=False,  # Plain queries for simplicity
                access_token=self._access_token,
                auto_key_setup=self._auto_key_setup,
            )
            self._initialized = True
            print(f"[EnVectorClient] Connected to {self._address}")

        except ImportError as e:
            print(f"[EnVectorClient] Warning: Could not import EnVectorSDKAdapter: {e}")
            raise RuntimeError(f"EnVectorSDKAdapter not available: {e}")
        except Exception as e:
            print(f"[EnVectorClient] Error initializing: {e}")
            raise

    @property
    def is_available(self) -> bool:
        """Check if client is available"""
        try:
            self._ensure_initialized()
            return self._adapter is not None
        except Exception:
            return False

    def create_index(
        self,
        index_name: str,
        dim: int,
        index_type: str = "FLAT"
    ) -> Dict[str, Any]:
        """
        Create a new vector index.

        Args:
            index_name: Name of the index
            dim: Embedding dimension
            index_type: Index type (FLAT or IVF_FLAT)

        Returns:
            Result dict with ok/error status
        """
        self._ensure_initialized()

        index_params = {"index_type": index_type}
        if index_type == "IVF_FLAT":
            index_params["nlist"] = 100
            index_params["default_nprobe"] = 10

        return self._adapter.call_create_index(
            index_name=index_name,
            dim=dim,
            index_params=index_params
        )

    def get_index_list(self) -> Dict[str, Any]:
        """Get list of all indexes"""
        self._ensure_initialized()
        return self._adapter.call_get_index_list()

    def get_index_info(self, index_name: str) -> Dict[str, Any]:
        """Get info about a specific index"""
        self._ensure_initialized()
        return self._adapter.call_get_index_info(index_name=index_name)

    def insert(
        self,
        index_name: str,
        vectors: List[List[float]],
        metadata: Optional[List[Dict]] = None
    ) -> Dict[str, Any]:
        """
        Insert vectors into an index.

        Args:
            index_name: Target index name
            vectors: List of embedding vectors
            metadata: Optional list of metadata dicts (one per vector)

        Returns:
            Result dict with ok/error status
        """
        self._ensure_initialized()

        if metadata:
            # Serialize metadata to JSON strings
            meta_list = [
                json.dumps(m) if isinstance(m, dict) else str(m)
                for m in metadata
            ]
        else:
            meta_list = [json.dumps({"index": i}) for i in range(len(vectors))]

        return self._adapter.call_insert(
            index_name=index_name,
            vectors=vectors,
            metadata=meta_list
        )

    def insert_with_text(
        self,
        index_name: str,
        texts: List[str],
        embedding_service,
        metadata: Optional[List[Dict]] = None
    ) -> Dict[str, Any]:
        """
        Embed texts and insert into index.

        Args:
            index_name: Target index name
            texts: List of texts to embed
            embedding_service: EmbeddingService instance
            metadata: Optional list of metadata dicts

        Returns:
            Result dict with ok/error status
        """
        # Generate embeddings
        vectors = embedding_service.embed(texts)

        # Add text to metadata if not provided
        if metadata is None:
            metadata = [{"text": t} for t in texts]
        else:
            for i, meta in enumerate(metadata):
                if "text" not in meta:
                    meta["text"] = texts[i]

        return self.insert(index_name, vectors, metadata)

    def search(
        self,
        index_name: str,
        query_vector: List[float],
        topk: int = 10
    ) -> Dict[str, Any]:
        """
        Search for similar vectors.

        Args:
            index_name: Index to search
            query_vector: Query embedding vector
            topk: Number of results to return

        Returns:
            Result dict with results list
        """
        self._ensure_initialized()

        result = self._adapter.call_search(
            index_name=index_name,
            query=query_vector,
            topk=topk
        )

        return result

    def search_with_text(
        self,
        index_name: str,
        query_text: str,
        embedding_service,
        topk: int = 10
    ) -> Dict[str, Any]:
        """
        Embed query text and search.

        Args:
            index_name: Index to search
            query_text: Query text to embed
            embedding_service: EmbeddingService instance
            topk: Number of results to return

        Returns:
            Result dict with results list
        """
        query_vector = embedding_service.embed_single(query_text)
        return self.search(index_name, query_vector, topk)

    def parse_search_results(self, result: Dict[str, Any]) -> List[Dict[str, Any]]:
        """
        Parse search results into a cleaner format.

        Args:
            result: Raw result from search()

        Returns:
            List of parsed results with text, metadata, score
        """
        if not result.get("ok"):
            return []

        parsed = []
        raw_results = result.get("results", [])

        # Handle nested structure
        if isinstance(raw_results, list):
            for item in raw_results:
                if isinstance(item, dict):
                    metadata = item.get("metadata", {})

                    # Parse JSON metadata if string
                    if isinstance(metadata, str):
                        try:
                            metadata = json.loads(metadata)
                        except json.JSONDecodeError:
                            metadata = {"raw": metadata}

                    parsed.append({
                        "id": item.get("id"),
                        "distance": item.get("distance", 0),
                        "score": 1.0 - item.get("distance", 0),  # Convert distance to similarity
                        "metadata": metadata,
                        "text": metadata.get("text", ""),
                    })

        # Sort by score descending
        parsed.sort(key=lambda x: x["score"], reverse=True)

        return parsed
