# Rune × envector-msa-1.4.0 Latency Report

> **[측정 환경]** envector 연결은 `secure=false`(plaintext gRPC)로 측정. Vault(`localhost:50051`)는 TLS 활성 상태.
> plaintext로 진행해도 무방하다고 판단한 이유: gRPC는 HTTP/2 커넥션을 재사용하므로 TLS 핸드셰이크 비용이 warmup 이후 측정값에 포함되지 않으며, 주요 병목이 FHE 연산에 있어 TLS 유무가 결과에 실질적 영향이 낮을 수 있다고 판단.
> 이후 envector TLS 환경(`secure=true`)에서도 동일 시나리오로 벤치마크를 진행할 예정.

## 실행 결과 요약

| 시나리오                           | feature       | p50 total | p95 total | n      |
| ---------------------------------- | ------------- | --------- | --------- | ------ |
| T1 짧은 영문 (~35 tokens)          | capture       | 102.8 ms  | 104.8 ms  | 8      |
| T2 긴 영문 (~155 tokens)           | capture       | 123.2 ms  | 126.8 ms  | 8      |
| T3 한국어                          | capture       | 155.0 ms  | 165.3 ms  | 8      |
| T4 중복 입력 (near-duplicate path) | capture       | 106.8 ms  | 201.2 ms  | 8      |
| T5 Recall — exact match            | recall        | 37.3 ms   | 40.4 ms   | 8      |
| T6 Recall — 한→영 cross-language   | recall        | 38.4 ms   | 39.3 ms   | 8      |
| T7 Recall — topk 1/3/5/10          | recall        | ~39–42 ms | ~42–46 ms | 5 each |
| T8 Batch per-item (size 1/5/10/20) | batch_capture | ~50 ms    | ~50 ms    | 3 each |
| T9 Vault health check              | vault_status  | 0.6 ms    | 0.8 ms    | 8      |

**capture 병목**: embedding (42–70 ms) + insert (52–78 ms) 가 전체의 ~94%를 차지.  
**recall 병목**: embedding (29–32 ms) + score (8–11 ms) 가 전체의 ~100%.  
**batch 선형성 확인**: item당 ~50 ms 일정 (batch size 1→20 전 구간).

---

## 주목할 점

### vault_topk / remind 가 0 ms 로 측정됨

`capture` 및 `recall` 전 시나리오에서 `vault_topk = 0.0 ms`, `remind = 0.0 ms`.  
이는 `envector_client.score()` 가 반환하는 `encrypted_blobs` 리스트가 비어 있어  
Vault TopK 복호화 분기가 실행되지 않았음을 의미한다.

**가능한 원인 (둘 중 하나 또는 복합):**

1. **envector 클러스터에 FHE eval key가 로드되지 않은 상태**  
   `insert`는 성공(52–78 ms)하지만 `index.scoring()` 이 빈 CipherBlock 리스트를 반환.  
   → envector-msa-1.4.0 에서 `load_key` / `load_index` 가 완료된 상태인지 확인 필요.

2. **`query_encryption = "plain"` 모드**  
   pyenvector 1.4.0 에서 plain 쿼리는 FHE ciphertext blob 대신 다른 경로로 결과를 처리할 수 있음.  
   → `query_encryption = "cipher"` 로 전환 후 재측정 필요.

**결론**: embedding / FHE insert / network score call 의 latency 는 정확하게 측정되었으며,  
vault_topk → remind 경로는 현재 환경에서 비활성 상태. **eval key 로드 후 재측정 권장**.

### T3 한국어 embedding 이 영문 대비 ~65% 느림

T1(42 ms) → T3(69 ms): 동일 모델(Qwen3-Embedding-0.6B)에서 한국어 텍스트가 상당히 느림.  
토큰 수가 비슷함에도 차이가 크므로 tokenizer subword 분절 수 차이로 추정.

### T4 중복 입력의 p95 가 p50 대비 2배 (106 → 201 ms)

`score` 단계에서 occasional spike (9 ms → 72 ms) 가 발생.  
중복 경로 자체의 오버헤드가 아닌 네트워크 지터로 보임. 재측정 시 확인 권장.

---

## Environment

- **date**: 2026-05-04
- **envector_endpoint**: 146.56.178.130:50050
- **vault_endpoint**: tcp://localhost:50051
- **embedding_model**: Qwen/Qwen3-Embedding-0.6B
- **embedding_mode**: sbert
- **index_name**: runecontext
- **key_id**: vault-key
- **network_rtt**: unknown
- **runs_per_scenario**: 8
- **warmup_runs**: 2

## Feature: `capture`

### T1_short_en

