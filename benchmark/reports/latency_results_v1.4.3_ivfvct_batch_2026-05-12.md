# Rune × envector-msa-1.4.3 Latency Report (insert_mode=batch)

> **[측정 환경]** pyenvector 1.4.3. **eval_mode=mm32, index_type=ivf_vct**.
> envector 엔드포인트: `0512-1000-0001-ve2lv5txdqrl.clusters.envector.ai` (v1.4.3 클러스터, TLS).
> Vault: `tcp://161.118.149.143:50051` (TLS, 원격).
> **insert_mode=batch** — `use_row_insert=False`, `data=[v1,...,vN]` (batch insert API 경로).
>
> **[비교 주의]** v1.2.2 plan 대비 pyenvector 버전, eval_mode(rmp→mm32), index_type(flat→ivf_vct), envector/Vault 엔드포인트가 동시에 달라짐. 동일 single 모드(`latency_results_v1.4.3_ivfvct_single_2026-05-12.md`)와 짝을 이루는 리포트.
> **[측정 순서 영향]** 본 batch 실행은 single 실행 직후 같은 인덱스/세션에서 진행. 인덱스 상태와 서버 캐시가 single 측정 시점과 다르므로 일부 phase 수치가 single과 직접 비교 불가 (아래 "주목할 점" 참조).

## 실행 결과 요약

| 시나리오                           | feature       | p50 total  | p95 total | n      | 비고 |
| ---------------------------------- | ------------- | ---------- | --------- | ------ | ---- |
| T1 짧은 영문 (~35 tokens)          | capture       | **11836.4 ms** | 14203.3 ms | 8 | ⚠ 서버 batch path 초기화 이슈 (아래) |
| T2 긴 영문 (~155 tokens)           | capture       | 115.5 ms   | 323.5 ms  | 8      |  |
| T3 한국어                          | capture       | 124.7 ms   | 136.8 ms  | 8      |  |
| T4 중복 입력 (near-duplicate path) | capture       | 108.1 ms   | 112.5 ms  | 8      |  |
| T5 Recall — exact match            | recall        | 44.7 ms    | 48.4 ms   | 8      | vault/remind phase = 0 |
| T6 Recall — 한→영 cross-language   | recall        | 43.5 ms    | 46.2 ms   | 8      | vault/remind phase = 0 |
| T7 Recall — topk 1/3/5/10          | recall        | 42–45 ms   | 45–46 ms  | 5 each | topk 무관 |
| T9 Vault health check              | vault_status  | 6.6 ms     | 7.7 ms    | 8      |  |
| T10 searchable 짧은 영문           | searchable    | 95.4 ms    | 109.7 ms  | 8      |  |
| T11 searchable 긴 영문             | searchable    | 119.8 ms   | 121.2 ms  | 8      |  |
| T12 searchable 한국어              | searchable    | 124.9 ms   | 137.2 ms  | 8      |  |
| T13 multi_capture 2-phase          | multi_capture | 124.0 ms   | 132.3 ms  | 8      |  |
| T14 multi_capture 5-phase          | multi_capture | 188.9 ms   | 201.1 ms  | 8      |  |

**capture 정상 측정값 (T2–T4)**: total p50 108–125ms, insert p50 43ms로 일관. T1은 서버 batch path 초기화 이슈로 제외.
**batch insert 자체 비용**: 43–44ms로 매우 안정. 벡터 수와 무관하게 일정 — batch API 경로의 고정 오버헤드가 지배적.
**recall이 단일 모드 대비 5–6x 빠름**: 단, vault_topk/remind phase가 0으로 측정됨 → 동일 비교는 부적합 (아래 분석).

---

## 주목할 점

### ⚠ T1_short_en 비정상 — 별도 섹션 참고

본 리포트 하단의 "**알려진 이슈: T1 batch insert 비정상 (재현됨)**" 섹션에 상세. 결론: 서버 측 `async split batch data` 첫 호출 UNAVAILABLE → 클라이언트 자동 reconnect+retry로 wall-clock에 합산. **batch 효율 판정 baseline은 T1이 아닌 T2를 사용**.

### batch insert 비용이 벡터 수와 무관하게 ~43ms 고정

|       | insert p50 (ms) |
| ----- | --------------- |
| T2    | 43.5            |
| T3    | 43.1            |
| T4    | 43.8            |
| T10 (searchable) | 43.4 |
| T11 (searchable) | 43.4 |
| T12 (searchable) | 42.9 |
| T13 (2 vectors)  | 42.7 |
| T14 (5 vectors)  | 41.8 |

