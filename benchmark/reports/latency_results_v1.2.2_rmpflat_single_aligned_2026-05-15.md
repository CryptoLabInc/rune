# Rune × envector-msa-1.2.2 Latency Report

## Environment

- **bench_version**: 1.4.3
- **date**: 2026-05-15
- **envector_endpoint**: 0511-1401-0001-4r6cwxfc908b.clusters.envector.io
- **vault_endpoint**: tcp://193.122.124.173:50051
- **embedding_model**: Qwen/Qwen3-Embedding-0.6B
- **embedding_mode**: sbert
- **index_name**: runecontext_bench
- **key_id**: vault-key
- **eval_mode**: rmp
- **index_type**: flat
- **insert_mode**: single
- **network_rtt**: unknown
- **runs_per_scenario**: 8
- **warmup_runs**: 2
- **direct_envector**: True
- **reset_policy**: per-scenario drop+create


## Feature: `capture`

### T1_short_en
- label: short English (~30 tokens)
- tokens_approx: 35
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 36.0 | 42.9 | 43.0 | 37.3 |
| score | 8 | 73.7 | 93.4 | 93.9 | 78.4 |
| vault_topk | 8 | 35.1 | 37.5 | 37.7 | 34.0 |
| insert | 8 | 495.8 | 607.5 | 647.9 | 497.3 |
| total | 8 | 651.2 | 750.6 | 783.5 | 646.9 |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 41.6 | 44.6 | 44.6 | 41.3 |
| score | 8 | 75.6 | 97.1 | 102.1 | 78.6 |
| vault_topk | 8 | 30.2 | 40.5 | 42.7 | 30.0 |
| insert | 8 | 470.8 | 510.4 | 512.1 | 474.9 |
| total | 8 | 634.1 | 653.8 | 656.2 | 624.8 |

### T3_korean
- label: Korean text
- tokens_approx: 50
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 47.3 | 50.3 | 50.6 | 47.4 |
| score | 8 | 79.3 | 94.9 | 97.5 | 79.2 |
| vault_topk | 8 | 32.2 | 49.9 | 53.3 | 33.4 |
| insert | 8 | 488.3 | 634.1 | 683.4 | 510.8 |
| total | 8 | 653.0 | 808.9 | 866.0 | 670.8 |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- insert_mode: single
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 36.5 | 41.1 | 41.2 | 37.1 |
| score | 8 | 87.2 | 123.8 | 127.4 | 91.7 |
| vault_topk | 8 | 34.1 | 45.5 | 48.0 | 33.9 |
| insert | 8 | 484.6 | 564.3 | 578.7 | 495.3 |
| total | 8 | 665.1 | 731.5 | 750.1 | 658.0 |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 31.4 | 34.3 | 34.4 | 30.1 |
| score | 8 | 154.6 | 156.1 | 156.5 | 151.8 |
| vault_topk | 8 | 75.9 | 81.9 | 83.4 | 74.9 |
| remind | 8 | 39.7 | 43.9 | 44.0 | 40.1 |
| total | 8 | 298.0 | 309.7 | 311.1 | 296.9 |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 33.6 | 36.2 | 36.3 | 33.7 |
| score | 8 | 162.9 | 168.7 | 169.3 | 162.0 |
| vault_topk | 8 | 75.8 | 86.1 | 86.8 | 77.9 |
| remind | 8 | 47.1 | 48.8 | 48.8 | 45.8 |
| total | 8 | 320.0 | 330.7 | 331.7 | 319.3 |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 33.1 | 41.5 | 42.8 | 35.0 |
| score | 5 | 170.3 | 175.7 | 176.6 | 167.5 |
| vault_topk | 5 | 74.6 | 77.2 | 77.4 | 75.0 |
| remind | 5 | 42.2 | 43.8 | 43.8 | 42.0 |
| total | 5 | 321.4 | 334.6 | 336.3 | 319.5 |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 34.2 | 36.3 | 36.5 | 34.4 |
| score | 5 | 160.3 | 187.9 | 193.0 | 166.1 |
| vault_topk | 5 | 77.5 | 79.5 | 79.9 | 76.5 |
| remind | 5 | 46.0 | 46.4 | 46.4 | 44.7 |
| total | 5 | 315.8 | 345.5 | 350.9 | 321.8 |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 34.7 | 35.2 | 35.3 | 34.1 |
| score | 5 | 153.7 | 165.0 | 166.0 | 155.6 |
| vault_topk | 5 | 77.8 | 79.0 | 79.1 | 74.1 |
| remind | 5 | 45.5 | 54.2 | 55.6 | 47.0 |
| total | 5 | 310.2 | 321.5 | 322.0 | 310.8 |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 36.1 | 36.5 | 36.6 | 34.8 |
| score | 5 | 150.0 | 156.4 | 157.3 | 150.2 |
| vault_topk | 5 | 76.0 | 77.2 | 77.4 | 75.1 |
| remind | 5 | 49.7 | 57.5 | 58.0 | 50.5 |
| total | 5 | 310.0 | 321.9 | 324.0 | 310.7 |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| vault_health_check | 8 | 8.4 | 9.0 | 9.1 | 8.2 |


## Feature: `searchable`

### T10_short_en_searchable
- label: short English (~30 tokens)
- tokens_approx: 35
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 40.4 | 44.4 | 44.5 | 40.7 |
| score | 8 | 75.5 | 95.7 | 99.2 | 77.3 |
| vault_topk | 8 | 28.9 | 48.1 | 53.6 | 30.7 |
| insert_searchable | 8 | 609.1 | 731.8 | 741.3 | 633.5 |
| total | 8 | 772.4 | 887.2 | 900.7 | 782.2 |

### T11_long_en_searchable
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 48.9 | 58.1 | 58.7 | 50.4 |
| score | 8 | 70.5 | 96.9 | 101.2 | 75.1 |
| vault_topk | 8 | 28.5 | 37.6 | 38.2 | 28.8 |
| insert_searchable | 8 | 581.7 | 622.4 | 626.3 | 583.2 |
| total | 8 | 731.2 | 799.4 | 806.3 | 737.6 |

### T12_korean_searchable
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 47.6 | 52.5 | 52.8 | 47.4 |
| score | 8 | 71.5 | 136.8 | 148.0 | 87.7 |
| vault_topk | 8 | 31.8 | 63.0 | 66.7 | 35.3 |
| insert_searchable | 8 | 567.6 | 752.6 | 763.9 | 601.4 |
| total | 8 | 714.8 | 1001.6 | 1026.9 | 771.8 |


## Feature: `multi_capture`

### T13_multi_2phase
- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 43.8 | 80.2 | 85.1 | 52.5 |
| score | 8 | 69.1 | 92.9 | 95.8 | 73.7 |
| vault_topk | 8 | 27.8 | 43.0 | 45.0 | 29.5 |
| insert_batch | 8 | 855.3 | 905.9 | 913.5 | 856.9 |
| total | 8 | 1013.4 | 1078.2 | 1089.7 | 1012.6 |

### T14_multi_5phase
- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 96.3 | 104.7 | 104.8 | 96.1 |
| score | 8 | 73.4 | 97.0 | 101.5 | 75.1 |
| vault_topk | 8 | 29.4 | 40.6 | 41.0 | 30.2 |
| insert_batch | 8 | 1231.7 | 1336.4 | 1346.3 | 1233.7 |
| total | 8 | 1424.0 | 1558.9 | 1560.4 | 1435.1 |
