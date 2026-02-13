"""
Query Processor

Parses and analyzes user queries to understand intent and extract entities.
Uses patterns from retrieval-patterns.md for intent classification.
Supports multilingual queries via LLM-based intent classification + translation.
"""

import json
import re
from typing import List, Optional
from dataclasses import dataclass, field
from enum import Enum

from ..common.language import LanguageInfo, detect_language


class QueryIntent(str, Enum):
    """Types of query intent"""
    DECISION_RATIONALE = "decision_rationale"  # "Why did we choose X?"
    FEATURE_HISTORY = "feature_history"  # "Have customers asked for X?"
    PATTERN_LOOKUP = "pattern_lookup"  # "How do we handle X?"
    TECHNICAL_CONTEXT = "technical_context"  # "What's our architecture for X?"
    SECURITY_COMPLIANCE = "security_compliance"  # "What are the security requirements?"
    HISTORICAL_CONTEXT = "historical_context"  # "When did we decide X?"
    ATTRIBUTION = "attribution"  # "Who decided on X?"
    GENERAL = "general"  # Catch-all


class TimeScope(str, Enum):
    """Time scope for queries"""
    LAST_WEEK = "last_week"
    LAST_MONTH = "last_month"
    LAST_QUARTER = "last_quarter"
    LAST_YEAR = "last_year"
    ALL_TIME = "all_time"


@dataclass
class ParsedQuery:
    """Parsed representation of a user query"""
    original: str
    cleaned: str
    intent: QueryIntent
    time_scope: TimeScope = TimeScope.ALL_TIME
    entities: List[str] = field(default_factory=list)
    keywords: List[str] = field(default_factory=list)
    expanded_queries: List[str] = field(default_factory=list)
    language: Optional[LanguageInfo] = None


