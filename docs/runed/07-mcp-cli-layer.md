# MCP / CLI 레이어 설계

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스 실측 대조 완료. 주요 교정:
> §2.1 서브커맨드 표에 `activate`/`deactivate`/`reset`의 정확한 단계 기술 추가,
> dormant 상태에서도 8개 tool 모두 무조건 등록되는 모델 명시. §3.3 toolRoutes
> 매핑의 Python 도구명과 일치 (`reload_pipelines`, `capture_history`, `delete_capture`).

이 문서는 runed 데몬 위에 얹는 얇은 클라이언트 레이어 — MCP shim과 CLI —
의 설계를 기술한다. runed의 HTTP API가 실제 계약이고, 이 레이어는 그 위의
편의 어댑터다.

---

## 1. 전체 그림

```
┌─────────────────────────────────────────────────────────────────┐
│                   얇은 클라이언트 레이어                         │
│                                                                 │
│  ┌──────────────────────┐    ┌──────────────────────────────┐   │
│  │  rune CLI (500-800   │    │  rune-mcp (800-1200 LoC)     │   │
│  │   LoC)               │    │  MCP stdio ↔ HTTP 변환기      │   │
│  │                      │    │                              │   │
│  │  - argv → JSON body  │    │  - JSON-RPC stdin → HTTP POST│   │
│  │  - HTTP POST 전송    │    │  - HTTP resp → JSON-RPC stdout│  │
│  │  - stdout에 JSON출력 │    │  - tools/list 스키마 제공     │   │
│  │  - exit code 매핑    │    │  - 세션 동안 상주             │   │
│  └──────────┬───────────┘    └──────────────┬───────────────┘   │
│             │                               │                   │
│             │     HTTP POST (unix socket)    │                   │
│             └───────────────┬───────────────┘                   │
└─────────────────────────────┼───────────────────────────────────┘
                              ▼
                    ┌──────────────────┐
                    │ runed (데몬)      │
                    │ HTTP API         │
                    │ (동일한 API)      │
                    └──────────────────┘
```

**핵심**: 둘 다 runed의 같은 HTTP API를 호출한다. 차이는 앞단의 입력/출력
형식 변환 뿐.

---

## 2. rune CLI 설계

### 2.1 서브커맨드 매핑

| CLI 서브커맨드 | HTTP 메서드 | runed 엔드포인트 |
|---|---|---|
| `rune capture --text-to-embed "..." --metadata '{...}'` | POST | /capture |
| `rune batch-capture --items '[{...},{...}]'` | POST | /batch-capture |
| `rune recall --query "..." [--topk N]` | POST | /recall |
| `rune status` | GET | /diagnostics + /health |
| `rune history [--limit N]` | GET | /history |
| `rune delete <record_id>` | DELETE | /captures/:id |
| `rune reload` | POST | /reload |
| `rune daemon start\|stop\|restart\|health\|logs` | — | 데몬 프로세스 관리 |
| `rune configure` | — | config.json 파일 I/O (데몬 불필요) |
| `rune activate` | — + POST | **1단계**: config.json 읽기 + 필수 필드 검증 + state를 "active"로 업데이트. **2단계**: POST /reload로 데몬이 Vault fetch + 모델 로드 트리거. **3단계**: GET /diagnostics로 활성화 성공 확인 (Python `commands/claude/activate.md` 패턴) |
| `rune deactivate` | — + POST | config.json state를 "dormant"로 + `dormant_reason="user_deactivated"` 설정 + POST /reload로 데몬이 dormant 모드로 전이 |
| `rune reset` | — | config.json 삭제 + 데몬 정지 (launchctl/systemctl). 프로세스 cleanup은 선택적 (unit 유지 여부 결정) |

**중요**: Python 모델 실측 재확인:
- 8개 HTTP 엔드포인트는 데몬 상태와 무관하게 **모두 등록**됨 (`mcp/server/server.py` L487-1137 `@self.mcp.tool` 데코레이터)
- dormant 상태에서는 각 endpoint body가 runtime에 state 체크로 거절 (`_ensure_pipelines()` or `DORMANT` 에러)
- Go 포팅 시 동일 패턴 권장: "모든 엔드포인트 상시 노출, state 게이트는 handler 내부"

