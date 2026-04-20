# runed 구현 명세 (Implementation Specification)

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스 실측 대조. 주요 교정:
> §2.3 AES 모드 **AES-256-CTR 확정** (pyenvector/utils/aes.py:52-58). §11.3
> record_id suffix 생성 시점 명확화 (`generate_record_id`가 아닌 `record_builder`
> 의 멀티 레코드 빌더가 붙임).

이 문서는 Go `runed` 데몬의 각 서브시스템을 **바로 구현할 수 있는 수준**으로
기술한다. 05-architecture-comparison.md의 §5를 풀어쓴 것이며, 현재 Python
소스 코드(vault_client.py, envector_sdk.py, server.py, 등)에서 추출한 프로토콜
상세, 데이터 타입, 와이어 포맷을 포함한다.

---

## 1. Vault 통신 (vault-go)

### 1.1 Proto 정의

패키지: `rune.vault.v1`. proto 파일은 `vault_service.proto`.

```protobuf
syntax = "proto3";
package rune.vault.v1;

service VaultService {
  rpc GetPublicKey    (GetPublicKeyRequest)     returns (GetPublicKeyResponse);
  rpc DecryptScores   (DecryptScoresRequest)    returns (DecryptScoresResponse);
  rpc DecryptMetadata (DecryptMetadataRequest)  returns (DecryptMetadataResponse);
}

// ── GetPublicKey ──────────────────────────────────
message GetPublicKeyRequest {
  string token = 1;            // Vault 인증 토큰 (evt_xxx)
}
message GetPublicKeyResponse {
  string key_bundle_json = 1;  // JSON 문자열: 아래 1.2절 참고
  string error = 2;            // 비어 있으면 성공
}

// ── DecryptScores ─────────────────────────────────
message DecryptScoresRequest {
  string token = 1;
  string encrypted_blob_b64 = 2;  // enVector가 반환한 FHE 스코어 ciphertext (base64)
  int32  top_k = 3;               // 최대 10
}
message DecryptScoresResponse {
  repeated ScoreEntry results = 1;
  string error = 2;
}
message ScoreEntry {
  int32  shard_idx = 1;
  int32  row_idx   = 2;
  double score     = 3;           // 0.0 ~ 1.0 cosine similarity (복호화된 plaintext)
}

// ── DecryptMetadata ───────────────────────────────
message DecryptMetadataRequest {
  string token = 1;
  repeated string encrypted_metadata_list = 2;  // per-record AES ciphertext (base64)
}
message DecryptMetadataResponse {
  repeated string decrypted_metadata = 1;  // per-record plaintext JSON 문자열
  string error = 2;
}
```

### 1.2 GetPublicKey 응답의 key_bundle_json 구조

`key_bundle_json`은 JSON 문자열이다. `json.Unmarshal` 후 아래 필드를 추출:

```json
{
  "EncKey.json": "<FHE 공개키 JSON 문자열>",
  "EvalKey.json": "<FHE 평가키 JSON 문자열 — 수십 MB 가능>",
  "index_name": "team-decisions",
  "key_id": "key_abc123",
  "agent_id": "agent_xyz",
  "agent_dek": "<base64-encoded 32-byte AES-256 DEK>",
  "envector_endpoint": "redcourage-xxx.clusters.envector.io",
  "envector_api_key": "uiEHmJ4..."
}
```

**구현 시 처리**:
- `EncKey.json` → `~/.rune/keys/EncKey.json` 에 캐시 (디스크 + in-memory)
- `EvalKey.json` → `~/.rune/keys/EvalKey.json` 에 캐시 (수십 MB — 디스크 캐시 중요)
- `index_name` → envector-go가 모든 insert/score/remind에서 사용
- `key_id` → envector-go 초기화 파라미터
- `agent_id` + `agent_dek` → capture 시 AES metadata 암호화에 사용
- `envector_endpoint` + `envector_api_key` → envector-go gRPC 연결에 사용
- 이 값들을 `~/.rune/config.json`의 `envector.endpoint`, `envector.api_key`에 기록
  (사용자가 직접 입력하지 않는 이유)

### 1.3 TLS 3모드

| config 값 | Go 구현 |
|---|---|
| `tls_disable: true` | `grpc.WithTransportCredentials(insecure.NewCredentials())` |
| `ca_cert: "/path/to/ca.pem"` | PEM 파일 읽기 → `credentials.NewTLS(&tls.Config{RootCAs: pool})` |
| `ca_cert: ""`, `tls_disable: false` | 시스템 CA 풀 사용 (Go 기본값) |

### 1.4 gRPC 옵션

현재 Python 코드(vault_client.py:166-169)에서 설정하는 옵션:

```go
opts := []grpc.DialOption{
    grpc.WithDefaultCallOptions(
        grpc.MaxCallRecvMsgSize(256 * 1024 * 1024), // 256 MB (EvalKey 때문)
        grpc.MaxCallSendMsgSize(256 * 1024 * 1024),
    ),
}
```

RFC §3.7에서 추가 제안하는 keepalive (현재 Python에는 없음):

```go
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                30 * time.Second,
    Timeout:             10 * time.Second,
    PermitWithoutStream: true,
})
```

### 1.5 Endpoint 파싱 로직

현재 `vault_client.py:117-140`의 로직을 Go로 포팅:

| 입력 | 파싱 결과 |
|---|---|
| `"vault:50051"` | `"vault:50051"` (직접 사용) |
| `"tcp://vault:50051"` | `"vault:50051"` (tcp:// 제거) |
| `"http://vault:50080/mcp"` | `"vault:50051"` (legacy HTTP, 포트 교체) |
| `"https://vault.example.com"` | `"vault.example.com:50051"` (포트 추가) |
| `"bare-hostname"` | `"bare-hostname:50051"` (기본 포트) |

`RUNEVAULT_GRPC_TARGET` 환경변수가 있으면 위 파싱을 무시하고 직접 사용.

### 1.6 Health Check

gRPC health check 프로토콜 사용 (`grpc.health.v1.Health/Check`). 실패 시
HTTP `/health` fallback (legacy endpoint가 http:// 인 경우만).

**HTTP fallback 상세**: gRPC health check 실패 시, endpoint가 `http://` 또는 `https://`이면 경로에서 `/mcp`, `/sse` suffix를 제거하고 `GET /health`를 시도한다 (vault_client.py:327-329).

```go
// Primary: gRPC health
healthClient := grpc_health_v1.NewHealthClient(conn)
resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
if resp.Status == grpc_health_v1.HealthCheckResponse_SERVING { return true }
```

### 1.7 vault-go 전체 인터페이스 (Go)

```go
package vault

type Client struct {
    conn   *grpc.ClientConn
    stub   pb.VaultServiceClient
    token  string
    config TLSConfig
}

type TLSConfig struct {
    Disable bool
    CACert  string // PEM 파일 경로. 비어있으면 시스템 CA
}

type ScoreEntry struct {
    ShardIdx int32
    RowIdx   int32
    Score    float64
}

// GetPublicKey: Vault에서 FHE 키 번들 다운로드
func (c *Client) GetPublicKey(ctx context.Context) (*KeyBundle, error)

// DecryptScores: FHE 스코어 ciphertext 복호화 → top-k plaintext 점수
func (c *Client) DecryptScores(ctx context.Context, blob64 string, topK int) ([]ScoreEntry, error)

// DecryptMetadata: AES 암호화된 metadata 복호화 → plaintext JSON 문자열 리스트
func (c *Client) DecryptMetadata(ctx context.Context, encryptedList []string) ([]string, error)

// Health: Vault 도달 가능 여부
func (c *Client) Health(ctx context.Context) bool

// Close: gRPC 채널 닫기
func (c *Client) Close() error
```

---

## 2. enVector 통신 (envector-go)

### 2.1 enVector SDK 개요

현재 Python은 `pyenvector` 패키지를 사용한다. 핵심 호출:

```python
ev.init(address, key_path, key_id, eval_mode, auto_key_setup, access_token)
index = ev.Index(index_name)
```

내부적으로 `ev.init()`은 enVector Cloud의 gRPC 서버에 연결하고, key_path에서
EncKey/EvalKey를 로드한다. `ev.Index(name)`는 특정 인덱스에 대한 핸들을 반환.

### 2.2 세 가지 핵심 연산

#### 2.2.1 Scoring (recall 경로)

```python
# Python
scores = index.scoring(query_vector)  # query는 List[float] (평문, query_encryption=False)
# → List[CipherBlock] (is_score=True)

# 각 CipherBlock에서:
serialized = cb.data.SerializeToString()  # protobuf 직렬화
blob_b64 = base64.b64encode(serialized).decode('utf-8')
# → 이 blob_b64를 Vault.DecryptScores에 전달
```

**Go 구현 포인트**:
- `query_encryption=False`이므로 쿼리 벡터는 평문 `[]float32`로 전송
- enVector가 반환하는 `CipherBlock`은 opaque 바이트 (FHE 수학 불필요)
- protobuf `SerializeToString()` → base64 인코딩만 하면 됨
- **FHE 라이브러리(CGO)가 필요 없는 경로**

#### 2.2.2 Remind / GetMetadata (recall 경로)

```python
# Python
results = index.indexer.get_metadata(
    index_name=index_name,
    idx=[{"shard_idx": 0, "row_idx": 42}, ...],  # DecryptScores 결과에서
    fields=["metadata"]
)
# → List[protobuf Metadata 객체]
# MessageToDict()로 변환 후 사용
```

**Go 구현 포인트**:
- DecryptScores가 반환한 `{shard_idx, row_idx}` 쌍으로 메타데이터 조회
- 반환된 metadata는 AES 암호화 상태 → Vault.DecryptMetadata로 복호화
- **FHE 라이브러리(CGO)가 필요 없는 경로**

#### 2.2.3 Insert (capture 경로)

```python
# Python
# 1. 평문 벡터를 FHE로 암호화 (EncKey 사용)
# 2. metadata를 AES로 암호화 (per-agent DEK)
# 3. enVector에 삽입
result = index.insert(
    data=vectors,      # List[List[float]] — pyenvector가 내부에서 EncKey로 암호화
    metadata=metadata  # List[str] — 이미 AES 암호화된 JSON 문자열
)
# → 각 벡터의 ID 반환
```

**Go 구현 포인트**:
- `index.insert(data=vectors)`가 내부에서 하는 일:
  1. `vectors` (평문 `[]float32`) → EncKey로 FHE 암호화 → `CipherBlock`
  2. `CipherBlock` + metadata를 enVector Cloud gRPC로 전송
- **이 FHE 암호화 단계가 CGO가 필요한 유일한 지점**
- RFC §3.4: envector C++ 코어의 encrypt 함수를 CGO로 바인딩

### 2.3 AES Metadata 암호화

capture 시 metadata를 per-agent DEK(32바이트 AES-256)로 암호화한다.
현재 Python 코드 (envector_sdk.py:227-234):

```python
from pyenvector.utils.aes import encrypt_metadata as aes_encrypt
ct = aes_encrypt(metadata_str, self._agent_dek)
envelope = json.dumps({"a": self._agent_id, "c": ct})
```

**봉투 포맷**:
```json
{"a": "agent_xyz", "c": "<base64 AES-256 ciphertext>"}
```

- `"a"`: agent_id (어떤 에이전트가 암호화했는지 식별)
- `"c"`: base64 인코딩된 AES-256 ciphertext

**Go 구현 (2026-04-17 확정)**: `crypto/aes` + `crypto/cipher.NewCTR` 사용.
pyenvector 소스(`pyenvector/utils/aes.py:52-58`) 확인 결과 **AES-256-CTR**로 확정.
docstring의 "AES-GCM" 언급은 오래된 주석이며 실제 구현은 CTR.

- IV: 16바이트 random, 와이어 포맷은 `IV || CT → base64`
- Padding 불필요 (CTR 스트림 모드)
- 키 크기: 32바이트 (AES-256, config.py L244-252에서 DEK 길이 검증)

**위치**: RFC 기준으로 `scribe/capture.go`에 직접 구현 (envector-go가 아님).
이유: AES metadata 암호화는 애플리케이션 레벨 기능이고 enVector 프로토콜
코어와 분리해야 함.

### 2.4 Connection Resilience

현재 Python의 `_with_reconnect` 패턴 (envector_sdk.py:185-196):

```
try:
    return fn()
except connection_error:
    reinit()     # ev.init() 재호출
    return fn()  # 1회 재시도
```

Go에서도 동일한 패턴 유지. gRPC 에러 메시지에서 연결 끊김 판단:

```go
var connectionErrorPatterns = []string{
    "UNAVAILABLE", "DEADLINE_EXCEEDED", "Connection refused",
    "Connection reset", "Stream removed", "RST_STREAM",
    "Broken pipe", "Transport closed", "Socket closed",
    "EOF", "failed to connect",
}
```

**enVector pre-warm**: `reload_pipelines` 후 `get_index_list()`를 60s 타임아웃으로 호출하여 gRPC 채널을 예열한다 (server.py:1051-1080). 비치명적 — 실패해도 reload 성공.

### 2.5 envector-go 전체 인터페이스 (Go)

```go
package envector

type Client struct {
    indexName  string
    keyID     string
    keyPath   string       // ~/.rune/keys/
    address   string       // enVector Cloud gRPC endpoint
    token     string       // enVector API key
    agentID   string
    agentDEK  []byte       // 32-byte AES-256 key
}

// Score: 평문 쿼리 벡터로 encrypted similarity search
// → FHE ciphertext blob (base64) 반환 — Vault.DecryptScores에 전달해야 함
func (c *Client) Score(ctx context.Context, query []float32) ([]string, error)

// Remind: shard_idx/row_idx 쌍으로 encrypted metadata 조회
// → AES ciphertext 문자열 리스트 — Vault.DecryptMetadata에 전달해야 함
func (c *Client) Remind(ctx context.Context, indices []ScoreEntry) ([]string, error)

// Insert: 평문 벡터를 FHE 암호화 후 enVector에 삽입
// ※ 이 함수 내부에서 CGO를 통해 envector C++ 코어의 encrypt를 호출
func (c *Client) Insert(ctx context.Context, vectors [][]float32, metadata []string) ([]string, error)

// GetIndexList: 사용 가능한 인덱스 목록
func (c *Client) GetIndexList(ctx context.Context) ([]string, error)

// Reinit: 연결 재설정 (sleep/wake 복구용)
func (c *Client) Reinit() error
```

---

## 3. 임베딩 엔진 (embed-go)

### 3.1 현재 사양

- 모델: `Qwen/Qwen3-Embedding-0.6B`
- 모드: `sbert` (sentence-transformers)
- 출력: L2 정규화된 float 벡터
- 차원: **1024** (Qwen3-Embedding-0.6B의 실제 출력 차원. `agents/common/schemas/embedding.py`의
  벤치마크 코멘트와 retriever 테스트에서 확인됨. 일부 테스트의 384는 MiniLM 클래스의 legacy default)
- 정규화: `EmbeddingAdapter._normalize_embeddings()`에서 L2 norm (embeddings.py:42-48)

### 3.2 Go에서의 옵션 (Open Question)

| 옵션 | 장점 | 단점 |
|---|---|---|
| ONNX Runtime (onnxruntime_go) | Go 네이티브, 단일 바이너리 | Qwen3 ONNX export 존재 여부 확인 필요 |
| Python 서브프로세스 | 확실히 동작, 모델 호환성 보장 | "단일 바이너리" 목표 위반, Python 의존 |
| HTTP 위임 (Ollama 호환) | 기존 생태계 재활용 | 외부 서비스 의존, 설치 복잡도 |

### 3.3 인터페이스 (Go)

```go
package embed

type Service struct {
    model     Model          // 실제 모델 (ONNX 또는 Python wrapper)
    dimension int            // 벡터 차원 (실측 후 확정)
}

// Embed: 텍스트 → L2 정규화된 float32 벡터
func (s *Service) Embed(ctx context.Context, text string) ([]float32, error)

// EmbedBatch: 여러 텍스트를 한 번에 임베딩 (쿼리 확장용)
func (s *Service) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

// Dimension: 벡터 차원 반환
func (s *Service) Dimension() int

// Ready: 모델 로드 완료 여부
func (s *Service) Ready() bool
```

---

## 4. Capture Gateway (scribe/capture.go)

### 4.1 데이터 흐름

```
HTTP POST /capture
  body: {
    "text_to_embed": "PostgreSQL was chosen over MongoDB because ACID...",
    "metadata": { ... agent가 구성한 opaque JSON ... },
    "session_id": "sess_abc"   (optional)
  }
       │
       ▼
  ┌─ Step 1: Embed ─────────────────────────────────────────────┐
  │ vec := embed.Embed(req.TextToEmbed)                         │
  │ → []float32 (예: 1024-dim, L2 정규화됨)                     │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  ┌─ Step 2: FHE 벡터 암호화 ───────────────────────────────────┐
  │ cipherBlock := envector.EncryptVector(vec, EncKey)           │
  │ → CipherBlock (opaque 바이트)                                │
  │ ※ CGO → envector C++ 코어 호출                              │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  ┌─ Step 3: AES metadata 암호화 ───────────────────────────────┐
  │ metaJSON := json.Marshal(req.Metadata)                      │
  │ ct := aes256Encrypt(metaJSON, agentDEK)                     │
  │ envelope := fmt.Sprintf(`{"a":"%s","c":"%s"}`, agentID, ct) │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  ┌─ Step 4: enVector Insert ───────────────────────────────────┐
  │ vectorIDs := envector.Insert(                               │
  │   indexName,                                                │
  │   []CipherBlock{cipherBlock},                               │
  │   []string{envelope},                                       │
  │ )                                                           │
  │ recordID := vectorIDs[0]                                    │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  ┌─ Step 5: 로컬 감사 로그 ────────────────────────────────────┐
  │ captureLog.Append(CaptureEntry{                             │
  │   Ts:           time.Now().UTC(),                           │
  │   Action:       "captured",                                 │
  │   ID:           recordID,                                   │
  │   Title:        record.Title,                               │
  │   Domain:       record.Domain,                              │
  │   Mode:         "agent-delegated",                          │
  │   NoveltyClass: noveltyClass,                               │
  │   NoveltyScore: noveltyScore,                               │
  │ })                                                          │
  │ → ~/.rune/capture_log.jsonl 에 1줄 append                   │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  HTTP 200 응답: {"ok": true, "record_id": "vec_123abc"}

※ Novelty 체크 실패는 **non-fatal** — 예외 발생 시 경고 로그 후 캡처를 계속 진행한다 (server.py:1370-1372).
```

### 4.2 Batch Capture

`POST /batch-capture`는 items 배열의 각 항목에 대해 위 Step 1-5를 **독립적으로**
실행한다. 한 항목의 실패가 다른 항목에 영향을 주지 않는다.

```json
// 응답
{
  "ok": true,
  "results": [
    {"ok": true, "record_id": "vec_001"},
    {"ok": false, "error": "enVector insert failed: timeout"},
    {"ok": true, "record_id": "vec_003"}
  ]
}
```

### 4.3 capture_log.jsonl 레코드 포맷

```json
{"ts":"2026-04-16T09:30:00+00:00","action":"captured","id":"vec_123abc","title":"PostgreSQL 선택","domain":"engineering","mode":"agent-delegated","novelty_class":"novel","novelty_score":0.25}
```

한 줄 = 한 레코드. 역시간순 읽기는 `GET /history`에서 수행.

---

## 5. Retriever Pipeline (retriever/)

### 5.1 전체 데이터 흐름

```
HTTP POST /recall
  body: {"query": "PostgreSQL 결정 이유", "topk": 5, "filters": {...}}
       │
       ▼
  ┌─ Stage 1: 쿼리 분석 (query.go) ─────────────────────────────┐
  │ parsed := queryProcessor.Process(query)                      │
  │                                                              │
  │ → ParsedQuery {                                              │
  │     Original:       "PostgreSQL 결정 이유"                    │
  │     Intent:         "rationale"  (8종 중 하나)               │
  │     Entities:       ["PostgreSQL"]                            │
  │     Keywords:       ["결정", "이유", "PostgreSQL"]            │
  │     TimeScope:      nil  (또는 LAST_MONTH 등)                │
  │     ExpandedQueries: [                                       │
  │       "PostgreSQL 결정 이유",                                 │
  │       "why PostgreSQL was chosen",                            │
  │       "PostgreSQL vs MongoDB decision"                        │
  │     ] (최대 5개, 상위 3개만 실제 검색에 사용)                  │
  │   }                                                          │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  ┌─ Stage 2: 멀티 쿼리 벡터 검색 (search.go) ──────────────────┐
  │ for each q in parsed.ExpandedQueries[:3]:                    │
  │                                                              │
  │   2a. vec := embed.Embed(q)                                  │
  │       → []float32 (평문 쿼리 벡터)                           │
  │                                                              │
  │   2b. blobs := envector.Score(vec)                           │
  │       → []string (FHE ciphertext, base64)                    │
  │                                                              │
  │   2c. entries := vault.DecryptScores(blobs[0], topK)         │
  │       → []ScoreEntry {ShardIdx, RowIdx, Score}               │
  │                                                              │
  │   2d. metaCT := envector.Remind(entries)                     │
  │       → []string (AES 암호화된 metadata, base64)             │
  │                                                              │
  │   2e. metaPlain := vault.DecryptMetadata(metaCT)             │
  │       → []string (plaintext JSON)                            │
  │                                                              │
  │   2f. records 에 추가, record_id로 dedup (첫 등장 유지)      │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  ┌─ Stage 3: 필터 + 재랭킹 (rerank.go) ────────────────────────┐
  │                                                              │
  │ 3a. metadata 필터 (best-effort):                             │
  │     if filters.Domain != "" → domain 필드 일치만 유지        │
  │     if filters.Status != "" → status 필드 일치만 유지        │
  │     if filters.Since != "" → created_at ≥ since만 유지       │
  │                                                              │
  │ 3b. 시간 범위 필터:                                          │
  │     if parsed.TimeScope != nil →                             │
  │       LAST_WEEK: 7일 이내만                                  │
  │       LAST_MONTH: 30일 이내만                                │
  │       LAST_QUARTER: 90일 이내만                              │
  │       LAST_YEAR: 365일 이내만                                │
  │                                                              │
  │ 3c. Recency 가중:                                            │
  │     ageDays := now - record.created_at (일 단위)             │
  │     decay := math.Pow(0.5, ageDays / 90)                    │
  │     // 90일 half-life: 90일 된 레코드는 0.5x, 180일은 0.25x │
  │                                                              │
  │ 3d. Status 승수:                                             │
  │     accepted   → 1.0                                         │
  │     proposed   → 0.9                                         │
  │     superseded → 0.5                                         │
  │     reverted   → 0.3                                         │
  │                                                              │
  │ 3e. 최종 점수 (가중 평균):                                   │
  │     adjustedScore = (0.7 * rawScore + 0.3 * decay) * statusMul │
  │                                                              │
  │ 3f. adjustedScore 내림차순 정렬                              │
  │     상위 topK개 선택                                         │
  └──────────────────────────────────────────────────────────────┘
       │
       ▼
  HTTP 200 응답:
  {
    "ok": true,
    "found": 3,
    "results": [
      {
        "record_id": "vec_abc",
        "score": 0.87,
        "adjusted_score": 0.82,
        "metadata": { ... 복호화된 원본 metadata ... }
      },
      ...
    ],
    "confidence": 0.82,   // 최고 adjusted_score
    "warnings": []
  }
```

### 5.2 의도 분류 (query.go)

MVP에서는 **영어 regex만** 구현 (비영어 LLM 라우팅은 DEFER):

| 의도 | regex 패턴 예시 |
|---|---|
| `rationale` | `why did we`, `reasoning behind`, `결정.*이유` |
| `implementation` | `how did we implement`, `what patterns` |
| `security` | `security considerations`, `compliance` |
| `performance` | `performance requirements`, `scalability` |
| `historical` | `when did we decide`, `have we discussed` |
| `team` | `who decided`, `which team owns` |
| `definition` | `what is our approach`, `what's our policy` |
| `other` | (위에 매칭되지 않으면) |

### 5.3 Phase Chain / Group 처리

**RFC 기준**: 데몬이 metadata를 opaque로 취급하므로 phase chain 확장을 하지 않음.
**우리 분석 기준**: DEFER (MVP는 flat list 반환).

어느 쪽이든 MVP에서는 phase chain 확장 로직이 없다. 결과는 flat list로 반환되며,
metadata 안에 `chain_id`, `phase_seq` 등이 있어도 데몬은 해석하지 않는다.
호출한 에이전트가 결과의 metadata를 읽어서 필요 시 그룹핑한다.

---

## 6. Config 관리 (config/)

### 6.1 Go 구조체

```go
type Config struct {
    Vault    VaultConfig    `json:"vault"`
    EnVector EnVectorConfig `json:"envector"`
    Embed    EmbedConfig    `json:"embedding"`
    Retriever RetrieverConfig `json:"retriever"`
    State       string `json:"state"`          // "active" | "dormant"
    DormantReason string `json:"dormant_reason"` // 코드: vault_unreachable, vault_token_invalid, user_deactivated, ...
    DormantSince  string `json:"dormant_since"`  // ISO 8601
}

type VaultConfig struct {
    Endpoint   string `json:"endpoint"`    // "tcp://vault:50051"
    Token      string `json:"token"`       // "evt_xxx"
    CACert     string `json:"ca_cert"`     // PEM 파일 경로 또는 ""
    TLSDisable bool   `json:"tls_disable"` // true면 plaintext
}

type EnVectorConfig struct {
    Endpoint string `json:"endpoint"` // Vault 번들에서 자동 채워짐
    APIKey   string `json:"api_key"`  // Vault 번들에서 자동 채워짐
}

type EmbedConfig struct {
    Mode  string `json:"mode"`  // "sbert"
    Model string `json:"model"` // "Qwen/Qwen3-Embedding-0.6B"
}

type RetrieverConfig struct {
    TopK                int     `json:"topk"`                 // 기본 10
    ConfidenceThreshold float64 `json:"confidence_threshold"` // 기본 0.5
}
```

**중요**: Go config 로더는 **알 수 없는 필드를 무시**해야 함 (v0.3.x config에
있는 legacy 필드: `llm.*`, `scribe.*` 등). `json.Unmarshal`은 기본적으로 이렇게
동작하므로 별도 처리 불필요.

### 6.2 Config Reload 메커니즘

1. **fsnotify watch**: `~/.rune/config.json`에 대한 파일시스템 이벤트 구독.
   파일이 변경되면 자동 reload.
2. **POST /reload**: 명시적 HTTP 호출로 reload 트리거.
3. **SIGHUP**: 시그널 수신 시 reload.

Reload 시 수행하는 작업:
- config.json 재파싱
- Vault 키 번들 재다운로드 (endpoint/token이 바뀌었을 수 있으므로)
- enVector 클라이언트 재초기화 (endpoint가 바뀌었을 수 있으므로)
- 상태 전이 평가 (dormant → active 가능)

### 6.3 상태 머신

```
                    configure (인프라 OK)
(not installed) ─────────────────────────→ active
        │                                    │
        │ configure (인프라 실패)              │ deactivate
        ▼                                    │ 또는 인프라 실패
     dormant ←───────────────────────────────┘
        │                                    │
        │ activate (인프라 OK)               │
        └────────────────────────────────────→ active
        │
        │ reset
        ▼
     dormant (config 삭제됨)
```

dormant_reason 코드:
- `vault_unreachable`: Vault gRPC 연결 실패
- `vault_token_invalid`: 인증 거부
- `envector_unreachable`: enVector 연결 실패
- `config_invalid`: 필수 필드 누락
- `user_deactivated`: 사용자가 명시적으로 deactivate

---

## 7. 데몬 Lifecycle (daemon.go)

### 7.1 Startup 시퀀스

```
runed 프로세스 시작
    │
    ├── 1. config.json 로딩
    │      state != "active" → dormant 모드로 시작
    │      (HTTP 서버는 뜨되, /recall /capture는 에러 반환)
    │
    ├── 2. Vault 키 번들 다운로드
    │      GetPublicKey(token) → key_bundle
    │      EncKey.json, EvalKey.json → ~/.rune/keys/ 캐시
    │      envector_endpoint, envector_api_key 추출
    │
    ├── 3. enVector 클라이언트 초기화
    │      ev.init(address, key_path, key_id, ...)
    │
    ├── 4. 임베딩 모델 로드 (가장 느린 단계, 수 초)
    │      embed.Service 초기화 → 모델을 메모리에 올림
    │
    ├── 5. Unix socket 리스닝 시작
    │      net.Listen("unix", "~/.rune/sock")
    │      os.Chmod(sockPath, 0600)
    │
    ├── 6. fsnotify watch 시작 (config.json)
    │
    └── 7. HTTP 서버 시작 (goroutine)
           → 요청 수신 대기
```

### 7.2 Shutdown 시퀀스

```
SIGTERM 또는 SIGINT 수신
    │
    ├── 1. HTTP 서버 graceful shutdown (in-flight 요청 대기, 30s 제한)
    ├── 2. gRPC 채널 close (vault + envector)
    ├── 3. 임베딩 모델 해제
    ├── 4. Unix socket 파일 삭제
    └── 5. 프로세스 종료
```

### 7.3 Sleep/Wake 복구

macOS 노트북 닫았다 열면 gRPC 채널이 stale해질 수 있다.

```
OS wake 이벤트 감지 (또는 gRPC 호출 실패 감지)
    │
    ├── vault gRPC 채널 닫기 + 재생성
    ├── envector gRPC 채널 닫기 + 재생성 (Reinit)
    └── health check로 복구 확인
```

---

## 8. 에러 처리 (errors.go)

### 8.1 에러 구조

```go
type RuneError struct {
    Code         string `json:"code"`          // 머신 리더블
    Message      string `json:"message"`       // 사람 리더블
    Retryable    bool   `json:"retryable"`     // 재시도 가능 여부
    RecoveryHint string `json:"recovery_hint"` // 복구 안내
}
```

### 8.2 에러 코드

| 코드 | retryable | 설명 |
|---|---|---|
| `VAULT_CONNECTION_ERROR` | true | Vault 도달 불가 |
| `VAULT_DECRYPTION_ERROR` | false | 복호화 실패 또는 토큰 인증 실패 (Python에서는 auth와 decrypt를 구분하지 않음) |
| `ENVECTOR_CONNECTION_ERROR` | true | enVector 도달 불가 |
| `ENVECTOR_INSERT_ERROR` | true | 삽입 실패 |
| `PIPELINE_NOT_READY` | false | 임베딩 모델 아직 로딩 중 (재시도보다는 /rune:activate로 재초기화) |
| `INVALID_INPUT` | false | 잘못된 요청 파라미터 |
| `INTERNAL_ERROR` | false | 예상치 못한 에러 (RuneError 기저 클래스 default) |
| `VAULT_AUTH_ERROR` | false | **Go 신규** — Python에는 없음; 토큰 인증 실패를 VAULT_DECRYPTION_ERROR에서 분리 |
| `CONFIG_ERROR` | false | **Go 신규** — Python에는 없음; config.json 파싱 실패 또는 필수값 누락 |
| `DORMANT` | false | **Go 신규** — Python에는 없음; dormant 상태를 명시적 에러 코드로 노출 |

### 8.3 HTTP 응답 매핑

```
RuneError.Retryable == true  → HTTP 503 Service Unavailable
RuneError.Code == DORMANT    → HTTP 503 + dormant_reason
RuneError.Code == INVALID_*  → HTTP 400 Bad Request
RuneError.Code == *_AUTH_*   → HTTP 401 Unauthorized
그 외 에러                    → HTTP 500 Internal Server Error

성공                          → HTTP 200 OK
```

모든 응답은 JSON body에 `"ok": true/false` 필드를 포함.
에러 시 `"error": { "code": "...", "message": "...", ... }` 포함.

---

## 9. 파일 시스템 레이아웃

```
~/.rune/
├── config.json              # 메인 config (사용자 + 데몬 공용)
├── sock                     # Unix domain socket (데몬이 생성, 0600)
├── keys/
│   ├── EncKey.json          # FHE 공개키 (Vault에서 캐시)
│   └── EvalKey.json         # FHE 평가키 (Vault에서 캐시, 수십 MB)
├── logs/
│   └── daemon.log           # 데몬 로그 (rotating)
├── capture_log.jsonl        # 캡처 감사 로그 (append-only)
├── certs/
│   └── ca.pem               # self-signed CA 인증서 (선택적)
└── review_queue.json        # legacy (MVP에서 소비자 없음, 건드리지 않음)
```

---

## 10. Capture 입력 검증 및 전처리

agent-delegated (KEEP) 경로에서 `_capture_single`이 수행하는 입력 정규화.
(server.py:1244-1324)

### 10.1 Tier 2 에이전트 거부 게이트

`extracted.tier2.capture == false` → 즉시 `{"ok": true, "captured": false}` 반환.
에이전트가 significance를 자체 판단하고 거부할 수 있다.
`tier2.reason` 필드가 있으면 `reason` 응답에 포함된다.

```python
tier2 = data.get("tier2", {})
if not tier2.get("capture", True):
    return {"ok": True, "captured": False,
            "reason": f"Agent rejected: {tier2.get('reason', 'no reason')}"}
```

### 10.2 Phase 배열 제한

`phases_data[:7]` — 최대 **7개** phase만 처리. 초과분은 무시.

### 10.3 Title 절삭

`title[:60]` — 모든 title/phase_title은 **60자** 초과 시 절삭.
group_title, single title, phase_title 모두 동일 규칙 적용.

### 10.4 Confidence 클램핑

```python
if isinstance(agent_confidence, (int, float)):
    agent_confidence = max(0.0, min(1.0, float(agent_confidence)))
else:
    agent_confidence = None  # → 이후 0.0으로 대체
```

숫자가 아닌 값(문자열, None 등)은 `None`으로 설정되고, detection 생성 시 `0.0`으로 변환.

### 10.5 1-element phase 배열 → single 레코드

`phases`가 정확히 **1개**이면 `phase_chain`이 아니라 **single** `ExtractedFields`로 취급.
2개 이상이면 `ExtractionResult.phases` (multi-phase), 0개 또는 없으면 flat fields에서 직접 구성.

### 10.6 reusable_insight 와이어링

agent JSON의 `reusable_insight` 또는 `group_summary` → `DecisionRecord.reusable_insight`로 매핑:

1. **Multi-phase**: `ExtractionResult.group_summary = data["reusable_insight"] or data["group_title"]`
2. **Single**: `ExtractionResult.group_summary = data["reusable_insight"] or ""`
3. **RecordBuilder**: `record.reusable_insight = pre_extraction.group_summary` (record_builder.py:313-315)
4. **임베딩 텍스트 선택**: `reusable_insight > payload.text` 우선 (embedding.py:27-30)

---

## 11. Record ID 생성 규칙

`generate_record_id` / `generate_group_id` (decision_record.py:245-259)

### 11.1 ID 포맷

| 종류 | 포맷 | 예시 |
|---|---|---|
| Record ID | `dec_YYYY-MM-DD_domain_slug` | `dec_2026-04-16_engineering_postgresql_선택_이유` |
| Group ID | `grp_YYYY-MM-DD_domain_slug` | `grp_2026-04-16_engineering_인프라_마이그레이션_결정` |

### 11.2 Slug 생성

title의 앞 3 단어를 소문자로 변환 후 `_`로 연결. 각 단어는 `isalnum()` 또는
`replace("_","").isalnum()` 통과 시에만 포함.

### 11.3 Multi-record suffix

| 그룹 타입 | suffix 패턴 | 예시 |
|---|---|---|
| `phase_chain` | `_p{seq}` | `dec_2026-04-16_engineering_배포_전략_결정_p0`, `..._p1` |
| `bundle` | `_b{seq}` | `dec_2026-04-16_engineering_아키텍처_리뷰_결과_b0`, `..._b1` |

suffix는 `generate_record_id()` 결과에 **후위 연결**된다 (record_builder.py:338-339).
즉 `generate_record_id()` 자체는 base만 생성하고, 멀티 레코드(phase_chain/bundle)인
경우에만 `record_builder._build_multi_record_from_extraction`가 suffix를 붙임
(`suffix = f"_b{seq}" if group_type == "bundle" else f"_p{seq}"`). 2026-04-17 실측 확정.

---

## 12. Confidence 계산 (Recall)

`_calculate_confidence()` (server.py:393-412) — LLM 없이 순수 수학으로 계산.

### 12.1 공식

상위 **5개** 결과에 대해:

```
position_weight = 1.0 / (i + 1)          # i=0 → 1.0, i=1 → 0.5, ...
certainty_weight:
    "supported"           → 1.0
    "partially_supported" → 0.6
    "unknown"             → 0.3
combined = position_weight × certainty_weight × score
```

최종 confidence:

```
total = sum(combined for each result)
confidence = min(1.0, total / 2.0)       # 소수점 2자리 반올림
```

결과가 없으면 `0.0`, `total_weight == 0`이면 `0.0` 반환.

---

## 13. 구조화 에러 응답 포맷

`make_error()` (errors.py:93-118) 기반 공통 응답 구조.

### 13.1 성공 응답

```json
{"ok": true, ...result fields...}
```

### 13.2 에러 응답

```json
{
  "ok": false,
  "error": {
    "code": "VAULT_CONNECTION_ERROR",
    "message": "Vault gRPC 연결 실패",
    "retryable": true,
    "recovery_hint": "Check Vault endpoint and token via /rune:status."
  }
}
```

- `RuneError` 인스턴스: `code`, `message`, `retryable` 포함, `recovery_hint`는 있을 때만 포함
- 일반 `Exception`: `code="INTERNAL_ERROR"`, `retryable=false`로 래핑

### 13.3 batch_capture 상세 응답

```json
{
  "ok": true,
  "total": 3,
  "results": [
    {"index": 0, "title": "DB 선택", "status": "captured", "novelty": "novel"},
    {"index": 1, "title": "캐시 전략", "status": "near_duplicate", "novelty": "near_duplicate"},
    {"index": 2, "title": "", "status": "error", "error": "Insert failed: timeout"}
  ],
  "captured": 1,
  "skipped": 1,
  "errors": 1
}
```

`status` 값: `captured` | `near_duplicate` | `skipped` | `error`.
빈 배열 입력 시 `{"ok": true, "total": 0, "results": [], "captured": 0, "skipped": 0, "errors": 0}`.

---

## 14. 환경변수 전체 목록

server.py의 `__main__` 블록 (1805-1886) 및 `_init_pipelines` (1647-1798) 에서 참조하는
모든 환경변수.

### 14.1 enVector 관련

| 환경변수 | 기본값 | 설명 |
|---|---|---|
| `ENVECTOR_CONFIG` | (없음) | `~/.rune/config.json` 경로. 설정 시 파일에서 vault/envector 설정 로드 |
| `ENVECTOR_ENDPOINT` | `""` | enVector Cloud gRPC endpoint (host:port 또는 URL) |
| `ENVECTOR_ADDRESS` | (없음) | `ENVECTOR_ENDPOINT`의 alias |
| `ENVECTOR_API_KEY` | `None` | enVector Cloud API 키 |
| `ENVECTOR_KEY_ID` | `"vault-key"` | enVector 키 식별자 |
| `ENVECTOR_KEY_PATH` | `<server_dir>/keys` | EncKey.json / EvalKey.json 디렉토리 경로 |
| `ENVECTOR_EVAL_MODE` | `"rmp"` | FHE 평가 모드 (`rmp`, `mm` 등) |
| `ENVECTOR_ENCRYPTED_QUERY` | `"false"` | 쿼리 벡터 암호화 여부 (`true`/`1`/`yes`) |
| `ENVECTOR_AUTO_KEY_SETUP` | `"true"` | Vault에서 자동 키 다운로드 여부 |

### 14.2 Rune-Vault 관련

| 환경변수 | 기본값 | 설명 |
|---|---|---|
| `RUNEVAULT_ENDPOINT` | `None` | Vault 엔드포인트 (tcp://, http://, hostname) |
| `RUNEVAULT_TOKEN` | `None` | Vault 인증 토큰 (`evt_xxx`) |
| `RUNEVAULT_GRPC_TARGET` | (없음) | gRPC 타겟 직접 지정 — endpoint 파싱 로직을 바이패스 |
| `VAULT_CA_CERT` | `None` | TLS CA 인증서 PEM 파일 경로 |
| `VAULT_TLS_DISABLE` | `"false"` | `"true"`면 plaintext gRPC 연결 |

### 14.3 LLM API 키

| 환경변수 | 기본값 | 설명 |
|---|---|---|
| `ANTHROPIC_API_KEY` | `""` | Anthropic Claude API 키 |
| `OPENAI_API_KEY` | `""` | OpenAI API 키 |
| `GOOGLE_API_KEY` | `""` | Google AI API 키 |
| `GEMINI_API_KEY` | (없음) | `GOOGLE_API_KEY`의 fallback alias |

### 14.4 LLM 프로바이더 선택

| 환경변수 | 기본값 | 설명 |
|---|---|---|
| `RUNE_LLM_PROVIDER` | `"anthropic"` | 주 LLM 프로바이더 (`anthropic`/`openai`/`google`/`auto`) |
| `RUNE_TIER2_LLM_PROVIDER` | (LLM_PROVIDER와 동일) | Tier 2 필터 전용 프로바이더 |
| `RUNE_AUTO_LLM_PROVIDER` | `""` | `auto` 모드에서 클라이언트 오버라이드 없을 때 사용할 프로바이더 |

### 14.5 서버 관련

| 환경변수 | 기본값 | 설명 |
|---|---|---|
| `MCP_SERVER_NAME` | `"envector_mcp_server"` | MCP 서버 advertised name |
| `SETUP_ONLY` | (없음) | `"1"` 설정 시 bootstrap-mcp.sh가 의존성 설치만 하고 서버 실행 없이 종료 |

---

## 15. 보안

### 15.1 로그 민감데이터 마스킹

`_SensitiveFilter` (server.py:25-43) — Python `logging.Filter`로 모든 로그 메시지에 적용.

**감지 패턴 2종**:

| 패턴 | 매칭 대상 |
|---|---|
| `(sk-\|pk-\|api_\|envector_\|evt_)[a-zA-Z0-9_-]{10,}` | 접두사 기반 시크릿 (API 키, Vault 토큰 등) |
| `(token\|key\|secret\|password)["\s:=]+[a-zA-Z0-9_-]{20,}` | 키-값 패턴의 시크릿 (대소문자 무시) |

**마스킹 방식**: 매칭된 문자열의 앞 **8자**를 유지하고 나머지를 `***`으로 치환.
예: `sk-proj-abcdef123456789` → `sk-proj-***`

### 15.2 PII 자동 삭제

`RecordBuilder.SENSITIVE_PATTERNS` (record_builder.py:89-95) — 캡처 시 레코드 텍스트에서
PII를 자동 치환하는 5종 regex.

| # | 패턴 | 치환 | 대상 |
|---|---|---|---|
| 1 | `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z\|a-z]{2,}\b` | `[EMAIL]` | 이메일 주소 |
| 2 | `\b\d{3}[-.]?\d{3}[-.]?\d{4}\b` | `[PHONE]` | 전화번호 (미국식 10자리) |
| 3 | `\b(?:sk\|pk\|api\|key\|token\|secret\|password)[_-][a-zA-Z0-9_-]{15,}\b` | `[API_KEY]` | 접두사 기반 API 키 (15자 이상) |
| 4 | `\b[A-Za-z0-9]{32,}\b` | `[API_KEY]` | 긴 영숫자 토큰 (32자 이상) |
| 5 | `\b[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4}\b` | `[CARD]` | 신용카드 번호 (16자리) |
