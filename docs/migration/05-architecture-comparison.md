# 아키텍처 비교: MCP + CLI + runed vs CLI + runed

이 문서는 현재 Python MCP 서버의 구조를 분해하고, Go 마이그레이션의 두 가지
아키텍처 안을 상세히 기술한다.

## 1. 현재: Python MCP 서버 (한 덩어리)

현재 `mcp/server/server.py` (~2,000 LoC)는 **프로토콜 껍질**과 **핵심 로직**이
한 파일에 뒤섞여 있다. 에이전트 호스트(Claude Code, Codex, Gemini)가 세션을
열 때마다 이 파일을 통째로 실행하는 독립 Python 프로세스가 스폰된다.

```
현재 server.py (모놀리스)
┌──────────────────────────────────────────────────────────┐
│ ① 프로토콜 껍질                                          │
│    FastMCP 프레임워크 (stdin/stdout JSON-RPC 2.0)        │
│    tool 등록 (@mcp.tool 데코레이터 × 8)                  │
│    ToolAnnotations (readOnlyHint, destructiveHint)       │
│    JSON-RPC 요청 라우팅 → tool 함수 디스패치              │
├──────────────────────────────────────────────────────────┤
│ ② 핵심 로직 (뇌)                                        │
│                                                          │
│  ┌─ config ───────────────────────────────────────────┐  │
│  │ config.json 로딩, active/dormant 상태 머신         │  │
│  │ dormant_reason 코드, env var override              │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ startup ──────────────────────────────────────────┐  │
│  │ fetch_keys_from_vault() → EncKey, EvalKey, DEK     │  │
│  │ enVector 자격증명 추출 (Vault 번들에서)             │  │
│  │ EmbeddingAdapter 초기화 (sbert + Qwen3-0.6B)       │  │
│  │ _init_pipelines() → scribe + retriever 파이프라인   │  │
│  │ _ensure_pipelines() → 120s 타임아웃 게이트          │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ capture ──────────────────────────────────────────┐  │
│  │ agent-delegated: extracted JSON → record_builder   │  │
│  │   → embed → novelty check → AES encrypt metadata   │  │
│  │   → envector.insert → capture_log.jsonl append     │  │
│  │ legacy 3-tier: detector → tier2 → llm_extractor    │  │
│  │   → record_builder → review_queue (← 데드 코드)    │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ recall ───────────────────────────────────────────┐  │
│  │ query_processor → searcher (multi-query)           │  │
│  │   → envector.score → vault.DecryptScores           │  │
│  │   → envector.remind → vault.DecryptMetadata        │  │
│  │   → phase chain expansion → group assembly         │  │
│  │   → metadata filters → recency rerank              │  │
│  │   → synthesizer (LLM 또는 마크다운 폴백)            │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ utility ──────────────────────────────────────────┐  │
│  │ vault_status, diagnostics, reload_pipelines        │  │
│  │ capture_history (JSONL 읽기), delete_capture       │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  ┌─ adapters ─────────────────────────────────────────┐  │
│  │ vault_client.py → gRPC (GetPublicKey, Decrypt*)    │  │
│  │ envector_sdk.py → pyenvector (insert, score, remind)│  │
│  │ embeddings.py → SBERTSDKAdapter (in-process)       │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
```

### 현재 구조의 문제

세션당 이 모놀리스 전체가 복제됨:

```
Claude 창 A  ──stdio──→  [ server.py 프로세스 A ]  ──→  Vault / enVector
                          (모델 300-800MB, 키, gRPC 채널)

Claude 창 B  ──stdio──→  [ server.py 프로세스 B ]  ──→  Vault / enVector
                          (모델 300-800MB, 키, gRPC 채널)  ← 전부 중복

Codex        ──stdio──→  [ server.py 프로세스 C ]  ──→  Vault / enVector
                          (모델 300-800MB, 키, gRPC 채널)  ← 또 중복
```

3 세션 = 모델 3번 로드, gRPC 채널 6개, RSS ~1-2 GB.

## 2. 분해: 뭐가 어디로 가는가

Go 마이그레이션은 이 모놀리스를 쪼개는 것이다. **두 안 모두** `runed` 데몬은
동일하고, 차이는 ① 프로토콜 껍질을 어떻게 처리하느냐 뿐이다.

