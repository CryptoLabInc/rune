# Rune × envector-msa-1.4.0 Latency Report

> **[측정 환경]** envector 연결은 `secure=false`(plaintext gRPC)로 측정. Vault(`localhost:50051`)는 TLS 활성 상태.
> plaintext로 진행해도 무방하다고 판단한 이유: gRPC는 HTTP/2 커넥션을 재사용하므로 TLS 핸드셰이크 비용이 warmup 이후 측정값에 포함되지 않으며, 주요 병목이 FHE 연산에 있어 TLS 유무가 결과에 실질적 영향이 낮을 수 있다고 판단.
> 이후 envector TLS 환경(`secure=true`)에서도 동일 시나리오로 벤치마크를 진행할 예정.
>
> **[2026-05-05 재측정 노트]** 2026-05-04 리포트에서 `vault_topk = 0 ms`로 측정되어 FHE 복호화 분기가 비활성으로 의심되던 부분이 해결되었음. 이번 측정은 eval key가 정상 로드된 상태에서 **end-to-end FHE 경로(novelty check + topk decrypt)** 를 모두 거치는 결과이며, 어제 측정 대비 capture는 ~50배, recall은 ~10배 증가한 실제 FHE 연산 비용을 반영함.

## 실행 결과 요약

| 시나리오                           | feature       | p50 total   | p95 total   | n      | Δ(2026-05-04 대비) |
| ---------------------------------- | ------------- | ----------- | ----------- | ------ | ------------------ |
| T1 짧은 영문 (~35 tokens)          | capture       | 5269.5 ms   | 5449.8 ms   | 8      | +5167 ms (×51)     |
| T2 긴 영문 (~155 tokens)           | capture       | 4707.0 ms   | 4931.9 ms   | 8      | +4584 ms (×38)     |
| T3 한국어                          | capture       | 4595.2 ms   | 4789.1 ms   | 8      | +4440 ms (×30)     |
| T4 중복 입력 (near-duplicate path) | capture       | 4886.1 ms   | 5020.2 ms   | 8      | +4779 ms (×46)     |
| T5 Recall — exact match            | recall        | 368.2 ms    | 385.3 ms    | 8      | +331 ms (×9.9)     |
| T6 Recall — 한→영 cross-language   | recall        | 384.1 ms    | 464.3 ms    | 8      | +346 ms (×10.0)    |
| T7 Recall — topk 1/3/5/10          | recall        | ~367–376 ms | ~374–402 ms | 5 each | ×9.2–×9.6          |
| T8 Batch per-item (size 1/5/10/20) | batch_capture | ~238–245 ms | ~242–246 ms | 3 each | ×4.8               |
| T9 Vault health check              | vault_status  | 0.8 ms      | 0.9 ms      | 8      | ≈ 동일             |

**capture 병목**: `insert` (4200–5000 ms, FHE encrypt + 원격 insert) 가 전체의 **~92%**를 차지. embed/score/vault_topk 모두 합쳐도 5–10% 수준.
**recall 병목**: `score` (180–200 ms) + `vault_topk` (100–115 ms) + `remind` (45–47 ms) 의 FHE 경로가 전체의 **~80%**, 임베딩(~40 ms)은 부수적.
**batch 선형성**: per-item ~240 ms로 batch size 1→20 전 구간 선형 (FHE insert 미수행, embed+score만 측정하는 path).
**recall topk 무관**: T7에서 topk 1→10에도 total p50 367–376 ms로 거의 동일. FHE inner_product cost가 index 크기 의존적이라 topk 변화 영향 없음.

---

## 주목할 점

### vault_topk가 정상 측정됨 — 어제 0 ms 문제 해결

어제 리포트에서 모든 시나리오의 `vault_topk = 0.0 ms`였던 것이 오늘은 capture 31–100 ms / recall 100–115 ms로 정상 측정됨. eval key 로드 상태가 정상화되어 FHE encrypted_blobs가 비어 있지 않게 되었고, Vault gRPC 복호화 분기가 실제로 실행됨.

**부수적으로 확인된 인프라 변경**: `mcp/adapter/vault_client.py:33`의 `MAX_MESSAGE_LENGTH`가 256 MB → ~1.95 GB로 상향되어야 했음. EvalKey 응답 크기가 약 1.18 GB에 달해 256 MB 한도에서 `RESOURCE_EXHAUSTED`로 실패하던 것을 동일 브랜치 내 plugin cache 버전과 일치시켜 해결.

