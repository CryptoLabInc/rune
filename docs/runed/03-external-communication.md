# 03. 외부 통신 상세

runed가 수행하는 모든 외부 네트워크 통신을 구현 가능한 수준으로 기술한다.
대상은 두 가지: **Rune-Vault** (키 관리 + 복호화)와 **enVector Cloud** (암호화 벡터 DB).

---

## 1. 외부 통신 전체 지도

```
runed
├── Unix socket (localhost only, ~/.rune/sock)     ← CLI/MCP shim 연결
├── gRPC → Rune-Vault (tcp://vault:50051)          ← 키 관리 + 복호화
│   ├── GetPublicKey        (키 번들 다운로드)
│   ├── DecryptScores       (FHE 스코어 복호화)
│   └── DecryptMetadata     (AES 메타데이터 복호화)
└── gRPC → enVector Cloud (via Go SDK)             ← 암호화 벡터 DB
    ├── Score               (recall: FHE 유사도 검색)
    ├── GetMetadata         (recall: 암호화된 메타데이터 조회)
    ├── Insert              (capture: FHE 암호화 벡터 + AES 메타데이터 삽입)
    └── GetIndexList        (diagnostics: 인덱스 목록)
```

**보안 불변식**: runed는 절대로 FHE 비밀키(SecKey)를 보유하지 않는다.
모든 복호화는 Vault에 위임한다. runed가 가진 키는 FHE 공개키(EncKey),
FHE 평가키(EvalKey), 그리고 per-agent AES-256 DEK뿐이다.

---

## 2. Vault gRPC 통신 상세

### 2.1 Proto 정의

패키지: `rune.vault.v1`. 파일: `vault_service.proto`.

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
  string key_bundle_json = 1;  // JSON 문자열: 2.2절 참고
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

### 2.2 GetPublicKey -- 키 번들 다운로드

**호출 시점**:
- 데몬 startup 시 1회
- config reload 시 (fsnotify가 `~/.rune/config.json` 변경 감지)
- `rune:activate` 커맨드로 dormant → active 전환 시

**요청**:
- `token`: `~/.rune/config.json`의 `vault.token` 필드 (형식: `evt_xxx`)

**응답**: `key_bundle_json`은 JSON 문자열이다. `json.Unmarshal` 후 아래 필드를 추출:

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

**구현 시 처리 순서**:

| 필드 | 저장 위치 | 용도 |
|---|---|---|
| `EncKey.json` | `~/.rune/keys/EncKey.json` (디스크 캐시 + in-memory) | enVector SDK의 FHE 암호화 입력 |
| `EvalKey.json` | `~/.rune/keys/EvalKey.json` (디스크 캐시 — 수십 MB) | enVector SDK의 FHE 평가 연산 입력 |
| `index_name` | in-memory | 모든 SDK 호출(score/insert/remind)의 필수 파라미터 |
| `key_id` | in-memory | enVector SDK 초기화 파라미터 |
| `agent_id` | in-memory | AES 메타데이터 암호화 시 봉투의 `"a"` 필드 |
| `agent_dek` | in-memory (base64 디코딩 후 `[]byte`, 32바이트) | AES-256-CTR 메타데이터 암호화 키 |
| `envector_endpoint` | `~/.rune/config.json`의 `envector.endpoint`에 기록 | enVector Go SDK gRPC 대상 |
| `envector_api_key` | `~/.rune/config.json`의 `envector.api_key`에 기록 | enVector Go SDK 인증 토큰 |

**Go pseudocode**:

```go
type KeyBundle struct {
    EncKeyJSON       string `json:"EncKey.json"`
    EvalKeyJSON      string `json:"EvalKey.json"`
    IndexName        string `json:"index_name"`
    KeyID            string `json:"key_id"`
    AgentID          string `json:"agent_id"`
    AgentDEK         string `json:"agent_dek"`           // base64-encoded
    EnvectorEndpoint string `json:"envector_endpoint"`
    EnvectorAPIKey   string `json:"envector_api_key"`
}

func (c *vaultClient) GetPublicKey(ctx context.Context) (*KeyBundle, error) {
    req := &pb.GetPublicKeyRequest{Token: c.token}
    resp, err := c.stub.GetPublicKey(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("gRPC GetPublicKey: %w", err)
    }
    if resp.Error != "" {
        return nil, fmt.Errorf("GetPublicKey failed: %s", resp.Error)
    }

    var bundle KeyBundle
    if err := json.Unmarshal([]byte(resp.KeyBundleJson), &bundle); err != nil {
        return nil, fmt.Errorf("GetPublicKey invalid JSON: %w", err)
    }

    // 키 파일 디스크 캐시
    keysDir := filepath.Join(os.Getenv("HOME"), ".rune", "keys")
    os.MkdirAll(keysDir, 0700)
    os.WriteFile(filepath.Join(keysDir, "EncKey.json"), []byte(bundle.EncKeyJSON), 0600)
    os.WriteFile(filepath.Join(keysDir, "EvalKey.json"), []byte(bundle.EvalKeyJSON), 0600)

    return &bundle, nil
}
```