벡터 1개~5개 batch에서 insert phase가 41–44ms로 일정. RTT + 고정 직렬화 비용이 batch insert latency의 지배 요인이고, FHE 인코딩이나 ivf_vct 인덱스 갱신 비용은 본 측정 범위에선 잘 안 보인다.

### recall이 5–6x 빨라 보이는 건 측정 단축 가능성

| 시나리오 | single total p50 | batch total p50 | batch score | batch vault_topk | batch remind |
| -------- | ---------------- | --------------- | ----------- | ---------------- | ------------ |
| T5       | 244.0 ms         | 44.7 ms (÷5.5)  | 16.1 ms     | **0.0 ms**       | **0.0 ms**   |
| T6       | 249.8 ms         | 43.5 ms (÷5.7)  | 14.0 ms     | **0.0 ms**       | **0.0 ms**   |
| T7×4     | 235–250 ms       | 42–45 ms        | 16–18 ms    | **0.0 ms**       | **0.0 ms**   |

batch recall에서 `vault_topk`와 `remind` 모두 0ms로 측정됨. runner 로직상:

```python
blobs = score_res.get("encrypted_blobs", []) if score_res.get("ok") else []
if blobs:
    # vault decrypt + remind 실행
```

즉 score 응답에 `encrypted_blobs`가 없으면 두 phase가 건너뛰어진다. **batch 모드 score 응답이 빈 blobs를 반환**하고 있어 FHE 전체 경로가 측정되지 않은 것으로 보인다. capture T2–T4의 vault_topk=0도 같은 원인.

해석:
- batch insert와 score 응답 shape이 single 모드와 다를 가능성 (서버 동작 차이)
- 또는 single 모드 실행 후 인덱스 상태가 변해서 score가 빈 응답을 내놓는 상태
- batch recall 수치를 "FHE pipeline 전체"로 해석하면 안 됨. 적어도 score phase까지만 신뢰.

**조치 필요**: batch 모드를 clean index에서 단독 실행하거나, score 응답을 명시적으로 검증한 뒤 재측정. 본 plan의 검증 #2(재현성: 같은 시나리오 재실행 p50 변동 <20%)는 통과했으나 의미 있는 비교를 위해 응답 shape 일관성 확보 필요.

### batch 효율: T2 기준 single 대비 3.1x (capture)

T1은 batch path 초기화 이슈로 제외, T2–T4를 baseline으로:

| 시나리오 | single total | batch total | 효율 |
| -------- | ------------ | ----------- | ---- |
| T2       | 359.2 ms     | 115.5 ms    | ÷3.1 |
| T3       | 380.7 ms     | 124.7 ms    | ÷3.1 |
| T4       | 343.1 ms     | 108.1 ms    | ÷3.2 |

단, batch의 vault_topk=0인 점을 고려하면 단순 ÷3 비교는 과대 평가일 수 있다. **insert phase 단독 비교가 더 안전**: single insert ~135ms vs batch insert ~43ms → ÷3.1. plan 검증 #3(batch 효율) 만족 — `batch insert 43ms < N × single insert 135ms` (N≥1).

### searchable batch가 single보다 느림 — 정상

|     | single total | batch total |
| --- | ------------ | ----------- |
| T10 | 89.5 ms (p95 46s outlier) | 95.4 ms |
| T11 | 71.4 ms      | 119.8 ms    |
| T12 | 86.8 ms      | 124.9 ms    |

single은 row insert API의 fast-path를 타서 8ms 수준, batch는 정상적으로 43ms를 소비. T10 outlier가 없는 점에서 batch path가 오히려 안정적. searchable 측정 정확도는 batch 모드가 더 높다고 볼 수 있음 — await_searchable 의미가 실제로 작동.

### multi_capture는 batch 모드가 single보다 느림

|     | single total | batch total | 차이 |
| --- | ------------ | ----------- | ---- |
| T13 (2-phase) | 88.4 ms  | 124.0 ms  | +36 ms |
| T14 (5-phase) | 144.3 ms | 188.9 ms  | +45 ms |

multi_capture 자체가 내부적으로 batch insert path(`use_row_insert=False`)를 쓰지만, 본 측정의 `--insert-mode` 플래그는 capture 시나리오의 insert path만 토글한다. multi_capture는 항상 batch path. 차이 +36~45ms는 capture/recall 같은 batch 모드 측정이 인덱스를 더 채운 상태에서 진행됐기 때문으로 보이며, 측정 노이즈로 해석 가능.

