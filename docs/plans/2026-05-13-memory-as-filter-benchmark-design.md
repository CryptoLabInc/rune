# Memory-as-Filter Benchmark Architecture (v3) — Design

**Date:** 2026-05-13
**Status:** Design draft, awaiting implementation plan
**Author:** Sunchul Jung (Rune project lead)
**Replaces:** `benchmark/` v0.2-era scenario-based suite

---

## 1. Context

Rune v0.3 abandons pattern-matching capture detection in favor of **Memory-as-Filter**: the agent generates a dense `reusable_insight` (128–512 tokens), embeds it, runs a recall against existing memory, and classifies the result by `max_similarity` (< 0.3 → NOVEL, 0.3–0.7 → EVOLUTION, > 0.7 → REDUNDANT).

The legacy benchmark (`benchmark/`) verifies behavior against 104 hand-authored JSONL scenarios with deterministic `expected_capture: true/false` labels. This approach is incompatible with v0.3 because:

- Hand-maintained answer keys do not scale and are themselves opinions.
- Deterministic labels cannot evaluate stochastic, agent-mediated decisions.
- Pass/fail per scenario cannot detect distributional regressions (drift, calibration loss).
- The benchmark cannot grow with memory size; it lives outside the system it tests.

This document specifies a replacement benchmark whose paradigm is **behavioral invariants under controlled intervention**, not labeled scenarios. The benchmark requires no maintenance of answer keys, evolves with the system, and uses generated probes plus statistical analysis to verify properties any sane organizational memory must exhibit.

---

## 2. Design decisions log

| # | Decision | Selected | Why |
|---|---|---|---|
| 1 | Form of correctness | **Behavioral invariant** (over LLM-as-judge or generator-injected ground truth) | Sustainable; matches stochastic nature; brain-science paradigm |
| 2 | v1 invariant scope | Pre-flight (2) + Factorial core + Property-based + RAVLT signature + Merge Fidelity | After expert review found the original 5-invariant flat list to be circular, statistically thin, and missing the largest behavioral surface (EVOLUTION fidelity) |
| 3 | Evaluation boundary | **Skill black-box** + infrastructure mock | Catches prompt-LLM regressions, matches user intent ("rune skill이 얼마나 잘 capture") |
| 4 | Session model | **Per-invariant ephemeral fresh-brain** | Clean attribution; reproducibility; matches "매번 zero-base" requirement |
| 5 | Probe generation | **Controlled probes only** with dual-embedding distance validation | Clean signal first; distractor realism is added via padded retrieval store, not free-form generation |
| 6 | Embedding circularity | **Dual-family probe distance** (e5-base + gte-small) while skill/mock keep production parity (bge-small) | Breaks the "cosine ranking itself" tautology |
| 7 | Compute platform | **DGX SPARK + local models** | Cost margin ≈ 0; large N feasible; reproducibility (no API drift) |
| 8 | Model family | **Qwen-only** (32B agent / 72B judge) with explicit family-collapse caveat in reports | User-selected for operational simplicity; collapse risk acknowledged as v1 known limitation |
| 9 | Legacy benchmark | Moved to `benchmark/legacy/`, retained for v0.2 regression detection only | v0.2 capture path still exists as fallback |

---

## 3. Architecture overview

```
┌──────────────────────────────────────────────────────────────────────┐
│  Pre-flight diagnostics — gating, halt-on-fail                       │
│   #0  Extraction Stability        (sharp question from review)       │
│   #X  Structural Confusability    (probe v0.3 architectural blind)   │
├──────────────────────────────────────────────────────────────────────┤
│  Core: Factorial Causal Intervention Design                          │
│   3 × 3 × 2 × 3 = 54 cells × N=30 trials = 1620 measured trials      │
│   Response surface + main effects + 2-way interactions               │
├──────────────────────────────────────────────────────────────────────┤
│  Bolt-on: Property-Based Testing (LLM-free)                          │
│   6 algebraic properties × 5000 random sequences, with shrinking     │
├──────────────────────────────────────────────────────────────────────┤
│  Signature: Rune-RAVLT                                               │
│   Encode → Interfere → Recall → Recognize. Publishable paradigm.     │
├──────────────────────────────────────────────────────────────────────┤
│  Embedded: Merge Fidelity Probe                                      │
│   Subset of EVOLUTION-band factorial cells, LLM-judged content       │
├──────────────────────────────────────────────────────────────────────┤
│  Cross-cutting: baselines, dual-embedding, statistics, reproducibility│
└──────────────────────────────────────────────────────────────────────┘
```