### 2.3 DecryptScores -- FHE 스코어 복호화

recall 경로의 핵심 단계. enVector Cloud에서 돌아온 FHE ciphertext를
Vault가 비밀키로 복호화하여 plaintext top-k 점수를 반환한다.

**플로우**:

```
runed (recall)
  │
  ├── 1. embed(query) → []float32 (평문 벡터)
  │
  ├── 2. enVectorSDK.Score(indexName, queryVec)
  │      → []string (FHE ciphertext blob, 각각 base64 인코딩)
  │      blob은 CipherBlock.data protobuf의 SerializeToString() 결과를
  │      base64 인코딩한 것. runed에게는 opaque 바이트.
  │
  ├── 3. vault.DecryptScores(blobB64, topK=5)
  │      요청: token + encrypted_blob_b64 + top_k
  │      Vault 내부에서:
  │        FHE SecKey로 ciphertext 복호화 → 전체 유사도 점수 배열
  │        → top_k개를 선택하여 (shard_idx, row_idx, score) 반환
  │      응답: []ScoreEntry
  │        score는 0.0~1.0 cosine similarity (이제 plaintext)
  │
  └── 4. 이 {shard_idx, row_idx} 쌍으로 GetMetadata 호출
```

**Go pseudocode**:

```go
func (c *vaultClient) DecryptScores(
    ctx context.Context,
    blobB64 string,
    topK int,
) ([]ScoreEntry, error) {
    req := &pb.DecryptScoresRequest{
        Token:            c.token,
        EncryptedBlobB64: blobB64,
        TopK:             int32(topK),
    }
    resp, err := c.stub.DecryptScores(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("gRPC DecryptScores: %w", err)
    }
    if resp.Error != "" {
        return nil, fmt.Errorf("DecryptScores: %s", resp.Error)
    }

    entries := make([]ScoreEntry, len(resp.Results))
    for i, r := range resp.Results {
        entries[i] = ScoreEntry{
            ShardIdx: r.ShardIdx,
            RowIdx:   r.RowIdx,
            Score:    r.Score,
        }
    }
    return entries, nil
}
```

**주의사항**:
- `encrypted_blob_b64`는 enVector SDK가 반환한 blob 중 하나를 그대로 전달.
  현재 Python에서는 `scores` 리스트에서 각 CipherBlock마다 별도의 blob을
  만들지만, Vault는 단일 blob을 받아 내부에서 모든 행의 점수를 복호화한 후
  top-k를 선별한다.
- `top_k`의 최대값은 10.

### 2.4 DecryptMetadata -- AES 메타데이터 복호화

DecryptScores 결과의 `{shard_idx, row_idx}` 쌍으로 enVector에서 가져온
AES ciphertext 메타데이터를 Vault가 복호화한다.

**플로우**:

```
runed (recall, 이어서)
  │
  ├── 5. enVectorSDK.GetMetadata(indexName, [{shard_idx, row_idx}, ...])
  │      → []string (각 문자열은 AES 암호화된 메타데이터)
  │
  │      각 문자열의 형태:
  │      {"a":"agent_xyz","c":"<base64 AES-256-CTR ciphertext>"}
  │      "a" = agent_id (어떤 에이전트가 암호화했는지 식별)
  │      "c" = base64 인코딩된 AES ciphertext
  │
  ├── 6. vault.DecryptMetadata(encryptedList)
  │      요청: token + encrypted_metadata_list (위의 문자열 배열)
  │      Vault 내부에서:
  │        "a" 필드로 agent 식별 → 해당 agent의 DEK 조회
  │        → "c" 필드를 base64 디코딩 → AES-256-CTR 복호화
  │      응답: []string (각각 plaintext JSON)
  │        → 이제 DecisionRecord의 원본 JSON을 가짐
  │
  └── 7. json.Unmarshal → Go struct로 사용
```

**Go pseudocode**:

```go
func (c *vaultClient) DecryptMetadata(
    ctx context.Context,
    encrypted []string,
) ([]string, error) {
    req := &pb.DecryptMetadataRequest{
        Token:                 c.token,
        EncryptedMetadataList: encrypted,
    }
    resp, err := c.stub.DecryptMetadata(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("gRPC DecryptMetadata: %w", err)
    }
    if resp.Error != "" {
        return nil, fmt.Errorf("DecryptMetadata: %s", resp.Error)
    }

    return resp.DecryptedMetadata, nil  // 각 원소가 plaintext JSON 문자열
}
```

**주의사항**:
- 응답의 각 문자열은 유효한 JSON이어야 한다. 파싱 실패 시 해당 레코드를
  skip하되 전체 호출을 실패시키지 않는 방식이 권장된다 (Python 현재 구현은
  전체 실패).
- 배열 순서는 요청의 `encrypted_metadata_list` 순서와 1:1 대응한다.

### 2.5 TLS 3모드