### 2.2 동작 흐름

```
rune recall --query "PostgreSQL" --topk 5
  │
  ├── 1. argv 파싱
  │      query = "PostgreSQL", topk = 5
  │
  ├── 2. JSON body 조립
  │      {"query":"PostgreSQL","topk":5,"filters":{}}
  │
  ├── 3. 데몬 연결 확인
  │      소켓 파일 존재? (~/.rune/sock)
  │      ├── 없음 → rune daemon start 시도 → 소켓 대기 (2s 타임아웃)
  │      └── 있음 → 계속
  │
  ├── 4. HTTP POST 전송
  │      POST http://unix:~/.rune/sock/recall
  │      Content-Type: application/json
  │      Body: {"query":"PostgreSQL","topk":5,"filters":{}}
  │
  ├── 5. 응답 수신
  │      HTTP 200: {"ok":true,"found":3,"results":[...]}
  │
  ├── 6. stdout 출력
  │      JSON 그대로 stdout에 출력 (pretty print 옵션 가능)
  │
  └── 7. exit code 결정
         ok == true → exit 0
         ok == false → exit 1
```

### 2.3 에러 처리

```
exit code 매핑:
  0  → 성공 (ok: true)
  1  → 애플리케이션 에러 (ok: false, stdout에 에러 JSON)
  2  → 데몬 연결 실패 (소켓 없음 / 연결 거부)
  3  → CLI 사용법 에러 (잘못된 플래그, 필수 인자 누락)

에러 시 출력:
  stdout: {"ok":false,"error":{"code":"DORMANT","message":"...","retryable":false}}
  stderr: (비어있음 — 모든 정보가 stdout JSON에)
  exit: 1

데몬 연결 실패 시:
  stdout: (비어있음)
  stderr: "error: daemon not running. Run 'rune daemon start'"
  exit: 2
```

### 2.4 데몬 On-demand 기동

```go
func ensureDaemon(sockPath string) error {
    // 1. 소켓 파일 존재 확인
    if _, err := os.Stat(sockPath); err == nil {
        // 2. 실제 연결 가능한지 확인 (파일만 있고 데몬이 죽은 경우 대비)
        conn, err := net.Dial("unix", sockPath)
        if err == nil {
            conn.Close()
            return nil // 데몬 살아있음
        }
        // 소켓 파일은 있지만 연결 불가 → stale 소켓 삭제
        os.Remove(sockPath)
    }

    // 3. 데몬 시작
    cmd := exec.Command(runed_binary_path())
    cmd.Start() // 백그라운드로 fork

    // 4. 소켓 대기 (2s budget)
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) {
        conn, err := net.Dial("unix", sockPath)
        if err == nil {
            conn.Close()
            return nil // 데몬 준비됨
        }
        time.Sleep(100 * time.Millisecond)
    }

    return fmt.Errorf("daemon failed to start within 2s")
}
```

### 2.5 JSON escape 이슈

`--metadata` 플래그에 깊은 JSON을 넘길 때 shell quote 중첩 문제:

```bash
# 이건 됨 (single quote로 감싸기)
rune capture --text-to-embed "text" --metadata '{"title":"test","domain":"arch"}'

# 이건 안 됨 (JSON 안에 single quote가 있으면)
rune capture --metadata '{"title":"it's broken"}'  # shell parse error
```

**해결**: stdin pipe 모드 지원

```bash
# pipe로 JSON 전달 (quote 문제 없음)
echo '{"text_to_embed":"text","metadata":{"title":"it'\''s fine"}}' | rune capture --stdin

# 또는 파일에서
rune capture --stdin < /tmp/capture-request.json
```

---

## 3. rune-mcp 설계 (MCP 유지 시)

### 3.1 동작 개요