Three different paradigms (factorial, algebraic, cognitive-battery) provide **three independent forms of evidence** about the same system. Their agreement (or disagreement) is itself a signal.

The Rune skill is treated as a black box: probes are injected at the agent-conversation level, the skill executes its real SKILL.md path with a real local LLM, and only the infrastructure beneath the MCP boundary (enVector cloud, Vault, FHE) is mocked.

---

## 4. Pre-flight diagnostics

Pre-flight runs first. Failure halts the rest of the suite; the report explains why downstream measurement is uninterpretable.

### 4.1 Extraction Stability (Invariant #0)

If the same input produces different `reusable_insight` outputs across reruns, every downstream measurement is built on noise. This is the single experiment that justifies running anything else.

- **Inputs**: 16 conversation excerpts spanning all Scribe trigger categories (architecture / debugging / incident / tradeoff / product / process / pr_review / casual...).
- **Procedure**: For each excerpt, call the skill instance 100 times under temperature=0.7 (production stochasticity), collect the produced insights.
- **Metric**: pairwise cosine similarity matrix per excerpt; report median, 5th percentile.
- **Pass gate**: median ≥ 0.85 AND 5th percentile ≥ 0.70 for ≥ 14 of 16 excerpts.
- **On fail**: full report annotated "extraction noise dominates"; downstream interpretation suspended.

### 4.2 Structural Confusability (Invariant #X)

The team has empirically shown bge-small-en-v1.5 puts opinion and decision in the EVOLUTION-to-REDUNDANT band when they share a topic (cosine = 0.81). The benchmark must probe whether v0.3 correctly distinguishes these as separate memories despite the embedding limitation.

- **Inputs**: 30 (topic, opinion-shaped insight, decision-shaped insight) triples generated by Llama-… no, Qwen 72B as generator under the family-collapse caveat (see §10).
- **Procedure**: capture opinion first, then decision; observe v0.3's classification of the decision.
- **Outcome categories**: (a) both NOVEL = correct architecture; (b) decision → EVOLUTION/REDUNDANT = architectural failure surfaced.
- **Report**: failure rate. Not a pass/fail gate — this is an architectural diagnostic. A failing result indicates v0.3 needs reusable_insight to encode structural tags (decision vs opinion) explicitly.

---

## 5. Core: Factorial causal intervention design

Replaces the original five flat invariants. Factorial design isolates main effects and detects interactions that single-axis tests are blind to.

### 5.1 Factors and levels

| Factor | Levels | Cognitive analog |
|---|---|---|
| `memory_load` | 10 / 100 / 1000 | Capacity, interference window |
| `target_similarity` | 0.2 / 0.5 / 0.8 (post-hoc bucketed) | Discrimination band |
| `distractor_density` | 1:1 / 10:1 | Production realism |
| `elapsed_episodes` | 0 / 50 / 500 | Recency, temporal interference |

3 × 3 × 2 × 3 = 54 cells. N = 30 trials per cell = 1620 measured trials.

### 5.2 Trial protocol

Each trial:

1. Fresh-brain reset.
2. Seed memory with `memory_load` distractor episodes drawn from a pre-curated distractor pool (see §8.2). Density factor controls relative count of topically-related vs unrelated items.
3. Inject the target episode.
4. Insert `elapsed_episodes` further filler episodes (distractor pool, unrelated topic).
5. Issue a query at controlled `target_similarity` band (post-hoc verified using e5-base distance).
6. Record: classification class, retrieval rank of target, max_similarity score, Hit@1, Hit@5, raw score distribution over top-20.

### 5.3 Analysis

- **Main effects**: per factor, marginal mean of each dependent variable.
- **Interaction effects**: 2-way interaction tables; report partial eta-squared.
- **Discrimination as signal detection**: across all cells, compute ROC and d-prime for binary "is the target retrievable above similarity threshold?" classifier sweeping threshold. Lifts directly from Macmillan & Creelman.
- **Recovery of original 5 invariants**: original hypotheses are recovered as specific cell projections:
  - Habituation ≅ `elapsed=0` row with monotone target similarity injection
  - Conservation ≅ storage size at end of memory_load=10/100/1000 with K paraphrases held constant
  - Selectivity ≅ Spearman/Kendall over target_similarity at fixed load
  - Retroactive interference (formerly Amnesia) ≅ `elapsed=500` vs `elapsed=0` delta at matched similarity
