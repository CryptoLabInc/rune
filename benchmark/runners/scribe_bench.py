#!/usr/bin/env python3
"""Scribe benchmark runner.

Tests the agent-delegated capture pipeline by feeding the actual scribe prompt
+ scenario input to an agent CLI, then scoring the output JSON against expectations.

This benchmarks what actually determines capture quality in v0.2.0:
the scribe prompt's ability to guide the agent's policy evaluation and extraction.

Usage:
    # Default: use claude CLI (no API key needed)
    python benchmark/runners/scribe_bench.py

    # Use a specific agent CLI
    python benchmark/runners/scribe_bench.py --agent gemini
    python benchmark/runners/scribe_bench.py --agent codex

    # Capture only / extraction only
    python benchmark/runners/scribe_bench.py --mode capture
    python benchmark/runners/scribe_bench.py --mode extraction

    # Filter by category
    python benchmark/runners/scribe_bench.py --category pr_review

    # Fallback: direct API call (for CI/automation without CLI auth)
    python benchmark/runners/scribe_bench.py --api-key $ANTHROPIC_API_KEY --provider anthropic

    # Save report
    python benchmark/runners/scribe_bench.py --report benchmark/reports/scribe.json
"""

from __future__ import annotations

import argparse
import json
import re
import shlex
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from runners.common import (
    RUNE_DIR,
    BenchmarkReport,
    ScenarioResult,
    check_title_keywords,
    load_scenarios,
)

# Known agent CLI configurations.
# Each maps to (command, args) where the prompt is piped via stdin.
AGENT_CLI = {
    "claude": (["claude"], ["-p"]),
    "gemini": (["gemini"], []),
    "codex": (["codex"], ["-q"]),
}

# Default models for direct API fallback
API_MODELS = {
    "anthropic": "claude-haiku-4-5-20251001",
    "openai": "gpt-4o-mini",
}

VERBOSE = False


def load_scribe_prompt() -> str:
    """Load and extract the evaluation-relevant sections from scribe.md.

    Strips out the Activation Check, Step 3 (MCP tool call), Handling Results,
    and Rules sections — the benchmark only needs Steps 1-2 (policy + extraction).
    """
    scribe_path = RUNE_DIR / "agents" / "claude" / "scribe.md"
    if not scribe_path.exists():
        print(f"Error: scribe prompt not found at {scribe_path}", file=sys.stderr)
        sys.exit(1)

    full_text = scribe_path.read_text()

    # Extract Steps 1-2 only (policy evaluation + structured extraction).
    # Cut everything before "## Step 1" and everything from "## Step 3" onward.
    sections_to_keep: list[str] = []
    current_section: list[str] = []
    keeping = False

    for line in full_text.splitlines():
        if line.startswith("## Step 1"):
            keeping = True
        elif line.startswith("## Step 3"):
            # Save accumulated section and stop
            if current_section:
                sections_to_keep.append("\n".join(current_section))
            keeping = False
            break

        if keeping:
            current_section.append(line)

    if current_section and keeping:
        sections_to_keep.append("\n".join(current_section))

    if not sections_to_keep:
        # Fallback: use full text if section markers changed
        return full_text

    return "\n\n".join(sections_to_keep)


def build_evaluation_prompt(scribe_prompt: str, input_text: str) -> str:
    """Build the prompt that simulates the scribe's evaluation of a message."""
    return f"""You are evaluating a workplace message for organizational memory capture.

{scribe_prompt}

---

Evaluate the following message. Output ONLY a single JSON object — no explanation, no markdown fences, no other text. Either a rejection:
{{"tier2": {{"capture": false, "reason": "...", "domain": "general"}}}}
Or a full extraction (Format A, B, or C as described above).

Message:
{input_text}"""


def call_agent_cli(prompt: str, agent: str) -> str:
    """Call an agent CLI, piping the prompt via stdin. Returns response text."""
    if agent in AGENT_CLI:
        cmd, args = AGENT_CLI[agent]
        full_cmd = cmd + args
    else:
        # Unknown agent: try splitting as a shell command
        full_cmd = shlex.split(agent)

    result = subprocess.run(
        full_cmd,
        input=prompt,
        capture_output=True,
        text=True,
        timeout=120,
    )

    if result.returncode != 0:
        stderr = result.stderr.strip()
        raise RuntimeError(
            f"{' '.join(full_cmd)} exited with code {result.returncode}: {stderr}"
        )

    return result.stdout