| config 값 | Go 구현 | 사용 케이스 |
|---|---|---|
| `tls_disable: true` | `grpc.WithTransportCredentials(insecure.NewCredentials())` | 로컬 개발 (`localhost:50051`) |
| `ca_cert: "/path/to/ca.pem"` | PEM 파일 읽기 → `tls.Config{RootCAs: pool}` → `credentials.NewTLS(&tlsCfg)` | self-hosted Vault (자체 CA) |
| `ca_cert: ""`, `tls_disable: false` | 시스템 CA 풀 사용 (Go 기본값 — `credentials.NewTLS(&tls.Config{})`) | 공인 인증서 사용 Vault |

```go
func buildTransportCredentials(cfg TLSConfig) (grpc.DialOption, error) {
    if cfg.Disable {
        log.Warn("TLS disabled - gRPC traffic is unencrypted. Local dev only.")
        return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
    }

    tlsCfg := &tls.Config{}
    if cfg.CACert != "" {
        pem, err := os.ReadFile(cfg.CACert)
        if err != nil {
            return nil, fmt.Errorf("CA cert read: %w", err)
        }
        pool := x509.NewCertPool()
        if !pool.AppendCertsFromPEM(pem) {
            return nil, fmt.Errorf("CA cert parse failed: %s", cfg.CACert)
        }
        tlsCfg.RootCAs = pool
        log.Info("Using custom CA certificate", "path", cfg.CACert)
    } else {
        log.Info("Using system CA bundle for TLS verification")
    }
    return grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)), nil
}
```

### 2.6 gRPC 옵션

```go
opts := []grpc.DialOption{
    grpc.WithDefaultCallOptions(
        grpc.MaxCallRecvMsgSize(256 * 1024 * 1024), // 256 MB (EvalKey가 수십 MB)
        grpc.MaxCallSendMsgSize(256 * 1024 * 1024),
    ),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                30 * time.Second,   // 30초 간격으로 ping
        Timeout:             10 * time.Second,   // ping 후 10초 이내 응답 없으면 dead
        PermitWithoutStream: true,               // 활성 스트림 없어도 keepalive 유지
    }),
}
```

**256 MB인 이유**: GetPublicKey 응답의 `EvalKey.json`이 수십 MB에 달할 수
있다. gRPC의 기본 최대 메시지 크기(4 MB)로는 수신 불가능.

**keepalive 설정**: Python 현재 구현에는 없지만 Go 전환 시 추가.
데몬이 장기간 실행되므로 idle connection이 NAT/firewall에 의해 끊기는 것을 방지.

### 2.7 Endpoint 파싱

다양한 형식의 endpoint 문자열을 gRPC target (`host:port`)으로 정규화한다.

| 입력 | 파싱 결과 | 설명 |
|---|---|---|
| `"vault:50051"` | `"vault:50051"` | 직접 사용 |
| `"tcp://vault:50051"` | `"vault:50051"` | `tcp://` 스킴 제거 |
| `"http://vault:50080/mcp"` | `"vault:50051"` | legacy HTTP, 포트를 50051로 교체 |
| `"https://vault.example.com"` | `"vault.example.com:50051"` | 포트 추가 |
| `"bare-hostname"` | `"bare-hostname:50051"` | 기본 포트 추가 |

**환경변수 오버라이드**: `RUNEVAULT_GRPC_TARGET`이 설정되어 있으면
위의 모든 파싱 로직을 무시하고 해당 값을 직접 gRPC target으로 사용한다.

```go
func deriveGRPCTarget(endpoint string) string {
    // 환경변수 오버라이드 확인
    if target := os.Getenv("RUNEVAULT_GRPC_TARGET"); target != "" {
        return target
    }

    u, err := url.Parse(endpoint)
    if err != nil || u.Scheme == "" || u.Scheme == "tcp" {
        // "host:port" 또는 "tcp://host:port" 형태
        host := u.Hostname()
        if host == "" {
            host = strings.Split(strings.Split(endpoint, ":")[0], "/")[0]
        }
        if port := u.Port(); port != "" {
            return host + ":" + port
        }
        return host + ":50051"
    }

    // http:// 또는 https:// (legacy)
    host := u.Hostname()
    return host + ":50051"
}
```

### 2.8 Health Check

**Primary**: gRPC health check 프로토콜 (`grpc.health.v1.Health/Check`)

```go
import "google.golang.org/grpc/health/grpc_health_v1"

func (c *vaultClient) Health(ctx context.Context) bool {
    // 1차: gRPC health
    healthClient := grpc_health_v1.NewHealthClient(c.conn)
    resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
        Service: "",  // 빈 문자열 = 서버 전체 상태
    })
    if err == nil && resp.Status == grpc_health_v1.HealthCheckResponse_SERVING {
        return true
    }

    // 2차: HTTP /health fallback (endpoint가 http:// 인 경우만)
    if isHTTPEndpoint(c.originalEndpoint) {
        baseURL := stripPathSuffix(c.originalEndpoint)
        httpResp, err := http.Get(baseURL + "/health")
        if err == nil && httpResp.StatusCode == 200 {
            return true
        }
    }

    return false
}
```

**Fallback 조건**: `originalEndpoint`가 `http://` 또는 `https://` 스킴인
경우에만 HTTP fallback을 시도한다. 순수 gRPC endpoint (`host:port`,
`tcp://host:port`)에서는 gRPC health만 사용.

