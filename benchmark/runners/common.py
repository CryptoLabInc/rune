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


# ──────────────────────────────────────────────────────────
# Latency benchmark data structures
# ──────────────────────────────────────────────────────────

@dataclass
class PhaseLatency:
    """Latency statistics for a single pipeline phase."""
    name: str
    samples_ms: list = field(default_factory=list)

    def _pct(self, p: float) -> float:
        import numpy as np
        return float(np.percentile(self.samples_ms, p)) if self.samples_ms else 0.0

    @property
    def p50(self) -> float:
        return self._pct(50)

    @property
    def p95(self) -> float:
        return self._pct(95)

    @property
    def p99(self) -> float:
        return self._pct(99)

    @property
    def mean(self) -> float:
        import numpy as np
        return float(np.mean(self.samples_ms)) if self.samples_ms else 0.0

    @property
    def min_ms(self) -> float:
        return float(min(self.samples_ms)) if self.samples_ms else 0.0

    @property
    def max_ms(self) -> float:
        return float(max(self.samples_ms)) if self.samples_ms else 0.0

    @property
    def n(self) -> int:
        return len(self.samples_ms)

    def to_dict(self) -> dict:
        return {
            "phase": self.name,
            "n": self.n,
            "p50_ms": round(self.p50, 2),
            "p95_ms": round(self.p95, 2),
            "p99_ms": round(self.p99, 2),
            "mean_ms": round(self.mean, 2),
            "min_ms": round(self.min_ms, 2),
            "max_ms": round(self.max_ms, 2),
            "samples_ms": [round(s, 2) for s in self.samples_ms],
        }


@dataclass
class LatencyScenarioResult:
    """Latency results for a single test scenario."""
    scenario_id: str
    feature: str        # "capture", "recall", "batch_capture", "vault_status"
    phases: list = field(default_factory=list)   # list[PhaseLatency]
    metadata: dict = field(default_factory=dict)
    error: str | None = None

    def to_dict(self) -> dict:
        return {
            "scenario_id": self.scenario_id,
            "feature": self.feature,
            "phases": [p.to_dict() for p in self.phases],
            "metadata": self.metadata,
            "error": self.error,
        }


