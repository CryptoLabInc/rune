"""
Synthesizer

LLM-based answer synthesis from search results.
Uses payload.text from Decision Records to generate coherent answers.

Key principle: Respect certainty levels from evidence.
- "supported" → confident answer
- "partially_supported" → "likely" or "based on partial evidence"
- "unknown" → "uncertain" or "no clear evidence found"
"""

import logging
from typing import List, Optional, Dict, Any
from dataclasses import dataclass, field

from .searcher import SearchResult
from .query_processor import ParsedQuery

logger = logging.getLogger("rune.retriever.synthesizer")


@dataclass
class SynthesizedAnswer:
    """Synthesized answer from LLM"""
    answer: str
    confidence: float  # 0.0 to 1.0
    sources: List[Dict[str, Any]]
    related_queries: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)  # e.g., "based on uncertain evidence"


# Synthesis prompt template
SYNTHESIS_PROMPT = """You are an AI assistant that answers questions based on organizational decision records.

{language_instruction}

Your task is to synthesize an answer from the search results below. Follow these rules strictly:

1. ONLY use information from the provided records. Do NOT make up information.
2. Respect the certainty level of each record:
   - "supported": You can state this confidently
   - "partially_supported": Qualify with "likely" or "based on available evidence"
   - "unknown": State "uncertain" or "no clear evidence found"
3. Always cite sources by their record ID
4. If no relevant information is found, say "No relevant records found in organizational memory."
5. Be concise but complete.

User Question: {query}

Search Results (Decision Records):
{records}

Instructions:
- Synthesize a clear, direct answer to the question
- Cite record IDs in brackets like [dec_2024-01-01_arch_example]
- Note any uncertainty from records with "unknown" or "partially_supported" certainty
- Suggest follow-up queries if helpful

Your Answer:"""


# Fallback templates per language
FALLBACK_TEMPLATES = {
    "en": """## Search Results for: "{query}"

Found {count} relevant record(s):

{formatted_results}

---
**Note**: This is a direct listing without LLM synthesis.
Configure ANTHROPIC_API_KEY for natural language answers.
""",
    "ko": """## "{query}" 검색 결과

{count}개의 관련 레코드를 찾았습니다:

{formatted_results}

---
**참고**: LLM 합성 없이 직접 목록을 표시합니다.
자연어 답변을 위해 ANTHROPIC_API_KEY를 설정하세요.
""",
    "ja": """## "{query}" の検索結果

{count}件の関連レコードが見つかりました:

{formatted_results}

---
**注意**: LLM合成なしの直接リスティングです。
自然言語での回答にはANTHROPIC_API_KEYを設定してください。
""",
}

# Keep original for backward compatibility
FALLBACK_TEMPLATE = FALLBACK_TEMPLATES["en"]


