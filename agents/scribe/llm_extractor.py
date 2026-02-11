"""
LLM-based Field Extractor

Extracts structured decision record fields from non-English text using LLM.
All outputs are translated to English for embedding consistency.
"""

import json
from dataclasses import dataclass, field
from typing import List, Optional


@dataclass
class ExtractedFields:
    """Fields extracted by LLM from non-English text"""
    title: str = ""
    rationale: str = ""
    problem: str = ""
    alternatives: List[str] = field(default_factory=list)
    trade_offs: List[str] = field(default_factory=list)
    status_hint: str = ""       # "proposed" | "accepted" | "rejected"
    tags: List[str] = field(default_factory=list)


EXTRACTION_PROMPT = """You are a structured information extractor for organizational decision records.

Given a message (which may be in any language), extract the following fields.
IMPORTANT: All output values MUST be in English (translate if needed).

Respond with a valid JSON object with these keys:
- "title": A short title for the decision (5-60 chars, in English)
- "rationale": The reasoning behind the decision (in English, empty string if not found)
- "problem": The problem being solved (in English, empty string if not found)
- "alternatives": List of alternatives considered (in English, empty list if none)
- "trade_offs": List of trade-offs mentioned (in English, empty list if none)
- "status_hint": One of "proposed", "accepted", "rejected" based on the tone/language
- "tags": List of relevant topic tags (in English, e.g. ["database", "migration"])

Rules:
- Translate ALL values to English
- Keep the title concise and descriptive
- If a field is not clearly present in the text, use empty string or empty list
- For status_hint: use "accepted" if the message indicates a finalized decision, "proposed" if tentative, "rejected" if something was decided against

Message to extract from:
{text}

JSON:"""


class LLMExtractor:
    """Extracts structured fields from non-English text using Claude API."""

    def __init__(
        self,
        anthropic_api_key: Optional[str] = None,
        model: str = "claude-sonnet-4-20250514",
    ):
        self._api_key = anthropic_api_key
        self._model = model
        self._client = None

        if anthropic_api_key:
            try:
                import anthropic
                self._client = anthropic.Anthropic(api_key=anthropic_api_key)
            except ImportError:
                print("[LLMExtractor] Warning: anthropic package not installed")
            except Exception as e:
                print(f"[LLMExtractor] Warning: Failed to init Anthropic client: {e}")

    @property
    def is_available(self) -> bool:
        """Check if LLM client is ready"""
        return self._client is not None

    def extract(self, text: str) -> ExtractedFields:
        """Extract structured fields from text using LLM.

        Args:
            text: Input text (any language)

        Returns:
            ExtractedFields with English-translated values
        """
        if not self.is_available:
            return ExtractedFields()

        try:
            prompt = EXTRACTION_PROMPT.format(text=text)

            response = self._client.messages.create(
                model=self._model,
                max_tokens=512,
                messages=[
                    {"role": "user", "content": prompt}
                ],
            )

            raw = response.content[0].text.strip()
            return self._parse_response(raw)
        except Exception as e:
            print(f"[LLMExtractor] Extraction failed: {e}")
            return ExtractedFields()

    def _parse_response(self, raw: str) -> ExtractedFields:
        """Parse LLM JSON response into ExtractedFields."""
        # Strip markdown code fences if present
        if raw.startswith("```"):
            lines = raw.split("\n")
            # Remove first and last lines (```json and ```)
            lines = [l for l in lines if not l.strip().startswith("```")]
            raw = "\n".join(lines)

        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            # Try to find JSON object in the response
            start = raw.find("{")
            end = raw.rfind("}") + 1
            if start >= 0 and end > start:
                try:
                    data = json.loads(raw[start:end])
                except json.JSONDecodeError:
                    return ExtractedFields()
            else:
                return ExtractedFields()

        return ExtractedFields(
            title=str(data.get("title", ""))[:60],
            rationale=str(data.get("rationale", "")),
            problem=str(data.get("problem", "")),
            alternatives=[str(a) for a in data.get("alternatives", []) if a],
            trade_offs=[str(t) for t in data.get("trade_offs", []) if t],
            status_hint=str(data.get("status_hint", "")).lower(),
            tags=[str(t).lower() for t in data.get("tags", []) if t],
        )
