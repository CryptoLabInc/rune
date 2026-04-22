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

### 스키마 v2 (Go, reduced from Python)

`~/.rune/config.json`:
```json
{
  "vault": {
    "endpoint": "tcp://vault-TEAM.oci.envector.io:50051",
    "token": "<user-provided token>",
    "ca_cert": "<path or empty>",
    "tls_disable": false
  },
  "state": "active" | "dormant",
  "dormant_reason": "...",        // optional, state == dormant 시
  "dormant_since": "2026-04-22T12:34:56Z",  // optional, state == dormant 시 (RFC3339 UTC)
  "metadata": { "configVersion": "2.0", "lastUpdated": "...", "installedFrom": "..." }
}
```

**읽기만**. rune-mcp는 config.json을 수정하지 않음 (state 전환은 rune-mcp 메모리에서만, config.json에 쓰는 건 `/rune:configure` 시점뿐).

envector 자격증명·embedding 설정·기타는 **메모리에만**. Vault 번들에서 매 부팅마다 재획득. 상세는 `overview/architecture.md` 참조.

> **Python 대비 의도적 divergence**: Python `_init_pipelines` (server.py:L1583-1591)는 Vault에서 받은 envector 자격증명을 `config.json`에 **영구 저장** (fast boot 용). Go v0.4는 **메모리만** — 매 부팅 Vault 호출. 이유:
> - 보안: 디스크에 envector API key 저장 안 함
> - 단순성: single source of truth (Vault). 디스크 캐시 stale 문제 제거
> - 비용: Vault.GetPublicKey 1회 추가 (무시 가능)

metadata는 `map[string]any`로 라운드트립 보존.

### Python v1 대비 drop된 section

v0.4에서 의도적으로 제거. Python에서 상속한 config.json이 있어도 **Go는 unknown section 무시** (read-only, pass-through as extra metadata). 파괴적 동작 없음.

| Python section | drop 이유 |
|---|---|
| `envector` | Vault 번들에서 매 부팅 재획득 (메모리만) |
| `embedding` | D30: embedder 외부 프로세스 책임 |
| `llm` | D14/D21/D28: agent-delegated, rune-mcp는 LLM 미사용 |
| `scribe` | scribe legacy 완전 제거 (tier2/webhook/patterns 등) |
| `retriever` | server 기본값 (topk default=5, max=10) 사용 |

→ v0.4 Go config 총 필드 수: ~10개 (Python ~40개 대비 대폭 축소).

### Env var override

Python 많은 env var override 중 Go는 **RUNE_STATE**만 optional 지원 (개발/테스트용). 나머지는 drop (해당 config section이 없으므로).

### 파일 시스템 레이아웃 · 권한

Python `agents/common/config.py:L13-18, L358-365` bit-identical:

| 경로 | 유형 | 권한 | 용도 |
|---|---|---|---|
| `~/.rune/` (CONFIG_DIR) | dir | `0700` | root |
| `~/.rune/config.json` (CONFIG_PATH) | file | `0600` | 이 스키마 |
| `~/.rune/logs/` (LOGS_DIR) | dir | `0700` | slog 로그 (쓰일 때) |
| `~/.rune/keys/` (KEYS_DIR) | dir | `0700` | EncKey/EvalKey 캐시 |
| `~/.rune/keys/<key_id>/EncKey.json` | file | `0600` | FHE 공개키 |
| `~/.rune/keys/<key_id>/EvalKey.json` | file | `0600` | FHE 연산키 |
| `~/.rune/capture_log.jsonl` (CAPTURE_LOG_PATH) | file | `0600` | append-only (D20) |

**Go 부팅 시**: `ensureDirectories()` 호출 → 없으면 생성 + umask 무시하고 명시적 `os.Chmod(0700)` (Python L287, L361-365 동일).

### Write 시점

- 이 repo의 rune-mcp: **읽기 전용**. config.json 쓰기 안 함
- `/rune:configure` slash command: **이 repo 밖** (`commands/rune/configure.md` Claude Code plugin) — 처음 설치 시 또는 Vault token 갱신 시 사용자 interactive 입력으로 쓰기
- state 전환 (active ↔ dormant)은 **rune-mcp 프로세스 메모리에서만**. config.json 파일은 부팅 시 초기 state만 읽고, 런타임 전환은 디스크에 반영 안 함 (다음 부팅 시 다시 "active" 또는 "dormant"로 갈지는 Vault 번들 획득 성공 여부로 결정)

### Dormant mode 동작 (Python `server.py:L1544-1547` bit-identical)

