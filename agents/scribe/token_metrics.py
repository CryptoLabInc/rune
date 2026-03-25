#!/usr/bin/env python3
"""
Token metrics visualiser for Gemini recall calls.

Reads rune_results.json, runs the same synthesis prompt used in _force_recall,
and prints token usage as an ASCII bar chart.

Usage:
    python agents/scribe/token_metrics.py [path/to/rune_results.json]
"""

import json
import os
import sys

SCORE_ANOMALY = 1.0
MIN_SCORE = 0.4
MAX_TOKENS = 500
BAR_WIDTH = 40


def _bar(value, maximum, width=BAR_WIDTH) -> str:
    filled = round((value / maximum) * width) if maximum else 0
    filled = min(filled, width)
    return "[" + "█" * filled + "░" * (width - filled) + f"] {value}"


def _pct(value, maximum) -> str:
    if not maximum:
        return "  0%"
    return f"{round(value / maximum * 100):3d}%"


def run(path: str) -> None:
    with open(path) as f:
        raw = json.load(f)

    # Apply same filter as _force_recall
    valid = [
        r for r in raw
        if 0.0 < r.get("score", 0.0) <= SCORE_ANOMALY
        and r.get("score", 0.0) >= MIN_SCORE
        and r.get("certainty") != "unknown"
        and (r.get("payload_text") or "").strip()
    ][:5]

    if not valid:
        print("No records pass the filter (score 0.4-1.0, known certainty, non-empty payload).")
        return

    # Build same prompt as _synthesize_results
    records_text = [r["payload_text"].strip() for r in valid]
    query = "<your recall query>"
    prompt = (
        f'A user asked: "{query}"\n\n'
        f"Here are the relevant organisational memory records:\n\n"
        + "\n\n---\n\n".join(records_text)
        + "\n\nWrite a short, plain-English summary (3-5 sentences) that directly answers the user's question "
        "using only the information in these records. "
        "Do not use bullet points or headers — just clear readable prose. "
        "If multiple records say the same thing, consolidate them into one statement. "
        "Do not fabricate anything not present in the records.\n\n"
        "Summary:"
    )

    prompt_chars = len(prompt)
    # Rough token estimate: ~4 chars per token
    prompt_tokens_est = prompt_chars // 4

    print("=" * 60)
    print("  Rune → Gemini Token Metrics")
    print(f"  File   : {path}")
    print(f"  Records: {len(valid)} (after filtering from {len(raw)} total)")
    print("=" * 60)

    print(f"\n  Records sent to Gemini:")
    for r in valid:
        print(f"    • [{r['score']:.2f}] {r.get('title', 'Untitled')[:55]}  ({r.get('certainty')})")

    print(f"\n  Prompt size")
    print(f"    Characters : {prompt_chars:,}")
    print(f"    Tokens est : ~{prompt_tokens_est:,}  (chars ÷ 4)")

    # Try to get real token count from Gemini API
    api_key = None
    config_path = os.path.expanduser("~/.rune/config.json")
    if os.path.exists(config_path):
        with open(config_path) as f:
            cfg = json.load(f)
        api_key = (cfg.get("llm") or {}).get("google_api_key") or os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY")
        model_name = (cfg.get("llm") or {}).get("google_model", "gemini-2.0-flash")

    if not api_key or api_key == "YOUR_GEMINI_KEY_HERE":
        print("\n  No Gemini API key found — showing estimates only.")
        print(f"\n  Output budget : {MAX_TOKENS} tokens (max_tokens setting)")
        print(f"  Prompt est    : ~{prompt_tokens_est} tokens")
        remaining = MAX_TOKENS - prompt_tokens_est
        print(f"  Headroom      : ~{max(0, remaining)} tokens for response")
        print("\n" + "=" * 60)
        return

    try:
        import google.generativeai as genai
        genai.configure(api_key=api_key)
        model = genai.GenerativeModel(model_name=model_name)

        # Count prompt tokens (free API call, no generation)
        count_resp = model.count_tokens(prompt)
        prompt_tokens = count_resp.total_tokens

        # Run actual generation to get output tokens
        print(f"\n  Calling Gemini ({model_name}) for real token counts...")
        response = model.generate_content(
            prompt,
            generation_config={"max_output_tokens": MAX_TOKENS},
        )
        usage = response.usage_metadata
        output_tokens = getattr(usage, "candidates_token_count", 0)
        total_tokens = getattr(usage, "total_token_count", prompt_tokens + output_tokens)

        finish = "unknown"
        try:
            finish = response.candidates[0].finish_reason.name
        except Exception:
            pass

        at_limit = finish == "MAX_TOKENS"

        print(f"\n  ── Token Usage ────────────────────────────────────────")
        print(f"  Prompt (sent)    {_bar(prompt_tokens, prompt_tokens + MAX_TOKENS)}")
        print(f"  Output (received){_bar(output_tokens, MAX_TOKENS)}  / {MAX_TOKENS} max")
        print(f"  Total            {_bar(total_tokens, prompt_tokens + MAX_TOKENS)}")
        print()
        print(f"  Output used  : {_pct(output_tokens, MAX_TOKENS)} of {MAX_TOKENS} token budget")
        print(f"  Finish reason: {finish}  {'⚠️  HIT MAX — response was cut off!' if at_limit else '✓  completed normally'}")

        print(f"\n  ── Gemini Response ────────────────────────────────────")
        print()
        for line in response.text.strip().splitlines():
            print(f"  {line}")

    except ImportError:
        print("\n  google-generativeai not installed. Run: pip install google-generativeai")
    except Exception as e:
        print(f"\n  Gemini API error: {e}")

    print("\n" + "=" * 60)


if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), "rune_results.json")
    if not os.path.exists(path):
        print(f"File not found: {path}")
        sys.exit(1)
    run(path)
