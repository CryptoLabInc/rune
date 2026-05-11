# Rune × envector-msa-1.2.2 Latency Report

> **[측정 환경]** pyenvector 1.2.2 사용. `secure` 파라미터 미지원 — `access_token` 설정 시 TLS 자동 활성.
> Vault는 원격(`tcp://193.122.124.173:50051`)에서 TLS로 실행 중.
> envector 엔드포인트는 클라우드 도메인 기반(`0511-1401-0001-4r6cwxfc908b.clusters.envector.io`)으로 이전되어, 이전 측정(2026-05-05, `146.56.178.130:50050` IP 직접 접속)과 네트워크 경로도 다름.
> **eval_mode=rmp, index_type=flat** (1.4.0은 eval_mode=mm, index_type=flat).
>
> **[비교 노트]** 2026-05-05 리포트(pyenvector 1.4.0)와의 수치 차이는 **pyenvector 버전**, **eval_mode(rmp vs mm)**, **엔드포인트/인프라 변경** 세 가지 요인이 복합되어 있어 버전 단독 영향으로 해석하면 안 됨.

## 실행 결과 요약

| 시나리오                           | feature       | p50 total  | p95 total  | n      | Δ(2026-05-05 대비)     |
| ---------------------------------- | ------------- | ---------- | ---------- | ------ | ---------------------- |
| T1 짧은 영문 (~35 tokens)          | capture       | 664.0 ms   | 792.0 ms   | 8      | -4605 ms (÷7.9)        |
| T2 긴 영문 (~155 tokens)           | capture       | 809.8 ms   | 955.1 ms   | 8      | -3897 ms (÷5.8)        |
| T3 한국어                          | capture       | 870.5 ms   | 951.7 ms   | 8      | -3724 ms (÷5.3)        |
| T4 중복 입력 (near-duplicate path) | capture       | 964.7 ms   | 1013.1 ms  | 8      | -3921 ms (÷5.1)        |
| T5 Recall — exact match            | recall        | 528.6 ms   | 554.2 ms   | 8      | +160 ms (×1.4)         |
| T6 Recall — 한→영 cross-language   | recall        | 515.6 ms   | 571.2 ms   | 8      | +131 ms (×1.3)         |
| T7 Recall — topk 1/3/5/10          | recall        | ~472–509 ms| ~498–527 ms| 5 each | ×1.3                   |
| T8 Batch per-item (size 1/5/10/20) | batch_capture | ~294–309 ms| ~300–383 ms| 3 each | ×1.3                   |
| T9 Vault health check              | vault_status  | 7.6 ms     | 9.5 ms     | 8      | +6.8 ms (×9.5)         |

**capture 병목**: `insert` (490–550 ms)가 전체의 **~74%**를 차지. 1.4.0(92%)보다 비중은 낮아졌으나 여전히 지배적.
**recall 병목**: `score` (255–273 ms) + `vault_topk` (131–176 ms)의 FHE 경로가 전체의 **~84%**, 임베딩(~34 ms)은 부수적.
**batch 선형성**: per-item 294–309 ms로 batch size 1→20 전 구간 선형 (embed+score만 측정하는 path).
**recall topk 무관**: T7에서 topk 1→10에도 total p50 472–509 ms로 거의 동일. FHE inner_product cost가 topk 변화와 무관한 구조.

## 1.4.0 vs 1.2.2 p50 비교

> `(×N)` = 1.4.0이 1.2.2보다 N배 느림. `(÷N)` = 1.4.0이 1.2.2보다 N배 빠름.

```
── capture (p50 ms, 기준 5269ms = 40칸) ──────────────────────────────

T1 영문    1.4.0  ████████████████████████████████████████  5269ms  (×7.9)
           1.2.2  █████  664ms

T2 긴영문  1.4.0  ████████████████████████████████████  4707ms  (×5.8)
           1.2.2  ██████  810ms

T3 한국어  1.4.0  ███████████████████████████████████  4595ms  (×5.3)
           1.2.2  ███████  871ms

T4 중복    1.4.0  █████████████████████████████████████  4886ms  (×5.1)
           1.2.2  ████████  965ms

── recall (p50 ms, 기준 529ms = 30칸) ────────────────────────────────

T5 exact   1.4.0  █████████████████████  368ms  (÷1.4)
           1.2.2  ██████████████████████████████  529ms

T6 cross   1.4.0  ██████████████████████  384ms  (÷1.3)
           1.2.2  █████████████████████████████  516ms

── vault health (p50 ms, 기준 7.6ms = 20칸) ──────────────────────────

T9 vault   1.4.0  ██  0.8ms  (÷9.5)
           1.2.2  ████████████████████  7.6ms
```

---

## 주목할 점

### capture가 1.4.0 대비 ~5–8x 빠름 — insert phase 개선이 핵심

|           | embed ms | score ms | vault_topk ms | insert ms | total ms |
| --------- | -------- | -------- | ------------- | --------- | -------- |
| 1.4.0 T1  | 92.6     | 100.4    | 31.9          | 5018.1    | 5269.5   |
| 1.2.2 T1  | 38.8     | 102.0    | 34.6          | 490.0     | 664.0    |

