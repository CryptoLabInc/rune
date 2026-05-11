#!/usr/bin/env python3
"""Rune × envector-msa-1.2.2 latency benchmark.

Measures wall-clock latency of each rune MCP feature broken down by pipeline
phase.  Runs standalone — no MCP server needed; adapters are imported directly.

Note: pyenvector 1.2.2 does not expose a `secure` parameter — TLS is enabled
automatically when `access_token` is supplied (cloud endpoint).

Scenarios
---------
  capture:
    T1  Short English text  (~30 tokens)
    T2  Long English text   (~150 tokens)
    T3  Korean text
    T4  Duplicate input     (novelty-check near-duplicate path)
  recall:
    T5  Exact match query
    T6  Cross-language semantic query (Korean → English)
    T7  topk scaling        (topk = 1, 3, 5, 10)
  batch_capture:
    T8  Batch size scaling  (sizes = 1, 5, 10, 20)
  vault_status:
    T9  Vault health check + diagnostics

Usage
-----
  python benchmark/runners/latency_bench.py
  python benchmark/runners/latency_bench.py --feature capture
  python benchmark/runners/latency_bench.py --runs 20
  python benchmark/runners/latency_bench.py \\
      --report benchmark/reports/latency_results_v1.4.0.md --format md
"""

from __future__ import annotations

import argparse
import asyncio
import base64
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Dict, List, Optional

import numpy as np

# ── path setup ────────────────────────────────────────────────────────────────
BENCHMARK_DIR = Path(__file__).resolve().parent.parent
RUNE_DIR = BENCHMARK_DIR.parent
MCP_DIR = RUNE_DIR / "mcp"

for _p in (str(RUNE_DIR), str(MCP_DIR), str(BENCHMARK_DIR)):
    if _p not in sys.path:
        sys.path.insert(0, _p)

from runners.common import (  # noqa: E402
    REPORTS_DIR,
    LatencyBenchReport,
    LatencyScenarioResult,
    PhaseLatency,
)

# ── sample inputs ─────────────────────────────────────────────────────────────

_SHORT_TEXT = (
    "We decided to use PostgreSQL as our primary database. "
    "Team familiarity and mature ecosystem were the key reasons. "
    "Redis considered but rejected due to durability concerns."
)

_LONG_TEXT = (
    "Full ADR: Context — our monolith hit 10k RPS limits. "
    "Considered: (1) horizontal scale with read replicas, "
    "(2) CQRS split, (3) microservices decomposition. "
    "Trade-offs: read replicas cheapest but doesn't solve write bottleneck; "
    "CQRS complex but keeps single codebase; microservices highest ops overhead. "
    "Decision: CQRS with event sourcing on order-service first. "
    "Rationale: allows independent scaling of read path, event log gives audit "
    "trail for compliance (legal requirement from Q2 review). "
    "Rollback plan: feature flag, revert within 2 sprints if P99 > 500ms."
)

_KOREAN_TEXT = (
    "Redis를 캐시 레이어로 사용하기로 결정했습니다. "
    "Memcached도 검토했지만 데이터 구조 지원(Sorted Set, List)이 필요해서 Redis로 확정. "
    "TTL은 1시간으로 설정. 담당: 백엔드팀."
)


def _make_extracted(text: str, title: str, domain: str = "architecture") -> str:
    """Minimal agent-delegated extracted JSON for capture benchmark."""
    return json.dumps({
        "tier2": {"capture": True, "domain": domain},
        "title": title,
        "rationale": "benchmark test scenario — no LLM involved",
        "problem": "benchmark setup",
        "alternatives": [],
        "trade_offs": [],
        "status_hint": "accepted",
        "tags": ["benchmark"],
        "reusable_insight": text[:120],
        "confidence": 0.9,
    }, ensure_ascii=False)


SCENARIOS_CAPTURE = [
    {
        "id": "T1_short_en",
        "text": _SHORT_TEXT,
        "title": "PostgreSQL chosen as primary DB",
        "domain": "architecture",
        "metadata": {"label": "short English (~30 tokens)", "tokens_approx": 35},
    },
    {
        "id": "T2_long_en",
        "text": _LONG_TEXT,
        "title": "CQRS event sourcing on order service",
        "domain": "architecture",
        "metadata": {"label": "long English (~150 tokens)", "tokens_approx": 155},
    },
    {
        "id": "T3_korean",
        "text": _KOREAN_TEXT,
        "title": "Redis 캐시 레이어 결정",
        "domain": "architecture",
        "metadata": {"label": "Korean text", "tokens_approx": 50},
    },
]

