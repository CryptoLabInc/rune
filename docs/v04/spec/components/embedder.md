# Embedder 통합 — 임베딩 gRPC 클라이언트

rune-mcp(Go)가 외부 임베딩 데몬(가칭 `embedder`)을 **gRPC 클라이언트**로 호출하기 위한 통합 가이드. embedder의 설치·모델·런타임·수명관리는 이 프로젝트의 관심사 **밖**. 여기서는 "rune-mcp가 어떻게 embedder와 통신하는가"만 다룬다.

> **네이밍 정리**: 초기 설계에서 "runed"는 중앙화 통합 데몬(Python MCP 대체 + 임베딩 내장) 구상이었다. **통합 데몬 의미는 폐기**됐고, 현재 **`runed`는 임베딩 전담 데몬(= embedder의 정식 프로젝트 이름)**. 형제 프로젝트 `github.com/CryptoLabInc/runed`.
> - **rune-mcp**: Python MCP를 Go로 포팅한 세션별 바이너리 (이 프로젝트, 임베딩 제외)
> - **embedder = `runed`**: 임베딩 모델만 담당하는 별도 프로세스. 머신당 1개 상주, gRPC over UDS
>
> 본 문서에서 "embedder"는 역할명, "`runed`"는 구체 구현·패키지명. 두 프로세스는 gRPC (unix socket)로 통신.

> 관련 결정: D30 (gRPC 프로토콜 확정). D6 · D9 · D29는 embedder 담당 범위로 이관(Archived).

## 책임 경계

| 항목 | 담당 |
|---|---|
| 임베딩 모델 선택·로드 | embedder |
| 런타임 (llama-server 등) | embedder |
| 모델 identity · dim 공시 | embedder (Info RPC) |
| 데몬 설치 · launchd/systemd 등록 | embedder 제공 도구 |
| 소켓 경로 기본값 | `~/.runed/embedding.sock` (runed convention) |
| rune-mcp 측 gRPC 클라이언트 | **rune-mcp (이 프로젝트)** |
| Retry · timeout · backoff 정책 | **rune-mcp** (D7) |
| Info 응답 캐시 · batch split | **rune-mcp** (D16 · D23) |
| 대응 에러 분류 · waitable | **rune-mcp** |

rune-mcp는 **embedder를 띄우지 않는다**. embedder는 운영 환경에서 이미 떠있는 전제. rune-mcp는 필요한 만큼 gRPC 호출만.

## Proto 계약 요약

> **Proto 소스**: `runed` 프로젝트가 정의·관리. rune-mcp는 generated Go 스텁을 import만 함.

확정값:
- 패키지: `runed.v1`
- 서비스: `RunedService`
- Go module: `github.com/CryptoLabInc/runed`
- Generated stub import path: `github.com/CryptoLabInc/runed/gen/runed/v1` (관례 alias `runedv1`)
- 클라이언트 라이브러리 (옵션): `github.com/CryptoLabInc/runed/client` — `Connect/Embed/EmbedBatch/Info/Close` wrapping 제공. rune-mcp는 이 라이브러리를 inner transport로 두고 정책 layer(retry · info cache · error mapping)만 자체 작성하는 hybrid 채택 가능.

```
service RunedService {
    rpc Embed(EmbedRequest) returns (EmbedResponse);
    rpc EmbedBatch(EmbedBatchRequest) returns (EmbedBatchResponse);
    rpc Info(InfoRequest) returns (InfoResponse);
    rpc Health(HealthRequest) returns (HealthResponse);
    rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

### RPC 요약

| RPC | 용도 | rune-mcp 사용 |
|---|---|---|
| `Embed(text) → vector` | 단일 텍스트 임베딩 | recall `searchByID` helper 등 단건 경로 |
| `EmbedBatch(texts) → embeddings` | 배치 임베딩 | capture Phase 6 · recall Phase 3 (D16 · D23) |
| `Info() → {daemon_version, model_identity, vector_dim, max_text_length, max_batch_size}` | 메타데이터 | 기동 후 1회 조회, 메모리 캐시 |
| `Health() → {status, uptime, total_requests}` | 상태 체크 | 장애 분류(LOADING vs DEGRADED) |
| `Shutdown(grace_seconds)` | 종료 요청 | **호출 안 함** (rune-mcp는 embedder 수명 관리 책임 없음) |

### 불변 계약

- **L2-normalization**: embedder가 자동 수행. rune-mcp는 별도 normalize 코드 불필요
- **dim**: Qwen3-Embedding-0.6B 기준 1024. `Info.vector_dim`으로 확인 후 불일치면 에러
- **최대 텍스트 길이**: `Info.max_text_length` (문자 수). 초과 시 `INVALID_ARGUMENT` 반환
- **최대 배치 크기**: `Info.max_batch_size`. 초과 시 rune-mcp 측에서 **split** 후 재호출
- **model_identity**: 변경되면 저장된 embedding 공간 무효. MVP에서는 **slog 로깅만** (Info cache 섹션 참조, post-MVP 재임베딩 migration tool의 breadcrumb). 자동 감지·차단은 Post-MVP

## 소켓 경로

- `runed` 데몬은 **`~/.runed/embedding.sock`**에 listen (macOS/Linux). Windows named pipe는 `runed` Plan B (현재 미지원).
- `runed` 측 환경변수 `RUNED_HOME` override 시 `$RUNED_HOME/embedding.sock`
- rune-mcp는 다음 우선순위로 소켓 경로 결정:
  1. 환경 변수 `RUNE_EMBEDDER_SOCKET`
  2. `~/.rune/config.json`의 `embedder.socket_path`
  3. 기본값 `~/.runed/embedding.sock` (runed convention)

## 클라이언트 구현

### 패키지 구조

```
internal/adapters/embedder/
  ├── client.go        # Client interface + newClient(sockPath) 생성자
  ├── info_cache.go    # Info 1회 호출 + struct 캐시
  ├── retry.go         # D7 retry 정책 (backoff [0, 500ms, 2s])
  └── errors.go        # embedder 에러 → 도메인 에러 매핑