`score`와 `vault_topk`는 거의 동일한 반면, `insert`가 5018ms → 490ms(÷10)로 압도적 개선. 1.4.0에서 EvalKey(~1.18 GB)를 매 insert마다 처리하는 비용이 컸던 것으로 추정. 1.2.2에는 해당 EvalKey 부담이 없음.
`embed`도 92ms → 39ms로 개선됐는데, warmup 이후 임베딩 모델 캐시 상태가 더 안정적이었을 수 있음.

### recall이 1.4.0 대비 ~1.4x 느려짐 — DNS+TLS 오버헤드 추가

|           | embed ms | score ms | vault_topk ms | remind ms | total ms |
| --------- | -------- | -------- | ------------- | --------- | --------- |
| 1.4.0 T5  | 40.4     | 182.7    | 98.5          | 45.9      | 368.2    |
| 1.2.2 T5  | 33.7     | 273.0    | 175.9         | 44.5      | 528.6    |

`score`(+90ms)와 `vault_topk`(+77ms)에서 주로 증가. 두 버전 간 차이는 pyenvector 버전 자체보다 **인프라 환경 변화**가 주 원인이다:

- **`vault_topk` +77ms**: Vault가 localhost → 원격(193.122.124.173)으로 변경된 직접적 결과. T9와 동일한 원인.
- **`score` +90ms**: envector 엔드포인트 변경(IP 직접 → 도메인 기반) + TLS 활성화(1.4.0은 plaintext, 1.2.2는 TLS) + eval_mode 차이(mm → rmp). 세 요인이 복합되어 있어 개별 기여도 분리는 불가.

이 수치들은 pyenvector 1.2.2 vs 1.4.0의 순수 알고리즘 성능 비교로 해석해선 안 된다.

### vault_health_check가 ~9.5x 느려짐 — Vault가 원격으로 변경