SCENARIOS_RECALL = [
    {
        "id": "T5_exact_match",
        "query": "Why did we choose PostgreSQL?",
        "topk": 5,
        "metadata": {"label": "exact match query"},
    },
    {
        "id": "T6_cross_lang",
        "query": "데이터베이스 선택 이유",
        "topk": 5,
        "metadata": {"label": "cross-language semantic (KO→EN)"},
    },
]

RECALL_TOPK_VARIANTS = [1, 3, 5, 10]

BATCH_SIZES = [1, 5, 10, 20]


# ── timing helpers ─────────────────────────────────────────────────────────────

class _Timer:
    """Simple context-manager wall-clock timer (ms)."""

    def __init__(self) -> None:
        self.elapsed_ms: float = 0.0
        self._start: float = 0.0

    def __enter__(self) -> "_Timer":
        self._start = time.perf_counter()
        return self

    def __exit__(self, *_: Any) -> None:
        self.elapsed_ms = (time.perf_counter() - self._start) * 1000.0


# ── main benchmark class ───────────────────────────────────────────────────────

class LatencyBenchmark:
    """
    Measures latency of each rune pipeline phase against a live envector-msa-1.4.0
    endpoint.  All adapter imports happen lazily so the module can be loaded
    even if optional deps are missing.
    """

    def __init__(self, runs: int = 10, warmup: int = 2) -> None:
        self.runs = runs
        self.warmup = warmup
        self._config: Any = None
        self._index_name: Optional[str] = None
        self._key_id: Optional[str] = None
        self._embedding: Any = None
        self._ev_client: Any = None
        self._vault: Any = None

    # ── setup ─────────────────────────────────────────────────────────────────

    async def setup(self) -> None:
        """Load config, fetch keys from Vault, init adapters."""
        from agents.common.config import load_config
        from agents.common.embedding_service import EmbeddingService
        from agents.common.envector_client import EnVectorClient
        from adapter.vault_client import VaultClient

        cfg = load_config()
        self._config = cfg

        print("  Connecting to Vault …", end=" ", flush=True)
        vault = VaultClient(
            vault_endpoint=cfg.vault.endpoint,
            vault_token=cfg.vault.token,
            ca_cert=cfg.vault.ca_cert or None,
            tls_disable=cfg.vault.tls_disable,
        )

        bundle = await vault.get_public_key()

        key_id = bundle.pop("key_id", None)
        index_name = bundle.pop("index_name", None)
        agent_id = bundle.pop("agent_id", None)
        agent_dek_b64 = bundle.pop("agent_dek", None)
        ev_endpoint = bundle.pop("envector_endpoint", None) or cfg.envector.endpoint
        ev_api_key = bundle.pop("envector_api_key", None) or cfg.envector.api_key

        if not key_id:
            raise RuntimeError("Vault did not return key_id")
        if not index_name:
            raise RuntimeError("Vault did not return index_name")

        key_path = Path.home() / ".rune" / "keys"
        key_dir = key_path / key_id
        key_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
        for filename, content in bundle.items():
            fp = key_dir / filename
            fd = os.open(str(fp), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, "w") as f:
                f.write(content)

        agent_dek: Optional[bytes] = None
        if agent_dek_b64:
            agent_dek = base64.b64decode(agent_dek_b64)

        self._index_name = index_name
        self._key_id = key_id
        self._vault = vault

        self._embedding = EmbeddingService(
            mode=cfg.embedding.mode,
            model=cfg.embedding.model,
        )

        self._ev_client = EnVectorClient(
            address=ev_endpoint,
            key_path=str(key_path),
            key_id=key_id,
            access_token=ev_api_key,
            auto_key_setup=False,
            agent_id=agent_id,
            agent_dek=agent_dek,
        )

        print("OK")
        print(f"    index  : {index_name}")
        print(f"    key_id : {key_id}")
        print(f"    endpoint: {ev_endpoint}")

    async def teardown(self) -> None:
        if self._vault is not None:
            await self._vault.close()

    # ── internal helpers ───────────────────────────────────────────────────────

    def _warmup_label(self, run_i: int) -> str:
        return "warmup" if run_i < self.warmup else f"run {run_i - self.warmup + 1}"

    def _valid_runs(self, all_timings: list[list[float]]) -> list[list[float]]:
        """Drop warmup runs, return effective samples."""
        return all_timings[self.warmup:]

    async def _single_capture_phases(
        self, text: str, title: str, domain: str
    ) -> dict[str, float]:
        """
        Run one capture and return per-phase latencies (ms).

        Phases:
          embed      — embed the reusable_insight locally
          score      — FHE-encrypted similarity search on enVector Cloud
          vault_topk — Vault TopK decrypt (gRPC)
          insert     — FHE encrypt + insert into enVector Cloud
          total      — wall clock for the whole call
        """
        reusable_insight = text[:120]

        total_start = time.perf_counter()

        # [1] Embed
        with _Timer() as t_embed:
            vec = self._embedding.embed_single(reusable_insight)
        embed_ms = t_embed.elapsed_ms

        # [2] Novelty score (encrypted similarity search)
        with _Timer() as t_score:
            score_res = self._ev_client.score(self._index_name, vec)
        score_ms = t_score.elapsed_ms

        # [3] Vault TopK decrypt
        vault_ms = 0.0
        blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
        if blobs:
            with _Timer() as t_vault:
                await self._vault.decrypt_search_results(blobs[0], top_k=3)
            vault_ms = t_vault.elapsed_ms

        # [4] Insert (embed internally + FHE encrypt + network)
        metadata = [{
            "id": f"bench-{domain}-{int(time.time()*1000)}",
            "title": title,
            "domain": domain,
            "status": "accepted",
            "reusable_insight": reusable_insight,
            "payload": {"text": text},
            "why": {"certainty": "supported"},
        }]
        with _Timer() as t_insert:
            self._ev_client.insert_with_text(
                index_name=self._index_name,
                texts=[reusable_insight],
                embedding_service=self._embedding,
                metadata=metadata,
            )
        insert_ms = t_insert.elapsed_ms

        total_ms = (time.perf_counter() - total_start) * 1000.0

        return {
            "embed": embed_ms,
            "score": score_ms,
            "vault_topk": vault_ms,
            "insert": insert_ms,
            "total": total_ms,
        }

    async def _single_recall_phases(
        self, query: str, topk: int
    ) -> dict[str, float]:
        """
        Run one recall and return per-phase latencies (ms).

        Phases:
          embed         — embed the query locally
          score         — FHE-encrypted similarity search
          vault_topk    — Vault TopK decrypt
          remind        — metadata retrieval from enVector
          total
        """
        total_start = time.perf_counter()

        # [1] Embed query
        with _Timer() as t_embed:
            vec = self._embedding.embed_single(query)
        embed_ms = t_embed.elapsed_ms

        # [2] Encrypted search
        with _Timer() as t_score:
            score_res = self._ev_client.score(self._index_name, vec)
        score_ms = t_score.elapsed_ms

        # [3] Vault TopK decrypt
        vault_ms = 0.0
        remind_ms = 0.0
        blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
        if blobs:
            with _Timer() as t_vault:
                vault_res = await self._vault.decrypt_search_results(blobs[0], top_k=topk)
            vault_ms = t_vault.elapsed_ms

            # [4] Metadata retrieval
            if vault_res.ok and vault_res.results:
                with _Timer() as t_remind:
                    self._ev_client.remind(
                        self._index_name,
                        vault_res.results,
                        output_fields=["metadata"],
                    )
                remind_ms = t_remind.elapsed_ms

        total_ms = (time.perf_counter() - total_start) * 1000.0

        return {
            "embed": embed_ms,
            "score": score_ms,
            "vault_topk": vault_ms,
            "remind": remind_ms,
            "total": total_ms,
        }

    def _build_phase_list(
        self,
        phase_names: list[str],
        all_timings: list[dict[str, float]],
    ) -> list[PhaseLatency]:
        """Convert list of per-run phase dicts → list of PhaseLatency."""
        valid = all_timings[self.warmup:]
        phases = []
        for name in phase_names:
            samples = [t[name] for t in valid if name in t]
            phases.append(PhaseLatency(name=name, samples_ms=samples))
        return phases

    # ── scenario runners ───────────────────────────────────────────────────────

    async def run_capture_scenario(self, scenario: dict) -> LatencyScenarioResult:
        sid = scenario["id"]
        text = scenario["text"]
        title = scenario["title"]
        domain = scenario["domain"]
        meta = scenario.get("metadata", {})

        print(f"  [{sid}] ", end="", flush=True)
        all_timings: list[dict[str, float]] = []

        for i in range(self.runs):
            label = self._warmup_label(i)
            print(f"{label} ", end="", flush=True)
            try:
                t = await self._single_capture_phases(text, title, domain)
                all_timings.append(t)
            except Exception as e:
                print(f"\n    ERROR on {label}: {e}")
                return LatencyScenarioResult(
                    scenario_id=sid,
                    feature="capture",
                    metadata=meta,
                    error=str(e),
                )

        print("done")
        phases = self._build_phase_list(
            ["embed", "score", "vault_topk", "insert", "total"],
            all_timings,
        )
        return LatencyScenarioResult(
            scenario_id=sid,
            feature="capture",
            phases=phases,
            metadata={**meta, "runs": self.runs - self.warmup},
        )

    async def run_capture_duplicate(self) -> LatencyScenarioResult:
        """T4: capture the same text twice — second should hit near-duplicate path."""
        sid = "T4_duplicate"
        text = _SHORT_TEXT
        title = "PostgreSQL chosen as primary DB (duplicate)"
        meta = {"label": "duplicate input — tests novelty near-duplicate path"}

        print(f"  [{sid}] ", end="", flush=True)
        all_timings: list[dict[str, float]] = []

        # First insert to ensure the record exists
        try:
            await self._single_capture_phases(text, title, "architecture")
        except Exception:
            pass

        # Now measure the duplicate path
        for i in range(self.runs):
            label = self._warmup_label(i)
            print(f"{label} ", end="", flush=True)
            try:
                t = await self._single_capture_phases(text, title, "architecture")
                all_timings.append(t)
            except Exception as e:
                print(f"\n    ERROR: {e}")
                return LatencyScenarioResult(
                    scenario_id=sid, feature="capture", metadata=meta, error=str(e)
                )

        print("done")
        phases = self._build_phase_list(
            ["embed", "score", "vault_topk", "insert", "total"],
            all_timings,
        )
        return LatencyScenarioResult(
            scenario_id=sid,
            feature="capture",
            phases=phases,
            metadata={**meta, "runs": self.runs - self.warmup},
        )

    async def run_recall_scenario(self, scenario: dict) -> LatencyScenarioResult:
        sid = scenario["id"]
        query = scenario["query"]
        topk = scenario.get("topk", 5)
        meta = scenario.get("metadata", {})

        print(f"  [{sid}] ", end="", flush=True)
        all_timings: list[dict[str, float]] = []

        for i in range(self.runs):
            label = self._warmup_label(i)
            print(f"{label} ", end="", flush=True)
            try:
                t = await self._single_recall_phases(query, topk)
                all_timings.append(t)
            except Exception as e:
                print(f"\n    ERROR: {e}")
                return LatencyScenarioResult(
                    scenario_id=sid, feature="recall", metadata=meta, error=str(e)
                )

        print("done")
        phases = self._build_phase_list(
            ["embed", "score", "vault_topk", "remind", "total"],
            all_timings,
        )
        return LatencyScenarioResult(
            scenario_id=sid,
            feature="recall",
            phases=phases,
            metadata={**meta, "topk": topk, "runs": self.runs - self.warmup},
        )

    async def run_recall_topk_scaling(self) -> list[LatencyScenarioResult]:
        """T7: measure recall latency at varying topk values."""
        results = []
        query = "architecture decisions"

        for topk in RECALL_TOPK_VARIANTS:
            sid = f"T7_topk_{topk}"
            print(f"  [{sid}] ", end="", flush=True)
            all_timings: list[dict[str, float]] = []
            runs = max(self.warmup + 3, min(self.runs, self.warmup + 5))

            for i in range(runs):
                label = self._warmup_label(i)
                print(f"{label} ", end="", flush=True)
                try:
                    t = await self._single_recall_phases(query, topk)
                    all_timings.append(t)
                except Exception as e:
                    print(f"\n    ERROR: {e}")
                    results.append(LatencyScenarioResult(
                        scenario_id=sid, feature="recall",
                        metadata={"topk": topk, "label": f"topk scaling topk={topk}"},
                        error=str(e),
                    ))
                    break
            else:
                print("done")
                phases = self._build_phase_list(
                    ["embed", "score", "vault_topk", "remind", "total"],
                    all_timings,
                )
                results.append(LatencyScenarioResult(
                    scenario_id=sid,
                    feature="recall",
                    phases=phases,
                    metadata={"topk": topk, "label": f"topk scaling topk={topk}",
                              "runs": runs - self.warmup},
                ))
        return results

    async def run_batch_capture_scaling(self) -> list[LatencyScenarioResult]:
        """T8: measure batch_capture latency at varying batch sizes."""
        results = []

        for bs in BATCH_SIZES:
            sid = f"T8_batch_{bs}"
            print(f"  [{sid}] ", end="", flush=True)
            runs = max(self.warmup + 1, min(3 + self.warmup, self.runs))

            total_samples: list[float] = []
            per_item_samples: list[float] = []
            error: Optional[str] = None

            # Build bs items
            items_texts = [
                f"Decision {i+1}: chose option A over B for batch benchmark (batch_size={bs})"
                for i in range(bs)
            ]

            for i in range(runs):
                label = self._warmup_label(i)
                print(f"{label} ", end="", flush=True)
                try:
                    t_start = time.perf_counter()
                    for text in items_texts:
                        vec = self._embedding.embed_single(text[:120])
                        self._ev_client.score(self._index_name, vec)
                        # skip vault+insert to keep test fast — measures embed+score overhead
                    t_total = (time.perf_counter() - t_start) * 1000.0
                    total_samples.append(t_total)
                    per_item_samples.append(t_total / bs)
                except Exception as e:
                    error = str(e)
                    print(f"\n    ERROR: {e}")
                    break

            if error:
                results.append(LatencyScenarioResult(
                    scenario_id=sid, feature="batch_capture",
                    metadata={"batch_size": bs}, error=error,
                ))
                continue

            print("done")
            valid_total = total_samples[self.warmup:]
            valid_per = per_item_samples[self.warmup:]
            phases = [
                PhaseLatency(name="total_batch", samples_ms=valid_total),
                PhaseLatency(name="per_item", samples_ms=valid_per),
            ]
            results.append(LatencyScenarioResult(
                scenario_id=sid,
                feature="batch_capture",
                phases=phases,
                metadata={
                    "batch_size": bs,
                    "label": f"batch size {bs} (embed+score per item)",
                    "runs": runs - self.warmup,
                },
            ))
        return results

    async def run_vault_status(self) -> LatencyScenarioResult:
        """T9: health check latency."""
        sid = "T9_vault_status"
        print(f"  [{sid}] ", end="", flush=True)
        samples: list[float] = []

        for i in range(self.runs):
            label = self._warmup_label(i)
            print(f"{label} ", end="", flush=True)
            with _Timer() as t:
                await self._vault.health_check()
            samples.append(t.elapsed_ms)

        print("done")
        valid = samples[self.warmup:]
        phases = [PhaseLatency(name="vault_health_check", samples_ms=valid)]
        return LatencyScenarioResult(
            scenario_id=sid,
            feature="vault_status",
            phases=phases,
            metadata={"label": "Vault gRPC health check", "runs": len(valid)},
        )

    # ── network baseline ───────────────────────────────────────────────────────

    def _measure_network_rtt(self) -> str:
        """ping envector endpoint, return mean RTT string."""
        host = (self._config.envector.endpoint or "").split(":")[0]
        if not host:
            return "unknown"
        try:
            out = subprocess.check_output(
                ["ping", "-c", "5", host],
                stderr=subprocess.DEVNULL,
                timeout=10,
            ).decode()
            for line in out.splitlines():
                if "avg" in line or "rtt" in line:
                    # e.g. "round-trip min/avg/max/stddev = 12.3/14.5/18.2/2.1 ms"
                    parts = line.split("=")[-1].strip().split("/")
                    if len(parts) >= 2:
                        return f"{parts[1]} ms (avg RTT)"
        except Exception:
            pass
        return "unknown"

    # ── orchestration ──────────────────────────────────────────────────────────

    async def run(
        self,
        feature_filter: Optional[str] = None,
    ) -> LatencyBenchReport:
        report = LatencyBenchReport()

        # Environment metadata
        rtt = self._measure_network_rtt()
        cfg = self._config
        report.env = {
            "date": __import__("datetime").date.today().isoformat(),
            "envector_endpoint": cfg.envector.endpoint,
            "vault_endpoint": cfg.vault.endpoint,
            "embedding_model": cfg.embedding.model,
            "embedding_mode": cfg.embedding.mode,
            "index_name": self._index_name,
            "key_id": self._key_id,
            "network_rtt": rtt,
            "runs_per_scenario": self.runs - self.warmup,
            "warmup_runs": self.warmup,
        }

        run_all = feature_filter is None
        run_capture = run_all or feature_filter == "capture"
        run_recall = run_all or feature_filter == "recall"
        run_batch = run_all or feature_filter == "batch_capture"
        run_vault = run_all or feature_filter == "vault_status"

        # ── capture ────────────────────────────────────────────────────────
        if run_capture:
            print("\n[capture]")
            for sc in SCENARIOS_CAPTURE:
                r = await self.run_capture_scenario(sc)
                report.add(r)
            r = await self.run_capture_duplicate()
            report.add(r)

        # ── recall ─────────────────────────────────────────────────────────
        if run_recall:
            print("\n[recall]")
            for sc in SCENARIOS_RECALL:
                r = await self.run_recall_scenario(sc)
                report.add(r)
            for r in await self.run_recall_topk_scaling():
                report.add(r)

        # ── batch_capture ──────────────────────────────────────────────────
        if run_batch:
            print("\n[batch_capture]")
            for r in await self.run_batch_capture_scaling():
                report.add(r)

        # ── vault_status ───────────────────────────────────────────────────
        if run_vault:
            print("\n[vault_status]")
            r = await self.run_vault_status()
            report.add(r)

        return report


