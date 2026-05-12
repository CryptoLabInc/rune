# Rune × envector-msa-1.4.3 Latency Report (insert_mode=single)

> **[측정 환경]** pyenvector 1.4.3. **eval_mode=mm32, index_type=ivf_vct**.
> envector 엔드포인트: `0512-1000-0001-ve2lv5txdqrl.clusters.envector.ai` (v1.4.3 클러스터, TLS).
> Vault: `tcp://161.118.149.143:50051` (TLS, 원격).
> **insert_mode=single** — `use_row_insert=True`, `data=[vec]` (row insert API 경로, 벡터 1개씩).
>
> **[비교 주의]** v1.2.2 plan 대비 pyenvector 버전, eval_mode(rmp→mm32), index_type(flat→ivf_vct), envector/Vault 엔드포인트가 동시에 달라짐. 동일 batch 모드(`latency_results_v1.4.3_ivfvct_batch_2026-05-12.md`)와 짝을 이루는 리포트.

## 실행 결과 요약

| 시나리오                           | feature       | p50 total | p95 total | n      | 비고 |
| ---------------------------------- | ------------- | --------- | --------- | ------ | ---- |
| T1 짧은 영문 (~35 tokens)          | capture       | 341.3 ms  | 413.6 ms  | 8      |  |
| T2 긴 영문 (~155 tokens)           | capture       | 359.2 ms  | 395.1 ms  | 8      |  |
| T3 한국어                          | capture       | 380.7 ms  | 415.4 ms  | 8      | embed +59% vs 영문 |
| T4 중복 입력 (near-duplicate path) | capture       | 343.1 ms  | 351.4 ms  | 8      | T1과 거의 동일 |
| T5 Recall — exact match            | recall        | 244.0 ms  | 267.9 ms  | 8      |  |
| T6 Recall — 한→영 cross-language   | recall        | 249.8 ms  | 254.9 ms  | 8      |  |
| T7 Recall — topk 1/3/5/10          | recall        | 235–250 ms | 252–278 ms | 5 each | topk 무관 |
| T9 Vault health check              | vault_status  | 6.5 ms    | 9.9 ms    | 8      |  |
| T10 searchable 짧은 영문           | searchable    | 89.5 ms   | 46238.5 ms | 8     | ⚠ p95 outlier (아래 참조) |
| T11 searchable 긴 영문             | searchable    | 71.4 ms   | 73.1 ms   | 8      |  |
| T12 searchable 한국어              | searchable    | 86.8 ms   | 88.1 ms   | 8      |  |
| T13 multi_capture 2-phase          | multi_capture | 88.4 ms   | 90.0 ms   | 8      |  |
| T14 multi_capture 5-phase          | multi_capture | 144.3 ms  | 146.8 ms  | 8      | 2-phase +56ms로 sub-linear |

**capture phase 비중 (T1 기준)**: embed 16% · score 39% · vault_topk 4% · insert 39%. insert가 더 이상 단독 지배적이지 않고 score와 거의 동률.
**recall 비중 (T5)**: embed 17% · score 56% · vault_topk 6% · remind 23%. FHE score가 여전히 가장 큰 단일 비용.
**recall topk 무관**: T7 topk 1→10 변화에도 total p50 235–250 ms. ivf_vct + nprobe 기반에서도 topk 변경 비용은 무시 가능.
**multi_capture 효율**: 5-phase가 2-phase 대비 +56ms (×1.6), 단일 capture를 5번 돌리는 비용(~1700ms)의 8.5%.

---

## 주목할 점

### capture insert가 더 이상 단독 지배 phase가 아님

| Phase | T1 p50 (ms) | 비중 |
| ----- | ----------- | ---- |
| embed      | 54.4  | 16% |
| score      | 134.8 | 39% |
| vault_topk | 14.5  | 4%  |
| insert     | 134.0 | 39% |
| **total**  | 341.3 | 100% |

