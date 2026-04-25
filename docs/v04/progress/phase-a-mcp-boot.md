# Phase A — MCP boot

> ✅ 통과 (2026-04-25) · 커밋 `19b7bf6` + `e80d6ea` · 브랜치 `yg/first-mcp-boot`
> 핵심 파일 2개: `cmd/rune-mcp/main.go` (74줄) · `internal/mcp/tools.go` (134줄)

## 1. 한 줄 요약

`rune-mcp` Go 바이너리가 **MCP 프로토콜 표면**(handshake + tools/list + tools/call)만 살아있는 상태. 외부 deps 0, 비즈니스 로직 0, 8개 tool 모두 stub.

**가치** — Claude Code가 이 바이너리를 spawn 하면 8 tool 카탈로그를 정상 인식. 이후 phase는 stub 본체만 채우면 끝. 검증 회로는 매 phase 재사용.

## 2. 동작하는 것 / 안 하는 것

**동작**

- `go build` 한 번에 8.3 MB 정적 바이너리 (Go 1.25+)
- MCP `initialize` handshake — `serverInfo: rune-mcp 0.4.0-alpha`
- `tools/list` — 8 tool 광고 (input/output schema는 Go struct에서 자동 추론)
- `tools/call` — 모두 `isError:true` + "not yet implemented" 응답 (JSON-RPC 자체는 valid)
- 종료: stdin EOF · SIGINT · SIGTERM 모두 exit 0

**안 함 (의도된 한계)**

| 영역 | 가능 시점 |
|---|---|
| 비즈니스 로직 (capture/recall 등) | Phase 5 |
| Vault / envector / embedder adapter | Phase 4 |
| `lifecycle.Manager` 상태 머신 | Phase 4 |
| `config.json` 로딩, `capture_log.jsonl` IO | Phase 4 |
| `request_id` 로깅, `SensitiveFilter` redaction | Phase 4 |

→ Phase A의 정확한 범위는 **"MCP 프로토콜 표면이 정상 동작한다"** 까지.

## 3. 8 tool 카탈로그

```
rune_batch_capture, rune_capture, rune_capture_history, rune_delete_capture,
rune_diagnostics, rune_recall, rune_reload_pipelines, rune_vault_status
```

(SDK가 알파벳순 정렬해 광고 — Python 원본과 bit-identical한 8 이름)

각 tool의 input/output schema는 `internal/domain/*` · `internal/service/*` 의 Go struct에서 SDK가 자동 추론. **Go 타입 = MCP API 계약**, 별도 IDL 없음.

## 4. 검증 — 5분 컷

### 4.1. 빌드 & 헬스 체크

```bash
cd <repo-root>
go build -o bin/rune-mcp ./cmd/rune-mcp
./bin/rune-mcp < /dev/null; echo "exit=$?"   # → exit=0
```

### 4.2. MCP 시퀀스 — `tools/list` 까지

순서: `initialize` → `notifications/initialized` → `tools/list`

```bash
{
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0.0.1"}}}'
  sleep 0.3
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep 0.1
  printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
  sleep 0.5
} | ./bin/rune-mcp 2>/dev/null | jq -r 'select(.id==2) | .result.tools[].name'
```

**기대**: 위 §3의 8 이름.

### 4.3. tool 한 개 호출 (stub 응답)

```bash
{
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0.0.1"}}}'
  sleep 0.3
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep 0.1
  printf '%s\n' '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"rune_diagnostics","arguments":{}}}'
  sleep 0.5
} | ./bin/rune-mcp 2>/dev/null | jq 'select(.id==3).result | {isError, text:.content[0].text}'
```

**기대**: `isError:true`, text는 `"rune_diagnostics is not yet implemented..."`.

> **MCP framing 핵심 3개**
> ① `initialize` → `notifications/initialized` → `tools/*` **순서 필수**
> ② 각 메시지는 `\n` 종결 (LSP의 Content-Length 미사용)
> ③ 마지막 `sleep 0.3~0.5` 없으면 EOF로 응답 끊김

### 4.4. Claude Code 등록 (선택)

`~/.claude/mcp.json`:

```json
{
  "mcpServers": {
    "rune-go-dev": {
      "command": "<repo-root>/bin/rune-mcp"
    }
  }
}
```

