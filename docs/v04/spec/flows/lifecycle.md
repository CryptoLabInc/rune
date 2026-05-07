# Lifecycle Flow — 나머지 6개 MCP tool

Capture/Recall을 제외한 MCP tool 6개의 설계. Python `mcp/server/server.py`의 동작을 **bit-identical**로 포팅. 각 tool은 단독 흐름이라 phase 구조 없이 tool 단위 섹션으로 정리.

## 대상 tool

**데이터 계열** (capture/recall의 보조):
- `batch_capture` — N개 decision 한 번에 저장 (session-end sweep)
- `capture_history` — 로컬 `~/.rune/capture_log.jsonl` 조회
- `delete_capture` — soft-delete (status=reverted로 변경 후 re-insert)

**운영·진단 계열**:
- `vault_status` — Vault 연결 상태
- `diagnostics` — 종합 헬스 리포트 (7 섹션)
- `reload_pipelines` — 파이프라인 재초기화 + envector warmup

## 공통 설계 규약

### State gate

모든 쓰기 tool (`batch_capture` · `delete_capture`)은 `_ensure_pipelines()` 경로를 거쳐야 함. Python `server.py`의 패턴과 동일:

```go
if err := deps.state.EnsurePipelines(ctx); err != nil {
    return nil, nil, err  // PipelineNotReadyError with recovery_hint
}
```

읽기 tool (`vault_status` · `diagnostics` · `capture_history`)은 state gate 통과 없이 동작 가능.

### Dormant 전환 (Python `_set_dormant_with_reason`, L171-200)

Vault/envector가 **운영 중 unreachable**이 되면 config.json의 `state`를 `dormant`로 낮추고 `dormant_reason`·`dormant_since` 기록. 이후 외부 agent가 `/rune:activate`로 재활성화.

Go 동일 패턴:

```go
func (s *stateManager) SetDormant(reason string) {
    // ~/.rune/config.json read-modify-write
    // 이미 같은 reason이면 skip (idempotent)
}
```

호출 지점:
- `tool_delete_capture`: Vault/envector 에러 시 (server.py:1182, 1194)
- `tool_recall`: Vault 에러 시 (server.py:1007)
- `_capture_single`: 동일 패턴

### Error shape

Python `make_error()` 패턴을 Go에서도 유지:

```go
type ErrorResponse struct {
    OK           bool    `json:"ok"`
    Error        string  `json:"error"`
    ErrorCode    string  `json:"error_code"`
    RecoveryHint *string `json:"recovery_hint,omitempty"`
}
```

MCP SDK는 Go 구조체를 자동 JSON 직렬화 → Python과 응답 필드 동일.

---

## 1. `rune_vault_status`

### 책임
Vault 연결·보안 모드 상태를 JSON으로 반환. **Read-only**, state gate 없음.

### Python 동작 (`server.py:490-528`)

```python
if self.vault is None:
    return {"ok": True, "vault_configured": False, "mode": "standard (no Vault)", ...}

vault_healthy = await self.vault.health_check()
return {
    "ok": True,
    "vault_configured": True,
    "vault_endpoint": ...,
    "secure_search_available": vault_healthy,
    "mode": "secure (Vault-backed)",
    "vault_healthy": vault_healthy,
    "team_index_name": ...,
}
```

### Go 설계

```go
type VaultStatusResult struct {
    OK                     bool    `json:"ok"`
    VaultConfigured        bool    `json:"vault_configured"`
    VaultEndpoint          *string `json:"vault_endpoint,omitempty"`
    SecureSearchAvailable  bool    `json:"secure_search_available"`
    Mode                   string  `json:"mode"`       // "secure (Vault-backed)" or "standard (no Vault)"
    VaultHealthy           *bool   `json:"vault_healthy,omitempty"`
    TeamIndexName          *string `json:"team_index_name,omitempty"`
    Warning                *string `json:"warning,omitempty"`
}

func (s *lifecycleService) VaultStatus(ctx context.Context) (*VaultStatusResult, error) {
    if s.vault == nil {
        warn := "secret key may be accessible locally. Configure Vault for secure mode."
        return &VaultStatusResult{
            OK: true,
            VaultConfigured: false,
            Mode: "standard (no Vault)",
            TeamIndexName: s.indexName,
            Warning: &warn,
        }, nil
    }
    healthy, err := s.vault.HealthCheck(ctx)
    if err != nil {
        // Python L526-528: make_error(VaultConnectionError)에 vault_configured=true 추가
        return nil, wrapVaultError(err, true)
    }
    ep := s.vault.Endpoint()
    return &VaultStatusResult{
        OK: true,
        VaultConfigured: true,
        VaultEndpoint: &ep,
        SecureSearchAvailable: healthy,
        Mode: "secure (Vault-backed)",
        VaultHealthy: &healthy,
        TeamIndexName: s.indexName,
    }, nil
}
```

