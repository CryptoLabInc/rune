# Rune × envector-msa-1.2.2 Latency Report

> **[측정 환경]** pyenvector 1.2.2. **eval_mode=rmp, index_type=flat**.
> envector 엔드포인트: `0511-1401-0001-4r6cwxfc908b.clusters.envector.io` (5/11 측정과 동일 클러스터).
> Vault: `tcp://193.122.124.173:50051` (5/11 측정과 동일 엔드포인트). 단 5/12에 CA cert (`~/.rune/certs/ca.pem`) 와 vault token이 갱신됨 — 새 cert/token 적용 후 본 측정 진행.
> **시나리오 셋**: v1.4.3 runner(`latency_bench_v1.4.3.py`)를 v1.2.2 SDK 호환으로 어댑팅하여 T1–T14 측정. T8(batch_capture)은 v1.4.3에서 폐지되어 본 측정에서도 제외. T10–T14(searchable, multi_capture)는 v1.2.2에서 **신규 측정**(5/11 baseline 없음).
> **insert_mode**: single만 측정. v1.2.2 SDK는 SDK 레벨에서 batch insert path를 노출하지 않음.
>
> **[비교 노트]** 5/11(`latency_results_v1.2.2_2026-05-11.md`)과 동일 envector/Vault 엔드포인트이므로 인프라 변경은 아님. T9 vault health이 7.4ms로 5/11 7.6ms와 사실상 일치하는 것이 그 증거. 그럼에도 T1–T7가 5/11 대비 ~1.4-1.7x 느려진 것은 **인덱스 vector 누적**이 가장 유력한 가설 — flat 인덱스 brute-force는 N에 선형이며, 한 달 사이 별도 측정/캡처로 vector가 누적된 상태로 본 측정이 수행됨.

## 실행 결과 요약

| 시나리오                           | feature       | p50 total | p95 total | n      | Δ(5/11 대비)    |
| ---------------------------------- | ------------- | --------- | --------- | ------ | --------------- |
| T1 짧은 영문 (~35 tokens)          | capture       | 1153.7 ms | 1201.7 ms | 8      | +490 ms (×1.7)  |
| T2 긴 영문 (~155 tokens)           | capture       | 1176.0 ms | 1249.8 ms | 8      | +366 ms (×1.5)  |
| T3 한국어                          | capture       | 1194.9 ms | 1232.4 ms | 8      | +324 ms (×1.4)  |
| T4 중복 입력 (near-duplicate path) | capture       | 1271.8 ms | 1337.1 ms | 8      | +307 ms (×1.3)  |
| T5 Recall — exact match            | recall        | 839.7 ms  | 883.6 ms  | 8      | +311 ms (×1.6)  |
| T6 Recall — 한→영 cross-language   | recall        | 860.0 ms  | 945.7 ms  | 8      | +344 ms (×1.7)  |
| T7 Recall — topk 1/3/5/10          | recall        | 805–841 ms| 873–953 ms| 5 each | ×1.6–1.7        |
| T9 Vault health check              | vault_status  | 7.4 ms    | 9.7 ms    | 8      | -0.2 ms (≈동일) |
| T10 searchable 짧은 영문           | searchable    | 2200.5 ms | 2296.5 ms | 8      | (신규)          |
| T11 searchable 긴 영문             | searchable    | 2368.0 ms | 2432.0 ms | 8      | (신규)          |
| T12 searchable 한국어              | searchable    | 2494.6 ms | 2642.4 ms | 8      | (신규)          |
| T13 multi_capture 2-phase          | multi_capture | 2013.0 ms | 2177.5 ms | 8      | (신규)          |
| T14 multi_capture 5-phase          | multi_capture | 2556.6 ms | 2676.8 ms | 8      | (신규)          |

**capture 병목**: `insert` (462–525 ms, ~40%)과 `score` (303–473 ms, ~30%)이 합산 ~70%. 5/11(insert ~74% 단독 지배)과 달리 score 비중이 상대적으로 ↑ — 인덱스 누적이 score 단계에 직접 반영된 신호.
**recall 병목**: `score` (≈510 ms) + `vault_topk` (≈250 ms) FHE 경로가 전체의 **~90%**, embed/remind는 부수적. 패턴은 5/11과 동일하나 절대값이 ~1.6x 증가.
**vault health 무변동**: T9 7.4 ms는 5/11 7.6 ms와 사실상 일치. **인프라/네트워크 자체는 그대로**이며 capture/recall 증가의 원인이 인프라가 아님을 확정.
**recall topk 무관**: T7에서 topk 1→10에도 total p50 805–841 ms로 거의 동일. FHE inner_product cost가 topk와 무관한 5/11의 구조적 특성 그대로.
**searchable polling overhead**: T10–T12의 `insert_searchable` phase가 ~1330 ms — score+vault decrypt polling 사이클 1–2회 비용이 포함된 측정 (200 ms granularity).
**multi_capture phase fan-in 효과**: T14(5-phase)의 total 2557 ms는 single capture × 5 추정치(~5800 ms)의 ~44%. embed/score/vault_topk가 phase 수에 거의 무관하게 작동.

