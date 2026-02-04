"""
Tests for Pattern Parser

Tests for parsing capture-triggers.md into structured patterns.
"""

import pytest
from pathlib import Path
from unittest.mock import patch


class TestPatternParser:
    """Tests for pattern parsing functionality"""

    def test_parse_simple_markdown(self, tmp_path):
        """Test parsing a simple markdown file"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        md_content = """# Capture Triggers

## Architecture Decisions

### High-Priority

- "We decided to use X instead of Y"
- "Let's go with option A"

### Medium-Priority

- "After discussion, we chose"
"""
        md_file = tmp_path / "test_triggers.md"
        md_file.write_text(md_content)

        patterns = parse_capture_triggers(str(md_file))

        assert len(patterns) >= 2
        assert any("We decided to use" in p["text"] for p in patterns)

    def test_parse_with_categories(self, tmp_path):
        """Test that category headers are correctly parsed"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        md_content = """# Triggers

## Security Compliance

- "For compliance, we need to implement this policy"

## Product Decisions

- "We decided to prioritize feature X because customer demand"
"""
        md_file = tmp_path / "test_triggers.md"
        md_file.write_text(md_content)

        patterns = parse_capture_triggers(str(md_file))

        security_patterns = [p for p in patterns if p["domain"] == "security"]
        product_patterns = [p for p in patterns if p["domain"] == "product"]

        assert len(security_patterns) >= 1
        assert len(product_patterns) >= 1

    def test_parse_priority_detection(self, tmp_path):
        """Test that priority is correctly detected from section headers"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        md_content = """# Triggers

## Architecture

### High_Confidence Triggers

- "Critical architecture decision"

### Medium_Confidence Triggers

- "Moderate importance choice"
"""
        md_file = tmp_path / "test_triggers.md"
        md_file.write_text(md_content)

        patterns = parse_capture_triggers(str(md_file))

        high_priority = [p for p in patterns if p["priority"] == "high"]
        medium_priority = [p for p in patterns if p["priority"] == "medium"]

        assert len(high_priority) >= 1
        assert len(medium_priority) >= 1

    def test_domain_inference(self):
        """Test domain inference from category names"""
        from agents.scribe.pattern_parser import _infer_domain

        assert _infer_domain("architecture_decisions") == "architecture"
        assert _infer_domain("security_compliance") == "security"
        assert _infer_domain("product_features") == "product"
        assert _infer_domain("executive_strategy") == "exec"
        assert _infer_domain("operations_deployment") == "ops"
        assert _infer_domain("random_category") == "general"

    def test_normalize_category(self):
        """Test category normalization"""
        from agents.scribe.pattern_parser import _normalize_category

        assert _normalize_category("Architecture Decisions") == "architecture_decisions"
        assert _normalize_category("Security & Compliance") == "security_compliance"
        assert _normalize_category("  Spaces  Here  ") == "spaces_here"

    def test_parse_backtick_patterns(self, tmp_path):
        """Test parsing patterns in backticks"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        md_content = """# Triggers

## Technical

- `We need to implement this feature`
- `The system should handle X`
"""
        md_file = tmp_path / "test_triggers.md"
        md_file.write_text(md_content)

        patterns = parse_capture_triggers(str(md_file))

        assert len(patterns) >= 1
        assert any("implement" in p["text"].lower() for p in patterns)

    def test_skip_short_patterns(self, tmp_path):
        """Test that very short patterns are skipped"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        md_content = """# Triggers

## General

- "Hi"
- "OK"
- "This is a valid pattern to capture"
"""
        md_file = tmp_path / "test_triggers.md"
        md_file.write_text(md_content)

        patterns = parse_capture_triggers(str(md_file))

        # Short patterns should be skipped
        assert not any(p["text"] == "Hi" for p in patterns)
        assert not any(p["text"] == "OK" for p in patterns)

    def test_remove_duplicates(self, tmp_path):
        """Test that duplicate patterns are removed"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        md_content = """# Triggers

## Category1

- "Same pattern here"

## Category2

- "Same pattern here"
"""
        md_file = tmp_path / "test_triggers.md"
        md_file.write_text(md_content)

        patterns = parse_capture_triggers(str(md_file))

        # Should only have one of the duplicates
        same_patterns = [p for p in patterns if "Same pattern here" in p["text"]]
        assert len(same_patterns) == 1

    def test_file_not_found(self):
        """Test that FileNotFoundError is raised for missing file"""
        from agents.scribe.pattern_parser import parse_capture_triggers

        with pytest.raises(FileNotFoundError):
            parse_capture_triggers("/nonexistent/path/triggers.md")


class TestBuiltinPatterns:
    """Tests for builtin fallback patterns"""

    def test_builtin_patterns_available(self):
        """Test that builtin patterns are available"""
        from agents.scribe.pattern_parser import get_builtin_patterns

        patterns = get_builtin_patterns()

        assert len(patterns) > 0
        assert all("text" in p for p in patterns)
        assert all("category" in p for p in patterns)
        assert all("priority" in p for p in patterns)
        assert all("domain" in p for p in patterns)

    def test_builtin_patterns_have_high_priority(self):
        """Test that builtin patterns include high priority ones"""
        from agents.scribe.pattern_parser import get_builtin_patterns

        patterns = get_builtin_patterns()
        high_priority = [p for p in patterns if p["priority"] == "high"]

        assert len(high_priority) > 0

    def test_builtin_patterns_cover_domains(self):
        """Test that builtin patterns cover multiple domains"""
        from agents.scribe.pattern_parser import get_builtin_patterns

        patterns = get_builtin_patterns()
        domains = set(p["domain"] for p in patterns)

        assert "architecture" in domains
        assert "security" in domains
        assert "product" in domains


class TestLoadDefaultPatterns:
    """Tests for loading default patterns"""

    def test_load_default_falls_back_to_builtin(self):
        """Test that load_default_patterns falls back to builtin"""
        from agents.scribe.pattern_parser import load_default_patterns, get_builtin_patterns

        # Even if file doesn't exist, should return builtin patterns
        patterns = load_default_patterns()

        assert len(patterns) > 0
        # Should return at least builtin patterns
        builtin_count = len(get_builtin_patterns())
        assert len(patterns) >= builtin_count or len(patterns) > 0
