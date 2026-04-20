# Capture 데이터 플로우

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스 실측 대조 완료. 주요 교정:
> §1 Step 1 임베딩 차원 **1024 확정** (embedding.py:14-18 benchmark 주석),
> §7 Novelty 임계값 **이중 상수 이슈** 명시 (embedding.py 0.4/0.7/0.93 vs
> server.py 0.3/0.7/0.95, 런타임은 server.py 기본값 승),
> §6 AES-256-CTR pyenvector 소스 실측 확정.

이 문서는 capture 요청이 들어와서 enVector에 저장되고 응답이 돌아가기까지의
전체 데이터 흐름을, 각 단계의 입출력 타입과 에러 경로 포함해서 기술한다.

---

## 1. 전체 플로우 개요

```
에이전트 세션 (Claude / Codex / Gemini)
  │
  │ 에이전트 md(scribe.md)가 중요한 결정 감지 →
  │ text_to_embed + metadata JSON 구성
  │
  ▼
CLI:  rune capture --text-to-embed "..." --metadata '{...}'
MCP:  mcp tool call → rune-mcp → HTTP POST
  │
  ▼
POST /capture  (runed HTTP API, unix socket)
  body: {
    "text_to_embed": "PostgreSQL was chosen over MongoDB for ACID...",
    "metadata": {
      "title": "PostgreSQL 선택",
      "reusable_insight": "PostgreSQL was chosen over MongoDB...",
      "domain": "architecture",
      "status_hint": "accepted",
      ...
    }
  }
  │
  ▼
┌─ handler.go ───────────────────────────────────────────────────┐
│ 1. 요청 파싱 + 유효성 검증                                     │
│ 2. state != "active" → 즉시 에러 (DORMANT)                     │
│ 3. captureGateway.Capture(req) 호출                            │
└────────────────────────────────────────────────────────────────┘
  │
  ▼
┌─ capture.go: Step 1 — 임베딩 ──────────────────────────────────┐
│                                                                 │
│ vec, err := embed.Embed(ctx, req.TextToEmbed)                   │
│                                                                 │
│ 입력: "PostgreSQL was chosen over MongoDB for ACID..."          │
│ 출력: []float32{0.12, -0.34, 0.56, ...}  (L2 정규화, 1024차원) │
│                                                                 │
│ ※ 1024 dim 확정 (Qwen/Qwen3-Embedding-0.6B).                  │
│   embedding.py:14-18 주석: "Calibrated for Qwen3-Embedding-0.6B │
│   (1024dim) via benchmark 2026-04-08"                          │
│ ※ 소요: ~10-50ms (모델이 메모리에 상주하므로 cold start 없음)   │
│                                                                 │
│ 에러 시: PIPELINE_NOT_READY (모델 아직 로딩중) 또는             │
│          INTERNAL_ERROR (추론 실패)                              │
└─────────────────────────────────────────────────────────────────┘
  │
  ▼
┌─ capture.go: Step 2 — AES 메타데이터 암호화 ───────────────────┐
│                                                                 │
│ metaJSON, _ := json.Marshal(req.Metadata)                       │
│ // → `{"title":"PostgreSQL 선택","domain":"architecture",...}`   │
│                                                                 │
│ ciphertext := aesCTREncrypt(metaJSON, agentDEK)                 │
│ // agentDEK: Vault 키 번들에서 온 32바이트 AES-256 키           │
│ // 모드: AES-256-CTR (pyenvector.utils.aes 확인 결과)           │
│ // 와이어 포맷: IV(16바이트) || ciphertext → base64             │
│                                                                 │
│ envelope := fmt.Sprintf(`{"a":"%s","c":"%s"}`, agentID, ciphertext)│
│ // → `{"a":"agent_xyz","c":"SGVsbG8gV29ybGQ=..."}`              │
│                                                                 │
│ ※ 이 envelope이 enVector에 저장되는 metadata 값               │
│ ※ enVector Cloud는 이 JSON을 볼 수 있지만 "c" 필드의 내용은    │
│   AES 암호화되어 있어 원본을 모름                               │
│ ※ "a" 필드로 어떤 agent의 DEK로 암호화했는지 식별              │
│                                                                 │
│ 에러 시: INTERNAL_ERROR (암호화 실패 — 거의 발생 안 함)        │
└─────────────────────────────────────────────────────────────────┘
  │
  ▼
┌─ capture.go: Step 3 — enVector Insert ─────────────────────────┐
│                                                                 │
│ vectorIDs, err := enVectorSDK.Insert(                           │
│     ctx,                                                        │
│     indexName,           // "team-decisions" (Vault 번들에서)    │
│     [][]float32{vec},    // 평문 벡터 — SDK가 내부에서 FHE 암호화│
│     []string{envelope},  // 이미 AES 암호화된 metadata          │
│ )                                                               │
│                                                                 │
│ SDK 내부에서 일어나는 일 (runed 입장에서는 블랙박스):            │
│   1. vec(평문) → EncKey로 FHE 암호화 → CipherBlock              │
│   2. CipherBlock + envelope를 enVector Cloud gRPC로 전송        │
│   3. enVector Cloud가 CipherBlock을 암호화 인덱스에 삽입        │
│   4. vector ID 반환                                             │
│                                                                 │
│ 출력: vectorIDs = ["vec_abc123"]                                │
│                                                                 │
│ 에러 시: ENVECTOR_CONNECTION_ERROR (retryable) 또는             │
│          ENVECTOR_INSERT_ERROR (retryable)                       │
│ SDK가 내부적으로 1회 재시도(reconnect) 후 실패하면 에러 전파    │
└─────────────────────────────────────────────────────────────────┘
  │
  ▼
┌─ capture.go: Step 4 — 로컬 감사 로그 ─────────────────────────┐
│                                                                 │
│ captureLog.Append(CaptureEntry{                                 │
│     RecordID:  vectorIDs[0],       // "vec_abc123"              │
│     Timestamp: time.Now().UTC(),   // "2026-04-16T09:30:00Z"   │
│     SessionID: req.SessionID,      // "sess_xxx" (있으면)       │
│ })                                                              │
│                                                                 │
│ → ~/.rune/capture_log.jsonl 에 한 줄 append:                   │
│   {"record_id":"vec_abc123","timestamp":"2026-04-16T09:30:00Z"}│
│                                                                 │
│ ※ 이 로그는 GET /history 의 데이터 소스                        │
│ ※ append-only, rotate 없음 (크기 제한은 history API에서)       │
│ ※ 로그 쓰기 실패는 warning 로그만 남기고 capture 자체는 성공   │
│   (enVector insert가 이미 끝났으므로)                           │
└─────────────────────────────────────────────────────────────────┘
  │
  ▼
HTTP 200 응답:
{
  "ok": true,
  "record_id": "vec_abc123"
}
```