| 기능 | runed (데몬) | 껍질 (MCP 또는 CLI) | 드롭 |
|---|:---:|:---:|:---:|
| config 로딩 + 상태 머신 | **●** | | |
| Vault 키 페칭 (EncKey, EvalKey, DEK) | **●** | | |
| 임베딩 모델 로딩 + embed() | **●** | | |
| capture: embed → encrypt → insert → log | **●** | | |
| recall: query → search → decrypt → rank | **●** | | |
| vault_status / diagnostics | **●** | | |
| capture_history (JSONL) | **●** | | |
| delete_capture (soft delete) | **●** | | |
| reload (config 재로딩) | **●** | | |
| envector SDK (insert/score/remind) | **●** | | |
| vault gRPC (3 RPC) | **●** | | |
| gRPC 채널 풀 + keepalive | **●** | | |
| fsnotify config watch | **●** | | |
| MCP stdio JSON-RPC 프레이밍 | | **●** | |
| tool 등록 + 스키마 노출 | | **●** | |
| 또는: CLI argv 파싱 + stdout JSON | | **●** | |
| legacy 3-tier (detector/tier2/extractor) | | | **●** |
| record_builder (agent-delegated 경로에서 사용) | **●** | | |
| novelty check (0.95 near-duplicate 블록; 런타임 기본값) | **●** | | |
| synthesizer | | | **●** |
| pattern_cache / review_queue | | | **●** |
| OpenAI/Google LLM 클라이언트 | | | **●** |
| provider auto-detect | | | **●** |
| Slack/Notion 웹훅 서버 | | | **●** |

**핵심**: `runed`는 두 안에서 100% 동일하다. 차이는 오직 "껍질" 열.

---

## 3. 안 A: MCP + CLI + runed (3 바이너리)

MCP 프로토콜을 유지한다. 에이전트 호스트의 기존 MCP 인프라를 그대로 활용하고,
에이전트 마크다운(`agents/claude/scribe.md` 등)은 수정하지 않는다.

### 3.1 컴포넌트

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Go 바이너리 3개                              │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ① rune-mcp  (per-session, 에이전트가 스폰)                         │
│     ─────────────────────────────────────────                        │
│     역할: MCP stdio JSON-RPC ↔ HTTP 변환기                           │
│     코드량: 800-1,200 LoC                                            │
│     수명: 에이전트 세션 동안 상주                                     │
│     무게: ~10 MB RSS (모델 없음, gRPC 없음, 키 없음)                 │
│     라이브러리: mark3labs/mcp-go                                     │
│     하는 일:                                                         │
│       - stdin에서 JSON-RPC 요청 읽기                                 │
│       - tool 이름 → HTTP endpoint 매핑                               │
│       - unix socket으로 runed에 HTTP POST                            │
│       - 응답을 MCP JSON-RPC로 감싸서 stdout 출력                     │
│       - runed가 안 떠 있으면 on-demand 기동                          │
│       - tools/list 요청 시 8개 tool 스키마 응답                      │
│                                                                      │
│  ② rune  (CLI, per-call 또는 사용자 직접 실행)                       │
│     ─────────────────────────────────────────                        │
│     역할: 사용자/스크립트용 CLI 인터페이스                            │
│     코드량: 500-800 LoC                                              │
│     수명: 호출 시 떴다가 결과 받으면 즉시 종료                       │
│     하는 일:                                                         │
│       - rune configure / activate / deactivate / reset / status      │
│       - rune daemon start|stop|restart|logs|health                   │
│       - rune recall / capture / history / delete (데몬 HTTP 호출)    │
│       - 디버깅 용도 (사용자가 터미널에서 직접 실행)                  │
│                                                                      │
│  ③ runed  (데몬, 항상 상주)                                          │
│     ─────────────────────────────────────────                        │
│     역할: 모든 핵심 로직                                              │
│     코드량: 추정 4,000-6,000 LoC                                     │
│     수명: 시스템 부팅 ~ 시스템 종료 (launchd/systemd)                │
│     무게: ~300-800 MB RSS (임베딩 모델이 지배적)                     │
│     하는 일: §4 참고                                                  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 3.2 전체 흐름도