```
에이전트 호스트 (Claude Code)
  │
  │ plugin.json의 mcpServers.envector.command → rune-mcp 실행
  │
  ▼
rune-mcp 프로세스 (세션 동안 상주)
  │
  ├── stdin에서 JSON-RPC 2.0 요청 읽기
  │    {"jsonrpc":"2.0","id":1,"method":"tools/call",
  │     "params":{"name":"recall","arguments":{"query":"...","topk":5}}}
  │
  ├── method별 디스패치:
  │    "initialize" → 초기화 응답 (서버 정보, capabilities)
  │    "tools/list" → 8개 tool 스키마 리스트 응답
  │    "tools/call" → tool name으로 HTTP endpoint 결정
  │
  ├── HTTP 요청 조립 + unix socket으로 전송
  │    POST http://unix:~/.rune/sock/recall
  │    Body: {"query":"...","topk":5}
  │
  ├── HTTP 응답 수신
  │    {"ok":true,"found":3,"results":[...]}
  │
  └── JSON-RPC 응답으로 감싸서 stdout에 출력
       {"jsonrpc":"2.0","id":1,"result":{"ok":true,"found":3,...}}
```

### 3.2 Tool 등록 (tools/list 응답)

**중요 (Python 모델 실측 2026-04-17)**: 8개 tool은 dormant/active 상태와 무관하게
**tools/list 응답에 항상 포함**된다. 등록은 생성자 시점에 무조건, 거절은 tool body
내부 runtime state 체크로 처리. rune-mcp shim도 이 모델을 그대로 계승하여, 상태에
따라 schema를 조건부로 바꾸지 않는다. 이렇게 하면:

- 에이전트가 tool discovery를 한 번만 하고 재호출 필요 없음
- state 전이(dormant → active)가 tool list 재협상 없이 즉시 반영
- capture/recall 시도 시점에 최신 state 체크로 자연스럽게 거절


```json
{
  "tools": [
    {
      "name": "capture",
      "description": "FHE 암호화 팀 메모리에 결정을 저장",
      "inputSchema": {
        "type": "object",
        "properties": {
          "text_to_embed": {"type": "string", "description": "임베딩할 텍스트"},
          "metadata": {"type": "object", "description": "저장할 메타데이터 (opaque JSON)"},
          "session_id": {"type": "string"}
        },
        "required": ["text_to_embed", "metadata"]
      }
    },
    {
      "name": "recall",
      "description": "암호화된 팀 메모리에서 시맨틱 검색",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": {"type": "string"},
          "topk": {"type": "integer", "default": 5},
          "domain": {"type": "string"},
          "status": {"type": "string"},
          "since": {"type": "string", "format": "date"}
        },
        "required": ["query"]
      }
    },
    {"name": "vault_status", "description": "Vault 연결 상태", "inputSchema": {"type": "object"}},
    {"name": "diagnostics", "description": "전체 시스템 진단 (vault 연결, 키 로딩, 파이프라인 상태, enVector 도달성)", "inputSchema": {"type": "object"}},
    {"name": "reload", "description": "config 재로딩", "inputSchema": {"type": "object"}},
    {"name": "history", "description": "캡처 히스토리 조회",
      "inputSchema": {"type":"object","properties":{"limit":{"type":"integer"},"domain":{"type":"string"},"since":{"type":"string"}}}},
    {"name": "delete", "description": "캡처 레코드 삭제",
      "inputSchema": {"type":"object","properties":{"record_id":{"type":"string"}},"required":["record_id"]}},
    {"name": "batch_capture", "description": "여러 결정을 한 번에 저장",
      "inputSchema": {"type":"object","properties":{"items":{"type":"array"},"session_id":{"type":"string"}},"required":["items"]}}
  ]
}
```

### 3.3 Tool Name → HTTP Endpoint 매핑