0.8ms(localhost) → 7.6ms(tcp://193.122.124.173:50051). Vault 엔드포인트가 로컬에서 원격으로 변경된 직접적 결과이며, 이는 pyenvector 버전과 무관하다.

### T4 중복 입력 score가 T1의 2.7x — near-duplicate path 동일 패턴

T1 score 102ms vs T4 score 279ms. 1.4.0에서도 동일하게 중복 입력 시 score phase가 무거운 패턴(T1: 100ms → T4: 211ms). near-duplicate 판정 경로에서 vault_topk decrypt 분기가 더 무거운 path를 타기 때문. 버전에 무관하게 일관된 패턴.

### T3 한국어 score/vault_topk가 영문보다 2x 무거움

|           | embed ms | score ms | vault_topk ms | insert ms |
| --------- | -------- | -------- | ------------- | --------- |
| T1 영문   | 38.8     | 102.0    | 34.6          | 490.0     |
| T3 한국어 | 49.4     | 210.2    | 101.1         | 516.5     |

embed는 소폭 증가(+27%), score/vault_topk는 2배 증가. 1.4.0 측정(T1: 100ms → T3: 167ms)과 동일한 경향이나 배율이 커졌다. 토크나이저 출력의 FHE vector 직렬화 페이로드 차이로 인한 것으로 추정.

### batch_capture per-item이 단일 capture의 ~44%

per-item ~300ms vs 단일 capture ~664ms. batch_capture는 **insert를 수행하지 않고** embed+score만 측정하므로 차이가 발생. 이 점은 1.4.0(per-item ~240ms vs total ~5000ms)과 구조적으로 동일.

---

## Environment

- **date**: 2026-05-11
- **envector_endpoint**: 0511-1401-0001-4r6cwxfc908b.clusters.envector.io
- **vault_endpoint**: tcp://193.122.124.173:50051
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

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 8 | 38.8   | 43.4   | 43.7   | 39.2    |
| score      | 8 | 102.0  | 124.4  | 131.5  | 104.0   |
| vault_topk | 8 | 34.6   | 42.5   | 44.4   | 35.7    |
| insert     | 8 | 490.0  | 590.2  | 597.2  | 514.5   |
| total      | 8 | 664.0  | 792.0  | 795.2  | 693.4   |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 8 | 42.9   | 45.5   | 45.6   | 43.0    |
| score      | 8 | 150.6  | 163.0  | 164.9  | 148.4   |
| vault_topk | 8 | 66.9   | 73.7   | 74.5   | 66.8    |
| insert     | 8 | 550.5  | 700.8  | 708.9  | 594.0   |
| total      | 8 | 809.8  | 955.1  | 972.0  | 852.2   |

### T3_korean
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 8 | 49.4   | 50.8   | 51.1   | 48.6    |
| score      | 8 | 210.2  | 236.6  | 237.9  | 208.7   |
| vault_topk | 8 | 101.1  | 122.4  | 125.9  | 102.6   |
| insert     | 8 | 516.5  | 543.4  | 550.4  | 516.7   |
| total      | 8 | 870.5  | 951.7  | 964.2  | 876.6   |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- runs: 8

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 8 | 38.9   | 43.4   | 43.6   | 39.5    |
| score      | 8 | 278.6  | 301.0  | 307.2  | 275.6   |
| vault_topk | 8 | 143.6  | 164.0  | 166.5  | 146.1   |
| insert     | 8 | 507.2  | 528.2  | 528.6  | 507.6   |
| total      | 8 | 964.7  | 1013.1 | 1018.7 | 969.0   |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 8 | 33.7   | 34.8   | 35.0   | 33.1    |
| score      | 8 | 273.0  | 288.7  | 289.9  | 275.5   |
| vault_topk | 8 | 175.9  | 193.8  | 195.1  | 173.8   |
| remind     | 8 | 44.5   | 47.7   | 48.2   | 44.3    |
| total      | 8 | 528.6  | 554.2  | 561.4  | 526.8   |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 8 | 33.5   | 35.2   | 35.7   | 32.7    |
| score      | 8 | 263.4  | 275.9  | 276.2  | 262.1   |
| vault_topk | 8 | 174.0  | 225.6  | 245.6  | 183.7   |
| remind     | 8 | 43.7   | 60.4   | 67.2   | 46.5    |
| total      | 8 | 515.6  | 571.2  | 579.5  | 525.0   |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 5 | 33.4   | 34.6   | 34.8   | 32.7    |
| score      | 5 | 265.8  | 282.9  | 286.2  | 264.7   |
| vault_topk | 5 | 167.8  | 172.3  | 173.0  | 167.3   |
| remind     | 5 | 44.5   | 46.0   | 46.1   | 44.1    |
| total      | 5 | 509.0  | 527.0  | 529.7  | 508.8   |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 5 | 34.1   | 34.8   | 34.9   | 32.6    |
| score      | 5 | 270.3  | 275.5  | 276.1  | 268.7   |
| vault_topk | 5 | 151.9  | 158.2  | 158.6  | 150.4   |
| remind     | 5 | 46.3   | 48.8   | 49.2   | 46.4    |
| total      | 5 | 501.0  | 511.4  | 513.1  | 498.1   |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 5 | 35.0   | 36.0   | 36.1   | 34.8    |
| score      | 5 | 269.5  | 278.0  | 279.3  | 266.0   |
| vault_topk | 5 | 131.0  | 139.9  | 140.5  | 134.1   |
| remind     | 5 | 45.8   | 47.8   | 47.9   | 45.9    |
| total      | 5 | 484.5  | 497.8  | 499.6  | 480.8   |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase      | n | p50 ms | p95 ms | p99 ms | mean ms |
| ---------- | - | ------ | ------ | ------ | ------- |
| embed      | 5 | 33.0   | 34.4   | 34.5   | 32.1    |
| score      | 5 | 254.9  | 261.2  | 261.7  | 255.7   |
| vault_topk | 5 | 134.0  | 166.6  | 171.4  | 139.8   |
| remind     | 5 | 49.4   | 50.5   | 50.6   | 48.9    |
| total      | 5 | 471.8  | 501.9  | 507.1  | 476.5   |


## Feature: `batch_capture`

### T8_batch_1
- batch_size: 1
- label: batch size 1 (embed+score per item)
- runs: 3

| Phase       | n | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | - | ------ | ------ | ------ | ------- |
| total_batch | 3 | 294.3  | 382.7  | 390.5  | 323.4   |
| per_item    | 3 | 294.3  | 382.7  | 390.5  | 323.4   |

### T8_batch_5
- batch_size: 5
- label: batch size 5 (embed+score per item)
- runs: 3

| Phase       | n | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | - | ------ | ------ | ------ | ------- |
| total_batch | 3 | 1500.1 | 1501.8 | 1502.0 | 1500.3  |
| per_item    | 3 | 300.0  | 300.4  | 300.4  | 300.1   |

### T8_batch_10
- batch_size: 10
- label: batch size 10 (embed+score per item)
- runs: 3

| Phase       | n | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | - | ------ | ------ | ------ | ------- |
| total_batch | 3 | 2996.3 | 3120.6 | 3131.6 | 3036.4  |
| per_item    | 3 | 299.6  | 312.1  | 313.2  | 303.6   |

### T8_batch_20
- batch_size: 20
- label: batch size 20 (embed+score per item)
- runs: 3

| Phase       | n | p50 ms | p95 ms | p99 ms | mean ms |
| ----------- | - | ------ | ------ | ------ | ------- |
| total_batch | 3 | 6182.3 | 6190.3 | 6191.0 | 6169.9  |
| per_item    | 3 | 309.1  | 309.5  | 309.5  | 308.5   |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase              | n | p50 ms | p95 ms | p99 ms | mean ms |
| ------------------ | - | ------ | ------ | ------ | ------- |
| vault_health_check | 8 | 7.6    | 9.5    | 9.7    | 8.0     |