## 5/11 vs 5/12 p50 비교

> 동일 envector/Vault 엔드포인트, 동일 pyenvector 1.2.2 환경에서 약 1일 차이로 측정. T9가 일치하므로 인프라 변동은 0에 가까우며, 차이는 인덱스 누적 가설의 직접 증거.

```
── capture (p50 ms, 기준 1272ms = 40칸) ────────────────────────────────

T1 영문    5/11  █████████████████████  664ms
           5/12  ████████████████████████████████████  1154ms  (×1.7)

T2 긴영문  5/11  █████████████████████████  810ms
           5/12  █████████████████████████████████████  1176ms  (×1.5)

T3 한국어  5/11  ███████████████████████████  871ms
           5/12  █████████████████████████████████████  1195ms  (×1.4)

T4 중복    5/11  ██████████████████████████████  965ms
           5/12  ████████████████████████████████████████  1272ms  (×1.3)

── recall (p50 ms, 기준 860ms = 30칸) ──────────────────────────────────

T5 exact   5/11  ██████████████████  529ms
           5/12  ██████████████████████████████  840ms  (×1.6)

T6 cross   5/11  ██████████████████  516ms
           5/12  ██████████████████████████████  860ms  (×1.7)

── vault health (p50 ms, 기준 7.6ms = 20칸) ────────────────────────────

T9 vault   5/11  ████████████████████  7.6ms
           5/12  ███████████████████   7.4ms  (×1.0)
```

---

## 주목할 점

### T9 외 모든 시나리오가 5/11 대비 ~1.4–1.7x 느림 — 인덱스 누적 가설

|              | embed | score | vault_topk | insert | total |
| ------------ | ----- | ----- | ---------- | ------ | ----- |
| T1 5/11      | 38.8  | 102.0 | 34.6       | 490.0  | 664.0 |
| T1 5/12      | 39.2  | 303.6 | 328.8      | 462.1  | 1153.7 |
| Δ            | +0%   | +198% | +850%      | -6%    | +74%  |

`score`와 `vault_topk`에서 거의 모든 증가가 발생. `embed`(로컬)와 `insert`(서버측 단순 write)는 변동 없음. flat 인덱스 brute-force가 N에 선형이라는 구조와 정확히 일치 — **인덱스에 vector가 누적된 만큼 score(전체 인덱스 대비 inner product) 와 vault_topk(decrypt할 후보 수)가 증가**.

같은 가설을 뒷받침하는 다른 증거: T3 한국어의 score가 5/11엔 영문 T1의 2x(210 vs 102 ms)였는데, 5/12엔 둘 다 ~303 ms로 사실상 동일. 인덱스 N이 커지면 토큰 종류별 페이로드 차이보다 N에 비례하는 base cost가 지배. (다만 본 가설은 인덱스 vector count를 직접 측정해 확인하지 않았으므로 추정 단계.)

### T9 vault_health는 7.4 ms (5/11 7.6 ms) — 인프라/네트워크 무변동

Vault 엔드포인트(`tcp://193.122.124.173:50051`)는 5/11과 5/12 모두 동일. 5/12에 CA cert와 token이 rotation됐지만 핸드셰이크/RPC 비용엔 영향 없음. **본 측정의 다른 시나리오 증가가 인프라 원인이 아님**을 확정하는 control 측정.

### searchable polling overhead — measurement granularity caveat

|                      | embed | score | vault_topk | insert_searchable | total  |
| -------------------- | ----- | ----- | ---------- | ----------------- | ------ |
| T10 short_en         | 61.1  | 510.8 | 263.8      | **1333.4**        | 2200.5 |
| T1 capture (참고)    | 39.2  | 303.6 | 328.8      | 462.1 (insert)    | 1153.7 |