### capture 시간 대부분이 insert에서 소비됨 (~4.5초, 전체의 92%)

`insert` phase가 capture 전체 시간의 92%. 내부적으로 FHE 암호화 + 원격 envector insert가 합쳐진 구간이며, 절대값은 4.2–5.0초 수준. (참고: envector-msa-1.4.0의 PPMM cache 최적화는 Search 경로 전용이며 insert와 무관 — `envector-msa-1.4.0/docs/design/ppmm-cache-optimization-analysis.md`.) 다음 측정 라운드에서:
1. FHE encrypt 단독 시간 (로컬 CPU)
2. 원격 envector `insert_data` gRPC 시간 (네트워크 + 서버측 저장)
을 분리 계측하면 병목 위치가 더 명확해질 것으로 보임.

### recall score 단계가 capture novelty score보다 ~2배 무거움

같은 `score` 호출인데도 capture 시 100 ms vs recall 시 180 ms. capture는 novelty check만 하고 단순 거리 metric을 받는 반면, recall은 그 결과를 가지고 후속 vault_topk + remind 분기로 연결되며 ScoreEntry payload가 달라지는 것으로 추정. 코드 경로 확인 필요.

### T3 한국어 capture가 영문보다 score/vault_topk만 느림

|           | embed        | score        | vault_topk   | insert        |
| --------- | ------------ | ------------ | ------------ | ------------- |
| T1 영문   | 92.6         | 100.4        | 31.9         | 5018.1        |
| T3 한국어 | 126.9 (+37%) | 167.5 (+67%) | 75.6 (+137%) | 4209.0 (-16%) |

embed/score/vault_topk는 한국어가 모두 느리지만 insert는 오히려 더 빠름. 토큰 차이로 embed가 느린 것은 어제와 동일하나, score/vault_topk가 토큰 수에 영향받는 이유는 명확하지 않음 (이론적으로 FHE 벡터 길이가 동일해야 함). 토크나이저 출력의 vector dim이 다르거나, payload 길이 차이가 직렬화 오버헤드로 나타났을 가능성.

### T4 중복 입력의 score가 다른 capture 시나리오의 2배

T1 score 100 ms vs T4 score 211 ms. T4는 같은 텍스트를 두 번 capture하는 시나리오로, novelty 결과가 high-similarity로 나오면서 vault_topk decrypt 분기가 더 무거운 path를 타는 것으로 보임. 어제 리포트의 "p95 spike 발생" 도 같은 원인으로 추정.

### batch_capture가 단일 capture의 ~5% 시간

per-item 240 ms vs 단일 capture 5000 ms. 차이의 대부분은 batch_capture가 **insert를 수행하지 않고** embed + score(novelty)만 측정하기 때문. batch insert를 포함한 진짜 batch 시간이 필요하면 runner에 별도 시나리오 추가 필요.

---

## Environment

- **date**: 2026-05-05
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
| embed      | 8   | 92.6   | 120.6  | 122.0  | 98.9    |
| score      | 8   | 100.4  | 115.0  | 116.4  | 98.5    |
| vault_topk | 8   | 31.9   | 47.3   | 49.4   | 33.6    |
| insert     | 8   | 5018.1 | 5242.2 | 5287.3 | 4951.1  |
| total      | 8   | 5269.5 | 5449.8 | 5486.2 | 5182.1  |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 100.6  | 139.3  | 144.6  | 108.0   |
| score      | 8   | 135.7  | 146.0  | 147.1  | 134.3   |
| vault_topk | 8   | 56.0   | 64.4   | 64.7   | 54.9    |
| insert     | 8   | 4406.8 | 4645.0 | 4695.4 | 4409.3  |
| total      | 8   | 4707.0 | 4931.9 | 4976.3 | 4706.7  |