```
 ┌────────────┐  ┌────────────┐  ┌────────────┐
 │ Claude 창A │  │ Claude 창B │  │   Codex    │   에이전트 호스트
 │ scribe.md  │  │ retriever  │  │ scribe.md  │   (에이전트 md는
 │ retriever  │  │    .md     │  │            │    수정 불필요)
 │    .md     │  │            │  │            │
 └─────┬──────┘  └─────┬──────┘  └─────┬──────┘
       │               │               │
       │ MCP tool call │ MCP tool call │ MCP tool call
       │ (stdio)       │ (stdio)       │ (stdio)
       ▼               ▼               ▼
 ┌───────────┐  ┌───────────┐  ┌───────────┐
 │ rune-mcp  │  │ rune-mcp  │  │ rune-mcp  │   ① 얇은 MCP shim
 │ ~10MB RSS │  │ ~10MB RSS │  │ ~10MB RSS │   세션당 1개 상주
 │ (Go)      │  │ (Go)      │  │ (Go)      │   MCP JSON-RPC ↔ HTTP
 └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
       │               │               │
       │ HTTP POST     │ HTTP POST     │ HTTP POST
       │ (unix socket: ~/.rune/sock)   │
       └───────────┬───┴───────────────┘
                   ▼
 ┌──────────────────────────────────────────────────────────┐
 │                    runed  (Go 데몬, 1개)                  │
 │                                                          │
 │  HTTP API (unix socket only, 0600)                       │
 │  ┌─────────────────────────────────────────────────────┐ │
 │  │ POST /capture    POST /batch-capture                │ │
 │  │ POST /recall     POST /reload                       │ │
 │  │ GET  /health     GET  /diagnostics                  │ │
 │  │ GET  /history    DELETE /captures/:id               │ │
 │  └──────────────────────────┬──────────────────────────┘ │
 │                             │                            │
 │  ┌──────────────────────────▼──────────────────────────┐ │
 │  │              핵심 로직 (§4 참고)                     │ │
 │  │  config + 상태 | embedding | retriever | capture    │ │
 │  └───────┬───────────────────────────────────┬─────────┘ │
 │          │                                   │           │
 │  ┌───────▼─────────┐              ┌──────────▼────────┐  │
 │  │ vault-go (gRPC) │              │ envector-go (CGO) │  │
 │  │ GetPublicKey    │              │ score / remind    │  │
 │  │ DecryptScores   │              │ insert (FHE enc)  │  │
 │  │ DecryptMetadata │              │ CipherBlock proto │  │
 │  └───────┬─────────┘              └──────────┬────────┘  │
 └──────────┼────────────────────────────────────┼──────────┘
            │                                    │
            ▼                                    ▼
    Rune-Vault (gRPC)                    enVector Cloud (gRPC)
```

### 3.3 호출 경로 예시: recall

```
1. 사용자가 Claude Code에서 "왜 PostgreSQL로 결정했지?" 라고 물음

2. Claude의 retriever.md가 활성화됨
   → "이건 decision-rationale 쿼리다. recall을 호출해야 함"
   → 에이전트가 MCP tool call 생성:
     mcp__plugin_rune_envector__recall(query="PostgreSQL 결정 이유", topk=5)

3. Claude Code 호스트가 rune-mcp의 stdin에 JSON-RPC 씀:
   {"jsonrpc":"2.0","id":42,"method":"tools/call",
    "params":{"name":"recall","arguments":{"query":"PostgreSQL 결정 이유","topk":5}}}

4. rune-mcp가 읽어서 HTTP로 변환:
   POST http://unix:~/.rune/sock/recall
   Body: {"query":"PostgreSQL 결정 이유","topk":5}

5. runed가 처리:
   → query_processor: 의도 분류, 엔티티 추출, 쿼리 확장
   → embedding: "PostgreSQL 결정 이유" → [0.12, -0.34, ...] (1024-dim)
   → envector.score(vec) → FHE ciphertext
   → vault.DecryptScores(ct) → top-5 {shard, row, score}
   → envector.remind([rows]) → encrypted metadata
   → vault.DecryptMetadata(ct) → plaintext DecisionRecord JSON
   → recency rerank → 필터 → 결과 조립

6. runed → rune-mcp (HTTP 응답):
   {"ok":true,"found":3,"results":[...],"confidence":0.87}

7. rune-mcp → Claude Code (stdout JSON-RPC):
   {"jsonrpc":"2.0","id":42,"result":{"ok":true,"found":3,...}}

8. Claude가 결과를 읽고 사용자에게 자연어 응답을 합성
```

