"""
Pattern Cache

Pre-embeds trigger patterns from capture-triggers.md at startup.
Used for on-device similarity-based decision detection.
"""

from typing import List, Dict, Tuple, Optional
from dataclasses import dataclass
import numpy as np

from .embedding_service import EmbeddingService


@dataclass
class PatternEntry:
    """A single trigger pattern with its embedding"""
    text: str
    category: str
    priority: str  # "high", "medium", "low"
    embedding: List[float]
    domain: Optional[str] = None


class PatternCache:
    """
    Cache of pre-embedded trigger patterns.

    At startup, loads patterns from capture-triggers.md,
    embeds them all, and stores for fast similarity lookup.
    """

    def __init__(self, embedding_service: EmbeddingService):
        """
        Initialize pattern cache.

        Args:
            embedding_service: EmbeddingService instance for generating embeddings
        """
        self._embedding = embedding_service
        self._patterns: List[PatternEntry] = []
        self._embeddings_matrix: Optional[np.ndarray] = None
        self._loaded = False

    @property
    def is_loaded(self) -> bool:
        """Check if patterns are loaded"""
        return self._loaded

    @property
    def pattern_count(self) -> int:
        """Number of loaded patterns"""
        return len(self._patterns)

    def load_patterns(self, patterns: List[Dict]) -> int:
        """
        Load and embed patterns.

        Args:
            patterns: List of pattern dicts with keys:
                - text: Pattern text
                - category: Category name
                - priority: "high", "medium", or "low"
                - domain: Optional domain classification

        Returns:
            Number of patterns loaded
        """
        if not patterns:
            print("[PatternCache] Warning: No patterns to load")
            return 0

        # Extract texts for batch embedding
        texts = [p["text"] for p in patterns]

        print(f"[PatternCache] Embedding {len(texts)} patterns...")
        embeddings = self._embedding.embed(texts)

        # Create PatternEntry objects
        self._patterns = [
            PatternEntry(
                text=p["text"],
                category=p.get("category", "general"),
                priority=p.get("priority", "medium"),
                domain=p.get("domain"),
                embedding=embeddings[i]
            )
            for i, p in enumerate(patterns)
        ]

        # Create embeddings matrix for fast batch similarity
        self._embeddings_matrix = np.array([p.embedding for p in self._patterns])
        self._loaded = True

        print(f"[PatternCache] Loaded {len(self._patterns)} patterns")
        return len(self._patterns)

    def find_best_match(
        self,
        text: str,
        threshold: float = 0.7
    ) -> Tuple[Optional[PatternEntry], float]:
        """
        Find the best matching pattern for input text.

        Args:
            text: Input text to match
            threshold: Minimum similarity threshold

        Returns:
            Tuple of (best_match, score) or (None, best_score) if below threshold
        """
        if not self._loaded:
            raise RuntimeError("Patterns not loaded. Call load_patterns() first.")

        if not text.strip():
            return (None, 0.0)

        # Embed input text
        text_embedding = self._embedding.embed_single(text)
        text_vec = np.array(text_embedding)

        # Compute similarities with all patterns (batch operation)
        similarities = np.dot(self._embeddings_matrix, text_vec)

        # Find best match
        best_idx = int(np.argmax(similarities))
        best_score = float(similarities[best_idx])

        if best_score >= threshold:
            return (self._patterns[best_idx], best_score)

        return (None, best_score)

    def find_top_matches(
        self,
        text: str,
        top_k: int = 5,
        threshold: float = 0.5
    ) -> List[Tuple[PatternEntry, float]]:
        """
        Find top-k matching patterns for input text.

        Args:
            text: Input text to match
            top_k: Number of top matches to return
            threshold: Minimum similarity threshold

        Returns:
            List of (pattern, score) tuples, sorted by score descending
        """
        if not self._loaded:
            raise RuntimeError("Patterns not loaded. Call load_patterns() first.")

        if not text.strip():
            return []

        # Embed input text
        text_embedding = self._embedding.embed_single(text)
        text_vec = np.array(text_embedding)

        # Compute similarities with all patterns
        similarities = np.dot(self._embeddings_matrix, text_vec)

        # Get top-k indices
        if len(similarities) <= top_k:
            top_indices = np.argsort(similarities)[::-1]
        else:
            top_indices = np.argpartition(similarities, -top_k)[-top_k:]
            top_indices = top_indices[np.argsort(similarities[top_indices])[::-1]]

        # Filter by threshold and create results
        results = []
        for idx in top_indices:
            score = float(similarities[idx])
            if score >= threshold:
                results.append((self._patterns[idx], score))

        return results

    def get_patterns_by_category(self, category: str) -> List[PatternEntry]:
        """Get all patterns in a category"""
        return [p for p in self._patterns if p.category.lower() == category.lower()]

    def get_patterns_by_priority(self, priority: str) -> List[PatternEntry]:
        """Get all patterns with given priority"""
        return [p for p in self._patterns if p.priority.lower() == priority.lower()]

    def get_high_priority_patterns(self) -> List[PatternEntry]:
        """Get all high-priority patterns"""
        return self.get_patterns_by_priority("high")

    def categories(self) -> List[str]:
        """Get list of unique categories"""
        return list(set(p.category for p in self._patterns))

    def clear(self) -> None:
        """Clear all loaded patterns"""
        self._patterns = []
        self._embeddings_matrix = None
        self._loaded = False