```

### Client 인터페이스

```go
package embedder

import (
    "context"

    runedv1 "github.com/CryptoLabInc/runed/gen/runed/v1"
)

type Client interface {
    // Phase 1 embed calls
    EmbedSingle(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

    // 메타데이터 · 헬스
    Info(ctx context.Context) (InfoSnapshot, error)
    Health(ctx context.Context) (HealthSnapshot, error)

    Close() error
}

type InfoSnapshot struct {
    DaemonVersion  string
    ModelIdentity  string
    VectorDim      int
    MaxTextLength  int
    MaxBatchSize   int
}

type HealthSnapshot struct {
    Status         string   // "OK" | "LOADING" | "DEGRADED" | "SHUTTING_DOWN"
    UptimeSeconds  int64
    TotalRequests  int64
}
```

### Dial

```go
func New(sockPath string) (Client, error) {
    conn, err := grpc.NewClient(
        "unix:"+sockPath,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    if err != nil { return nil, err }
    return &client{
        conn: conn,
        svc:  runedv1.NewRunedServiceClient(conn),
    }, nil
}
```

Unix socket에서는 TLS 불필요 (커널-mediated, 같은 머신). `insecure.NewCredentials()`가 표준.

### EmbedBatch with split

```go
func (c *client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
    info, err := c.infoCache.Get(ctx)
    if err != nil { return nil, err }

    if len(texts) <= info.MaxBatchSize {
        return c.embedBatchOnce(ctx, texts)
    }

    // embedder가 받을 수 있는 한도를 넘으면 split
    var out [][]float32
    for i := 0; i < len(texts); i += info.MaxBatchSize {
        end := i + info.MaxBatchSize
        if end > len(texts) { end = len(texts) }
        chunk, err := c.embedBatchOnce(ctx, texts[i:end])
        if err != nil { return nil, err }
        out = append(out, chunk...)
    }
    return out, nil
}

func (c *client) embedBatchOnce(ctx context.Context, texts []string) ([][]float32, error) {
    resp, err := c.retry(ctx, func(ctx context.Context) (*runedv1.EmbedBatchResponse, error) {
        return c.svc.EmbedBatch(ctx, &runedv1.EmbedBatchRequest{Texts: texts})
    })
    if err != nil { return nil, err }
    if len(resp.Embeddings) != len(texts) {
        return nil, fmt.Errorf("embedder: expected %d embeddings, got %d", len(texts), len(resp.Embeddings))
    }
    out := make([][]float32, len(resp.Embeddings))
    for i, e := range resp.Embeddings {
        if len(e.Vector) != c.infoCache.Snapshot().VectorDim {
            return nil, fmt.Errorf("embedder: vector dim mismatch at index %d", i)
        }
        out[i] = e.Vector
    }
    return out, nil
}
```

### Retry 정책 (D7)

```go
var backoff = []time.Duration{0, 500 * time.Millisecond, 2 * time.Second}

func (c *client) retry[R any](ctx context.Context, call func(context.Context) (R, error)) (R, error) {
    var zero R
    var lastErr error
    for _, delay := range backoff {
        if delay > 0 {
            select {
            case <-time.After(delay):
            case <-ctx.Done(): return zero, ctx.Err()
            }
        }
        r, err := call(ctx)
        if err == nil { return r, nil }

        if !retryable(err) { return zero, err }
        lastErr = err
    }
    return zero, fmt.Errorf("embedder: all retries exhausted: %w", lastErr)
}

func retryable(err error) bool {
    st, ok := status.FromError(err)
    if !ok { return false }
    switch st.Code() {
    case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
        return true
    }
    return false
}
```

### Info 캐시

```go
type infoCache struct {
    once sync.Once
    snap InfoSnapshot
    err  error
    svc  runedv1.RunedServiceClient
}

func (ic *infoCache) Get(ctx context.Context) (InfoSnapshot, error) {
    ic.once.Do(func() {
        resp, err := ic.svc.Info(ctx, &runedv1.InfoRequest{})
        if err != nil { ic.err = err; return }
        ic.snap = InfoSnapshot{
            DaemonVersion: resp.DaemonVersion,
            ModelIdentity: resp.ModelIdentity,
            VectorDim:     int(resp.VectorDim),
            MaxTextLength: int(resp.MaxTextLength),
            MaxBatchSize:  int(resp.MaxBatchSize),
        }
        // Breadcrumb for post-MVP re-embedding migration tool (D30).
        // Model 변경 자동 감지/차단은 MVP scope 밖. 로그로만 기록.
        slog.Info("embedder info loaded",
            "daemon_version", ic.snap.DaemonVersion,
            "model_identity", ic.snap.ModelIdentity,
            "vector_dim", ic.snap.VectorDim,
            "max_batch_size", ic.snap.MaxBatchSize,
        )
    })
    return ic.snap, ic.err
}
```

> `sync.Once`로 **첫 호출 시 1회만** 조회. embedder가 런타임 중 config 변경으로 값이 바뀌지는 않는 전제 (재시작 필요). 런타임 변경 지원이 필요해지면 TTL 캐시로 확장.

### Health 활용

첫 embed 호출 실패 시 `Health` 조회로 분류:
- `LOADING` → 잠시 후 재시도 (wait-and-retry 대기)
- `DEGRADED` → 경고 로그 + 상위 `EmbedderDegradedError` 전파
- `SHUTTING_DOWN` → 즉시 실패 + 상위 `EmbedderUnavailableError`

## 에러 매핑

| embedder gRPC code | rune-mcp 도메인 에러 | 동작 |
|---|---|---|
| `OK` | — | 정상 |
| `INVALID_ARGUMENT` | `EmbedderInvalidInputError` | 상위 전파 (text 길이 초과 등) |
| `RESOURCE_EXHAUSTED` | `EmbedderBusyError` | retry |
| `UNAVAILABLE` | `EmbedderUnavailableError` | retry → 최종 dormant 전환 고려 |
| `DEADLINE_EXCEEDED` | `EmbedderTimeoutError` | retry |
| 기타 | `EmbedderError(wrap)` | 상위 전파 |

## 테스트 전략

- **Unit**: Mock `EmbedderServiceClient`으로 Batch split · retry · info 캐시 검증
- **Contract**: embedder 프로젝트가 제공하는 테스트용 mock 데몬(있다면)으로 Integration
- **Smoke**: 실제 embedder 프로세스에 Info 호출 성공 · Embed 1회 왕복 vector dim 일치

## 미결 / 외부 조율

- ~~**Socket 기본 경로**~~: ✅ Resolved — `~/.runed/embedding.sock` (runed convention)
- ~~**Proto 패키지 이름·module path**~~: ✅ Resolved — `runed.v1` / `RunedService` / `github.com/CryptoLabInc/runed`
- **Info 응답 schema 진화**: `runed`가 `Info`에 필드 추가 시 proto backward-compat (proto3 규약으로 자동) — rune-mcp는 알려진 필드만 파싱
- **Model identity 변경 대응**: MVP는 로그만, Post-MVP에 재임베딩 마이그레이션 도구 (경로 TBD)
- **Toolchain 호환**: `runed`의 `go 1.26.2` directive로 rune-go의 `go 1.25.9` → 1.26 이상 bump 필요. Phase 1 deps 작업 시 결정.

## 참고

- Proto 파일: `github.com/CryptoLabInc/runed/proto/runed/v1/runed.proto` (rune-go는 `go.mod`로 import만)
- 클라이언트 라이브러리: `github.com/CryptoLabInc/runed/client` (Plan A alpha — gRPC dial + Health 사전체크 wrapping)
- 결정 이력: `overview/decisions.md` D6 · D9 · D29 (Archived; 'runed'라는 이름은 embedder의 정식 이름으로 부활), D30 (Current — `runed.v1` / `RunedService` 확정)
- 관련 flow: `spec/flows/capture.md` Phase 3 · 6, `spec/flows/recall.md` Phase 3
