#!/usr/bin/env python3
"""Retriever benchmark runner.

Evaluates retriever quality using offline embedding similarity.
FHE is transparent to similarity scores, so offline cosine similarity
accurately predicts enVector Cloud recall performance.

Usage:
    python benchmark/runners/retriever_bench.py
    python benchmark/runners/retriever_bench.py --category exact_match
    python benchmark/runners/retriever_bench.py --report benchmark/reports/retriever.json
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

RUNE_DIR = Path(__file__).resolve().parent.parent.parent

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
sys.path.insert(0, str(RUNE_DIR / "agents"))

from runners.common import (
    BenchmarkReport,
    ScenarioResult,
    load_scenarios,
)


def evaluate_offline(scenarios: list[dict]) -> BenchmarkReport:
    """Evaluate recall using embedding similarity (no server needed)."""
    from common.embedding_service import EmbeddingService

    embedding_service = EmbeddingService()
    report = BenchmarkReport(bench_type="retriever")

    for i, scenario in enumerate(scenarios):
        sid = scenario["id"]
        seed_records = scenario["seed_records"]
        query = scenario["query"]
        expected_titles = scenario.get("expected_match_titles", [])
        min_score = scenario.get("min_score", 0.35)

        print(f"  [{i + 1}/{len(scenarios)}] {sid}...", end=" ", flush=True)

        # Embed seed records
        record_embeddings = []
        for record in seed_records:
            text = f"{record['title']}. {record['content']}"
            emb = embedding_service.embed(text)
            record_embeddings.append((record["title"], emb))

        # Embed query
        query_emb = embedding_service.embed(query)

        # Compute similarities
        scores = []
        for title, rec_emb in record_embeddings:
            sim = embedding_service.cosine_similarity(query_emb, rec_emb)
            scores.append((title, sim))

        scores.sort(key=lambda x: x[1], reverse=True)

        matched_titles = [t for t, s in scores if s >= min_score]
        hits = [t for t in expected_titles if t in matched_titles]
        passed = len(hits) == len(expected_titles)

        # MRR
        mrr_values = []
        for expected_title in expected_titles:
            for rank, (title, _) in enumerate(scores, 1):
                if title == expected_title:
                    mrr_values.append(1.0 / rank)
                    break
            else:
                mrr_values.append(0.0)
        mrr = sum(mrr_values) / len(mrr_values) if mrr_values else 0.0

        details: dict = {
            "scores": [(t, round(s, 4)) for t, s in scores],
            "matched_titles": matched_titles,
            "hits": hits,
            "mrr": round(mrr, 4),
            "min_score": min_score,
        }
        if not passed:
            missed = [t for t in expected_titles if t not in matched_titles]
            details["reason"] = f"Missing matches: {missed}"

        report.add(
            ScenarioResult(
                scenario_id=sid,
                category=scenario["category"],
                passed=passed,
                expected=expected_titles,
                actual=matched_titles,
                details=details,
            )
        )
        print("PASS" if passed else "FAIL")

    return report


def main() -> None:
    parser = argparse.ArgumentParser(description="Rune retriever benchmark")
    parser.add_argument(
        "--report", type=Path, default=None, help="Save report to this path"
    )
    parser.add_argument(
        "--category",
        default=None,
        help="Filter to specific category (e.g. 'exact_match', 'semantic_match')",
    )
    args = parser.parse_args()

    all_scenarios = load_scenarios("recall")

    if args.category:
        all_scenarios = [
            s for s in all_scenarios if args.category in s["category"]
        ]

    if not all_scenarios:
        print("No retriever scenarios found.", file=sys.stderr)
        sys.exit(1)

    print(f"=== Retriever Benchmark ({len(all_scenarios)} scenarios) ===\n")

    report = evaluate_offline(all_scenarios)
    report.print_summary()

    if args.report:
        saved = report.save(args.report)
        print(f"Report saved to: {saved}")
    else:
        report.save()


if __name__ == "__main__":
    main()
