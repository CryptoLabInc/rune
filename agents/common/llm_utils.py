"""Shared utilities for parsing LLM responses."""

from __future__ import annotations

import json


def parse_llm_json(raw: str) -> dict:
    """Parse JSON from an LLM response, handling code fences and preamble text.

    Tries in order:
    1. Strip markdown code fences, then json.loads
    2. Direct json.loads on the raw string
    3. Extract substring between first '{' and last '}', then json.loads
    4. Return empty dict
    """
    if not raw:
        return {}

    text = raw
    if text.startswith("```"):
        lines = text.split("\n")
        lines = [l for l in lines if not l.strip().startswith("```")]
        text = "\n".join(lines)

    try:
        return json.loads(text)
    except json.JSONDecodeError:
        pass

    start = raw.find("{")
    end = raw.rfind("}") + 1
    if start >= 0 and end > start:
        try:
            return json.loads(raw[start:end])
        except json.JSONDecodeError:
            pass

    return {}
