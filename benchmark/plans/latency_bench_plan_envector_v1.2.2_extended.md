# Rune × envector-msa-1.2.2 Latency Benchmark Plan (extended v1.4.3 시나리오)

> **[측정 환경]** pyenvector 1.2.2 사용. **eval_mode=rmp, index_type=flat**.
> envector 엔드포인트: `0511-1401-0001-4r6cwxfc908b.clusters.envector.io` (v1.2.2 클러스터, TLS).
> Vault: `tcp://193.122.124.173:50051` (TLS).

## Context

본 plan은 v1.4.3 plan(`latency_bench_plan_envector_v1.4.3.md`)의 14개 시나리오(T1–T14)를
**v1.2.2 환경(rmp/flat)에서 동일하게 측정**하기 위해 작성한다.

v1.2.2의 기존 runner(`benchmark/runners/latency_bench.py`)는 손대지 않고, v1.4.3 runner
(`benchmark/runners/latency_bench_v1.4.3.py`)를 가져와 SDK 호출부만 v1.2.2 호환으로 어댑팅한다.

v1.4.3 환경과의 차이:

| 항목 | v1.2.2 (이번 측정) | v1.4.3 |
|------|--------|--------|
| pyenvector | 1.2.2 | 1.4.3 |
| eval_mode | rmp | mm32 |
| index_type | flat | ivf_vct |
| insert_mode | **single 만** (SDK 레벨에 single/batch 분기 없음) | single, batch 각각 |
| envector 엔드포인트 | `0511-1401-...envector.io` | `0512-1000-...envector.ai` |
| Vault 엔드포인트 | `193.122.124.173:50051` | `161.118.149.143:50051` |

> **[비교 주의]** pyenvector 버전, eval_mode, index_type, 엔드포인트가 동시에 달라졌으므로
> v1.4.3 수치와의 차이를 단일 요인으로 해석해선 안 된다.

> **[insert_mode 정의]** v1.4.3 runner의 `--insert-mode {single,batch}` 옵션은
> SDK 레벨의 `use_row_insert` 분기(`data=[vec]` vs `data=[v1,...,vN]`)를 의미한다.
> v1.2.2 pyenvector는 SDK 레벨에서 항상 single insert이므로 **batch insert는 측정 대상에서 제외**한다.
> (rune 애플리케이션 레벨의 `multi_capture`(T13–T14)와는 다른 개념이므로 혼동 금지.)

---

## 호환성 정책 (v1.2.2 환경 어댑팅)

| v1.4.3 runner 요소 | 시나리오의 의미 | v1.2.2 환경에서의 측정 방법 |
|---|---|---|
| `EnVectorClient(eval_mode, index_type)` 인자 | 인프라 설정 | v1.2.2 `EnVectorClient`에는 해당 인자 없음 — runner에서 **인자 제거**. eval_mode는 v1.2.2 클라이언트 내부 디폴트(`rmp`)에 의존. |
| `EnVectorClient(secure=...)` 인자 | TLS 모드 | v1.2.2에 없음 — runner에서 **제거**. v1.2.2는 access_token이 있으면 TLS 자동 활성. |
| `--insert-mode batch` / `use_row_insert=False` | N개 벡터를 SDK 한 호출로 삽입 | v1.2.2 SDK 레벨에 없음 — **측정 대상에서 제외**. v1.4.3 only로 비교 리포트에 표기. |
| `T10–T12 searchable` (`insert(await_searchable=True)`) | **capture 후 recall이 가능해지기까지의 시점** | v1.4.3는 `MERGED_SAVED` 상태로 측정. v1.2.2는 `MERGED_SAVED`가 노출되지 않으므로 **score → vault decrypt 폴링으로 top-1 cosine similarity가 임계(`0.999`)에 도달하는 순간**까지 대기. 측정 정밀도 ≈ poll_interval(200ms) + per-cycle RPC latency. |
| `T13–T14 multi_capture` (`use_row_insert=False`) | 다중 phase 결정의 누적/단계별 latency | application-level 시나리오이므로 호출 인자만 맞추면 phase 구조 그대로 재현. v1.2.2 SDK가 vectors=[v1,...,vN]을 어떻게 처리하든 의미는 보존. |
| `envector_secure` bundle field | vault 응답의 secure 플래그 | v1.2.2 클라이언트에 전달 안 함 — bundle에서 pop만 (다운스트림 cert 쓰기는 영향 없음). |