> `<repo-root>` 는 본인 체크아웃 경로의 **절대 경로**로 치환 (`~`/상대경로 미지원).

Claude 재시작 후 `/mcp` 에서 `rune-go-dev` 가 connected 표시되면 합격. tool 호출하면 빨간 "not implemented" 응답 — **이게 정상**.

> ⚠️ 기존 Python `envector` MCP가 같은 8 이름 광고. namespace는 `mcp__rune-go-dev__*` vs `mcp__envector__*` 로 분리돼 충돌은 없지만 카탈로그가 중복 보임.

### 4.5. MCP Inspector (시각적, 선택)

```bash
npx -y @modelcontextprotocol/inspector ./bin/rune-mcp
```

브라우저(`localhost:6274`)에 8 tool 리스트 + schema + raw JSON-RPC.

## 5. 8 tool 한 번씩 호출 — `mcp_call` 헬퍼

현재 셸에 paste:

```bash
mcp_call() {
  local tool="$1"; local args="$2"; [ -z "$args" ] && args='{}'
  {
    printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"x","version":"0.0.1"}}}'
    sleep 0.3
    printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
    sleep 0.1
    printf '%s\n' "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"$tool\",\"arguments\":$args}}"
    sleep 0.5
  } | ./bin/rune-mcp 2>/dev/null | jq -c 'select(.id==2)'
}
```

> ⚠️ default value를 `${2:-{}}` 로 쓰면 bash parameter expansion이 깨짐. 위 `[ -z ... ] && args='{}'` 패턴 필수.

```bash
# 인자 없는 4개
mcp_call rune_diagnostics
mcp_call rune_vault_status
mcp_call rune_reload_pipelines
mcp_call rune_capture_history

# 인자 필수 4개
mcp_call rune_recall '{"query":"hello"}'
mcp_call rune_capture '{"text":"hi","source":"test","extracted":{}}'
mcp_call rune_delete_capture '{"record_id":"dec_test"}'
mcp_call rune_batch_capture '{"items":"[]"}'
```

8개 모두 동일한 stub 응답.

## 6. Troubleshooting

| 증상 | 해결 |
|---|---|
| `go build`: `go >= 1.25.0 required` | `go install golang.org/dl/go1.25@latest && go1.25 download` 또는 brew 업그레이드 |
| `go build`: `missing go.sum entry` | `go mod tidy` 후 재빌드 |
| 응답이 안 옴 / 끊김 | 마지막 `sleep` 을 0.5+ 로 |
| Claude Code 에서 tool 미인식 | `cat ~/.claude/mcp.json \| jq .` 로 JSON 검증 + `chmod +x bin/rune-mcp` + 절대 경로 |
| coproc syntax error (macOS bash 3.2) | `brew install bash` 또는 zsh 사용 |

## 7. 다음 마일스톤

**가벼운 후속**

- **Phase A.5 (smoke test)** — `internal/mcp/register_test.go` 에 `mcp.NewInMemoryTransports()` 로 in-memory 서버 띄워 tools/list 8개 회귀 가드. ~50 LoC, CI에서 `AddTool` schema-inference 회귀 자동 감지
- **Phase B** — `rune_diagnostics` environment 섹션 stdlib 응답 (`runtime.GOOS` · `runtime.Version` · `os.Getwd`). 첫 진짜 응답 흐름, 2-3시간 PR

**7-Phase 로드맵 본격 진입** (서로 병렬 가능)

- **Phase 1** — 외부 deps (gRPC · protobuf · envector-go SDK · embedder proto stub)
- **Phase 2** — `internal/domain` + `internal/policy` 순수 로직 (TM scope, 외부 deps 0)
- **Phase 3** — `record_builder` 703 LoC + `payload_text` 364 LoC 라인 단위 포팅
- **Phase 4a/b/c** — Vault / envector / embedder adapter (Phase 1 머지 후)
- **Phase 5** — service 오케스트레이션. `stubHandler` → `service.X.Handle` 교체
- **Phase 7** — 검증 (golden fixture byte-identical · bufconn · Python↔Go shadow run)

→ 의존성 그래프는 `flow-matrix.md §5-d`. Phase 6은 본 문서가 부분 선행이라 Phase 5에 흡수.
