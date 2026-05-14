# Rune × envector-msa-1.4.3 Latency Report

## Environment

- **bench_version**: 1.4.3
- **date**: 2026-05-14
- **envector_endpoint**: vault-38c4ec-od73k6fb0p1w.clusters.envector.ai
- **vault_endpoint**: tcp://161.118.149.143:50051
- **embedding_model**: Qwen/Qwen3-Embedding-0.6B
- **embedding_mode**: sbert
- **index_name**: runecontext
- **key_id**: vault-key
- **eval_mode**: mm32
- **index_type**: ivf_vct
- **insert_mode**: single
- **network_rtt**: unknown
- **runs_per_scenario**: 8
- **warmup_runs**: 2

## 실행결과 요약 (single insert-mode)

| Feature       | 시나리오                    | p50 ms    | p95 ms    | p99 ms    | n   | Status |
| ------------- | --------------------------- | --------: | --------: | --------: | --: | ------ |
| capture       | T1_short_en                 |     457.7 |     481.6 |     485.5 |   8 | OK     |
| capture       | T2_long_en                  |     433.6 |     625.6 |     683.1 |   8 | OK     |
| capture       | T3_korean                   |     306.5 |     337.9 |     345.8 |   8 | OK     |
| capture       | T4_duplicate                |     282.7 |     300.2 |     301.0 |   8 | OK     |
| recall        | T5_exact_match              |     257.9 |     288.4 |     292.0 |   8 | OK     |
| recall        | T6_cross_lang               |     247.9 |     516.0 |     620.9 |   8 | OK     |
| recall        | T7_topk_1                   |     234.2 |     244.6 |     245.3 |   5 | OK     |
| recall        | T7_topk_3                   |     261.4 |     275.2 |     277.9 |   5 | OK     |
| recall        | T7_topk_5                   |     244.5 |     266.6 |     270.4 |   5 | OK     |
| recall        | T7_topk_10                  |     240.6 |     249.1 |     249.8 |   5 | OK     |
| vault_status  | T9_vault_status             |       7.6 |      12.2 |      13.7 |   8 | OK     |
| searchable    | T10_short_en_searchable     |         — |         — |         — |   0 | FAIL (timeout 60s) |
| searchable    | T11_long_en_searchable      |         — |         — |         — |   0 | FAIL (timeout 60s) |
| searchable    | T12_korean_searchable       |  15 997.3 |  16 194.8 |  16 204.2 |   8 | OK     |
| multi_capture | T13_multi_2phase            |     367.0 |  47 826.5 |  62 200.3 |   8 | OK (p95 outlier) |
| multi_capture | T14_multi_5phase            |     150.2 |     163.8 |     166.2 |   8 | OK     |

## 해석 및 주목할 점 (single insert-mode)

### 핵심 요약
- **capture p50: 282–458 ms**, **recall p50: 234–262 ms**, **vault health: 7.6 ms**.
- capture latency의 약 55–60%가 `insert` 단계 (row-insert RPC 자체 비용)에 집중됨. embed/score/vault_topk는 비교적 안정.
- recall은 모든 topk 값(1/3/5/10)에서 234–262 ms 범위로 거의 동일 — **topk가 latency에 미치는 영향 미미**. score(FHE) 비용이 지배적이고 remind 단계는 부차적.

### Phase별 패턴
- **embed**: 영어 단문 58–69 ms, 한국어 82 ms, recall query 35–46 ms — 한국어 토큰화/세그멘테이션이 약 20–25 ms 추가.
- **score (FHE novelty)**: 모든 capture/recall에서 일관되게 125–135 ms p50. p95에서 가끔 300–480 ms 튐 (T2, T6) → 서버 큐잉 또는 batched flush 영향 추정.
- **vault_topk**: 15–16 ms로 매우 안정 (gRPC RTT + FHE 부분 복호화).
- **insert (row-insert)**: 영어 단문 256 ms, 영어 장문 228 ms, 한국어 74 ms, 중복 79 ms → **텍스트 길이보다 vault TopK가 hit한 후 novelty path로 분기되었는지가 영향이 큼**. 한국어/중복은 score에서 novelty 낮게 나와 빠른 경로로 빠진 것으로 추정.