> **[Searchable polling caveat]** v1.2.2 polling은 200ms granularity + per-cycle RPC latency가
> 측정에 포함된다. v1.4.3의 server-push 측정(~80ms)과 직접 비교 시 polling 오버헤드를 차감하지 말 것 —
> "두 환경에서 capture 후 recall 가능 시점을 각각 어떤 방법으로 잴 수 있는가"의 비교로 해석.

---

## 측정 대상 기능 및 파이프라인 분해

### Feature 1: `capture` (T1–T4)

```
[1] 텍스트 → Embedding (로컬, Qwen/Qwen3-Embedding-0.6B)
[2] Novelty Check → envector inner_product (FHE, eval_mode=rmp, index_type=flat)
[3] Vault TopK Decrypt (gRPC tcp://193.122.124.173:50051)
[4] FHE Encrypt → index.insert(vectors=[vec]) — SDK 레벨 single insert
────
Total end-to-end
```

### Feature 2: `recall` (T5–T7)

```
[1] 쿼리 → Embedding (로컬)
[2] Encrypted Search → envector (eval_mode=rmp, flat brute-force)
[3] Vault TopK Decrypt (gRPC)
[4] remind: 메타데이터 조회
────
Total end-to-end
```

### Feature 3: `vault_status` (T9)

- Vault gRPC health check latency.

### Feature 4: `multi_capture` (T13–T14)

```
[1] texts → embed(texts): N개 벡터 배치 임베딩
[2] Novelty Check → envector score (primary record = texts[0])
[3] Vault TopK Decrypt (gRPC)
[4] index.insert(vectors=vecs, metadata=...): N개 벡터를 SDK 한 호출로 전달 (v1.2.2 SDK 내부 처리)
────
Total end-to-end
```

> **[목적]** 실제 capture에서 multi-phase decision 처리 경로를 application 레벨에서 재현.
> 시나리오: T13 = 2-phase, T14 = 5-phase.

### Feature 5: `searchable` (T10–T12) — **v1.2.2 polling 방식**

```
[1] 텍스트 → Embedding (로컬)
[2] Novelty Check → envector score (FHE)
[3] Vault TopK Decrypt (gRPC)
[4] index.insert(vectors=[vec])
    → 폴링: (score → vault decrypt) 반복하여 top-1 score ≥ 0.999일 때까지 대기
    — RPC 제출 시간 + polling overhead까지 합산
────
Total end-to-end (queryable 시점까지)
```

> **[v1.4.3와의 측정 메커니즘 차이]**
> v1.4.3: `insert(await_searchable=True)` — 서버가 `MERGED_SAVED` 상태 push할 때 RPC 반환.
> v1.2.2: 클라이언트가 score 결과를 폴링하여 top-1 cos similarity가 ~1.0이 되는 시점 검출.
> 같은 의미(=queryable 시점)를 다른 메커니즘으로 잰다.

> **[시나리오]** T1–T3 입력(짧은 영어, 긴 영어, 한국어)을 그대로 재사용.
> T10 = T1 입력 / T11 = T2 입력 / T12 = T3 입력.

---

## 테스트 시나리오

시나리오 정의(T1–T9), 입력 텍스트, topk 변형은 v1.4.3 plan과 동일.
모든 시나리오는 **flat 인덱스 대상**.

