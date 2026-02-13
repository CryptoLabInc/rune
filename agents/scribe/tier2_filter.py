"""
Tier 2 LLM Filter — Policy-based capture judgment.

After Tier 1 (embedding similarity) finds candidates, Tier 2 uses a small LLM
(Haiku) to judge whether the message is truly worth capturing as organizational
memory, based on natural language policy.

Token budget: ~200 tokens per call (policy summary + message + response).
"""

import json
from dataclasses import dataclass
from typing import Optional


FILTER_POLICY = """You judge whether a workplace message contains significant organizational knowledge that should be permanently recorded.

CAPTURE if the message contains:
- A concrete decision with reasoning (technology choice, architecture, process change)
- A policy or standard being established or changed
- A trade-off analysis or rejection of an alternative
- A lesson learned from an incident, failure, or debugging session
- A commitment or agreement that affects the team
- Incident postmortem findings, root cause analysis, or corrective actions
- Debugging breakthroughs: root cause identified, fix applied, workaround found
- Bug triage outcomes: severity, ownership, or fix strategy decided
- QA findings that change test strategy or acceptance criteria
- Legal/compliance decisions or regulatory interpretations
- Budget allocations, pricing changes, or cost optimization decisions
- Sales intelligence: deal outcomes, competitive insights, customer requirements
- Customer escalation resolutions or churn analysis insights
- Research findings, experiment results, or proof-of-concept conclusions
- Risk assessments with mitigation strategies

DO NOT CAPTURE:
- Casual conversation, greetings, or social chat
- Questions without answers or decisions
- Status updates without decisions or insights ("still working on X")
- Vague opinions without commitment ("maybe we should...")
- Routine alerts/deployments with no decision or learning attached

Respond with JSON only: {"capture": true/false, "reason": "one sentence", "domain": "architecture|security|product|exec|ops|design|data|hr|marketing|incident|debugging|qa|legal|finance|sales|customer_success|research|risk|general"}"""


@dataclass
class FilterResult:
    """Result of Tier 2 LLM filter."""
    should_capture: bool
    reason: str
    domain: str = "general"
    raw_response: Optional[str] = None


class Tier2Filter:
    """
    LLM-based policy filter for capture decisions.

    Uses a small, fast model (Haiku) to evaluate whether a Tier 1 candidate
    is truly worth capturing as organizational memory.
    """

    def __init__(
        self,
        anthropic_api_key: Optional[str] = None,
        model: str = "claude-haiku-4-5-20251001",
    ):
        self._api_key = anthropic_api_key
        self._model = model
        self._client = None

        if anthropic_api_key:
            try:
                import anthropic
                self._client = anthropic.Anthropic(api_key=anthropic_api_key)
            except ImportError:
                print("[Tier2Filter] Warning: anthropic package not installed")
            except Exception as e:
                print(f"[Tier2Filter] Warning: Failed to init client: {e}")

    @property
    def is_available(self) -> bool:
        return self._client is not None

    def evaluate(self, text: str, tier1_score: float = 0.0, tier1_pattern: str = "") -> FilterResult:
        """
        Evaluate whether a message should be captured.

        Args:
            text: The candidate message text
            tier1_score: Similarity score from Tier 1 (for context)
            tier1_pattern: Matched pattern from Tier 1 (for context)

        Returns:
            FilterResult with capture decision and reasoning
        """
        if not self.is_available:
            # Fallback: pass through (let Tier 1 decision stand)
            return FilterResult(
                should_capture=True,
                reason="LLM filter unavailable, defaulting to Tier 1 decision",
            )

        try:
            user_msg = f"Message: {text[:500]}"
            if tier1_pattern:
                user_msg += f"\n(Tier 1 matched pattern: \"{tier1_pattern[:80]}\")"

            response = self._client.messages.create(
                model=self._model,
                max_tokens=100,
                system=FILTER_POLICY,
                messages=[{"role": "user", "content": user_msg}],
            )

            raw = response.content[0].text.strip()
            return self._parse_response(raw)

        except Exception as e:
            print(f"[Tier2Filter] Evaluation failed: {e}")
            # Fallback: pass through
            return FilterResult(
                should_capture=True,
                reason=f"LLM filter error ({e}), defaulting to capture",
            )

    def _parse_response(self, raw: str) -> FilterResult:
        """Parse LLM JSON response."""
        # Strip markdown fences if present
        text = raw
        if text.startswith("```"):
            lines = text.split("\n")
            lines = [l for l in lines if not l.strip().startswith("```")]
            text = "\n".join(lines)

        try:
            data = json.loads(text)
        except json.JSONDecodeError:
            start = text.find("{")
            end = text.rfind("}") + 1
            if start >= 0 and end > start:
                try:
                    data = json.loads(text[start:end])
                except json.JSONDecodeError:
                    return FilterResult(
                        should_capture=True,
                        reason="Failed to parse LLM response, defaulting to capture",
                        raw_response=raw,
                    )
            else:
                return FilterResult(
                    should_capture=True,
                    reason="No JSON in LLM response, defaulting to capture",
                    raw_response=raw,
                )

        return FilterResult(
            should_capture=bool(data.get("capture", True)),
            reason=str(data.get("reason", "")),
            domain=str(data.get("domain", "general")).lower(),
            raw_response=raw,
        )
