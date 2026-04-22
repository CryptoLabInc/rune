# Vault 통합 — gRPC 클라이언트

Rune-Vault는 FHE 키 브로커 + 복호화 서비스. **Python 현재 구조 그대로 유지**. Go 포팅은 `mcp/adapter/vault_client.py` 기능을 `internal/adapters/vault/`로 이식.

## 역할

- rune-mcp에게 FHE 키 번들 공급 (`GetPublicKey`)
- envector에서 받은 ciphertext blob 복호화 (`DecryptScores`, `DecryptMetadata`)
- 세션마다 독립 연결 — 세션 수 = Vault 연결 수

**Vault-delegated 보안 모델의 핵심**: SecKey는 **Vault만** 보유. rune-mcp는 SecKey에 절대 접근하지 않고, 복호화가 필요한 순간 ciphertext를 Vault로 보내 결과만 받는다. 이로써 rune-mcp 프로세스가 손상되어도 SecKey 노출 없음.

## RPC Surface

Python `vault_client.py`가 호출하는 3 RPC 그대로 유지. proto도 기존 것 재사용.

### GetPublicKey

- **입력**: `vault_token` (인증)
- **출력**: 번들
  ```
  {
    EncKey           string,  // FHE 공개키. 로컬 저장 OK
    EvalKey          string,  // FHE 연산키. 로컬 저장 OK
    envector_endpoint string,
    envector_api_key  string,
    agent_id          string,
    agent_dek         bytes(32),  // 반드시 정확히 32바이트 — 길이 검증 필수
    key_id            string,
    index_name        string
  }
  ```
- **호출 시점**:
  - rune-mcp 부팅 직후 (매번)
  - 번들이 메모리에서 invalidate된 경우 (거의 발생 안 함)

### DecryptScores

- **입력**: `token` (vault_token) + `encrypted_blob_b64` (envector `Score` 응답의 ciphertext, base64) + `top_k: int` (default **5**, max 10)
- **출력**: `results: [{shard_idx: int32, row_idx: int32, score: float64}, ...]` (top-k 정렬)
- **호출 시점**: recall · near-duplicate check 때 envector `Score` 직후
- **Python 참조**: `vault_client.py:L217-261`

### DecryptMetadata

- **입력**: `token` (vault_token) + `encrypted_metadata_list: List[str]` (AES envelope 문자열 배열, 각각 `{"a": agent_id, "c": base64(IV||CT)}`)
- **출력**: `decrypted_metadata: List[str]` — **Vault가 AES 복호화까지 수행**한 plaintext **JSON 문자열** 배열. rune-mcp는 각 문자열을 `json.Unmarshal`로 parse만
- **호출 시점**: recall Phase 5에서 AES envelope 포맷으로 분류된 entries에 대해 일괄 호출
- **중요**: Vault가 agent_dek을 내부 보유하고 AES-256-CTR 복호화 수행. rune-mcp는 로컬 복호화 **안 함** (Vault-delegated audit trail). capture 경로에서는 rune-mcp가 직접 암호화하지만 (local agent_dek), recall 복호화는 Vault에 위임
- **Python 참조**: `vault_client.py:L263-299` + proto docstring "Decrypts a list of AES-encrypted metadata strings"

## Endpoint 파싱·정규화

Python `vault_client.py:L116-140` `_derive_grpc_target` 동작을 Go로 이식.

**우선순위** (Python `vault_client.py:L108-110`):
1. 환경변수 `RUNEVAULT_GRPC_TARGET` (명시적 override, 최우선)
2. `vault_endpoint` (config.json or `RUNEVAULT_ENDPOINT` env)에서 `_derive_grpc_target`으로 자동 추출

Go에서도 동일 우선순위 적용 (env var override 지원).

3가지 입력 형식 지원:

| 입력 | 정규화 |
|---|---|
| `tcp://host:port` | `host:port` (gRPC 표준) |
| `http://host:port/path` | `host:port` (scheme/path 제거) |
| `https://host:port/path` | 동일 |
| `host:port` | 그대로 |
| `host` | `host:50051` (default port) |