- label: short English (~30 tokens)
- tokens_approx: 35
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 42.1   | 42.7   | 42.7   | 42.1    |
| score      | 8   | 9.0    | 10.0   | 10.2   | 8.8     |
| vault_topk | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| insert     | 8   | 51.7   | 54.0   | 54.5   | 51.8    |
| total      | 8   | 102.8  | 104.8  | 105.1  | 102.7   |

### T2_long_en

- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 52.8   | 53.5   | 53.6   | 53.0    |
| score      | 8   | 8.5    | 10.8   | 11.4   | 8.7     |
| vault_topk | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| insert     | 8   | 62.0   | 63.6   | 63.6   | 61.8    |
| total      | 8   | 123.2  | 126.8  | 127.5  | 123.5   |

### T3_korean

- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 69.2   | 69.7   | 69.7   | 69.1    |
| score      | 8   | 8.8    | 10.1   | 10.2   | 8.8     |
| vault_topk | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| insert     | 8   | 77.6   | 86.3   | 89.3   | 79.1    |
| total      | 8   | 155.0  | 165.3  | 168.6  | 157.0   |

### T4_duplicate

- label: duplicate input — tests novelty near-duplicate path
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 42.3   | 43.3   | 43.6   | 42.4    |
| score      | 8   | 9.5    | 72.8   | 74.4   | 25.5    |
| vault_topk | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| insert     | 8   | 54.6   | 87.5   | 91.6   | 61.8    |
| total      | 8   | 106.8  | 201.2  | 203.3  | 129.7   |

## Feature: `recall`

### T5_exact_match

- label: exact match query
- topk: 5
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 28.7   | 29.8   | 30.0   | 28.7    |
| score      | 8   | 8.7    | 11.1   | 11.5   | 9.1     |
| vault_topk | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| remind     | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| total      | 8   | 37.3   | 40.4   | 41.0   | 37.8    |

### T6_cross_lang

- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 30.1   | 30.5   | 30.6   | 30.0    |
| score      | 8   | 8.3    | 9.5    | 9.6    | 8.3     |
| vault_topk | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| remind     | 8   | 0.0    | 0.0    | 0.0    | 0.0     |
| total      | 8   | 38.4   | 39.3   | 39.4   | 38.3    |

### T7_topk_1

- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 31.6   | 33.4   | 33.4   | 31.7    |
| score      | 5   | 8.1    | 12.2   | 12.6   | 9.2     |
| vault_topk | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| remind     | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| total      | 5   | 39.8   | 45.2   | 46.0   | 40.8    |

### T7_topk_3

- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 30.3   | 30.6   | 30.6   | 29.6    |
| score      | 5   | 11.0   | 15.2   | 15.3   | 11.8    |
| vault_topk | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| remind     | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| total      | 5   | 41.7   | 45.5   | 45.6   | 41.4    |

### T7_topk_5

- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 29.2   | 29.7   | 29.7   | 29.1    |
| score      | 5   | 9.8    | 13.2   | 13.7   | 10.1    |
| vault_topk | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| remind     | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| total      | 5   | 39.3   | 42.3   | 42.9   | 39.2    |

### T7_topk_10

- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 28.6   | 30.2   | 30.4   | 28.8    |
| score      | 5   | 8.5    | 11.1   | 11.3   | 9.3     |
| vault_topk | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| remind     | 5   | 0.0    | 0.0    | 0.0    | 0.0     |
| total      | 5   | 38.9   | 39.5   | 39.6   | 38.1    |

## Feature: `batch_capture`

### T8_batch_1

- batch_size: 1
- label: batch size 1 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 49.9   | 50.1   | 50.2   | 49.4    |
| per_item    | 3   | 49.9   | 50.1   | 50.2   | 49.4    |

### T8_batch_5

- batch_size: 5
- label: batch size 5 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 248.9  | 249.3  | 249.4  | 248.4   |
| per_item    | 3   | 49.8   | 49.9   | 49.9   | 49.7    |

### T8_batch_10

- batch_size: 10
- label: batch size 10 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 499.2  | 499.8  | 499.8  | 499.2   |
| per_item    | 3   | 49.9   | 50.0   | 50.0   | 49.9    |

### T8_batch_20

- batch_size: 20
- label: batch size 20 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 1018.2 | 1019.0 | 1019.0 | 1016.5  |
| per_item    | 3   | 50.9   | 50.9   | 51.0   | 50.8    |

## Feature: `vault_status`

### T9_vault_status

- label: Vault gRPC health check
- runs: 8

| Phase              | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ------------------ | --- | ------ | ------ | ------ | ------- |
| vault_health_check | 8   | 0.6    | 0.8    | 0.8    | 0.7     |