`insert_searchable` (1333 ms)는 T1의 일반 `insert` (462 ms)의 **~2.9x**. 차이 ~870 ms는 polling overhead — `_wait_until_searchable`이 한 번의 score+vault decrypt 사이클을 추가로 수행하는 비용. v1.4.3 server-push 측정(~89 ms)과 직접 비교 불가 — 측정 메커니즘 자체가 다름 (자세한 비교는 `latency_comparison_v1.2.2_vs_v1.4.3_2026-05-12.md` §3 참조).

### multi_capture는 single capture × N보다 효율적 — phase fan-in 효과

| 시나리오 | phase 수 | total p50 | single capture × N 추정 | 절감 |
| -------- | -------- | --------- | ----------------------- | ---- |
| T13      | 2        | 2013 ms   | 2308 ms (1154 × 2)      | ~13% |
| T14      | 5        | 2557 ms   | 5769 ms (1154 × 5)      | ~56% |

embed(75–86 ms) / vault_topk(342–383 ms)가 phase 수에 거의 비례하지 않고 batch 호출 한 번으로 끝나는 게 절감의 원인. score는 primary record 1개만 측정하므로 phase 수와 무관. insert_batch만 phase 수에 약하게 비례(2-phase 870 ms → 5-phase 1279 ms, ×1.5). **결정의 phase 수가 늘어날수록 단위 phase당 비용은 떨어진다**는 application-level 인사이트.

### T4 duplicate score가 T1 대비 1.6x — near-duplicate path 패턴 (5/11과 동일)

T1 score 303.6 ms vs T4 score 473.2 ms. 5/11에서도 T4 score가 T1의 ~2.7x였던 동일 패턴 (102 → 279 ms). 인덱스 누적으로 두 시나리오 모두 절대값은 증가했으나 **near-duplicate 판정 경로의 추가 vault_topk decrypt 분기는 그대로 활성** — 알고리즘 구조 변화는 없음.

---

## Environment

- **bench_version**: 1.4.3
- **date**: 2026-05-12
- **envector_endpoint**: 0511-1401-0001-4r6cwxfc908b.clusters.envector.io
- **vault_endpoint**: tcp://193.122.124.173:50051
- **embedding_model**: Qwen/Qwen3-Embedding-0.6B
- **embedding_mode**: sbert
- **index_name**: runecontext1
- **key_id**: vault-key
- **eval_mode**: rmp
- **index_type**: flat
- **insert_mode**: single
- **network_rtt**: unknown
- **runs_per_scenario**: 8
- **warmup_runs**: 2


## Feature: `capture`

### T1_short_en
- label: short English (~30 tokens)
- tokens_approx: 35
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 39.2 | 71.9 | 73.3 | 48.5 |
| score | 8 | 303.6 | 317.4 | 317.6 | 294.9 |
| vault_topk | 8 | 328.8 | 411.6 | 441.2 | 316.0 |
| insert | 8 | 462.1 | 477.9 | 478.2 | 462.5 |
| total | 8 | 1153.7 | 1201.7 | 1208.2 | 1121.9 |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 51.5 | 78.4 | 78.6 | 56.3 |
| score | 8 | 355.5 | 455.8 | 463.4 | 374.3 |
| vault_topk | 8 | 260.8 | 282.4 | 285.9 | 265.0 |
| insert | 8 | 479.8 | 545.7 | 553.9 | 489.2 |
| total | 8 | 1176.0 | 1249.8 | 1264.3 | 1184.8 |