class QueryProcessor:
    """
    Processes user queries for organizational memory search.

    Responsibilities:
    1. Clean and normalize query text
    2. Detect query intent (why, how, what, when, who)
    3. Extract entities and keywords
    4. Determine time scope
    5. Generate query expansions for better recall
    """

    # Intent detection patterns (from retrieval-patterns.md)
    INTENT_PATTERNS = {
        QueryIntent.DECISION_RATIONALE: [
            r"why did we (choose|decide|go with|select|pick)",
            r"what was the (reasoning|rationale|logic|thinking)",
            r"why .+ over .+",
            r"what were the (reasons|factors)",
            r"why (not|didn't we)",
            r"reasoning behind",
        ],
        QueryIntent.FEATURE_HISTORY: [
            r"(have|did) (customers?|users?) (asked|requested|wanted)",
            r"feature request",
            r"why did we (reject|say no|decline)",
            r"(how many|which) customers",
            r"customer feedback (on|about)",
        ],
        QueryIntent.PATTERN_LOOKUP: [
            r"how do we (handle|deal with|approach|manage)",
            r"what'?s our (approach|process|standard|convention)",
            r"is there (an?|existing) (pattern|standard|convention)",
            r"what'?s the (best practice|recommended way)",
            r"how should (we|I)",
        ],
        QueryIntent.TECHNICAL_CONTEXT: [
            r"what'?s our (architecture|design|system) for",
            r"how (does|is) .+ (implemented|built|designed)",
            r"(explain|describe) (the|our) .+ (system|architecture|design)",
            r"technical (details|overview) (of|for)",
        ],
        QueryIntent.SECURITY_COMPLIANCE: [
            r"(security|compliance) (requirements?|considerations?)",
            r"what (security|privacy) (measures|controls)",
            r"(gdpr|hipaa|sox|pci) (requirements?|compliance)",
            r"audit (requirements?|trail)",
        ],
        QueryIntent.HISTORICAL_CONTEXT: [
            r"when did we (decide|choose|implement|launch)",
            r"(history|timeline) of",
            r"(have|did) we (ever|previously)",
            r"how long (have|has) .+ been",
        ],
        QueryIntent.ATTRIBUTION: [
            r"who (decided|chose|approved|owns)",
            r"which (team|person|group) (is responsible|decided|owns)",
            r"(owner|maintainer) of",
        ],
    }

    # Time scope patterns
    TIME_PATTERNS = {
        TimeScope.LAST_WEEK: [r"last week", r"this week", r"past week", r"7 days"],
        TimeScope.LAST_MONTH: [r"last month", r"this month", r"past month", r"30 days"],
        TimeScope.LAST_QUARTER: [r"last quarter", r"this quarter", r"Q[1-4]", r"past 3 months"],
        TimeScope.LAST_YEAR: [r"last year", r"this year", r"20\d{2}", r"past year"],
    }

    # Stop words to filter from keywords
    STOP_WORDS = {
        "the", "a", "an", "is", "are", "was", "were", "be", "been", "being",
        "have", "has", "had", "do", "does", "did", "will", "would", "could",
        "should", "may", "might", "must", "shall", "can", "need", "dare",
        "ought", "used", "to", "of", "in", "for", "on", "with", "at", "by",
        "from", "up", "about", "into", "over", "after", "we", "our", "us",
        "i", "me", "my", "you", "your", "it", "its", "they", "them", "their",
        "this", "that", "these", "those", "what", "which", "who", "whom",
        "when", "where", "why", "how", "and", "or", "but", "if", "because",
        "as", "until", "while", "although", "though", "even", "just", "also",
    }

    # LLM prompt for multilingual query parsing
    QUERY_PARSE_PROMPT = """Analyze this user query and extract structured information.
The query may be in any language. Translate all outputs to English.

Respond with a valid JSON object:
{{
    "intent": one of ["decision_rationale", "feature_history", "pattern_lookup", "technical_context", "security_compliance", "historical_context", "attribution", "general"],
    "english_query": "the query translated to English",
    "entities": ["list", "of", "named", "entities"],
    "keywords": ["important", "keywords", "in", "english"],
    "time_scope": one of ["last_week", "last_month", "last_quarter", "last_year", "all_time"]
}}

Query: {query}

JSON:"""

    def __init__(
        self,
        anthropic_api_key: Optional[str] = None,
        model: str = "claude-sonnet-4-20250514",
    ):
        """Initialize query processor.

        Args:
            anthropic_api_key: Optional API key for LLM-based multilingual parsing
            model: Anthropic model to use
        """
        self._llm_client = None
        self._model = model

        if anthropic_api_key:
            try:
                import anthropic
                self._llm_client = anthropic.Anthropic(api_key=anthropic_api_key)
            except ImportError:
                pass
            except Exception:
                pass

    def parse(self, query: str) -> ParsedQuery:
        """
        Parse a user query into structured form.

        Args:
            query: Raw user query string

        Returns:
            ParsedQuery with intent, entities, and expansions
        """
        language = detect_language(query)

        if language.is_english or not self._llm_client:
            # English path: existing regex (unchanged)
            return self._parse_english(query, language)
        else:
            # Non-English path: LLM classification + translation
            return self._parse_multilingual(query, language)

    def _parse_english(self, query: str, language: Optional[LanguageInfo] = None) -> ParsedQuery:
        """Parse English query using regex patterns (original logic)."""
        # Clean query
        cleaned = self._clean_query(query)

        # Detect intent
        intent = self._detect_intent(cleaned)

        # Detect time scope
        time_scope = self._detect_time_scope(cleaned)

        # Extract entities
        entities = self._extract_entities(query)

        # Extract keywords
        keywords = self._extract_keywords(cleaned)

        # Generate query expansions
        expanded = self._generate_expansions(cleaned, intent, entities)

        return ParsedQuery(
            original=query,
            cleaned=cleaned,
            intent=intent,
            time_scope=time_scope,
            entities=entities,
            keywords=keywords,
            expanded_queries=expanded,
            language=language,
        )

    def _parse_multilingual(self, query: str, language: LanguageInfo) -> ParsedQuery:
        """Parse non-English query using LLM for intent classification + translation."""
        try:
            prompt = self.QUERY_PARSE_PROMPT.format(query=query)
            response = self._llm_client.messages.create(
                model=self._model,
                max_tokens=256,
                messages=[{"role": "user", "content": prompt}],
            )
            raw = response.content[0].text.strip()
            result = self._parse_llm_query_response(raw)

            # Map intent string to enum
            intent_map = {v.value: v for v in QueryIntent}
            intent = intent_map.get(result.get("intent", ""), QueryIntent.GENERAL)

            # Map time_scope string to enum
            scope_map = {v.value: v for v in TimeScope}
            time_scope = scope_map.get(result.get("time_scope", ""), TimeScope.ALL_TIME)

            english_query = result.get("english_query", query)

            # expanded_queries: original + English translation (both searched)
            expanded = [query, english_query]
            # Add intent-based expansions on the English translation
            english_expansions = self._generate_expansions(
                english_query.lower(), intent, result.get("entities", [])
            )
            for exp in english_expansions:
                if exp not in expanded:
                    expanded.append(exp)

            return ParsedQuery(
                original=query,
                cleaned=query,
                intent=intent,
                time_scope=time_scope,
                entities=result.get("entities", []),
                keywords=result.get("keywords", []),
                expanded_queries=expanded[:7],
                language=language,
            )
        except Exception as e:
            print(f"[QueryProcessor] LLM parsing failed: {e}")
            # Fallback to regex parsing
            return self._parse_english(query, language)

    def _parse_llm_query_response(self, raw: str) -> dict:
        """Parse LLM JSON response for query analysis."""
        if raw.startswith("```"):
            lines = raw.split("\n")
            lines = [l for l in lines if not l.strip().startswith("```")]
            raw = "\n".join(lines)

        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            start = raw.find("{")
            end = raw.rfind("}") + 1
            if start >= 0 and end > start:
                try:
                    return json.loads(raw[start:end])
                except json.JSONDecodeError:
                    return {}
            return {}

    def _clean_query(self, query: str) -> str:
        """Clean and normalize query text"""
        # Lowercase
        cleaned = query.lower().strip()

        # Remove extra whitespace
        cleaned = re.sub(r'\s+', ' ', cleaned)

        # Remove trailing punctuation (but keep question marks)
        cleaned = re.sub(r'[.!,;:]+$', '', cleaned)

        return cleaned

    def _detect_intent(self, query: str) -> QueryIntent:
        """Detect the primary intent of the query"""
        query_lower = query.lower()

        for intent, patterns in self.INTENT_PATTERNS.items():
            for pattern in patterns:
                if re.search(pattern, query_lower, re.IGNORECASE):
                    return intent

        return QueryIntent.GENERAL

    def _detect_time_scope(self, query: str) -> TimeScope:
        """Detect time scope from query"""
        query_lower = query.lower()

        for scope, patterns in self.TIME_PATTERNS.items():
            for pattern in patterns:
                if re.search(pattern, query_lower, re.IGNORECASE):
                    return scope

        return TimeScope.ALL_TIME

    def _extract_entities(self, query: str) -> List[str]:
        """Extract named entities from query"""
        entities = []

        # Extract quoted strings
        quoted = re.findall(r'"([^"]+)"|\'([^\']+)\'', query)
        for q in quoted:
            entity = q[0] or q[1]
            if entity and len(entity) > 1:
                entities.append(entity)

        # Extract capitalized words/phrases (potential proper nouns)
        # But not at the start of sentences
        words = query.split()
        for i, word in enumerate(words):
            if i > 0 and word[0].isupper() and len(word) > 1:
                # Check if it's a multi-word entity
                phrase = [word]
                j = i + 1
                while j < len(words) and words[j][0].isupper():
                    phrase.append(words[j])
                    j += 1
                entity = ' '.join(phrase)
                if entity not in entities:
                    entities.append(entity)

        # Extract technology names (common patterns)
        tech_patterns = [
            r'\b(PostgreSQL|MySQL|MongoDB|Redis|Elasticsearch|Kafka)\b',
            r'\b(React|Vue|Angular|Next\.js|Node\.js|Python|Java|Go)\b',
            r'\b(AWS|GCP|Azure|Kubernetes|Docker|Terraform)\b',
            r'\b(REST|GraphQL|gRPC|WebSocket|HTTP|HTTPS)\b',
        ]
        for pattern in tech_patterns:
            matches = re.findall(pattern, query, re.IGNORECASE)
            entities.extend(matches)

        # Deduplicate and return
        return list(dict.fromkeys(entities))[:10]

    def _extract_keywords(self, query: str) -> List[str]:
        """Extract important keywords from query"""
        # Split into words
        words = re.findall(r'\b\w+\b', query.lower())

        # Filter stop words and short words
        keywords = [
            w for w in words
            if w not in self.STOP_WORDS and len(w) > 2
        ]

        # Deduplicate and return
        return list(dict.fromkeys(keywords))[:15]

    def _generate_expansions(
        self,
        query: str,
        intent: QueryIntent,
        entities: List[str]
    ) -> List[str]:
        """Generate query expansions for better recall"""
        expansions = [query]  # Include original

        # Intent-based expansions
        if intent == QueryIntent.DECISION_RATIONALE:
            expansions.extend([
                f"decision {query}",
                f"rationale {query}",
                f"trade-off {query}",
            ])
        elif intent == QueryIntent.FEATURE_HISTORY:
            expansions.extend([
                f"customer request {query}",
                f"feature rejected {query}",
            ])
        elif intent == QueryIntent.PATTERN_LOOKUP:
            expansions.extend([
                f"standard approach {query}",
                f"best practice {query}",
            ])
        elif intent == QueryIntent.TECHNICAL_CONTEXT:
            expansions.extend([
                f"architecture {query}",
                f"implementation {query}",
            ])

        # Entity-based expansions
        for entity in entities[:3]:
            expansions.append(f"{entity} decision")
            expansions.append(f"why {entity}")

        # Deduplicate and limit
        seen = set()
        unique = []
        for exp in expansions:
            if exp.lower() not in seen:
                seen.add(exp.lower())
                unique.append(exp)

        return unique[:5]

    def format_for_search(self, parsed: ParsedQuery) -> str:
        """
        Format parsed query for enVector search.

        Combines original query with key entities and keywords
        for better semantic matching.
        """
        parts = [parsed.cleaned]

        # Add entities
        if parsed.entities:
            parts.append("entities: " + ", ".join(parsed.entities[:3]))

        # Add keywords
        if parsed.keywords:
            parts.append("keywords: " + ", ".join(parsed.keywords[:5]))

        return " | ".join(parts)