# ── CLI ────────────────────────────────────────────────────────────────────────

def _print_summary(report: LatencyBenchReport) -> None:
    print("\n" + "=" * 64)
    print("  rune latency benchmark — summary")
    print("=" * 64)

    for s in report.scenarios:
        if s.error:
            print(f"  [FAIL] {s.scenario_id}: {s.error}")
            continue
        total_phase = next((p for p in s.phases if p.name == "total"), None)
        if total_phase and total_phase.samples_ms:
            print(
                f"  {s.scenario_id:<30} "
                f"p50={total_phase.p50:7.1f}ms  "
                f"p95={total_phase.p95:7.1f}ms  "
                f"n={total_phase.n}"
            )
        else:
            for p in s.phases:
                print(
                    f"  {s.scenario_id}/{p.name:<26} "
                    f"p50={p.p50:7.1f}ms  "
                    f"p95={p.p95:7.1f}ms  "
                    f"n={p.n}"
                )
    print("=" * 64 + "\n")


async def _main(args: argparse.Namespace) -> None:
    bench = LatencyBenchmark(runs=args.runs, warmup=args.warmup)

    print("\nSetting up …")
    await bench.setup()

    print(f"\nRunning benchmark (runs={args.runs - args.warmup} effective, warmup={args.warmup}) …")
    report = await bench.run(feature_filter=args.feature)

    await bench.teardown()

    _print_summary(report)

    if args.report:
        report_path = Path(args.report)
        if args.format == "json":
            saved = report.save_json(report_path)
        else:
            saved = report.save_markdown(report_path)
        print(f"Report saved → {saved}")
    else:
        print(report.to_markdown())


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Rune × envector-msa-1.4.0 latency benchmark"
    )
    parser.add_argument(
        "--feature",
        choices=["capture", "recall", "batch_capture", "vault_status"],
        default=None,
        help="Run only this feature (default: all)",
    )
    parser.add_argument(
        "--runs",
        type=int,
        default=10,
        help="Total runs per scenario including warmup (default: 10)",
    )
    parser.add_argument(
        "--warmup",
        type=int,
        default=2,
        help="Warmup runs to discard (default: 2)",
    )
    parser.add_argument(
        "--report",
        default=None,
        help="Path to save the report (default: print to stdout)",
    )
    parser.add_argument(
        "--format",
        choices=["md", "json"],
        default="md",
        help="Report format (default: md)",
    )
    args = parser.parse_args()

    if args.warmup >= args.runs:
        parser.error(f"--warmup ({args.warmup}) must be < --runs ({args.runs})")

    asyncio.run(_main(args))


if __name__ == "__main__":
    main()