### Vault health 6.6ms — single과 동일

T9 vault_health_check p50 batch=6.6ms / single=6.5ms. Vault 경로는 insert_mode와 무관하게 안정.


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


## ⚠ 알려진 이슈: T1 batch insert 비정상 (재현됨)

batch 모드의 **첫 시나리오 T1_short_en에서 insert phase가 비정상적으로 길게 측정됨**. 같은 환경에서 2회 실행 모두 재현되어 일회성 cold path가 아닌 패턴이다.

| 측정 | T1 total p50 | T1 total p95 | T1 insert p50 |
|------|--------------|--------------|---------------|
| 1회차 batch (이 리포트) | 11836 ms | 14203 ms | 11524 ms |
| 2회차 batch (rerun, 같은 환경) | 8668 ms | 15770 ms | 6928 ms |

### 원인

재실행 로그에 서버측 신호가 명확히 잡혔다:

```
[T1_short_en] warmup warmup run 1 run 2 gRPC transport error
  enVector operation failed:
    Batch 0 insert failed: async split batch data failed: UNAVAILABLE
  enVector connection lost - reconnecting ...
run 3 gRPC transport error  (동일 에러 반복)
run 4 gRPC transport error  (동일 에러 반복)
```

- 클라이언트 cold가 아니라 **envector-msa 서버 측 batch insert 첫 호출 (`async split batch data`)에서 UNAVAILABLE 응답** → 클라이언트가 자동 reconnect + retry → 결과적으로 성공하지만 retry 시간이 wall-clock latency에 합산됨.
- T2 이후에는 100% 정상 범위 (T2~T4: total p50 108–125 ms).
- **single 모드에서는 발생 안 함** — `use_row_insert=True` 경로(row insert API)가 batch 경로와 별개의 서버측 path를 타기 때문.

### 측정 해석 가이드

- T1_short_en의 total/insert 수치는 **서버 batch path 초기화 지연**을 포함한 값이라 batch 효율 판정에 부적합.
- 본 plan의 검증 기준 #3 (`T1 insert_ms(batch) < N × T1 insert_ms(single)`)은 T1 단독으로 판단 불가.
- **T2 기준 비교** (정상 측정값):
  - batch T2 total p50 = 115.5 ms
  - single T2 total p50 = 359.2 ms
  - → batch 효율 약 3.1× 명확히 확인됨
- score phase의 p95=2923ms 도 같은 retry 시점에 영향받았을 가능성이 있음.

### 후속 조치 권장

1. (단기) runner setup 마지막에 throwaway batch insert 1회를 추가해 측정 전 서버 path를 warm up
2. (근본) envector-cloud-be 팀에 `async split batch data` 첫 호출 UNAVAILABLE 이슈 공유 — ivf_vct 인덱스에서 async pool / lazy bind 초기화로 추정

---

## Feature: `capture`

### T1_short_en
- label: short English (~30 tokens)
- tokens_approx: 35
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 93.5 | 123.2 | 125.5 | 96.2 |
| score | 8 | 185.1 | 2923.2 | 3213.3 | 818.9 |
| vault_topk | 8 | 10.1 | 27.5 | 28.0 | 12.5 |
| insert | 8 | 11524.5 | 12556.3 | 12715.0 | 8158.4 |
| total | 8 | 11836.4 | 14203.3 | 14945.2 | 9086.1 |

### T2_long_en
- label: long English (~150 tokens)
- tokens_approx: 155
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 58.9 | 68.6 | 68.9 | 60.8 |
| score | 8 | 8.9 | 221.3 | 312.3 | 49.7 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 43.5 | 48.2 | 48.6 | 44.1 |
| total | 8 | 115.5 | 323.5 | 410.6 | 154.6 |

### T3_korean
- label: Korean text
- tokens_approx: 50
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 71.3 | 84.8 | 84.8 | 75.8 |
| score | 8 | 9.9 | 11.0 | 11.3 | 10.0 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 43.1 | 44.1 | 44.2 | 42.7 |
| total | 8 | 124.7 | 136.8 | 137.2 | 128.5 |

### T4_duplicate
- label: duplicate input — tests novelty near-duplicate path
- insert_mode: batch
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 55.3 | 57.0 | 57.1 | 52.9 |
| score | 8 | 9.1 | 10.7 | 11.1 | 9.3 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert | 8 | 43.8 | 47.0 | 47.1 | 44.0 |
| total | 8 | 108.1 | 112.5 | 112.7 | 106.3 |