### 구현 위치
- `internal/mcp/tools.go` — tool 등록
- `internal/service/lifecycle.go` — `VaultStatus`
- `internal/adapters/vault/client.go` — `HealthCheck`

---

## 2. `rune_diagnostics`

### 책임
7개 subsystem 상태를 한 번에 집합해 JSON으로 반환. **Read-only**, state gate 없음.

### Python 동작 (`server.py:530-684`)

7 섹션:
1. `environment`: OS, runtime 버전, cwd
2. `state`: `~/.rune/config.json`의 state/dormant_reason/dormant_since
3. `vault`: configured/healthy/endpoint
4. `keys`: enc_key_loaded / key_id / agent_dek_loaded
5. `pipelines`: scribe/retriever 초기화 여부 + active LLM provider
6. `embedding`: model/mode
7. `envector`: reachable/latency_ms/error_type/hint (5s timeout, thread pool isolation)

마지막으로 `vault_healthy==false` 또는 `enc_key_loaded==false`이면 `ok=false`.

### Go 설계

Python 응답 구조 bit-identical. 각 섹션은 독립 함수:

```go
type DiagnosticsResult struct {
    OK           bool              `json:"ok"`
    Environment  EnvInfo           `json:"environment"`
    State        *string           `json:"state,omitempty"`
    DormantReason *string          `json:"dormant_reason,omitempty"`
    DormantSince  *string          `json:"dormant_since,omitempty"`
    Vault        VaultInfo         `json:"vault"`
    Keys         KeysInfo          `json:"keys"`
    Pipelines    PipelinesInfo     `json:"pipelines"`
    Embedding    EmbeddingInfo     `json:"embedding"`
    Envector     EnvectorInfo      `json:"envector"`
}

func (s *lifecycleService) Diagnostics(ctx context.Context) *DiagnosticsResult {
    r := &DiagnosticsResult{OK: true}
    r.Environment = s.collectEnv()
    if cfg, err := readRuneConfig(); err == nil {
        r.State = &cfg.State
        r.DormantReason = cfg.DormantReason
        r.DormantSince = cfg.DormantSince
    }
    r.Vault = s.collectVaultInfo(ctx)
    r.Keys = s.collectKeysInfo()
    r.Pipelines = s.collectPipelinesInfo()
    r.Embedding = s.collectEmbeddingInfo()
    r.Envector = s.collectEnvectorInfo(ctx, 5*time.Second)  // Python ENVECTOR_DIAGNOSIS_TIMEOUT

    if r.Vault.Configured && !r.Vault.Healthy { r.OK = false }
    if !r.Keys.EncKeyLoaded { r.OK = false }
    return r
}
```

#### Envector timeout + error classification (Python L626-676)

envector `invoke_get_index_list` 호출을 **5초 timeout**. Python은 ThreadPoolExecutor로 blocking call을 isolate. Go는 `context.WithTimeout` + goroutine + select로 자연스럽게 구현:

