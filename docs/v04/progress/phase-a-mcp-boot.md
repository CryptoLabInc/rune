# Phase A — MCP boot (handshake + tools/list)

> **합격 상태**: ✅ 통과 (2026-04-25)
> **관련 커밋**: [`19b7bf6`](../../../) — `feat(go): first MCP boot — handshake + tools/list (Phase A)`
> **브랜치**: `yg/first-mcp-boot` (origin/`yg/first-mcp-boot`)
> **수정 파일 5개**: `cmd/rune-mcp/main.go` · `internal/mcp/tools.go` · `go.mod` · `go.sum` · `.gitignore`

## 목적

`rune-mcp` Go 바이너리가 **외부 의존성 없이** 다음을 수행하는 첫 번째 마일스톤:

1. 빌드되고 정적 바이너리로 떨어진다
2. stdio JSON-RPC로 MCP 클라이언트(Claude Code 등)와 `initialize` handshake에 응답한다
3. `tools/list` 응답에 8개 tool을 자동 추론된 schema와 함께 광고한다
4. `tools/call` 응답은 "not yet implemented" stub이지만 JSON-RPC 자체는 valid
5. stdin EOF · SIGINT · SIGTERM 모두 exit 0으로 정상 종료

> 이 단계의 가치 — 비즈니스 로직 0이지만 **MCP 프로토콜 표면**이 살아있다. 즉 Claude Code가 우리 바이너리를 spawn해서 도구 카탈로그를 정상 인식한다. 이후 phase는 각 tool 본체를 채워가는 작업으로, 매번 같은 검증 회로를 재사용한다.

---

## 1. 동작하는 기능 6가지

### F1. 바이너리 빌드 / 실행

- `go build` 한 번으로 단일 정적 바이너리 (Python venv 자가복구 같은 부트스트랩 제거)
- 환경 변수 · config.json 모두 미사용 (Phase A 한정)
- stdin/stdout으로만 통신
- 산출 크기: 약 8.3 MB

### F2. MCP `initialize` handshake

JSON-RPC 2.0의 `initialize` 메서드 요청에 응답.

**요청 예**:
```json
{"jsonrpc":"2.0","id":1,"method":"initialize",
 "params":{"protocolVersion":"2024-11-05","capabilities":{},
 "clientInfo":{"name":"x","version":"0.0.1"}}}
```

**응답** (실측):
```json
{"jsonrpc":"2.0","id":1,"result":{
  "capabilities":{"logging":{},"tools":{"listChanged":true}},
  "protocolVersion":"2024-11-05",
  "serverInfo":{"name":"rune-mcp","version":"0.4.0-alpha"}}}
```

`serverInfo`는 `cmd/rune-mcp/main.go:33`의 `version` 상수를 그대로 광고. capabilities로 tools와 logging 카테고리를 클라이언트에게 알린다.

### F3. `tools/list` — 8개 tool 카탈로그

Python `mcp/server/server.py`와 bit-identical한 8 tool 이름:

```
rune_batch_capture
rune_capture
rune_capture_history
rune_delete_capture
rune_diagnostics
rune_recall
rune_reload_pipelines
rune_vault_status
```

각 tool마다:
- `name` — 위 8개 중 하나
- `description` — Claude가 도구 선택 시 읽는 한 문장 (`internal/mcp/tools.go::Register`에서 정의)
- `inputSchema` — Go input 타입에서 자동 추출
- `outputSchema` — Go output 타입에서 자동 추출

### F4. JSON Schema 자동 추론

SDK(`github.com/google/jsonschema-go`)가 Go struct를 보고 다음을 자동 변환:

| Go 표현 | JSON Schema |
|---|---|
| `string` / `int` / `float64` / `bool` | `{"type":"string"}` 등 |
| `*T` 포인터 | `{"type":["null","string"]}` (nullable) |
| `[]T` | `{"type":"array","items":{...}}` |
| nested struct | `{"type":"object","properties":{...}}` |
| `json:"x,omitempty"` | `required` 배열에서 제외 |
| `additionalProperties` | 기본 false (struct 한정) |

