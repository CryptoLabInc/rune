"""Tests for shared LLM response parsing utilities."""

import pytest
from agents.common.llm_utils import parse_llm_json


class TestParseLlmJson:
    def test_valid_json(self):
        assert parse_llm_json('{"key": "value"}') == {"key": "value"}

    def test_json_with_markdown_fences(self):
        raw = '```json\n{"capture": true, "reason": "test"}\n```'
        result = parse_llm_json(raw)
        assert result == {"capture": True, "reason": "test"}

    def test_json_with_plain_fences(self):
        raw = '```\n{"a": 1}\n```'
        assert parse_llm_json(raw) == {"a": 1}

    def test_json_embedded_in_text(self):
        raw = 'Here is the result: {"key": "value"} and some trailing text.'
        assert parse_llm_json(raw) == {"key": "value"}

    def test_no_json_returns_empty_dict(self):
        assert parse_llm_json("This is not JSON at all") == {}

    def test_empty_string_returns_empty_dict(self):
        assert parse_llm_json("") == {}

    def test_nested_json(self):
        raw = '{"phases": [{"title": "A"}, {"title": "B"}]}'
        result = parse_llm_json(raw)
        assert len(result["phases"]) == 2

    def test_invalid_json_with_braces_returns_empty(self):
        raw = '{"broken: json'
        assert parse_llm_json(raw) == {}
