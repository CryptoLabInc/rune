#!/usr/bin/env python3
"""
Deterministic formatter for rune_results.json.

Reads the exported search results and prints a clean, human-readable summary
without any LLM API calls.

Usage:
    python format_results.py [path/to/rune_results.json]
"""

import json
import sys
import os
from datetime import datetime

# Score thresholds (mirrors searcher.py)
SCORE_ANOMALY = 1.0   # scores above this are bad vectors — skip
MIN_SCORE     = 0.4   # scores below this are low relevance — de-emphasise


def _certainty_icon(certainty: str) -> str:
    return {
        "confirmed":  "[confirmed]",
        "supported":  "[supported]",
        "inferred":   "[inferred ]",
        "speculative":"[speculate]",
        "unknown":    "[unknown  ]",
    }.get((certainty or "unknown").lower(), "[unknown  ]")


def _status_label(status: str) -> str:
    return {
        "accepted": "ACCEPTED",
        "proposed": "proposed",
        "rejected": "rejected",
        "unknown":  "unknown ",
    }.get((status or "unknown").lower(), "unknown ")


def _score_bar(score: float, width: int = 10) -> str:
    """Visual bar for relevance score (0.0 – 1.0 range)."""
    clamped = max(0.0, min(score, 1.0))
    filled = round(clamped * width)
    return "[" + "#" * filled + "-" * (width - filled) + f"] {score:.3f}"


def _truncate(text: str, max_len: int = 280) -> str:
    text = (text or "").strip().replace("\n", " ")
    if len(text) > max_len:
        return text[:max_len - 1] + "…"
    return text


def _format_result(idx: int, r: dict) -> str:
    score     = r.get("score", 0.0)
    title     = r.get("title") or "Untitled"
    domain    = r.get("domain") or "—"
    certainty = r.get("certainty") or "unknown"
    status    = r.get("status") or "unknown"
    text      = r.get("payload_text") or ""
    record_id = r.get("record_id") or "—"
    group_id  = r.get("group_id")
    phase_seq = r.get("phase_seq")
    phase_total = r.get("phase_total")

    # Pull richer fields from nested metadata if present
    meta = r.get("metadata") or {}
    decision_what = (meta.get("decision") or {}).get("what") or ""
    decision_when = (meta.get("decision") or {}).get("when") or ""
    decision_where = (meta.get("decision") or {}).get("where") or ""
    problem = (meta.get("context") or {}).get("problem") or ""
    rationale = (meta.get("rationale") or {}).get("summary") or ""
    tags = meta.get("tags") or []

    lines = []
    lines.append(f"  {'─'*70}")
    lines.append(f"  #{idx}  {title}")
    lines.append(f"       Relevance : {_score_bar(score)}")
    lines.append(f"       Certainty : {_certainty_icon(certainty)}   Status: {_status_label(status)}")
    lines.append(f"       Domain    : {domain}   ID: {record_id}")

    if decision_when or decision_where:
        when_where = "   ".join(filter(None, [decision_when, decision_where]))
        lines.append(f"       When/Where: {when_where}")

    if tags:
        lines.append(f"       Tags      : {', '.join(tags)}")

    if group_id and phase_seq is not None:
        lines.append(f"       Group     : {group_id}  (phase {phase_seq}/{phase_total})")

    lines.append("")

    # Best available text: prefer decision.what > problem > payload snippet
    summary = decision_what or problem or text
    if summary:
        lines.append(f"       {_truncate(summary, 300)}")

    if rationale:
        lines.append("")
        lines.append(f"       Why: {_truncate(rationale, 200)}")

    return "\n".join(lines)


