# Rune × envector-msa-1.4.3 Latency Report

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
| embed | 8 | 110.3 | 140.5 | 144.5 | 106.5 |
| score | 8 | 196.5 | 3205.5 | 3228.1 | 924.6 |
| vault_topk | 8 | 0.0 | 27.9 | 28.8 | 9.1 |
| insert | 8 | 6928.3 | 13457.2 | 13608.9 | 6864.4 |
| total | 8 | 8668.1 | 15770.4 | 16775.0 | 7904.7 |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 65.7 | 68.6 | 68.6 | 62.4 |
| score | 8 | 8.5 | 9.8 | 10.0 | 8.7 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 44.7 | 46.0 | 46.3 | 40.2 |
| total | 8 | 113.5 | 122.6 | 122.9 | 111.2 |

### T3_korean
- label: Korean text
- tokens_approx: 50
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 77.4 | 85.1 | 85.4 | 77.8 |
| score | 8 | 9.8 | 12.7 | 13.3 | 10.5 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 42.4 | 45.3 | 45.6 | 42.6 |
| total | 8 | 132.2 | 137.8 | 137.9 | 130.9 |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 43.6 | 56.3 | 56.5 | 47.8 |
| score | 8 | 9.9 | 16.8 | 17.0 | 11.7 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 43.3 | 45.1 | 45.4 | 42.7 |
| total | 8 | 99.2 | 114.1 | 116.6 | 102.1 |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 30.5 | 32.4 | 32.8 | 30.4 |
| score | 8 | 12.2 | 16.4 | 16.6 | 13.3 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 8 | 43.5 | 45.9 | 46.4 | 43.7 |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 30.7 | 31.6 | 31.7 | 30.7 |
| score | 8 | 13.6 | 16.0 | 16.9 | 13.1 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 8 | 44.2 | 46.1 | 46.9 | 43.8 |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 29.5 | 29.9 | 29.9 | 29.4 |
| score | 5 | 13.4 | 17.4 | 17.9 | 14.3 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 43.3 | 46.1 | 46.5 | 43.7 |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 29.6 | 32.5 | 33.0 | 29.7 |
| score | 5 | 14.7 | 18.8 | 19.4 | 14.8 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 44.3 | 46.5 | 46.8 | 44.5 |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 29.1 | 29.2 | 29.2 | 28.8 |
| score | 5 | 15.5 | 16.6 | 16.7 | 15.1 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 43.8 | 45.2 | 45.2 | 44.0 |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 28.0 | 29.1 | 29.2 | 28.2 |
| score | 5 | 14.9 | 17.0 | 17.0 | 15.5 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 44.1 | 45.5 | 45.8 | 43.7 |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| vault_health_check | 8 | 6.3 | 7.2 | 7.5 | 6.2 |


## Feature: `searchable`

### T10_short_en_searchable
- label: short English (~30 tokens)
- tokens_approx: 35
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 45.0 | 53.7 | 54.0 | 46.7 |
| score | 8 | 9.1 | 9.9 | 10.0 | 9.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 44.4 | 49.7 | 51.5 | 44.9 |
| total | 8 | 99.8 | 106.8 | 108.2 | 100.7 |

### T11_long_en_searchable
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 58.4 | 63.5 | 64.3 | 59.1 |
| score | 8 | 8.5 | 11.8 | 13.1 | 9.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 44.8 | 47.6 | 48.0 | 45.0 |
| total | 8 | 113.4 | 116.0 | 116.1 | 113.2 |

### T12_korean_searchable
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 70.6 | 71.2 | 71.3 | 70.5 |
| score | 8 | 8.2 | 9.3 | 9.4 | 8.4 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 45.2 | 46.9 | 47.4 | 40.9 |
| total | 8 | 124.2 | 125.3 | 125.5 | 119.9 |


## Feature: `multi_capture`

### T13_multi_2phase
- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 69.9 | 71.1 | 71.2 | 70.1 |
| score | 8 | 9.4 | 11.7 | 11.8 | 9.7 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 41.0 | 43.2 | 43.3 | 41.1 |
| total | 8 | 120.6 | 123.0 | 123.2 | 120.9 |

### T14_multi_5phase
- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 137.6 | 148.6 | 149.6 | 138.5 |
| score | 8 | 11.4 | 67.5 | 90.0 | 22.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 41.9 | 44.0 | 44.3 | 38.9 |
| total | 8 | 192.7 | 236.2 | 250.2 | 199.4 |
