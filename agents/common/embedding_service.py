"""
Embedding Service

Wraps the existing EmbeddingAdapter from envector-mcp-server.
Provides on-device embedding generation using fastembed.
"""

import sys
from pathlib import Path
from typing import List, Optional
import numpy as np

# Add envector-mcp-server to path
MCP_SERVER_PATH = Path(__file__).parent.parent.parent / "mcp" / "envector-mcp-server" / "srcs"
if str(MCP_SERVER_PATH) not in sys.path:
    sys.path.insert(0, str(MCP_SERVER_PATH))


class EmbeddingService:
    """
    Singleton embedding service for Rune agents.

    Uses fastembed by default for on-device embedding generation.
    This avoids external API calls and keeps data local.
    """

    _instance: Optional["EmbeddingService"] = None
    _adapter = None

    def __new__(cls, mode: str = "femb", model: str = "sentence-transformers/all-MiniLM-L6-v2"):
        if cls._instance is None:
            cls._instance = super().__new__(cls)
            cls._instance._init_adapter(mode, model)
        return cls._instance

    def _init_adapter(self, mode: str, model: str) -> None:
        """Initialize the underlying EmbeddingAdapter"""
        try:
            from adapter.embeddings import EmbeddingAdapter
            self._adapter = EmbeddingAdapter(mode=mode, model_name=model)
            self._mode = mode
            self._model = model
            print(f"[EmbeddingService] Initialized with mode={mode}, model={model}")
        except ImportError as e:
            print(f"[EmbeddingService] Warning: Could not import EmbeddingAdapter: {e}")
            print("[EmbeddingService] Using fallback mode (no embeddings)")
            self._adapter = None

    @property
    def is_available(self) -> bool:
        """Check if embedding service is available"""
        return self._adapter is not None

    def embed(self, texts: List[str]) -> List[List[float]]:
        """
        Generate embeddings for a list of texts.

        Args:
            texts: List of strings to embed

        Returns:
            List of embedding vectors (L2 normalized)
        """
        if not self._adapter:
            raise RuntimeError("EmbeddingAdapter not initialized")

        if not texts:
            return []

        embeddings = self._adapter.get_embedding(texts)

        # Ensure consistent return type
        if isinstance(embeddings, np.ndarray):
            return embeddings.tolist()
        return embeddings

    def embed_single(self, text: str) -> List[float]:
        """
        Generate embedding for a single text.

        Args:
            text: String to embed

        Returns:
            Embedding vector (L2 normalized)
        """
        if not text:
            raise ValueError("Cannot embed empty text")

        embeddings = self.embed([text])
        return embeddings[0]

    def cosine_similarity(self, vec1: List[float], vec2: List[float]) -> float:
        """
        Compute cosine similarity between two vectors.

        Note: EmbeddingAdapter already L2 normalizes vectors,
        so dot product equals cosine similarity.

        Args:
            vec1: First embedding vector
            vec2: Second embedding vector

        Returns:
            Cosine similarity score (0.0 to 1.0)
        """
        v1 = np.array(vec1)
        v2 = np.array(vec2)

        # Handle potential dimension mismatch
        if v1.shape != v2.shape:
            raise ValueError(f"Vector dimension mismatch: {v1.shape} vs {v2.shape}")

        # For normalized vectors, dot product = cosine similarity
        similarity = float(np.dot(v1, v2))

        # Clamp to valid range (numerical precision issues)
        return max(0.0, min(1.0, similarity))

    def batch_cosine_similarity(
        self,
        query_vec: List[float],
        vectors: List[List[float]]
    ) -> List[float]:
        """
        Compute cosine similarity between a query and multiple vectors.

        Args:
            query_vec: Query embedding vector
            vectors: List of embedding vectors to compare against

        Returns:
            List of similarity scores
        """
        if not vectors:
            return []

        query = np.array(query_vec)
        matrix = np.array(vectors)

        # Matrix multiplication for batch similarity
        similarities = np.dot(matrix, query)

        # Clamp to valid range
        similarities = np.clip(similarities, 0.0, 1.0)

        return similarities.tolist()


# Module-level singleton getter
_service_instance: Optional[EmbeddingService] = None


def get_embedding_service(
    mode: str = "femb",
    model: str = "sentence-transformers/all-MiniLM-L6-v2"
) -> EmbeddingService:
    """
    Get the singleton EmbeddingService instance.

    Args:
        mode: Embedding mode (femb, sbert, hf, openai)
        model: Model name

    Returns:
        EmbeddingService instance
    """
    global _service_instance

    if _service_instance is None:
        _service_instance = EmbeddingService(mode=mode, model=model)

    return _service_instance