def format_results(path: str) -> str:
    with open(path) as f:
        results = json.load(f)

    if not isinstance(results, list):
        return "Error: expected a JSON array."

    # Partition: anomalous, low-relevance, good
    anomalous = [r for r in results if r.get("score", 0.0) > SCORE_ANOMALY]
    valid     = [r for r in results if r.get("score", 0.0) <= SCORE_ANOMALY]
    good      = [r for r in valid  if r.get("score", 0.0) >= MIN_SCORE]
    weak      = [r for r in valid  if r.get("score", 0.0) <  MIN_SCORE]

    # Sort good results by score descending
    good.sort(key=lambda r: r.get("score", 0.0), reverse=True)

    from datetime import timezone
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    lines = []
    lines.append("=" * 72)
    lines.append("  Rune Search Results")
    lines.append(f"  File   : {path}")
    lines.append(f"  Time   : {now}")
    lines.append(f"  Total  : {len(results)}  |  Good: {len(good)}  |  Weak: {len(weak)}  |  Anomalous: {len(anomalous)}")
    lines.append("=" * 72)

    if not valid:
        lines.append("\n  No valid results found.")
        return "\n".join(lines)

    if good:
        lines.append(f"\n  HIGH-RELEVANCE RESULTS  (score >= {MIN_SCORE})")
        for idx, r in enumerate(good, start=1):
            lines.append(_format_result(idx, r))
    else:
        lines.append("\n  No high-relevance results found.")

    if weak:
        lines.append(f"\n  LOW-RELEVANCE RESULTS  (score < {MIN_SCORE})  — included for completeness")
        for idx, r in enumerate(weak, start=len(good) + 1):
            lines.append(_format_result(idx, r))

    if anomalous:
        lines.append(f"\n  ANOMALOUS RESULTS SKIPPED  (score > {SCORE_ANOMALY})  — likely bad vectors")
        for r in anomalous:
            lines.append(f"    • record_id={r.get('record_id')}  score={r.get('score', 0.0):.3f}")

    lines.append("\n" + "=" * 72)
    return "\n".join(lines)


def _extract_section(text: str, heading: str) -> str:
    """Pull the content of a ## Heading section from markdown, stripping boilerplate."""
    import re
    pattern = rf"##\s+{re.escape(heading)}\s*\n(.*?)(?=\n##|\Z)"
    match = re.search(pattern, text, flags=re.DOTALL)
    if not match:
        return ""
    content = match.group(1).strip()
    # Drop lines that are pure boilerplate placeholders
    boilerplate = {
        "(not documented)", "(none documented)", "(no rationale documented)",
        "- (none documented)", "- (none documented)\n",
    }
    if content in boilerplate or not content:
        return ""
    # Strip leading "- " from single-item lists to read as prose
    lines = [ln.lstrip("- ").strip() for ln in content.splitlines() if ln.strip() and ln.strip() not in boilerplate]
    return " ".join(lines)


def format_results_from_records(query: str, records: list) -> str:
    """Pick the highest-scoring valid result and return it as plain English sentences.

    Args:
        query:   The original search query string.
        records: List of SearchResult dataclass instances.

    Returns:
        Plain-text string suitable for posting to Slack.
    """
    valid = [r for r in records if 0.0 < getattr(r, "score", 0.0) <= SCORE_ANOMALY]
    if not valid:
        return f"*Recall:* _{query}_\n\nNo relevant results found."

    best = max(valid, key=lambda r: getattr(r, "score", 0.0))
    r_dict = best.__dict__ if hasattr(best, "__dict__") else dict(best)

    title = r_dict.get("title") or "Untitled"
    text  = (r_dict.get("payload_text") or "").strip()

    if not text:
        return f"*Recall:* _{query}_\n\n{title}"

    decision   = _extract_section(text, "Decision")
    problem    = _extract_section(text, "Problem")
    rationale  = _extract_section(text, "Why (Rationale)")
    trade_offs = _extract_section(text, "Trade-offs")

    # Strip "Missing Information:..." and "Certainty:..." lines from rationale
    import re
    rationale = re.sub(r"Missing Information:.*", "", rationale, flags=re.DOTALL).strip()
    rationale = re.sub(r"Certainty:\s*\S+", "", rationale).strip()

    parts = []
    if decision:
        parts.append(decision)
    if problem:
        parts.append(f"The problem: {problem}.")
    if rationale:
        parts.append(f"Rationale: {rationale}.")
    if trade_offs:
        parts.append(f"Trade-offs: {trade_offs}.")

    body = " ".join(parts) if parts else title
    return f"*Recall:* _{query}_\n\n{body}"


if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), "rune_results.json")

    if not os.path.exists(path):
        print(f"File not found: {path}")
        sys.exit(1)

    print(format_results(path))
