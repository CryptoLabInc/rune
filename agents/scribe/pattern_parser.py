"""
Pattern Parser

Parses trigger patterns from patterns/capture-triggers.md.
Extracts phrases organized by category and priority.
"""

import re
from pathlib import Path
from typing import List, Dict, Optional


# Domain mapping from category names
CATEGORY_TO_DOMAIN = {
    "architecture": "architecture",
    "technical": "architecture",
    "security": "security",
    "compliance": "security",
    "product": "product",
    "business": "product",
    "executive": "exec",
    "strategic": "exec",
    "ops": "ops",
    "operations": "ops",
    "deployment": "ops",
    "design": "design",
    "ux": "design",
    "data": "data",
    "analytics": "data",
    "hr": "hr",
    "hiring": "hr",
    "people": "hr",
    "marketing": "marketing",
    "performance": "architecture",
    "optimization": "architecture",
    "technical_debt": "architecture",
}


def _normalize_category(raw_category: str) -> str:
    """Normalize category name to lowercase with underscores"""
    # Remove special characters, convert to lowercase
    normalized = re.sub(r'[^a-zA-Z0-9\s]', '', raw_category.lower())
    normalized = re.sub(r'\s+', '_', normalized.strip())
    return normalized


def _infer_domain(category: str) -> str:
    """Infer domain from category name"""
    category_lower = category.lower()

    # Check direct mapping
    for key, domain in CATEGORY_TO_DOMAIN.items():
        if key in category_lower:
            return domain

    return "general"


def _detect_priority(line: str, section_context: str) -> str:
    """Detect priority from line content and section context"""
    line_lower = line.lower()
    section_lower = section_context.lower()

    # High priority indicators
    high_indicators = [
        "high_confidence", "high-confidence", "high priority",
        "always capture", "critical", "must capture",
        "explicit decision", "trade-off", "security", "compliance"
    ]

    # Medium priority indicators
    medium_indicators = [
        "medium_confidence", "medium-confidence", "medium priority",
        "usually capture", "context-dependent"
    ]

    # Check section context first
    for indicator in high_indicators:
        if indicator in section_lower:
            return "high"

    for indicator in medium_indicators:
        if indicator in section_lower:
            return "medium"

    # Check line content
    for indicator in high_indicators:
        if indicator in line_lower:
            return "high"

    # Default to medium
    return "medium"


def parse_capture_triggers(md_path: str) -> List[Dict]:
    """
    Parse capture-triggers.md into structured pattern list.

    Args:
        md_path: Path to capture-triggers.md file

    Returns:
        List of dicts with keys:
            - text: Pattern text
            - category: Category name
            - priority: "high", "medium", or "low"
            - domain: Domain classification

    Example:
        [
            {
                "text": "We decided to use X instead of Y because...",
                "category": "architecture_technical_decisions",
                "priority": "high",
                "domain": "architecture"
            },
            ...
        ]
    """
    path = Path(md_path)
    if not path.exists():
        raise FileNotFoundError(f"Pattern file not found: {md_path}")

    content = path.read_text(encoding='utf-8')
    patterns = []

    current_category = "general"
    current_section = ""
    current_priority = "medium"

    lines = content.split('\n')

    for line in lines:
        line_stripped = line.strip()

        # Skip empty lines and comments
        if not line_stripped or line_stripped.startswith('<!--'):
            continue

        # Detect category headers (## Category Name)
        if line_stripped.startswith('## '):
            current_category = _normalize_category(line_stripped[3:])
            current_section = line_stripped
            continue

        # Detect subsection headers (### Subsection)
        if line_stripped.startswith('### '):
            current_section = line_stripped
            # Update priority based on section name
            current_priority = _detect_priority(line_stripped, current_section)
            continue

        # Detect priority markers in content
        if 'HIGH_CONFIDENCE' in line_stripped or 'High-Priority' in line_stripped:
            current_priority = "high"
            continue
        if 'MEDIUM_CONFIDENCE' in line_stripped or 'Medium-Priority' in line_stripped:
            current_priority = "medium"
            continue

        # Extract quoted patterns: - "pattern text..."
        quote_match = re.match(r'^[-*]\s*["\']([^"\']+)["\']', line_stripped)
        if quote_match:
            pattern_text = quote_match.group(1).strip()
            if len(pattern_text) >= 5:  # Skip very short patterns
                patterns.append({
                    "text": pattern_text,
                    "category": current_category,
                    "priority": current_priority,
                    "domain": _infer_domain(current_category),
                })
            continue

        # Extract patterns in code blocks or with specific markers
        # Pattern: `pattern text`
        backtick_match = re.match(r'^[-*]?\s*`([^`]+)`', line_stripped)
        if backtick_match:
            pattern_text = backtick_match.group(1).strip()
            if len(pattern_text) >= 5:
                patterns.append({
                    "text": pattern_text,
                    "category": current_category,
                    "priority": current_priority,
                    "domain": _infer_domain(current_category),
                })
            continue

        # Extract list patterns that look like trigger phrases
        # Pattern: - We decided to... / - Let's go with...
        list_match = re.match(r'^[-*]\s+([A-Z][^.!?\n]{10,})', line_stripped)
        if list_match:
            text = list_match.group(1).strip()
            # Skip if it looks like a description rather than a pattern
            if not text.endswith(':') and not text.startswith('Example'):
                # Check for trigger phrase indicators
                trigger_indicators = [
                    'we decided', 'let\'s go', 'chose', 'decision',
                    'trade-off', 'because', 'rationale', 'reason',
                    'policy', 'requirement', 'must', 'should',
                ]
                text_lower = text.lower()
                if any(ind in text_lower for ind in trigger_indicators):
                    patterns.append({
                        "text": text,
                        "category": current_category,
                        "priority": current_priority,
                        "domain": _infer_domain(current_category),
                    })

    # Remove duplicates while preserving order
    seen = set()
    unique_patterns = []
    for p in patterns:
        key = p["text"].lower()
        if key not in seen:
            seen.add(key)
            unique_patterns.append(p)

    return unique_patterns


