"""
Retriever Agent - Organizational Context Retrieval

Searches organizational memory and returns raw results for the main agent to synthesize.

Key Components:
- QueryProcessor: Parses and expands user queries
- Searcher: Searches enVector for relevant context

Pipeline:
1. Parse user query (intent, entities, time scope)
2. Search enVector for relevant Decision Records
3. Return raw results with payload.text (main agent synthesizes)
"""

from .query_processor import QueryProcessor, ParsedQuery
from .searcher import Searcher, SearchResult

__all__ = [
    "QueryProcessor",
    "ParsedQuery",
    "Searcher",
    "SearchResult",
]