**HTTP fallback 시 경로 정리**: endpoint가 `/mcp` 또는 `/sse`로 끝나면
해당 suffix를 제거한 후 `/health`를 붙인다.

### 2.9 에러 처리

gRPC 에러를 `RuneError`로 매핑하고 retryable 여부를 판단한다.

| gRPC Status Code | retryable | 의미 | recovery_hint |
|---|---|---|---|
| `UNAVAILABLE` | true | 서버 도달 불가 | "Vault 서버 상태 확인" |
| `DEADLINE_EXCEEDED` | true | 타임아웃 | "네트워크 상태 확인 또는 timeout 증가" |
| `UNAUTHENTICATED` | false | 토큰 무효 | "vault.token 확인 (evt_xxx)" |
| `PERMISSION_DENIED` | false | 권한 부족 | "Vault 접근 권한 확인" |
| `INTERNAL` | false | 서버 내부 오류 | "Vault 서버 로그 확인" |
| `INVALID_ARGUMENT` | false | 잘못된 입력 | (요청 파라미터 점검) |

```go
type RuneError struct {
    Code         string // "vault_unavailable", "vault_auth_failed", 등
    Message      string
    Retryable    bool
    RecoveryHint string
}

func mapGRPCError(err error) *RuneError {
    st, ok := status.FromError(err)
    if !ok {
        return &RuneError{
            Code: "vault_unknown", Message: err.Error(),
            Retryable: false,
        }
    }
    switch st.Code() {
    case codes.Unavailable:
        return &RuneError{
            Code: "vault_unavailable", Message: st.Message(),
            Retryable: true, RecoveryHint: "Vault 서버 상태 확인",
        }
    case codes.DeadlineExceeded:
        return &RuneError{
            Code: "vault_timeout", Message: st.Message(),
            Retryable: true, RecoveryHint: "네트워크 상태 확인",
        }
    case codes.Unauthenticated:
        return &RuneError{
            Code: "vault_auth_failed", Message: st.Message(),
            Retryable: false, RecoveryHint: "vault.token 확인",
        }
    default:
        return &RuneError{
            Code: "vault_error", Message: st.Message(),
            Retryable: false,
        }
    }
}
```

**재시도 정책**: retryable=true인 에러에 대해 최대 2회 재시도,
exponential backoff (1s, 2s). 3회 실패 시 에러를 상위로 전파.

### 2.10 Go 인터페이스

```go
package vault

// VaultClient는 Rune-Vault gRPC 서비스의 Go 인터페이스다.
// 테스트 시 mock으로 교체 가능하도록 interface로 정의.
type VaultClient interface {
    // GetPublicKey는 Vault에서 FHE 키 번들을 다운로드한다.
    // 데몬 startup 및 config reload 시 호출.
    GetPublicKey(ctx context.Context) (*KeyBundle, error)

    // DecryptScores는 FHE 스코어 ciphertext를 복호화하여
    // top-k plaintext 점수를 반환한다.
    DecryptScores(ctx context.Context, blobB64 string, topK int) ([]ScoreEntry, error)

    // DecryptMetadata는 AES 암호화된 metadata를 복호화하여
    // plaintext JSON 문자열 리스트를 반환한다.
    DecryptMetadata(ctx context.Context, encrypted []string) ([]string, error)

    // Health는 Vault 도달 가능 여부를 반환한다.
    Health(ctx context.Context) bool

    // Close는 gRPC 채널을 닫는다.
    Close() error
}

// 구현체 struct
type vaultClient struct {
    conn             *grpc.ClientConn
    stub             pb.VaultServiceClient
    token            string
    tlsCfg           TLSConfig
    originalEndpoint string       // HTTP fallback 판단용
    timeout          time.Duration
}

type TLSConfig struct {
    Disable bool   // true면 insecure plaintext
    CACert  string // PEM 파일 경로. 비어있으면 시스템 CA
}

type ScoreEntry struct {
    ShardIdx int32   `json:"shard_idx"`
    RowIdx   int32   `json:"row_idx"`
    Score    float64 `json:"score"`
}
```

---

## 3. enVector Go SDK 통신 상세

### 3.1 SDK는 팀원이 개발 (외부 의존성)

runed는 enVector Go SDK를 `import`해서 사용한다. SDK가 내부적으로
enVector Cloud의 gRPC 채널을 관리하므로, runed는 gRPC 연결 상세를
직접 다루지 않는다.

SDK가 담당하는 것:
- enVector Cloud gRPC 연결 관리 (dial, 채널 유지, reconnect)
- EncKey/EvalKey 로드 및 FHE 암호화 (C++ 코어 via CGO)
- CipherBlock protobuf 직렬화/역직렬화

runed가 담당하는 것:
- SDK에 넘길 config 값 준비 (endpoint, API key, key path 등 -- 전부 Vault 키 번들에서 추출)
- SDK 초기화 및 재초기화 호출
- SDK 반환값을 Vault로 전달하는 중개

### 3.2 SDK 인터페이스 계약