```go
var toolRoutes = map[string]struct {
    method   string
    endpoint string
}{
    "capture":       {"POST", "/capture"},
    "batch_capture": {"POST", "/batch-capture"},
    "recall":        {"POST", "/recall"},
    "vault_status":  {"GET",  "/vault-status"},  // 또는 /diagnostics 서브셋 재사용
    "diagnostics":   {"GET",  "/diagnostics"},
    "reload_pipelines": {"POST", "/reload"},     // Python 도구명은 reload_pipelines
    "capture_history": {"GET",  "/history"},     // Python 도구명은 capture_history
    "delete_capture":  {"DELETE", "/captures/{record_id}"},  // Python 도구명은 delete_capture
}
```

### 3.4 에러 매핑

```go
func httpToMCPError(httpStatus int, body []byte) mcp.ToolResult {
    var resp struct {
        OK    bool      `json:"ok"`
        Error RuneError `json:"error"`
    }
    json.Unmarshal(body, &resp)

    if resp.OK {
        return mcp.ToolResult{Content: body} // 성공
    }

    return mcp.ToolResult{
        IsError: true,
        Content: body, // 에러 JSON 그대로 전달
    }
}
```

### 3.5 Go MCP 라이브러리

`github.com/mark3labs/mcp-go` 사용 시 boilerplate:

```go
package main

import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func main() {
    s := server.NewMCPServer("rune", "0.4.0")

    // Tool 등록
    s.AddTool(mcp.NewTool("recall", ...), recallHandler)
    s.AddTool(mcp.NewTool("capture", ...), captureHandler)
    // ... 8개 tool 전부

    // stdio 서버 시작
    server.ServeStdio(s)
}

func recallHandler(args map[string]interface{}) (*mcp.CallToolResult, error) {
    // 1. args → JSON body
    // 2. HTTP POST unix socket /recall
    // 3. 응답 → CallToolResult
}
```

---

## 4. 두 레이어의 비교

| 차원 | rune CLI | rune-mcp |
|---|---|---|
| 생존 시간 | 호출당 ~20ms 떴다 죽음 | 세션 동안 상주 |
| 호출 오버헤드 | 15-30ms (fork+exec) | < 2ms (이미 실행 중) |
| 메모리 | 0 (상주 프로세스 없음) | ~10 MB per session |
| 에러 표면 | exit code + stdout JSON | MCP 구조화 에러 |
| 디버깅 | `rune recall --query "x"` 직접 실행 | JSON-RPC 파이프 인스펙션 필요 |
| tool discovery | 없음 (md가 유일한 가이드) | tools/list 자동 노출 |
| 에이전트 md 수정 | 필요 (~4-5시간) | 불필요 |
| 구현 복잡도 | 500-800 LoC | 800-1,200 LoC |

---

## 5. 보험 전략: 왜 둘 다 가능한가

runed의 HTTP API가 **실제 계약**이므로:

```
Step 1 (MVP):  rune CLI만 구현 (500-800 LoC)
               에이전트 md를 CLI 호출로 수정

Step 2 (필요 시): rune-mcp 추가 구현 (800-1,200 LoC)
                  plugin.json에 mcpServers 복원
                  에이전트 md를 MCP 호출로 되돌림

Step 3 (궁극):   둘 다 유지
                  사용자가 MCP 또는 CLI를 선택
```

**runed 구현에는 영향 없음.** 어떤 순서로 갈지는 MVP 이후 결정.

---

## 6. plugin.json 변경

### CLI 전용 (MVP)

```json
{
  "name": "rune",
  "version": "0.4.0",
  "commands": "./commands/claude/",
  "agents": [
    "./agents/claude/scribe.md",
    "./agents/claude/retriever.md"
  ]
}
```

### MCP 추가 시

```json
{
  "name": "rune",
  "version": "0.4.0",
  "commands": "./commands/claude/",
  "agents": [
    "./agents/claude/scribe.md",
    "./agents/claude/retriever.md"
  ],
  "mcpServers": {
    "envector": {
      "command": "${CLAUDE_PLUGIN_ROOT}/bin/rune-mcp",
      "env": {
        "RUNE_CONFIG": "${HOME}/.rune/config.json"
      }
    }
  }
}
```