→ **Go 타입 정의 = MCP API 계약**. 별도 IDL 없음.

예: `rune_recall`의 input은 `domain.RecallArgs` (Go struct)에서 자동으로:
```json
{"type":"object",
 "properties":{"query":{"type":"string"},"topk":{"type":"integer"},
   "domain":{"type":["null","string"]},
   "status":{"type":["null","string"]},
   "since":{"type":["null","string"]}},
 "required":["query"]}
```

### F5. `tools/call` — stub 응답 (Phase A 한정)

8 tool 어느 것을 호출해도 동일한 형태로 응답:

```json
{"jsonrpc":"2.0","id":3,"result":{
  "isError":true,
  "content":[{"type":"text",
    "text":"<tool_name> is not yet implemented (skeleton phase A — MCP handshake + tools/list only)."}],
  "structuredContent":{... output 타입의 zero value ...}}}
```

핵심:
- JSON-RPC 자체는 정상 (오류 처리 valid)
- `isError: true` → Claude UI에서는 빨간 에러로 표시
- `structuredContent`에 zero value의 output struct가 포함됨 → Phase 5에서 진짜 데이터로 교체될 자리

### F6. 정상 종료 처리

다음 4가지 종료 사유 모두 **exit 0** + 깔끔한 종료:

- stdin EOF (Claude 창 닫힘)
- SIGINT (`Ctrl-C`)
- SIGTERM (`kill <pid>`)
- 컨텍스트 cancel

`cmd/rune-mcp/main.go::isNormalShutdown(err)` 함수가 `io.EOF` · `context.Canceled` · `"server is closing"` (jsonrpc2 internal error 메시지) 모두 정상으로 분류.

---

## 2. 동작하지 않는 것 (Phase A 한계)

| 영역 | 상태 | 가능 시점 |
|---|---|---|
| 실제 capture / recall 비즈니스 로직 | 미구현 | Phase 5 (`service/*` 채워질 때) |
| Vault gRPC 연결 | 미구현 | Phase 4 |
| Envector SDK 연결 | 미구현 | Phase 4 (Q4 PR 머지 후) |
| Embedder gRPC 연결 | 미구현 | Phase 4 (proto stub 필요) |
| 상태 머신 (`lifecycle.Manager`) | 미작동 | Phase 4 (boot loop 시작) |
| `CheckState` 게이트 | 코드는 있으나 호출 안 됨 | Phase 5 |
| `request_id` 로깅 | 미구현 | Phase 4 (`obs/slog.go` 보강) |
| `SensitiveFilter` redaction | 미구현 | 동상 |
| `config.json` 로딩 | 미구현 (빈 Deps) | Phase 4 |
| `capture_log.jsonl` 쓰기/읽기 | 미구현 | Phase 4 (`logio` 본격 구현) |

→ Phase A의 정확한 범위는 **"MCP 프로토콜 표면이 정상 동작한다"** 까지. 어느 tool을 호출해도 비즈니스 로직 0.

---

## 3. 기능 확인 — 3개 레벨

### Level 1 — CLI 직접 (외부 의존성 0)

가장 빠른 검증. Claude Code도 Inspector도 필요 없음.

#### 1.1. 빌드

```bash
cd /Users/redcourage/cryptolab/rune-project/rune
go build -o bin/rune-mcp ./cmd/rune-mcp
ls -la bin/rune-mcp
```

**기대**: `-rwxr-xr-x ... bin/rune-mcp` (~8 MB). 컴파일 에러 없음.

#### 1.2. 종료 (stdin EOF)

```bash
./bin/rune-mcp < /dev/null; echo "exit=$?"
```

**기대**: `exit=0`, 다른 출력 없음.

#### 1.3. `initialize` 응답

```bash
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0.0.1"}}}'
  sleep 0.3
} | ./bin/rune-mcp | jq .
```

**기대**: `serverInfo.name == "rune-mcp"`, `version == "0.4.0-alpha"`, `capabilities.tools` 광고됨.

#### 1.4. `tools/list` — 이름 8개