def call_api(prompt: str, provider: str, api_key: str, model: str) -> str:
    """Fallback: call LLM via API SDK directly."""
    if provider == "anthropic":
        import anthropic

        client = anthropic.Anthropic(api_key=api_key)
        response = client.messages.create(
            model=model,
            max_tokens=2048,
            messages=[{"role": "user", "content": prompt}],
        )
        return response.content[0].text

    elif provider == "openai":
        import openai

        client = openai.OpenAI(api_key=api_key)
        response = client.chat.completions.create(
            model=model,
            max_tokens=2048,
            messages=[{"role": "user", "content": prompt}],
        )
        return response.choices[0].message.content or ""

    else:
        raise ValueError(f"Unsupported provider: {provider}")


def call_llm(
    prompt: str,
    *,
    agent: str | None = None,
    provider: str | None = None,
    api_key: str | None = None,
    model: str | None = None,
) -> str:
    """Unified LLM call: prefers agent CLI, falls back to API."""
    if api_key and provider:
        mdl = model or API_MODELS.get(provider, "claude-haiku-4-5-20251001")
        return call_api(prompt, provider, api_key, mdl)
    else:
        return call_agent_cli(prompt, agent or "claude")


def parse_json_response(text: str) -> dict | None:
    """Extract JSON from LLM response, handling various wrapping formats."""
    text = text.strip()

    # 1. Direct parse (clean JSON)
    if text.startswith("{"):
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            pass

    # 2. Markdown code block
    match = re.search(r"```(?:json)?\s*\n(.*?)\n\s*```", text, re.DOTALL)
    if match:
        try:
            return json.loads(match.group(1).strip())
        except json.JSONDecodeError:
            pass

    # 3. Find the outermost balanced { ... } using brace counting
    start = text.find("{")
    if start != -1:
        depth = 0
        in_string = False
        escape_next = False
        for i in range(start, len(text)):
            ch = text[i]
            if escape_next:
                escape_next = False
                continue
            if ch == "\\":
                escape_next = True
                continue
            if ch == '"' and not escape_next:
                in_string = not in_string
                continue
            if in_string:
                continue
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    candidate = text[start : i + 1]
                    try:
                        return json.loads(candidate)
                    except json.JSONDecodeError:
                        break

    return None


def evaluate_capture(
    scenarios: list[dict],
    scribe_prompt: str,
    llm_kwargs: dict,
) -> BenchmarkReport:
    """Evaluate capture accuracy: does the scribe correctly capture or skip?"""
    report = BenchmarkReport(bench_type="scribe-capture")

    for i, scenario in enumerate(scenarios):
        sid = scenario["id"]
        expected = scenario["expected_capture"]
        text = scenario["input"]

        print(f"  [{i + 1}/{len(scenarios)}] {sid}...", end=" ", flush=True)

        prompt = build_evaluation_prompt(scribe_prompt, text)

        try:
            response_text = call_llm(prompt, **llm_kwargs)
            parsed = parse_json_response(response_text)
        except Exception as e:
            report.add(
                ScenarioResult(
                    scenario_id=sid,
                    category=scenario["category"],
                    passed=False,
                    expected=expected,
                    actual=None,
                    details={"error": str(e), "reason": f"LLM call failed: {e}"},
                )
            )
            print("ERROR")
            if VERBOSE:
                print(f"    {e}")
            continue

        if parsed is None:
            report.add(
                ScenarioResult(
                    scenario_id=sid,
                    category=scenario["category"],
                    passed=False,
                    expected=expected,
                    actual=None,
                    details={
                        "reason": "Failed to parse JSON from response",
                        "raw_response": response_text[:500],
                    },
                )
            )
            print("PARSE_ERROR")
            if VERBOSE:
                print(f"    Raw ({len(response_text)} chars): {response_text[:200]}")
            continue

        # Determine actual capture decision
        tier2 = parsed.get("tier2", {})
        actual = tier2.get("capture", False)

        # Capture implied by presence of extraction fields without tier2
        if "title" in parsed and "tier2" not in parsed:
            actual = True
        if "phases" in parsed and "tier2" not in parsed:
            actual = True

        passed = actual == expected
        details: dict = {
            "tier2_capture": actual,
            "tier2_domain": tier2.get("domain"),
            "tier2_reason": tier2.get("reason"),
        }

        if not passed:
            if expected and not actual:
                details["reason"] = "False negative: should have been captured"
            else:
                details["reason"] = "False positive: should not have been captured"

        expected_fields = scenario.get("expected_fields", {})
        if passed and expected and "domain" in expected_fields:
            details["domain_match"] = (
                tier2.get("domain") == expected_fields["domain"]
            )

        report.add(
            ScenarioResult(
                scenario_id=sid,
                category=scenario["category"],
                passed=passed,
                expected=expected,
                actual=actual,
                details=details,
            )
        )
        print("PASS" if passed else "FAIL")
        time.sleep(0.1)

    return report