### 주목할 만한 이슈
1. **searchable T10/T11 60 s 타임아웃 실패** — `wait_for_insert_stage`가 MERGED_SAVED를 못 잡음. T12(한국어)만 성공했는데 `segmentation_wait` 단독으로 **15 350 ms p50** — single insert 후 백엔드 segmentation 완료까지 15초 이상 걸리는 게 현재 구조. searchable SLA 관점에서 가장 큰 병목.
2. **T13 (2-phase multi) p95 47 826 ms 거대 outlier** — `insert_batch` 단독 p95=47 484 ms. 출력 로그상 "Application error from server" 다수. multi_capture 경로에 서버 측 일시적 장애 또는 retry 폭주 가능성. T14(5-phase)는 영향 없이 정상(150 ms p50) → **2-phase 특정 페이로드 패턴이 트리거할 가능성**.
3. **T2 long_en score p95 = 299.6 ms** (p50의 2.3배) — 페이로드 길이가 FHE 연산 시간을 늘리지는 않지만, score 시점의 서버 부하 분산이 p95 변동을 키움.

## Feature: `capture`

### T1_short_en

- label: short English (~30 tokens)
- tokens_approx: 35
- insert_mode: single
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 58.5   | 59.7   | 59.8   | 58.1    |
| score      | 8   | 128.0  | 138.5  | 138.6  | 130.8   |
| vault_topk | 8   | 15.5   | 18.5   | 18.9   | 16.1    |
| insert     | 8   | 255.6  | 268.0  | 272.8  | 256.2   |
| total      | 8   | 457.7  | 481.6  | 485.5  | 461.3   |

### T2_long_en

- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: single
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 68.8   | 70.9   | 71.2   | 68.5    |
| score      | 8   | 129.6  | 299.6  | 357.0  | 163.3   |
| vault_topk | 8   | 15.1   | 16.6   | 16.9   | 15.3    |
| insert     | 8   | 228.3  | 265.7  | 276.9  | 197.1   |
| total      | 8   | 433.6  | 625.6  | 683.1  | 444.1   |

### T3_korean

- label: Korean text
- tokens_approx: 50
- insert_mode: single
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 82.3   | 86.2   | 86.7   | 82.4    |
| score      | 8   | 130.7  | 143.6  | 145.9  | 133.0   |
| vault_topk | 8   | 15.5   | 17.0   | 17.1   | 15.8    |
| insert     | 8   | 74.0   | 108.1  | 119.5  | 78.4    |
| total      | 8   | 306.5  | 337.9  | 345.8  | 309.6   |

### T4_duplicate

- label: duplicate input — tests novelty near-duplicate path
- insert_mode: single
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 58.8   | 60.4   | 60.5   | 58.2    |
| score      | 8   | 135.5  | 144.0  | 145.5  | 136.0   |
| vault_topk | 8   | 15.8   | 19.3   | 20.0   | 16.3    |
| insert     | 8   | 78.8   | 84.8   | 85.5   | 75.7    |
| total      | 8   | 282.7  | 300.2  | 301.0  | 286.3   |

## Feature: `recall`

### T5_exact_match

- label: exact match query
- topk: 5
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 44.8   | 46.0   | 46.4   | 43.8    |
| score      | 8   | 125.2  | 150.4  | 155.8  | 130.8   |
| vault_topk | 8   | 15.6   | 17.4   | 17.7   | 15.6    |
| remind     | 8   | 72.2   | 78.5   | 79.5   | 72.3    |
| total      | 8   | 257.9  | 288.4  | 292.0  | 262.5   |

### T6_cross_lang

- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 46.1   | 48.4   | 48.6   | 45.6    |
| score      | 8   | 128.5  | 379.8  | 481.6  | 176.2   |
| vault_topk | 8   | 15.1   | 19.4   | 20.5   | 15.9    |
| remind     | 8   | 64.1   | 75.2   | 76.2   | 63.8    |
| total      | 8   | 247.9  | 516.0  | 620.9  | 301.5   |

### T7_topk_1

- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 35.8   | 37.9   | 38.3   | 36.0    |
| score      | 5   | 125.8  | 132.0  | 132.9  | 127.5   |
| vault_topk | 5   | 15.5   | 16.3   | 16.5   | 15.5    |
| remind     | 5   | 55.7   | 62.6   | 63.0   | 57.2    |
| total      | 5   | 234.2  | 244.6  | 245.3  | 236.2   |

### T7_topk_3

- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 35.3   | 41.4   | 41.8   | 37.1    |
| score      | 5   | 129.8  | 152.7  | 156.3  | 135.5   |
| vault_topk | 5   | 15.5   | 16.1   | 16.2   | 15.5    |
| remind     | 5   | 76.2   | 79.9   | 80.3   | 76.2    |
| total      | 5   | 261.4  | 275.2  | 277.9  | 264.2   |