```go
package envector

// EnVectorClient는 enVector Go SDK의 인터페이스 계약이다.
// runed가 소비하는 쪽이고, SDK 팀이 구현한다.
type EnVectorClient interface {
    // Init은 SDK를 초기화한다. enVector Cloud에 gRPC 연결을 수립하고,
    // keyPath에서 EncKey/EvalKey를 로드한다.
    Init(cfg Config) error

    // Score는 평문 쿼리 벡터로 암호화 유사도 검색을 수행한다.
    // 반환값: FHE ciphertext blob의 base64 문자열 배열.
    // 이 blob은 runed에게 opaque — Vault.DecryptScores에 그대로 전달.
    Score(ctx context.Context, indexName string, query []float32) ([]string, error)

    // GetMetadata는 shard_idx/row_idx 쌍으로 암호화된 메타데이터를 조회한다.
    // 반환값: AES ciphertext 문자열 배열 (각각 {"a":"...","c":"..."} 형태).
    // Vault.DecryptMetadata에 그대로 전달.
    GetMetadata(ctx context.Context, indexName string, indices []Index, fields []string) ([]string, error)

    // Insert는 평문 벡터를 FHE 암호화 후 enVector에 삽입한다.
    // metadata는 이미 AES 암호화된 문자열 배열 (runed의 capture.go에서 암호화).
    // 반환값: 삽입된 벡터 ID 문자열 배열.
    Insert(ctx context.Context, indexName string, vectors [][]float32, metadata []string) ([]string, error)

    // GetIndexList는 사용 가능한 인덱스 목록을 반환한다.
    GetIndexList(ctx context.Context) ([]string, error)

    // Reinit는 연결을 재설정한다 (sleep/wake 복구용).
    Reinit() error

    // Close는 gRPC 채널을 닫는다.
    Close() error
}

type Config struct {
    Address      string // enVector Cloud gRPC endpoint (예: "redcourage-xxx.clusters.envector.io")
    KeyPath      string // 키 파일 디렉토리 (예: "~/.rune/keys/")
    KeyID        string // 키 식별자 (예: "key_abc123")
    EvalMode     string // "rmp"
    AccessToken  string // enVector API key
    AutoKeySetup bool   // false (키는 Vault에서 제공)
}

type Index struct {
    ShardIdx int32 `json:"shard_idx"`
    RowIdx   int32 `json:"row_idx"`
}
```

### 3.3 Score 연산 (recall 경로)

**입력**: `indexName` (string) + `query` ([]float32, 평문 벡터)

**내부 동작** (SDK가 처리):
1. `query_encryption=false`이므로 쿼리 벡터는 평문 `[]float32` 그대로 enVector Cloud에 전송
2. enVector Cloud가 FHE 동형 연산으로 유사도 검색 수행
3. 결과는 `CipherBlock` (FHE 암호화된 스코어) 배열로 반환
4. SDK가 각 CipherBlock의 protobuf 데이터를 `SerializeToString()` → base64 인코딩

**출력**: `[]string` -- 각 원소는 base64 인코딩된 FHE ciphertext blob

**runed의 역할**: 이 blob을 그대로 `vault.DecryptScores(blobB64, topK)`에 전달.
FHE 수학은 runed에서 전혀 수행하지 않는다. **CGO 불필요 경로**.

```
runed              enVector SDK             enVector Cloud
  │                     │                         │
  │ Score(idx, vec)     │                         │
  ├────────────────────>│                         │
  │                     │  gRPC: scoring(vec)     │
  │                     ├────────────────────────>│
  │                     │                         │ FHE 동형 연산
  │                     │  []CipherBlock          │
  │                     │<────────────────────────┤
  │                     │                         │
  │                     │ serialize + base64      │
  │ []string (blobs)    │                         │
  │<────────────────────┤                         │
```

### 3.4 GetMetadata 연산 (recall 경로)

**입력**: `indexName` (string) + `indices` ([]Index -- DecryptScores 결과의 shard_idx/row_idx 쌍) + `fields` ([]string, 보통 `["metadata"]`)

**내부 동작** (SDK가 처리):
1. enVector Cloud에 gRPC로 메타데이터 조회 요청
2. 반환된 protobuf Metadata 객체에서 `metadata` 필드 추출

**출력**: `[]string` -- 각 원소는 AES 암호화된 메타데이터 문자열

각 문자열의 형태:
```json
{"a":"agent_xyz","c":"<base64 AES-256-CTR ciphertext>"}
```

**runed의 역할**: 이 문자열 배열을 그대로 `vault.DecryptMetadata(encrypted)`에 전달.
**CGO 불필요 경로**.

```
runed              enVector SDK             enVector Cloud
  │                     │                         │
  │ GetMetadata(...)    │                         │
  ├────────────────────>│                         │
  │                     │  gRPC: get_metadata     │
  │                     ├────────────────────────>│
  │                     │  []Metadata protobuf    │
  │                     │<────────────────────────┤
  │                     │ extract "metadata" field│
  │ []string (AES ct)   │                         │
  │<────────────────────┤                         │
```

### 3.5 Insert 연산 (capture 경로)