`score`와 `insert`가 거의 동률(135ms vs 134ms). ivf_vct 인덱스 + row insert API 경로에서 insert가 가벼워져 score와 비슷한 규모로 떨어졌다. capture 추가 개선 여지는 insert 단독 최적화보단 FHE score 경로 개선에서 더 큰 효과 기대.

### T4 중복 입력에서 near-duplicate path 부담 사라짐

|         | embed | score | vault_topk | insert | total |
| ------- | ----- | ----- | ---------- | ------ | ----- |
| T1 영문 | 54.4  | 134.8 | 14.5       | 134.0  | 341.3 |
| T4 중복 | 56.4  | 132.7 | 14.5       | 136.6  | 343.1 |

T1과 T4가 사실상 동일 (±2ms). 이전(rmp/flat) 환경에서는 T4 score가 T1의 2.7x로 무거웠는데 (v1.2.2 측정: T1 102ms → T4 279ms), mm32/ivf_vct 조합에선 중복 입력의 vault_topk decrypt 분기 비용이 사라졌다. nprobe 기반 후보 셋이 작아 decrypt 결과 차이가 latency에 영향을 안 주는 것으로 추정.

### T3 한국어 embed +59% vs 영문 (score/vault는 영향 없음)

|           | embed | score | vault_topk | insert |
| --------- | ----- | ----- | ---------- | ------ |
| T1 영문   | 54.4  | 134.8 | 14.5       | 134.0  |
| T3 한국어 | 86.2  | 136.0 | 15.2       | 139.1  |

embed만 1.59x (Qwen 토크나이저 + 모델 forward 비용). 이전(v1.2.2 rmp/flat)에서는 한국어가 **score/vault_topk까지 2x 무거웠는데**, mm32/ivf_vct에선 FHE 경로가 토큰 분포에 둔감해졌다. 한국어 capture 최적화 포인트는 임베딩 모델 자체.

### recall topk가 latency에 무영향

| topk | total p50 (ms) |
| ---- | -------------- |
| 1    | 250.0          |
| 3    | 235.0          |
| 5    | 244.2          |
| 10   | 249.5          |

ivf_vct + nprobe 기반 search이지만 topk 변경에 따른 추가 비용은 측정 noise 수준. FHE inner_product가 topk와 무관한 구조라는 점은 v1.2.2 flat과 동일하다.

### ⚠ T10 searchable 첫 시나리오 p95 outlier (median은 정상)

T10 total: p50 89.5ms, p95 **46238ms**. 첫 searchable scenario에서 8회 측정 중 1–2회가 ~45초로 폭증. T11/T12는 71–88ms로 안정. 원인 후보:

- 이전 capture 단계에서 다량 insert된 데이터의 첫 MERGED_SAVED flush가 await_searchable=True 측정 시점에 트리거됨
- 이전 batch 모드 T1 이슈(`async split batch data` UNAVAILABLE)와 같은 서버측 path 초기화 가능성

p50은 정상값이므로 중앙 경향 해석엔 영향 없으나, **search visibility 99분위 SLA를 잡을 때는 별도 측정**이 필요. 단, 본 runner는 await_searchable RPC 제출과 서버 MERGED_SAVED 대기를 분리 측정 불가 (`EnVectorClient.insert()`가 request_id 미반환). 필요 시 `benchmark/runners/insert_row_only.py` 사용.

### searchable insert_searchable phase가 capture insert보다 빠름 — 동작 검증 필요

|           | insert (capture) | insert_searchable (searchable) |
| --------- | ---------------- | ------------------------------ |
| 짧은 영문 | 134.0 ms (T1)    | 8.4 ms (T11/T12 기준, T10 outlier 제외) |

`await_searchable=True`가 더해진 path가 일반 insert보다 빨라지는 건 물리적으로 불가능. 두 가지 가능성:
- runner의 측정 순서상 searchable 시점에는 이미 인덱스가 따끈한 상태 + 동일 row 재삽입 path가 fast-path를 타고 있음
- await_searchable이 실제로는 대기를 수행하지 않고 즉시 반환 (서버 동작 또는 SDK 동작)

