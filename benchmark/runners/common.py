"""Shared utilities for rune benchmark runners."""

from __future__ import annotations

import json
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

BENCHMARK_DIR = Path(__file__).resolve().parent.parent
SCENARIOS_DIR = BENCHMARK_DIR / "scenarios"
REPORTS_DIR = BENCHMARK_DIR / "reports"
RUNE_DIR = BENCHMARK_DIR.parent  # rune repo root


@dataclass
class ScenarioResult:
    scenario_id: str
    category: str
    passed: bool
    expected: Any = None
    actual: Any = None
    details: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return asdict(self)


@dataclass
class BenchmarkReport:
    bench_type: str
    total: int = 0
    passed: int = 0
    failed: int = 0
    results: list[ScenarioResult] = field(default_factory=list)
    summary: dict = field(default_factory=dict)

    @property
    def accuracy(self) -> float:
        return self.passed / self.total if self.total > 0 else 0.0

    def add(self, result: ScenarioResult) -> None:
        self.results.append(result)
        self.total += 1
        if result.passed:
            self.passed += 1
        else:
            self.failed += 1

    def compute_summary(self) -> None:
        categories: dict[str, dict[str, int]] = {}
        for r in self.results:
            cat = r.category
            if cat not in categories:
                categories[cat] = {"total": 0, "passed": 0, "failed": 0}
            categories[cat]["total"] += 1
            if r.passed:
                categories[cat]["passed"] += 1
            else:
                categories[cat]["failed"] += 1

        self.summary = {
            "overall_accuracy": round(self.accuracy, 4),
            "total": self.total,
            "passed": self.passed,
            "failed": self.failed,
            "by_category": {
                cat: {
                    **stats,
                    "accuracy": round(
                        stats["passed"] / stats["total"] if stats["total"] else 0, 4
                    ),
                }
                for cat, stats in sorted(categories.items())
            },
        }

    def save(self, path: Path | None = None) -> Path:
        self.compute_summary()
        if path is None:
            REPORTS_DIR.mkdir(parents=True, exist_ok=True)
            path = REPORTS_DIR / f"{self.bench_type}_report.json"
        path.write_text(json.dumps(self.to_dict(), indent=2, ensure_ascii=False))
        return path

    def to_dict(self) -> dict:
        return {
            "bench_type": self.bench_type,
            "summary": self.summary,
            "results": [r.to_dict() for r in self.results],
        }

    def print_summary(self) -> None:
        self.compute_summary()
        print(f"\n{'=' * 60}")
        print(f"  rune benchmark: {self.bench_type}")
        print(f"{'=' * 60}")
        print(
            f"  Overall: {self.passed}/{self.total} passed "
            f"({self.summary['overall_accuracy']:.1%})"
        )
        print(f"{'─' * 60}")

        for cat, stats in self.summary["by_category"].items():
            status = "PASS" if stats["failed"] == 0 else "FAIL"
            print(
                f"  [{status}] {cat}: "
                f"{stats['passed']}/{stats['total']} ({stats['accuracy']:.0%})"
            )

        print(f"{'=' * 60}\n")

        failed = [r for r in self.results if not r.passed]
        if failed:
            print(f"  Failed scenarios ({len(failed)}):")
            for r in failed:
                print(f"    - {r.scenario_id}: {r.details.get('reason', 'unknown')}")
            print()


def load_scenarios(category_prefix: str) -> list[dict]:
    """Load all scenarios matching a category prefix from JSONL files."""
    scenarios = []
    search_dir = SCENARIOS_DIR / category_prefix
    if not search_dir.exists():
        print(f"Warning: directory not found: {search_dir}", file=sys.stderr)
        return scenarios

    for jsonl_file in sorted(search_dir.rglob("*.jsonl")):
        with open(jsonl_file) as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    scenarios.append(json.loads(line))
                except json.JSONDecodeError as e:
                    print(
                        f"Warning: invalid JSON in {jsonl_file}:{line_num}: {e}",
                        file=sys.stderr,
                    )
    return scenarios


def check_title_keywords(title: str, keywords: list[str]) -> bool:
    """Check if a title contains any of the expected keywords (case-insensitive)."""
    title_lower = title.lower()
    return any(kw.lower() in title_lower for kw in keywords)