이 경로에서 FHE 암호화가 발생한다. **CGO가 필요한 유일한 경로**.

**입력**:
- `indexName` (string)
- `vectors` ([][]float32 -- 평문 임베딩 벡터)
- `metadata` ([]string -- **이미 AES 암호화된** JSON 문자열)

**내부 동작** (SDK가 처리):
1. `vectors` (평문 `[]float32`) → EncKey로 FHE 암호화 → `CipherBlock` 생성
2. `CipherBlock` + metadata를 enVector Cloud gRPC로 전송
3. metadata는 SDK가 건드리지 않고 그대로 pass-through

**출력**: `[]string` -- 삽입된 벡터의 ID

**중요**: metadata에 대한 AES 암호화는 runed의 `capture.go`에서 수행한다
(6절 참고). SDK는 metadata를 암호화하지 않는다 -- 받은 문자열을
그대로 enVector Cloud에 전달할 뿐이다.

```
runed (capture.go)       enVector SDK             enVector Cloud
  │                          │                         │
  │ AES encrypt metadata     │                         │
  │ (agent_dek 사용)         │                         │
  │                          │                         │
  │ Insert(idx, vecs, meta)  │                         │
  ├─────────────────────────>│                         │
  │                          │ FHE encrypt vectors     │
  │                          │ (EncKey, CGO/C++ core)  │
  │                          │                         │
  │                          │ gRPC: insert            │
  │                          │ (CipherBlocks + meta)   │
  │                          ├────────────────────────>│
  │                          │ []vectorID              │
  │                          │<────────────────────────┤
  │ []string (IDs)           │                         │
  │<─────────────────────────┤                         │
```

### 3.6 연결 복구

SDK는 내부적으로 연결 복구 로직을 구현해야 한다. 현재 Python의
`_with_reconnect` 패턴과 동일한 전략:

**패턴**: 1회 시도 → 연결 에러 감지 → `Reinit()` → 1회 재시도

```go
// SDK 내부 구현 (참고)
func (c *client) withReconnect(fn func() error) error {
    err := fn()
    if err == nil {
        return nil
    }
    if !isConnectionError(err) {
        return err  // 연결 에러가 아니면 그대로 반환
    }
    log.Warn("enVector connection lost, reconnecting...", "err", err)
    if reinitErr := c.reinit(); reinitErr != nil {
        return fmt.Errorf("reconnect failed: %w", reinitErr)
    }
    return fn()  // 1회 재시도
}
```

**연결 끊김 판단 패턴** (에러 메시지에 포함 여부):

```go
var connectionErrorPatterns = []string{
    "UNAVAILABLE",
    "DEADLINE_EXCEEDED",
    "Connection refused",
    "Connection reset",
    "Stream removed",
    "RST_STREAM",
    "Broken pipe",
    "Transport closed",
    "Socket closed",
    "EOF",
    "failed to connect",
}
```

**sleep/wake 복구**: runed가 macOS sleep/wake 이벤트를 감지하면
(02-installation-and-lifecycle.md 참고) `sdk.Reinit()`을 호출하여
enVector Cloud gRPC 연결을 재설정한다.

---

## 4. 두 외부 서비스의 호출 순서 (recall 전체 예시)

하나의 recall 연산에서 Vault와 enVector가 교차로 호출되는 전체 흐름.
멀티쿼리 검색 시 이 흐름이 최대 3회 반복된다 (expanded_queries 상위 3개).

```
CLI/MCP        runed               Embedder         enVector SDK        Vault gRPC
  │              │                     │                 │                   │
  │ recall(q)    │                     │                 │                   │
  ├─────────────>│                     │                 │                   │
  │              │                     │                 │                   │
  │              │ embed(query)        │                 │                   │
  │              ├────────────────────>│                 │                   │
  │              │ []float32           │                 │                   │
  │              │<────────────────────┤                 │                   │
  │              │                     │                 │                   │
  │              │ Score(idx, vec)     │                 │                   │
  │              ├───────────────────────────────────────>│                  │
  │              │ []string (FHE blob b64)               │                  │
  │              │<──────────────────────────────────────┤                   │
  │              │                     │                 │                   │
  │              │ DecryptScores(blob, topK=5)           │                   │
  │              ├──────────────────────────────────────────────────────────>│
  │              │ []ScoreEntry{shard, row, score}                           │
  │              │<─────────────────────────────────────────────────────────┤
  │              │                     │                 │                   │
  │              │ GetMetadata(idx, [{shard,row}...])    │                   │
  │              ├───────────────────────────────────────>│                  │
  │              │ []string (AES ct, {"a":"..","c":"..})  │                  │
  │              │<──────────────────────────────────────┤                   │
  │              │                     │                 │                   │
  │              │ DecryptMetadata(encrypted_list)       │                   │
  │              ├──────────────────────────────────────────────────────────>│
  │              │ []string (plaintext JSON)                                 │
  │              │<─────────────────────────────────────────────────────────┤
  │              │                     │                 │                   │
  │              │ json.Unmarshal → []DecisionRecord     │                   │
  │              │ dedup + rerank + filter               │                   │
  │              │                     │                 │                   │
  │  결과 응답    │                     │                 │                   │
  │<─────────────┤                     │                 │                   │
```

