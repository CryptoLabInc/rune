# Rune × envector-msa-1.4.3 Latency Benchmark Plan

> **[측정 환경]** pyenvector 1.4.3 사용. eval_mode=mm32, index_type=ivf_vct.
> envector-cloud-be에 배포된 v1.4.3 클러스터 엔드포인트 사용.
> Vault는 원격(`tcp://193.122.124.173:50051`)에서 TLS로 실행 중.

## Context

v1.2.2 latency benchmark(`benchmark/plans/latency_bench_plan_envector_v1.2.2.md`)의 후속으로,
**envector-msa-1.4.3 환경**에서 두 가지 insert_mode(single, batch)를 각각 측정한다.

v1.2.2 plan과 달라진 환경:

| 항목 | v1.2.2 | v1.4.3 |
|------|--------|--------|
| pyenvector | 1.2.2 | 1.4.3 |
| eval_mode | rmp | mm32 |
| index_type | flat | ivf_vct |
| insert_mode | single | **single**, **batch** 각각 측정 |
| envector 엔드포인트 | `0511-1401-0001-4r6cwxfc908b.clusters.envector.io` | v1.4.3 클러스터 (배포 후 결정) |

> **[비교 주의]** eval_mode, index_type, 인프라가 동시에 달라졌으므로  
> v1.2.2 수치와의 차이를 단일 요인으로 해석해선 안 된다.

> **[insert_mode 정의]** Rune `batch_capture` MCP tool(embed+score만 수행, insert 없음)과 무관.
> - `single insert`: `use_row_insert=True`, `data=[vec]` — row insert API 경로 (벡터 1개)
> - `batch insert`: `use_row_insert=False`, `data=[v1,...,vN]` — batch insert API 경로 (벡터 N개)

---

## 측정 대상 기능 및 파이프라인 분해

### Feature 1: `capture` (MCP tool)

```
[1] 텍스트 → Embedding (로컬, Qwen/Qwen3-Embedding-0.6B)
[2] Novelty Check → envector inner_product (FHE, eval_mode=mm32, index_type=ivf_vct)
[3] Vault TopK Decrypt (gRPC tcp://193.122.124.173:50051)
[4] FHE Encrypt → index.insert (single: 1개 / batch: N개 한 번에)
────
Total end-to-end
```

### Feature 2: `recall` (MCP tool)

```
[1] 쿼리 → Embedding (로컬)
[2] Encrypted Search → envector (eval_mode=mm32, ivf_vct nprobe 기반)
[3] Vault TopK Decrypt (gRPC 원격)
[4] 메타데이터 조회 (로컬 JSON 파싱)
────
Total end-to-end
```

### Feature 3: `vault_status` (MCP tool)

- Vault gRPC 연결 latency (원격 서버 RTT 포함)

### Feature 4: `multi_capture` (다중 phase 동시 embed+insert)

```
[1] texts → embed(texts): N개 벡터 배치 임베딩 (embed_single × N 아님)
[2] Novelty Check → envector score (primary record = texts[0])
[3] Vault TopK Decrypt (gRPC)
[4] FHE Encrypt → index.insert(vectors=vecs, use_row_insert=False): N개 배치 삽입
────
Total end-to-end
```

> **[목적]** 실제 capture에서 multi-phase decision 처리 경로를 재현.
> server.py의 `record_builder.build_phases()` → `insert_with_text(texts=[...])` 경로.
> single capture(`embed_single` × 1)와 비교해 배치 embed/insert 오버헤드 측정.

> **[시나리오]**
> T13 = 2-phase (DB + 캐시 레이어 두 단계 결정)
> T14 = 5-phase (마이크로서비스 전환 ADR 수준 복잡 결정)

### Feature 5: `searchable` (insert → MERGED_SAVED 대기)

```
[1] 텍스트 → Embedding (로컬)
[2] Novelty Check → envector score (FHE)
[3] Vault TopK Decrypt (gRPC)
[4] FHE Encrypt → index.insert(await_searchable=True)
     — RPC 제출 + 서버 MERGED_SAVED 상태까지 대기 포함
────
Total end-to-end (MERGED_SAVED 시점까지)
```