**구현** (`internal/adapters/vault/endpoint.go`):
```go
func NormalizeEndpoint(raw string) (string, error) {
    if strings.HasPrefix(raw, "tcp://") {
        return strings.TrimPrefix(raw, "tcp://"), nil
    }
    if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
        u, err := url.Parse(raw)
        if err != nil { return "", err }
        host := u.Host
        if !strings.Contains(host, ":") { host += ":50051" }
        return host, nil
    }
    // bare hostname
    if !strings.Contains(raw, ":") { raw += ":50051" }
    return raw, nil
}
```

## Health check (2-tier)

Python `vault_client.py:L301-337` `health_check()` 동작:

### Tier 1: gRPC 표준 health check (`grpc_health.v1`)

```python
from grpc_health.v1 import health_pb2, health_pb2_grpc
health_stub = health_grpc.HealthStub(self._channel)
resp = await health_stub.Check(
    health_proto.HealthCheckRequest(service=""),
    timeout=5.0,
)
return resp.status == health_proto.HealthCheckResponse.SERVING
```

Go 대응:
```go
import "google.golang.org/grpc/health/grpc_health_v1"

stub := grpc_health_v1.NewHealthClient(conn)
resp, err := stub.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
return err == nil && resp.Status == grpc_health_v1.HealthCheckResponse_SERVING
```

- Timeout: 5s (Python L315)
- Service 이름 빈 문자열 (전체 서버 상태)

### Tier 2: HTTP `/health` fallback (진단용)

Tier 1 실패 시, **endpoint가 http(s):// scheme인 경우만** 시도:

```
원본 endpoint가 http(s):// scheme
  → /mcp, /sse path suffix 제거
  → GET {base}/health
  → 응답 status 2xx면 "endpoint 살아있음, gRPC만 실패" 진단
```

Go 포팅 (`internal/adapters/vault/health.go`):
```go
func HealthFallback(ctx context.Context, rawEndpoint string) error {
    if !strings.HasPrefix(rawEndpoint, "http") { return ErrNotHTTPScheme }
    u, _ := url.Parse(rawEndpoint)
    // strip /mcp, /sse suffixes
    u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/mcp"), "/sse")
    u.Path += "/health"
    resp, err := http.Get(u.String())
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != 200 { return fmt.Errorf("health %d", resp.StatusCode) }
    return nil
}
```

용도: 진단 메시지에 "gRPC 실패했지만 HTTP health는 살아있음 → Vault endpoint 자체는 괜찮고 gRPC 포트만 문제" 같은 유용한 정보 포함.

## 연결 관리

### 세션별 독립 연결

각 rune-mcp 프로세스가 **자기 Vault gRPC 채널**을 독립적으로 관리. 공유 풀 없음.

```go
conn, err := grpc.NewClient(
    endpoint,
    grpc.WithTransportCredentials(creds),
    grpc.WithDefaultCallOptions(
        grpc.MaxCallRecvMsgSize(MaxVaultMessageLength), // 256MB (EvalKey 수용)
        grpc.MaxCallSendMsgSize(MaxVaultMessageLength),
    ),
    grpc.WithKeepaliveParams(keepaliveParams),
)
defer conn.Close()
client := vaultpb.NewRuneVaultServiceClient(conn)
```

- 세션 3개 = Vault 연결 3개
- Vault 서버 입장에서는 여러 token이 동시 접속 (같은 token일 수도, 다를 수도)

### 메시지 크기 제한 (EvalKey 수용)

Python `mcp/adapter/vault_client.py:L33, L166-169`와 bit-identical:

```go
// internal/adapters/vault/client.go
const MaxVaultMessageLength = 256 * 1024 * 1024  // 256 MB

// grpc.NewClient 생성 시 WithDefaultCallOptions로 주입
grpc.WithDefaultCallOptions(
    grpc.MaxCallRecvMsgSize(MaxVaultMessageLength),
    grpc.MaxCallSendMsgSize(MaxVaultMessageLength),
)
```