- **Baselines on every plot**: Random / Always-NOVEL / Always-REDUNDANT bands.

### 5.4 Reframing of original invariants

Following expert critique:

| Original name | Corrected name | Reason |
|---|---|---|
| Habituation | Recognition under repeated exposure | Habituation is response attenuation without storage; the targeted phenomenon is repetition priming / familiarity (Yonelinas, 2002) |
| Amnesia Diagnosis | Retroactive interference under load | Ribot's law / amnesia describes lesion-driven graded loss; the measured phenomenon is RI (Underwood, 1957) |

---

## 6. Bolt-on: Property-based testing

LLM-free. Mock store only. Catches algebraic bugs the LLM-driven probes never produce.

### 6.1 Properties

1. **Idempotence**: `capture(x); capture(x)` produces same store size as `capture(x)` alone.
2. **Monotonicity**: store size is non-decreasing over capture sequences.
3. **Capture conservation**: N exact-paraphrase captures of the same insight produce store growth ≤ 1.
4. **Recall determinism (modulo declared stochasticity)**: same query at temperature=0 yields identical top-k twice.
5. **Recall stability under irrelevant insertion**: top-k recall for query Q is unchanged when an episode unrelated to Q is inserted between query-runs.
6. **Commutativity modulo merge**: `capture(a); capture(b)` and `capture(b); capture(a)` yield set-equivalent final stores up to merge ordering.

### 6.2 Procedure

- Hypothesis-style randomized sequence generation; 5000 sequences per property; max sequence length 50.
- Shrinking on counterexample (smallest failing sequence reported).
- Cost: zero LLM calls. Skill agent stub returns insights deterministically derived from input text for properties 1–4. Properties 5–6 use the real skill but no LLM is invoked beyond the existing fresh-brain runs.

---

## 7. Signature: Rune-RAVLT

A single publishable paradigm modeled on the Rey Auditory Verbal Learning Test (Rey, 1964). Provides a legible, well-validated structure for the cognitive-science audience and an anchor for cross-system comparison.

### 7.1 Protocol

1. **List A encoding**: 15 distinct decision insights generated by Qwen 72B at controlled cross-topic spread (post-hoc verified pairwise cosine < 0.4 between any List A pair). Inject sequentially into fresh brain.
2. **Immediate cued recall**: For each List A item, issue a paraphrase query (target_similarity ≈ 0.7). Record `immediate_recalled / 15`.
3. **Interference phase (List B)**: Inject 15 distinct decision insights from disjoint topics.
4. **Delayed recall A**: Re-issue List A queries. Record `delayed_recalled / 15`. **Retroactive interference effect = immediate_recalled − delayed_recalled**.
5. **Recognition test**: Present List A's 15 items + 15 *novel-but-similar* foils (semantically near, factually different). Each is issued as a recognition query; the system's classification (NOVEL vs EVOLUTION/REDUNDANT) determines hit/false-alarm.
   - Hit rate = P(REDUNDANT | item in List A)
   - False alarm rate = P(REDUNDANT | foil)
   - d-prime computed in standard signal-detection form.

### 7.2 Statistics

20 independent seeds. Per-metric: mean, bootstrap 95% CI. RI effect size in Cohen's d. d-prime mean and 95% CI.

### 7.3 Interpretation hooks

The paper-facing narrative is: "Rune-RAVLT measures three classical memory phenomena in a single paradigm. The system's RI curve and recognition d-prime can be compared qualitatively with human norms (Schmidt, 1996), bearing in mind Rune is not a human memory system and the comparison is structural, not quantitative."

---

## 8. Cross-cutting concerns

### 8.1 Dual embedding (anti-circularity)

- **Skill side**: bge-small-en-v1.5 (production parity).
- **Mock enVector store**: bge-small-en-v1.5 (production parity).
- **Probe synthesis & distance validation**: e5-base-v2 AND gte-small. Both must agree that a probe is within the target similarity bucket; disagreement above tolerance rejects the probe.
- **Cross-family agreement reported per run** as a sanity statistic. Low agreement is itself diagnostic about probe-bucket reliability.