부팅 시 `config.state != "active"`면 **pipeline init skip**:
- `_scribe = nil`, `_retriever = nil`
- MCP 서버 자체는 정상 실행 — 읽기 전용 tool은 동작 (`vault_status`, `diagnostics`, `capture_history`)
- 쓰기/검색 tool (capture, batch_capture, recall, delete_capture, reload_pipelines)은 `_ensure_pipelines()`에서 `PipelineNotReadyError` 반환
- 사용자가 `/rune:activate` (상위 plugin)로 state=active 전환 + rune-mcp 재기동 필요

이는 "degraded mode"로 부팅 실패를 **부분적으로 견딤** — 사용자가 진단 tool (`vault_status`, `diagnostics`)로 원인 파악 가능.

## FHE 키 관리

**메모리**:
- `EncKey`: FHE 암호화 공개키. Insert 시 envector-go SDK가 사용
- `EvalKey`: FHE 연산 키. `ActivateKeys`로 envector 서버에 업로드
- `agent_dek`: 32B AES-256 DEK. metadata envelope 암호화용. 매 세션 · Vault 번들에서 받음

**디스크**: `~/.rune/keys/<key_id>/`에 `EncKey.json`, `EvalKey.json`만 0600 저장 (재부팅 빠른 복구용). **`SecKey.json`은 절대 저장하지 않음** — Vault 소유.

**zeroize**: 프로세스 종료 시 메모리에서 agent_dek·SecKey 유사 키 전부 `for i := range dek { dek[i] = 0 }; runtime.KeepAlive(dek)` 패턴으로 정리. hard guarantee는 아니지만 best effort.

## AES envelope (capture는 rune-mcp, recall은 Vault 복호화)

envector-go SDK는 metadata를 **opaque string**으로 취급.

**비대칭 책임 분담** (Python 동작 bit-identical):
- **Capture 경로**: rune-mcp service 레이어가 local `agent_dek`으로 AES-256-CTR 암호화 → envelope 생성 → envector SDK의 Insert에 opaque string으로 전달 (Python `envector_sdk.py:L227-234, L249-253`)
- **Recall 경로**: envector SDK의 `GetMetadata`(=Python `call_remind`)는 **ciphertext를 opaque로 그대로 반환만 함** — SDK는 decrypt 안 함. service 레이어(Python `searcher.py:L444` batch / `L455` per-entry fallback; Go `internal/service/recall.go` Phase 5)가 **Vault.DecryptMetadata를 직접 호출**해서 plaintext 획득 (Python `vault_client.py:L263-299` 정의)

즉 rune-mcp는 암호화만 local, 복호화는 Vault 위임 (audit trail 보존). envector SDK는 양쪽 모두에서 metadata를 opaque string으로만 취급.

**포맷** (pyenvector `mcp/adapter/envector_sdk.py:L227-234` + `pyenvector/utils/aes.py:L52-58`과 bit-identical):
```
{"a": agent_id, "c": base64(IV||CT)}
```

**필드 의미**:
- **`"a"`** = **agent_id** (string). Vault 번들에서 받은 에이전트 식별자 (예: `"agent_xyz"`). 복호화 시 `agent_dek` lookup 키로 사용. envector는 이 값을 보지 않음 (opaque)
- **`"c"`** = **ciphertext**. `base64(IV(16B) ‖ AES-256-CTR(agent_dek, metadata_json_utf8))` — IV를 ciphertext 앞에 concat 후 전체를 base64 encoding

**세부**:
- Algorithm: AES-256-**CTR** (pyenvector 내부 docstring에 "GCM" 표기 있으나 실제 코드는 CTR — pyenvector bug)
- IV: 16B `crypto/rand` (Python `secrets.token_bytes(16)` 대응)
- padding: 없음 (CTR 모드)
- AAD: 없음 (CTR에서 무의미)
- Wire: `base64.StdEncoding`
- MAC: **없음** — malleability 있음. Q1에서 HMAC-SHA256 필드 `"m"` 추가 검토 중 (Deferred)

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
| `policy/pii.go` | 5 regex (email/phone/API key prefix/32+hex/card) — **rune-mcp 내부에서 실행** (Python `record_builder.py:L228` `_redact_sensitive` bit-identical). `BuildPhases` 진입부에서 `raw_event.text`에 적용한 결과 `cleanText`가 extraction helpers (title/evidence/decision 추출)에 공급됨. `original_text` 필드에는 redact 전 원본 저장 (AES envelope으로 암호화되어 envector에 저장). D13 Option A에 따라 record_builder가 rune-mcp 소속 |

**원칙**: 이 패키지는 테스트 fixture 기반 golden 비교로 Python과 bit-identical 보장.

## Capture log

`~/.rune/capture_log.jsonl` (0600). rune-mcp가 append only.

