# Rune × envector-msa-1.2.2 Latency Benchmark Plan

> **[측정 환경]** pyenvector 1.2.2 사용. `secure` 파라미터 미지원 — `access_token` 설정 시 TLS 자동 활성(plaintext 옵션 없음).
> Vault는 원격(`tcp://193.122.124.173:50051`)에서 TLS로 실행 중.
> envector 엔드포인트: `0511-1401-0001-4r6cwxfc908b.clusters.envector.io`(클라우드 도메인 기반).
> **eval_mode=rmp, index_type=flat**.

## Context

v1.4.0 latency benchmark(`benchmark/plans/latency_bench_plan_envector_v1.4.0.md`)의 후속으로,
동일한 시나리오(T1–T9)를 **pyenvector 1.2.2 환경**에서 재측정한다.

v1.4.0 plan과 달라진 환경:

| 항목 | v1.4.0 | v1.2.2 |
|------|--------|--------|
| pyenvector | 1.4.0 | 1.2.2 |
| eval_mode | mm | rmp |
| index_type | flat | flat |
| envector TLS | `secure=false` (plaintext) | TLS 자동 활성 (access_token) |
| envector endpoint | `146.56.178.130:50050` (IP 직접) | `0511-1401-0001-4r6cwxfc908b.clusters.envector.io` (도메인) |
| Vault | `tcp://localhost:50051` (로컬) | `tcp://193.122.124.173:50051` (원격) |
| runner `secure=` 파라미터 | `cfg.envector.secure` 전달 | 제거 (1.2.2 미지원, TLS는 access_token으로 자동 활성) |

> **[비교 주의]** 위 세 가지 차이(버전, eval_mode, 인프라)가 동시에 바뀌었으므로 v1.4.0 수치와의 차이를 pyenvector 버전 단독 영향으로 해석해선 안 된다.

---

## 측정 대상 기능 및 파이프라인 분해

### Feature 1: `capture` (MCP tool)

```
[1] 텍스트 → Embedding (로컬, Qwen/Qwen3-Embedding-0.6B)
[2] Novelty Check → envector inner_product (FHE 암호화 쿼리 전송, eval_mode=rmp)
[3] Vault TopK Decrypt (gRPC tcp://193.122.124.173:50051)
[4] FHE Encrypt → pyenvector insert_data (envector 저장)
[5] capture_log 기록 (로컬 파일)
────
Total end-to-end
```

### Feature 2: `recall` (MCP tool)

```
[1] 쿼리 → Embedding (로컬)
[2] Encrypted Search → envector inner_product (eval_mode=rmp)
[3] Vault TopK Decrypt (gRPC 원격)
[4] 메타데이터 조회 (로컬 JSON 파싱)
────
Total end-to-end
```

### Feature 3: `batch_capture` (MCP tool)

- batch size: 1, 5, 10, 20
- insert 미수행 — embed + score(novelty)만 측정
- 총 시간 + item당 평균 latency

### Feature 4: `vault_status` (MCP tool)

- Vault gRPC 연결 latency (원격 서버 RTT 포함)

---

## 테스트 시나리오

시나리오 정의(T1–T9), 입력 텍스트, topk 변형, batch size는 v1.4.0 plan과 동일:
`benchmark/plans/latency_bench_plan_envector_v1.4.0.md` 참조.

---

## 측정 방법론

v1.4.0 plan과 동일:

- **반복**: 10회 (warmup 2회 제외, 유효 8회)
- **보고 지표**: p50, p95, p99, mean (ms 단위)
- **타이머**: `time.perf_counter()`
- **단계별 측정**: embed / score / vault_topk / insert(또는 remind) / total 개별 계측

---

## 구현 파일

| 파일 | 설명 |
|------|------|
| `benchmark/runners/latency_bench.py` | v1.4.0에서 port. `secure=` 파라미터 제거(1.2.2 미지원) |
| `benchmark/runners/common.py` | `PhaseLatency`, `LatencyScenarioResult`, `LatencyBenchReport` dataclass 포함 |
| `benchmark/reports/latency_results_v1.2.2_2026-05-11.md` | 실행 결과 |

---

## 실행 방법

```bash
# 사전 확인
.venv/bin/python benchmark/runners/latency_bench.py \
    --feature vault_status --runs 3 --warmup 1

# 전체 실행
.venv/bin/python benchmark/runners/latency_bench.py \
    --runs 10 --warmup 2 \
    --report benchmark/reports/latency_results_v1.2.2_2026-05-11.md \
    --format md
```

---

## 검증 방법

v1.4.0 plan과 동일:

1. **단계별 합계 일치**: `sum(phase latencies) ≈ total` (±5% 허용)
2. **재현성**: 같은 시나리오 재실행 시 p50 변동 < 20%
3. **batch 선형성**: T8에서 item당 latency가 batch_size 증가에도 일정한지 확인
4. **중복 감지 동작**: T4에서 두 번째 capture의 score phase가 첫 번째 대비 증가하는지 확인