T10 p95=46s는 첫 시나리오에서 진짜 MERGED_SAVED 대기가 한 번 일어났다는 신호이므로, T11/T12의 빠른 값은 **이미 searchable 상태인 데이터에 대한 fast-path**로 해석하는 게 자연스럽다. T10–T12를 의도대로 측정하려면 매 측정마다 새로운 vector를 넣어 강제 flush 트리거가 필요.

### multi_capture는 단일 capture 대비 N개당 매우 효율적

| 시나리오 | phase 수 | total p50 | 단일 capture × N 추정 | 절감 |
| -------- | -------- | --------- | --------------------- | ---- |
| T13      | 2        | 88.4 ms   | ~680 ms (T1×2)        | ÷7.7 |
| T14      | 5        | 144.3 ms  | ~1700 ms (T1×5)       | ÷11.8 |

`embed(texts)` 한 번 + 1차 record만 novelty score + 모든 vector를 batch insert(use_row_insert=False). 결정에 여러 phase가 묶이는 실제 capture 경로에서 비용이 거의 N에 비례하지 않는다 — server.py의 `record_builder.build_phases()` 경로를 그대로 따라가는 시나리오가 가치 큼.

### Vault health 6.5ms — endpoint 위치 영향

T9 vault_health_check p50=6.5ms (vault: `tcp://161.118.149.143:50051`). v1.2.2 측정(7.6ms, 다른 IP)과 큰 차이 없음. Vault gRPC RTT는 안정적.


## Environment

- **bench_version**: 1.4.3
- **date**: 2026-05-12
- **envector_endpoint**: 0512-1000-0001-ve2lv5txdqrl.clusters.envector.ai
- **vault_endpoint**: tcp://161.118.149.143:50051
- **embedding_model**: Qwen/Qwen3-Embedding-0.6B
- **embedding_mode**: sbert
- **index_name**: runecontext1
- **key_id**: vault-key
- **eval_mode**: mm32
- **index_type**: ivf_vct
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
| embed | 8 | 54.4 | 56.9 | 57.0 | 54.0 |
| score | 8 | 134.8 | 192.0 | 211.6 | 143.8 |
| vault_topk | 8 | 14.5 | 26.3 | 30.6 | 16.7 |
| insert | 8 | 134.0 | 170.4 | 180.1 | 140.5 |
| total | 8 | 341.3 | 413.6 | 423.7 | 355.0 |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 66.8 | 67.9 | 68.0 | 66.0 |
| score | 8 | 138.8 | 155.1 | 156.8 | 141.7 |
| vault_topk | 8 | 15.7 | 17.1 | 17.2 | 15.4 |
| insert | 8 | 140.2 | 158.7 | 165.5 | 142.2 |
| total | 8 | 359.2 | 395.1 | 400.3 | 365.4 |