> **[주의]** `EnVectorClient.insert()`는 request_id를 반환하지 않으므로
> RPC 제출 시간과 서버 대기 시간을 분리 측정할 수 없다.
> 세부 분해가 필요한 경우 `benchmark/runners/insert_row_only.py` 사용.

> **[시나리오]** T1–T3 입력(짧은 영어, 긴 영어, 한국어)을 그대로 재사용.
> T10 = T1 입력 / T11 = T2 입력 / T12 = T3 입력.

---

## 테스트 시나리오

시나리오 정의(T1–T9), 입력 텍스트, topk 변형, batch size는 v1.2.2 plan과 동일.  
모든 시나리오는 **ivf_vct 인덱스** 대상.

| ID | Feature | insert_mode | 내용 |
|----|---------|-------------|------|
| T1 | capture | 지정값 | 짧은 영어 (~30 tokens) |
| T2 | capture | 지정값 | 긴 영어 (~150 tokens) |
| T3 | capture | 지정값 | 한국어 |
| T4 | capture | 지정값 | 중복 입력 (novelty near-dup) |
| T5 | recall  | — | exact match query |
| T6 | recall  | — | cross-lang KO→EN |
| T7 | recall  | — | topk scaling (1, 3, 5, 10) |
| T8 | vault_status | — | gRPC health check |
| T10 | searchable | — | 짧은 영어 → insert(await_searchable=True), MERGED_SAVED 대기 포함 |
| T11 | searchable | — | 긴 영어 → insert(await_searchable=True), MERGED_SAVED 대기 포함 |
| T12 | searchable | — | 한국어 → insert(await_searchable=True), MERGED_SAVED 대기 포함 |
| T13 | multi_capture | — | 2-phase: embed(2texts) + insert 2 vectors batch |
| T14 | multi_capture | — | 5-phase: embed(5texts) + insert 5 vectors batch |

---

## 측정 방법론

- **반복**: 10회 (warmup 2회 제외, 유효 8회)
- **보고 지표**: p50, p95, p99, mean (ms 단위)
- **타이머**: `time.perf_counter()`
- **단계별 측정**: embed / score / vault_topk / insert(또는 remind) / total 개별 계측

---

## 인프라 전제조건

1. envector-msa-1.4.3 → envector-cloud-be에 배포 완료
2. **ivf_vct 인덱스** 사전 생성 (nlist, default_nprobe 설정 포함)
3. Vault에 mm32 eval_mode 키 등록 완료
4. `~/.rune/config.json`의 `envector.endpoint` → v1.4.3 클러스터 URL로 업데이트

---

## 구현 파일

| 파일 | 설명 |
|------|------|
| `benchmark/runners/latency_bench_v1.4.3.py` | 신규 runner. `--insert-mode single|batch` 필수 |
| `benchmark/runners/common.py` | `PhaseLatency`, `LatencyScenarioResult`, `LatencyBenchReport` |
| `agents/common/envector_client.py` | `eval_mode` 파라미터 추가 (기본값 "mm32") |

---

## 실행 방법

```bash
# 사전 확인: vault 연결만 테스트
.venv/bin/python benchmark/runners/latency_bench_v1.4.3.py \
    --insert-mode single --feature vault_status --runs 3 --warmup 1

# single insert 전체 실행
.venv/bin/python benchmark/runners/latency_bench_v1.4.3.py \
    --insert-mode single --runs 10 --warmup 2 \
    --report benchmark/reports/latency_results_v1.4.3_ivfvct_single_$(date +%Y-%m-%d).md \
    --format md

# batch insert 전체 실행
.venv/bin/python benchmark/runners/latency_bench_v1.4.3.py \
    --insert-mode batch --runs 10 --warmup 2 \
    --report benchmark/reports/latency_results_v1.4.3_ivfvct_batch_$(date +%Y-%m-%d).md \
    --format md
```

---

## 검증 방법

1. **단계별 합계 일치**: `sum(phase latencies) ≈ total` (±5% 허용)
2. **재현성**: 같은 시나리오 재실행 시 p50 변동 < 20%
3. **batch 효율**: T1 insert_ms(batch) < N × T1 insert_ms(single) (배치 효율 확인)
4. **IVF_VCT score latency**: v1.2.2 flat score와 비교 → nprobe 오버헤드 반영 여부 확인
5. **중복 감지 동작**: T4에서 score phase가 T1 대비 증가하는지 확인