**왜 필요한가**:
- `GetPublicKey` 응답의 `EvalKey`(FHE 연산키)가 **수십 MB** 크기
- gRPC 기본 max message size = 4MB → **수신 실패** (`ResourceExhausted`)
- Python은 `grpc.max_send_message_length` + `grpc.max_receive_message_length` 양방향 256MB
- Go는 `MaxCallRecvMsgSize` + `MaxCallSendMsgSize` 동일 설정

**insecure / secure 양쪽 동일 적용** (Python `vault_client.py:L170-182`는 TLS 분기 안에서도 같은 `options`를 전달).

### TLS

- **server TLS** (MVP 기본): config `tls_disable: false`, CA cert 선택
- **mTLS** (post-MVP): cert 프로비저닝 인프라 필요 → 로드맵

```go
func loadCreds(cfg VaultConfig) credentials.TransportCredentials {
    if cfg.TLSDisable {
        return insecure.NewCredentials()  // bufconn/dev only
    }
    tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
    if cfg.CACert != "" {
        pool := x509.NewCertPool()
        cert, _ := os.ReadFile(cfg.CACert)
        pool.AppendCertsFromPEM(cert)
        tlsCfg.RootCAs = pool
    } else {
        pool, _ := x509.SystemCertPool()
        tlsCfg.RootCAs = pool
    }
    return credentials.NewTLS(tlsCfg)
}
```

### Keepalive

```go
grpc.WithKeepaliveParams(keepalive.ClientParameters{
    Time:                30 * time.Second,
    Timeout:             10 * time.Second,
    PermitWithoutStream: true,
})
```

네트워크 sleep/wake · NAT 타임아웃 대응. stale connection 자동 감지.

## 인증 (Bearer token)

Vault는 gRPC metadata의 `authorization: Bearer <vault.token>` 헤더로 인증. 매 RPC에 주입:

```go
func (c *Client) authCtx(ctx context.Context) context.Context {
    return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}
```

`vault.token`은 `/rune:configure` 시점 사용자가 입력하여 `config.json`에 영구 저장 (vault 섹션).

## 타임아웃·재시도

### Python default (전 RPC 공통)

Python `vault_client.py:L84` `timeout: float = 30.0` — 모든 RPC (GetPublicKey, DecryptScores, DecryptMetadata)에 동일 30초 적용. `self._stub.X(request, timeout=self.timeout)` 패턴.

Go도 기본 30초로 맞춤 (bit-identical). health_check만 별도 5초 (Python L315).

### 부팅 시 (GetPublicKey)

`rune-mcp` 부팅 시 호출. 실패하면 `waiting_for_vault` 상태로 exp backoff retry (상세는 `rune-mcp.md` 참조). 재시도 경계 1s → 60s cap, 무한 반복. 단일 시도 timeout 30초.

### 런타임 (DecryptScores, DecryptMetadata)

capture/recall 경로에 포함. 실패 시 정책:
- context timeout 30s (Python default 기준)
- exp backoff 2-retry (1s, 2s) 후 에러 반환
- 에러에 `retryable=true` 마킹 → 에이전트에게 재시도 힌트

3회 연속 실패 시 `state=dormant` 전환은 **하지 않음** (너무 공격적). 대신 metric `rune_vault_errors_total{endpoint}` 증가. 영구 장애는 사용자가 `/rune:vault_status`로 진단.

### Circuit breaker (post-MVP)

`github.com/sony/gobreaker` per-endpoint:
- 5회 연속 실패 → open 30s
- open 상태 → fast-fail (`ErrVaultCircuitOpen`)
- half-open 1 probe → 성공이면 close

runtime latency 폭증 방지. MVP에서는 간단한 exp backoff로 시작, 실측 데이터 보고 도입.

## 에러 분류

```go
// internal/adapters/vault/errors.go
var (
    ErrVaultUnavailable = &Error{Code: "VAULT_UNAVAILABLE", Retryable: true}
    ErrVaultAuthFailed  = &Error{Code: "VAULT_AUTH_FAILED",  Retryable: false}
    ErrVaultKeyNotFound = &Error{Code: "VAULT_KEY_NOT_FOUND", Retryable: false}
    ErrVaultInternal    = &Error{Code: "VAULT_INTERNAL",     Retryable: true}
    ErrVaultTimeout     = &Error{Code: "VAULT_TIMEOUT",      Retryable: true}
)
```

