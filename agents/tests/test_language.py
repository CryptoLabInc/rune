"""
Tests for Language Detection Service

Tests language detection, Unicode script fallback, and LanguageInfo properties.
"""

import pytest


class TestLanguageInfo:
    """Tests for LanguageInfo dataclass"""

    def test_english_language_info(self):
        from agents.common.language import LanguageInfo

        info = LanguageInfo(code="en", confidence=0.99, script="Latin")

        assert info.is_english is True
        assert info.needs_llm_extraction is False

    def test_korean_language_info(self):
        from agents.common.language import LanguageInfo

        info = LanguageInfo(code="ko", confidence=0.95, script="Hangul")

        assert info.is_english is False
        assert info.needs_llm_extraction is True

    def test_japanese_language_info(self):
        from agents.common.language import LanguageInfo

        info = LanguageInfo(code="ja", confidence=0.90, script="Kana")

        assert info.is_english is False
        assert info.needs_llm_extraction is True

    def test_language_info_is_frozen(self):
        from agents.common.language import LanguageInfo

        info = LanguageInfo(code="en", confidence=1.0, script="Latin")

        with pytest.raises(AttributeError):
            info.code = "ko"


class TestDetectLanguage:
    """Tests for detect_language function"""

    def test_detect_english(self):
        from agents.common.language import detect_language

        result = detect_language("We decided to use PostgreSQL instead of MySQL")

        assert result.code == "en"
        assert result.is_english is True
        assert result.confidence > 0.0

    def test_detect_korean(self):
        from agents.common.language import detect_language

        result = detect_language("PostgreSQL을 사용하기로 결정했다. 팀에서 논의한 결과 성능이 더 좋기 때문이다.")

        assert result.code == "ko"
        assert result.is_english is False
        assert result.script == "Hangul"

    def test_detect_japanese(self):
        from agents.common.language import detect_language

        result = detect_language("PostgreSQLを使うことに決めた。チームで議論した結果、パフォーマンスが良いからだ。")

        assert result.code == "ja"
        assert result.is_english is False
        assert result.script in ("Kana", "CJK")

    def test_empty_text_defaults_to_english(self):
        from agents.common.language import detect_language

        result = detect_language("")
        assert result.code == "en"

        result = detect_language("   ")
        assert result.code == "en"

    def test_short_text_defaults_to_english(self):
        from agents.common.language import detect_language

        result = detect_language("Hello")
        assert result.code == "en"

    def test_short_korean_detected_by_script(self):
        from agents.common.language import detect_language

        result = detect_language("결정했다")

        # Even short text should detect Korean via Unicode script
        assert result.code == "ko"
        assert result.script == "Hangul"

    def test_short_japanese_detected_by_script(self):
        from agents.common.language import detect_language

        result = detect_language("決めた")

        # Should detect CJK/Japanese via script
        assert result.code in ("ja", "zh")

    def test_mixed_english_korean(self):
        from agents.common.language import detect_language

        result = detect_language("PostgreSQL을 사용하기로 결정했습니다. 이유는 JSON 지원이 좋기 때문입니다.")

        # Should detect Korean (Hangul is dominant script)
        assert result.code == "ko"

    def test_confidence_range(self):
        from agents.common.language import detect_language

        result = detect_language("We decided to use PostgreSQL because of better JSON support")

        assert 0.0 <= result.confidence <= 1.0

    def test_none_input(self):
        from agents.common.language import detect_language

        result = detect_language(None)
        assert result.code == "en"


class TestDetectScript:
    """Tests for Unicode script detection fallback"""

    def test_hangul_script(self):
        from agents.common.language import _detect_script

        script, lang = _detect_script("한글 테스트입니다")

        assert script == "Hangul"
        assert lang == "ko"

    def test_kana_script(self):
        from agents.common.language import _detect_script

        script, lang = _detect_script("テストです")

        assert script == "Kana"
        assert lang == "ja"

    def test_latin_script(self):
        from agents.common.language import _detect_script

        script, lang = _detect_script("Hello World")

        assert script == "Latin"
        assert lang is None

    def test_cjk_with_kana_is_japanese(self):
        from agents.common.language import _detect_script

        # Japanese text with both kanji and hiragana
        script, lang = _detect_script("決定した理由は")

        assert lang == "ja" or script in ("Kana", "CJK")

    def test_empty_text(self):
        from agents.common.language import _detect_script

        script, lang = _detect_script("")

        assert script == "Latin"
        assert lang is None
