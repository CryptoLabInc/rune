#!/usr/bin/env python3
"""Rune latency benchmark — unified runner for pyenvector 1.2.2 and 1.4.3.

A single runner that targets either SDK version. `get_sdk_adapter()` detects
the installed `pyenvector.__version__` and returns the matching adapter
(V122Adapter / V143Adapter). Everything version-specific lives behind the
`SdkAdapter` interface in `benchmark/runners/sdk/`; this runner stays
SDK-agnostic.

  - pyenvector 1.2.2 → eval_mode=rmp,  index_type=flat
  - pyenvector 1.4.3 → eval_mode=mm32, index_type=ivf_vct

Same scenario IDs (T1–T14) and phase decomposition on both, so the two SDK
configurations can be compared on identical scenarios.

insert_mode:
  - 1.2.2 has ONLY a batch insert path (Index.insert always routes through
    _insert_bulk; there is no single-row API). `--insert-mode single` is
    accepted but pinned to batch, and the pinning is logged at setup.
  - 1.4.3 has a single-row path (use_row_insert); `--insert-mode` toggles it.

searchable measurement differs by SDK and is NOT directly comparable
per-phase — only the total insert→searchable time is:
  - 1.2.2 — client-side score polling (top-1 cosine ≥ 0.999); single phase.
  - 1.4.3 — server lifecycle, 3 phases (insert_rpc / load / wait).

`--direct-envector` mode:
  - Provisions a dedicated `runecontext_bench` index instead of touching the
    live `runecontext` index.
  - Drops + recreates the bench index between scenarios so each scenario's
    latency numbers start from a known empty state.
  - For recall scenarios, primes the bench index with deterministic random
    records (RNG seed 0xBEEF) before measurement.
  - On teardown, drops the bench index.

Scenarios
---------
  capture:       T1 short EN / T2 long EN / T3 Korean / T4 duplicate
  recall:        T5 exact match / T6 cross-language / T7 topk scaling
  vault_status:  T9 vault health check
  multi_capture: T13 2-phase / T14 5-phase
  searchable:    T10 short EN / T11 long EN / T12 Korean

Usage
-----
  python benchmark/runners/latency_bench.py --insert-mode single
  python benchmark/runners/latency_bench.py \\
      --insert-mode single --feature capture --runs 5

  # Bench-index mode with per-scenario reset:
  python benchmark/runners/latency_bench.py \\
      --insert-mode single --direct-envector --runs 10 --warmup 2 \\
      --report benchmark/reports/latency_results_<sdk>_<date>.md
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
from runners.sdk import SearchableCtx, get_sdk_adapter  # noqa: E402

# ── constants ─────────────────────────────────────────────────────────────────

# eval_mode / index_type are no longer module constants — they are properties
# of the SDK adapter (V122Adapter: rmp/flat, V143Adapter: mm32/ivf_vct) and
# are resolved at runtime by get_sdk_adapter().

# Bench-index dimension, used only when --direct-envector is set. The index
# type is supplied by the adapter (adapter.index_type), so there is no params
# dict here.
BENCH_DIM = 1024

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

_MULTI_2_PHASE = [
    (
        "We chose PostgreSQL as the primary database. "
        "Team familiarity and mature ecosystem were decisive factors."
    ),
    (
        "Redis selected for session caching layer. "
        "TTL set to 30 minutes. Memcached rejected due to lack of data structure support."
    ),
]

_MULTI_5_PHASE = [
    (
        "Context: monolith hit 10k RPS ceiling. "
        "Decision: migrate to event-driven microservices architecture."
    ),
    (
        "Auth service extracted first. OAuth2 with JWT chosen. "
        "Session cookies rejected for statelessness requirement."
    ),
    (
        "Order service adopts CQRS. Write path via Kafka, "
        "read path via PostgreSQL read replicas."
    ),
    (
        "API gateway with per-tenant rate limiting (1000 req/s). "
        "Nginx selected over custom solution for operational maturity."
    ),
    (
        "Deployment on Kubernetes with Helm charts. "
        "Blue-green strategy for zero-downtime releases. Rollback within one sprint."
    ),
]

SCENARIOS_MULTI_CAPTURE = [
    {
        "id": "T13_multi_2phase",
        "texts": _MULTI_2_PHASE,
        "domain": "architecture",
        "metadata": {"label": "2-phase multi-capture", "phase_count": 2},
    },
    {
        "id": "T14_multi_5phase",
        "texts": _MULTI_5_PHASE,
        "domain": "architecture",
        "metadata": {"label": "5-phase multi-capture", "phase_count": 5},
    },
]


# ── timing helper ─────────────────────────────────────────────────────────────

class _Timer:
    def __init__(self) -> None:
        self.elapsed_ms: float = 0.0
        self._start: float = 0.0

    def __enter__(self) -> "_Timer":
        self._start = time.perf_counter()
        return self

    def __exit__(self, *_: Any) -> None:
        self.elapsed_ms = (time.perf_counter() - self._start) * 1000.0


# ── benchmark class ───────────────────────────────────────────────────────────

class LatencyBenchmark:
    """
    Latency benchmark for envector-msa-1.4.3 (eval_mode=mm32, index_type=ivf_vct).

    insert_mode controls how vectors are submitted during capture scenarios:
      "single" — index.insert(data=[vec]) called once per vector
      "batch"  — index.insert(data=[v1,...,vN]) called once per batch
    """

    def __init__(
        self,
        runs: int = 10,
        warmup: int = 2,
        insert_mode: str = "single",
        direct_envector: bool = False,
        bench_index_name: str = "runecontext_bench",
    ) -> None:
        self.runs = runs
        self.warmup = warmup
        self.insert_mode = insert_mode
        self.direct_envector = direct_envector
        self.bench_index_name = bench_index_name
        self._config: Any = None
        self._index_name: Optional[str] = None
        self._key_id: Optional[str] = None
        self._embedding: Any = None
        self._adapter: Any = None       # SdkAdapter, built by setup()
        self._vault: Any = None
        # Populated by _setup_*; used by _prime_bench_index for metadata wire encrypt.
        self._agent_dek: Optional[bytes] = None

    # ── setup ─────────────────────────────────────────────────────────────────

    async def setup(self) -> None:
        from agents.common.config import load_config

        cfg = load_config()
        self._config = cfg

        # Detect the installed pyenvector version and pick the matching
        # adapter (V122Adapter / V143Adapter). Raises on an unsupported version.
        self._adapter = get_sdk_adapter()

        if self.direct_envector:
            await self._setup_direct_envector()
        else:
            await self._setup_vault()

    def _connect_adapter(
        self,
        *,
        address: str,
        key_id: str,
        key_path: str,
        access_token: Optional[str],
        agent_id: Optional[str],
        agent_dek: Optional[bytes],
        secure: Optional[bool],
    ) -> None:
        """Wire self._adapter to the cluster.

        `secure` may be None — the Vault bundle did not include
        `envector_secure`. In that case it is omitted from connect() so the
        adapter's secure-by-default (True) applies.
        """
        kwargs: dict = dict(
            address=address,
            key_id=key_id,
            key_path=key_path,
            access_token=access_token,
            agent_id=agent_id,
            agent_dek=agent_dek,
        )
        if secure is not None:
            kwargs["secure"] = secure
        self._adapter.connect(**kwargs)

    async def _setup_vault(self) -> None:
        """Default path: use the live runecontext index via Vault-issued bundle."""
        from agents.common.embedding_service import EmbeddingService
        from adapter.vault_client import VaultClient

        cfg = self._config

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
        # `envector_secure` is the 1.4.x TLS toggle. Keep its value to forward
        # to the adapter (v1.2.2 ignores it); pop it so it is not written as a
        # cert file alongside the remaining bundle fields below.
        ev_secure = bundle.pop("envector_secure", None)

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
        self._agent_dek = agent_dek

        self._embedding = EmbeddingService(
            mode=cfg.embedding.mode,
            model=cfg.embedding.model,
        )

        self._connect_adapter(
            address=ev_endpoint,
            key_id=key_id,
            key_path=str(key_path),
            access_token=ev_api_key,
            agent_id=agent_id,
            agent_dek=agent_dek,
            secure=ev_secure,
        )

        print("OK")
        print(f"    sdk        : pyenvector {self._adapter.sdk_version}")
        print(f"    index      : {index_name}")
        print(f"    key_id     : {key_id}")
        print(f"    endpoint   : {ev_endpoint}")
        print(f"    eval_mode  : {self._adapter.eval_mode}")
        print(f"    index_type : {self._adapter.index_type}")
        print(f"    insert_mode: {self.insert_mode}")

    async def _setup_direct_envector(self) -> None:
        """Benchmark-index mode (ported from the v1.4.3 reference, v1.2.2 adapted).

        Connects to Vault the same way the production path does (for `vault-key`,
        `agent_dek`, and envector credentials), but overrides the bundle's
        `index_name` with `self.bench_index_name` so a dedicated bench index is
        used instead of the live `runecontext`. The bench index is dropped +
        recreated as FLAT (dim=1024) so this runner can never touch live data.

        Vault is still used for FHE score decryption (the SecKey only lives on
        Vault — same as production).
        """
        from agents.common.embedding_service import EmbeddingService
        from adapter.vault_client import VaultClient

        cfg = self._config

        print("  Connecting to Vault (for benchmark index mode)...", end=" ", flush=True)
        vault = VaultClient(
            vault_endpoint=cfg.vault.endpoint,
            vault_token=cfg.vault.token,
            ca_cert=cfg.vault.ca_cert or None,
            tls_disable=cfg.vault.tls_disable,
        )
        bundle = await vault.get_public_key()

        key_id = bundle.pop("key_id", None)
        bundle.pop("index_name", None)  # discard live index; we use bench_index_name
        agent_id = bundle.pop("agent_id", None)
        agent_dek_b64 = bundle.pop("agent_dek", None)
        ev_endpoint = bundle.pop("envector_endpoint", None) or cfg.envector.endpoint
        ev_api_key = bundle.pop("envector_api_key", None) or cfg.envector.api_key
        # `envector_secure` is the 1.4.x TLS toggle — keep its value for the
        # adapter (v1.2.2 ignores it).
        ev_secure = bundle.pop("envector_secure", None)

        if not key_id:
            raise RuntimeError("Vault did not return key_id")

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

        self._index_name = self.bench_index_name
        self._key_id = key_id
        self._vault = vault
        self._agent_dek = agent_dek

        self._embedding = EmbeddingService(
            mode=cfg.embedding.mode,
            model=cfg.embedding.model,
        )

        # adapter.connect() builds the EnVectorSDKAdapter with auto_key_setup=
        # False (ev.init() must not unload `vault-key` while the live
        # runecontext index still references it) and retries the initial
        # handshake internally — so the runner no longer needs a retry loop.
        self._connect_adapter(
            address=ev_endpoint,
            key_id=key_id,
            key_path=str(key_path),
            access_token=ev_api_key,
            agent_id=agent_id,
            agent_dek=agent_dek,
            secure=ev_secure,
        )

        # Clean start: drop any leftover bench index from prior runs.
        self._reset_bench_index()

        print("OK")
        print(f"    sdk        : pyenvector {self._adapter.sdk_version}")
        print(f"    index      : {self._index_name}  (bench-only, separate from runecontext)")
        print(f"    key_id     : {self._key_id}  (shared with production - read-only here)")
        print(f"    endpoint   : {ev_endpoint}")
        print(f"    eval_mode  : {self._adapter.eval_mode}")
        print(f"    index_type : {self._adapter.index_type}")
        print(f"    insert_mode: {self.insert_mode}")
        print(f"    reset      : per-scenario drop+create")

    # ── bench-index helpers (direct_envector only) ────────────────────────────

    def _reset_bench_index(self) -> None:
        """Drop + recreate the bench index. Refuses to touch `runecontext`.

        The cluster's drop is async and `get_index_list` keeps the name visible
        long after drop is accepted, so we can't poll the listing for completion.
        Instead poll create_index — it succeeds the moment the drop fully retires.
        """
        if not self.direct_envector:
            raise RuntimeError(
                "_reset_bench_index is bench-index mode only - refusing to "
                "drop the production index."
            )
        if self._index_name == "runecontext":
            raise RuntimeError(
                f"_reset_bench_index refusing to operate on production index "
                f"name 'runecontext' - set --bench-index to something else."
            )

        # Each adapter call (list / drop / create) wraps its own
        # _with_reconnect, so no outer reconnect wrapper is needed here.
        # create_index uses the adapter's index_type (flat for 1.2.2,
        # ivf_vct for 1.4.3).
        if self._index_name in self._adapter.list_index_names():
            self._adapter.drop_index(self._index_name)

        deadline = time.monotonic() + 180.0
        saw_being_deleted = False
        last_err: Optional[Exception] = None
        while time.monotonic() < deadline:
            try:
                self._adapter.create_index(self._index_name, BENCH_DIM)
                return
            except Exception as e:
                last_err = e
                msg = str(e).lower()
                if "being deleted" in msg or "notready" in msg:
                    saw_being_deleted = True
                    time.sleep(2.0)
                    continue
                raise

        if saw_being_deleted:
            raise RuntimeError(
                f"_reset_bench_index: bench index {self._index_name!r} "
                f"is stuck in 'being deleted' state - drop_index returns "
                f"ok but the cluster never completes the delete. "
                f"Workaround: rerun with --bench-index <fresh-name>. "
                f"Last error: {last_err}"
            )
        raise last_err if last_err is not None else RuntimeError(
            f"_reset_bench_index: timed out without ever calling "
            f"create_index for {self._index_name!r}"
        )

    def _ensure_index_loaded(self) -> None:
        """Pre-load the bench index. Safe to call repeatedly (idempotent).

        The connection handshake + retry already happened in adapter.connect();
        adapter.load_index() wraps its own _with_reconnect, so this is now a
        one-liner.
        """
        self._adapter.load_index(self._index_name)

    def _wait_for_score_ready(
        self, probe_vec: list, timeout_s: float = 300.0, poll_interval_s: float = 1.0
    ) -> float:
        """Poll score() until it returns ok; return the wait duration.

        After a v1.2.2 insert the SDK gives no signal that the new vectors are
        actually queryable — calling score() before the cluster has stabilised
        can return errors like "shard list is empty". This helper is called
        outside the measured window so the next iteration starts from a known
        searchable state.
        """
        start = time.monotonic()
        deadline = start + timeout_s
        last_err: Optional[str] = None
        while time.monotonic() < deadline:
            res = self._adapter.score(self._index_name, probe_vec)
            if res.get("ok"):
                return time.monotonic() - start
            last_err = res.get("error")
            time.sleep(poll_interval_s)
        raise RuntimeError(
            f"score-ready wait timed out after {timeout_s}s. "
            f"Last error: {last_err}"
        )

    async def _vault_decrypt_with_retry(
        self,
        encrypted_blob: str,
        top_k: int,
        max_attempts: int = 5,
    ):
        """Wrap vault.decrypt_search_results with RESOURCE_EXHAUSTED backoff.

        Per-scenario reset + priming + 10 measurement runs across 14 scenarios
        puts more sustained load on Vault than the prior 5/12 single-shot
        measurement did, so this guard matters even though 5/12 did not hit
        the rate limiter.
        """
        import re
        last_err: Optional[Exception] = None
        for attempt in range(max_attempts):
            try:
                return await self._vault.decrypt_search_results(
                    encrypted_blob, top_k=top_k
                )
            except Exception as e:
                last_err = e
                msg = str(e)
                if "RESOURCE_EXHAUSTED" not in msg and "Rate limit" not in msg:
                    raise
                m = re.search(r"Retry after (\d+(?:\.\d+)?)\s*s", msg)
                delay = float(m.group(1)) + 1.0 if m else 25.0
                print(
                    f"\n    Vault rate limit (attempt {attempt + 1}/{max_attempts}): "
                    f"sleeping {delay:.1f}s",
                    flush=True,
                )
                await asyncio.sleep(delay)
        assert last_err is not None
        raise last_err

    def _prime_bench_index(self, n_records: int = 20) -> None:
        """Insert deterministic random records so recall has data to score.

        RNG seed 0xBEEF matches the v1.4.3 reference exactly, so the priming
        vectors are deterministic across SDK versions. The adapter's insert()
        handles metadata JSON-encoding and app-layer encryption; we call
        `_wait_for_score_ready` once at the end so the recall scenario starts
        on a queryable state.
        """
        if not self.direct_envector:
            return

        rng = np.random.default_rng(0xBEEF)

        print(
            f"  priming {self._index_name} with {n_records} records...",
            end=" ", flush=True,
        )
        start = time.monotonic()
        last_vec: Optional[list] = None
        for i in range(n_records):
            vec = rng.standard_normal(BENCH_DIM).astype(np.float32).tolist()
            last_vec = vec
            meta = self._build_insert_metadata(
                f"priming record {i}", f"prime-{i}", "priming"
            )
            self._adapter.insert(self._index_name, [vec], [meta])

        # Make sure the recall scenario's first score() doesn't trip on a
        # half-stable index.
        if last_vec is not None:
            self._wait_for_score_ready(last_vec)

        elapsed = time.monotonic() - start
        print(f"done in {elapsed:.1f}s")

    async def teardown(self) -> None:
        # Drop the bench index — only when --direct-envector was used so the
        # production runecontext is never touched.
        if self.direct_envector and self._index_name and self._adapter is not None:
            try:
                self._adapter.drop_index(self._index_name)
                print(f"  teardown: drop_index({self._index_name!r}) queued")
            except Exception as e:
                print(f"  teardown: drop_index failed (non-fatal): {e}")

        if self._vault is not None:
            await self._vault.close()

    # ── helpers ───────────────────────────────────────────────────────────────

    def _warmup_label(self, run_i: int) -> str:
        return "warmup" if run_i < self.warmup else f"run {run_i - self.warmup + 1}"

    def _build_phase_list(
        self,
        phase_names: list[str],
        all_timings: list[dict[str, float]],
    ) -> list[PhaseLatency]:
        valid = all_timings[self.warmup:]
        phases = []
        for name in phase_names:
            samples = [t[name] for t in valid if name in t]
            phases.append(PhaseLatency(name=name, samples_ms=samples))
        return phases

    def _build_insert_metadata(self, text: str, title: str, domain: str) -> dict:
        return {
            "id": f"bench-{domain}-{int(time.time()*1000)}",
            "title": title,
            "domain": domain,
            "status": "accepted",
            "reusable_insight": text[:120],
            "payload": {"text": text},
            "why": {"certainty": "supported"},
        }

    # ── single capture phases ─────────────────────────────────────────────────

    async def _single_capture_phases(
        self, text: str, title: str, domain: str, batch_size: int = 1
    ) -> dict[str, float]:
        """
        Run one capture iteration and return per-phase latencies (ms).

        The adapter's insert() honours row_insert on 1.4.x (single-row API)
        and ignores it on 1.2.2 (batch-only path). insert_mode=="single" maps
        to row_insert=True.

        Phases: embed / score / vault_topk / insert / total
        """
        reusable_insight = text[:120]
        total_start = time.perf_counter()

        # [1] Embed
        with _Timer() as t_embed:
            vec = self._embedding.embed_single(reusable_insight)
        embed_ms = t_embed.elapsed_ms

        # [2] Novelty score (encrypted similarity search)
        with _Timer() as t_score:
            score_res = self._adapter.score(self._index_name, vec)
        score_ms = t_score.elapsed_ms

        # [3] Vault TopK decrypt
        vault_ms = 0.0
        blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
        if blobs:
            with _Timer() as t_vault:
                await self._vault_decrypt_with_retry(blobs[0], top_k=3)
            vault_ms = t_vault.elapsed_ms

        # [4] Insert
        if self.insert_mode == "batch" and batch_size > 1:
            vectors = [vec] * batch_size
            metadata = [self._build_insert_metadata(text, title, domain) for _ in range(batch_size)]
        else:
            vectors = [vec]
            metadata = [self._build_insert_metadata(text, title, domain)]

        with _Timer() as t_insert:
            self._adapter.insert(
                self._index_name,
                vectors,
                metadata,
                row_insert=(self.insert_mode == "single"),
            )
        insert_ms = t_insert.elapsed_ms

        total_ms = (time.perf_counter() - total_start) * 1000.0

        # Outside the measured window: wait until the index reflects this insert
        # so the next iteration's score() doesn't trip on a transient empty-shard
        # error. Bench-index mode only — in vault mode we don't own the index.
        if self.direct_envector:
            self._wait_for_score_ready(vec)

        return {
            "embed": embed_ms,
            "score": score_ms,
            "vault_topk": vault_ms,
            "insert": insert_ms,
            "total": total_ms,
        }

    # ── recall phases ─────────────────────────────────────────────────────────

    async def _single_recall_phases(
        self, query: str, topk: int
    ) -> dict[str, float]:
        """Phases: embed / score / vault_topk / remind / total"""
        total_start = time.perf_counter()

        with _Timer() as t_embed:
            vec = self._embedding.embed_single(query)
        embed_ms = t_embed.elapsed_ms

        with _Timer() as t_score:
            score_res = self._adapter.score(self._index_name, vec)
        score_ms = t_score.elapsed_ms

        vault_ms = 0.0
        remind_ms = 0.0
        blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
        if blobs:
            with _Timer() as t_vault:
                vault_res = await self._vault_decrypt_with_retry(blobs[0], top_k=topk)
            vault_ms = t_vault.elapsed_ms

            if vault_res.ok and vault_res.results:
                with _Timer() as t_remind:
                    self._adapter.remind(
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
            metadata={**meta, "insert_mode": self.insert_mode, "runs": self.runs - self.warmup},
        )

    async def run_capture_duplicate(self) -> LatencyScenarioResult:
        """T4: capture same text twice — second hits near-duplicate path."""
        sid = "T4_duplicate"
        text = _SHORT_TEXT
        title = "PostgreSQL chosen as primary DB (duplicate)"
        meta = {"label": "duplicate input — tests novelty near-duplicate path"}

        print(f"  [{sid}] ", end="", flush=True)
        all_timings: list[dict[str, float]] = []

        try:
            await self._single_capture_phases(text, title, "architecture")
        except Exception:
            pass

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
            metadata={**meta, "insert_mode": self.insert_mode, "runs": self.runs - self.warmup},
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

    async def _searchable_capture_phases(
        self, text: str, title: str, domain: str
    ) -> dict[str, float]:
        """Measure time from capture start until data is searchable.

        embed / score / vault_topk are measured here (SDK-agnostic). The
        insert->searchable segment is delegated to the adapter, whose phase
        breakdown differs by SDK version:
          - 1.2.2 -> 1 phase  (client score polling)
          - 1.4.3 -> 3 phases (insert_rpc / load_index / wait_searchable)

        Phases: embed / score / vault_topk / <adapter searchable phases> / total
        """
        reusable_insight = text[:120]
        total_start = time.perf_counter()

        with _Timer() as t_embed:
            vec = self._embedding.embed_single(reusable_insight)
        embed_ms = t_embed.elapsed_ms

        with _Timer() as t_score:
            score_res = self._adapter.score(self._index_name, vec)
        score_ms = t_score.elapsed_ms

        vault_ms = 0.0
        blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
        if blobs:
            with _Timer() as t_vault:
                await self._vault_decrypt_with_retry(blobs[0], top_k=3)
            vault_ms = t_vault.elapsed_ms

        # Delegate the insert->searchable measurement to the adapter; it
        # returns {phase_name: ms} with SDK-specific phase names.
        ctx = SearchableCtx(
            index_name=self._index_name,
            vec=vec,
            metadata=[self._build_insert_metadata(text, title, domain)],
            vault=self._vault,
            insert_mode=self.insert_mode,
        )
        searchable_phases = await self._adapter.measure_insert_to_searchable(ctx)

        total_ms = (time.perf_counter() - total_start) * 1000.0
        return {
            "embed": embed_ms,
            "score": score_ms,
            "vault_topk": vault_ms,
            **searchable_phases,
            "total": total_ms,
        }

    async def run_searchable_scenario(self, scenario: dict) -> LatencyScenarioResult:
        """T10-T12: measure capture → searchable latency.

        v1.4.3: insert submit + server-push wait until `MERGED_SAVED`
        (request's vectors moved into canonical non-raw shards, pre-publish).
        v1.2.2: insert submit + client polling until top-1 cos similarity ≈ 1.0.
        """
        _parts = scenario["id"].split("_", 1)
        sid = f"T{int(_parts[0][1:]) + 9}_{_parts[1]}_searchable"
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
                t = await self._searchable_capture_phases(text, title, domain)
                all_timings.append(t)
            except Exception as e:
                print(f"\n    ERROR on {label}: {e}")
                return LatencyScenarioResult(
                    scenario_id=sid,
                    feature="searchable",
                    metadata=meta,
                    error=str(e),
                )

        print("done")
        # Phase names are SDK-specific: embed/score/vault_topk are fixed, the
        # insert->searchable phases come from the adapter (1 for v1.2.2,
        # 3 for v1.4.3).
        phase_names = (
            ["embed", "score", "vault_topk"]
            + self._adapter.searchable_phase_names()
            + ["total"]
        )
        phases = self._build_phase_list(phase_names, all_timings)
        return LatencyScenarioResult(
            scenario_id=sid,
            feature="searchable",
            phases=phases,
            metadata={**meta, "runs": self.runs - self.warmup},
        )

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

    # ── multi-phase capture ────────────────────────────────────────────────────

    async def _multi_capture_phases(
        self, texts: list[str], domain: str
    ) -> dict[str, float]:
        """
        Multi-phase capture: N records embedded and inserted as a batch.

        Mirrors the real capture path when a decision has multiple phases
        (server.py: record_builder.build_phases → insert_with_text(texts)).

        Phases:
          embed_batch  — embed(texts): single gRPC call, N vectors at once
          score        — novelty check on primary record (texts[0])
          vault_topk   — Vault decrypt on primary record's score
          insert_batch — insert all N vectors in one batch API call
          total        — wall clock including all phases
        """
        total_start = time.perf_counter()
        insights = [t[:120] for t in texts]

        # [1] Batch embed — uses embed(texts), not embed_single
        with _Timer() as t_embed:
            vecs = self._embedding.embed(insights)
        embed_ms = t_embed.elapsed_ms

        # [2] Novelty score on primary record (first phase)
        with _Timer() as t_score:
            score_res = self._adapter.score(self._index_name, vecs[0])
        score_ms = t_score.elapsed_ms

        # [3] Vault TopK decrypt
        vault_ms = 0.0
        blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
        if blobs:
            with _Timer() as t_vault:
                await self._vault_decrypt_with_retry(blobs[0], top_k=3)
            vault_ms = t_vault.elapsed_ms

        # [4] Insert all N vectors in one call — multi-phase capture is always
        # a batch insert (row_insert=False).
        metadata = [
            self._build_insert_metadata(t, f"phase-{i + 1}", domain)
            for i, t in enumerate(texts)
        ]
        with _Timer() as t_insert:
            self._adapter.insert(
                self._index_name,
                vecs,
                metadata,
                row_insert=False,
            )
        insert_ms = t_insert.elapsed_ms

        total_ms = (time.perf_counter() - total_start) * 1000.0

        # Same rationale as _single_capture_phases: probe the index with the
        # first vector outside the measurement window so the next iteration
        # starts on a stable state.
        if self.direct_envector:
            self._wait_for_score_ready(vecs[0])

        return {
            "embed_batch": embed_ms,
            "score": score_ms,
            "vault_topk": vault_ms,
            "insert_batch": insert_ms,
            "total": total_ms,
        }

    async def run_multi_capture_scenario(self, scenario: dict) -> LatencyScenarioResult:
        """T13-T14: multi-phase capture latency (batch embed + batch insert)."""
        sid = scenario["id"]
        texts = scenario["texts"]
        domain = scenario["domain"]
        meta = scenario.get("metadata", {})

        print(f"  [{sid}] ", end="", flush=True)
        all_timings: list[dict[str, float]] = []

        for i in range(self.runs):
            label = self._warmup_label(i)
            print(f"{label} ", end="", flush=True)
            try:
                t = await self._multi_capture_phases(texts, domain)
                all_timings.append(t)
            except Exception as e:
                print(f"\n    ERROR on {label}: {e}")
                return LatencyScenarioResult(
                    scenario_id=sid,
                    feature="multi_capture",
                    metadata=meta,
                    error=str(e),
                )

        print("done")
        phases = self._build_phase_list(
            ["embed_batch", "score", "vault_topk", "insert_batch", "total"],
            all_timings,
        )
        return LatencyScenarioResult(
            scenario_id=sid,
            feature="multi_capture",
            phases=phases,
            metadata={**meta, "runs": self.runs - self.warmup},
        )

    # ── network baseline ───────────────────────────────────────────────────────

    def _measure_network_rtt(self) -> str:
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

        rtt = self._measure_network_rtt()
        cfg = self._config
        report.env = {
            "sdk_version": self._adapter.sdk_version,
            "date": __import__("datetime").date.today().isoformat(),
            "envector_endpoint": cfg.envector.endpoint,
            "vault_endpoint": cfg.vault.endpoint,
            "embedding_model": cfg.embedding.model,
            "embedding_mode": cfg.embedding.mode,
            "index_name": self._index_name,
            "key_id": self._key_id,
            "eval_mode": self._adapter.eval_mode,
            "index_type": self._adapter.index_type,
            "insert_mode": self.insert_mode,
            "network_rtt": rtt,
            "runs_per_scenario": self.runs - self.warmup,
            "warmup_runs": self.warmup,
            "direct_envector": self.direct_envector,
            "reset_policy": (
                "per-scenario drop+create" if self.direct_envector
                else "no reset (shared production index)"
            ),
        }

        run_all = feature_filter is None
        run_capture = run_all or feature_filter == "capture"
        run_recall = run_all or feature_filter == "recall"
        run_vault = run_all or feature_filter == "vault_status"
        run_searchable = run_all or feature_filter == "searchable"
        run_multi = run_all or feature_filter == "multi_capture"

        # Only meaningful with --direct-envector. In default (vault) mode this
        # is a no-op so the orchestration body stays identical for both modes.
        def _reset_for(scenario_label: str) -> None:
            if not self.direct_envector:
                return
            print(f"  reset[{scenario_label}]...", end=" ", flush=True)
            self._reset_bench_index()
            self._ensure_index_loaded()
            print("done")

        if run_capture:
            print("\n[capture]")
            for sc in SCENARIOS_CAPTURE:
                _reset_for(sc["id"])
                r = await self.run_capture_scenario(sc)
                report.add(r)
            _reset_for("T4_duplicate")
            r = await self.run_capture_duplicate()
            report.add(r)

        if run_recall:
            print("\n[recall]")
            for sc in SCENARIOS_RECALL:
                _reset_for(sc["id"])
                self._prime_bench_index()
                r = await self.run_recall_scenario(sc)
                report.add(r)
            _reset_for("T7_topk_scaling")
            self._prime_bench_index()
            for r in await self.run_recall_topk_scaling():
                report.add(r)

        if run_vault:
            print("\n[vault_status]")
            r = await self.run_vault_status()
            report.add(r)

        if run_searchable:
            print("\n[searchable]")
            for sc in SCENARIOS_CAPTURE[:3]:  # T1, T2, T3 — short/long/Korean
                _reset_for(sc["id"] + "_searchable")
                r = await self.run_searchable_scenario(sc)
                report.add(r)

        if run_multi:
            print("\n[multi_capture]")
            for sc in SCENARIOS_MULTI_CAPTURE:
                _reset_for(sc["id"])
                r = await self.run_multi_capture_scenario(sc)
                report.add(r)

        return report


# ── CLI ────────────────────────────────────────────────────────────────────────

def _print_summary(report: LatencyBenchReport) -> None:
    env = report.env
    print("\n" + "=" * 64)
    print(
        f"  rune latency benchmark — pyenvector {env.get('sdk_version', '?')} "
        f"({env.get('eval_mode', '?')}/{env.get('index_type', '?')})"
    )
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
    bench = LatencyBenchmark(
        runs=args.runs,
        warmup=args.warmup,
        insert_mode=args.insert_mode,
        direct_envector=args.direct_envector,
        bench_index_name=args.bench_index,
    )

    mode_label = "bench-index" if args.direct_envector else "vault-mediated"
    print(
        f"\nSetting up … (mode={mode_label}, insert_mode={args.insert_mode}) "
        f"— SDK version auto-detected; eval_mode/index_type printed below"
    )
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
        description=(
            "Rune latency benchmark — unified runner. The SDK version "
            "(pyenvector 1.2.2 / 1.4.3) is auto-detected at runtime."
        )
    )
    parser.add_argument(
        "--insert-mode",
        choices=["single", "batch"],
        required=True,
        help=(
            "Insert mode. On pyenvector 1.4.3 this toggles the single-row vs "
            "batch insert API. On 1.2.2 there is no single-row path, so "
            "'single' is effectively batch."
        ),
    )
    parser.add_argument(
        "--feature",
        choices=["capture", "recall", "vault_status", "searchable", "multi_capture"],
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
    parser.add_argument(
        "--direct-envector",
        action="store_true",
        help=(
            "Benchmark index mode: provision a dedicated FLAT bench index "
            "(default `runecontext_bench`), drop+recreate it between scenarios "
            "for clean latency numbers, and prime it with 20 records before "
            "each recall scenario. Vault is still used for keys and FHE score "
            "decryption (the SecKey only lives on Vault). Does NOT touch the "
            "live runecontext data."
        ),
    )
    parser.add_argument(
        "--bench-index",
        default="runecontext_bench",
        help="Bench index name (--direct-envector only, default: runecontext_bench)",
    )
    args = parser.parse_args()

    if args.warmup >= args.runs:
        parser.error(f"--warmup ({args.warmup}) must be < --runs ({args.runs})")

    asyncio.run(_main(args))


if __name__ == "__main__":
    main()