@dataclass
class LatencyBenchReport:
    """Aggregated latency benchmark report."""
    bench_type: str = "latency"
    env: dict = field(default_factory=dict)
    scenarios: list = field(default_factory=list)  # list[LatencyScenarioResult]

    def add(self, result: "LatencyScenarioResult") -> None:
        self.scenarios.append(result)

    def to_dict(self) -> dict:
        return {
            "bench_type": self.bench_type,
            "env": self.env,
            "scenarios": [s.to_dict() for s in self.scenarios],
        }

    def save_json(self, path: Path) -> Path:
        REPORTS_DIR.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(self.to_dict(), indent=2, ensure_ascii=False))
        return path

    def to_markdown(self) -> str:
        lines: list[str] = []
        lines.append("# Rune × envector-msa-1.2.2 Latency Report\n")
        lines.append("## Environment\n")
        for k, v in self.env.items():
            lines.append(f"- **{k}**: {v}")
        lines.append("")

        by_feature: dict[str, list] = {}
        for s in self.scenarios:
            by_feature.setdefault(s.feature, []).append(s)

        for feature, results in by_feature.items():
            lines.append(f"\n## Feature: `{feature}`\n")
            for res in results:
                lines.append(f"### {res.scenario_id}")
                if res.metadata:
                    for k, v in res.metadata.items():
                        lines.append(f"- {k}: {v}")
                if res.error:
                    lines.append(f"\n> **Error**: {res.error}\n")
                    continue
                lines.append("")
                header = "| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |"
                sep    = "|-------|---|--------|--------|--------|---------|"
                lines.append(header)
                lines.append(sep)
                for p in res.phases:
                    lines.append(
                        f"| {p.name} | {p.n} "
                        f"| {p.p50:.1f} | {p.p95:.1f} | {p.p99:.1f} | {p.mean:.1f} |"
                    )
                lines.append("")

        return "\n".join(lines)

    def save_markdown(self, path: Path) -> Path:
        REPORTS_DIR.mkdir(parents=True, exist_ok=True)
        path.write_text(self.to_markdown(), encoding="utf-8")
        return path

    # ── sweep report ──────────────────────────────────────────────────────

    def to_markdown_sweep(self, primer_rows: list) -> str:
        """N-axis sweep report — one table per scenario.

        Differs from `to_markdown()` by pivoting the axes. `to_markdown()`
        prints one table per scenario with rows = phase; that shape answers
        "where does this scenario spend its time?". A sweep instead asks
        "how does latency move as the index grows?", so here the rows are the
        primed index size N and the columns are per-phase p50 (ms), with
        `total` p95 tacked on so tail behaviour stays visible.

        Each sweep result carries its N in `metadata["sweep_n"]` (set by the
        runner's run_sweep). `primer_rows` fixes the row order — the grid is
        read in the order the operator measured it.
        """
        lines: list[str] = []
        sdk = self.env.get("sdk_version", "?")
        lines.append(f"# Rune Latency Sweep Report — pyenvector {sdk}\n")

        lines.append("## Environment\n")
        for k, v in self.env.items():
            lines.append(f"- **{k}**: {v}")
        lines.append("")

        # Group results by scenario; each scenario is measured once per N.
        # dict preserves first-seen order, so scenarios stay in run order.
        by_sid: dict[str, dict] = {}
        for s in self.scenarios:
            by_sid.setdefault(s.scenario_id, {})[s.metadata.get("sweep_n")] = s

        lines.append("\n## Sweep results\n")
        lines.append(
            "Rows = primed index size N. Columns = phase p50 (ms); the final "
            "column is `total` p95. A blank cell means that N was not measured "
            "for the scenario; `ERROR` means the scenario failed at that N.\n"
        )

        for sid, by_n in by_sid.items():
            lines.append(f"### {sid}\n")

            # Column order is taken from the first error-free result — every N
            # measures the same scenario, so the phase set is stable.
            phase_names: list[str] = []
            feature = ""
            for s in by_n.values():
                if not s.error and s.phases:
                    phase_names = [p.name for p in s.phases]
                    feature = s.feature
                    break

            if not phase_names:
                lines.append("> Every grid point errored — no phase data.\n")
                for N in primer_rows:
                    s = by_n.get(N)
                    if s is not None and s.error:
                        lines.append(f"- N={N}: `{s.error}`")
                lines.append("")
                continue

            lines.append(f"_feature: `{feature}`_\n")
            cols = len(phase_names) + 2  # N + one per phase + total p95
            lines.append(
                "| N | "
                + " | ".join(f"{p} p50" for p in phase_names)
                + " | total p95 |"
            )
            lines.append("|---" * cols + "|")

            for N in primer_rows:
                s = by_n.get(N)
                if s is None:
                    lines.append(f"| {N} |" + " |" * (cols - 1))
                    continue
                if s.error:
                    lines.append(
                        f"| {N} | " + " | ".join(["ERROR"] * (cols - 1)) + " |"
                    )
                    continue
                pmap = {p.name: p for p in s.phases}
                cells = []
                for name in phase_names:
                    p = pmap.get(name)
                    cells.append(f"{p.p50:.1f}" if p and p.samples_ms else "—")
                total = pmap.get("total")
                cells.append(
                    f"{total.p95:.1f}" if total and total.samples_ms else "—"
                )
                lines.append(f"| {N} | " + " | ".join(cells) + " |")
            lines.append("")

        return "\n".join(lines)

    def save_markdown_sweep(self, path: Path, primer_rows: list) -> Path:
        REPORTS_DIR.mkdir(parents=True, exist_ok=True)
        path.write_text(self.to_markdown_sweep(primer_rows), encoding="utf-8")
        return path