```go
func (s *lifecycleService) collectEnvectorInfo(ctx context.Context, timeout time.Duration) EnvectorInfo {
    if s.envector == nil { return EnvectorInfo{Reachable: false} }

    ctx2, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    type res struct { latency time.Duration; err error }
    ch := make(chan res, 1)
    t0 := time.Now()
    go func() {
        _, err := s.envector.GetIndexList(ctx2)
        ch <- res{time.Since(t0), err}
    }()

    select {
    case r := <-ch:
        if r.err != nil {
            return classifyEnvectorError(r.err, r.latency)
        }
        return EnvectorInfo{
            Reachable: true,
            LatencyMs: float64(r.latency.Milliseconds()),
        }
    case <-ctx2.Done():
        return EnvectorInfo{
            Reachable: false,
            Error: fmt.Sprintf("Health check timed out after %.0fs", timeout.Seconds()),
            ErrorType: "timeout",
            ElapsedMs: float64(time.Since(t0).Milliseconds()),
        }
    }
}
```

#### Error classification (Python L655-672)

`UNAVAILABLE|Connection refused` → `connection_refused`
`UNAUTHENTICATED|401` → `auth_failure`
`DEADLINE_EXCEEDED` → `deadline_exceeded`
기타 → `unknown`

각 error_type마다 `hint` 필드 부착 (Python 문구 동일).

### 구현 위치
- `internal/service/lifecycle.go` — `Diagnostics` + collectors
- `internal/service/diagnostics_classify.go` — envector error classification

---

## 3. `rune_batch_capture`

### 책임
JSON array로 들어온 N개 decision item을 **순차 · 독립 처리**. 하나 실패해도 나머지 진행. Per-item novelty 체크, near-duplicate는 skip.

### Python 동작 (`server.py:810-896`)

```python
items_list = json.loads(items)
# per-item loop
for i, item in enumerate(items_list):
    item_text = item.get("reusable_insight") or item.get("title") or "[batch_capture]"
    result = await self._capture_single(text=item_text, extracted=json.dumps(item), ...)
    # classify status: captured / skipped / near_duplicate / error
```

응답:
```json
{"ok": true, "total": N, "results": [{index, title, status, novelty}, ...],
 "captured": X, "skipped": Y, "errors": Z}
```

### Go 설계

```go
type BatchCaptureArgs struct {
    Items   string  `json:"items"`     // JSON array string
    Source  string  `json:"source,omitempty"`
    User    *string `json:"user,omitempty"`
    Channel *string `json:"channel,omitempty"`
}

type BatchCaptureResult struct {
    OK       bool              `json:"ok"`
    Total    int               `json:"total"`
    Results  []BatchItemResult `json:"results"`
    Captured int               `json:"captured"`
    Skipped  int               `json:"skipped"`
    Errors   int               `json:"errors"`
}

type BatchItemResult struct {
    Index   int     `json:"index"`
    Title   string  `json:"title"`
    Status  string  `json:"status"`   // captured / skipped / near_duplicate / error
    Novelty string  `json:"novelty,omitempty"`
    Error   *string `json:"error,omitempty"`
}

func (s *captureService) Batch(ctx context.Context, args BatchCaptureArgs) (*BatchCaptureResult, error) {
    var items []json.RawMessage
    if err := json.Unmarshal([]byte(args.Items), &items); err != nil {
        return &BatchCaptureResult{OK: false /* error wrapped */}, invalidInput("items must be JSON array")
    }
    if len(items) == 0 {
        return &BatchCaptureResult{OK: true, Total: 0}, nil
    }

    results := make([]BatchItemResult, 0, len(items))
    for i, raw := range items {
        title, text := extractItemTextTitle(raw)  // reusable_insight || title || "[batch_capture]"
        single, err := s.captureSingle(ctx, SingleCaptureInput{
            Text: text, Source: args.Source, User: args.User, Channel: args.Channel,
            Extracted: string(raw),
        })
        results = append(results, classifyBatchItem(i, title, single, err))
    }

    return summarizeBatch(results), nil
}
```

### 관련 결정
- **D14** (record_builder agent-delegated): batch도 `extracted` 필수 — MVP에서 pre_extraction 없는 item은 에러
- **D16** (multi-record batch embedding): capture와 공유

### 구현 위치
- `internal/service/capture.go` — `Batch` wrapper
- `internal/mcp/tools.go` — tool 등록

