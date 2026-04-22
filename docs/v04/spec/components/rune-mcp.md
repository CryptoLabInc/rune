# rune-mcp — 세션별 MCP 프로세스

Claude Code(및 호환 에이전트)가 세션마다 spawn하는 Go 바이너리. Python `mcp/server/server.py`의 Go 대체. 모델을 제외한 모든 세션 상태·로직·외부 통신 책임.

## 존재 이유 (scope)

- Claude Code의 MCP 확장점을 구현하여 에이전트가 rune 기능에 접근하게 한다
- 세션별 격리된 상태(Vault token, FHE 키, gRPC 연결)를 보유한다
- Vault·envector·`embedder`와 통신해 capture·recall 로직을 완결한다
- 모델 추론은 **직접 하지 않고** 외부 `embedder` 프로세스(gRPC)에 위임한다 (D30)

**책임이 아닌 것**:
- 임베딩 모델 로드·실행 — `embedder` 외부 프로세스 담당
- 전역 상태 공유 — 세션 간 공유 없음. 각 MCP는 독립
- SecKey 관리 — Vault 소유, rune-mcp는 접근 안 함

## 프로세스 수명

- **Spawn**: Claude Code가 plugin.json을 보고 `rune-mcp` 바이너리를 stdio로 실행
- **Runtime**: stdio JSON-RPC 2.0 요청 처리. MCP protocol handshake 후 tool call 응답
- **Exit**: stdin EOF 또는 SIGTERM 시 graceful shutdown
  1. inflight 요청 완료 대기 (timeout 30s)
  2. envector `keys.Close()` · `client.Close()` · Vault `conn.Close()`
  3. DEK zeroize (`for i := range dek { dek[i] = 0 }; runtime.KeepAlive(dek)`)
  4. process exit

**제약**: Go 프로세스는 상주가 아니라 세션 수명과 동기화. Claude 창 닫으면 종료됨 — Python MCP와 동일 행동.

## 상태 머신

```
(spawn)
   │
   ↓
starting ──(Vault OK)──→ active ←──────┐
   │                        ↓          │
   │                  /rune:deactivate  │
   │                        ↓          │
   │                     dormant        │
   │                        ↑          │
   │                  /rune:activate   │
   │                                    │
   └──(Vault 실패)──→ waiting_for_vault │
                             │          │
                             └──(복구)──┘

(백그라운드: waiting_for_vault 상태에서 exp backoff retry 무한 반복)
```

| State | 의미 | 요청 처리 |
|---|---|---|
| `starting` | Vault 첫 호출 진행 중 | 503 `{"status":"starting"}` |
| `waiting_for_vault` | Vault 연속 실패, 백그라운드 retry | 503 `{"code":"VAULT_PENDING","last_error":"..."}` |
| `active` | 정상 | tool call 처리 |
| `dormant` | 사용자가 명시적으로 deactivate | 503 `{"code":"DORMANT","hint":"run /rune:activate"}` |

상세 retry 정책은 "부팅 시퀀스"에서.

## 부팅 시퀀스

```go
// 의사코드
func main() {
    cfg := config.Load("~/.rune/config.json")  // vault.endpoint · vault.token · state · metadata

    mcp := NewMCPServer(stdio)
    mcp.RegisterTools(...)
    go mcp.Serve()  // Claude Code와 stdio 통신

    go runBootLoop(cfg)  // Vault 번들 획득 + retry

    waitForShutdown()
}

func runBootLoop(cfg *Config) {
    state.Store(StateStarting)

    backoff := []time.Duration{
        1*time.Second, 2*time.Second, 5*time.Second,
        15*time.Second, 30*time.Second, 60*time.Second,
    }
    attempt := 0
    for {
        bundle, err := vault.GetPublicKey(ctx, cfg.Vault.Token)
        if err == nil {
            saveKeysToDisk(bundle.EncKey, bundle.EvalKey)  // ~/.rune/keys/<key_id>/
            loadInMemory(bundle)  // envector creds, agent_dek, EncKey/EvalKey for FHE
            initEnvectorClient()
            state.Store(StateActive)
            return
        }

        state.Store(StateWaitingForVault)
        slog.Warn("vault unreachable, will retry", "attempt", attempt, "err", err)

        if attempt == 20 {
            slog.Error("vault persistent failure — check config")
            metrics.VaultPersistentFailure.Inc()
        }

        wait := backoff[min(attempt, len(backoff)-1)]
        time.Sleep(wait)
        attempt++
    }
}
```

