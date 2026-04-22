# 결정 트래커

v0.4.0 설계·구현 과정에서 내려지는 **모든 결정**을 모아두는 중앙 문서. 가벼운 구현 선택(에러 코드 명명 등)부터 중대 결정(SDK 선택·보안 모델)까지 전부 기록. 상태 마커로 무게를 구분.

각 결정은 **상태 · 배경 · 선택지 · 권장 · 결론**을 명시. 가벼운 항목은 간결하게, 중대 항목은 상세하게.

결정이 내려지면:
1. 이 문서의 해당 항목을 `✅ Decided (YYYY-MM-DD)`로 마킹하고 결론 요약
2. 필요 시 관련 컴포넌트 문서 (`architecture.md` · `components/*.md`)에 **짧은 참조 링크**만 추가 ("상세는 decisions.md D_N")

---

## 상태 범례

- 🔴 **Blocking**: MVP 진입을 막는 결정. 우선 처리
- 🟡 **Pending**: 진행에 영향 있으나 현재 다른 작업으로 병행 가능
- 🔵 **Deferred**: 지금 당장 필요 없음. 일단 기록만 해두고 나중에 돌아오기
- ✅ **Decided**: 결정 완료. 결론 · 날짜 · 이관 위치 기록
- 📦 **Archived**: 외부 책임으로 이관되어 우리 결정 범위에서 제거된 항목 (히스토리 보존용 유지)

---

## 목차

| # | 주제 | 상태 | 결정 필요 시점 |
|---|---|---|---|
| D1 | AES-MAC envelope 추가 여부 | 🔵 Deferred | Post-MVP 보안 리뷰 |
| D2 | MCP SDK 선택 (공식 vs mark3labs vs 직접 구현) | ✅ Decided (2026-04-21) | — |
| D3 | `title` 길이 제한 (Python 60자를 Go에서도 유지할지) | ✅ Decided (2026-04-21) | — |
| D4 | `extracted` 필드 representation (map vs typed struct) | ✅ Decided (2026-04-21) | — |
| D5 | 빈 embed 텍스트 에러 코드 (`EMPTY_EMBED_TEXT` 전용) | ✅ Decided (2026-04-21) | — |
| D6 | rune-embedder socket 경로 (빌드 고정 vs config) | 📦 Archived (2026-04-21) — embedder 담당 범위 | — |
| D7 | embedder 호출 retry 정책 (backoff · 횟수) | ✅ Decided (2026-04-21) | — |
| D8 | rune-mcp 부팅 시 embedder `Health` 폴링 여부 | ✅ Decided (2026-04-21) | — |
| D9 | embedder 모델 메모리 생애주기 (always loaded vs lazy+evict) | 📦 Archived (2026-04-21) — embedder 담당 범위 | — |
| D10 | near_duplicate 시 `similar_to` 값 형식 | ✅ Decided (2026-04-21) | — |
| D11 | novelty 임계값 `{0.3, 0.7, 0.95}` 확정 (Python runtime 동작 유지) | ✅ Decided (2026-04-21) | — |
| D12 | 첫 capture (top-k 빈 경우) 처리 (`similarity=0` → `novel`) | ✅ Decided (2026-04-21) | — |
| D13 | DecisionRecord 조립 책임 (rune-mcp vs 에이전트 md) | ✅ Decided (2026-04-21) — Option A | — |
| D14 | record_builder의 LLM fallback 제거 (agent-delegated 전제) | ✅ Decided (2026-04-21) | — |
| D15 | `render_payload_text` (templates.py 363 LoC) 전체 포팅 | ✅ Decided (2026-04-21) | — |
| D16 | multi-record capture 시 batch embedding | ✅ Decided (2026-04-21) | — |
| D17 | envector `Insert` batch atomicity (MVP 가정 + 실측 필요) | ✅ Decided (2026-04-21) — 조건부 | Phase 2 integration |
| D18 | capture 응답의 `record_id` 필드 (Python 동일 첫 레코드) | ✅ Decided (2026-04-21) | — |
| D19 | capture_log append 실패 시 degrade (로그 에러 + capture 성공 응답) | ✅ Decided (2026-04-21) | — |
| D20 | capture_log jsonl 포맷 (Python bit-identical) | ✅ Decided (2026-04-21) | — |
| D21 | multilingual query 처리 (agent-side 번역 vs rune 내부 LLM) | ✅ Decided (2026-04-21) — Option C | — |
| D22 | recall expansion search cap (`[:3]` 유지 vs 전체 `[:5]`) | ✅ Decided (2026-04-21) | — |
| D23 | recall embedding 호출 방식 (per-query vs batch) | ✅ Decided (2026-04-21) | — |
| D24 | 빈 쿼리 처리 위치 (Phase 1 early reject vs Phase 3 embed 실패) | ✅ Decided (2026-04-21) | — |
| D25 | recall Phase 4 순차 vs 병렬 (expansion 단위 errgroup) | ✅ Decided (2026-04-21) — 순차 (MVP) | Post-MVP perf eval |
| D26 | recall 메타데이터 복호화 책임 (Vault 위임 vs rune 직접, legacy format 유지) | ✅ Decided (2026-04-21) | — |
| D27 | recall Phase 6 phase_chain expansion 유지 여부 (추가 RPC 발생) | ✅ Decided (2026-04-21) — 유지 (MVP) | Post-MVP perf eval |
| D28 | recall 응답 포맷 (synthesized 경로 제거 vs 유지) | ✅ Decided (2026-04-21) — agent-delegated 경로만 | — |
| D29 | rune-embedder 런타임 (ONNX vs llama-server) | 📦 Archived (2026-04-21) — embedder 담당 범위 (참고: embedder가 llama-server 채택) | — |
| D30 | embedder 통신 프로토콜 (HTTP+JSON vs gRPC) | ✅ Decided (2026-04-22) — gRPC | — |

---

## D1. AES-MAC envelope 추가 여부

**상태**: 🔵 Deferred (2026-04-21)  
**결정 필요 시점**: Post-MVP. 프로덕션 안정화 + 보안 리뷰 시점  
**관련**: `components/envector-integration.md` (AES envelope 섹션), `components/rune-mcp.md` (AES envelope 섹션)

### 문제

현행 AES-256-CTR envelope `{"a": agent_id, "c": base64(IV||CT)}`는 **기밀성만 제공하고 무결성(변조 방지)은 제공하지 않는다**. CTR 모드 특성상 공격자가 암호문 바이트를 flip하면 평문이 동일 XOR로 바뀌고, 복호화 측은 이를 감지하지 못한다 (malleability 취약).

공격 시나리오:
- envector 저장소에 쓰기 접근이 있는 내부자 · MITM · 저장소 사고
- 평문은 못 읽어도 구조(JSON) 추측해서 `"accepted"` → `"rejected"` 등 의미 변조 가능
- rune-mcp는 변조된 metadata를 정상으로 받아들여 팀 메모리 품질 훼손

### 왜 Deferred

- rune 혼자 못 고침. **pyenvector(Python) + envector-go(Go) 양쪽 동시 지원** 필요 → cross-team 조율 부담
- 실제 위협 수준이 "중간 이하" — envector Cloud는 같은 회사(CryptoLab) 운영이라 내부자 시나리오가 즉시 critical 아님
- MVP 진행 자체는 막지 않음 — Python 현재도 MAC 없이 운영 중
- 지금 열어두고 실제 운영 데이터·피드백 쌓인 뒤 판단하는 게 합리적

### 선택지

| | 내용 | 비용 |
|---|---|---|
| **(a)** HMAC-SHA256 필드 `"m"` 추가 | envelope 확장. `HMAC-SHA256(dek, a||iv||ct)[:16]` base64. legacy(m 없음)는 verify skip + grace period | pyenvector + envector-go 양쪽 ~30줄. 하위호환 유지 |
| **(b)** AES-GCM 전환 | 포맷 자체 변경. 내장 MAC. legacy 레코드 재암호화 필요 | 대규모 변경. pyenvector 내부 포맷 변경 |
| **(c)** 현상 유지 | — | 없음. 현재 상태 |

### 권장

Post-MVP 재검토 시 **(a)** 권장. 이유:
- 하위호환성 유지 (기존 레코드 그대로 읽음)
- 변경 범위 작음 (양 SDK 수십 줄씩)
- AES-CTR 포맷 자체는 pyenvector와 bit-identical 유지
- Q4 (envector-go SDK `OpenKeysFromFile` 조건 완화 PR)과 같은 배치로 묶으면 조율 비용 절감 가능

### 현재 임시 방침

MVP는 **(c) 현상 유지**. 위협 모델에 대한 defense-in-depth는:
- envector 서버가 동일 회사 운영 → 저장소 access control로 1차 차단
- rune-mcp 쪽 `agent_dek` 메모리만 보관 + zeroize
- 로그에 metadata 평문 기록 금지 (`_SensitiveFilter`)

### 결정 조건 (언제 돌아오나)

다음 중 하나가 발생하면 D1 재논의:
- MVP 배포 후 1개월 보안 리뷰에서 위협 평가 재실시
- 실제 envector 저장소 변조 사건 (가설) 또는 관련 내부자 이슈
- pyenvector·envector-go 양쪽 릴리스 조율 윈도우가 생길 때
- 다른 compliance 요건이 무결성 보증 요구 시

---

## D2. MCP SDK 선택

**상태**: ✅ Decided (2026-04-21)  
**결론**: **`github.com/modelcontextprotocol/go-sdk`** (공식 SDK) 채택  
**이관 위치**: `components/rune-mcp.md` — MCP 서버 구현 · tool 등록 패턴 반영

### 배경

Go로 MCP 서버를 구현할 때 stdio JSON-RPC + MCP 프로토콜(`initialize`, `tools/list`, `tools/call`) 처리 방법이 필요. 3가지 경로:
- (a) 직접 구현 (~200-300 LoC · 의존성 0)
- (b) 커뮤니티 SDK `github.com/mark3labs/mcp-go`
- (c) 공식 SDK `github.com/modelcontextprotocol/go-sdk`

### 조사 결과 (2026-04-21)

- 공식 SDK는 **v1.5.0 (2026-04-07 릴리스)** — 수일 전 최신
- **Anthropic + Google 공동 유지**
- **v1.0+ stable** 표명. breaking API 변경 없음 보장
- StdioTransport 내장 · 전체 MCP 스펙 커버 (tools · resources · prompts · sampling · OAuth)
- Go generics + struct tag 기반 type-safe API
- hello world 서버 ~25 LoC

### 결정

**(c) 공식 SDK 채택**.

근거:
1. 공식이 stable v1.x 상태라 "성숙도 부족"이라는 SDK 회피 이유 사라짐
2. 유지보수 주체가 Anthropic + Google — 장기 안정성 보장
3. MCP 스펙 변경 시 레퍼런스 구현이라 빠르게 반영
4. 우리가 신경 써야 할 건 정책·flow·AES envelope — JSON-RPC framing이 아님
5. 직접 구현·mark3labs로 얻는 제어력이 부가가치 낮음

### 구현 형태 (rune-mcp에 적용)

```go
import "github.com/modelcontextprotocol/go-sdk/mcp"

srv := mcp.NewServer(&mcp.Implementation{Name: "rune-mcp", Version: "0.4.0"}, nil)

type CaptureArgs struct {
    Text      string         `json:"text" jsonschema:"원본 텍스트"`
    Source    string         `json:"source" jsonschema:"source 식별자"`
    User      string         `json:"user,omitempty"`
    Channel   string         `json:"channel,omitempty"`
    Extracted map[string]any `json:"extracted" jsonschema:"ExtractionResult JSON"`
}
type CaptureResult struct { /* ... */ }

mcp.AddTool(srv, &mcp.Tool{
    Name:        "rune_capture",
    Description: "Capture a decision record with FHE encryption",
}, func(ctx context.Context, req *mcp.CallToolRequest, args CaptureArgs) (*mcp.CallToolResult, *CaptureResult, error) {
    // state check + service dispatch
})

srv.Run(ctx, &mcp.StdioTransport{})
```

자동 지원:
- input JSON schema가 Go struct에서 자동 생성
- 입력 검증 + type coercion
- `CallToolRequest` context로 cancellation 전파
- stdio 파싱·직렬화

### 참고