### 에러
- `items` invalid JSON → `InvalidInputError`
- `items` not array → `InvalidInputError`
- Per-item 실패: status="error" + 기록, 다른 item 진행

---

## 4. `rune_capture_history`

### 책임
로컬 `~/.rune/capture_log.jsonl` 파일을 역순으로 읽어 최근 N개 반환. 외부 시스템 (Vault/envector) 호출 없음.

### Python 동작 (`server.py:1092-1111` + helper `_read_capture_log` L140-168)

```python
def _read_capture_log(limit=20, domain=None, since=None) -> list:
    if not os.path.exists(CAPTURE_LOG_PATH):
        return []
    lines = open(CAPTURE_LOG_PATH).readlines()
    entries = []
    for line in reversed(lines):  # 역순
        entry = json.loads(line)
        if domain and entry.get("domain") != domain: continue
        if since and entry.get("ts", "") < since: continue
        entries.append(entry)
        if len(entries) >= limit: break
    return entries
```

응답: `{"ok": true, "count": N, "entries": [...]}`.

### Go 설계

```go
type CaptureHistoryArgs struct {
    Limit  int     `json:"limit,omitempty"`   // default 20, max 100
    Domain *string `json:"domain,omitempty"`
    Since  *string `json:"since,omitempty"`   // ISO date, lexicographic 비교
}

type CaptureHistoryResult struct {
    OK      bool                   `json:"ok"`
    Count   int                    `json:"count"`
    Entries []map[string]any       `json:"entries"`  // jsonl entry 그대로
}

func (s *lifecycleService) CaptureHistory(args CaptureHistoryArgs) (*CaptureHistoryResult, error) {
    limit := args.Limit
    if limit == 0 { limit = 20 }
    if limit > 100 { limit = 100 }  // Python min(limit, 100)

    path := filepath.Join(runeDir(), "capture_log.jsonl")
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return &CaptureHistoryResult{OK: true, Count: 0, Entries: []map[string]any{}}, nil
        }
        return nil, err  // Python은 빈 list 반환, Go는 에러 전파 (Python 동일로 하려면 return empty)
    }

    lines := bytes.Split(data, []byte{'\n'})
    var entries []map[string]any
    for i := len(lines) - 1; i >= 0; i-- {
        line := bytes.TrimSpace(lines[i])
        if len(line) == 0 { continue }
        var entry map[string]any
        if json.Unmarshal(line, &entry) != nil { continue }
        if args.Domain != nil {
            if d, _ := entry["domain"].(string); d != *args.Domain { continue }
        }
        if args.Since != nil {
            if ts, _ := entry["ts"].(string); ts < *args.Since { continue }
        }
        entries = append(entries, entry)
        if len(entries) >= limit { break }
    }

    return &CaptureHistoryResult{OK: true, Count: len(entries), Entries: entries}, nil
}
```

> Python은 read 실패 시 빈 list 반환 (`except Exception: return []`). Go도 동일 방식 유지 — 로컬 IO 에러를 API 에러로 변환하지 않음.

### 관련 결정
- **D20** (capture_log jsonl 포맷 bit-identical): 이 tool의 파싱 정확성을 보장하는 근거

### 구현 위치
- `internal/service/lifecycle.go` — `CaptureHistory`
- `internal/adapters/logio/reader.go` — reverse line reader helper

---

## 5. `rune_delete_capture`

### 책임
**Soft-delete**: 레코드를 물리 삭제하지 않고 status를 `"reverted"`로 변경 후 re-insert. Recall에서 `STATUS_MULTIPLIER["reverted"]=0.3`으로 자연 demote.

### Python 동작 (`server.py:1113-1206`)

```python
target = await searcher.search_by_id(record_id)  # "ID: {record_id}" embed hack
if not target: return make_error(InvalidInputError(...))

metadata = target.metadata
metadata["status"] = "reverted"

# Re-insert with updated metadata (new ciphertext)
embedding_text = metadata.get("reusable_insight").strip() or target.payload_text
insert_result = envector_client.insert_with_text(index, [embedding_text], embedding_service, [metadata])

_append_capture_log(record_id, target.title, target.domain, "soft-delete", action="deleted")
return {"ok": true, "deleted": true, "record_id", "title", "method": "soft-delete (status=reverted)"}
```

