"""
Rune Agents

Working implementations of Scribe (context capture) and Retriever (context retrieval).

Philosophy:
- All memory is reproducible from payload.text (Markdown)
- Evidence-based reasoning: "Why" cannot be confirmed without quotes
- On-device similarity search for decision detection
- Text-only storage (no binary data)

Usage:
    from agents.common import load_config, EmbeddingService, PatternCache
    from agents.common.schemas import DecisionRecord, render_payload_text
    from agents.scribe import DecisionDetector, RecordBuilder
    from agents.retriever import Searcher, Synthesizer
"""

__version__ = "0.1.0"