gRPC status → 위 sentinel에 매핑:
- `Unauthenticated` → `ErrVaultAuthFailed`
- `NotFound` → `ErrVaultKeyNotFound`
- `Unavailable`, `DeadlineExceeded` → `ErrVaultUnavailable` · `ErrVaultTimeout`
- 기타 → `ErrVaultInternal`

## agent_dek 검증

Vault 번들의 `agent_dek`는 반드시 정확히 **32바이트** (AES-256 키). 크기 다르면 Vault 설정 버그 가능 → 즉시 에러:

```go
if len(bundle.AgentDEK) != 32 {
    return fmt.Errorf("vault: invalid agent_dek size %d (expected 32)", len(bundle.AgentDEK))
}
```

이 검증 실패는 retry 의미 없음 (`retryable=false`).

## 메모리 관리

- `EncKey`, `EvalKey`, `agent_dek` 모두 메모리 `[]byte`
- 프로세스 종료 시 zeroize:
  ```go
  for i := range agentDEK { agentDEK[i] = 0 }
  runtime.KeepAlive(agentDEK)
  ```
- `EncKey`/`EvalKey`는 디스크에도 캐시되지만 재부팅 때 Vault에서 새로 받음 (stale 방지)

## 패키지 레이아웃

```
internal/adapters/vault/
├── client.go         # NewClient, 3 RPC 메서드
├── endpoint.go       # NormalizeEndpoint
├── health.go         # HealthFallback (진단용)
├── errors.go         # typed errors
└── client_test.go    # bufconn 기반 unit test
```

Proto는 기존 `mcp/adapter/vault_proto/vault_service.proto`를 Go stub으로 재생성:
```bash
protoc --go_out=. --go-grpc_out=. vault_service.proto
```

생성된 stub은 `internal/adapters/vault/pb/` 에 위치.

## 테스트 전략

### Unit (bufconn)

```go
// internal/adapters/vault/client_test.go
func TestGetPublicKey_BufconnHappyPath(t *testing.T) {
    lis := bufconn.Listen(1024*1024)
    server := grpc.NewServer()
    vaultpb.RegisterRuneVaultServiceServer(server, &mockVault{...})
    go server.Serve(lis)

    conn, _ := grpc.DialContext(ctx, "bufconn",
        grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
            return lis.Dial()
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    client := vault.NewClient(conn, "test-token")

    bundle, err := client.GetPublicKey(ctx)
    require.NoError(t, err)
    require.Len(t, bundle.AgentDEK, 32)
}
```

### Integration (`//go:build integration`)

실 staging Vault 호출. CI 별도 잡:
- 정상 토큰 → 번들 반환
- 잘못된 토큰 → `Unauthenticated`
- 네트워크 차단 → `Unavailable`

### synctest (Go 1.25)

boot retry 결정적 검증:
```go
//go:build go1.25
func TestBootRetry_ExpBackoff(t *testing.T) {
    synctest.Test(t, func(t *testing.T) {
        vault := &MockVault{FailTimes: 3}
        start := time.Now()
        bundle, err := runBoot(vault)
        require.NoError(t, err)
        // 1s + 2s + 5s = 8s 누적 대기 확인
        require.InDelta(t, 8*time.Second, time.Since(start), 100*time.Millisecond)
    })
}
```

## 제약 · 미결

- Python `vault_client.py`의 legacy HTTP endpoint 분기 (L70, L93-94, L117-140) — Go에서 유지할지 폐기할지: **폐기 제안** (MVP). 기존 production 동작은 gRPC만 씀
- mTLS (post-MVP)
- Circuit breaker 정식 도입 (post-MVP 실측 후)
- `DecryptBundle` (DecryptScores + DecryptMetadata 통합 RPC) — proto 변경 필요, cross-team, post-MVP