**각 화살표의 데이터 타입**:

| 단계 | 출발 | 도착 | 데이터 타입 |
|---|---|---|---|
| 1 | CLI/MCP | runed | `string` (자연어 쿼리) |
| 2 | runed | Embedder | `string` (쿼리 텍스트) |
| 3 | Embedder | runed | `[]float32` (임베딩 벡터) |
| 4 | runed | enVector SDK | `string` (indexName) + `[]float32` (쿼리) |
| 5 | enVector SDK | runed | `[]string` (FHE ciphertext blob, base64) |
| 6 | runed | Vault | `string` (blob_b64) + `int` (top_k) |
| 7 | Vault | runed | `[]ScoreEntry` (shard_idx, row_idx, score) |
| 8 | runed | enVector SDK | `string` (indexName) + `[]Index` ({shard, row}) |
| 9 | enVector SDK | runed | `[]string` (AES ciphertext, `{"a":"..","c":".."}`) |
| 10 | runed | Vault | `[]string` (encrypted_metadata_list) |
| 11 | Vault | runed | `[]string` (plaintext JSON) |
| 12 | runed | CLI/MCP | recall 응답 struct (results, confidence, related_queries 등) |

---

## 5. 두 외부 서비스의 호출 순서 (capture 전체 예시)

```
CLI/MCP        runed (capture.go)    Embedder         enVector SDK        Vault gRPC
  │              │                     │                 │                   │
  │ capture(     │                     │                 │                   │
  │   extracted) │                     │                 │                   │
  ├─────────────>│                     │                 │                   │
  │              │                     │                 │                   │
  │              │ build DecisionRecord from extracted   │                   │
  │              │                     │                 │                   │
  │              │ embed(record.embed_text)              │                   │
  │              ├────────────────────>│                 │                   │
  │              │ []float32           │                 │                   │
  │              │<────────────────────┤                 │                   │
  │              │                     │                 │                   │
  │              │ ── novelty check (선택적) ──          │                   │
  │              │ Score(idx, vec)     │                 │                   │
  │              ├───────────────────────────────────────>│                  │
  │              │ []string (FHE blob)                   │                  │
  │              │<──────────────────────────────────────┤                   │
  │              │                     │                 │                   │
  │              │ DecryptScores(blob, topK=1)           │                   │
  │              ├──────────────────────────────────────────────────────────>│
  │              │ []ScoreEntry                                              │
  │              │<─────────────────────────────────────────────────────────┤
  │              │                     │                 │                   │
  │              │ if top_score >= 0.93 → near_duplicate → 저장 안 함        │
  │              │                     │                 │                   │
  │              │ ── AES metadata 암호화 (runed 내부) ──│                   │
  │              │ json.Marshal(record) → plaintext JSON │                   │
  │              │ AES-256-CTR encrypt(plaintext, agent_dek)                 │
  │              │ → base64 ciphertext                   │                   │
  │              │ → envelope: {"a":"agent_id","c":"<ct>"}                   │
  │              │                     │                 │                   │
  │              │ Insert(idx, [vec], [envelope])        │                   │
  │              ├───────────────────────────────────────>│                  │
  │              │                     │                 │ FHE encrypt vec   │
  │              │                     │                 │ (CGO, EncKey)     │
  │              │                     │                 │ gRPC insert       │
  │              │ []string (vectorID)                   │                  │
  │              │<──────────────────────────────────────┤                   │
  │              │                     │                 │                   │
  │              │ capture_log.jsonl에 append            │                   │
  │              │                     │                 │                   │
  │ {ok, record_id}                   │                 │                   │
  │<─────────────┤                     │                 │                   │
```

**각 화살표의 데이터 타입**:

| 단계 | 출발 | 도착 | 데이터 타입 |
|---|---|---|---|
| 1 | CLI/MCP | runed | `CaptureRequest` (text, source, user, channel, extracted JSON) |
| 2 | runed | Embedder | `string` (record.embed_text) |
| 3 | Embedder | runed | `[]float32` (임베딩 벡터) |
| 4 | runed | enVector SDK | `string` (indexName) + `[]float32` (벡터) -- novelty check |
| 5 | enVector SDK | runed | `[]string` (FHE blob) |
| 6 | runed | Vault | `string` (blob_b64) + `int` (topK=1) -- novelty check |
| 7 | Vault | runed | `[]ScoreEntry` -- top_score >= 0.93이면 duplicate |
| 8 | runed 내부 | -- | `string` (AES encrypted envelope JSON) |
| 9 | runed | enVector SDK | `string` (indexName) + `[][]float32` (벡터) + `[]string` (암호화된 metadata) |
| 10 | enVector SDK | runed | `[]string` (vector ID) |
| 11 | runed | CLI/MCP | `CaptureResponse` (ok, record_id, shard, row) |

---

## 6. AES Metadata 암호화 (runed 내부, SDK 아님)

### 6.1 위치와 책임 분리