### Go 설계

```go
type DeleteCaptureArgs struct {
    RecordID string `json:"record_id"`
}

type DeleteCaptureResult struct {
    OK       bool   `json:"ok"`
    Deleted  bool   `json:"deleted"`
    RecordID string `json:"record_id"`
    Title    string `json:"title"`
    Method   string `json:"method"`    // "soft-delete (status=reverted)"
}

func (s *lifecycleService) DeleteCapture(ctx context.Context, args DeleteCaptureArgs) (*DeleteCaptureResult, error) {
    // Phase 1: search by id (Python `searcher.search_by_id`)
    target, err := s.searchByID(ctx, args.RecordID)
    if err != nil { return nil, wrapVaultError(err) }
    if target == nil {
        return nil, invalidInput(fmt.Sprintf("Record '%s' not found in search results. Use capture_history to find valid record IDs.", args.RecordID))
    }

    // Phase 2: mutate metadata
    target.Metadata["status"] = "reverted"

    // Phase 3: re-embed + re-insert
    embedText := firstNonEmpty(
        strings.TrimSpace(stringOr(target.Metadata["reusable_insight"])),
        target.PayloadText,
    )
    vec, err := s.embedder.EmbedSingle(ctx, embedText)
    if err != nil { return nil, wrapEmbedError(err) }

    aes, err := s.encryptMetadata(target.Metadata)  // Phase 5 (AES envelope) of capture flow 재사용
    if err != nil { return nil, err }

    if err := s.envector.Insert(ctx, s.indexName, [][]float32{vec}, []string{aes}); err != nil {
        s.state.SetDormant("envector_unreachable")
        return nil, wrapEnvectorError(err)
    }

    // Phase 4: append capture log
    _ = s.captureLog.Append(logio.CaptureLogEntry{
        TS: time.Now().UTC().Format(time.RFC3339),
        Action: "deleted",
        ID: args.RecordID,
        Title: target.Title,
        Domain: target.Domain,
        Mode: "soft-delete",
    })

    return &DeleteCaptureResult{
        OK: true, Deleted: true,
        RecordID: args.RecordID, Title: target.Title,
        Method: "soft-delete (status=reverted)",
    }, nil
}
```

### `searchByID` (Python `searcher.py:561-567`)

Python hack 그대로 유지 (D25/D27 원칙 연장):

```go
func (s *lifecycleService) searchByID(ctx context.Context, id string) (*SearchHit, error) {
    query := fmt.Sprintf("ID: %s", id)
    vec, err := s.embedder.EmbedSingle(ctx, query)
    if err != nil { return nil, err }
    hits, err := s.searchSingle(ctx, vec, 5)
    if err != nil { return nil, err }
    for _, h := range hits {
        if h.RecordID == id { return &h, nil }
    }
    return nil, nil  // not found
}
```

### 에러 (Python L1180-1206 매핑)

| 에러 | Python | Go | Side-effect |
|---|---|---|---|
| `VaultError` | `VaultConnectionError` + dormant | 동일 | `state.SetDormant("vault_unreachable")` |
| `ConnectionError`/`OSError` | `EnvectorConnectionError` + dormant | 동일 | `state.SetDormant("envector_unreachable")` |
| 기타 | `make_error(e)` | wrap+return | — |

### 관련 결정
- **D20** (capture_log append): soft-delete 기록도 동일 포맷
- Metadata re-encrypt는 capture flow Phase 3 (`record_builder`) + Phase 5 (AES envelope) 재사용

### 구현 위치
- `internal/service/lifecycle.go` — `DeleteCapture`
- `internal/service/search.go` — `searchByID` (recall 과 공유 가능)

---

## 6. `rune_reload_pipelines`

### 책임
`~/.rune/config.json`을 다시 읽어 scribe·retriever 파이프라인을 재초기화. 이후 envector `GetIndexList`로 **warmup** (connection + RegisterKey까지 pre-resolve).

### Python 동작 (`server.py:1038-1089`)