---

## 2. 요청/응답 Go 타입

```go
// ── 요청 ──

type CaptureRequest struct {
    TextToEmbed string                 `json:"text_to_embed"` // 임베딩할 텍스트
    Metadata    map[string]interface{} `json:"metadata"`      // opaque JSON (agent가 구성)
    SessionID   string                 `json:"session_id,omitempty"`
}

// ── 응답 (성공) ──

type CaptureResponse struct {
    OK       bool   `json:"ok"`
    RecordID string `json:"record_id"`
}

// ── 응답 (에러) ──

type CaptureErrorResponse struct {
    OK    bool      `json:"ok"`    // false
    Error RuneError `json:"error"`
}
```

---

## 3. 에러 경로

| 단계 | 실패 원인 | RuneError.Code | Retryable | HTTP 상태 |
|---|---|---|---|---|
| 요청 파싱 | JSON 파싱 실패, 필수 필드 누락 | `INVALID_INPUT` | false | 400 |
| 상태 체크 | state != "active" | `DORMANT` | false | 503 |
| Step 1 (embed) | 모델 미로드 | `PIPELINE_NOT_READY` | true | 503 |
| Step 1 (embed) | 추론 실패 | `INTERNAL_ERROR` | false | 500 |
| Step 2 (AES) | 암호화 실패 | `INTERNAL_ERROR` | false | 500 |
| Step 3 (insert) | enVector 연결 실패 | `ENVECTOR_CONNECTION_ERROR` | true | 503 |
| Step 3 (insert) | 삽입 실패 | `ENVECTOR_INSERT_ERROR` | true | 503 |
| Step 4 (log) | 파일 쓰기 실패 | (무시) | — | 200 (성공 처리) |