AES 메타데이터 암호화는 **runed의 `capture.go`에서 직접 수행**한다.
enVector SDK는 이 작업에 관여하지 않는다.

이유: AES metadata 암호화는 애플리케이션 레벨 기능이다. enVector
프로토콜 코어(FHE 벡터 암호화)와 분리해야 SDK가 범용성을 유지한다.

### 6.2 키 출처

- **DEK** (Data Encryption Key): Vault 키 번들의 `agent_dek` 필드
  - base64 디코딩 후 32바이트 (`[]byte`)
  - 데몬 메모리에 보관, 디스크에 저장하지 않음
- **agent_id**: Vault 키 번들의 `agent_id` 필드
  - 암호화 봉투의 `"a"` 필드에 들어감
  - Vault가 복호화 시 어떤 DEK를 사용할지 식별하는 데 사용

### 6.3 암호화 알고리즘

`pyenvector.utils.aes` 소스 코드를 확인한 결과:

- **알고리즘**: AES-256-CTR (Counter mode)
- **키 크기**: 32바이트 (256비트)
- **IV 크기**: 16바이트 (128비트), `crypto/rand`로 생성
- **ciphertext 형식**: `IV (16 bytes) || ciphertext`
  - IV가 ciphertext 앞에 prepend됨
  - base64 인코딩하여 문자열로 변환

**주의**: docstring에는 "AES-GCM"이라고 기술되어 있지만, 실제 구현은
`modes.CTR`을 사용한다 (인증 태그 없음). Go 구현 시 반드시 CTR 모드를
사용해야 기존 데이터와 호환된다.

### 6.4 봉투 포맷

```json
{"a":"agent_xyz","c":"<base64 encoded IV+ciphertext>"}
```

- `"a"` (agent_id): 어떤 에이전트의 DEK로 암호화되었는지 식별
- `"c"` (ciphertext): base64 인코딩된 `IV || ciphertext`

### 6.5 Go 구현

```go
package capture

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
)

const (
    aes256KeySize = 32
    aesCTRIVSize  = 16  // AES block size = 128 bits
)

// MetadataEnvelope는 AES 암호화된 메타데이터의 JSON 봉투.
type MetadataEnvelope struct {
    AgentID    string `json:"a"`
    Ciphertext string `json:"c"`  // base64(IV || ciphertext)
}

// EncryptMetadata는 plaintext를 AES-256-CTR로 암호화하고
// 봉투 JSON 문자열을 반환한다.
func EncryptMetadata(plaintext []byte, agentID string, dek []byte) (string, error) {
    if len(dek) != aes256KeySize {
        return "", fmt.Errorf("DEK must be %d bytes, got %d", aes256KeySize, len(dek))
    }

    block, err := aes.NewCipher(dek)
    if err != nil {
        return "", fmt.Errorf("aes.NewCipher: %w", err)
    }

    // IV 생성 (16바이트, crypto/rand)
    iv := make([]byte, aesCTRIVSize)
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return "", fmt.Errorf("IV generation: %w", err)
    }

    // AES-256-CTR 암호화
    stream := cipher.NewCTR(block, iv)
    ciphertext := make([]byte, len(plaintext))
    stream.XORKeyStream(ciphertext, plaintext)

    // IV || ciphertext → base64
    combined := append(iv, ciphertext...)
    b64 := base64.StdEncoding.EncodeToString(combined)

    // 봉투 JSON 생성
    envelope := MetadataEnvelope{
        AgentID:    agentID,
        Ciphertext: b64,
    }
    envelopeJSON, err := json.Marshal(envelope)
    if err != nil {
        return "", fmt.Errorf("envelope marshal: %w", err)
    }

    return string(envelopeJSON), nil
}
```

### 6.6 capture.go에서의 사용 흐름

```go
func (s *CaptureService) execute(ctx context.Context, req CaptureRequest) (*CaptureResponse, error) {
    // 1. extracted JSON → DecisionRecord 생성
    record := buildRecord(req.Extracted)

    // 2. 임베딩
    vec, err := s.embedder.Embed(ctx, record.EmbedText())
    if err != nil { return nil, err }

    // 3. Novelty check (선택적)
    if s.noveltyEnabled {
        blobs, _ := s.envector.Score(ctx, s.indexName, vec)
        if len(blobs) > 0 {
            entries, _ := s.vault.DecryptScores(ctx, blobs[0], 1)
            if len(entries) > 0 && entries[0].Score >= 0.93 {
                return &CaptureResponse{Ok: true, Duplicate: true}, nil
            }
        }
    }

    // 4. metadata AES 암호화
    recordJSON, _ := json.Marshal(record)
    envelope, err := EncryptMetadata(recordJSON, s.agentID, s.agentDEK)
    if err != nil { return nil, err }

    // 5. enVector Insert (SDK가 vec를 FHE 암호화)
    ids, err := s.envector.Insert(ctx, s.indexName, [][]float32{vec}, []string{envelope})
    if err != nil { return nil, err }

    // 6. capture_log.jsonl에 기록
    s.logCapture(record, ids[0])

    return &CaptureResponse{Ok: true, RecordID: record.ID}, nil
}
```