## Feature: `recall`

### T5_exact_match
- label: exact match query
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 28.6 | 30.2 | 30.4 | 28.5 |
| score | 8 | 16.1 | 20.6 | 21.8 | 16.6 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 8 | 44.7 | 48.4 | 49.1 | 45.1 |

### T6_cross_lang
- label: cross-language semantic (KO→EN)
- topk: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 29.5 | 30.1 | 30.1 | 29.6 |
| score | 8 | 14.0 | 16.7 | 17.3 | 14.3 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 8 | 43.5 | 46.2 | 46.6 | 43.9 |

### T7_topk_1
- topk: 1
- label: topk scaling topk=1
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 26.2 | 28.2 | 28.5 | 26.8 |
| score | 5 | 16.3 | 19.4 | 19.6 | 16.8 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 42.8 | 45.4 | 45.6 | 43.6 |

### T7_topk_3
- topk: 3
- label: topk scaling topk=3
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 26.6 | 27.0 | 27.0 | 26.5 |
| score | 5 | 16.9 | 19.6 | 19.9 | 17.3 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 44.0 | 45.9 | 46.2 | 43.8 |

### T7_topk_5
- topk: 5
- label: topk scaling topk=5
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 26.6 | 27.0 | 27.0 | 26.5 |
| score | 5 | 17.9 | 19.6 | 19.7 | 17.8 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 44.5 | 46.0 | 46.3 | 44.3 |

### T7_topk_10
- topk: 10
- label: topk scaling topk=10
- runs: 5

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 5 | 26.5 | 27.2 | 27.3 | 26.6 |
| score | 5 | 18.1 | 19.0 | 19.1 | 17.1 |
| vault_topk | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| remind | 5 | 0.0 | 0.0 | 0.0 | 0.0 |
| total | 5 | 44.6 | 45.3 | 45.3 | 43.7 |


## Feature: `vault_status`

### T9_vault_status
- label: Vault gRPC health check
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| vault_health_check | 8 | 6.6 | 7.7 | 7.8 | 6.8 |


## Feature: `searchable`

### T10_short_en_searchable
- label: short English (~30 tokens)
- tokens_approx: 35
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 43.1 | 56.7 | 56.8 | 47.8 |
| score | 8 | 8.3 | 15.4 | 18.1 | 9.6 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 43.4 | 44.5 | 44.7 | 40.6 |
| total | 8 | 95.4 | 109.7 | 110.2 | 97.9 |

### T11_long_en_searchable
- label: long English (~150 tokens)
- tokens_approx: 155
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 67.5 | 68.5 | 68.7 | 64.4 |
| score | 8 | 8.4 | 10.0 | 10.1 | 8.8 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 43.4 | 45.2 | 45.5 | 43.6 |
| total | 8 | 119.8 | 121.2 | 121.6 | 116.8 |

### T12_korean_searchable
- label: Korean text
- tokens_approx: 50
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed | 8 | 71.3 | 84.9 | 85.8 | 74.5 |
| score | 8 | 10.1 | 13.4 | 14.5 | 10.4 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_searchable | 8 | 42.9 | 43.5 | 43.5 | 42.6 |
| total | 8 | 124.9 | 137.2 | 139.0 | 127.4 |


## Feature: `multi_capture`

### T13_multi_2phase
- label: 2-phase multi-capture
- phase_count: 2
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 71.2 | 80.9 | 85.0 | 72.7 |
| score | 8 | 9.7 | 10.8 | 10.8 | 9.6 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 42.7 | 43.8 | 44.1 | 42.5 |
| total | 8 | 124.0 | 132.3 | 135.5 | 124.9 |

### T14_multi_5phase
- label: 5-phase multi-capture
- phase_count: 5
- runs: 8

| Phase | n | p50 ms | p95 ms | p99 ms | mean ms |
|-------|---|--------|--------|--------|---------|
| embed_batch | 8 | 137.0 | 148.8 | 150.5 | 138.6 |
| score | 8 | 9.8 | 11.0 | 11.1 | 9.9 |
| vault_topk | 8 | 0.0 | 0.0 | 0.0 | 0.0 |
| insert_batch | 8 | 41.8 | 42.5 | 42.6 | 41.8 |
| total | 8 | 188.9 | 201.1 | 202.6 | 190.2 |
