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
- **insert_mode**: batch
- **network_rtt**: unknown
- **runs_per_scenario**: 8
- **warmup_runs**: 2

## 해석 및 주목할 점 (batch insert-mode)

### 핵심 요약
- **capture p50: 59–89 ms** (single 대비 5–7배 빠름), **recall p50: 34–40 ms** (single 대비 6–7배 빠름).
- `insert` 단계가 8–10 ms로 급감 — batch API 경로가 row-insert 대비 RPC overhead를 거의 제거.
- vault health 6.3 ms (single 7.6 ms와 유사) — gRPC 자체 RTT는 일관.

### Phase별 패턴
- **score**: capture 9–13 ms, recall 8–11 ms — single 모드 125–135 ms 대비 **10배 이상 빠름**. batch insert 경로에서는 score가 다른 코드 패스(예: cached 또는 light path)로 분기되었거나, novelty 결과가 비어 FHE 결과 디코드 비용이 줄었을 가능성 큼.
- **vault_topk = 0.0 (대부분)**: `encrypted_blobs`가 비어 vault decrypt를 스킵한 것. score 결과가 hit가 없거나 batch 경로에서 blobs를 반환하지 않는 케이스 → **recall이 실제 결과를 반환하는지 별도 검증 필요** (T5–T7에서 `remind=0.0`도 같은 이유).
- **insert (batch)**: 8–10 ms로 일정. 단, T1에서 p95=52 933 ms / mean=10 924 ms 거대 outlier 1회 (다른 runs는 정상). cold-start 또는 일시적 backend 응답 지연일 가능성. 

### 주목할 만한 이슈
1. **T1 short_en `insert` p95 52 933 ms (p99 70 237 ms)** — 8회 중 1회만 수십 초 걸린 spike. p50은 10.2 ms로 정상. **batch 모드에서도 첫 insert 시점에 cold-start spike가 살아있음**. warmup=2로 부족, 사실상 warmup=3 이상이 필요해 보임.
2. **searchable T10/T11/T12 전부 실패** — `DependencyError: connection refused (10.96.83.35:50051)`. **백엔드 인덱스 노드와의 연결 자체가 끊긴 상태**. single 실행 때는 60s 타임아웃 형태였는데 batch 실행 시점에는 connection refused로 더 명확한 인프라 이슈로 표출. 측정 인터벌 사이 서버 상태 변화 추정.
3. **recall에서 `vault_topk`/`remind` = 0.0** — single 모드에서는 15 ms / 60–80 ms로 정상 측정되었던 단계가 batch 실행 시점에 전부 0. **recall이 실제로 결과를 받아오는지, blobs가 왜 비는지 확인 필요**. 단순 score만 측정되고 있다면 batch 모드 recall p50(35 ms)은 과소평가됨.
4. **multi_capture는 정상** — T13 87.9 ms / T14 147.5 ms. single 실행에서 보였던 T13 outlier(47 826 ms)가 batch에서는 사라짐 → multi_capture 경로는 본래 batch insert를 쓰므로 mode 차이가 거의 없어야 하는데, single 측정 시 발생한 "Application error from server" 폭주가 batch 실행 시점엔 진정된 것으로 보임.

### single 대비 정량 비교 (p50 ms)

| 시나리오            | single | batch | 배수      |
| ------------------- | -----: | ----: | --------: |
| T1 short_en         | 457.7  |  73.7 | 6.2× 빠름 |
| T2 long_en          | 433.6  |  72.7 | 6.0× 빠름 |
| T3 korean           | 306.5  |  88.8 | 3.5× 빠름 |
| T4 duplicate        | 282.7  |  59.3 | 4.8× 빠름 |
| T5 exact_match      | 257.9  |  35.9 | 7.2× 빠름 |
| T6 cross_lang       | 247.9  |  38.8 | 6.4× 빠름 |
| T7 topk_5           | 244.5  |  34.7 | 7.0× 빠름 |
| T14 multi_5phase    | 150.2  | 147.5 |    동등   |


## Feature: `capture`

