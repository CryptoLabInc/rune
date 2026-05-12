"""
EnVector Client

Wraps EnVectorSDKAdapter for direct access to enVector operations.
Avoids MCP protocol overhead by importing adapters directly.
"""

import json
import logging
import os
import sys
from pathlib import Path
from typing import List, Dict, Any, Optional

logger = logging.getLogger("rune.common.envector")

# Add mcp/ to path so `from adapter import ...` works
MCP_ROOT = Path(__file__).parent.parent.parent / "mcp"
if str(MCP_ROOT) not in sys.path:
    sys.path.insert(0, str(MCP_ROOT))


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
        key_id: str = None,
        access_token: Optional[str] = None,
        secure: Optional[bool] = None,
        auto_key_setup: bool = True,
        agent_id: Optional[str] = None,
        agent_dek: Optional[bytes] = None,
        eval_mode: str = "mm32",
        index_type: str = "ivf_vct",
    ):
        """
        Initialize EnVector client.

        Args:
            address: enVector server address (host:port or cloud URL)
            key_path: Path to store/load encryption keys
            key_id: Key identifier
            access_token: Cloud access token (for enVector Cloud)
            secure: TLS toggle for pyenvector 1.4 (None = SDK default)
            auto_key_setup: Auto-generate keys if not found
            agent_id: Per-agent identifier for app-layer metadata encryption
            agent_dek: Per-agent AES-256 DEK (32 bytes) from Vault
            eval_mode: FHE evaluation mode ("mm32" or "rmp"); overridable via ENVECTOR_EVAL_MODE
            index_type: Index structure type ("ivf_vct" or "flat")
        """
        self._address = address
        self._key_path = Path(key_path).expanduser()
        self._key_id = key_id
        self._access_token = access_token
        self._secure = secure
        self._auto_key_setup = auto_key_setup
        self._agent_id = agent_id
        self._agent_dek = agent_dek
        self._eval_mode = eval_mode
        self._index_type = index_type
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
                eval_mode=os.getenv("ENVECTOR_EVAL_MODE", self._eval_mode),
                query_encryption="plain",  # Plain queries for simplicity
                access_token=self._access_token,
                secure=self._secure,
                auto_key_setup=self._auto_key_setup,
                agent_id=self._agent_id,
                agent_dek=self._agent_dek,
                index_type=self._index_type,
            )
            self._initialized = True
            logger.info("Connected to %s", self._address)

        except ImportError as e:
            logger.warning("Could not import EnVectorSDKAdapter: %s", e)
            raise RuntimeError(f"EnVectorSDKAdapter not available: {e}")
        except Exception as e:
            logger.error("Error initializing: %s", e)
            raise

    @property
    def is_available(self) -> bool:
        """Check if client is available"""
        try:
            self._ensure_initialized()
            return self._adapter is not None
        except Exception:
            return False

    def get_index_list(self) -> Dict[str, Any]:
        """Get list of all indexes"""
        self._ensure_initialized()
        return self._adapter.call_get_index_list()

    def insert(
        self,
        index_name: str,
        vectors: List[List[float]],
        metadata: Optional[List[Dict]] = None,
        await_searchable: bool = False,
        use_row_insert: bool = False,
    ) -> Dict[str, Any]:
        """
        Insert vectors into an index.

        Args:
            index_name: Target index name
            vectors: List of embedding vectors
            metadata: Optional list of metadata dicts (one per vector)
            await_searchable: If True, block until data is searchable (MERGED_SAVED)
            use_row_insert: If True, use single-row insert API path (len(vectors) must be 1)

        Returns:
            Result dict with ok/error status
        """
        self._ensure_initialized()

        if use_row_insert and len(vectors) != 1:
            raise ValueError(f"use_row_insert=True requires exactly 1 vector, got {len(vectors)}")

        if metadata:
            meta_list = [
                json.dumps(m) if isinstance(m, dict) else str(m)
                for m in metadata
            ]
        else:
            meta_list = [json.dumps({"index": i}) for i in range(len(vectors))]

        return self._adapter.call_insert(
            index_name=index_name,
            vectors=vectors,
            metadata=meta_list,
            await_searchable=await_searchable,
            use_row_insert=use_row_insert,
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

    def score(
        self,
        index_name: str,
        query_vector: List[float],
    ) -> Dict[str, Any]:
        """
        Encrypted similarity scoring (Vault-secured pipeline step 1).

        Returns encrypted score blobs for Vault decryption.

        Args:
            index_name: Index to score against
            query_vector: Query embedding vector

        Returns:
            Result dict with encrypted_blobs list
        """
        self._ensure_initialized()
        return self._adapter.call_score(
            index_name=index_name,
            query=[query_vector],
        )

    def remind(
        self,
        index_name: str,
        indices: List[Dict[str, Any]],
        output_fields: Optional[List[str]] = None,
    ) -> Dict[str, Any]:
        """
        Retrieve metadata for indices returned by Vault (Vault-secured pipeline step 3).

        Args:
            index_name: Index to fetch metadata from
            indices: List of dicts with shard_idx, row_idx, score
            output_fields: Fields to include (default: ["metadata"])

        Returns:
            Result dict with metadata entries
        """
        self._ensure_initialized()
        return self._adapter.call_remind(
            index_name=index_name,
            indices=indices,
            output_fields=output_fields,
        )

