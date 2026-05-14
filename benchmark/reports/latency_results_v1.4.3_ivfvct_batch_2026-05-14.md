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