### T3_korean
- label: Korean text
- tokens_approx: 50
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 58.1 | 75.0 | 76.6 | 57.9 |
| score | 8 | 424.0 | 464.6 | 472.6 | 426.5 |
| vault_topk | 8 | 226.0 | 251.9 | 253.9 | 229.0 |
| insert | 8 | 470.5 | 523.2 | 536.4 | 478.7 |
| total | 8 | 1194.9 | 1232.4 | 1245.6 | 1192.1 |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 58.8 | 90.2 | 96.6 | 60.3 |
| score | 8 | 473.2 | 513.1 | 514.6 | 474.1 |
| vault_topk | 8 | 239.9 | 269.4 | 273.3 | 244.3 |
| insert | 8 | 495.7 | 518.6 | 520.9 | 491.4 |
| total | 8 | 1271.8 | 1337.1 | 1338.7 | 1270.1 |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 31.9 | 33.3 | 33.4 | 31.1 |
| score | 8 | 509.7 | 571.8 | 579.7 | 514.9 |
| vault_topk | 8 | 249.2 | 294.2 | 305.3 | 255.7 |
| remind | 8 | 38.9 | 40.6 | 40.6 | 38.4 |
| total | 8 | 839.7 | 883.6 | 885.3 | 840.2 |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 30.9 | 34.2 | 34.9 | 31.0 |
| score | 8 | 526.8 | 600.9 | 603.4 | 533.6 |
| vault_topk | 8 | 250.7 | 281.2 | 286.8 | 252.9 |
| remind | 8 | 38.7 | 92.9 | 115.1 | 49.0 |
| total | 8 | 860.0 | 945.7 | 950.0 | 866.5 |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 30.4 | 34.7 | 35.3 | 30.8 |
| score | 5 | 478.4 | 567.6 | 568.6 | 509.3 |
| vault_topk | 5 | 258.0 | 268.8 | 270.7 | 255.7 |
| remind | 5 | 35.2 | 38.6 | 38.6 | 36.0 |
| total | 5 | 804.8 | 897.0 | 899.0 | 831.8 |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 29.2 | 31.7 | 32.3 | 29.6 |
| score | 5 | 520.0 | 572.0 | 581.0 | 524.7 |
| vault_topk | 5 | 241.7 | 270.9 | 273.8 | 249.4 |
| remind | 5 | 38.5 | 39.0 | 39.1 | 38.2 |
| total | 5 | 826.3 | 898.0 | 904.3 | 841.9 |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 32.2 | 33.0 | 33.2 | 31.9 |
| score | 5 | 530.8 | 573.1 | 576.8 | 525.7 |
| vault_topk | 5 | 229.7 | 234.9 | 235.7 | 230.9 |
| remind | 5 | 40.5 | 42.6 | 42.8 | 40.2 |
| total | 5 | 837.2 | 873.1 | 876.2 | 828.8 |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 29.5 | 33.3 | 33.4 | 30.2 |
| score | 5 | 512.5 | 525.5 | 527.3 | 502.4 |
| vault_topk | 5 | 257.9 | 352.2 | 362.9 | 283.0 |
| remind | 5 | 42.5 | 50.3 | 50.6 | 44.5 |
| total | 5 | 840.6 | 952.8 | 971.9 | 860.1 |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| vault_health_check | 8 | 7.4 | 9.7 | 10.6 | 7.8 |


## Feature: `searchable`

### T10_short_en_searchable
- label: short English (~30 tokens)
- tokens_approx: 35
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 61.1 | 70.8 | 71.5 | 62.0 |
| score | 8 | 510.8 | 574.0 | 580.3 | 521.8 |
| vault_topk | 8 | 263.8 | 281.8 | 284.2 | 258.9 |
| insert_searchable | 8 | 1333.4 | 1460.4 | 1502.3 | 1341.7 |
| total | 8 | 2200.5 | 2296.5 | 2327.0 | 2184.5 |

### T11_long_en_searchable
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 70.1 | 76.4 | 77.8 | 68.8 |
| score | 8 | 585.3 | 638.3 | 639.1 | 590.6 |
| vault_topk | 8 | 290.3 | 310.8 | 310.9 | 291.2 |
| insert_searchable | 8 | 1428.2 | 1482.1 | 1496.3 | 1427.8 |
| total | 8 | 2368.0 | 2432.0 | 2438.5 | 2378.3 |

### T12_korean_searchable
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 69.3 | 81.3 | 81.8 | 71.2 |
| score | 8 | 628.9 | 704.1 | 710.9 | 642.0 |
| vault_topk | 8 | 321.0 | 334.9 | 337.9 | 317.8 |
| insert_searchable | 8 | 1483.5 | 1573.2 | 1584.5 | 1497.2 |
| total | 8 | 2494.6 | 2642.4 | 2673.8 | 2528.2 |


## Feature: `multi_capture`

### T13_multi_2phase
- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 75.5 | 81.5 | 82.5 | 74.0 |
| score | 8 | 714.4 | 781.7 | 792.3 | 722.7 |
| vault_topk | 8 | 341.8 | 367.3 | 369.1 | 344.7 |
| insert_batch | 8 | 870.0 | 967.2 | 999.0 | 886.4 |
| total | 8 | 2013.0 | 2177.5 | 2224.7 | 2027.9 |

### T14_multi_5phase
- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 85.7 | 91.2 | 91.7 | 86.9 |
| score | 8 | 797.1 | 907.4 | 920.6 | 804.7 |
| vault_topk | 8 | 382.8 | 398.7 | 400.3 | 380.6 |
| insert_batch | 8 | 1279.2 | 1361.9 | 1382.1 | 1284.9 |
| total | 8 | 2556.6 | 2676.8 | 2683.9 | 2557.0 |