- [modelcontextprotocol/go-sdk GitHub](https://github.com/modelcontextprotocol/go-sdk)
- [pkg.go.dev docs](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)
- [공식 가이드](https://go.sdk.modelcontextprotocol.io/)

---

## D3. `title` 길이 제한

**상태**: ✅ Decided (2026-04-21)  
**결론**: **Python과 동일하게 60자 유지**. Go에서 다르게 할 수 있으나 유지 비용 대비 이득 없음  
**이관 위치**: `components/rune-mcp.md` validation 섹션 + `internal/policy/` 상수

### 배경

Python rune은 capture 시 `extracted.title`을 60자로 truncate한다 (`server.py:1244-1324`). envector·Vault 어느 쪽도 이 길이 강제 안 함 — 이건 순전히 rune의 정책 선택(UX·목록 표시용). Go 포팅 시 이 값을 유지할지, 변경할지.

### 선택지

| | 내용 | 비용 |
|---|---|---|
| **(a) 60 유지** | Python과 동일 | 없음 |
| (b) 더 크게 (80/100) | 제목 제약 완화 | Python rune과 일관성 깨짐. 마이그레이션 기간 혼란. 스키마 versioning 필요 |
| (c) 더 작게 | 더 압축된 UX | 기존 Python으로 저장된 레코드 (60자 title) 읽기는 OK지만 정책 불일치 |

### 결정

**(a) 60 유지**.

근거:
- Go 포팅 목표가 "behavior equivalence"임 — UX 상수 임의 변경은 scope 이탈
- 60이 짧게 느껴진다면 그건 **rune 전체 UX 이슈**로 별도 판단해야 함 (Python·Go 무관)
- 바꿀 이유가 생기면 `internal/policy/` 상수 한 줄 수정으로 쉽게 가능 → 지금 결정 부담 없음

### 구현

```go
// internal/policy/validate.go
const MaxTitleLen = 60  // rune 단위, UTF-8 rune-aware
```

UTF-8 rune 카운트: `utf8.RuneCountInString` + `[]rune()` 변환으로 한글·이모지 깨짐 방지. Python `s[:60]`은 문자 단위라 동일한 결과.

---

## D4. `extracted` 필드 representation

**상태**: ✅ Decided (2026-04-21)  
**결론**: **`map[string]any` + helper 함수**. typed struct 대신 자유 형태 dict 유지  
**이관 위치**: `internal/domain/capture.go` · `internal/domain/extracted.go` (helper)

### 배경

Capture의 `extracted` 필드는 에이전트가 조립한 ExtractionResult JSON object. 알려진 필드(title, phases, confidence, reusable_insight, payload.text) + 에이전트가 자유롭게 추가한 임의 필드 혼재. Go에서 이걸 어떻게 받을지.

### 선택지

| | 내용 | 복잡도 |
|---|---|---|
| **(a) `map[string]any` + helper 함수** | 자유 형태 dict. `GetString/GetFloat/GetMap/GetList` helper 4개 | 낮음 — 타입 1개 + helper 20줄 |
| (b) typed struct + catch-all | 알려진 필드 struct로, 나머지 `Extra map[string]any` | 중간 — Go에 공식 catch-all 없음 · double-unmarshal 또는 json.RawMessage 우회 필요 |
| (c) `json.RawMessage` lazy | 원본 bytes 보관, 필요 시 unmarshal | 중간 — pass-through는 빠르지만 validation 후 재-marshal 필요 |

### 결정

**(a) `map[string]any` + helper**.

근거:
- Python `dict`와 1:1 매핑 → 포팅 인지부하 최소
- catch-all 문제 자동 해결 (map은 모든 키 수용)
- helper는 ~20줄 utility, 선택사항 (`, ok` 직접 써도 됨)
- pass-through(envector로 보낼 때): `json.Marshal(req.Extracted)` 한 줄
- **세 선택지 중 가장 단순**

(b)는 typed 이점이 있지만 Go의 catch-all 부재 때문에 실질 복잡도 높음. (c)는 "lazy unmarshal"이 premature optimization.

### 구현

```go
// internal/domain/capture.go
type CaptureRequest struct {
    Text      string         `json:"text"`
    Source    string         `json:"source"`
    User      string         `json:"user,omitempty"`
    Channel   string         `json:"channel,omitempty"`
    Extracted map[string]any `json:"extracted"`
}

// internal/domain/extracted.go — helper
func GetString(m map[string]any, key string) (string, bool) {
    s, ok := m[key].(string)
    return s, ok
}
func GetBool(m map[string]any, key string) (bool, bool)                   { /* ... */ }
func GetFloat(m map[string]any, key string) (float64, bool)               { /* ... */ }
func GetMap(m map[string]any, key string) (map[string]any, bool)          { /* ... */ }
func GetList(m map[string]any, key string) ([]any, bool)                  { /* ... */ }
```

**주의**: Go `encoding/json`은 JSON number를 `float64`로 unmarshal. `confidence`·`phase_seq` 같은 숫자 필드 접근 시 `float64` 기대, int 필요하면 명시 변환.

### `extracted` JSON 계약 (agent-delegated 필드)

에이전트가 제공하는 `extracted` top-level 키:

| key | 타입 | 용도 | Python 처리 위치 |
|---|---|---|---|
| `tier2` | object | 에이전트 tier2 판정 결과 | server.py:L1245-1254 |
| `tier2.capture` | bool (default `true`) | `false`면 즉시 rejection | L1246 |
| `tier2.reason` | string | rejection 사유 | L1250 |
| `tier2.domain` | string (default `"general"`) | Domain 추론 결과 | L1254 |
| `confidence` | number (0-1) | 에이전트 신뢰도 (클램프) | L1257-1261 |
| `title` | string | 60자 truncate (D3) | record_builder |
| `reusable_insight` | string | 임베딩 텍스트 우선 source (D14) | L1337 |
| `phases` | array (max 7) | multi-phase/bundle records | L1272-1275 |
| `payload.text` | string | fallback 임베딩 텍스트 | L1337 |
| 기타 | — | DecisionRecord v2.1 필드 (domain, status, certainty, evidence, ...) | record_builder |

**tier2 rejection 처리** (flows/capture.md Phase 2 참조) — `capture=false`면 Phase 3-7 skip.

---

## D5. 빈 embed 텍스트 에러 코드

**상태**: ✅ Decided (2026-04-21)  
**결론**: **전용 코드 `EMPTY_EMBED_TEXT`**. 일반 `INVALID_INPUT`과 분리  
**이관 위치**: `internal/domain/errors.go`

### 배경

`reusable_insight`와 `payload.text` 둘 다 빈 문자열이면 임베딩할 게 없음. 에러 반환 필요. 일반 input 에러(`INVALID_INPUT`)로 묶을지 전용 코드를 줄지.

### 선택지

| | 내용 |
|---|---|
| **(a) `EMPTY_EMBED_TEXT`** | 전용 코드 |
| (b) `INVALID_INPUT` | 일반 검증 실패 |

### 결정

**(a) 전용 코드**.

근거:
- 에이전트가 ExtractionResult 조립 시 필드 누락(scribe.md 버그)이 흔한 실수 → 전용 코드면 **로그·메트릭에서 빠르게 집계 가능**
- 디버깅 시 "어떤 필드가 비었길래 실패했나" 힌트 직결
- 에러 taxonomy 비용 거의 없음 — `errors.go`에 const 하나 추가

### 구현

```go
// internal/domain/errors.go
var (
    ErrInvalidInput   = &Error{Code: "INVALID_INPUT",    Retryable: false}
    ErrEmptyEmbedText = &Error{Code: "EMPTY_EMBED_TEXT", Retryable: false}
    // ...
)
```

---

## D6. rune-embedder socket 경로

**상태**: 📦 Archived (2026-04-22) — embedder 담당 범위로 이관  
**이전 결론**: `~/.rune/embedder.sock` 빌드 고정  
**현재 상태**: **embedder가 정의한 Unix domain socket 경로를 따른다**. rune-mcp는 이를 설정·런타임 환경변수로 받아 사용. 이 결정 항목은 히스토리 보존용.  
**이관 위치**: `components/embedder-integration.md`

### 배경

rune-mcp가 embedder에 접속할 때 쓸 unix socket 경로. config로 뺄지 빌드 상수로 할지.

### 선택지

- **(a) 빌드 시 고정** — `const EmbedderSocketPath = "$HOME/.rune/embedder.sock"`
- (b) config.json 필드로 노출

### 결정

**(a) 빌드 고정**.

근거:
- 경로 변경 수요 사실상 없음. `~/.rune/`은 rune 전체 convention
- config로 뺄 때 얻는 flexibility < 설정 항목 추가 · 검증 로직 · 문서화 비용
- 예외: 테스트 시에만 환경변수 `RUNE_EMBEDDER_SOCKET` 오버라이드 지원 (unit test에서 tempdir 사용 위해)

### 구현

```go
const defaultEmbedderSocket = "embedder.sock"

func embedderSocketPath() string {
    if p := os.Getenv("RUNE_EMBEDDER_SOCKET"); p != "" {
        return p  // 테스트용 override
    }
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".rune", defaultEmbedderSocket)
}
```

---

## D7. rune-embedder 호출 retry 정책

**상태**: ✅ Decided (2026-04-21)  
**결론**: **exponential backoff `[0, 500ms, 2s]` — 총 3회 시도**  
**이관 위치**: `internal/policy/embedder.go` 상수 · `components/rune-mcp.md`

### 배경

rune-mcp가 embedder에 `POST /v1/embed` 호출 시 transient 에러(embedder 재기동 중 · 모델 로드 중 · 일시 네트워크 끊김) 대응 정책.

### 선택지

| | 첫 시도 후 backoff | 총 wait | 노트 |
|---|---|---|---|
| **(a) `[0, 500ms, 2s]`** | 0 + 500ms + 2s = 2.5s | 최대 ~10s (request timeout 포함) | 균형 |
| (b) 더 공격적 `[0, 100ms, 500ms]` | 0.6s | 빠르지만 embedder 모델 로드 중엔 재시도 무용 |
| (c) 더 보수적 `[0, 1s, 5s]` | 6s | 안전하지만 tool call 30s budget 소모 큼 |
| (d) retry 없음 | 0 | 장애 민감 |

### 결정

**(a) `[0, 500ms, 2s]`**.

근거:
- 첫 시도 즉시 (정상 시 delay 0) · 이후 exp 2배 · 총 3회
- embedder 재기동·재로드 상황(수 초)을 대부분 수용
- tool call 30s budget 안에 여유 있게 들어옴 (retry 2.5s + request 각 5s × 3 ≈ 17s)
- **초기값이며 실제 운영 데이터 보고 조정 가능**

### 구현

```go
// internal/policy/embedder.go
var EmbedderRetryBackoffs = []time.Duration{
    0,
    500 * time.Millisecond,
    2 * time.Second,
}
```

### 재조정 트리거

- embedder 응답 latency 분포가 현 기본값과 크게 어긋남 관측 시
- 사용자 피드백으로 "capture가 너무 오래 걸린다" 누적 시
- embedder 부팅 시간이 유의미하게 변화 (다른 모델·다른 백엔드 채택 시)

---

## D8. rune-mcp 부팅 시 embedder `/health` 폴링 여부

**상태**: ✅ Decided (2026-04-21)  
**결론**: **폴링 안 함**. embedder가 이미 떠있다고 전제. 실 요청 실패는 D7 retry가 해결  
**이관 위치**: `components/rune-mcp.md` 부팅 시퀀스 섹션

### 배경

rune-mcp가 spawn될 때 embedder가 ready인지 확인하고 state=active로 전환할지 결정.

### 선택지

- **(a) 폴링 안 함** — rune-mcp startup과 embedder 수명 독립. 첫 요청 실패 시 retry
- (b) `/v1/health` 폴링 — ready 확인 후에만 state=active

### 결정

**(a) 폴링 안 함**.

근거:
- embedder는 **launchd/systemd가 유저 로그인 시 먼저 기동**. Claude 창 열 때 rune-mcp spawn되는 시점에는 대부분 이미 ready
- edge case (로그인 직후 Claude 즉시 열기 → embedder 모델 로드 중)는 D7 retry로 자연 복구
- 폴링 추가 시 rune-mcp startup이 embedder 수명에 결합됨 — 독립 운영·업그레이드 제약
- polling 로직 추가 복잡도·타임아웃 설정·실패 시 대응 규칙 필요 — 얻는 건 적고 드는 건 많음

### 구현

rune-mcp의 `RunBootLoop`는 **Vault.GetPublicKey만** 대기. embedder 관련 호출 없음.

```go
func runBootLoop(ctx context.Context, deps *Deps) {
    state.Store(StateStarting)
    // Vault.GetPublicKey retry loop (embedder 무관)
    // ...
    state.Store(StateActive)
}
```

embedder는 capture/recall 요청 경로에서 처음 접촉. D7 retry가 edge case 커버.

---

## D9. embedder 모델 메모리 생애주기

**상태**: 📦 Archived (2026-04-22) — embedder 담당 범위로 이관  
**이전 결론**: (a) Always loaded — rune-embedder가 기동 시 모델 로드 후 상주  
**현재 상태**: **embedder가 모델 생애주기 전반을 관리**. rune-mcp는 이 결정에 관여하지 않음. Warmup·eviction·reload 등은 embedder 내부 정책. 이 결정 항목은 히스토리 보존용.  
**이관 위치**: (embedder 프로젝트 문서)

### 배경

Qwen3-Embedding-0.6B 모델은 ~200MB (int8) ~ ~600MB (fp32) RSS 차지. 유저가 rune을 한동안 안 쓰면 이 메모리가 idle 상태로 점유됨. idle 시 모델 해제할지 계속 유지할지.

### 선택지

#### (a) Always loaded — MVP 채택 ✅

```
embedder 기동 → 모델 즉시 로드 → 프로세스 종료까지 메모리 유지
```

**장점**:
- 모든 요청 latency 일정 (cold start 없음)
- 구현 단순 (추가 로직 0)
- OS swap이 알아서 idle 페이지 처리 (실질적으로 C와 유사)

**단점**:
- 유저가 rune 안 쓸 때도 ~500MB 점유 (개발자 머신 16GB+ 기준 3% — 체감 적음)

#### (b) Configurable — 향후 선택지로 기록

```go
type Config struct {
    IdleTimeout   time.Duration  // 0 = never unload (default)
    LoadOnStartup bool           // default true
}
```

- 기본값은 (a)와 동일 동작
- 사용자가 config에서 idle timeout 설정 시 (c) 동작

**구현 비용**: ~60 LoC (race-free lazy load + eviction). 고정 패턴이라 한번 짜면 유지 부담 낮음.

**도입 트리거**:
- 저사양 환경(8GB RAM) 사용자 비중 유의미해짐
- "idle 메모리 점유" 피드백 누적
- 멀티 사용자 · 공유 서버 배포 시나리오 발생

#### (c) Lazy load + 기본 eviction 30분 — 향후 선택지로 기록

```
embedder 기동 → HTTP 서버만 listen
 │
 │ 첫 요청 도착 → 모델 로드 (2-10s) → inference
 │ 이후 요청들 → 정상 처리
 │ 30분 요청 없음 → 모델 해제 (메모리 회수)
 │ 다시 요청 → 재로드
```

**장점**:
- 메모리 절감 aggressive (idle 시 ~20MB)

**단점**:
- 첫/재로드 요청 latency 2-10s (사용자 체감 큼)
- 30분 간격 사용자에게 "매번 처음 요청이 느림" 반복
- 동시성 복잡 (로드 중 요청 queuing · eviction vs inflight 경쟁 · refcount)

**도입 트리거**:
- embedder를 강력한 공유 리소스로 사용 (다중 사용자 · 다수 에이전트 · 장기간 idle 보장 필요)
- 메모리 절감이 latency 희생을 정당화하는 환경

### 결정 근거

개발자 머신 주 대상 환경에서:
- 500MB 절감의 실질 가치 < cold start 5-10s UX 비용
- OS swap이 유사 역할 수행 (앱 로직 재구현 불필요)
- 복잡도·race 위험 회피

MVP는 단순·예측 가능 우선. (b)/(c)는 사용 패턴 관측 후 도입.

### 재평가 트리거

D9 재논의가 필요한 신호:
- 프로덕션 환경에서 embedder RSS 점유가 실제 문제로 보고됨
- 저사양 사용자(8GB RAM 이하) 비중 증가
- 멀티 사용자 공유 환경으로 확장 (예: 팀 서버)
- rune 사용 패턴이 극단적 bursty로 바뀜 (하루 2-3회 짧게 쓰고 긴 idle)

재논의 시 (b) configurable부터 시도, 실측 후 (c) 기본 eviction 여부 판단.

---

## D10. near_duplicate 시 `similar_to` 값 형식

**상태**: ✅ Decided (2026-04-21)  
**결론**: **`record_id` 문자열** (best-effort lookup). 조회 실패 시 `shard=X,row=Y` 좌표로 degrade  
**이관 위치**: `internal/service/capture.go` · `components/rune-mcp.md` capture 경로

### 배경

Capture 시 기존 레코드와 유사도 ≥0.95면 `near_duplicate`로 거부하고 응답에 `similar_to` 필드 포함. 이 값에 뭘 넣을지.

### 선택지

| | 내용 | 비용 |
|---|---|---|
| **(a) `record_id`** (best-effort) | 가장 유사한 기존 레코드의 고유 ID (예: `dec_2026-04-10_eng_postgres-choice`). 조회 실패 시 좌표로 degrade | envector.GetMetadata 1회 + AES 복호화 1회 추가 |
| (b) `shard=X,row=Y` 좌표만 | 단순 | 사용자·에이전트에게 의미 없음 |
| (c) 둘 다 | | 응답 필드 비대 |

### 결정

**(a) record_id + degrade**.

근거:
- 에이전트가 사용자에게 "이미 `dec_2026-04-10_...` 레코드가 있어요" 같은 의미 있는 힌트 제공
- GetMetadata + AES 복호화는 이미 recall 경로에서 쓰는 조합. 추가 구현 없음
- lookup 실패는 transient — capture 판정 자체는 이미 끝났으므로 degrade가 합리적

### degrade 방침

- `similar_to` 조회 실패 시 → `shard=X,row=Y` 좌표로 대체
- near_duplicate 판정은 유지 (사용자에게 "중복이다"는 여전히 전달됨)
- capture 자체를 fail 처리 안 함 (조회 실패가 판정 정확성에 영향 없음)

### 구현

```go
// internal/service/capture.go
func (s *CaptureService) lookupSimilarRecord(ctx context.Context, top *domain.ScoreEntry) string {
    if top == nil { return "" }
    ref := envector.MetadataRef{
        ShardIdx: uint64(top.ShardIdx),
        RowIdx:   uint64(top.RowIdx),
    }
    metas, err := s.envector.GetMetadata(ctx, []envector.MetadataRef{ref}, []string{"metadata"})
    if err != nil || len(metas) == 0 {
        return fmt.Sprintf("shard=%d,row=%d", top.ShardIdx, top.RowIdx)  // degrade
    }
    plaintext, err := aesctr.Open(s.agentDEK, s.agentID, metas[0].Data)
    if err != nil {
        return fmt.Sprintf("shard=%d,row=%d", top.ShardIdx, top.RowIdx)  // degrade
    }
    var meta map[string]any
    if err := json.Unmarshal(plaintext, &meta); err != nil {
        return ""
    }
    recordID, _ := domain.GetString(meta, "record_id")
    return recordID
}
```

---

## D11. Novelty 임계값

**상태**: ✅ Decided (2026-04-21)  
**결론**: **`{novel: 0.3, related: 0.7, near_dup: 0.95}`** — Python `server.py:100-108` 런타임 기본값과 동일  
**이관 위치**: `internal/policy/novelty.go` 상수 · `components/rune-mcp.md` novelty 섹션

### 배경

Python 코드베이스에 novelty 임계값이 **두 곳에 서로 다른 값**으로 존재:
- `mcp/server/server.py:100-108` (`_classify_novelty` 함수 default 인자): `{0.3, 0.7, 0.95}` — **런타임 실제 사용 값**
- `agents/common/schemas/embedding.py:16-18` 주석: `{0.4, 0.7, 0.93}` — benchmark 2026-04-08에서 튜닝된 값이지만 런타임 미반영

### 결정

**MVP는 런타임 동작과 동일한 `{0.3, 0.7, 0.95}`**.

근거:
- "Python → Go 포팅은 behavior equivalence"가 기본 원칙
- 기존 저장된 레코드의 novelty 판정이 Python과 Go에서 bit-identical 유지
- benchmark 튜닝값 `{0.4, 0.7, 0.93}`은 **post-MVP에 A/B 검증 후 채택 여부 별도 결정**

### 구현

```go
// internal/policy/novelty.go
var DefaultNoveltyThresholds = NoveltyThresholds{
    Novel:   0.3,
    Related: 0.7,
    NearDup: 0.95,
}
```

스키마 버저닝 관점에서 `Scheme` 타입으로 묶어두면 post-MVP 전환 시 config.embedding.scheme 필드로 선택 가능. 지금은 상수 하나면 충분.

### 재평가 트리거

Post-MVP에 benchmark 기반 `{0.4, 0.7, 0.93}` 전환 검토. 조건:
- 현재 0.95 임계값 아래 통과되는 "실질 중복" 피드백 누적
- novelty 품질 정량 평가 셋 구축 후 A/B

---

## D12. 첫 capture · top-k 빈 경우 처리

**상태**: ✅ Decided (2026-04-21)  
**결론**: **`similarity = 0` · `novel` 판정**. 정상 Insert 진행  
**이관 위치**: `internal/service/capture.go`

### 배경

Capture 시 envector.Score가 빈 결과 반환하는 경우:
- index가 비어있음 (아직 저장된 레코드 0개)
- 모든 기존 레코드가 삭제됨
- 기타 이유로 비교 대상 없음

이때 novelty 판정을 어떻게 할지.

### 선택지

- **(a) `similarity = 0` · `novel` 판정** — 비교 대상 없음 = 새로움
- (b) `novelty = "unknown"` 별도 class
- (c) Insert 자체를 거부

### 결정

**(a) similarity=0 · novel**.

근거:
- 의미론적으로 합리적: 비교 대상 없음 = 기존과 완전 다름 = 새로움
- 첫 capture 시나리오가 특수 상태로 처리되면 후속 코드에 분기 추가 (edge case 늘어남)
- `novel` 판정은 정상 Insert 경로로 이어짐 — 사용자 경험 자연스러움
- (b) "unknown" class는 정보가치 없고 에이전트 처리만 복잡해짐
- (c) 거부는 "첫 capture가 실패함"이라는 말도 안 되는 UX

### 구현

```go
// internal/service/capture.go
var similarity float64  // zero value = 0.0
if len(blobs) > 0 {
    entries, err := s.vault.DecryptScores(ctx, blobs[0])
    if err != nil { return nil, mapVaultErr(err) }
    if len(entries) > 0 {
        similarity = entries[0].Score
    }
}
// similarity == 0이면 ClassifyNovelty가 novel 반환
novelty := policy.ClassifyNovelty(similarity, s.scheme.Novelty)
```

---

## D13. DecisionRecord 조립 책임

**상태**: ✅ Decided (2026-04-21) — Option A 채택. B·C는 미래 선택지로 기록  
**결론**: **Option A — rune-mcp가 `extracted`(ExtractionResult)를 받아 `DecisionRecord` 조립**. Python `agents/scribe/record_builder.py` (703 LoC) 기능을 Go로 포팅  
**이관 위치**: `internal/service/capture.go` · `internal/policy/record_builder.go` (신규) · `internal/domain/decision_record.go`

### 배경

envector에 저장되는 metadata는 **DecisionRecord v2.1 전체 JSON** (Python `server.py:1376` `r.model_dump(mode="json")` 실측). 20+ 필드(`id`, `domain`, `status`, `decision`, `context`, `why`, `evidence`, `reusable_insight`, `payload`, `quality`, ...).

Python은 `record_builder.py`에서 `ExtractionResult` → `DecisionRecord`로 변환:
- PII 마스킹 (email · phone · API key prefix · 32+자 hex · card 5종)
- quote strip 4 patterns
- ID 생성 + suffix (`_p{seq}`, `_b{seq}`)
- Domain 판정 (130+ category → 19 enum 매핑)
- Certainty/Status 규칙 (evidence-based)
- Group fields (phase chain · bundle)
- 총 약 700 LoC

Go rune-mcp가 이 책임을 어떻게 분담할지 결정 필요.

### 선택지

#### Option A — rune-mcp가 풍부한 빌드 (✅ 채택)

- Python `record_builder.py` 700+ LoC 전체를 Go로 포팅
- 에이전트는 기존 ExtractionResult 그대로 보냄 → rune-mcp가 가공
- **Python과 behavior equivalent**. 기존 저장된 레코드와 완벽 호환 (결정 D3의 "Python equivalence" 원칙과 일관)
- 에이전트 md 프롬프트 수정 최소 (기존 구조 유지)
- Go 구현 600-900 LoC 추가 부담

#### Option B — 에이전트가 완성된 DecisionRecord 전송 (미래 선택지)

- 에이전트 md가 DecisionRecord v2.1 full shape 직접 생성 (PII 마스킹·certainty 판정을 프롬프트로)
- rune-mcp는 struct로 받아 최소 검증만 → 그대로 JSON 저장
- Go 구현 ~100 LoC
- **에이전트 md 대폭 재작성 필요** (scribe.md 프롬프트에 record_builder 로직 전부)
- LLM에게 PII 정규식 마스킹 같은 결정론적 일을 맡기는 건 실수 여지 큼

#### Option C — 하이브리드 (미래 선택지)

- 에이전트 ExtractionResult 받되 rune-mcp가 **최소 필드만 정규화** (id · timestamp · schema_version · PII 마스킹)
- 나머지는 에이전트가 채운 대로 수용
- Go 구현 ~200-300 LoC
- 에이전트가 DecisionRecord v2.1 알아야 하는 건 같지만 일부 필드 누락 허용
- A/B 사이 타협

### 결정

**Option A 채택**.

근거:
- "Python behavior equivalence" 원칙 (기존 결정들과 일관: D3 title 60, D11 novelty 임계, D10 record_id 형식)
- 기존 Python으로 저장된 레코드 호환 (역호환 요건)
- PII 마스킹 같은 결정론적 작업을 LLM 프롬프트로 맡기는 게 **실수 여지 큼** → Go 코드로 확실히 수행
- 포팅 700 LoC는 기계적 작업 (복잡하지만 well-defined)
- 에이전트 md는 기존 형태 유지 → 마이그레이션 risk 낮음

### 구현 계획

Python 참조:
- `agents/scribe/record_builder.py` (703 LoC)
- `agents/common/schemas/decision_record.py` (259 LoC) — enum · Pydantic 스키마

Go 대응 파일 제안:
```
internal/domain/decision_record.go     # Pydantic → Go struct + JSON tags (~300 LoC)
internal/policy/record_builder.go      # build_phases · PII · quote strip · domain 매핑 (~500 LoC)
internal/policy/record_id.go           # generate_record_id + suffix 규칙 (~80 LoC)
internal/policy/pii.go                 # 5 regex 마스킹 (~50 LoC)
```

**검증 전략**:
- Python에서 golden fixture 생성 (`scripts/gen_golden.py`) — 같은 `extracted` 입력에 대한 DecisionRecord JSON 출력 덤프
- Go 테스트가 같은 입력으로 빌드 후 JSON 비교 → bit-identical 검증
- 100개 이상 실제 capture 샘플로 커버

### 재평가 트리거

Option B·C로 전환 검토 조건:
- Go record_builder 유지 부담이 팀에게 실제로 크게 느껴질 때
- 에이전트 md 프롬프트가 풍부해져서 자연스럽게 B 가능해질 때 (LLM 성능 향상)
- ExtractionResult 스키마가 반복 변경되어 "에이전트가 완성본 만드는 게 더 유연" 판단 시

Post-MVP에 B 실험 후 (a) 품질 검증 (b) 에이전트 프롬프트 안정성 확인 시 전환 가능.

### 구현 주의사항

- `generate_record_id` 슬러그 규칙 (`decision_record.py:245-251`): `split()[:3]` + `w.isalnum() or w.replace("_", "").isalnum()` 필터. **한글 등 비 ASCII 문자는 `isalnum()` True** — 유니코드 동작 확인 필요. Go는 `unicode.IsLetter`/`IsDigit` 조합
- `ensure_evidence_certainty_consistency()` 규칙: evidence 없으면 certainty=supported → unknown 강등. Python 동작 그대로 이식
- Group fields (`group_id`, `phase_seq`, `phase_total`) — phase chain **expansion 유지** (D27, 2026-04-21 결정). Python `_expand_phase_chains`와 동일하게 MVP에 포함. `flows/recall.md` Phase 6 참조
- 현재 `record_builder.py`에 있는 **legacy Slack/Notion integration** 경로는 **MVP에서 제거** (agent-delegated 모드에선 미사용)

---

## D14. record_builder LLM fallback 제거

**상태**: ✅ Decided (2026-04-21)  
**결론**: **LLM fallback 경로 제거**. rune-mcp는 `pre_extraction`(에이전트가 만든 `ExtractionResult`) 필수로 요구. 없으면 에러  
**이관 위치**: `internal/policy/record_builder.go`

### 배경

Python `record_builder.py`의 `build_phases`:
- `pre_extraction`이 있으면 그대로 사용 (agent-delegated)
- `pre_extraction`이 없고 `LLMExtractor` 사용 가능하면 LLM이 직접 텍스트 추출
- LLM도 없으면 regex fallback (`build()` 경로)

rune MVP 방향:
- `scribe.md` 에이전트가 항상 `ExtractionResult`를 구축해서 rune-mcp로 전달
- rune-mcp는 LLM 직접 호출 안 함 (다른 에이전트의 역할과 분리)

### 결정

**LLM fallback 제거** · **regex fallback도 제거** · `pre_extraction` 필수.

```go
if preExtraction == nil {
    return nil, domain.ErrExtractionMissing  // non-retryable
}
```

근거:
- 에이전트가 이미 LLM 갖고 있음 (Claude/Codex). rune-mcp가 추가 LLM 부담질 이유 없음
- MVP 단순화 — LLMExtractor(421 LoC)·detector(225 LoC) 포팅 회피
- 에이전트 md(scribe.md)에서 ExtractionResult 생성 프롬프트는 이미 기존 Python 코드에서 성숙

### 재평가 트리거

Non-agent 환경(예: CLI 직접 호출·Slack 핸들러)에서 rune-mcp 쓸 필요 생기면 재검토. MVP는 agent-only.

---

## D15. `render_payload_text` 전체 포팅

**상태**: ✅ Decided (2026-04-21)  
**결론**: **Python `agents/common/schemas/templates.py` (363 LoC) 전체 Go 포팅**. 최소 템플릿으로 단순화 안 함  
**이관 위치**: `internal/policy/payload_text.go` · 테스트는 golden fixture

### 배경

DecisionRecord의 `payload.text`는 "memory reproduction용 markdown 텍스트" (schema 2.1 주석). `reusable_insight`가 embedding 주 대상이긴 하지만 `payload.text`는:
- fallback embedding 대상 (reusable_insight 비었을 때)
- 사용자·에이전트에게 record 전체를 한눈에 보여주는 rendering
- Python recall 응답의 `summary` 필드

Python `templates.py` 363 LoC:
- EN/KO/JA 다국어 템플릿 (MVP는 EN 우선)
- 각 필드(context, why, evidence, links 등)를 markdown으로 렌더
- 조건부 섹션 (evidence 없으면 생략 등)

### 결정

**전체 포팅** — Python 파일을 **canonical source로 선언**, Go는 라인 단위 미러링.

근거:
- Python 동작 equivalence 원칙 (D3·D11·D13과 일관)
- `payload.text`는 envector에 저장돼 recall 때 활용. 단순화하면 기존 레코드와 차이 발생
- 순수 함수라 포팅 기계적
- 다국어 template은 일단 유지 (MVP scope 안에서 EN만 쓰더라도 KO/JA 포팅 비용 크지 않음)

### 포팅 contract (Go 개발자가 구현할 항목)

**Canonical source**: `agents/common/schemas/templates.py` (363 LoC, 읽기 필수)

포팅 대상:

| Python symbol (templates.py) | 라인 | Go 대응 | MVP 여부 |
|---|---|---|---|
| `PAYLOAD_TEMPLATE` (멀티라인 format string) | L14~ | const `payloadTemplate` | ✅ 필수 |
| `_format_alternatives(alternatives, chosen)` — "chosen" marker 삽입 | L52 | `formatAlternatives` | ✅ 필수 |
| `_format_trade_offs(trade_offs)` | L66 | `formatTradeOffs` | ✅ 필수 |
| `_format_assumptions(assumptions)` | L73 | `formatAssumptions` | ✅ 필수 |
| `_format_risks(risks)` | L85 | `formatRisks` | ✅ 필수 |
| `_format_evidence(evidence)` — quote 번호 매김 | L97 | `formatEvidence` | ✅ 필수 |
| `_format_links(links)` | L118 | `formatLinks` | ✅ 필수 |
| `_format_tags(tags)` | L131 | `formatTags` | ✅ 필수 |
| `render_payload_text(record)` — 메인 (phase_line/group_summary 삽입 포함) | L138-222 | `RenderPayloadText` | ✅ 필수 |
| `render_compact_payload(record)` | L225 | `RenderCompactPayload` | 🟡 Post-MVP (retriever test에서만 사용) |
| `render_display_text(record, language)` — 다국어 EN/KO/JA | L288 | `RenderDisplayText` | 🟡 Post-MVP (retriever test에서만 사용) |

### 검증 방식 (필수)

**Golden fixture test** — Python↔Go bit-identical 보장:

1. Python에서 대표 DecisionRecord 50개 샘플로 `render_payload_text` 실행 → `testdata/payload_text/golden/{id}.md` 저장
2. Go 구현에서 동일 input으로 실행 → **byte-for-byte 비교**
3. 차이 발견 시 test fail

이 fixture는 **포팅 완료 판정 기준**이다.

### 재평가 트리거

- `payload.text`가 실사용에서 거의 읽히지 않음 관측 시 단순화 검토
- 다국어 렌더가 bug-prone으로 판명 시 EN-only로 축소
- 에이전트가 `payload.text`를 직접 생성해서 넣어주는 모델로 전환 시 (Option B의 부분 채택)

---

## D16. Multi-record capture 시 batch embedding

**상태**: ✅ Decided (2026-04-21)  
**결론**: **phase chain 등 N개 레코드 생성 시 rune-embedder `/v1/embed`에 batch 1회 호출**. 개별 N회 아님  
**이관 위치**: `internal/service/capture.go` · `components/rune-embedder.md`

### 배경

Python record_builder가 `build_phases()` 호출 결과 1-7개 record 반환 가능 (phase chain / bundle). 각 record마다 `reusable_insight` 또는 `payload.text`를 embedding 대상으로 씀. Go 포팅 시 embedder HTTP 호출을 개별로 N번 vs batch 1번으로 할지 결정.

### 선택지

| | 동작 | Latency | 비고 |
|---|---|---|---|
| **(a) batch 1회** | `{"texts": [t1, t2, t3]}` | ~120-150ms (예상) | 네트워크 왕복 1번, 모델 batch dim |
| (b) 개별 N회 | N번 호출 | ~N × 100ms | 네트워크·추론 setup 반복 |

### 결정

**(a) batch 1회**.

근거:
- rune-embedder API가 이미 배열 입력 `{"texts":[...]}` 지원 — 추가 구현 0
- Latency 2-3배 개선 (N개 record일수록 차이 큼)
- 모델 추론이 batch dim으로 병렬 처리 효율적
- 단일 record 케이스 (len=1)는 batch로 호출해도 overhead 없음

### Trade-off · 구현 주의

- **에러 격리**: batch 중 하나 실패 시 전체 실패 (보통). 대응: 사전 검증(텍스트 길이 상한) + 실패 시 응답은 retryable 에러로 전체 capture 재시도
- **메모리 peak**: 모든 텍스트 동시 처리. phase chain max 7 × reusable_insight(256-768 tokens) → embedder 내부 메모리 일시 증가 경미
- **단일 호출 API와의 통일성**: `EmbedOne`은 `Embed([]string{text})[0]` 형태로 편의 wrapper. API는 항상 batch 기반

### 구현

```go
// internal/service/capture.go - Phase 5 통합
embedTexts := make([]string, len(records))
for i, r := range records {
    embedTexts[i] = selectEmbedTextForRecord(r)
}
vectors, err := s.embedder.Embed(ctx, embedTexts)  // batch 1회
if err != nil { return nil, err }
// vectors[i] <-> records[i] 1:1 대응
```

### Recall 경로에도 적용

Multi-query recall(expanded queries max 3)에도 같은 원칙:
```go
// Phase: recall의 query expansion
queries := []string{parsed.Original, parsed.Expanded[0], parsed.Expanded[1]}
vectors, err := s.embedder.Embed(ctx, queries)  // batch
// 이후 errgroup으로 vectors[i]마다 envector.Score 병렬
```

---

## D17. envector `Insert` batch atomicity

**상태**: ✅ Decided (2026-04-21) — **조건부**: MVP는 "atomic 가정"으로 진행. Phase 2 integration test에서 실 envector 서버 동작 실측 후 확정  
**결론**: MVP 코드는 atomic batch 전제로 작성 · 부분 실패는 발생 안 한다고 가정 · 실측 결과 다르면 D17 재검토  
**이관 위치**: `components/envector-integration.md` Insert 섹션 · `internal/adapters/envector/client.go`

### 배경

rune-mcp의 `proceedToInsert`가 N개 record (phase chain)를 한 번의 `InsertRequest`로 envector 서버에 보냄. 서버가 이 batch를:
- **All-or-nothing**: 전부 성공 또는 전부 실패
- **Best-effort**: 가능한 만큼 저장하고 실패한 것 보고
- **Streaming partial**: 일부 청크 성공 후 끊김

중 어느 쪽으로 처리하는지 **envector 서버 동작이 확정적으로 문서화되어 있지 않음** (SDK 코드 레벨 실측 기준).

envector-go SDK 내부(`insert.go`):
- `BatchInsertData` streaming RPC 사용
- 1 MiB 청크 단위로 frames 나눠 전송
- 응답은 마지막에 한 번 (`CloseAndRecv`)
- 서버가 중간 실패하면 에러 리턴

### 선택지

| | 코드 가정 | 부분 실패 시 행동 |
|---|---|---|
| **(a) atomic 전제 (MVP 채택)** | 성공이면 전부 저장됨 전제 | 에러 시 재시도 or 에러 반환. 부분 저장 없음 가정 |
| (b) partial 허용 | 일부 성공 가능 전제 | 성공 ID 리스트와 실패 리스트 분리 처리. 복구 로직 필요 |
| (c) 단건씩 순차 Insert | 한 record씩 호출 | 격리 완벽. 성능·네트워크 비용 증가 |

### 결정

**(a) atomic 전제로 MVP 진행** · **실측 후 Phase 2 integration test 시점에 재확정**.

근거:
- envector-go SDK가 여러 vectors를 단일 `InsertRequest`로 받는 설계 → 서버 입장에서 batch가 기본 단위
- gRPC streaming에서 중간 실패 시 전체 에러 반환하는 게 일반적 패턴
- MVP scope에서 partial-failure 복구 로직까지 구현하면 과도한 복잡도
- 실측 결과가 atomic 아니어도 대응 가능: D17 재검토로 (b) 또는 (c) 전환

### 구현 가정

```go
// internal/service/capture.go
result, err := s.envector.Insert(ctx, envector.InsertRequest{
    Vectors:  vectors,
    Metadata: envelopes,
})
if err != nil {
    // atomic 가정: 전체 실패로 간주. 재시도 권장
    return nil, domain.ErrEnvectorInsertFailed.With("cause", err.Error())
}
// 성공: result.ItemIDs는 N개 모두 반환됨 전제
if len(result.ItemIDs) != len(vectors) {
    // 이 경우는 atomic 가정 위배 — 로그로 기록하고 에러
    slog.ErrorContext(ctx, "atomicity violation",
        "expected", len(vectors), "got", len(result.ItemIDs))
    return nil, domain.ErrEnvectorInconsistent
}
```

**`len(ItemIDs) != len(Vectors)` 방어 체크**: "atomic이라더니 일부 missing" 같은 이상 케이스 catch용. 발생 시 즉시 에러 + 로그 수집해 D17 재검토 트리거.

### Phase 2 integration test 항목

다음을 확인해서 D17 최종 확정:
1. **정상 케이스**: N=5 batch → `len(ItemIDs) == 5`, 모두 유효 int64
2. **vector dim mismatch** (일부만 1024 아님): 전체 실패? 부분 저장?
3. **metadata 너무 큼** (하나가 크기 초과): 전체 실패? 부분 저장?
4. **서버 중단 중** (insert 진행 중 서버 다운): 어떤 에러? 재시도 안전한가?
5. **duplicate ID** (envector가 vector ID 자동 생성하므로 중복 불가능해야 함): 확인

결과에 따라:
- 모든 케이스 atomic → D17 "atomic 확정" 마킹
- 일부 partial → D17 재논의, (b) 또는 (c) 전환

### 재평가 트리거

- Phase 2 integration test 실측 결과
- 프로덕션 운영 중 `len(ItemIDs) != len(Vectors)` 로그 관찰
- envector 서버 동작 문서 갱신 시

---

## D18. capture 응답의 `record_id` 필드

**상태**: ✅ Decided (2026-04-21) — Python 동일 (첫 레코드 id). (b)·(c)는 미래 선택지로 기록  
**결론**: **`records[0].id`만 반환** (Python `server.py:1391`과 동일)  
**이관 위치**: `internal/domain/capture.go` · `components/rune-mcp.md`

### 배경

multi-record capture (phase chain) 성공 시 rune-mcp가 에이전트에게 반환하는 `CaptureResponse`의 `record_id` 필드 형태 결정. Python 기본값 · 확장 옵션 중 선택.

### 선택지

| | 응답 | 장단 |
|---|---|---|
| **(a) `record_id = records[0].id`** (✅ 채택) | 첫 레코드만 | Python 동일. 단순. 에이전트가 group_id로 전체 묶음 추적 가능 |
| (b) `record_ids = [모든 id]` | 전체 리스트 | 투명. 응답 스키마 변경 (에이전트 프롬프트도 수정) |
| (c) `record_id + group_id + phase_total` | 대표 id + 그룹 정보 | 풍부. 스키마 변경. 에이전트가 그룹 있는지 쉽게 판단 |

### 결정

**(a) Python 동일** 채택.

근거:
- Python 현재 동작 (`server.py:1391`)과 **1:1 equivalence** → 에이전트 md 수정 불필요
- 단순 API contract — 에이전트·사용자가 하나의 record_id로 이 capture 전체를 참조 가능
- multi-record 정보는 record metadata 자체에 `group_id`/`phase_seq`/`phase_total` 필드 있음 → 필요하면 recall 시 확인 가능
- multi-record 여부가 응답에 명시 안 되지만 대부분 정상 UX (1 capture → 1 응답)

### 확장 옵션 (재평가 시)

**(b) 전체 리스트**:
- multi-record 상세 정보가 응답에 즉시 노출되어야 하는 유스케이스 생기면 전환
- 예: 에이전트가 phase chain 직후 특정 phase만 조회해야 하는 경우
- 구현 쉬움 — 응답 필드 하나 추가

**(c) 그룹 정보 포함**:
- 에이전트가 capture 직후 group_id로 관련 recall 트리거하는 경우
- 응답 스키마 복잡도 증가 — 단순 케이스(단일 record)에도 null 필드 노출

### 구현

```go
// internal/domain/capture.go
type CaptureResponse struct {
    OK        bool   `json:"ok"`
    Captured  bool   `json:"captured"`  // near_duplicate면 false
    RecordID  string `json:"record_id,omitempty"`  // records[0].id
    Summary   string `json:"summary,omitempty"`    // records[0].title (Python server.py:1392)
    Domain    string `json:"domain,omitempty"`     // records[0].domain
    Certainty string `json:"certainty,omitempty"`  // records[0].why.certainty
    Novelty   *NoveltyResult `json:"novelty,omitempty"`
    Reason    string `json:"reason,omitempty"`     // "near_duplicate" 등
    SimilarTo string `json:"similar_to,omitempty"`
}
```

Python `server.py:1388-1398`:
```python
result = {
    "ok": True,
    "captured": True,
    "record_id": first.id,
    "summary": first.title,
    "domain": first.domain.value,
    "certainty": first.why.certainty.value,
}
```

→ **Go struct 필드명도 동일하게 유지** (에이전트 md가 JSON 응답 기존 형태로 파싱).

### 재평가 트리거

- 에이전트 md가 multi-record 상세 정보를 필요로 하는 유스케이스 등장
- recall UX가 "phase chain 개별 추적" 요구
- capture + 즉시 recall 같은 고급 패턴 구현 시

---

## D19. capture_log append 실패 시 동작

**상태**: ✅ Decided (2026-04-21)  
**결론**: **Degrade** — 로그 실패 시 slog error 남기고 capture는 정상 성공 응답  
**이관 위치**: `internal/adapters/logio/capture_log.go` · `internal/service/capture.go`

### 배경

envector.Insert 성공 후 `~/.rune/capture_log.jsonl`에 한 줄 append. 이 append가 실패할 수 있는 경우 (디스크 풀·권한·파일 핸들 부족·FS 에러·디렉토리 부재). 실패 시 capture 자체를 어떻게 처리할지.

### 결정

**Degrade**: slog error + capture 성공 응답.

근거:
- envector 저장이 source of truth. capture_log는 **감사·디버깅 편의용 이차 사본**
- 로그 실패로 사용자 capture가 실패하면 UX 나쁨
- rollback(envector에서 record 삭제)은 일관성 보장 어려움 (rollback도 실패하면?)
- Python 동작 동일 (`_append_capture_log`가 `try/except` + `logger.debug("failed")`)

### 구현

```go
if err := s.captureLog.Append(entry); err != nil {
    slog.ErrorContext(ctx, "capture_log append failed",
        "err", err, "record_id", records[0].ID)
    // 에러 리턴 안 함. capture 응답은 정상
}
```

---

## D20. capture_log jsonl 포맷

**상태**: ✅ Decided (2026-04-21)  
**결론**: **Python bit-identical 포맷 채택**. 같은 파일에 append하는 호환성 확보  
**이관 위치**: `internal/adapters/logio/capture_log.go`

### 배경

Python `mcp/server/server.py:_append_capture_log` 실측 포맷:
```json
{
  "ts": "2026-04-21T10:30:00+00:00",
  "action": "captured" | "deleted",
  "id": "dec_2026-04-21_architecture_postgres_choice",
  "title": "PostgreSQL 선택",
  "domain": "architecture",
  "mode": "standard" | "soft-delete",
  "novelty_class": "novel",
  "novelty_score": 0.23
}
```

특이사항:
- `ensure_ascii=False` — 한글 등 원본 유지
- multi-record capture여도 **첫 레코드만** 로그 (`first.id`, `first.title`, `first.domain`)
- `novelty_class`/`novelty_score`는 값 있을 때만 포함 (omitempty)

### 결정

**Python 포맷 그대로 채택** (bit-identical).

근거:
- 마이그레이션 기간 동안 Python과 Go가 **같은 파일에 append** 가능 (혼재 안전)
- 감사 로그 분석 툴(기존 있다면) 재사용 가능
- Go `json.Marshal`은 기본으로 non-ASCII를 escape 안 함 → ensure_ascii=False 자동 만족

### 구현

```go
// internal/domain/logio.go
type LogEntry struct {
    TS           string  `json:"ts"`                              // ISO-8601 UTC
    Action       string  `json:"action"`                           // "captured" | "deleted"
    ID           string  `json:"id"`                               // first record_id
    Title        string  `json:"title"`                            // first title
    Domain       string  `json:"domain"`                           // first domain
    Mode         string  `json:"mode"`                             // "standard" 등
    NoveltyClass string  `json:"novelty_class,omitempty"`
    NoveltyScore float64 `json:"novelty_score,omitempty"`
}
```

**주의**: Go `omitempty`는 float64 0.0일 때 필드 생략. Python은 `if novelty_class:`로 체크. 동작 동일한지 확인:
- `novelty_score = 0.0` + `novelty_class = "novel"`: Python은 둘 다 포함 / Go는 score만 생략 → **mismatch 가능**
- 대응: `NoveltyScore *float64` 포인터로 바꿔서 명시적 `nil` 체크

실측 시점에 확정. 기본값은 포인터 버전으로 Python 동작 재현.

### Multi-process 안전

Python은 단일 프로세스라 file lock 없음. Go는 세션별 rune-mcp 여러 개 가능 → **`syscall.Flock(LOCK_EX)` 필요**:

```go
func (l *CaptureLog) Append(entry LogEntry) error {
    line, _ := json.Marshal(entry)
    l.mu.Lock()  // intra-process
    defer l.mu.Unlock()

    f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if err != nil { return err }
    defer f.Close()

    // inter-process 안전
    syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    _, err = f.Write(append(line, '\n'))
    return err
}
```

flock 실패 시 에러 반환 → D19 degrade로 흡수.

---

## D21. multilingual query 처리

**상태**: ✅ Decided (2026-04-21) — Option C (agent-side 사전 번역)

### 배경

Python `agents/retriever/query_processor.py`는 두 경로를 가짐:
- **English path** (`_parse_english`): 31 intent regex + 16 time regex + 81 stopwords + 21 tech names로 구성된 순수 regex 파싱
- **Multilingual path** (`_parse_multilingual`): `LLMClient.generate(QUERY_PARSE_PROMPT)` 호출하여 번역 + intent + entity 추출 (non-English 감지 시)

`server.py:1749-1755`에서 `QueryProcessor`는 항상 초기화되며, 내부 `self._llm.is_available` 체크로 분기. LLM key가 없으면 자동으로 English path로 fallback.

**명시적 정책 불일치**:
- Recall synthesis는 `server.py:1777`에서 "agent-delegated mode — raw results returned"로 명시적 LLM-free 선언
- Query parsing의 LLM 사용은 명시 방침 없이 "key 있으면 쓴다" 상태 — agent-delegated 철학과 일관성 부재

Go rune-mcp는 `synthesizer` 부재 + D14(record_builder LLM fallback 제거)로 **LLM-free** 방향으로 명확하게 수렴. Query parsing도 이 방향에 편승할지가 쟁점.

### 선택지

**A. Python 동일 — LLM key 있으면 multilingual 활성화**
- rune-mcp에 LLM client (Anthropic/OpenAI/Gemini) 추가
- API key 관리 (`~/.rune/config.json`), timeout, 에러 처리 등 복잡도 증가
- 장점: 한국어 쿼리 품질 Python과 동등

**B. LLM path 완전 제거 — English regex only**
- 구현 최단
- 단점: 한국어 쿼리 시 intent=GENERAL로 떨어지고, entity 추출도 tech-regex로만 (한국어 entity 유실)
- recall 품질 저하

**C. Agent-side 사전 번역** ⭐
- Claude Code (혹은 다른 agent)가 recall 호출 전 쿼리를 영어로 번역
- rune-mcp는 English path만 구현
- `agents/claude/retriever.md`에 "recall 호출 전 영어로 번역" 규칙 추가
- 장점: rune-mcp LLM-free 유지 (synthesizer + record_builder와 정책 일관), 품질 손실 최소

### 결론

**Option C 채택**. 이유:
1. **정책 일관성**: rune-mcp는 embedding · encryption · storage만 담당. LLM 추론은 전부 agent로 위임 (D14 + synthesizer 부재와 동일 철학).
2. **자연스러움**: Claude Code는 이미 user와 대화 중이므로 쿼리 번역이 추가 cost 아님. 번역 결과를 사용자에게 노출할 필요도 없음 (internal).
3. **유지보수**: LLM client 의존성 · API key 관리 · provider별 timeout 튜닝을 rune-mcp에서 제거.
4. **recall 품질**: 한국어 쿼리도 agent 번역을 거친 뒤 English path로 들어가므로 Python LLM path와 비교해 품질 손실 크지 않음 (Python도 내부적으로 번역 + English expansion을 수행).

### 구현

**rune-mcp 쪽**:
- `internal/policy/query_processor.go` — Python `_parse_english` 6단계 1:1 포팅
- `_parse_multilingual` · `detect_language()` · `LLMClient` 전부 **제거**
- `QueryParseResult` (Go struct) = `ParsedQuery` (Python dataclass) 동일 필드, 단 `language` 필드는 제거 (감지 안 함)
- Expansion 상한: Python `[:5]` 유지 (D16 batch embedding과 정합)

**Agent 쪽** (`agents/claude/retriever.md` · `agents/gemini/retriever.md` 등):
- 새 섹션 "Query translation" 추가:
  > 사용자 쿼리가 비-영어일 경우, recall 호출 전 의미를 보존한 영어 쿼리로 번역하여 `query` 파라미터에 전달한다. 사용자에게는 원본 언어로 응답한다.
- 실제 translation 구현 — agent의 native LLM 호출로 해결 (rune-mcp 변경 없음)

### 재평가 트리거

이후 recall 품질 측정에서 다음 중 하나라도 관측되면 Option A 재검토:
- 한국어·일본어·중국어 쿼리의 entity 추출율이 영어 대비 30% 이상 저하
- Agent가 번역 단계를 빈번히 건너뛰거나 왜곡된 번역 (내부 eval로 측정)
- 기업 고객이 "번역 없는 native 쿼리" 요구 (multi-tenant 시나리오)

재평가 시 Option A로 전환하려면 `internal/policy/query_processor.go`에 multilingual dispatch 추가하고 `internal/adapters/llm/` 패키지 신설.

---

## D22. Recall expansion search cap

**상태**: ✅ Decided (2026-04-21) — Option A (`[:3]` 유지)

### 배경

Phase 2 `generateExpansions`는 intent + entity 기반으로 최대 **5개** expansion을 생성한다 (`query_processor.py:417` `return unique[:5]`). 하지만 Phase 3/4에서 실제로 embed + search에 사용되는 expansion 수는 별개 제한:

Python `searcher.py:160`:
```python
for expanded_query in query.expanded_queries[:3]:
    results = await self._search_single(expanded_query, topk)
```

즉 **생성은 5, 사용은 3** — 2개는 버려진다.

Go rune-mcp 포팅 시 이 cap을 그대로 유지할지 확장할지 쟁점.

### 선택지

**A. `[:3]` 유지** ⭐
- Python bit-identical
- Recall cost (Vault · envector 왕복 N회) 예측 가능
- 성능/품질 특성 Python과 동일

**B. `[:5]` 전부 사용**
- Expansion 품질 소폭 향상 가능 (특히 multi-entity 쿼리)
- Vault `DecryptScores` · envector `Score` 호출 5회로 증가 → latency + cost ↑
- 실증 데이터 없음 (Python 수치 기반 하한 3은 경험적 선택으로 보임)

### 결론

**Option A 채택** — Python 동작 보존 원칙.

### 구현

- Phase 2: `unique[:5]` 생성 유지 (Python과 동일, 디버깅/로깅 용도로 모든 expansion을 `ParsedQuery.ExpandedQueries`에 보관)
- Phase 3/4: `parsed.ExpandedQueries[:3]`만 batch embed + score

### 재평가 트리거

- Recall 품질 eval에서 `[:3]` → `[:5]` 변경 시 precision@5 유의미 상승 관측 (e.g. +5% 이상)
- 또는 Vault/envector 호출 비용이 낮아져 `[:5]` cost penalty가 무시할 수준

---

## D23. Recall embedding 호출 방식

**상태**: ✅ Decided (2026-04-21) — Batch (B)

### 배경

Python `searcher.py:384`는 per-query 순차 embed:
```python
for expanded_query in query.expanded_queries[:3]:
    query_vector = self._embedding.embed_single(query_text)
    # ... score · decrypt · metadata
```

즉 **3× HTTP round-trip + 3× model forward pass**.

Go rune-mcp는 별도 `rune-embedder` 데몬과 HTTP+JSON으로 통신 (D6). Capture 쪽은 D16으로 "multi-record batch embedding" 결정 완료. Recall도 같은 원칙을 적용할지가 쟁점.

### 선택지

**A. Per-query 순차 (Python 동일)**
- 3× HTTP round-trip
- embedder 측에서는 single embed가 batch보다 처리 단순
- Python bit-identical

**B. Batch 1회** ⭐
- 1× HTTP round-trip + embedder가 batch forward pass 가능
- D16(capture batch)과 정책 일관
- Recall latency 감소 (3 RTT → 1 RTT)
- Embedder가 이미 batch 지원

### 결론

**Option B 채택** — D16과 일관, 명백한 성능 우위, embedder 변경 없음.

### 구현

```go
// Phase 3
texts := parsed.ExpandedQueries[:min(3, len(parsed.ExpandedQueries))]
vectors, err := embedderClient.EmbedBatch(ctx, texts)
// vectors: [][]float32 길이 = len(texts)
```

Embedder의 `/v1/embed` API는 capture/recall 공용 (요청 body에 `texts: []string`).

### 재평가 트리거

- Embedder에서 batch 지원이 제거되거나 per-query forward보다 batch가 오히려 느려지는 경우
- 현재로서는 재평가 사유 없음

---

## D24. 빈 쿼리 처리 위치

**상태**: ✅ Decided (2026-04-21) — Option B (Phase 1 early reject)

### 배경

사용자가 `recall(query="")` 또는 `recall(query="   ")`처럼 빈/공백만 있는 쿼리를 호출할 경우 처리 경로:

**Python 동작**:
- `QueryProcessor.parse("")` → `ParsedQuery(cleaned="", expanded_queries=[""], ...)` (방어 로직 없음)
- `Searcher._search_with_expansions` → `embed_single("")` 호출
- `EmbeddingService.embed_single` L93: `raise ValueError("Cannot embed empty text")`
- `tool_recall` try/except가 에러 응답으로 변환

즉 Python은 **Phase 3 embed 시점에 실패**. 명시적 빈 쿼리 방어 없음.

### 선택지

**A. Python 동일 — Phase 3 embed 시점 실패**
- D5 `EMPTY_EMBED_TEXT` 에러 재사용
- 불필요한 Phase 2 파싱 비용 발생
- 에러 메시지가 "cannot embed empty text"로 user-facing에선 덜 명확

**B. Phase 1 early reject** ⭐
- `strings.TrimSpace(args.Query) == ""` 체크 → `InvalidInputError("query is empty")` 즉시 반환
- Parse/embed 호출 안 함 — 불필요한 비용 제거
- Capture Phase 2의 "알려진 필드 검증" 패턴과 일관
- 에러 메시지 명확

### 결론

**Option B 채택**. Capture가 이미 Phase 2에서 early 검증을 하므로 Recall도 같은 패턴을 사용. 성능/명확성 모두 우위.

### 구현

`internal/mcp/tools.go` recall handler:

```go
if strings.TrimSpace(args.Query) == "" {
    return nil, nil, invalidInput("query is empty")
}
if args.TopK == 0 { args.TopK = 5 }
if args.TopK > 10 {
    return nil, nil, invalidInput("topk must be 10 or less.")
}
```

에러 코드는 Python의 `InvalidInputError`와 동일한 semantics.

### 재평가 트리거

- 실제 운영에서 "공백으로만 된 쿼리"를 의도적으로 지원해야 하는 시나리오 발생 (예: 최근 N개 반환 등 — 현재는 `recall_history` 별도 tool로 처리됨)

---

## D25. Recall Phase 4 병렬화

**상태**: ✅ Decided (2026-04-21) — MVP는 순차 (Python 동일). Post-MVP 재평가.

### 배경

Recall Phase 4는 expansion 3개 각각에 대해 4 RPC 순차 실행 (envector Score → Vault DecryptScores → envector Remind → Vault DecryptMetadata).

Python `searcher.py:153-176`은 `for expanded_query in query.expanded_queries[:3]: await self._search_single(...)` — **순차 await**. `asyncio.gather`로 병렬화 가능했으나 Python은 선택하지 않음. 이유는 기록되어 있지 않으나 구현 단순성 + 작은 규모(3 expansion)로 추정.

Go는 `errgroup.Group`으로 expansion 단위 fan-out이 문법적으로 자연스럽고 오버헤드 미미. 병렬화 시 이득 시뮬레이션:

| 방식 | RPC 실행 | 예상 latency (RPC 50ms 가정) |
|---|---|---|
| Python 순차 (12 RPC) | 4 × 3 순차 | ~600ms |
| expansion 병렬 (errgroup fan-out=3) | 4 × 1 순차 × 3 병렬 | ~200ms |
| batch score + 병렬 decrypt | 1 batch + 3 병렬 | ~150ms |

### 추가 발견

envector SDK `index.scoring(query)` (`envector_sdk.py:265-286`)는 multi-vector batch를 네이티브 지원:
- `query: Union[List[float], List[List[float]]]`
- 결과: `List[CipherBlock]`

하지만 Python wrapper (`EnVectorClient.score`)는 `query=[query_vector]` (길이 1) 로만 호출. **batch API 미활용**.

Vault `decrypt_search_results`는 현재 API가 single blob 기반 (`encrypted_blob_b64=blobs[0]`). Batch 지원 여부 별도 확인 필요.

### 선택지

**A. 순차 (Python bit-identical)** ⭐ MVP
- 복잡도 최소, 포팅 정확성 최우선
- latency penalty 감수

**B. Expansion 병렬 (errgroup fan-out)**
- 3× 성능 향상 확실
- envector-go + vault-go gRPC stub은 기본적으로 concurrent-safe (gRPC Go 컨벤션)
- Partial failure isolation 쉬움

**C. Batch score + 병렬 decrypt**
- 최대 성능
- envector-go SDK batch 지원 여부 미검증 → **추가 조사 필요**
- Vault batch DecryptScores API 확장 필요 가능성

### 결론

**Option A 채택 (MVP)** — 목표는 Python 방식을 그대로 가져가는 것. 초기 포팅에서 동작 정확성 검증이 최우선. 성능 최적화는 실측 이후 별도 작업.

### 구현

```go
var all []SearchHit
seen := map[string]struct{}{}
for i, expQuery := range exps {
    results := runSearchSingle(ctx, deps, expQuery, vectors[i], topk)
    for _, r := range results {
        if _, dup := seen[r.RecordID]; dup { continue }
        seen[r.RecordID] = struct{}{}
        all = append(all, r)
    }
}
sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
return all
```

Python `_search_with_expansions` (line 153-176)과 동일 구조.

### 재평가 트리거

- Recall p50 latency > 500ms (실측 기반)
- Expansion 수가 3 → 5 이상으로 늘어날 경우 (D22 재평가와 연동)
- 고빈도 recall 호출 시나리오 (dashboard·autocomplete 등) 추가될 경우

재평가 시 Option B (expansion 병렬)부터 적용 → 여전히 부족하면 Option C (batch score) 고려. Option C는 envector-go SDK batch API 확인 + Vault `DecryptScores` batch 확장 PR 필요.

---

## D26. Recall 메타데이터 복호화 책임

**상태**: ✅ Decided (2026-04-21) — Python 동작 유지 (Vault 위임 + legacy format + per-entry fallback 모두 유지)

### 배경

Rune 시스템은 metadata 보호에 **agent별 AES-256 DEK (`agent_dek`)**를 사용. 부팅 시 Vault가 agent_dek을 발급 → rune 메모리에 보관.

Python 현황:
- **Capture 경로**: rune이 agent_dek을 보유 → `envector_sdk.py:227-234` `_app_encrypt_metadata`에서 **rune-mcp가 직접 AES encrypt** 후 envelope 삽입
- **Recall 경로**: envector에서 받은 encrypted metadata entries에 대해 `vault_client.py:263-295` `decrypt_metadata` RPC로 **Vault에 복호화 위임**. rune은 AES decrypt 코드 보유하지 않음

즉 encrypt는 rune 직접, decrypt는 Vault 위임 — 비대칭.

또한 Python `searcher.py:417-464`는 metadata entry의 `data` 필드를 3가지 포맷으로 분기 처리:
1. `{"a": agent_id, "c": base64(IV||CT)}` — AES envelope, Vault.DecryptMetadata로 batch 위임
2. Plain JSON dict (envelope 아님) — 직접 `json.loads` 사용
3. Base64-encoded JSON — `base64.b64decode` → `json.loads` (legacy?)

그리고 batch decrypt 실패 시 per-entry loop fallback (L452-464).

Go 포팅에서 이 모든 동작을 유지할지 단순화할지 쟁점.

### 쟁점별 선택

#### 1. AES decrypt 책임

| 옵션 | 설명 |
|---|---|
| A (Python 동일) ⭐ | Vault에 위임. rune-mcp에 AES decrypt 코드 없음 |
| B (rune 직접) | rune이 agent_dek으로 직접 decrypt. 1 RPC 감소 |

**A 선택**. 이유:
- Python bit-identical 원칙
- Vault가 AES key lifecycle을 single source로 유지 (key rotation, revoke 대응)
- Capture가 rune 직접 encrypt하는 건 "새 레코드의 plaintext를 이미 알고 있기 때문" — recall은 다수의 기존 encrypted 레코드를 한 번에 복호화 → batch RPC가 효율
- 비대칭이지만 각 경로 특성에 맞는 선택

#### 2. Legacy base64 format 유지

| 옵션 | 설명 |
|---|---|
| A (유지) ⭐ | Python L430-440의 base64 fallback 경로 이식 |
| B (제거) | envelope 전용으로 단순화 |

**A 선택**. 이유:
- 기존 enVector 데이터 중 base64 포맷으로 저장된 레코드가 있을 수 있음 (Python이 이 fallback을 유지하는 이유)
- 제거 시 historical data 접근 불가능
- 비용: Go 구현에 15줄 내외 추가

#### 3. Batch decrypt 실패 시 per-entry fallback

| 옵션 | 설명 |
|---|---|
| A (유지) ⭐ | Python L452-464 per-entry loop fallback |
| B (제거) | Batch 실패 = 전체 실패로 단순화 |

**A 선택**. 이유:
- Batch 실패 원인이 "1개 entry corrupted"일 때 나머지 N-1개 유지 가능
- Fallback 없으면 1개 corrupt로 recall 전체 실패 → user-facing 품질 저하
- Python도 이 fallback으로 자주 구제되는 시나리오 (부분 corrupt data)

### 구현

```go
type metadataFormat int
const (
    fmtAESEnvelope metadataFormat = iota
    fmtPlainJSON
    fmtBase64JSON
)

func classifyMetadata(data string) (metadataFormat, any) {
    // 1. JSON parse 시도
    var parsed any
    if err := json.Unmarshal([]byte(data), &parsed); err == nil {
        if m, ok := parsed.(map[string]any); ok {
            if _, hasA := m["a"]; hasA {
                if _, hasC := m["c"]; hasC {
                    return fmtAESEnvelope, data  // 원문 유지 (Vault에 넘김)
                }
            }
        }
        return fmtPlainJSON, parsed
    }
    // 2. Base64 → JSON fallback
    if raw, err := base64.StdEncoding.DecodeString(data); err == nil {
        var parsed2 any
        if err := json.Unmarshal(raw, &parsed2); err == nil {
            return fmtBase64JSON, parsed2
        }
    }
    // 3. unrecognized — skip
    return -1, nil
}

// Batch decrypt
aesItems := []aesItem{}  // (idx, data) pairs
for i, e := range entries {
    fmt, val := classifyMetadata(e.Data)
    switch fmt {
    case fmtAESEnvelope:
        aesItems = append(aesItems, aesItem{i, e.Data})
    case fmtPlainJSON, fmtBase64JSON:
        entries[i].Metadata = val
    }
}

if len(aesItems) > 0 {
    dataList := collectData(aesItems)
    decrypted, err := vault.DecryptMetadata(ctx, dataList)
    if err == nil && len(decrypted) == len(aesItems) {
        for k, it := range aesItems {
            entries[it.idx].Metadata = decrypted[k]
        }
    } else {
        // Per-entry fallback
        for _, it := range aesItems {
            d, e := vault.DecryptMetadata(ctx, []string{it.data})
            if e == nil && len(d) > 0 {
                entries[it.idx].Metadata = d[0]
            } else {
                entries[it.idx].Metadata = map[string]any{}
            }
        }
    }
}
```

### 재평가 트리거

- Legacy base64 format 데이터가 더 이상 존재하지 않는 것이 확인된 경우 → 해당 경로 제거 (option 2B)
- Vault AES decrypt RPC 가용성이 낮아 rune 직접 decrypt가 필요해지는 경우 → option 1B 고려 (agent_dek 수명 주기 재설계 필요)

---

## D27. Recall Phase 6 phase_chain expansion 유지 여부

**상태**: ✅ Decided (2026-04-21) — 유지 (Python 동일, MVP)

### 배경

Python `searcher.py:306-365` `_expand_phase_chains`는 Phase 6의 첫 단계에서 동작:

1. 현재 `results` 내 phase chain 레코드들을 스캔
2. 각 group에 대해 "sibling 누락" 감지 (`phase_total > 실제 results 내 해당 group 멤버 수`)
3. 누락이 있는 group 중 **최대 2개** (`max_chains=2`)를 대상으로 **추가 검색**:
   ```python
   siblings = await self._search_single(f"Group: {group_id}", topk=10)
   ```
4. 각 추가 검색 = 4 RPC (envector.Score → Vault.DecryptScores → envector.Remind → Vault.DecryptMetadata)
5. 즉 최악 case **+8 RPC** latency 추가 + `"Group: {group_id}"` 문자열을 embed에 넣어 sibling 발견에 의존

### 쟁점

이 방식은 다음 성질을 가짐:
- **Hack-y**: `group_id`를 자연어 query처럼 embed — 의미적 매칭에 베팅. 실제 envector에 `group_id`가 DecisionRecord text에 포함되어 있어야 매칭 발생
- **Latency**: MVP 단계 recall의 최악 latency를 크게 늘림 (순차 실행 + 8 RPC)
- **Completeness 가치**: phase chain이 흩어져 반환될 때 이를 한 묶음으로 제시하는 UX 개선

### 선택지

**A. Python 동일 — phase_chain expansion 유지** ⭐
- Python bit-identical 원칙 유지 (D25와 일관)
- Phase chain UX 완결성 보장
- Latency 상승 감수 (MVP 단계)

**B. MVP에서는 phase_chain expansion 미구현**
- 구현 단순화
- Phase chain이 불완전하게 반환될 수 있음 (일부 sibling 누락)
- `_assemble_groups`는 남기되 expansion만 skip

### 결론

**Option A 채택**. Python 방식을 그대로 가져가는 원칙. Latency 이슈는 실측 후 D25 재평가와 함께 처리.

### 구현

```go
func (s *recallService) expandPhaseChains(
    ctx context.Context, results []SearchHit, vec []float32,
) []SearchHit {
    const maxChains = 2
    seenGroups := map[string]struct{}{}
    var toExpand []string

    for _, r := range results {
        if r.GroupID == nil { continue }
        gid := *r.GroupID
        if _, dup := seenGroups[gid]; dup { continue }

        total := 0
        if r.PhaseTotal != nil { total = *r.PhaseTotal }
        present := 0
        for _, x := range results {
            if x.GroupID != nil && *x.GroupID == gid { present++ }
        }
        if present < total {
            seenGroups[gid] = struct{}{}
            toExpand = append(toExpand, gid)
            if len(toExpand) >= maxChains { break }
        }
    }

    if len(toExpand) == 0 { return results }

    existingIDs := map[string]struct{}{}
    for _, r := range results { existingIDs[r.RecordID] = struct{}{} }

    groupSiblings := map[string][]SearchHit{}
    for _, gid := range toExpand {
        // Python: _search_single(f"Group: {group_id}", topk=10)
        query := fmt.Sprintf("Group: %s", gid)
        groupVec, err := s.embedder.EmbedSingle(ctx, query)
        if err != nil { continue }
        siblings, err := s.searchSingle(ctx, groupVec, 10)
        if err != nil { continue }

        var chain []SearchHit
        for _, sib := range siblings {
            if sib.GroupID != nil && *sib.GroupID == gid {
                if _, exists := existingIDs[sib.RecordID]; !exists {
                    chain = append(chain, sib)
                }
            }
        }
        sort.SliceStable(chain, func(i, j int) bool {
            return phaseSeqOrZero(chain[i]) < phaseSeqOrZero(chain[j])
        })
        groupSiblings[gid] = chain
    }

    // Merge: insert expanded siblings in phase order
    // (Python L343-365 동일 로직)
    ...
}
```

### 재평가 트리거

- D25와 동일: Recall p50 latency > 500ms 또는 phase_chain expansion이 실제 recall 중 발생 빈도 측정
- Phase chain 레코드 구조를 변경하여 expansion 없이 완전한 결과 보장 가능한 경우
- Envector 측에서 group_id 기반 필터 API 제공하는 경우

---

## D28. Recall 응답 포맷

**상태**: ✅ Decided (2026-04-21) — agent-delegated 경로만 구현 (synthesized 경로 제거)

### 배경

Python `server.py:tool_recall` (L950-1003)은 두 개의 응답 경로를 가짐:

**Agent-delegated 경로** (L953-990, synthesizer 없거나 LLM key 없을 때):
```json
{
  "ok": true,
  "found": <count>,
  "results": [ {record_id, title, content, domain, certainty, score, [group_id, group_type, phase_seq, phase_total]}, ... ],
  "confidence": <float>,
  "sources": [ <top 5 요약> ],
  "synthesized": false
}
```

**Server-side synthesizer 경로** (L992-1003, LLM key 있을 때):
```json
{
  "ok": true,
  "found": <count>,
  "answer": "<LLM answer>",
  "confidence": <float>,
  "sources": [...],
  "warnings": [...],
  "related_queries": [...],
  "synthesized": true
}
```

Server-side synthesizer는 rune 내부에서 LLM 호출 (`synthesizer.synthesize(query, results)`) → answer 텍스트 합성.

### Go 철학과의 일관성

Go rune-mcp는 다음 원칙으로 수렴 중:
- **D14**: capture `record_builder` LLM fallback 제거 → rune은 agent가 만든 extracted JSON만 받음
- **D21**: query 번역 LLM 제거 → agent가 영어로 번역 후 호출
- **D26**: AES decrypt는 Vault 위임, rune은 LLM/crypto 계산 거의 없음

즉 **rune-mcp는 LLM-free 원칙**으로 일관. Synthesizer 경로를 구현하려면:
- LLM client (Anthropic/OpenAI/Gemini) 의존성 추가
- API key 관리, timeout, 에러 처리
- 실제 유저 가치는 agent가 이미 답변 합성 가능 (Claude Code 등) → 중복

### 선택지

**A. Agent-delegated 경로만 구현 — synthesized 필드는 항상 false** ⭐
- Python 응답 schema 유지 (agent 호환)
- rune은 LLM-free
- `synthesized: false` 고정값으로 삽입

**B. Synthesized 필드 자체 제거**
- 응답 포맷 단순화
- Python과 schema 차이 발생 → agent (scribe md, retriever md) 업데이트 필요

**C. Synthesized 경로까지 구현**
- Python 양쪽 경로 유지
- LLM 의존성 + 복잡도 증가. 철학(D14/D21) 위반

### 결론

**Option A 채택**. 이유:
1. Python 응답 schema와 **완전 호환** — Claude Code 등 기존 agent가 파싱 변경 없이 동작
2. rune-mcp LLM-free 원칙 유지 (D14/D21/D26과 일관)
3. Answer 합성은 agent 측이 이미 훨씬 잘 수행 (context 보유 + 대화 흐름)
4. 미래 synthesized 경로 필요 시 Option C로 전환 가능 (B를 피해 backward-compat 확보)

### 구현

```go
type RecallResult struct {
    OK          bool              `json:"ok"`
    Found       int               `json:"found"`
    Results     []RecallHit       `json:"results"`
    Confidence  float64           `json:"confidence"`
    Sources     []RecallSource    `json:"sources"`
    Synthesized bool              `json:"synthesized"`  // 항상 false
}

type RecallHit struct {
    RecordID   string   `json:"record_id"`
    Title      string   `json:"title"`
    Content    string   `json:"content"`      // payload_text
    Domain     string   `json:"domain"`
    Certainty  string   `json:"certainty"`
    Score      float64  `json:"score"`
    GroupID    *string  `json:"group_id,omitempty"`
    GroupType  *string  `json:"group_type,omitempty"`
    PhaseSeq   *int     `json:"phase_seq,omitempty"`
    PhaseTotal *int     `json:"phase_total,omitempty"`
}

type RecallSource struct {
    RecordID  string  `json:"record_id"`
    Title     string  `json:"title"`
    Domain    string  `json:"domain"`
    Certainty string  `json:"certainty"`
    Score     float64 `json:"score"`
}
```

`answer`, `warnings`, `related_queries` 필드는 **구현 안 함**. agent가 `results` + `content`로 답변 합성 담당.

### 재평가 트리거

- 기업 고객이 "agent 없는 순수 rune-mcp 호출에서도 answer 필요" 요구
- Multi-tenant 환경에서 agent-delegated가 작동하지 않는 시나리오
- Pipeline 측정에서 agent 답변 품질이 유의미하게 저하되어 server-side LLM이 필요한 경우

재평가 시 Option C로 전환: `internal/adapters/llm/` 패키지 신설 + synthesizer 구현. 응답 schema는 이미 `synthesized` 필드를 갖고 있어 변경 없음.

---

## D29. Rune-embedder 런타임

**상태**: 📦 Archived (2026-04-22) — embedder 담당 범위로 이관  
**이전 결론**: llama-server (Option B)  
**현재 상태**: **embedder 프로젝트가 임베딩 서비스를 자체 데몬(가칭 `embedder`)으로 제공**. 런타임 선택(ONNX vs llama-server) 및 구현 세부는 embedder 프로젝트 결정 사항. 참고로 embedder도 llama-server를 채택한 것으로 알려져 있으나, 이 프로젝트는 이 선택과 독립적으로 gRPC 계약만 따른다. 이 결정 항목은 아래 근거 기록을 히스토리로 보존.  
**이관 위치**: (embedder 프로젝트 문서) · 클라이언트 측은 `components/embedder-integration.md`

### 배경 (히스토리)

`rune-embedder`는 임베딩 모델을 호스팅하는 **공유 상주 데몬**. 각 `rune-mcp` 세션이 unix socket(`~/.rune/embedder.sock`, D6)으로 HTTP+JSON 요청을 보낸다. 모델이 세션별로 메모리 복제되지 않도록 분리한 것이 v0.4.0 아키텍처의 핵심 동기 (README 참조).

문제는 **어떤 런타임으로 모델을 실행할 것인가**:

### 선택지

**A. ONNX Runtime (직접 링크)**
- `github.com/sugarme/gotch` 혹은 CGO 기반 ONNX 바인딩을 Go에 직접 링크
- 모델: BGE-large-en-v1.5 등을 ONNX export → quantization
- 장점: 단일 바이너리, 외부 의존성 최소
- 단점:
  - CGO 복잡도 (빌드 · 크로스 컴파일 · 바이너리 사이즈)
  - Go에서 ONNX Runtime 바인딩 성숙도 낮음 (Python 대비)
  - 모델 교체 시 Go 코드 재빌드 필요
  - 다른 프로젝트 재사용 어려움 (rune 전용 바이너리)

**B. llama-server (별도 프로세스 + HTTP)** ⭐
- `llama.cpp`의 `llama-server` 바이너리를 별도 프로세스로 실행
- rune-embedder는 thin wrapper — llama-server에 HTTP proxy만 수행
- 모델: GGUF 포맷 (quantized), BGE 류 임베딩 모델 공식 지원
- 장점:
  - **재사용성**: llama-server 자체는 rune과 독립 — 다른 프로젝트도 같은 데몬 공유 가능
  - **생태계**: llama.cpp GGUF 모델 다수, quantization tooling 성숙
  - **HTTP 인터페이스 기본 제공**: rune-embedder wrapper가 얇음 (socket proxy만)
  - **독립 업데이트**: llama-server 바이너리만 교체해도 rune 영향 없음
  - **CGO 불필요**: Go 측은 HTTP 클라이언트만
- 단점:
  - 별도 바이너리 의존성 (`brew install llama.cpp` 혹은 동등)
  - 프로세스 2개 (rune-embedder + llama-server) — 관리 복잡도 소폭 증가
  - HTTP 오버헤드 (localhost 수 ms, 실제로는 무시할 수준)

### 결론

**Option B 채택 (llama-server)**. 결정 근거:

1. **재사용성** — 사용자 코멘트: *"임베딩 모델만 따로 관리해야 나중에 다른 프로젝트에서 재사용할 수 있을 것 같다"*. llama-server는 rune 독립적으로 동작하는 표준 HTTP 서버라 CryptoLab 내 다른 Go 프로젝트도 같은 데몬을 공유 가능.
2. **CGO 회피** — Go 바이너리의 최대 매력은 순수 Go 빌드. ONNX 직접 링크는 CGO 필수 → cross-build 복잡, CI에 C toolchain 요구.
3. **모델 교체 용이** — GGUF 포맷은 커뮤니티 모델 풍부. llama.cpp가 활발한 업데이트.
4. **실측 충분성** — Unix socket + HTTP는 수 ms RTT. Embedding 자체 forward-pass는 수십~수백 ms → HTTP 오버헤드는 무시 가능.

### 구현

#### rune-embedder 구조

```
cmd/rune-embedder/main.go
  ├── llama-server 관리 (fork + supervise)
  ├── unix socket listener (~/.rune/embedder.sock)
  └── HTTP handler: POST /v1/embed → llama-server /v1/embeddings 프록시
```

#### API 호환

rune-mcp → rune-embedder는 capture/recall flow에서 이미 정의된 형태:

```http
POST /v1/embed
{"texts": ["...", "..."]}
```

응답:
```json
{"vectors": [[...], [...]], "model": "bge-large-en-v1.5", "dim": 1024}
```

rune-embedder는 내부적으로 llama-server의 OpenAI-호환 `/v1/embeddings` endpoint를 호출:

```http
POST http://127.0.0.1:<llama-port>/v1/embeddings
{"input": ["...", "..."], "model": "bge-large-en-v1.5"}
```

필요 시 batch → single 분해 (llama-server batch 지원 여부 확인 후 조정, 현재 llama-server는 batch 지원).

#### llama-server 기동

- `components/rune-embedder.md`에 정의된 launchd/systemd unit이 llama-server 바이너리를 자식 프로세스로 fork
- 경로: `/opt/homebrew/bin/llama-server` (macOS Homebrew) · `/usr/local/bin/llama-server` (Linux 기본)
- 모델 경로: `~/.rune/models/bge-large-en-v1.5-q8_0.gguf` (사용자 제공 기본값)

### 재평가 트리거

- llama-server 빌드/배포가 어려워지는 경우 (licensing, binary hosting 이슈)
- Embedding latency가 병목 + HTTP RTT가 실측에서 critical path의 > 10%인 경우 (ONNX 직접 링크로 인한 절감 가치 평가)
- GPU 가속이 필요해지는 경우 (llama-server CUDA 빌드 지원, 우선순위 낮음)

---

## D30. Embedder 통신 프로토콜

**상태**: ✅ Decided (2026-04-22) — gRPC over Unix socket

### 배경

초기 v0.4.0 설계에서 rune-mcp ↔ 임베딩 데몬 간 통신은 **HTTP+JSON over Unix socket**으로 가정됨 (이전 D6 · D29 문서 참조). 이 가정 하에 `flows/capture.md` Phase 6, `flows/recall.md` Phase 3에 HTTP 호출 예시 작성됨.

**아키텍처 재정리** (2026-04-22):
- 초기에는 "runed"라는 이름의 **중앙화 통합 데몬** (Python MCP 대체 + 임베딩 내장)으로 구상 → 폐기
- 현재 구조:
  - **rune-mcp** (이 프로젝트): Python MCP를 Go로 포팅. 임베딩 제외한 모든 기능.
  - **embedder** (별도 프로세스, 가칭): 임베딩만 전담. gRPC 서버.
- 두 프로세스 간 통신: **gRPC over Unix domain socket**

### Proto 계약 (embedder-provided)

> Proto 파일은 embedder 프로젝트가 정의·관리. 서비스·패키지 이름은 embedder 팀이 최종 결정. 아래는 rune-mcp가 **사용하는** 인터페이스의 placeholder.

```
service EmbedderService {
    rpc Embed(EmbedRequest) returns (EmbedResponse);
    rpc EmbedBatch(EmbedBatchRequest) returns (EmbedBatchResponse);
    rpc Info(InfoRequest) returns (InfoResponse);
    rpc Health(HealthRequest) returns (HealthResponse);
    rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

- Transport: gRPC over Unix domain socket (macOS/Linux) 또는 named pipe (Windows)
- 패키지/서비스 이름: embedder 팀 결정 (placeholder `embedder.v1`)
- L2-normalization: embedder가 자동 수행 (opt-out 없음)
- 한도 노출: `Info` 응답이 `max_text_length`, `max_batch_size`, `vector_dim`, `model_identity` 반환

### 선택지

**A. HTTP+JSON (이전 가정)**
- rune-mcp가 표준 `net/http` + `encoding/json`으로 호출
- 장점: Go 표준 라이브러리, 의존성 없음
- 단점: embedder 측이 gRPC로 확정 → 양쪽 다른 스택 유지 비용

**B. gRPC** ⭐
- `google.golang.org/grpc` + 생성된 stub
- 장점: proto 계약 일원화, 버전 관리 일원화, 스키마 드리프트 탐지
- 단점: 의존성 추가 (protobuf·grpc-go), Go 바이너리 사이즈 소폭 증가

### 결론

**Option B 채택**. 이유:
1. **계약 일원화** — embedder 프로젝트가 유일한 소스 오브 트루스 (proto 파일). rune-mcp는 생성된 Go stub을 그대로 사용 → 스키마 mismatch 가능성 원천 차단.
2. **책임 경계 명확** — 임베딩 관련 모든 규약이 proto에 집약. rune-mcp에는 "어떻게 embed를 얻는가"만 있고 "어떻게 embed를 만드는가"는 없음.
3. **gRPC 의존성 경미** — 현재 rune은 이미 Vault · envector 등과 gRPC 통신. 새 의존성 아님.

### 구현 방침

**클라이언트 생성**:
- embedder 프로젝트가 제공하는 generated Go stub을 `go.mod`로 import
- `internal/adapters/embedder/` 패키지에 얇은 래퍼:
  ```go
  type Client interface {
      EmbedSingle(ctx context.Context, text string) ([]float32, error)
      EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
      Info(ctx context.Context) (InfoSnapshot, error)
      Health(ctx context.Context) (HealthSnapshot, error)
  }
  ```
- 생성된 client stub 위에 retry(D7)·context timeout을 감싸는 정도

**Socket 경로**:
- 빌드 시 고정 아님 (D6 archived). config / env 변수로 전달받아 `grpc.Dial("unix://"+sockPath, ...)`
- 기본값: embedder 프로젝트가 정의한 convention을 따름. `~/.rune/config.json`의 `embedder.socket_path` 필드로 override 가능

**Info 활용**:
- rune-mcp 기동 직후(또는 첫 embed 호출 전)에 `Info` 1회 호출
- 응답을 메모리에 캐시: `max_batch_size`는 D16/D23의 batch embed 시 split 기준으로 사용
- `model_identity`는 메모리에 기록(추후 저장된 embedding과 불일치 감지 시 재임베딩 트리거, MVP에서는 로깅만)
- `vector_dim`은 기대값(1024)과 비교, 불일치 시 에러

**Health 활용**:
- D8(부팅 시 health 폴링 여부)는 여전히 "부팅 시 폴링 안 함" 유지. 첫 embed 호출 시 실패하면 retry → `DEGRADED`/`LOADING` 식별해 waitable error 반환

**Shutdown**:
- rune-mcp는 embedder 생애주기 관리 **안 함**. Shutdown RPC는 **호출하지 않음** (embedder 프로젝트 · 운영 도구 담당)

### 재평가 트리거

- Proto 계약에 breaking change — embedder 프로젝트와 버전 negotiation 필요 시
- 동일 프로세스 내 Go native 임베딩(예: 향후 ONNX Runtime Go 내장)이 필요해져 embedder 의존을 끊는 경우
- gRPC가 Unix socket 환경에서 예상치 못한 문제를 일으키는 경우 (우선순위 낮음, 실측 필요)

### 이관 위치

- `components/embedder-integration.md` — 클라이언트 사용법 · proto 요약 · Info 캐시 정책
- `flows/capture.md` Phase 3 · `flows/recall.md` Phase 3 — 호출 코드 샘플 (HTTP → gRPC)
- `architecture.md` — 3-프로세스 그림에 "embedder는 외부 프로세스" 명시

---

## 새 결정 항목 추가 규칙

새 결정이 필요해지면 다음 양식으로 추가:

```markdown
## D<N>. <짧은 제목>

**상태**: 🔴 Blocking | 🟡 Pending | 🔵 Deferred | ✅ Decided  (YYYY-MM-DD)
**결정 필요 시점**: <언제까지 결정되어야 하나>
**관련**: <관련 컴포넌트·ADR·섹션 링크>

### 문제
<왜 결정이 필요한가>

### 왜 (Deferred / Pending / ...)
<현재 상태에 대한 설명>

### 선택지
| | 내용 | 비용 |
|---|---|---|
| (a) | ... | ... |
| (b) | ... | ... |

### 권장
<어느 쪽으로 기울어있는지, 왜>

### 결정 조건
<언제 돌아와야 하나 · 어떤 신호가 트리거인가>
```