```python
self._pipelines_ready.wait()
self._pipelines_error = None
result = self._init_pipelines()

# Pre-warm envector (60s timeout)
if result["scribe"] and self.envector:
    envector.invoke_get_index_list()  # ThreadPool + 60s timeout

return {"ok": ..., "state": ..., "scribe_initialized": ..., "retriever_initialized": ...,
        "errors": ..., "envector_warmup": {"ok", "latency_ms" | "error"}}
```

### Go 설계

```go
type ReloadPipelinesResult struct {
    OK                    bool                `json:"ok"`
    State                 string              `json:"state"`
    ScribeInitialized     bool                `json:"scribe_initialized"`
    RetrieverInitialized  bool                `json:"retriever_initialized"`
    Errors                []string            `json:"errors,omitempty"`
    EnvectorWarmup        *WarmupInfo         `json:"envector_warmup,omitempty"`
}

type WarmupInfo struct {
    OK         bool     `json:"ok"`
    LatencyMs  *float64 `json:"latency_ms,omitempty"`
    Error      *string  `json:"error,omitempty"`
}

func (s *lifecycleService) ReloadPipelines(ctx context.Context) (*ReloadPipelinesResult, error) {
    // 이전 init을 완료될 때까지 기다림 (race 방지)
    s.state.AwaitInitDone()
    s.state.ClearError()

    res := s.state.ReinitPipelines(ctx)

    // Envector warmup (Python 60s)
    var warmup *WarmupInfo
    if res.ScribeInit && s.envector != nil {
        warmup = s.warmupEnvector(ctx, 60*time.Second)
    }

    return &ReloadPipelinesResult{
        OK: len(res.Errors) == 0,
        State: res.State,
        ScribeInitialized: res.ScribeInit,
        RetrieverInitialized: res.RetrieverInit,
        Errors: res.Errors,
        EnvectorWarmup: warmup,
    }, nil
}
```

### 관련 결정
- Warmup timeout 60s (Python `WARMUP_TIMEOUT`) — 이유: `RegisterKey`는 수 십초 가능 → diagnostics timeout(5s)과 구분

### 구현 위치
- `internal/service/lifecycle.go` — `ReloadPipelines`
- `internal/mcp/state.go` — `AwaitInitDone` · `ReinitPipelines`

---

## 결정 번호 색인

Lifecycle flow는 **새 결정을 발생시키지 않는다**. 기존 결정 재활용:

| Tool | 관련 결정 |
|---|---|
| vault_status | — (단순 프록시) |
| diagnostics | — (read-only 조회) |
| batch_capture | D14 · D16 (capture flow 재사용) |
| capture_history | D20 (jsonl 포맷 bit-identical) |
| delete_capture | D20 (로그) · capture Phase 3 (record_builder) + Phase 5 (AES envelope) 재사용 |
| reload_pipelines | — |

모두 **Python 동작 bit-identical** 원칙 (D25/D27 정신 연장). 새 쟁점이 발견되면 D30+로 추가.

## 패키지 레이아웃 (lifecycle 관련)

```
cmd/rune-mcp/main.go            # tool 등록
internal/mcp/
    tools.go                    # 모든 MCP tool handler
    state.go                    # 상태 머신 + SetDormant/AwaitInitDone/ReinitPipelines
internal/service/
    lifecycle.go                # VaultStatus · Diagnostics · CaptureHistory · DeleteCapture · ReloadPipelines
    capture.go                  # Batch (capture와 공유)
    diagnostics_classify.go     # envector error classification
internal/adapters/
    vault/client.go             # HealthCheck · Endpoint
    envector/client.go          # GetIndexList
    logio/reader.go             # reverse jsonl reader
```

## 테스트 전략

- **Unit**: 각 tool 함수 단위 + error classification 테이블
- **Golden JSON**: Python 응답 sample과 byte-identical 비교 (capture_history/diagnostics/vault_status)
- **Integration**: 로컬 capture_log.jsonl + 실제 Vault+envector로 e2e
- **Concurrency**: `reload_pipelines` 호출 중 다른 tool 호출 시 race 없는지 (`AwaitInitDone` 동작)