| ID | Feature | 내용 |
|----|---------|------|
| T1 | capture | 짧은 영어 (~30 tokens) |
| T2 | capture | 긴 영어 (~150 tokens) |
| T3 | capture | 한국어 |
| T4 | capture | 중복 입력 (novelty near-dup) |
| T5 | recall  | exact match query |
| T6 | recall  | cross-lang KO→EN |
| T7 | recall  | topk scaling (1, 3, 5, 10) |
| T9 | vault_status | gRPC health check |
| T10 | searchable | 짧은 영어 → insert + score-threshold polling |
| T11 | searchable | 긴 영어 → insert + score-threshold polling |
| T12 | searchable | 한국어 → insert + score-threshold polling |
| T13 | multi_capture | 2-phase: embed(2texts) + insert 2 vectors |
| T14 | multi_capture | 5-phase: embed(5texts) + insert 5 vectors |

> **[v1.2.2 기존 측정과의 차이]** `benchmark/runners/latency_bench.py`(2026-05-11)는 T1–T9까지만 측정.
> 본 plan은 T10–T14를 추가하여 v1.4.3와 동일한 시나리오 셋을 확보한다. T8(batch_capture)은 v1.4.3에서 폐지되었으므로 본 측정에서도 제외.

---

## 측정 방법론

- **반복**: 10회 (warmup 2회 제외, 유효 8회)
- **보고 지표**: p50, p95, p99, mean (ms 단위)
- **타이머**: `time.perf_counter()`
- **단계별 측정**: embed / score / vault_topk / insert(또는 remind / insert_searchable / insert_batch) / total 개별 계측

---

## 인프라 전제조건

1. v1.2.2 envector 클러스터 가동 (`0511-1401-...envector.io`)
2. v1.2.2 Vault 가동 (`tcp://193.122.124.173:50051`), 최신 CA cert (`~/.rune/certs/ca.pem`, 2026-05-12 갱신)
3. `~/.rune/config.json`의 `envector.endpoint` → v1.2.2 클러스터 URL, `vault.token` → 유효한 토큰
4. Python venv `/Users/heeyeon/Desktop/Projects/rune/.venv/` (pyenvector 1.2.2 설치 확인 완료)

---

## 구현 파일

| 파일 | 설명 |
|------|------|
| `benchmark/runners/latency_bench_v1.4.3.py` | v1.4.3 runner를 가져와 v1.2.2 호환으로 어댑팅 (SDK 호출부만 수정) |
| `benchmark/runners/common.py` | `PhaseLatency`, `LatencyScenarioResult`, `LatencyBenchReport` — v1.2.2 브랜치 원본 사용 |
| `agents/common/envector_client.py` | **v1.2.2 브랜치 원본 그대로** (eval_mode 인자 백포팅 X — 비교 공정성 보호) |

---

## 실행 방법

```bash
# 사전 확인: 가장 짧은 시나리오만 smoke
/Users/heeyeon/Desktop/Projects/rune/.venv/bin/python \
    benchmark/runners/latency_bench_v1.4.3.py \
    --insert-mode single --feature capture --runs 2 --warmup 1

# 본 측정 (v1.2.2 SDK는 항상 single insert이므로 single만 실행)
/Users/heeyeon/Desktop/Projects/rune/.venv/bin/python \
    benchmark/runners/latency_bench_v1.4.3.py \
    --insert-mode single --runs 10 --warmup 2 \
    --report benchmark/reports/latency_results_v1.2.2_rmpflat_single_$(date +%Y-%m-%d).md \
    --format md
```

---

## 검증 방법

1. **단계별 합계 일치**: `sum(phase latencies) ≈ total` (±5% 허용)
2. **재현성**: 같은 시나리오 재실행 시 p50 변동 < 20%
3. **기존 v1.2.2 측정(2026-05-11)과의 일관성**: T1–T7, T9의 p50이 ±20% 이내인지 확인. 큰 차이가 나면 인프라(네트워크 RTT) 변화로 해석.
4. **flat score latency**: v1.4.3 ivf_vct score(nprobe 탐색)와 비교 → flat은 brute-force라 score phase latency 차이의 주요 원인 후보.
5. **searchable polling overhead**: T10–T12의 `insert_searchable` phase가 일반 capture T1–T3의 `insert`보다 최소 polling interval(~200ms) 이상 길어야 정상.