**다중 세션 동시 append 안전장치**:
- Go `sync.Mutex` (intra-process)
- OS `flock(LOCK_EX)` (inter-process) — 여러 rune-mcp가 같은 파일에 쓸 때
- 한 줄 atomic append (fsync 후)

Rotation: 초기엔 없음. 실측 후 lumberjack 등 검토.

> **Python 대비 divergence (의도적)**: Python `mcp/server/server.py:L115-138` `_append_capture_log`는 **`os.O_APPEND`만 사용, flock 없음**. POSIX `O_APPEND`는 `PIPE_BUF`(4KB) 이하 single write에 대해 kernel이 atomic append 보장하므로 JSON line 1개(보통 500-1000B) 수준에선 inter-process race가 실질적으로 발생하지 않는다. Go가 `flock(LOCK_EX)`을 추가하는 건 **보수적 안전장치** — multi-session이 본격 활성화되는 Go 환경에서 lumberjack rotation 등 향후 확장 대비. 순수 정합성 목적이라면 Python과 동일하게 `O_APPEND`만으로도 충분하나, Go 표준 관행(`sync.Mutex` + `flock`)을 채택.

## Observability

- **slog** (stdlib): structured JSON to stderr (Claude Code가 수집)
- **SensitiveFilter** (Python `server.py:L25-40` `_SensitiveFilter` 포팅) — 2 regex:
  - 패턴 1: `(sk-|pk-|api_|envector_|evt_)[a-zA-Z0-9_-]{10,}` — 6 prefix + 10자 이상 alphanumeric
  - 패턴 2: `(?i)(token|key|secret|password)["\s:=]+[a-zA-Z0-9_-]{20,}` — 4 field name + separator + 20자 이상
  - 치환: `m.group()[:8] + "***"` (첫 8자 보존 + 별 3개). Go: `strings.Replace` + 각 match 수동 처리 or regexp.ReplaceAllFunc
- **request_id**: 매 tool call에 UUID 부여, context로 전파. 로그·에러에 포함

**Metric은 rune-mcp에 내장 안 함** — 세션 수가 가변이라 scrape하기 어렵고, 의미 있는 메트릭은 `embedder` 쪽이 공유 지점이라 훨씬 유용. rune-mcp 모니터링은 slog의 structured events만.

## 에러 처리

### 응답 shape (Python `mcp/server/errors.py` bit-identical)

```json
{
  "ok": false,
  "error": {
    "code": "VAULT_CONNECTION_ERROR",
    "message": "Vault gRPC dial failed: ...",
    "retryable": true,
    "recovery_hint": "Vault is unreachable. Check: (1) Is the Vault server running? ..."
  }
}
```

- `ok`: 항상 `false` (에러 시)
- `error.code`: 아래 8종 중 하나 (string enum)
- `error.message`: Python `str(exc)` 동등 (원본 예외 메시지)
- `error.retryable`: bool — 에이전트가 재시도할지 판정
- `error.recovery_hint`: optional string — 사용자에게 전달할 조치 힌트 (없으면 생략)

### 에러 코드 8종 (Python 7 bit-identical + 1 Go-specific)

Python `mcp/server/errors.py` 포팅 + v0.4 embedder gRPC 전용 추가:

| # | Code | Go 타입 (`internal/domain/errors.go`) | retryable | 발생 상황 | recovery_hint 요지 |
|---|---|---|---|---|---|
| 1 | `INTERNAL_ERROR` | `ErrInternal` (base) | `false` | panic recovery · 알려지지 않은 에러 | (empty) |
| 2 | `VAULT_CONNECTION_ERROR` | `ErrVaultConnection` | **`true`** | Vault gRPC dial 실패 · Unavailable · DeadlineExceeded | "Vault is unreachable. Check endpoint · config.json. Run /rune:status." |
| 3 | `VAULT_DECRYPTION_ERROR` | `ErrVaultDecryption` | `false` | Vault가 DecryptScores/DecryptMetadata 거부 (Unauthenticated / NotFound) | "Vault rejected decryption. Check token · permissions. Run /rune:configure." |
| 4 | `ENVECTOR_CONNECTION_ERROR` | `ErrEnvectorConnection` | **`true`** | envector Cloud 연결 실패 · typed `ErrKeysXxx` 중 transient | "Cannot reach enVector. Check network · endpoint. Run /rune:status." |
| 5 | `ENVECTOR_INSERT_ERROR` | `ErrEnvectorInsert` | **`true`** | `idx.Insert` 실패 (batch 단위) | "Failed to store. Retry may succeed. Check API key via /rune:status." |
| 6 | `PIPELINE_NOT_READY` | `ErrPipelineNotReady` | `false` | `state != active` 모든 경우 (starting · waiting_for_vault · dormant) | state별 recovery_hint 다름 (아래 참조) |
| 7 | `INVALID_INPUT` | `ErrInvalidInput` | `false` | tool argument 검증 실패 (topk > 10, text 빈, JSON parse fail, tier2 검증 등) | "Check input parameters and try again" |
| 8 | `EMBEDDER_UNREACHABLE` ⭐ | `ErrEmbedderUnreachable` | **`true`** | embedder gRPC 연결 실패 (D30, Go 신규) | "Embedder daemon not responding. Check socket · retry in a moment." |

