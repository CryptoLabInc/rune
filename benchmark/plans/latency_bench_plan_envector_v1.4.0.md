# Rune × envector-msa-1.4.0 Latency Benchmark Plan

> **[측정 환경]** envector 연결은 `secure=false`(plaintext gRPC)로 측정. Vault(`localhost:50051`)는 TLS 활성 상태.
> plaintext로 진행해도 무방하다고 판단한 이유: gRPC는 HTTP/2 커넥션을 재사용하므로 TLS 핸드셰이크 비용이 warmup 이후 측정값에 포함되지 않으며, 주요 병목이 FHE 연산에 있어 TLS 유무가 결과에 실질적 영향이 낮을 수 있다고 판단.
> 이후 envector TLS 환경(`secure=true`)에서도 동일 시나리오로 벤치마크를 진행할 예정.

## Context

기존 benchmark suite는 **capture 품질(정확도)과 recall 품질(hit rate)**을 측정하는 데 집중되어 있다.  
이번 작업은 rune의 각 MCP 기능을 실행했을 때 **실제 응답 시간(latency)**을 측정하는 것이 목표다.

envector-msa-1.4.0은 이전 버전(rmp)에서 크게 변경되었다:
- **KMS**: 독립 서비스로 분리 (Port 50060)  
- **Audit**: 독립 서비스로 분리 (Port 50052)  
- **pyenvector 1.4.0a5**: `secure` 파라미터 추가, 암호화 최적화 (PPMM cache 1.31–1.40x)  
- **gRPC-Gateway**: REST 전사(transcoding) 지원

rune의 현재 config는 `envector.endpoint = 146.56.178.130:50050` (원격 클라우드)를 사용하고 있으며,  
vault는 `tcp://localhost:50051`에서 로컬 gRPC로 실행된다.

---

## 측정 대상 기능 및 파이프라인 분해

### Feature 1: `capture` (MCP tool)

전체 파이프라인:
```
[1] 텍스트 → Embedding (로컬 fastembed, Qwen/Qwen3-Embedding-0.6B)
[2] Novelty Check → envector inner_product (FHE 암호화 쿼리 전송)
[3] Vault TopK Decrypt (gRPC localhost:50051)
[4] FHE Encrypt → pyenvector insert_data (envector 저장)
[5] capture_log 기록 (로컬 파일)
────
Total end-to-end
```

### Feature 2: `recall` (MCP tool)

전체 파이프라인:
```
[1] 쿼리 → Embedding (로컬)
[2] Encrypted Search → envector inner_product
[3] Vault TopK Decrypt (gRPC)
[4] 메타데이터 조회 (로컬 JSON 파싱)
────
Total end-to-end
```

### Feature 3: `batch_capture` (MCP tool)

- batch size: 1, 5, 10, 20
- 총 시간 + item당 평균 latency

### Feature 4: `vault_status` / `diagnostics` (MCP tool)

- Vault gRPC 연결 latency
- envector 헬스체크 latency

---

## 구체적 테스트 시나리오 (각 기능별)

### [T1] Capture — 짧은 영문 텍스트 (baseline)
```json
{
  "input": "We decided to use PostgreSQL as our primary database. Team familiarity and mature ecosystem were the key reasons. Redis considered but rejected due to durability concerns.",
  "domain": "architecture",
  "expected_insight": "PostgreSQL chosen over Redis for primary DB due to team familiarity and durability"
}
```
반복: 10회 (첫 2회 warmup 제외)

### [T2] Capture — 긴 영문 텍스트 (embedding 부하 테스트)
```json
{
  "input": "Full ADR: Context — our monolith hit 10k RPS limits. Considered: (1) horizontal scale with read replicas, (2) CQRS split, (3) microservices decomposition. Trade-offs: read replicas cheapest but doesn't solve write bottleneck; CQRS complex but keeps single codebase; microservices highest ops overhead. Decision: CQRS with event sourcing on order-service first. Rationale: allows independent scaling of read path, event log gives audit trail for compliance (legal requirement from Q2 review). Rollback plan: feature flag, revert within 2 sprints if P99 > 500ms.",
  "domain": "architecture",
  "label": "long_text_~150_tokens"
}
```
반복: 10회

### [T3] Capture — 한국어 텍스트 (multi-language)
```json
{
  "input": "Redis를 캐시 레이어로 사용하기로 결정했습니다. Memcached도 검토했지만 데이터 구조 지원(Sorted Set, List)이 필요해서 Redis로 확정. TTL은 1시간으로 설정. 담당: 백엔드팀.",
  "domain": "architecture",
  "label": "korean_text"
}
```
반복: 10회

### [T4] Capture — 중복 입력 (novelty check 동작 확인)
- T1의 입력을 두 번 연속 capture
- 두 번째 호출의 novelty check 결과(near-duplicate) 및 응답 시간 비교
- 중복 스킵 여부 확인

### [T5] Recall — Exact match 쿼리
```json
{
  "query": "Why did we choose PostgreSQL?",
  "topk": 5,
  "label": "exact_match"
}
```
반복: 10회

### [T6] Recall — Semantic match 쿼리 (의미 기반)
```json
{
  "query": "데이터베이스 선택 이유",
  "topk": 5,
  "label": "cross_language_semantic"
}
```
반복: 10회

### [T7] Recall — topk 변화에 따른 latency
```json
{
  "query": "architecture decisions",
  "topk_variants": [1, 3, 5, 10],
  "label": "topk_scaling"
}
```
각 topk 당 5회 반복