### 3.4 plugin.json (안 A)

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

---

## 4. 안 B: CLI + runed (2 바이너리, RFC 제안)

MCP 프로토콜을 제거한다. 에이전트 마크다운이 `rune` CLI를 Bash로 직접
호출하도록 재작성한다.

### 4.1 컴포넌트

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Go 바이너리 2개                              │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ① rune  (CLI, per-call 에피메랄)                                    │
│     ─────────────────────────────────────────                        │
│     역할: 사용자 + 에이전트 공용 CLI                                  │
│     코드량: 500-800 LoC                                              │
│     수명: 호출 → 결과 → 즉시 종료 (수십 ms)                          │
│     무게: 프로세스 존재 시간이 짧아 RSS 무의미                       │
│     하는 일:                                                         │
│       - argv 파싱 → JSON body 조립                                   │
│       - unix socket으로 runed에 HTTP POST                            │
│       - 응답 JSON을 stdout 출력                                      │
│       - exit code 매핑 (ok=true → 0, ok=false → 1)                  │
│       - runed가 안 떠 있으면 on-demand 기동                          │
│       - 서브커맨드: recall, capture, batch-capture, status,          │
│         history, delete, reload, daemon start|stop|restart           │
│       - configure / activate / deactivate / reset (config 파일 I/O) │
│                                                                      │
│  ② runed  (데몬, 항상 상주)                                          │
│     ─────────────────────────────────────────                        │
│     역할: 안 A와 100% 동일                                           │
│     코드량: 추정 4,000-6,000 LoC                                     │
│     수명: 시스템 부팅 ~ 시스템 종료 (launchd/systemd)                │
│     무게: ~300-800 MB RSS                                            │
│     하는 일: §5 참고                                                  │
│                                                                      │
│  (rune-mcp는 없음)                                                   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 4.2 전체 흐름도

```
 ┌────────────┐  ┌────────────┐  ┌────────────┐
 │ Claude 창A │  │ Claude 창B │  │   Codex    │   에이전트 호스트
 │ scribe.md  │  │ retriever  │  │ scribe.md  │   (md를 CLI 호출로
 │ retriever  │  │    .md     │  │            │    재작성함)
 │    .md     │  │            │  │            │
 └─────┬──────┘  └─────┬──────┘  └─────┬──────┘
       │               │               │
       │ Bash tool     │ Bash tool     │ Bash tool
       │ exec          │ exec          │ exec
       ▼               ▼               ▼
 ┌───────────┐  ┌───────────┐  ┌───────────┐
 │ rune CLI  │  │ rune CLI  │  │ rune CLI  │   ① 에피메랄 CLI
 │ (Go)      │  │ (Go)      │  │ (Go)      │   호출할 때만 존재
 │ 떴다 죽음 │  │ 떴다 죽음 │  │ 떴다 죽음 │   상주 프로세스 0개
 └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
       │               │               │
       │ HTTP POST     │ HTTP POST     │ HTTP POST
       │ (unix socket: ~/.rune/sock)   │
       └───────────┬───┴───────────────┘
                   ▼
 ┌──────────────────────────────────────────────────────────┐
 │                    runed  (Go 데몬, 1개)                  │
 │                                                          │
 │               (안 A의 runed와 100% 동일)                  │
 │                                                          │
 │  HTTP API (unix socket only, 0600)                       │
 │  ┌─────────────────────────────────────────────────────┐ │
 │  │ POST /capture    POST /batch-capture                │ │
 │  │ POST /recall     POST /reload                       │ │
 │  │ GET  /health     GET  /diagnostics                  │ │
 │  │ GET  /history    DELETE /captures/:id               │ │
 │  └──────────────────────────┬──────────────────────────┘ │
 │                             │                            │
 │  ┌──────────────────────────▼──────────────────────────┐ │
 │  │              핵심 로직 (§5 참고)                     │ │
 │  │  config + 상태 | embedding | retriever | capture    │ │
 │  └───────┬───────────────────────────────────┬─────────┘ │
 │          │                                   │           │
 │  ┌───────▼─────────┐              ┌──────────▼────────┐  │
 │  │ vault-go (gRPC) │              │ envector-go (CGO) │  │
 │  └───────┬─────────┘              └──────────┬────────┘  │
 └──────────┼────────────────────────────────────┼──────────┘
            ▼                                    ▼
    Rune-Vault (gRPC)                    enVector Cloud (gRPC)
```