> **⭐ `EMBEDDER_UNREACHABLE`**: Python에 없음 (Python은 embedding이 process-internal). D30 외부 embedder 분리로 새 code 필요.

### `PIPELINE_NOT_READY` 상태별 recovery_hint

Python은 state 구분 안 하고 고정 hint ("Run /rune:activate"). Go는 internal state 기반으로 hint differentiation:

| internal state | recovery_hint |
|---|---|
| `starting` | "Rune is starting up. Wait 1-2 seconds and retry." |
| `waiting_for_vault` | "Waiting for Vault connection. Last error: {vault_err}. Run /rune:vault_status." |
| `dormant` (reason=user_deactivated) | "Rune is deactivated. Run /rune:activate to re-enable." |
| `dormant` (reason=vault_unreachable) | "Rune went dormant due to Vault failure. Check endpoint in config.json." |
| `dormant` (reason=envector_unreachable) | "Rune went dormant due to envector failure. Check network · API key." |

code 자체는 모두 `PIPELINE_NOT_READY` → 에이전트 retryable 판정 일관성 유지.

### metadata corruption (partial degrade, code 없음)

recall 경로에서 AES envelope 복호화 실패 시 Python은 **조용히 skip** (`searcher.py:L438` `logger.warning + entry["metadata"] = {}`). Go도 동일:
- 해당 record만 빈 metadata로 degrade
- 나머지 결과 정상 반환
- 별도 error code 만들지 않음 (partial failure)

### 에러 분류 매핑 (상위 레벨)

| 상황 | Go action | code |
|---|---|---|
| state != active | 응답 반환 (retry 안 함) | `PIPELINE_NOT_READY` |
| Vault gRPC dial 실패 | exp backoff 2-retry → 실패 시 반환 + 3회 연속 실패 시 내부 state `waiting_for_vault` 전환 | `VAULT_CONNECTION_ERROR` |
| Vault 인증 실패 | 즉시 반환 (재시도 무의미) | `VAULT_DECRYPTION_ERROR` |
| envector gRPC 실패 | exp backoff 2-retry → 반환 | `ENVECTOR_CONNECTION_ERROR` |
| envector Insert 실패 | exp backoff 2-retry → 반환 | `ENVECTOR_INSERT_ERROR` |
| embedder gRPC 실패 | D7 backoff `[0, 500ms, 2s]` × 3 → 반환 | `EMBEDDER_UNREACHABLE` |
| JSON 파싱 · 검증 실패 | 즉시 반환 | `INVALID_INPUT` |
| AES 복호화 실패 (recall) | 해당 record만 skip, warn log | (no error code, partial degrade) |
| Panic in handler | `recover()` middleware가 catch → 500 | `INTERNAL_ERROR` |

### Go helper (Python `make_error` 동등)

```go
// internal/domain/errors.go
type RuneError struct {
    Code          string
    Message       string
    Retryable     bool
    RecoveryHint  string
}

func (e *RuneError) Error() string { return e.Message }

// MakeError: exception → MCP 응답 map
func MakeError(err error) map[string]any {
    var re *RuneError
    if errors.As(err, &re) {
        r := map[string]any{
            "ok":    false,
            "error": map[string]any{
                "code":      re.Code,
                "message":   re.Message,
                "retryable": re.Retryable,
            },
        }
        if re.RecoveryHint != "" {
            r["error"].(map[string]any)["recovery_hint"] = re.RecoveryHint
        }
        return r
    }
    // 알려지지 않은 에러 → INTERNAL_ERROR fallback
    return map[string]any{
        "ok": false,
        "error": map[string]any{
            "code":      "INTERNAL_ERROR",
            "message":   err.Error(),
            "retryable": false,
        },
    }
}
```

### 타임아웃

- **Pipeline readiness (`_ensure_pipelines`)**: **120s** — tool call 진입부에서 백그라운드 init 대기. 초과 시 `PipelineNotReadyError` + hint "embedding model may still be downloading" (Python `server.py:L1503-1518`)
- tool call 전체: 30s (context.WithTimeout)
- Vault gRPC: 30s (Python default · `spec/components/vault.md` 참조)
- envector gRPC: 10s (Score/GetMetadata), 30s (Insert/ActivateKeys)
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