### T3_korean
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 126.9  | 147.3  | 149.4  | 128.3   |
| score      | 8   | 167.5  | 184.9  | 187.2  | 168.7   |
| vault_topk | 8   | 75.6   | 91.1   | 92.6   | 76.4    |
| insert     | 8   | 4209.0 | 4397.0 | 4405.8 | 4246.7  |
| total      | 8   | 4595.2 | 4789.1 | 4806.0 | 4620.1  |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 115.2  | 132.1  | 132.4  | 113.4   |
| score      | 8   | 211.4  | 229.7  | 231.1  | 209.3   |
| vault_topk | 8   | 99.7   | 108.9  | 109.6  | 99.7    |
| insert     | 8   | 4435.8 | 4568.8 | 4572.0 | 4446.1  |
| total      | 8   | 4886.1 | 5020.2 | 5021.7 | 4868.4  |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 40.4   | 41.2   | 41.3   | 39.7    |
| score      | 8   | 182.7  | 200.0  | 204.1  | 185.3   |
| vault_topk | 8   | 98.5   | 105.7  | 106.0  | 99.7    |
| remind     | 8   | 45.9   | 49.7   | 50.6   | 46.5    |
| total      | 8   | 368.2  | 385.3  | 386.0  | 371.2   |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 8   | 41.4   | 47.1   | 47.3   | 42.6    |
| score      | 8   | 195.4  | 217.7  | 221.0  | 197.5   |
| vault_topk | 8   | 104.6  | 157.1  | 177.1  | 114.1   |
| remind     | 8   | 47.0   | 53.2   | 53.8   | 47.4    |
| total      | 8   | 384.1  | 464.3  | 483.0  | 401.5   |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 37.4   | 42.6   | 43.6   | 37.7    |
| score      | 5   | 186.7  | 191.7  | 192.0  | 186.5   |
| vault_topk | 5   | 101.2  | 103.2  | 103.6  | 100.7   |
| remind     | 5   | 43.5   | 45.1   | 45.3   | 43.0    |
| total      | 5   | 367.7  | 375.1  | 375.3  | 367.9   |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 38.0   | 38.5   | 38.6   | 36.8    |
| score      | 5   | 181.0  | 191.1  | 191.5  | 182.7   |
| vault_topk | 5   | 105.4  | 109.0  | 109.2  | 104.4   |
| remind     | 5   | 45.0   | 45.8   | 45.8   | 44.7    |
| total      | 5   | 368.1  | 376.4  | 377.2  | 368.7   |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 36.5   | 38.3   | 38.3   | 36.8    |
| score      | 5   | 178.7  | 196.3  | 196.5  | 184.6   |
| vault_topk | 5   | 114.5  | 139.6  | 143.8  | 116.2   |
| remind     | 5   | 44.8   | 46.1   | 46.3   | 44.9    |
| total      | 5   | 376.4  | 402.2  | 403.8  | 382.4   |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase      | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | --- | ------ | ------ | ------ | ------- |
| embed      | 5   | 38.2   | 39.3   | 39.5   | 37.2    |
| score      | 5   | 181.9  | 190.2  | 190.6  | 183.8   |
| vault_topk | 5   | 101.8  | 106.0  | 106.4  | 102.6   |
| remind     | 5   | 46.2   | 47.2   | 47.3   | 46.0    |
| total      | 5   | 369.4  | 374.0  | 374.0  | 369.7   |


## Feature: `batch_capture`

### T8_batch_1
- batch_size: 1
- label: batch size 1 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 237.9  | 242.2  | 242.6  | 239.2   |
| per_item    | 3   | 237.9  | 242.2  | 242.6  | 239.2   |

### T8_batch_5
- batch_size: 5
- label: batch size 5 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 1207.8 | 1209.7 | 1209.9 | 1207.5  |
| per_item    | 3   | 241.6  | 241.9  | 242.0  | 241.5   |

### T8_batch_10
- batch_size: 10
- label: batch size 10 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 2409.5 | 2437.7 | 2440.2 | 2415.5  |
| per_item    | 3   | 240.9  | 243.8  | 244.0  | 241.5   |

### T8_batch_20
- batch_size: 20
- label: batch size 20 (embed+score per item)
- runs: 3

| Phase       | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | --- | ------ | ------ | ------ | ------- |
| total_batch | 3   | 4892.0 | 4907.7 | 4909.1 | 4846.8  |
| per_item    | 3   | 244.6  | 245.4  | 245.5  | 242.3   |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase              | n   | p50 ms | p95 ms | p99 ms | mean ms |
| ------------------ | --- | ------ | ------ | ------ | ------- |
| vault_health_check | 8   | 0.8    | 0.9    | 0.9    | 0.8     |