### 4.3 호출 경로 예시: recall

```
1. 사용자가 Claude Code에서 "왜 PostgreSQL로 결정했지?" 라고 물음

2. Claude의 retriever.md가 활성화됨 (재작성된 버전)
   → "이건 decision-rationale 쿼리다. rune recall을 실행해야 함"
   → 에이전트가 Bash tool을 통해 CLI 실행:

     rune recall --query "PostgreSQL 결정 이유" --topk 5

3. OS가 rune Go 바이너리를 fork+exec (~15ms)
   rune 바이너리가:
   → argv 파싱: query="PostgreSQL 결정 이유", topk=5
   → JSON body 조립: {"query":"PostgreSQL 결정 이유","topk":5}
   → unix socket (~/.rune/sock) 열기
   → HTTP POST /recall 전송

4. runed가 처리 (안 A와 완전히 동일):
   → query_processor → embedding → envector.score
   → vault.DecryptScores → envector.remind
   → vault.DecryptMetadata → rerank → 결과 조립

5. runed → rune CLI (HTTP 응답):
   {"ok":true,"found":3,"results":[...],"confidence":0.87}

6. rune CLI가 stdout에 JSON 출력:
   {"ok":true,"found":3,"results":[...],"confidence":0.87}
   exit code 0

7. Claude Code의 Bash tool이 stdout을 캡처해서 에이전트에게 전달

8. Claude가 결과를 읽고 사용자에게 자연어 응답을 합성
```

### 4.4 에이전트 md 변경 예시 (capture)

**변경 전** (현재, MCP):
```markdown
## Step 3: Call the MCP Tool

mcp__plugin_rune_envector__capture(
    text="<the original significant text>",
    source="claude_agent",
    user="<user if known>",
    channel="<context if known>",
    extracted='<JSON string from Step 2>'
)

**Important**: The `extracted` parameter is a JSON **string**, not a JSON object.

## Handling Results

- `captured: true` — Report briefly: "Captured: [summary] (ID: [record_id])"
- `captured: false` — The message was filtered out. Do not retry.
- `ok: false` — An error occurred. Report the error briefly.
```

**변경 후** (안 B, CLI):
```markdown
## Step 3: Call the CLI

rune capture \
  --text "<the original significant text>" \
  --source "claude_agent" \
  --user "<user if known>" \
  --channel "<context if known>" \
  --extracted '<JSON string from Step 2>'

The CLI returns JSON on stdout.

## Handling Results

- stdout의 `"ok": true` + `"captured": true` → "Captured: [summary] (ID: [record_id])"
- stdout의 `"captured": false` → The message was filtered out. Do not retry.
- exit code ≠ 0 또는 `"ok": false` → Report the error message.
```

**변경 범위**: Step 3 (호출부) + 에러 처리 섹션 — 각 md 파일의 ~10%.
Step 1 (정책 판단) + Step 2 (추출 스키마) — **수정 없음** (transport-independent).

### 4.5 plugin.json (안 B)

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

`mcpServers` 블록이 사라진다. 에이전트 md가 CLI를 직접 호출하므로 MCP
서버 선언이 불필요.

---

## 5. runed 내부 구조 (두 안 공통)

`runed`는 **안 A든 안 B든 동일한 Go 바이너리**다. 아래는 내부 구조.