### T3_korean
- label: Korean text
- tokens_approx: 50
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 86.2 | 110.0 | 116.8 | 90.4 |
| score | 8 | 136.0 | 158.4 | 161.1 | 140.0 |
| vault_topk | 8 | 15.2 | 22.8 | 25.7 | 16.4 |
| insert | 8 | 139.1 | 149.3 | 151.1 | 139.6 |
| total | 8 | 380.7 | 415.4 | 419.1 | 386.5 |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 56.4 | 58.1 | 58.5 | 55.6 |
| score | 8 | 132.7 | 141.6 | 142.2 | 134.0 |
| vault_topk | 8 | 14.5 | 15.4 | 15.6 | 14.4 |
| insert | 8 | 136.6 | 146.3 | 148.3 | 137.0 |
| total | 8 | 343.1 | 351.4 | 351.4 | 341.1 |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 41.3 | 43.0 | 43.3 | 40.3 |
| score | 8 | 137.4 | 149.7 | 150.0 | 139.1 |
| vault_topk | 8 | 14.3 | 16.0 | 16.4 | 14.7 |
| remind | 8 | 57.3 | 61.7 | 61.9 | 56.5 |
| total | 8 | 244.0 | 267.9 | 268.5 | 250.6 |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 42.2 | 45.1 | 45.2 | 42.3 |
| score | 8 | 133.4 | 143.2 | 144.8 | 134.5 |
| vault_topk | 8 | 14.3 | 22.9 | 25.5 | 15.9 |
| remind | 8 | 54.4 | 59.2 | 59.4 | 54.4 |
| total | 8 | 249.8 | 254.9 | 255.7 | 247.1 |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 36.6 | 38.0 | 38.2 | 36.3 |
| score | 5 | 149.0 | 151.2 | 151.6 | 144.0 |
| vault_topk | 5 | 14.7 | 16.5 | 16.8 | 15.0 |
| remind | 5 | 55.3 | 66.9 | 68.4 | 56.2 |
| total | 5 | 250.0 | 261.1 | 261.7 | 251.5 |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 36.7 | 37.6 | 37.7 | 33.2 |
| score | 5 | 135.3 | 147.9 | 149.9 | 136.8 |
| vault_topk | 5 | 14.8 | 18.1 | 18.7 | 15.6 |
| remind | 5 | 53.3 | 56.5 | 56.8 | 53.8 |
| total | 5 | 235.0 | 252.7 | 254.9 | 239.4 |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 34.3 | 37.9 | 38.2 | 33.4 |
| score | 5 | 138.8 | 166.4 | 169.1 | 144.4 |
| vault_topk | 5 | 14.4 | 16.4 | 16.8 | 14.9 |
| remind | 5 | 55.6 | 60.5 | 61.3 | 55.3 |
| total | 5 | 244.2 | 267.8 | 271.0 | 248.0 |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 32.9 | 36.2 | 36.2 | 32.8 |
| score | 5 | 142.5 | 182.2 | 187.1 | 150.5 |
| vault_topk | 5 | 14.7 | 15.5 | 15.6 | 14.9 |
| remind | 5 | 53.6 | 55.9 | 56.2 | 54.0 |
| total | 5 | 249.5 | 278.4 | 282.4 | 252.2 |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| vault_health_check | 8 | 6.5 | 9.9 | 10.6 | 7.0 |


## Feature: `searchable`

### T10_short_en_searchable
- label: short English (~30 tokens)
- tokens_approx: 35
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 71.9 | 1686.0 | 2101.6 | 421.1 |
| score | 8 | 9.6 | 615.1 | 624.2 | 180.3 |
| vault_topk | 8 | 0.0 | 16.6 | 16.8 | 6.0 |
| insert_searchable | 8 | 8.4 | 44512.6 | 44954.6 | 12462.7 |
| total | 8 | 89.5 | 46238.5 | 46391.1 | 13070.1 |

### T11_long_en_searchable
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 53.3 | 54.2 | 54.4 | 53.5 |
| score | 8 | 9.2 | 10.1 | 10.1 | 9.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 8.5 | 10.4 | 10.9 | 8.6 |
| total | 8 | 71.4 | 73.1 | 73.3 | 71.1 |

### T12_korean_searchable
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 69.7 | 70.4 | 70.6 | 69.7 |
| score | 8 | 9.0 | 10.2 | 10.4 | 9.3 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 7.7 | 9.3 | 9.7 | 8.0 |
| total | 8 | 86.8 | 88.1 | 88.5 | 87.0 |


## Feature: `multi_capture`

### T13_multi_2phase
- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 69.8 | 70.7 | 71.0 | 69.9 |
| score | 8 | 9.0 | 11.6 | 12.1 | 9.4 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 8.3 | 9.9 | 9.9 | 8.5 |
| total | 8 | 88.4 | 90.0 | 90.0 | 87.8 |

### T14_multi_5phase
- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 127.3 | 129.0 | 129.2 | 127.7 |
| score | 8 | 9.9 | 10.7 | 10.8 | 9.7 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 7.1 | 8.2 | 8.2 | 7.3 |
| total | 8 | 144.3 | 146.8 | 147.1 | 144.7 |