### [T8] Batch Capture — batch size 스케일링
```json
{
  "batch_sizes": [1, 5, 10, 20],
  "inputs": ["decision_{i}: chose option A over B because ..."],
  "label": "batch_scaling"
}
```
각 batch size 당 3회 반복

### [T9] Vault Status / Diagnostics 헬스체크
- `vault_status` 10회 반복
- `diagnostics` 10회 반복
- Vault gRPC cold/warm 연결 구분

### [T10] Memory 크기별 Recall latency (회귀 테스트)
- 빈 index에서 recall
- 10개 capture 후 recall
- 50개 capture 후 recall
- 각각 5회 반복하여 index 크기가 latency에 미치는 영향 확인

---

## 측정 방법론

### 통계 기준
- **반복**: 최소 10회 (warmup 2회 제외하여 8회 유효)
- **보고 지표**: p50, p95, p99, mean, min, max (ms 단위)
- **타이머**: `time.perf_counter()` (Python 고정밀 타이머)
- **단계별 측정**: 각 파이프라인 단계를 개별 타이머로 계측

### 환경 조건
- envector-msa-1.4.0: 원격 클라우드 (`146.56.178.130:50050`, `secure=false`)
- Vault: 로컬 (`tcp://localhost:50051`)
- Embedding model: `Qwen/Qwen3-Embedding-0.6B` (로컬)
- 네트워크 조건: 고정 IP, 테스트 전 `ping 146.56.178.130` 기준 RTT 기록

---

## 구현할 파일

### 1. `benchmark/runners/latency_bench.py` (신규)

기존 `common.py`의 `BenchmarkReport` / `ScenarioResult` 재사용.  
새로 추가할 구조:
```python
@dataclass
class LatencyMeasurement:
    phase: str          # "embedding", "novelty_check", "fhe_insert", "topk_decrypt", "total"
    samples_ms: list[float]
    p50: float
    p95: float
    p99: float
    mean: float

@dataclass
class LatencyBenchReport:
    feature: str        # "capture", "recall", "batch_capture", "vault_status"
    scenario_id: str
    measurements: list[LatencyMeasurement]
    metadata: dict      # text_length, topk, batch_size, etc.
```

**측정 방식**: MCP server를 직접 import하지 않고, `mcp/adapter/envector_sdk.py`와 `mcp/adapter/vault_client.py`를 직접 호출하는 방식으로 각 단계 계측.

**실행 방법**:
```bash
# 모든 시나리오
python benchmark/runners/latency_bench.py

# 특정 feature
python benchmark/runners/latency_bench.py --feature capture
python benchmark/runners/latency_bench.py --feature recall
python benchmark/runners/latency_bench.py --feature batch_capture

# 반복 횟수 지정
python benchmark/runners/latency_bench.py --runs 20

# 보고서 저장
python benchmark/runners/latency_bench.py --report benchmark/reports/latency_results_v1.4.0.md --format md
python benchmark/runners/latency_bench.py --report benchmark/reports/latency_results_v1.4.0.json --format json
```

### 2. `benchmark/reports/latency_results_v1.4.0.md` (보고서)

테스트 실행 후 자동 생성. 형식:
```markdown
# Rune × envector-msa-1.4.0 Latency Report

## Environment
- Date: 2026-05-XX
- envector endpoint: 146.56.178.130:50050
- Vault: tcp://localhost:50051
- Embedding model: Qwen/Qwen3-Embedding-0.6B
- Network baseline: XX ms RTT

## Feature: capture

### T1: Short English text (~30 tokens)
| Phase             | p50   | p95   | p99   | mean  |
|-------------------|-------|-------|-------|-------|
| embedding         |       |       |       |       |
| novelty_check     |       |       |       |       |
| fhe_encrypt+insert|       |       |       |       |
| vault_topk_decrypt|       |       |       |       |
| **total**         |       |       |       |       |

...
```

---

## 실행 전 사전 조건 체크

```bash
# 1. Vault 실행 확인
grpc_health_probe -addr=localhost:50051 || echo "Vault not running"

# 2. envector 연결 확인
python -c "import pyenvector as ev; ev.init(host='146.56.178.130', port=50050)"

# 3. Embedding model 로드 확인
python -c "from fastembed import TextEmbedding; m = TextEmbedding('Qwen/Qwen3-Embedding-0.6B'); print('OK')"

# 4. 네트워크 baseline
ping -c 5 146.56.178.130
```

---

## 검증 방법

1. **단계별 합계 일치**: `sum(phase latencies) ≈ total` (±5% 허용)  
2. **재현성**: 같은 시나리오를 서로 다른 시간에 실행했을 때 p50 변동 < 20%  
3. **스케일 선형성 확인**: T8 batch_capture에서 item당 latency가 batch_size 증가에도 일정한지 확인  
4. **중복 감지 동작**: T4에서 두 번째 capture가 "near_duplicate"로 skip되는지 확인  

---

## 수정할 파일 목록

| 파일 | 작업 |
|------|------|
| `benchmark/runners/latency_bench.py` | 신규 생성 |
| `benchmark/reports/latency_results_v1.4.0.md` | 실행 후 자동 생성 |
| `benchmark/runners/common.py` | `LatencyMeasurement`, `LatencyBenchReport` dataclass 추가 |

수정하지 않는 파일:
- `mcp/adapter/envector_sdk.py` — 직접 호출하되 수정 없음
- `mcp/adapter/vault_client.py` — 직접 호출하되 수정 없음
- `agents/common/embedding_service.py` — 직접 호출하되 수정 없음
