# Latency Sweep Benchmark — v1.2.2 (rmp/flat) vs v1.4.3 (mm32/ivf_vct)

> **Supersedes** `latency_bench_plan_envector_v1.2.2.md`,
> `latency_bench_plan_envector_v1.2.2_extended.md`, and
> `latency_bench_plan_envector_v1.4.3.md`. Those describe the older
> per-version runners (`latency_bench_v1.4.3.py`, since removed). The current
> work uses one adapter-driven runner — `benchmark/runners/latency_bench.py`
> — with an index-size sweep mode.

## Goal

Does v1.4.3's `mm32 + ivf_vct` have a real latency advantage over v1.2.2's
`rmp + flat`, in the regime where ivf_vct's centroid optimization is active?

ivf_vct gathers ~4096 rows into a virtual centroid and merges them. Below
N≈4096 there is a single centroid — effectively a flat scan, no IVF benefit.
The comparison is only meaningful at **N > 4096** (multi-centroid).

## Unified runner

`benchmark/runners/latency_bench.py` is SDK-agnostic. `get_sdk_adapter()`
detects the installed `pyenvector.__version__` → V122Adapter (1.2.x) or
V143Adapter (1.4.x). Sweep mode (`--primer-rows N1,N2,...`) measures the
selected scenarios across a grid of index sizes N; see the runner's
`run_sweep` docstring. Raw samples stream to a long-format CSV
(`N,scenario,run_idx,phase,latency_ms`).

## Methodology — asymmetric grid

| | v1.2.2 / flat | v1.4.3 / ivf_vct |
|---|---|---|
| Scaling | O(N), no regime change | a centroid added every ~4096 rows |
| High-N measurement | infeasible — score ≈ 11 ms/record, so N=16384 ≈ 3 min/call | feasible — ivf is sublinear |
| Grid strategy | measure [0..8192], **extrapolate the line above** | measure across the centroid boundaries |

flat is a brute scan — provably linear, and the data confirms a near-perfect
line. Extrapolating it above the measured range is rigorous, not a guess.
ivf has a regime change at every centroid boundary, so it cannot be
extrapolated — it must be measured.

The grids overlap on [0, 8192] for direct comparison; above 8192, v1.2.2 is
the extrapolated flat line and v1.4.3 is measured. ivf cost ≈ `nprobe × 4096`
vectors (≈ constant for fixed nprobe), so the crossover N — where ivf meets
the flat line — sits near `nprobe × 4096`.

> The two SDKs run on different machines (the SDK versions cannot coexist).
> The dominant N-dependent phase (`score`) is cluster-side, so client-machine
> differences mostly affect the smaller phases — note this in the report.

## Phase 3 — v1.2.2 sweep   `[machine: pyenvector 1.2.2]`

**Status: running** (launched 2026-05-16, ~16 h).

```
python benchmark/runners/latency_bench.py \
    --direct-envector \
    --primer-rows 0,256,1024,2048,4096,8192 \
    --runs 11 --warmup 3 \
    --report benchmark/reports/latency_sweep_v122_rmpflat_2026-05-16.md \
    --raw-csv benchmark/reports/raw/latency_sweep_v122_rmpflat_2026-05-16.csv
```

8 effective runs/N. flat is a line, so few runs/N suffice — the 6-point line
fit averages the noise.

## Phase 4 — v1.4.3 sweep + comparison   `[machine: pyenvector 1.4.3]`

Prerequisites: pyenvector 1.4.3 installed; the v1.4.3 cluster endpoint set in
`~/.rune/config.json`; the `rune/.venv` (Python 3.12).

### Step 1 — implement V143Adapter

`benchmark/runners/sdk/v143.py` is a **skeleton** — its `connect`, `insert`,
and `measure_insert_to_searchable` methods raise `NotImplementedError`. The
method docstrings carry detailed implementation guidance (constructor
kwargs, the forced `await_completion=False`+`load=False` insert path, the
3-phase searchable decomposition). Reference implementation:

```
git show benchmark/envector-latency-v1.4.3:benchmark/runners/latency_bench_v1.4.3.py
```

Verify the adapter loads:

```
python -c "import sys; sys.path.insert(0,'benchmark/runners'); from runners.sdk import get_sdk_adapter; print(get_sdk_adapter().sdk_version)"
```

### Step 2 — smoke test

```
python benchmark/runners/latency_bench.py --direct-envector \
    --primer-rows 0,4096,8192 --runs 5 --warmup 1 --raw-csv /tmp/smoke_v143.csv
```

Confirms the sweep loop completes with no errors before the long run.

### Step 3 — full sweep

```
python benchmark/runners/latency_bench.py \
    --direct-envector \
    --primer-rows 0,256,1024,2048,4096,8192,12288,16384 \
    --runs 15 --warmup 3 \
    --report benchmark/reports/latency_sweep_v143_mm32ivf_<date>.md \
    --raw-csv benchmark/reports/raw/latency_sweep_v143_mm32ivf_<date>.csv
```

The grid overlaps v1.2.2 on [0..8192] and extends to 16384 (4 centroids);
add 32768 for more centroids if wanted. To probe the exact transition, dense
points around a boundary help (e.g. `4095,4096,4097`). 12 effective runs/N —
ivf is fast, so more runs are cheap here.

> **Caveat** — verify the ivf centroid merge has completed after priming each
> N, before measuring. `_prime_bench_index` waits only for `score` readiness,
> which may not imply merge-complete; measuring an un-merged ivf index would
> understate its optimization. Record the pyenvector build and SDK bug state
> in the report env (see the v143.py `measure_insert_to_searchable`
> docstring).

### Step 4 — comparison report

Merge the two raw CSVs. Per scenario, overlay v1.2.2-flat (measured 0–8192 +
linear extrapolation above) and v1.4.3-ivf (measured) latency-vs-N curves.
Report the crossover N, the winner in each region, and the speed-up factor.
State which config each SDK measured and the two-machine caveat in the
environment section. (`searchable` is measured by different mechanisms per
SDK — see v143.py — so compare only total insert→searchable time there.)

## Parallel execution

Phase 3 (this machine, v1.2.2) and Phase 4 (other machine, v1.4.3) are
independent. After `git pull`, Phase 4 Step 1 (implement V143Adapter) can
start immediately — it does not wait on Phase 3.