def evaluate_extraction(
    scenarios: list[dict],
    scribe_prompt: str,
    llm_kwargs: dict,
) -> BenchmarkReport:
    """Evaluate extraction quality: is the extracted JSON well-structured?"""
    report = BenchmarkReport(bench_type="scribe-extraction")

    for i, scenario in enumerate(scenarios):
        sid = scenario["id"]
        text = scenario["input"]
        expected_type = scenario["expected_extraction_type"]
        expected_fields = scenario.get("expected_fields", {})

        print(f"  [{i + 1}/{len(scenarios)}] {sid}...", end=" ", flush=True)

        prompt = build_evaluation_prompt(scribe_prompt, text)

        try:
            response_text = call_llm(prompt, **llm_kwargs)
            parsed = parse_json_response(response_text)
        except Exception as e:
            report.add(
                ScenarioResult(
                    scenario_id=sid,
                    category=scenario["category"],
                    passed=False,
                    expected=expected_type,
                    actual=None,
                    details={"error": str(e), "reason": f"LLM call failed: {e}"},
                )
            )
            print("ERROR")
            if VERBOSE:
                print(f"    {e}")
            continue

        if parsed is None:
            report.add(
                ScenarioResult(
                    scenario_id=sid,
                    category=scenario["category"],
                    passed=False,
                    expected=expected_type,
                    actual=None,
                    details={
                        "reason": "Failed to parse JSON from response",
                        "raw_response": response_text[:500],
                    },
                )
            )
            print("PARSE_ERROR")
            if VERBOSE:
                print(f"    Raw ({len(response_text)} chars): {response_text[:200]}")
            continue

        # Check if scribe decided to capture at all
        tier2 = parsed.get("tier2", {})
        if not tier2.get("capture", True):
            report.add(
                ScenarioResult(
                    scenario_id=sid,
                    category=scenario["category"],
                    passed=False,
                    expected=expected_type,
                    actual="rejected",
                    details={
                        "reason": "Scribe rejected capture for extraction scenario",
                        "tier2_reason": tier2.get("reason"),
                    },
                )
            )
            print("REJECTED")
            continue

        # Determine actual extraction type
        group_type = parsed.get("group_type")
        if group_type == "bundle":
            actual_type = "bundle"
        elif group_type == "phase_chain" or "phases" in parsed:
            actual_type = "phase_chain"
        else:
            actual_type = "single"

        checks: dict[str, bool] = {}
        reasons: list[str] = []

        type_match = actual_type == expected_type
        checks["type_match"] = type_match
        if not type_match:
            reasons.append(f"Type: expected {expected_type}, got {actual_type}")

        if "title_keywords" in expected_fields:
            title = parsed.get("title") or parsed.get("group_title") or ""
            kw_match = check_title_keywords(title, expected_fields["title_keywords"])
            checks["title_keywords"] = kw_match
            if not kw_match:
                reasons.append(
                    f"Title '{title}' missing keywords: "
                    f"{expected_fields['title_keywords']}"
                )

        if "status_hint" in expected_fields:
            actual_status = parsed.get("status_hint", "")
            status_match = actual_status == expected_fields["status_hint"]
            checks["status_hint"] = status_match
            if not status_match:
                reasons.append(
                    f"Status: expected {expected_fields['status_hint']}, got {actual_status}"
                )

        if "min_alternatives" in expected_fields:
            if actual_type == "single":
                alt_count = len(parsed.get("alternatives", []))
            else:
                alt_count = sum(
                    len(p.get("alternatives", []))
                    for p in parsed.get("phases", [])
                )
            alt_ok = alt_count >= expected_fields["min_alternatives"]
            checks["min_alternatives"] = alt_ok
            if not alt_ok:
                reasons.append(
                    f"Alternatives: {alt_count} < {expected_fields['min_alternatives']}"
                )

        if "min_trade_offs" in expected_fields:
            if actual_type == "single":
                to_count = len(parsed.get("trade_offs", []))
            else:
                to_count = sum(
                    len(p.get("trade_offs", []))
                    for p in parsed.get("phases", [])
                )
            to_ok = to_count >= expected_fields["min_trade_offs"]
            checks["min_trade_offs"] = to_ok
            if not to_ok:
                reasons.append(
                    f"Trade-offs: {to_count} < {expected_fields['min_trade_offs']}"
                )

        phases = parsed.get("phases", [])
        if "min_phases" in expected_fields:
            min_ok = len(phases) >= expected_fields["min_phases"]
            checks["min_phases"] = min_ok
            if not min_ok:
                reasons.append(
                    f"Phases: {len(phases)} < {expected_fields['min_phases']}"
                )
        if "max_phases" in expected_fields:
            max_ok = len(phases) <= expected_fields["max_phases"]
            checks["max_phases"] = max_ok
            if not max_ok:
                reasons.append(
                    f"Phases: {len(phases)} > {expected_fields['max_phases']}"
                )

        passed = all(checks.values())
        details = {
            "checks": checks,
            "actual_type": actual_type,
            "title": parsed.get("title") or parsed.get("group_title"),
        }
        if phases:
            details["phase_count"] = len(phases)
            details["phase_titles"] = [p.get("phase_title", "") for p in phases]
        if reasons:
            details["reason"] = "; ".join(reasons)

        report.add(
            ScenarioResult(
                scenario_id=sid,
                category=scenario["category"],
                passed=passed,
                expected=expected_type,
                actual=actual_type,
                details=details,
            )
        )
        print("PASS" if passed else "FAIL")
        time.sleep(0.1)

    return report


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Rune scribe benchmark — tests capture quality via agent-delegated flow"
    )

    # Agent CLI mode (default)
    parser.add_argument(
        "--agent",
        default="claude",
        help=(
            "Agent CLI to use. Built-in: claude, gemini, codex. "
            "Or pass a custom command, e.g. 'my-agent --flag'. (default: claude)"
        ),
    )

    # API fallback mode (for CI / environments without CLI auth)
    api_group = parser.add_argument_group("API fallback (optional)")
    api_group.add_argument(
        "--api-key",
        default=None,
        help="Use direct API call instead of agent CLI",
    )
    api_group.add_argument(
        "--provider",
        default="anthropic",
        help="API provider: anthropic, openai (only with --api-key)",
    )
    api_group.add_argument(
        "--model",
        default=None,
        help="Model name (only with --api-key)",
    )

    parser.add_argument(
        "--mode",
        choices=["capture", "extraction", "all"],
        default="all",
        help="What to benchmark (default: all)",
    )
    parser.add_argument(
        "--report", type=Path, default=None, help="Save report to this path"
    )
    parser.add_argument(
        "--category",
        default=None,
        help="Filter to specific category (e.g. 'architecture', 'pr_review')",
    )
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Show raw LLM response on parse errors",
    )
    args = parser.parse_args()

    global VERBOSE
    VERBOSE = args.verbose

    # Build LLM call kwargs
    if args.api_key:
        llm_kwargs = {
            "provider": args.provider,
            "api_key": args.api_key,
            "model": args.model,
        }
        backend_desc = f"API: {args.provider} / {args.model or API_MODELS.get(args.provider)}"
    else:
        llm_kwargs = {"agent": args.agent}
        backend_desc = f"Agent CLI: {args.agent}"

    scribe_prompt = load_scribe_prompt()

    print(f"Backend: {backend_desc}")
    print(f"Scribe prompt: {len(scribe_prompt)} chars\n")

    if args.mode in ("capture", "all"):
        should_capture = load_scenarios("capture/should_capture")
        should_not_capture = load_scenarios("capture/should_not_capture")
        capture_scenarios = should_capture + should_not_capture

        if args.category:
            capture_scenarios = [
                s for s in capture_scenarios if args.category in s["category"]
            ]

        if capture_scenarios:
            print(f"=== Capture Benchmark ({len(capture_scenarios)} scenarios) ===\n")
            capture_report = evaluate_capture(
                capture_scenarios, scribe_prompt, llm_kwargs
            )
            capture_report.print_summary()

            if args.report:
                p = args.report.with_stem(args.report.stem + "-capture")
                capture_report.save(p)
                print(f"Report saved to: {p}")
            else:
                capture_report.save()

    if args.mode in ("extraction", "all"):
        extraction_scenarios = load_scenarios("extraction")

        if args.category:
            extraction_scenarios = [
                s for s in extraction_scenarios if args.category in s["category"]
            ]

        if extraction_scenarios:
            print(
                f"\n=== Extraction Benchmark ({len(extraction_scenarios)} scenarios) ===\n"
            )
            extraction_report = evaluate_extraction(
                extraction_scenarios, scribe_prompt, llm_kwargs
            )
            extraction_report.print_summary()

            if args.report:
                p = args.report.with_stem(args.report.stem + "-extraction")
                extraction_report.save(p)
                print(f"Report saved to: {p}")
            else:
                extraction_report.save()


if __name__ == "__main__":
    main()