**핵심**:
- `state.Store`는 `atomic.Pointer[State]` 또는 `atomic.Int32` 사용
- boot loop는 **데몬 수명 내내** 백그라운드 유지 (Vault 장애 복구 반응)
- capture/recall 요청은 `state.Load()` 확인 후 상태에 따라 분기

## MCP 서버 구현 — 공식 SDK 채택

`github.com/modelcontextprotocol/go-sdk` (v1.5.0+, stable) 사용. 이유·비교는 `overview/decisions.md` D2 참조.

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

func setupServer(deps *Deps) *mcp.Server {
    srv := mcp.NewServer(&mcp.Implementation{Name: "rune-mcp", Version: "0.4.0"}, nil)

    mcp.AddTool(srv, &mcp.Tool{
        Name: "rune_capture", Description: "Capture a decision record",
    }, makeCaptureHandler(deps))

    mcp.AddTool(srv, &mcp.Tool{Name: "rune_recall", /* ... */}, makeRecallHandler(deps))
    // ... 8 tools 총 등록

    return srv
}

func main() {
    cfg := lifecycle.LoadConfig()
    deps := lifecycle.NewDeps(ctx, cfg)
    go lifecycle.RunBootLoop(ctx, deps)

    srv := setupServer(deps)
    if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
        slog.Error("serve", "err", err)
        os.Exit(1)
    }
}
```

SDK가 자동 처리:
- stdio JSON-RPC 파싱·직렬화
- `initialize` handshake · capabilities negotiation
- `tools/list`에 tool schema 자동 노출 (Go struct + `jsonschema:` tag)
- `tools/call` dispatch + input 검증
- notification · cancellation 전파

rune-mcp가 책임지는 것:
- 각 tool handler의 비즈니스 로직 (state check → service call → response 포맷)
- `Deps` 주입 구조 (envector client · vault client · embedder client · state machine · logger)

## MCP Tools (8개)

Python `mcp/server/server.py`가 노출하는 8개 tool을 그대로 유지. 이름·파라미터·응답 shape bit-identical.

### tool_capture
- **입력**: `text` (원문), `source`, `user`, `channel`, `extracted` (ExtractionResult JSON)
- **처리**:
  1. 검증 (`phases[:7]`, `title[:60]`, `confidence clamp [0,1]`)
  2. `text_to_embed` 선택 (`reusable_insight > payload.text`, trim 후)
  3. `embedder.Embed(text)` gRPC 호출 (단일 텍스트, D30)
  4. envector `Score(vec)` → Vault `DecryptScores(blob)` → top-1 similarity
  5. `policy.ClassifyNovelty(sim)`
  6. `near_duplicate(≥0.95)`면 Insert 생략 → `{"ok":false, "reason":"near_duplicate", "similar_to":"<id>"}`
  7. 아니면: AES envelope `{"a","c"}` 생성 → envector `Insert(vec, envelope)` → `capture_log.jsonl` append
- **응답**: `{"ok":true, "record_id":..., "novelty":...}`

### tool_recall
- **입력**: `query`, `topk`, `filters`
- **처리**:
  1. `policy.ParseQuery(query)` → intent(31 regex), entities(4-stage), time scope, expanded[0..2]
  2. `embedder.EmbedBatch(texts)` gRPC 호출 (expanded 최대 3개, D23 batch 1회)
  3. 순차로 envector `Score` → Vault `DecryptScores` 3번 (D25 sequential, Post-MVP 병렬화 고려)
  4. dedup + filter
  5. envector `GetMetadata(refs)` → AES envelope 배열
  6. 각 envelope을 rune-mcp가 로컬에서 AES-CTR 복호화 (`agent_dek` 사용)
  7. `policy.Rerank`: `(0.7×raw + 0.3×decay) × status_mul`, half-life 90일
- **응답**: `{"results":[...], "confidence":..., "sources":[...]}`

### tool_reload_pipelines (`/rune:activate` 핸들러)
- `state=dormant` → `state=active` 전환. Vault 미호출한 상태였다면 `GetPublicKey` 호출

### tool_deactivate (`/rune:deactivate` 핸들러)
- `state=dormant`로 세팅. 프로세스는 유지 (Claude 세션 내에서는)

### tool_delete_capture
- envector에서 record 삭제 (Vault/envector 권한 확인 필요)

### tool_capture_history
- `capture_log.jsonl` 테일 읽기

### tool_vault_status
- Vault 연결 상태 · last_error · state · `waiting_for_vault` 진단 정보 반환

### tool_diagnostics
- 전체 헬스체크: Vault 연결 · envector 연결 · embedder `/health` · state · metrics snapshot

### tool_batch_capture (옵션)
- 여러 record 한 번에 capture. Python 구현 유지

## Config 로딩

`~/.rune/config.json`은 **영구 3-섹션**:
```json
{
  "vault": {
    "endpoint": "tcp://vault-TEAM.oci.envector.io:50051",
    "token": "<user-provided token>",
    "ca_cert": "<path or empty>",
    "tls_disable": false
  },
  "state": "active" | "dormant",
  "metadata": { "configVersion": "2.0", "lastUpdated": "...", "installedFrom": "..." }
}
```

**읽기만**. rune-mcp는 config.json을 수정하지 않음 (state 전환은 rune-mcp 메모리에서만, config.json에 쓰는 건 `/rune:configure` 시점뿐).

envector 자격증명·embedding 설정·기타는 **메모리에만**. Vault 번들에서 매 부팅마다 재획득. 상세는 `overview/architecture.md` 참조.

metadata는 `map[string]any`로 라운드트립 보존.

## FHE 키 관리

**메모리**:
- `EncKey`: FHE 암호화 공개키. Insert 시 envector-go SDK가 사용
- `EvalKey`: FHE 연산 키. `ActivateKeys`로 envector 서버에 업로드
- `agent_dek`: 32B AES-256 DEK. metadata envelope 암호화용. 매 세션 · Vault 번들에서 받음

**디스크**: `~/.rune/keys/<key_id>/`에 `EncKey.json`, `EvalKey.json`만 0600 저장 (재부팅 빠른 복구용). **`SecKey.json`은 절대 저장하지 않음** — Vault 소유.

**zeroize**: 프로세스 종료 시 메모리에서 agent_dek·SecKey 유사 키 전부 `for i := range dek { dek[i] = 0 }; runtime.KeepAlive(dek)` 패턴으로 정리. hard guarantee는 아니지만 best effort.

## AES envelope (rune-mcp 자체 구현)

envector-go SDK는 metadata를 **opaque string**으로 취급. AES 암·복호화는 rune-mcp 책임.

**포맷** (pyenvector와 bit-identical):
```
{"a": agent_id, "c": base64(IV||CT)}
```
- AES-256-CTR
- IV 16B `crypto/rand`
- padding 없음 (CTR 모드)
- Wire: base64.StdEncoding

**Go 구현** (`internal/adapters/envector/aes_ctr.go`):
```go
func Seal(dek []byte, agentID string, plaintext []byte) (string, error) {
    block, _ := aes.NewCipher(dek)
    iv := make([]byte, 16)
    io.ReadFull(rand.Reader, iv)
    stream := cipher.NewCTR(block, iv)
    ct := make([]byte, len(plaintext))
    stream.XORKeyStream(ct, plaintext)
    return json.Marshal(map[string]string{
        "a": agentID,
        "c": base64.StdEncoding.EncodeToString(append(iv, ct...)),
    })
}
```

**Open question**: MAC 필드 `"m"` 추가 (Q1, `overview/open-questions.md` 참조).

## Policy (순수 함수, 공유)

`internal/policy/`에 수학·상수 집중. I/O 없음. 외부 deps 없음.

| 파일 | 내용 |
|---|---|
| `policy/novelty.go` | 임계 `0.3/0.7/0.95`, `ClassifyNovelty(sim) → Class` |
| `policy/rerank.go` | `(0.7×raw + 0.3×decay) × status_mul`, half-life 90d |
| `policy/query.go` | 31 intent regex, 81 stop words, 4-stage entity, 16 time patterns |
| `policy/record_id.go` | `dec_YYYY-MM-DD_<domain>_<slug>` 생성 |
| `policy/pii.go` | 5 regex (email/phone/API key prefix/32+hex/card) — 참조용. 실제 마스킹은 에이전트 md 책임 (결정 #13 방향) |

**원칙**: 이 패키지는 테스트 fixture 기반 golden 비교로 Python과 bit-identical 보장.

## Capture log

`~/.rune/capture_log.jsonl` (0600). rune-mcp가 append only.

**다중 세션 동시 append 안전장치**:
- Go `sync.Mutex` (intra-process)
- OS `flock(LOCK_EX)` (inter-process) — 여러 rune-mcp가 같은 파일에 쓸 때
- 한 줄 atomic append (fsync 후)

Rotation: 초기엔 없음. 실측 후 lumberjack 등 검토.

## Observability

- **slog** (stdlib): structured JSON to stderr (Claude Code가 수집)
- **SensitiveFilter** (Python `_SensitiveFilter` 포팅): `sk-` · `api_` · `envector_` · `evt_` · `token=` · `Bearer ` 등 접두사 20자+ 자동 마스킹
- **request_id**: 매 tool call에 UUID 부여, context로 전파. 로그·에러에 포함

**Metric은 rune-mcp에 내장 안 함** — 세션 수가 가변이라 scrape하기 어렵고, 의미 있는 메트릭은 `embedder` 쪽이 공유 지점이라 훨씬 유용. rune-mcp 모니터링은 slog의 structured events만.

## 에러 처리

| 상황 | 반환 |
|---|---|
| state != active | 503 status-specific (`starting`/`VAULT_PENDING`/`DORMANT`) |
| Vault RPC 실패 | exp backoff 2-retry → `retryable=true` 에러 반환. 3회 연속이면 `waiting_for_vault` 전환 |
| envector RPC 실패 | exp backoff 2-retry → `retryable=true` |
| `embedder` gRPC 연결 실패 | `embedder_unreachable` 에러 (D30 retry 정책). 에이전트에 retry 제안 |
| AES 복호화 실패 | `metadata_corrupted` 에러. 해당 record만 skip하고 나머지 결과 반환 (partial degrade) |
| Panic in handler | `recover()` middleware가 잡아 500 `INTERNAL_ERROR`. 다른 요청 무영향 |

**타임아웃**:
- tool call 전체: 30s (context.WithTimeout)
- Vault gRPC: 10s
- envector gRPC: 10s
- `embedder` gRPC: 5s (D30, unix socket)

## 패키지 레이아웃 (rune-mcp 한정)

```
cmd/rune-mcp/main.go                # stdio + lifecycle
internal/
  ├── mcp/
  │   ├── server.go                 # MCP protocol dispatcher
  │   ├── tools.go                  # 8 tool handlers
  │   ├── state.go                  # state machine (atomic)
  │   └── validate.go               # phases[:7], title[:60], clamp
  ├── lifecycle/
  │   ├── boot.go                   # Vault boot retry loop
  │   └── shutdown.go               # graceful 30s
  ├── adapters/
  │   ├── config/                   # 3-section loader
  │   ├── vault/                    # gRPC client
  │   ├── envector/                 # SDK + AES envelope
  │   ├── embedder/                 # gRPC client to `embedder` (external process, D30)
  │   └── logio/                    # capture_log.jsonl flock append
  ├── policy/                       # pure: novelty · rerank · query · record_id · pii
  ├── domain/                       # DecisionRecord v2.1, capture/query types
  └── obs/
      ├── slog.go                   # SensitiveFilter handler
      └── request_id.go
```

## 테스트 전략

- **Unit (policy/)**: pure function, golden fixture 비교, race-free, <1ms per test
- **Adapter**: bufconn(gRPC) / httptest(HTTP) / 테스트용 mock
- **Service (tools/)**: 모든 외부 의존 mock 주입. happy/edge/error 경로
- **Concurrency (synctest)**: Go 1.25 `testing/synctest`로 boot retry · debounce · timeout 결정적 테스트
- **Integration**: 실 Vault + 실 envector (mock backend 또는 staging). build tag `//go:build integration`
- **E2E**: rune-mcp spawn + stdio JSON-RPC 실제 왕복

## 제약 · 미결

- AES envelope MAC 필드 (`overview/open-questions.md` Q1)
- Multi-MCP에서 envector `ActivateKeys` 경쟁 (Q3)
- envector-go SDK `OpenKeysFromFile` 조건 완화 PR (Q4)
- Vault 영구 실패 UX (Q9)