```bash
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0.0.1"}}}'
  sleep 0.3
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep 0.1
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
  sleep 0.5
} | ./bin/rune-mcp 2>/dev/null | jq -r 'select(.id==2) | .result.tools[].name'
```

**기대 출력** (8줄):

```
rune_batch_capture
rune_capture
rune_capture_history
rune_delete_capture
rune_diagnostics
rune_recall
rune_reload_pipelines
rune_vault_status
```

#### 1.5. 특정 tool의 input schema

```bash
# 위 1.4 명령에서 마지막 jq만 변경
| jq 'select(.id==2) | .result.tools[] | select(.name=="rune_recall") | .inputSchema'
```

**기대**: `required: ["query"]`, 나머지 4 필드는 nullable optional.

#### 1.6. `tools/call` — stub 응답

```bash
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0.0.1"}}}'
  sleep 0.3
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep 0.1
  echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"rune_diagnostics","arguments":{}}}'
  sleep 0.5
} | ./bin/rune-mcp 2>/dev/null | jq 'select(.id==3) | {isError: .result.isError, text: .result.content[0].text}'
```

**기대**:

```json
{"isError": true,
 "text": "rune_diagnostics is not yet implemented (skeleton phase A — MCP handshake + tools/list only)."}
```

#### Level 1 합격 기준

1.1 ~ 1.6 모두 위 기대 결과대로면 합격.

---

### Level 2 — Claude Code에 등록해서 실전 확인

#### 2.1. mcp.json 위치 확인

```bash
ls -la ~/.claude/mcp.json 2>/dev/null
```

- 파일 있음 → `cp ~/.claude/mcp.json ~/.claude/mcp.json.backup` 으로 백업
- 파일 없음 → 새로 만든다

#### 2.2. entry 추가

**처음 만드는 경우** — 다음 그대로 저장:

```json
{
  "mcpServers": {
    "rune-go-dev": {
      "command": "/Users/redcourage/cryptolab/rune-project/rune/bin/rune-mcp"
    }
  }
}
```

**기존 파일이 있는 경우** — `mcpServers` 객체 안에 `rune-go-dev` 키만 추가 (기존 entries 보존).

> ⚠️ 기존 `envector` (Python MCP) entry를 절대 삭제하지 말 것. 두 MCP 공존 가능 — tool 이름 충돌 없음 (`rune_*` 8개는 Go 쪽 전용).

#### 2.3. Claude Code 재시작

- 모든 Claude 창 종료 후 재실행
- 또는 Cmd+Q → 재실행

#### 2.4. tool 인식 확인 (3가지 방법 중 택일)

**방법 A — `/mcp` 슬래시 명령**: 새 채팅 입력창에 `/mcp` 입력 → 등록된 MCP 서버 목록 표시 → `rune-go-dev` 항목이 "connected" 상태로 보여야 함

**방법 B — 도구 아이콘**: 입력창 옆 도구/플러그인 아이콘 → `rune-go-dev` 펼치면 8 tool 리스트

**방법 C — 직접 호출**: Claude에게 "rune_diagnostics 호출해서 결과 보여줘" → tool 인식 후 호출 → 빨간 에러 메시지 (`not yet implemented`) 표시되면 정상. "그런 도구 못 찾았어"가 나오면 등록 실패

#### 2.5. (선택) Claude Code 로그 확인

- macOS Cmd+Shift+P → "Open Output" → MCP 카테고리
- `rune-go-dev: connecting...` → `connected`. 에러 없어야 함

#### Level 2 합격 기준

- `/mcp` 또는 도구 목록에 `rune-go-dev` + 8 tool 표시
- 임의 tool 호출 시 not implemented 응답 (이게 정상)

---

### Level 3 — MCP Inspector (시각적, 선택)

#### 3.1. 실행

```bash
cd /Users/redcourage/cryptolab/rune-project/rune
npx -y @modelcontextprotocol/inspector ./bin/rune-mcp
```

브라우저 자동 오픈 (보통 `localhost:6274`).

#### 3.2. UI에서 확인

- **Server info**: `rune-mcp` 0.4.0-alpha
- **Tools 탭**: 8개 list. 각 클릭 → input/output schema 시각적 표시
- **Tool 호출**: 클릭 → 폼에 인자 → "Run tool" → response (Phase A는 isError)
- **History**: JSON-RPC 메시지 raw 보기

