"""
Provider-agnostic LLM client for Rune pipelines.

Supports Anthropic, OpenAI, and Google Gemini with a shared text-generation
interface.
"""

from __future__ import annotations

import logging
from typing import Optional

logger = logging.getLogger("rune.common.llm_client")


class LLMClient:
    """Unified text generation client across LLM providers."""

    def __init__(
        self,
        provider: str = "anthropic",
        model: str = "",
        anthropic_api_key: Optional[str] = None,
        openai_api_key: Optional[str] = None,
        google_api_key: Optional[str] = None,
    ) -> None:
        self.provider = (provider or "anthropic").lower()
        self.model = model
        self._client = None

        if self.provider == "auto":
            raise ValueError(
                '"auto" provider must be resolved before creating LLMClient. '
                'Use _resolve_provider() in the MCP server or scribe server.'
            )

        if self.provider == "anthropic":
            if not anthropic_api_key:
                logger.info("%s API key not provided, LLM client unavailable", self.provider)
                return
            try:
                import anthropic

                self._client = anthropic.Anthropic(api_key=anthropic_api_key)
            except ImportError:
                logger.warning("anthropic package not installed")
            except Exception as e:
                logger.warning("Failed to initialize Anthropic client: %s", e)
            return

        if self.provider == "openai":
            if not openai_api_key:
                logger.info("%s API key not provided, LLM client unavailable", self.provider)
                return
            try:
                from openai import OpenAI

                self._client = OpenAI(api_key=openai_api_key)
            except ImportError:
                logger.warning("openai package not installed")
            except Exception as e:
                logger.warning("Failed to initialize OpenAI client: %s", e)
            return

        if self.provider == "google":
            if not google_api_key:
                logger.info("%s API key not provided, LLM client unavailable", self.provider)
                return
            try:
                import google.generativeai as genai

                genai.configure(api_key=google_api_key)
                self._client = genai  # Store the module, not a model instance
                self._google_models = {}  # Cache models by system prompt hash
            except ImportError:
                logger.warning("google-generativeai package not installed")
            except Exception as e:
                logger.warning("Failed to initialize Gemini client: %s", e)
            return

        logger.warning("Unsupported LLM provider: %s", self.provider)

    @property
    def is_available(self) -> bool:
        return self._client is not None

    def generate(
        self,
        prompt: str,
        *,
        system: Optional[str] = None,
        max_tokens: int = 512,
        timeout: float = 30.0,
    ) -> str:
        if not self.is_available:
            raise RuntimeError("LLM client is not available")

        if self.provider == "anthropic":
            response = self._client.messages.create(
                model=self.model,
                max_tokens=max_tokens,
                system=system,
                messages=[{"role": "user", "content": prompt}],
                timeout=timeout,
            )
            return response.content[0].text.strip()

        if self.provider == "openai":
            messages = []
            if system:
                messages.append({"role": "system", "content": system})
            messages.append({"role": "user", "content": prompt})
            response = self._client.chat.completions.create(
                model=self.model,
                max_tokens=max_tokens,
                messages=messages,
                timeout=timeout,
            )
            return (response.choices[0].message.content or "").strip()

        if self.provider == "google":
            import hashlib

            cache_key = hashlib.md5((system or "").encode()).hexdigest()
            if cache_key not in self._google_models:
                kwargs = {"model_name": self.model}
                if system:
                    kwargs["system_instruction"] = system
                self._google_models[cache_key] = self._client.GenerativeModel(**kwargs)
            model = self._google_models[cache_key]
            response = model.generate_content(
                prompt,
                generation_config={"max_output_tokens": max_tokens},
                request_options={"timeout": timeout},
            )
            return response.text.strip()

        raise RuntimeError(f"Unsupported LLM provider: {self.provider}")