def load_default_patterns() -> List[Dict]:
    """
    Load patterns from the default capture-triggers.md location.

    Returns:
        List of pattern dicts
    """
    # Find patterns directory relative to this file
    current_dir = Path(__file__).parent
    patterns_dir = current_dir.parent.parent / "patterns"
    default_path = patterns_dir / "capture-triggers.md"

    if not default_path.exists():
        print(f"[PatternParser] Warning: Default patterns file not found at {default_path}")
        return get_builtin_patterns()

    return parse_capture_triggers(str(default_path))


def load_all_language_patterns() -> List[Dict]:
    """Load patterns from all language-specific capture-triggers files.

    Discovers capture-triggers.*.md files in the patterns/ directory
    and merges them with the base English patterns.

    Returns:
        List of pattern dicts, each with an optional 'language' key
    """
    current_dir = Path(__file__).parent
    patterns_dir = current_dir.parent.parent / "patterns"
    all_patterns = []

    # English base patterns
    base = patterns_dir / "capture-triggers.md"
    if base.exists():
        base_patterns = parse_capture_triggers(str(base))
        for p in base_patterns:
            p["language"] = "en"
        all_patterns.extend(base_patterns)

    # Language-specific patterns (capture-triggers.ko.md, capture-triggers.ja.md, ...)
    for lang_file in sorted(patterns_dir.glob("capture-triggers.*.md")):
        lang_code = lang_file.stem.split(".")[-1]
        try:
            lang_patterns = parse_capture_triggers(str(lang_file))
            for p in lang_patterns:
                p["language"] = lang_code
            all_patterns.extend(lang_patterns)
            print(f"[PatternParser] Loaded {len(lang_patterns)} patterns for '{lang_code}'")
        except Exception as e:
            print(f"[PatternParser] Warning: Failed to load {lang_file}: {e}")

    return all_patterns or get_builtin_patterns()


def get_builtin_patterns() -> List[Dict]:
    """
    Return built-in fallback patterns when file is not available.

    These are the core patterns that should always be available.
    """
    return [
        # Architecture decisions
        {"text": "We decided to use", "category": "architecture", "priority": "high", "domain": "architecture"},
        {"text": "We chose X over Y because", "category": "architecture", "priority": "high", "domain": "architecture"},
        {"text": "Let's go with", "category": "architecture", "priority": "high", "domain": "architecture"},
        {"text": "The trade-off is", "category": "architecture", "priority": "high", "domain": "architecture"},
        {"text": "Design decision:", "category": "architecture", "priority": "high", "domain": "architecture"},
        {"text": "Architecture decision:", "category": "architecture", "priority": "high", "domain": "architecture"},

        # Security decisions
        {"text": "Security-wise, we should", "category": "security", "priority": "high", "domain": "security"},
        {"text": "For compliance, we need", "category": "security", "priority": "high", "domain": "security"},
        {"text": "The encryption strategy is", "category": "security", "priority": "high", "domain": "security"},

        # Product decisions
        {"text": "We're prioritizing", "category": "product", "priority": "high", "domain": "product"},
        {"text": "Feature rejected because", "category": "product", "priority": "high", "domain": "product"},
        {"text": "Customer feedback shows", "category": "product", "priority": "medium", "domain": "product"},

        # General decisions
        {"text": "The reason we", "category": "general", "priority": "high", "domain": "general"},
        {"text": "After discussion, we", "category": "general", "priority": "medium", "domain": "general"},
        {"text": "The team agreed", "category": "general", "priority": "medium", "domain": "general"},
        {"text": "Consensus:", "category": "general", "priority": "high", "domain": "general"},
        {"text": "Final decision:", "category": "general", "priority": "high", "domain": "general"},

        # Performance
        {"text": "Performance bottleneck identified:", "category": "performance", "priority": "high", "domain": "architecture"},
        {"text": "This doesn't scale because", "category": "performance", "priority": "high", "domain": "architecture"},

        # Technical debt
        {"text": "Technical debt—adding to backlog", "category": "technical_debt", "priority": "medium", "domain": "architecture"},
        {"text": "We can refactor this later", "category": "technical_debt", "priority": "medium", "domain": "architecture"},
    ]
