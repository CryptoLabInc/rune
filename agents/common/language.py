"""
Language Detection Service

Automatic per-message language detection using langdetect + Unicode script fallback.
Used to route messages to LLM extraction (non-English) or regex extraction (English).
"""

import re
import unicodedata
from dataclasses import dataclass
from typing import Optional

# Matches any Hangul, Kana, or CJK character
_NON_LATIN_RE = re.compile(
    r'[\u1100-\u11FF\u3040-\u309F\u30A0-\u30FF\u3130-\u318F'
    r'\u3400-\u4DBF\u4E00-\u9FFF\uAC00-\uD7AF]'
)

# Seed langdetect for deterministic results
try:
    from langdetect import DetectorFactory
    DetectorFactory.seed = 0
except ImportError:
    pass


@dataclass(frozen=True)
class LanguageInfo:
    """Detected language information"""
    code: str           # ISO 639-1: "en", "ko", "ja"
    confidence: float   # 0.0~1.0
    script: str         # "Latin", "Hangul", "CJK", "Kana", "Mixed"

    @property
    def is_english(self) -> bool:
        return self.code == "en"

    @property
    def needs_llm_extraction(self) -> bool:
        """Non-English text needs LLM extraction path"""
        return not self.is_english


# Unicode range based script detection
_SCRIPT_RANGES = [
    (0xAC00, 0xD7AF, "Hangul", "ko"),    # Hangul Syllables
    (0x1100, 0x11FF, "Hangul", "ko"),    # Hangul Jamo
    (0x3130, 0x318F, "Hangul", "ko"),    # Hangul Compatibility Jamo
    (0x3040, 0x309F, "Kana", "ja"),      # Hiragana
    (0x30A0, 0x30FF, "Kana", "ja"),      # Katakana
    (0x4E00, 0x9FFF, "CJK", "zh"),      # CJK Unified Ideographs
    (0x3400, 0x4DBF, "CJK", "zh"),      # CJK Extension A
]


def _detect_script(text: str) -> tuple[str, Optional[str]]:
    """Detect dominant script from Unicode character ranges.

    Returns:
        (script_name, language_code) or ("Latin", None) for ASCII-dominant text
    """
    script_counts: dict[str, int] = {}
    lang_counts: dict[str, int] = {}
    total = 0

    for ch in text:
        if ch.isspace() or ch in '.,!?;:"\'-()[]{}':
            continue
        total += 1
        cp = ord(ch)
        matched = False
        for start, end, script, lang in _SCRIPT_RANGES:
            if start <= cp <= end:
                script_counts[script] = script_counts.get(script, 0) + 1
                lang_counts[lang] = lang_counts.get(lang, 0) + 1
                matched = True
                break
        if not matched:
            script_counts["Latin"] = script_counts.get("Latin", 0) + 1

    if total == 0:
        return "Latin", None

    # Find dominant script
    dominant_script = max(script_counts, key=script_counts.get)
    dominant_count = script_counts[dominant_script]

    # If multiple scripts are significant, mark as Mixed
    non_latin_scripts = {k: v for k, v in script_counts.items() if k != "Latin"}
    if len(non_latin_scripts) > 1:
        top_two = sorted(non_latin_scripts.values(), reverse=True)
        if len(top_two) >= 2 and top_two[1] > total * 0.2:
            # Japanese text often mixes CJK + Kana
            if "Kana" in non_latin_scripts and "CJK" in non_latin_scripts:
                return "Kana", "ja"
            return "Mixed", None

    # Determine language from dominant non-Latin script
    if dominant_script != "Latin" and dominant_count > total * 0.15:
        # Find the language for this script
        for lang, count in lang_counts.items():
            if count == max(lang_counts.values()):
                # Special case: CJK with Kana = Japanese
                if "Kana" in script_counts and script_counts.get("Kana", 0) > 0:
                    return "Kana", "ja"
                return dominant_script, lang

    return "Latin", None


def detect_language(text: str) -> LanguageInfo:
    """Detect language of input text.

    Uses langdetect library with Unicode script-based fallback.
    Short texts (<10 chars) default to English.

    Args:
        text: Input text to detect language for

    Returns:
        LanguageInfo with detected language code, confidence, and script
    """
    if not text or not text.strip():
        return LanguageInfo(code="en", confidence=1.0, script="Latin")

    cleaned = text.strip()

    # Very short text defaults to English
    if len(cleaned) < 10:
        # But check for obvious non-Latin scripts
        script, lang = _detect_script(cleaned)
        if lang:
            return LanguageInfo(code=lang, confidence=0.6, script=script)
        return LanguageInfo(code="en", confidence=0.5, script="Latin")

    # Determine script first — used to validate langdetect results
    script, script_lang = _detect_script(cleaned)

    # Try langdetect
    try:
        from langdetect import detect_langs
        results = detect_langs(cleaned)
        if results:
            top = results[0]
            lang_code = top.lang
            confidence = top.prob

            # If text is purely Latin-script (no Hangul/Kana/CJK characters),
            # treat as English. langdetect frequently misclassifies short English
            # text as fr, af, nl, de, etc. The LLM extraction path is designed
            # for CJK scripts (ko, ja, zh), not Latin-script languages.
            # However, if there ARE any non-Latin chars (e.g., Korean text with
            # English terms like "PostgreSQL"), trust langdetect.
            if lang_code != "en" and not _NON_LATIN_RE.search(cleaned):
                return LanguageInfo(code="en", confidence=0.5, script="Latin")

            return LanguageInfo(
                code=lang_code,
                confidence=round(confidence, 4),
                script=script,
            )
    except ImportError:
        pass  # langdetect not installed, fall through to Unicode fallback
    except Exception:
        pass  # langdetect failed (e.g., too short), fall through

    # Fallback: Unicode script-based detection
    if script_lang:
        return LanguageInfo(code=script_lang, confidence=0.7, script=script)

    # Default to English for Latin script
    return LanguageInfo(code="en", confidence=0.5, script="Latin")