```
runed 프로세스 내부
┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  ┌─ HTTP Server ──────────────────────────────────────────────┐  │
│  │ net.Listen("unix", "~/.rune/sock")                         │  │
│  │ chmod 0600 + uid 검증                                      │  │
│  │                                                            │  │
│  │ POST /capture        → captureHandler                      │  │
│  │ POST /batch-capture  → batchCaptureHandler                 │  │
│  │ POST /recall         → recallHandler                       │  │
│  │ POST /reload         → reloadHandler                       │  │
│  │ GET  /health         → healthHandler                       │  │
│  │ GET  /diagnostics    → diagnosticsHandler                  │  │
│  │ GET  /history        → historyHandler                      │  │
│  │ DELETE /captures/:id → deleteHandler                       │  │
│  └────────────────────────────┬───────────────────────────────┘  │
│                               │                                  │
│  ┌────────────────────────────▼───────────────────────────────┐  │
│  │                    Config Manager                          │  │
│  │  ~/.rune/config.json 로딩                                  │  │
│  │  fsnotify watch (파일 변경 자동 감지)                      │  │
│  │  SIGHUP 수신 시 reload                                     │  │
│  │  active/dormant 상태 머신                                  │  │
│  │  dormant_reason 코드 관리                                  │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                  Embedding Engine                          │  │
│  │  모델: Qwen/Qwen3-Embedding-0.6B (sbert 모드)             │  │
│  │  프로세스 시작 시 1회 로드 → 메모리 상주                   │  │
│  │  embed(text) → []float32                                   │  │
│  │  (ONNX Runtime 또는 Python 서브프로세스 — open question)   │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                  Capture Gateway                           │  │
│  │  1. embed(text_to_embed) → vec                             │  │
│  │  2. envector.EncryptVector(vec, EncKey) → CipherBlock      │  │
│  │  3. AES encrypt metadata (per-agent DEK)                   │  │
│  │  4. envector.Insert(index, cipher, encrypted_metadata)     │  │
│  │  5. capture_log.jsonl append                               │  │
│  │                                                            │  │
│  │  novelty check (≥0.95 near-duplicate 블록; 런타임 기본값)  │  │
│  │  ※ embedding.py 상수(0.93)와 server.py 기본값(0.95) 불일치 │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                  Retriever Pipeline                        │  │
│  │                                                            │  │
│  │  query_processor.go                                        │  │
│  │    → 의도 분류 (regex, 영어)                               │  │
│  │    → 엔티티 + 키워드 추출                                  │  │
│  │    → 시간 범위 추출                                        │  │
│  │    → 쿼리 확장 (expanded queries)                          │  │
│  │                                                            │  │
│  │  searcher.go                                               │  │
│  │    → 멀티 쿼리 벡터 검색                                   │  │
│  │    → envector.score() → FHE ciphertext                     │  │
│  │    → vault.DecryptScores() → top-k                         │  │
│  │    → envector.remind() → encrypted metadata                │  │
│  │    → vault.DecryptMetadata() → plaintext records           │  │
│  │    → dedup by record_id                                    │  │
│  │                                                            │  │
│  │  reranker.go                                               │  │
│  │    → metadata 필터 (domain / status / since)               │  │
│  │    → 시간 범위 필터                                        │  │
│  │    → recency 가중 (90일 half-life)                         │  │
│  │    → status 승수 (accepted > proposed > superseded > reverted) │  │
│  │    → 최종 정렬                                             │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                  vault-go                                  │  │
│  │  gRPC 클라이언트 (단일 채널 풀, keepalive)                 │  │
│  │  GetPublicKey  → EncKey.json + EvalKey.json + 번들         │  │
│  │  DecryptScores → FHE ciphertext → plaintext top-k          │  │
│  │  DecryptMetadata → AES ciphertext → plaintext JSON         │  │
│  │  TLS 3모드: system CA / custom CA / tls_disable            │  │
│  │  키 캐시: ~/.rune/keys/ (디스크) + in-memory               │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                  envector-go                               │  │
│  │  gRPC 스텁 (score, remind, insert, get_index_list)         │  │
│  │  CipherBlock proto (opaque 바이트)                         │  │
│  │  EncKey 기반 벡터 암호화 (CGO → envector C++ 코어)         │  │
│  │  쿼리 벡터는 평문 전송 (query_encryption=False)            │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                  Utility                                   │  │
│  │  capture_log.go — JSONL append + 역시간순 읽기             │  │
│  │  errors.go — RuneError (code, retryable, recovery_hint)    │  │
│  │  daemon.go — signal 처리, graceful shutdown, sleep/wake    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 5.1 runed HTTP API 상세

모든 엔드포인트는 unix socket에서만 접근 가능 (localhost TCP 아님).

```
POST /capture
  요청: {"text_to_embed": str, "metadata": object, "session_id"?: str}
  응답: {"ok": bool, "record_id": str, "error"?: {...}}

POST /batch-capture
  요청: {"items": [{"text_to_embed": str, "metadata": object}, ...]}
  응답: {"ok": bool, "results": [{"ok": bool, "record_id": str, "error"?: str}, ...]}