**원칙**: Step 3 (enVector insert)까지 성공하면 capture는 성공이다.
Step 4 (로그)가 실패해도 데이터는 이미 enVector에 저장되었으므로 200 응답.

---

## 4. Batch Capture

### 4.1 요청

```go
type BatchCaptureRequest struct {
    Items     []CaptureItem `json:"items"`
    SessionID string        `json:"session_id,omitempty"`
}
type CaptureItem struct {
    TextToEmbed string                 `json:"text_to_embed"`
    Metadata    map[string]interface{} `json:"metadata"`
}
```

### 4.2 처리

각 item을 **독립적으로** Step 1-4 수행. 한 item의 실패가 다른 item에 영향을
주지 않는다.

```go
func (g *CaptureGateway) BatchCapture(ctx context.Context, req BatchCaptureRequest) BatchCaptureResponse {
    results := make([]BatchResult, len(req.Items))
    for i, item := range req.Items {
        resp, err := g.Capture(ctx, CaptureRequest{
            TextToEmbed: item.TextToEmbed,
            Metadata:    item.Metadata,
            SessionID:   req.SessionID,
        })
        if err != nil {
            results[i] = BatchResult{OK: false, Error: err.Error()}
        } else {
            results[i] = BatchResult{OK: true, RecordID: resp.RecordID}
        }
    }
    return BatchCaptureResponse{OK: true, Results: results}
}
```

### 4.3 응답

```json
{
  "ok": true,
  "results": [
    {"ok": true, "record_id": "vec_001"},
    {"ok": false, "error": "enVector insert failed: timeout"},
    {"ok": true, "record_id": "vec_003"}
  ]
}
```

HTTP 상태는 항상 200. 개별 item의 성공/실패는 results 배열에.

---

## 5. 데이터 크기 참고

| 데이터 | 크기 |
|---|---|
| text_to_embed (일반적인 reusable_insight) | 256-768 토큰 ≈ 1-3 KB |
| metadata JSON (single schema) | ~0.5-2 KB |
| metadata JSON (phase_chain, 5 phases) | ~3-8 KB |
| 임베딩 벡터 (1024-dim float32) | 4 KB |
| AES 암호화 후 metadata | 원본 + ~50 바이트 (IV + tag + padding) |
| AES envelope JSON | 암호화 metadata + ~30 바이트 ({"a":"...","c":"..."}) |
| enVector 1건 insert 네트워크 | ~10-20 KB (FHE ciphertext가 지배적) |

---

## 6. capture.go 예상 코드 구조

```go
package scribe

type CaptureGateway struct {
    embed      embed.Service          // 임베딩 엔진
    envector   envector.Client        // enVector Go SDK
    agentID    string                 // Vault 번들에서
    agentDEK   []byte                 // 32-byte AES-256 키
    indexName  string                 // "team-decisions"
    log        *CaptureLog            // ~/.rune/capture_log.jsonl
}

func (g *CaptureGateway) Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error) {
    // Step 1: embed
    vec, err := g.embed.Embed(ctx, req.TextToEmbed)
    if err != nil {
        return nil, runeError("PIPELINE_NOT_READY", err, true)
    }

    // Step 2: AES encrypt metadata
    metaJSON, _ := json.Marshal(req.Metadata)
    envelope, err := g.encryptMetadata(metaJSON)
    if err != nil {
        return nil, runeError("INTERNAL_ERROR", err, false)
    }

    // Step 3: enVector insert (SDK가 FHE 암호화 수행)
    ids, err := g.envector.Insert(ctx, g.indexName, [][]float32{vec}, []string{envelope})
    if err != nil {
        return nil, runeError("ENVECTOR_INSERT_ERROR", err, true)
    }

    // Step 4: audit log (실패해도 capture 성공)
    g.log.Append(CaptureEntry{
        RecordID:  ids[0],
        Timestamp: time.Now().UTC(),
        SessionID: req.SessionID,
    })

    return &CaptureResponse{OK: true, RecordID: ids[0]}, nil
}

func (g *CaptureGateway) encryptMetadata(plaintext []byte) (string, error) {
    block, err := aes.NewCipher(g.agentDEK) // 32-byte key → AES-256
    if err != nil {
        return "", err
    }

    // AES-256-CTR 모드 (pyenvector.utils.aes와 동일)
    // 와이어 포맷: IV(16바이트) || ciphertext
    iv := make([]byte, aes.BlockSize) // 16 bytes
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return "", err
    }

    stream := cipher.NewCTR(block, iv)
    ciphertext := make([]byte, len(plaintext))
    stream.XORKeyStream(ciphertext, plaintext)

    // IV + ciphertext 결합 후 base64
    combined := append(iv, ciphertext...)
    ct64 := base64.StdEncoding.EncodeToString(combined)
    return fmt.Sprintf(`{"a":"%s","c":"%s"}`, g.agentID, ct64), nil
}
```