class Synthesizer:
    """
    Synthesizes answers from search results using LLM.

    Falls back to simple formatting if LLM is not available.
    """

    def __init__(
        self,
        anthropic_api_key: Optional[str] = None,
        model: str = "claude-sonnet-4-20250514"
    ):
        """
        Initialize synthesizer.

        Args:
            anthropic_api_key: Anthropic API key (optional)
            model: Model to use for synthesis
        """
        self._api_key = anthropic_api_key
        self._model = model
        self._client = None

        if anthropic_api_key:
            try:
                import anthropic
                self._client = anthropic.Anthropic(api_key=anthropic_api_key)
            except ImportError:
                logger.warning("anthropic package not installed")
            except Exception as e:
                logger.warning("Failed to init Anthropic client: %s", e)

    @property
    def has_llm(self) -> bool:
        """Check if LLM is available"""
        return self._client is not None

    def synthesize(
        self,
        query: ParsedQuery,
        results: List[SearchResult]
    ) -> SynthesizedAnswer:
        """
        Synthesize an answer from search results.

        Args:
            query: Parsed user query
            results: Search results from Searcher

        Returns:
            SynthesizedAnswer with answer text and metadata
        """
        if not results:
            return SynthesizedAnswer(
                answer="No relevant records found in organizational memory.",
                confidence=0.0,
                sources=[],
                related_queries=self._suggest_alternatives(query),
                warnings=["No search results found"],
            )

        # Try LLM synthesis first
        if self.has_llm:
            try:
                return self._synthesize_with_llm(query, results)
            except Exception as e:
                logger.warning("LLM synthesis failed: %s", e)
                # Fall through to fallback

        # Fallback to simple formatting
        return self._synthesize_fallback(query, results)

    def _synthesize_with_llm(
        self,
        query: ParsedQuery,
        results: List[SearchResult]
    ) -> SynthesizedAnswer:
        """Synthesize using LLM"""
        # Format records for prompt
        records_text = self._format_records_for_prompt(results)

        # Determine language instruction
        if query.language and not query.language.is_english:
            language_instruction = (
                f"IMPORTANT: The user asked in {query.language.code}. "
                f"Respond in the SAME language ({query.language.code}). "
                f"The source records may be in English — translate relevant parts."
            )
        else:
            language_instruction = "Respond in English."

        # Build prompt
        prompt = SYNTHESIS_PROMPT.format(
            query=query.original,
            records=records_text,
            language_instruction=language_instruction,
        )

        # Call LLM
        response = self._client.messages.create(
            model=self._model,
            max_tokens=1024,
            messages=[
                {"role": "user", "content": prompt}
            ],
            timeout=30.0,
        )

        answer_text = response.content[0].text

        # Calculate confidence based on results
        confidence = self._calculate_confidence(results)

        # Extract sources
        sources = [
            {
                "record_id": r.record_id,
                "title": r.title,
                "domain": r.domain,
                "certainty": r.certainty,
                "score": r.score,
            }
            for r in results[:5]
        ]

        # Check for warnings
        warnings = []
        uncertain_count = sum(1 for r in results if r.certainty == "unknown")
        if uncertain_count > 0:
            warnings.append(f"{uncertain_count} record(s) have uncertain evidence")

        partial_count = sum(1 for r in results if r.certainty == "partially_supported")
        if partial_count > 0:
            warnings.append(f"{partial_count} record(s) have partial evidence")

        return SynthesizedAnswer(
            answer=answer_text,
            confidence=confidence,
            sources=sources,
            related_queries=self._suggest_followups(query, results),
            warnings=warnings,
        )

    def _synthesize_fallback(
        self,
        query: ParsedQuery,
        results: List[SearchResult]
    ) -> SynthesizedAnswer:
        """Fallback synthesis without LLM"""
        # Format results
        formatted_results = []
        for i, r in enumerate(results[:5], 1):
            certainty_marker = {
                "supported": "✓",
                "partially_supported": "~",
                "unknown": "?",
            }.get(r.certainty, "?")

            formatted_results.append(f"""
### {i}. {r.title} [{r.record_id}]
**Domain**: {r.domain} | **Certainty**: {certainty_marker} {r.certainty} | **Score**: {r.score:.2f}

{r.payload_text[:500]}{"..." if len(r.payload_text) > 500 else ""}
""")

        # Select language-specific fallback template
        lang_code = query.language.code if query.language else "en"
        template = FALLBACK_TEMPLATES.get(lang_code, FALLBACK_TEMPLATES["en"])

        answer = template.format(
            query=query.original,
            count=len(results),
            formatted_results="\n".join(formatted_results)
        )

        confidence = self._calculate_confidence(results)

        sources = [
            {
                "record_id": r.record_id,
                "title": r.title,
                "domain": r.domain,
                "certainty": r.certainty,
                "score": r.score,
            }
            for r in results[:5]
        ]

        return SynthesizedAnswer(
            answer=answer,
            confidence=confidence,
            sources=sources,
            related_queries=self._suggest_followups(query, results),
            warnings=["LLM not available - showing raw results"],
        )

    def _format_records_for_prompt(self, results: List[SearchResult]) -> str:
        """Format search results for LLM prompt, grouping linked records."""
        # Group results: linked groups together, standalone separate
        groups = []  # List of (group_id_or_none, group_type, [results])
        seen_groups = set()

        for r in results:
            if r.is_phase and r.group_id:
                if r.group_id not in seen_groups:
                    seen_groups.add(r.group_id)
                    chain = [x for x in results if x.group_id == r.group_id]
                    chain.sort(key=lambda x: x.phase_seq if x.phase_seq is not None else 0)
                    gtype = r.group_type or "phase_chain"
                    groups.append((r.group_id, gtype, chain))
            elif not r.is_phase:
                groups.append((None, None, [r]))

        formatted = []
        record_num = 1

        for group_id, group_type, group_results in groups:
            if group_id and len(group_results) > 1:
                first = group_results[0]
                if group_type == "bundle":
                    # Bundle — detail facets of a single decision
                    formatted.append(f"""
---
Record {record_num}: Decision Bundle [{group_id}]
Overall Domain: {first.domain}
Facets: {len(group_results)}
""")
                    for facet in group_results:
                        seq = (facet.phase_seq or 0) + 1
                        formatted.append(f"""
Facet {seq}: {facet.title} [{facet.record_id}]
Certainty: {facet.certainty}

{facet.payload_text[:800]}
""")
                else:
                    # Phase chain — sequential reasoning
                    formatted.append(f"""
---
Record {record_num}: Phase Chain [{group_id}]
Overall Domain: {first.domain}
Phases: {len(group_results)}
""")
                    for phase in group_results:
                        seq = (phase.phase_seq or 0) + 1
                        total = phase.phase_total or len(group_results)
                        formatted.append(f"""
Phase {seq}/{total}: {phase.title} [{phase.record_id}]
Certainty: {phase.certainty}

{phase.payload_text[:800]}
""")
                formatted.append("---\n")
                record_num += 1
            else:
                # Single record (standalone)
                r = group_results[0]
                formatted.append(f"""
---
Record {record_num}: [{r.record_id}]
Title: {r.title}
Domain: {r.domain}
Certainty: {r.certainty}
Relevance Score: {r.score:.2f}

Content:
{r.payload_text[:1000]}
---
""")
                record_num += 1

        return "\n".join(formatted)

    def _calculate_confidence(self, results: List[SearchResult]) -> float:
        """Calculate overall confidence from results"""
        if not results:
            return 0.0

        # Weights for certainty levels
        certainty_weights = {
            "supported": 1.0,
            "partially_supported": 0.6,
            "unknown": 0.3,
        }

        # Weighted average of top results
        total_weight = 0.0
        total_score = 0.0

        for i, r in enumerate(results[:5]):
            # Position weight (higher for top results)
            position_weight = 1.0 / (i + 1)

            # Certainty weight
            cert_weight = certainty_weights.get(r.certainty, 0.3)

            # Combined weight
            weight = position_weight * cert_weight * r.score
            total_weight += weight
            total_score += weight

        if total_weight == 0:
            return 0.0

        # Normalize to 0-1 range
        confidence = min(1.0, total_score / 2.0)  # Divide by 2 for reasonable scaling

        return round(confidence, 2)

    def _suggest_alternatives(self, query: ParsedQuery) -> List[str]:
        """Suggest alternative queries when no results found"""
        suggestions = []

        # Broader search suggestions
        if query.entities:
            for entity in query.entities[:2]:
                suggestions.append(f"Tell me about {entity}")

        # By intent
        if query.intent.value != "general":
            suggestions.append(f"What decisions have we made about {' '.join(query.keywords[:3])}")

        # Generic
        suggestions.append("What recent decisions have we made?")

        return suggestions[:3]

    def _suggest_followups(
        self,
        query: ParsedQuery,
        results: List[SearchResult]
    ) -> List[str]:
        """Suggest follow-up queries based on results"""
        suggestions = []

        # Based on result domains
        domains = set(r.domain for r in results[:3])
        for domain in domains:
            if domain != "general":
                suggestions.append(f"What other {domain} decisions have we made?")

        # Based on entities in results
        for r in results[:2]:
            if r.title:
                # Extract key term from title
                words = r.title.split()[:3]
                if words:
                    suggestions.append(f"Why did we decide on {' '.join(words)}?")

        # Generic follow-ups
        suggestions.append("What were the alternatives considered?")
        suggestions.append("Who was involved in this decision?")

        return suggestions[:3]


def format_answer_for_display(answer: SynthesizedAnswer) -> str:
    """Format synthesized answer for CLI/UI display"""
    lines = [
        answer.answer,
        "",
        f"**Confidence**: {answer.confidence:.0%}",
    ]

    if answer.warnings:
        lines.append("")
        lines.append("**Warnings**:")
        for w in answer.warnings:
            lines.append(f"  - {w}")

    if answer.sources:
        lines.append("")
        lines.append("**Sources**:")
        for s in answer.sources[:3]:
            lines.append(f"  - [{s['record_id']}] {s['title']} ({s['certainty']})")

    if answer.related_queries:
        lines.append("")
        lines.append("**Related queries**:")
        for q in answer.related_queries:
            lines.append(f"  - {q}")

    return "\n".join(lines)