POST /recall
  요청: {"query": str, "topk": int, "filters": {"domain"?: str, "status"?: str, "since"?: str}}
  응답: {"ok": bool, "found": int, "results": [...], "confidence": float, "warnings": [...]}

POST /reload
  요청: (빈 body)
  응답: {"ok": bool, "state": "active"|"dormant", "dormant_reason"?: str}

GET /health
  응답: {"ok": bool, "uptime_seconds": int, "state": "active"|"dormant"}

GET /diagnostics
  응답: {"vault": {...}, "envector": {...}, "embedding": {...}, "config": {...}}

GET /history?limit=N&domain=X&since=ISO
  응답: {"ok": bool, "entries": [...]}

DELETE /captures/:id
  응답: {"ok": bool}
```

이 API는 **안 A의 rune-mcp든, 안 B의 rune CLI든 동일하게 호출**한다.

---

## 6. 나란히 비교

```
┌─ 안 A: MCP + CLI + runed ─────────────────────────────────────────────┐
│                                                                        │
│  Claude ──MCP stdio──→ rune-mcp ──HTTP──┐                             │
│  Codex  ──MCP stdio──→ rune-mcp ──HTTP──┤──→ runed (데몬) → Vault    │
│  Gemini ──MCP stdio──→ rune-mcp ──HTTP──┘              ↘ enVector    │
│                                                                        │
│  + 사용자 터미널 ──→ rune CLI ──HTTP──→ runed (같은 데몬)             │
│                                                                        │
│  바이너리: 3개 (rune-mcp, rune, runed)                                │
│  세션당 상주 프로세스: 1개 (rune-mcp, ~10MB)                          │
│  에이전트 md 수정: 없음                                                │
│  총 thin-layer 코드: ~1,800 LoC (shim 1,000 + CLI 800)               │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘

┌─ 안 B: CLI + runed (RFC) ─────────────────────────────────────────────┐
│                                                                        │
│  Claude ──Bash exec──→ rune CLI ──HTTP──┐                             │
│  Codex  ──Bash exec──→ rune CLI ──HTTP──┤──→ runed (데몬) → Vault    │
│  Gemini ──Bash exec──→ rune CLI ──HTTP──┘              ↘ enVector    │
│                                                                        │
│  + 사용자 터미널 ──→ rune CLI ──HTTP──→ runed (같은 데몬)             │
│                                                                        │
│  바이너리: 2개 (rune, runed)                                          │
│  세션당 상주 프로세스: 0개                                             │
│  에이전트 md 수정: ~4-5시간 (Step 3 호출부만, 전체의 ~10%)            │
│  총 thin-layer 코드: ~700 LoC (CLI만)                                 │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘

┌─ 공통 (두 안 모두) ───────────────────────────────────────────────────┐
│                                                                        │
│  runed 코드: 4,000-6,000 LoC (동일)                                   │
│  runed RSS: ~300-800 MB (동일)                                        │
│  Vault gRPC: 동일                                                      │
│  enVector CGO: 동일                                                    │
│  임베딩 모델: 동일                                                     │
│  HTTP API: 동일                                                        │
│  config 관리: 동일                                                     │
│  daemon lifecycle (launchd/systemd): 동일                             │
│                                                                        │
│  ⇒ 전체 Go 코드의 ~85-90%가 공통                                      │
│  ⇒ 안 선택은 ~10-15%의 얇은 껍질에만 영향                             │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 7. 보험 전략: CLI 우선, MCP 문 열어두기

두 안이 runed를 공유하므로, 아래 전략이 가능하다:

```
Phase 1:  runed 구현 (두 안 공통, 작업의 85-90%)
Phase 2:  rune CLI 구현 (안 B, 500-800 LoC)
          에이전트 md 수정 (4-5시간)
          → 안 B로 배포, 동작 확인

(필요 시)
Phase 3:  rune-mcp 추가 구현 (800-1,200 LoC)
          → plugin.json에 mcpServers 블록 복원
          → 에이전트 md를 MCP 호출로 되돌림 (또는 두 버전 공존)
```

runed의 HTTP API를 MCP tool 스키마와 1:1로 설계해두면, Phase 3의 MCP
shim은 순수한 변환기이므로 언제든 추가할 수 있다. **지금 안 만들어도 문을
닫는 게 아니다.**