#### Level 3 합격 기준

- 8 tool list 표시됨
- 각 tool schema가 올바른 모양 (required field 표시 등)
- 임의 호출 시 빨간 에러

---

## 4. Troubleshooting

| 증상 | 가능한 원인 | 해결 |
|---|---|---|
| `go build` 실패: `go >= 1.25.0 required` | Go 버전 낮음 | `go install golang.org/dl/go1.25@latest && go1.25 download`, 또는 brew/asdf로 1.25 업그레이드 |
| `go build` 실패: `missing go.sum entry` | `go mod tidy` 안 함 | `go mod tidy` 후 재빌드 |
| Level 1.3 응답 없음 | sleep 너무 짧아 EOF 먼저 옴 | sleep을 0.5 이상으로 |
| Level 2에서 tool 안 보임 | 절대 경로 아님 / JSON 문법 오류 / 권한 부족 | `cat ~/.claude/mcp.json \| jq .`로 JSON 검증 + `chmod +x bin/rune-mcp` |
| Level 2 "Connection failed" | 바이너리 경로 틀림 / 바이너리 stale | 경로 재확인 + `go build` 재실행 |
| Level 2에서 not implemented 안 나오고 다른 에러 | tool 이름 오타 | 8 이름 정확히 입력 |
| Level 3 npx 실패 | Node.js 미설치 | `brew install node` 또는 Level 1·2로 우회 |

---

## 5. 코드 변경 요약

### `cmd/rune-mcp/main.go` (rewrite, 80줄)

스텁 1줄짜리 `log.Println("rune-mcp skeleton — not yet implemented")` 를 다음으로 교체:

- `context.WithCancel` + signal handler (SIGINT/SIGTERM → cancel)
- 빈 `mcp.Deps{}` (Phase A는 adapter 미주입)
- `sdkmcp.NewServer(&Implementation{Name:"rune-mcp", Version:"0.4.0-alpha"}, nil)`
- `mcp.Register(srv, deps)` — 8 tool 등록
- `srv.Run(ctx, &sdkmcp.StdioTransport{})`
- `isNormalShutdown(err)` 헬퍼 — io.EOF · ctx Canceled · "server is closing" 모두 정상으로 분류 → exit 0

### `internal/mcp/tools.go` (rewrite, 137줄)

기존 8 tool stub 함수를 SDK handler 패턴으로 재구성:

- `Register(srv, deps)` — 8 `sdkmcp.AddTool` 호출
- `stubHandler[In, Out any](toolName)` — generic factory. 어느 tool이든 `IsError=true` + 텍스트 메시지 + zero output 반환
- `stubResult(toolName)` — 메시지 빌더
- `Deps` 구조체는 여전히 필드 주석 처리 상태 (Phase 4부터 활성화)

### `go.mod` / `go.sum`

- `github.com/modelcontextprotocol/go-sdk v1.5.0` (D2)
- transitive deps: `jsonschema-go`, `uritemplate`, `oauth2`, `segmentio/encoding`, `golang-jwt/jwt`, ...
- Go toolchain `1.24` → `1.25` (SDK 요구)

### `.gitignore`

```
# Go build artifacts
bin/
*.test
coverage.out
```

---

## 6. 다음 마일스톤

이 문서가 통과되면 다음 둘 중 선택:

- **Phase B** — `rune_diagnostics`의 environment 섹션을 stdlib만으로 채워서 진짜 응답 받기 (작은 PR, 2-3시간)
  - `runtime.GOOS` · `runtime.Version` · `os.Getwd` · `runtime.GOARCH` 만으로 가능
  - 다른 6 섹션은 `null`/zero로 두고 `OK=true` 고정
  - 첫 번째 진짜 응답 → "MCP 표면 + 한 tool 데이터 흐름" 검증
- **Phase 1 본격** — `go.mod`에 gRPC · protobuf · envector-go SDK · embedder proto stub 추가. 이후 Phase 4 adapter 작업의 전제 조건
