# Rune × envector-msa-1.2.2 Latency Report

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
