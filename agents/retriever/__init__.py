"""
Retriever Agent - Organizational Context Retrieval

Searches organizational memory and synthesizes answers using LLM.

Key Components:
- QueryProcessor: Parses and expands user queries
- Searcher: Searches enVector for relevant context
- Synthesizer: LLM-based answer synthesis from payload.text

Pipeline:
1. Parse user query (intent, entities, time scope)
2. Search enVector for relevant Decision Records
3. Extract payload.text from results
4. Synthesize answer with LLM (respecting certainty levels)
"""

from .query_processor import QueryProcessor, ParsedQuery
from .searcher import Searcher, SearchResult
from .synthesizer import Synthesizer, SynthesizedAnswer

__all__ = [
    "QueryProcessor",
    "ParsedQuery",
    "Searcher",
    "SearchResult",
    "Synthesizer",
    "SynthesizedAnswer",
]