### 8.2 Distractor pool

Pre-curated library of ~3000 decision insights spanning topics orthogonal to common test topics (e.g., HR policies, FHE key rotation, frontend a11y, observability dashboards…). Generated once with Qwen 72B + human spot-check; serialized to disk; loaded at trial start. Not regenerated per run (reproducibility).

### 8.3 Baselines

Three baselines computed alongside every dependent variable:

- **Random**: shuffle the labels/indices/order, recompute.
- **Always-NOVEL**: filter is disabled; every capture stores; recall computed over the stored superset.
- **Always-REDUNDANT**: filter blocks every capture; store is empty; recall always fails.

Plots overlay all three baselines. The real system result must lie outside the random band and between the two trivial baselines on every dependent variable; otherwise the metric is not discriminative.

### 8.4 Statistics

- Kendall's τ-b (small-n robust) over Spearman ρ where rank correlation is needed.
- Bootstrap 95% CIs on all summary statistics (10000 resamples).
- Effect sizes: Cohen's d (location), partial η² (factor strength), Cliff's δ (distribution-free).
- d-prime / ROC AUC for recognition tasks.

### 8.5 Model selection and the family-collapse caveat

v1 ships with Qwen-family-only: Qwen 2.5 32B-Instruct as the skill agent's local LLM, Qwen 2.5 72B-Instruct as the probe generator and judge.

This is acknowledged as a v1 known limitation:

> All LLM components in this benchmark belong to the Qwen family. Per Zheng et al. (2023) and the LLM-as-judge literature, shared-family generator/judge configurations may produce inflated agreement (estimated 20–30%). Results should be read with this in mind. v2 plans cross-family runs (Llama, Mistral) to bound the collapse effect.

### 8.6 Reproducibility

- Model checkpoints pinned by hash (config records exact `Qwen2.5-32B-Instruct@<hash>`).
- Random seeds fixed per trial.
- Two measurement modes: deterministic (temperature=0) for invariant gating; stochastic (temperature=0.7) for extraction-stability and production-fidelity studies. Mode recorded per metric.
- Distractor pool serialized and content-hashed.
- DGX SPARK environment captured (CUDA driver, vLLM version) in run metadata.

### 8.7 LLM-as-judge usage

Constrained to two cases:

1. **Merge fidelity scoring** (§9): factual checklist comparison against original and refinement insights.
2. **Anomaly diagnosis**: when a factorial cell or RAVLT metric falls outside its expected range and outside baselines, the judge LLM produces a qualitative explanation appended to the report.

Factorial main metrics and property-based properties do NOT use the judge — they are computed numerically.

---

## 9. Merge Fidelity (embedded)

EVOLUTION is the largest behavioral surface and the original design had no fidelity test. Implemented inside the factorial sweep as a follow-up scoring pass over EVOLUTION-classified cells:

- For each EVOLUTION case in the `target_similarity = 0.5` slabs:
  - Extract `facts(X)` and `facts(X')` via Qwen 72B (structured fact list).
  - Extract `facts(stored_after_merge)` similarly.
  - Compute precision = |facts(stored) ∩ facts(X ∪ X')| / |facts(stored)|, recall = |facts(stored) ∩ facts(X ∪ X')| / |facts(X ∪ X')|.
- Aggregate to mean precision/recall over all EVOLUTION cases. Pass: precision ≥ 0.8 AND recall ≥ 0.8.

A merge that silently drops half the content of one insight will fail this regardless of how good the embedding similarity looks.

---

## 10. Compute budget on DGX SPARK

Marginal cost ≈ 0; the constraint is GPU time on a single node.

Rough wall-clock estimates (Qwen 2.5 32B served via vLLM, batch=8, FP8):

| Suite | Trial count | Est. wall-clock |
|---|---|---|
| Extraction Stability | 1600 skill calls | ~1.5 h |
| Structural Confusability | 60 skill calls | <10 min |
| Factorial | 1620 trials × ~3 skill calls | ~5 h |
| Property-based | 5000 sequences, mock-only | <10 min |
| Rune-RAVLT | 20 seeds × ~90 episodes × ~2 calls | ~3 h |
| Merge Fidelity (judge) | ~300 cases × 1 Qwen 72B call | ~1 h |
| **Total full run** | | **~11 h** |

Schedule: nightly full run feasible. Pre-flight subset (~2 h) on PR. v1 starts as on-demand; nightly enabled after 2 weeks of stability.

---

## 11. Directory layout

```
benchmark/v3/
  preflight/
    extraction_stability.py
    confusability.py
  factorial/
    design.py
    runner.py
    surface.py
  property/
    properties.py
    shrinker.py
  ravlt/
    paradigm.py
  merge_fidelity/
    judge_runner.py
  generators/
    probe_synthesis.py
    distractor_pool.py
    distractor_pool.jsonl.gz   # pre-curated, content-hashed
  mocks/
    envector_mock.py
    vault_stub.py
  agents/
    skill_driver.py            # in-proc skill instance, mock MCP boundary
    serving.py                 # vLLM client wrapper
  baselines/
    random_baseline.py
    always_novel.py
    always_redundant.py
  analysis/
    statistics.py              # τ-b, bootstrap, d-prime, η²
    plotting.py                # response surface, ROC, learning curves
  report/
    summary.md.jinja
    factorial_section.py
    property_section.py
    ravlt_section.py
  cli.py
  README.md

benchmark/legacy/             # moved from benchmark/
  scenarios/ runners/ datasets/ ...  # untouched; v0.2 regression only
```

CLI:

```bash
python -m benchmark.v3 preflight    # halt-on-fail gate
python -m benchmark.v3 factorial
python -m benchmark.v3 property
python -m benchmark.v3 ravlt
python -m benchmark.v3 all          # full pipeline + unified report
```

---

## 12. Legacy migration

- `benchmark/` → `benchmark/legacy/` (verbatim move).
- `benchmark/legacy/README.md` updated with header: "v0.2 regression detection only. v0.3 Memory-as-Filter evaluation lives in `benchmark/v3/`."
- Existing CI hooks switched to call `benchmark/v3/cli.py preflight` for PR checks. Legacy left runnable but not gating.

---

## 13. v2 backlog (not in v1)

Tracked here so they aren't lost:

- **Cross-family runs** (Llama / Mistral generator+judge) to bound family-collapse inflation.
- **Encoding specificity** invariant (Tulving & Thomson, 1973).
- **Schema-based gist extraction** (Reyna & Brainerd's fuzzy-trace theory).
- **Proactive interference** specifically as a separate invariant.
- **Testing effect / retrieval-induced consolidation** (Roediger & Karpicke, 2006).
- **Context-dependent recall** (Godden & Baddeley, 1975) once Rune has metadata-conditioned recall.
- **Long-horizon integration scenario** combining 3+ factors over 1000+ episodes to catch interaction-only failures.
- **Order/serial-position curves** (Murdock, 1962).

---

## 14. Cited literature

- Yonelinas, A. P. (2002). The nature of recollection and familiarity: A review of 30 years of research.
- Underwood, B. J. (1957). Interference and forgetting.
- Tulving, E., & Thomson, D. M. (1973). Encoding specificity and retrieval processes in episodic memory.
- Reyna, V. F., & Brainerd, C. J. (1995). Fuzzy-trace theory.
- Roediger, H. L., & Karpicke, J. D. (2006). Test-enhanced learning.
- Macmillan, N. A., & Creelman, C. D. (2005). Detection theory: A user's guide.
- Schmidt, M. (1996). Rey Auditory Verbal Learning Test: A handbook.
- Murdock, B. B. (1962). The serial position effect of free recall.
- Zheng, L., et al. (2023). Judging LLM-as-a-Judge with MT-Bench and Chatbot Arena.

---

## 15. Open questions for implementation plan

1. Distractor pool curation: human-in-the-loop spot check size? (suggested 5% sample of ~3000.)
2. vLLM serving topology on DGX SPARK: single 32B + 72B coresident vs swap-on-demand? Memory pressure on 128 GB unified RAM unclear.
3. Skill instance isolation: spawn a new Python process per trial vs in-proc reset? Performance vs cleanliness trade-off.
4. Pre-flight failure escalation: do we hard-stop the suite, or run downstream anyway with a noise-dominated annotation?
5. Merge-fidelity fact extraction prompt: who designs the structured fact schema, and how do we evaluate fact-extractor quality itself?