### T7_topk_5

- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 37.2   | 39.2   | 39.3   | 36.9    |
| score      | 5   | 133.0  | 138.8  | 139.6  | 131.5   |
| vault_topk | 5   | 15.0   | 20.5   | 21.5   | 16.2    |
| remind     | 5   | 63.5   | 74.6   | 75.3   | 65.4    |
| total      | 5   | 244.5  | 266.6  | 270.4  | 250.1   |

### T7_topk_10

- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 39.6   | 41.5   | 41.8   | 38.8    |
| score      | 5   | 126.1  | 129.2  | 129.7  | 124.9   |
| vault_topk | 5   | 15.0   | 16.6   | 16.7   | 15.3    |
| remind     | 5   | 62.7   | 67.1   | 67.6   | 63.0    |
| total      | 5   | 240.6  | 249.1  | 249.8  | 241.9   |

## Feature: `vault_status`

### T9_vault_status

- label: Vault gRPC health check
- runs: 8

| Phase              | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ------------------ | --- | ------ | ------ | ------ | ------- |
| vault_health_check | 8   | 7.6    | 12.2   | 13.7   | 8.3     |

## Feature: `searchable`

### T10_short_en_searchable

- label: short English (~30 tokens)
- tokens_approx: 35

> **Error**: wait_for_insert_stage failed: EnvectorTimeoutError("Timed out waiting for index operation state MERGED_SAVED (index='runecontext', request_id='01KRJ30V62H8KVHJ3CSR9JQCZA', total_row_count=1, searchable_row_count=1, elapsed=60.48s, timeout=59.99995266698534s). | Action: Retry with longer timeout | Request ID: 01KRJ30V62H8KVHJ3CSR9JQCZA")

### T11_long_en_searchable

- label: long English (~150 tokens)
- tokens_approx: 155

> **Error**: wait_for_insert_stage failed: EnvectorTimeoutError("Timed out waiting for index operation state MERGED_SAVED (index='runecontext', request_id='01KRJ34HQFQGTZH9Q0G63N9YWK', total_row_count=1, searchable_row_count=1, elapsed=60.12s, timeout=59.99998391699046s). | Action: Retry with longer timeout | Request ID: 01KRJ34HQFQGTZH9Q0G63N9YWK")

### T12_korean_searchable

- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase             | n   | p50 ms  | p95 ms  | p99 ms  | mean ms |
| ----------------- | --- | ------- | ------- | ------- | ------- |
| embed             | 8   | 146.6   | 155.6   | 156.8   | 143.2   |
| score             | 8   | 175.1   | 195.8   | 196.8   | 178.7   |
| vault_topk        | 8   | 16.0    | 20.7    | 20.7    | 17.2    |
| insert_rpc        | 8   | 260.3   | 362.1   | 367.7   | 278.5   |
| segmentation_wait | 8   | 15350.3 | 15575.6 | 15589.6 | 15361.4 |
| searchable_total  | 8   | 15676.9 | 15848.0 | 15858.3 | 15639.9 |
| total             | 8   | 15997.3 | 16194.8 | 16204.2 | 15979.1 |

## Feature: `multi_capture`

### T13_multi_2phase

- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase        | n   | p50 ms | p95 ms  | p99 ms  | mean ms |
| ------------ | --- | ------ | ------- | ------- | ------- |
| embed_batch  | 8   | 103.3  | 155.5   | 161.1   | 107.8   |
| score        | 8   | 88.2   | 366.4   | 447.7   | 128.2   |
| vault_topk   | 8   | 0.0    | 15.8    | 15.9    | 5.8     |
| insert_batch | 8   | 12.4   | 47484.0 | 61855.0 | 11621.1 |
| total        | 8   | 367.0  | 47826.5 | 62200.3 | 11863.0 |

### T14_multi_5phase

- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase        | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ------------ | --- | ------ | ------ | ------ | ------- |
| embed_batch  | 8   | 128.5  | 129.7  | 130.0  | 128.5   |
| score        | 8   | 11.6   | 23.1   | 26.9   | 13.5    |
| vault_topk   | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| insert_batch | 8   | 9.6    | 16.3   | 16.6   | 11.0    |
| total        | 8   | 150.2  | 163.8  | 166.2  | 153.1   |