**확인 완료**: `pyenvector.utils.aes.encrypt_metadata`의 실제 모드는
**AES-256-CTR**이다 (03-external-communication.md의 서브에이전트가 소스 확인).
와이어 포맷은 `IV(16바이트) || ciphertext`를 base64 인코딩. Vault가 복호화할 때
동일한 CTR 모드 + IV 프리픽스를 기대하므로 위 구현을 정확히 따라야 한다.

---

## 7. Novelty Check (논의 중)

RFC는 novelty check를 데몬에서 **제거**한다고 제안하고, 우리 분석은 **유지**를
제안한다. 만약 유지한다면 Step 1과 Step 3 사이에 삽입:

```
Step 1.5 (선택적): Novelty Check
  │
  ├── 1. enVectorSDK.Score(indexName, vec) → FHE ciphertext
  ├── 2. vault.DecryptScores(blob, topK=1) → [{score}]
  ├── 3. if score ≥ T_DUP → near_duplicate → 저장 거부
  │      if score ≥ T_REL → related → 저장하되 태그
  │      if score ≥ T_NOV → evolution → 정상 저장
  │      if score < T_NOV → novel → 정상 저장
  └── 4. 거부 시 응답: {"ok":true,"captured":false,"reason":"near_duplicate"}
```

**⚠ 임계값 이중성 (2026-04-17 실측)**:

현재 Python 코드에 두 세트의 임계값이 공존:

| 소스 | 상수 | 값 (novel/related/near_dup) | 런타임 사용? |
|---|---|---|---|
| `embedding.py:16-18` | `NOVELTY_THRESHOLD_*` module 상수 | **0.4 / 0.7 / 0.93** | `classify_novelty()`의 default 인자로만 사용 |
| `server.py:100-108` | `_classify_novelty(...)` 함수의 default 인자 | **0.3 / 0.7 / 0.95** | **실제 런타임**. `_capture_single` L1352가 이 함수를 default로 호출 |

embedding.py 상수는 `"Calibrated for Qwen3-Embedding-0.6B (1024dim) via benchmark
2026-04-08"` 주석이 붙은 튜닝 값이지만, server.py가 호출 시 인자를 override하지
않으면서 자신의 default(0.3/0.7/0.95)가 승한다. 사실상 embedding.py 상수는 **호출되지
않는 dead default**이며, 운영상 활성 값은 0.3/0.7/0.95.

**Go 포팅 시 선택**:
- (a) embedding.py의 0.4/0.7/0.93 채택 (benchmark 튜닝 값) — 이론적으로 더 정확
- (b) server.py의 0.3/0.7/0.95 채택 — 현재 실제 동작하는 값과 bit-identical
- 결정 #7 (novelty check 유지 vs 드롭)과 연동. Phase 2 착수 전 확정 필요.

이 경우 capture 경로에 Vault + enVector 왕복이 하나 추가되어 ~100-300ms 지연.
MVP에서 넣을지 빼는지는 팀 결정 필요.