### T1_short_en
- label: short English (~30 tokens)
- tokens_approx: 35
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 52.6 | 111.7 | 116.6 | 67.2 |
| score | 8 | 13.3 | 360.7 | 373.4 | 116.7 |
| vault_topk | 8 | 0.0 | 18.2 | 18.4 | 4.5 |
| insert | 8 | 10.2 | 52933.6 | 70237.1 | 10924.5 |
| total | 8 | 73.7 | 53277.7 | 70528.1 | 11112.9 |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 53.2 | 53.9 | 54.1 | 53.3 |
| score | 8 | 10.2 | 12.8 | 13.6 | 10.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 8.7 | 11.5 | 12.5 | 9.2 |
| total | 8 | 72.7 | 74.8 | 75.2 | 72.5 |

### T3_korean
- label: Korean text
- tokens_approx: 50
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 69.6 | 70.0 | 70.1 | 69.5 |
| score | 8 | 8.9 | 11.7 | 12.3 | 9.2 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 10.3 | 11.5 | 11.7 | 9.6 |
| total | 8 | 88.8 | 90.2 | 90.3 | 88.4 |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 41.8 | 41.9 | 41.9 | 41.8 |
| score | 8 | 9.1 | 11.0 | 11.4 | 9.2 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 8.2 | 8.6 | 8.6 | 8.1 |
| total | 8 | 59.3 | 60.5 | 60.5 | 59.0 |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 26.4 | 26.7 | 26.7 | 26.4 |
| score | 8 | 9.4 | 13.5 | 14.4 | 9.7 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 8 | 35.9 | 39.8 | 40.7 | 36.1 |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 29.6 | 31.2 | 31.4 | 29.9 |
| score | 8 | 8.8 | 11.4 | 12.2 | 9.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 8 | 38.8 | 41.2 | 41.7 | 38.8 |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 28.9 | 32.0 | 32.6 | 29.0 |
| score | 5 | 8.0 | 9.8 | 9.8 | 8.5 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 38.6 | 40.3 | 40.6 | 37.4 |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 26.2 | 28.5 | 28.8 | 26.8 |
| score | 5 | 10.9 | 12.3 | 12.4 | 10.4 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 37.0 | 40.0 | 40.4 | 37.2 |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 25.5 | 26.7 | 26.9 | 25.8 |
| score | 5 | 8.3 | 9.6 | 9.7 | 8.6 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 34.7 | 35.1 | 35.1 | 34.4 |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 25.6 | 26.6 | 26.8 | 25.8 |
| score | 5 | 8.5 | 9.9 | 10.1 | 8.7 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 34.3 | 35.6 | 35.7 | 34.5 |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| vault_health_check | 8 | 6.3 | 8.0 | 8.4 | 6.5 |


## Feature: `searchable`

### T10_short_en_searchable
- label: short English (~30 tokens)
- tokens_approx: 35

> **Error**: wait_for_insert_stage failed: DependencyError('failed to forward GetIndexList request to backend: rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial tcp 10.96.83.35:50051: connect: connection refused" | Action: Retry in a few seconds | Request ID: 41b06a4b3d2aee0b4b6c')

### T11_long_en_searchable
- label: long English (~150 tokens)
- tokens_approx: 155

> **Error**: wait_for_insert_stage failed: DependencyError('failed to forward GetIndexList request to backend: rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial tcp 10.96.83.35:50051: connect: connection refused" | Action: Retry in a few seconds | Request ID: 36b35ab094a1d323a6de')

### T12_korean_searchable
- label: Korean text
- tokens_approx: 50

> **Error**: wait_for_insert_stage failed: DependencyError('failed to forward GetIndexList request to backend: rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial tcp 10.96.83.35:50051: connect: connection refused" | Action: Retry in a few seconds | Request ID: 743831b32bd9c9a0f79c')


## Feature: `multi_capture`

### T13_multi_2phase
- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 69.9 | 70.4 | 70.4 | 69.9 |
| score | 8 | 8.9 | 10.1 | 10.2 | 9.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 8.8 | 10.7 | 11.1 | 9.0 |
| total | 8 | 87.9 | 89.3 | 89.5 | 87.9 |

### T14_multi_5phase
- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 128.9 | 129.3 | 129.3 | 128.8 |
| score | 8 | 10.3 | 15.2 | 17.0 | 11.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 8.7 | 9.5 | 9.6 | 8.5 |
| total | 8 | 147.5 | 153.4 | 155.5 | 148.3 |
