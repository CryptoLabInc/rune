# 아키텍처 전환 개요: Python MCP → Go runed

이 문서는 현재 Python MCP 서버 아키텍처에서 Go `runed` 데몬 아키텍처로의
전환을 다룬다. 구현자가 전체 그림을 잡기 위한 레퍼런스.

---

## 1. 현재 아키텍처 (Python MCP)

에이전트 호스트(Claude Code, Codex, Gemini CLI)가 세션을 열 때마다
`scripts/bootstrap-mcp.sh`를 실행해 **독립된 Python 프로세스**를 스폰한다.
각 프로세스는 venv를 생성하고, pip으로 ~30개 패키지를 설치하고,
`sentence-transformers` 모델을 로딩하고, Vault + enVector로 gRPC 채널을 연다.

세션이 3개면 이 전체가 3번 반복된다.

```
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│    Claude Code A    │  │    Claude Code B    │  │       Codex         │
│   (에이전트 세션)    │  │   (에이전트 세션)    │  │   (에이전트 세션)    │
└─────────┬───────────┘  └─────────┬───────────┘  └─────────┬───────────┘
          │ stdio                  │ stdio                  │ stdio
          │ (JSON-RPC 2.0)        │ (JSON-RPC 2.0)        │ (JSON-RPC 2.0)
          ▼                        ▼                        ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│  Python 프로세스 A  │  │  Python 프로세스 B  │  │  Python 프로세스 C  │
│                     │  │                     │  │                     │
│ ┌─────────────────┐ │  │ ┌─────────────────┐ │  │ ┌─────────────────┐ │
│ │ FastMCP 프레임  │ │  │ │ FastMCP 프레임  │ │  │ │ FastMCP 프레임  │ │
│ │  워크 (stdio)   │ │  │ │  워크 (stdio)   │ │  │ │  워크 (stdio)   │ │
│ ├─────────────────┤ │  │ ├─────────────────┤ │  │ ├─────────────────┤ │
│ │ 임베딩 모델     │ │  │ │ 임베딩 모델     │ │  │ │ 임베딩 모델     │ │
│ │ (300-800MB RSS) │ │  │ │ (300-800MB RSS) │ │  │ │ (300-800MB RSS) │ │
│ ├─────────────────┤ │  │ ├─────────────────┤ │  │ ├─────────────────┤ │
│ │ FHE 키          │ │  │ │ FHE 키          │ │  │ │ FHE 키          │ │
│ │ (EncKey+EvalKey)│ │  │ │ (EncKey+EvalKey)│ │  │ │ (EncKey+EvalKey)│ │
│ ├─────────────────┤ │  │ ├─────────────────┤ │  │ ├─────────────────┤ │
│ │ gRPC 채널 ×2   │ │  │ │ gRPC 채널 ×2   │ │  │ │ gRPC 채널 ×2   │ │
│ │ (Vault+enVector)│ │  │ │ (Vault+enVector)│ │  │ │ (Vault+enVector)│ │
│ └─────────────────┘ │  │ └─────────────────┘ │  │ └─────────────────┘ │
└─────────┬───────────┘  └─────────┬───────────┘  └─────────┬───────────┘
          │                        │                        │
          │ gRPC                   │ gRPC                   │ gRPC
          │ (각각 독립 채널)        │ (각각 독립 채널)        │ (각각 독립 채널)
          ▼                        ▼                        ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          외부 서비스                                     │
│                                                                         │
│   Rune-Vault (gRPC)                    enVector Cloud (gRPC)            │
│   - GetPublicKey                       - score (FHE cosine sim)         │
│   - DecryptScores                      - remind (메타데이터 조회)       │
│   - DecryptMetadata                    - insert (FHE 벡터 삽입)         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 리소스 테이블

| 지표 | 1세션 | 3세션 |
|---|---|---|
| 프로세스 수 | 1 | 3 |
| 임베딩 모델 RSS | 300-800 MB | 900-2,400 MB |
| Vault gRPC 채널 | 1 | 3 |
| enVector gRPC 채널 | 1 | 3 |
| 모델 cold start | 수 초 | 수 초 x 3 |
| FHE 키 메모리 | 수십 MB | 수십 MB x 3 |
| config 로딩 | 매 세션 | 매 세션 x 3 |

### 문제점

1. **리소스 낭비**: 임베딩 모델은 read-only인데 프로세스마다 중복 로드
2. **cold start 반복**: 매 세션마다 venv 확인 → pip install → 모델 로드 → Vault 키 fetch
3. **gRPC 채널 증식**: HTTP/2 multiplexing을 활용하지 못하고 채널이 세션 수만큼 증가
4. **bootstrap 복잡도**: `bootstrap-mcp.sh`에 3단계 self-healing 로직
   (venv 오염 감지, pip shebang 재작성, fastembed 캐시 정리)
5. **Python 의존성**: torch가 transitive dependency로 따라옴, 빌드 환경에 따라 수백 MB

---

## 2. 새 아키텍처 (Go runed 데몬)

단일 `runed` Go 데몬이 지속적으로 실행된다 (launchd 또는 systemd user unit).
임베딩 모델, gRPC 채널 풀, FHE 키를 메모리에 보유하고,
유닉스 소켓(`~/.rune/sock`, 퍼미션 `0600`)으로 요청을 받는다.

에이전트 세션은 **thin client**를 통해 데몬에 접근한다.
thin client는 두 종류:
- **`rune` CLI**: 호출마다 떴다가 바로 종료 (ephemeral). Bash exec으로 실행.
- **`rune-mcp`**: 세션 동안 상주하는 MCP stdio 어댑터. ~10 MB RSS.

둘 다 유닉스 소켓을 통해 HTTP 요청을 데몬에 전달할 뿐이다.
모델, 키, gRPC 채널 등 무거운 자원은 일절 갖지 않는다.

```
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│    Claude Code A    │  │    Claude Code B    │  │       Codex         │
│   (에이전트 세션)    │  │   (에이전트 세션)    │  │   (에이전트 세션)    │
└─────────┬───────────┘  └─────────┬───────────┘  └─────────┬───────────┘
          │                        │                        │
          │ MCP stdio 또는         │ MCP stdio              │ Bash exec
          │ Bash exec              │                        │ (rune recall ...)
          ▼                        ▼                        ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐
│    thin client      │  │    thin client      │  │    thin client      │
│   rune-mcp (~10MB)  │  │   rune-mcp (~10MB)  │  │   rune CLI          │
│   또는 rune CLI     │  │                     │  │   (ephemeral)       │
│                     │  │                     │  │                     │
│ JSON-RPC ↔ HTTP     │  │ JSON-RPC ↔ HTTP     │  │ argv → HTTP POST   │
│ 변환만 수행         │  │ 변환만 수행         │  │ 결과 출력 후 종료   │
└─────────┬───────────┘  └─────────┬───────────┘  └─────────┬───────────┘
          │                        │                        │
          │ HTTP POST              │ HTTP POST              │ HTTP POST
          │ (unix socket)          │ (unix socket)          │ (unix socket)
          │                        │                        │
          └────────────┬───────────┴────────────────────────┘
                       │
                       ▼
          ~/.rune/sock (unix domain socket, 0600)
                       │
                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        runed  (Go 데몬, 1개)                        │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │ HTTP Router (net/http, unix socket listener)                  │  │
│  │                                                                │  │
│  │ POST /capture    POST /batch-capture    POST /recall          │  │
│  │ POST /reload     GET  /health           GET  /diagnostics     │  │
│  │ GET  /history    DELETE /captures/:id   GET  /vault-status    │  │
│  └──────────────────────────┬─────────────────────────────────────┘  │
│                             │                                        │
│  ┌──────────────────────────▼─────────────────────────────────────┐  │
│  │                       핵심 로직                                 │  │
│  │                                                                │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐ │  │
│  │  │ config 관리  │  │ 임베딩 모델  │  │ FHE 키 캐시          │ │  │
│  │  │ (fsnotify)   │  │ (read-only)  │  │ (EncKey + EvalKey)   │ │  │
│  │  │ RWMutex 보호 │  │ 300-800MB    │  │ 수십 MB              │ │  │
│  │  └──────────────┘  └──────────────┘  └──────────────────────┘ │  │
│  │                                                                │  │
│  │  ┌─────────────────────────────────────────────────────────┐  │  │
│  │  │ capture: embed → FHE encrypt → AES encrypt → insert    │  │  │
│  │  │ recall:  embed → score → decrypt → filter → rerank     │  │  │
│  │  │ utility: history, delete, diagnostics, vault_status     │  │  │
│  │  └─────────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌─────────────────────┐          ┌──────────────────────────────┐   │
│  │ vault-go (gRPC)     │          │ envector-go (Go SDK)         │   │
│  │ 채널 풀 (1~2 conn)  │          │ 채널 풀 (1~2 conn)          │   │
│  │ HTTP/2 multiplexing │          │ HTTP/2 multiplexing          │   │
│  └─────────┬───────────┘          └──────────────┬───────────────┘   │
└────────────┼─────────────────────────────────────┼───────────────────┘
             │                                     │
             │ gRPC/TLS                            │ gRPC/TLS
             ▼                                     ▼
     Rune-Vault (gRPC)                     enVector Cloud (gRPC)
```

### 리소스 테이블

| 지표 | 1세션 | 3세션 |
|---|---|---|
| 데몬 프로세스 | 1 | 1 |
| thin client | 1 | 3 (CLI: ephemeral / MCP: ~10 MB each) |
| 임베딩 모델 RSS | 300-800 MB | 300-800 MB (공유) |
| Vault gRPC 채널 | 1 | 1 (HTTP/2 multiplexing) |
| enVector gRPC 채널 | 1 | 1 |
| 모델 cold start | 데몬 수명 동안 1회 | 데몬 수명 동안 1회 |
| FHE 키 메모리 | 수십 MB (1벌) | 수십 MB (1벌, 공유) |
| config 로딩 | 1회 + fsnotify 갱신 | 1회 + fsnotify 갱신 |

### 비교 요약

```
              현재 (Python MCP)              새 (Go runed)
              ────────────────               ──────────────
3세션 RSS     900-2,400 MB                   330-930 MB
              (모델 3벌 + Python 오버헤드)    (모델 1벌 + thin client 30MB)

gRPC 채널     6개 (Vault 3 + enVector 3)     2개 (Vault 1 + enVector 1)

cold start    매 세션마다 수 초              데몬 최초 기동 시 1회

설치          venv + pip + ~30 패키지        단일 바이너리 다운로드
```

---

## 3. 설치 과정 변화

### 3.1 현재 (Python)

```
claude plugin install rune
  │
  ▼
plugin.json의 mcpServers.envector.command 실행
  │
  ▼
scripts/bootstrap-mcp.sh
  │
  ├── Python 3.12+ 확인
  │
  ├── .venv 생성 (없으면)
  │     └── python3 -m venv .venv
  │
  ├── pip install -r requirements.txt (~30개 패키지)
  │     ├── pyenvector          (FHE SDK)
  │     ├── fastmcp             (MCP 프레임워크)
  │     ├── sentence-transformers (임베딩)
  │     ├── torch               (transitive dep, 수백 MB)
  │     ├── fastembed            (ONNX 임베딩)
  │     ├── numpy, pydantic, httpx, ...
  │     ├── anthropic, openai, google-generativeai  (LLM 클라이언트)
  │     └── 기타 ~20개
  │
  ├── self-healing 레이어 1: venv Python 버전 vs site-packages 버전 불일치 감지
  │     └── 불일치 시 rm -rf .venv → 재생성
  │
  ├── self-healing 레이어 2: pip shebang 오염 감지
  │     └── Claude Code가 플러그인 디렉토리를 복사할 때 shebang 경로 깨짐
  │     └── 감지 시 모든 bin/* 스크립트의 shebang 재작성
  │
  ├── self-healing 레이어 3: fastembed 모델 캐시 불완전 다운로드 감지
  │     └── .incomplete 파일 존재 시 모델 디렉토리 삭제 → 재다운로드 유도
  │
  └── exec .venv/bin/python3 mcp/server/server.py --mode stdio
```

**문제점**:
- venv 생성: 5-15초 (디스크 I/O, Python 버전에 따라 다름)
- pip install: 30초-수 분 (네트워크, torch 크기, 캐시 유무)
- Python 버전 불일치: macOS 업데이트 후 Python 3.11 → 3.12로 바뀌면 venv 재생성 필요
- Claude Code 플러그인 캐시: 디렉토리 복사 시 venv 내부 경로가 깨짐
- torch transitive dependency: sentence-transformers → torch, 500MB+
- 3단계 self-healing이 필요한 이유 자체가 문제

### 3.2 새로 (Go)

```
claude plugin install rune
  │
  ▼
plugin.json 또는 install hook이:
  │
  ├── target OS/arch 감지
  │     darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
  │
  ├── pre-built Go 바이너리 다운로드
  │     ├── rune    (CLI, ~15 MB)     → ~/.rune/bin/rune
  │     └── runed   (데몬, ~20 MB)    → ~/.rune/bin/runed
  │     (rune-mcp는 선택적, MCP 모드 시)
  │
  ├── 데몬 등록
  │     ├── macOS: launchd plist → ~/Library/LaunchAgents/com.rune.daemon.plist
  │     └── Linux: systemd user unit → ~/.config/systemd/user/runed.service
  │
  ├── runed 데몬 기동
  │     └── launchctl load 또는 systemctl --user start runed
  │
  └── 데몬 startup 시퀀스:
        ├── config.json 로드 (없으면 dormant 상태로 대기)
        ├── Vault 연결 → 키 번들 다운로드 (EncKey, EvalKey, DEK, enVector 자격증명)
        ├── FHE 키 → ~/.rune/keys/ 캐시 + 메모리 로드
        ├── 임베딩 모델 다운로드 (첫 실행만, 이후 캐시)
        ├── 임베딩 모델 메모리 로드
        ├── gRPC 채널 풀 초기화
        └── unix socket listen → ready
```

**개선점**:
- venv 없음, pip 없음, Python 불필요
- 단일 정적 바이너리, 크로스 컴파일 완료 상태로 배포
- 설치 시간: 바이너리 다운로드 수 초
- self-healing 불필요 (shebang, venv 오염, 모델 캐시 문제 전부 해당 없음)
- 데몬이 한 번 뜨면 이후 세션은 즉시 사용 가능 (cold start 없음)

---

## 4. 멀티세션 모델

### 4.1 요청 처리 방식

`runed`는 **stateless request handler**다.
각 tool call(capture, recall 등)은 유닉스 소켓을 통한 독립된 HTTP 요청이며,
Go goroutine이 하나씩 처리한다.

```
세션 A: recall 요청 ──────┐
                           │     ┌─────────────────────────────────┐
세션 B: capture 요청 ──────┼────→│ runed HTTP handler              │
                           │     │                                 │
세션 C: recall 요청 ───────┘     │  goroutine 1: recall (세션 A)  │
                                 │  goroutine 2: capture (세션 B) │
                                 │  goroutine 3: recall (세션 C)  │
                                 │                                 │
                                 │  공유 자원 (read-only):         │
                                 │    임베딩 모델                   │
                                 │    FHE 키                       │
                                 │    gRPC 채널 풀                 │
                                 │    config (RWMutex)             │
                                 └─────────────────────────────────┘
```

### 4.2 공유 자원과 동시성 안전

| 자원 | 공유 방식 | 동시성 보장 |
|---|---|---|
| 임베딩 모델 | 로드 후 read-only | 모델 가중치 읽기만 하므로 lock 불필요 |
| gRPC 채널 풀 | 연결 풀 공유 | HTTP/2 stream multiplexing: 하나의 TCP 연결 위에 복수 RPC 동시 수행 |
| FHE 키 (EncKey, EvalKey) | 로드 후 read-only | Vault에서 fetch 후 변경 없음, lock 불필요 |
| config | `sync.RWMutex` 보호 | 읽기는 `RLock` (reader끼리 blocking 없음), fsnotify 갱신 시 `Lock` |
| capture_log.jsonl | 파일 쓰기 | `sync.Mutex` 또는 buffered writer로 직렬화 |

### 4.3 세션 식별

데몬에 per-session state는 없다. `session_id`는 요청 body에 포함되지만
감사(audit)/로깅 용도일 뿐이다:

```json
{
  "query": "PostgreSQL 결정 이유",
  "topk": 5,
  "session_id": "claude-abc123"
}
```

- 데몬은 `session_id`를 capture_log.jsonl에 기록
- 처리 로직은 `session_id`에 의존하지 않음
- 세션이 종료되어도 데몬 측에서 정리할 것이 없음

### 4.4 동시 요청이 안전한 이유

**임베딩 모델 추론**: 모델 가중치는 로딩 후 read-only 메모리 영역이다.
여러 goroutine이 동시에 embed()를 호출해도 각자 자기 입력 텐서를 할당하고,
공유 가중치에 대해 행렬 곱만 수행한다. 쓰기가 없으므로 경합이 없다.

**gRPC 채널**: gRPC는 HTTP/2 위에 구축된다. 하나의 TCP 연결 위에 독립된
stream이 다중화(multiplexing)되므로, 하나의 채널로 여러 RPC를 동시에 보낼
수 있다. `google.golang.org/grpc`의 `ClientConn`은 내부적으로 connection pool +
stream 관리를 수행하며, goroutine-safe하다.

**config 읽기**: 대부분의 요청은 config를 읽기만 한다 (`RLock`). Reader끼리는
blocking이 없다. config 갱신(fsnotify)은 `Lock`을 잡지만, 갱신 빈도가 극히
낮으므로 (사용자가 config.json을 수동 편집할 때만) 실질적 경합이 없다.

---

## 5. 메모리 최적화

### 5.1 메모리 구성 분석

`runed`의 RSS는 대부분 임베딩 모델이 차지한다.

```
runed 프로세스 메모리 분해 (steady-state 추정)
───────────────────────────────────────────────────────────────

  임베딩 모델 가중치             300-800 MB
  ──────────────────────         ██████████████████████████████
  (모델 선택에 따라 달라짐.       (전체의 ~80%)
   fastembed ONNX 기준
   ~300 MB, sbert 기준
   ~500 MB, 큰 모델 ~800 MB)

  FHE 키                         50-80 MB
  ──────────────────────         ████
  EncKey.json: 수 KB
  EvalKey.json: 수십 MB
  DEK: 32 bytes

  Go 런타임 + 버퍼               20-30 MB
  ──────────────────────         ██
  goroutine 스택, GC 메타데이터,
  HTTP 서버 버퍼, gRPC 라이브러리

  gRPC 연결                      ~1-2 MB
  ──────────────────────         ▏
  TCP 소켓 버퍼는 커널 관리,
  userspace는 메타데이터만

  per-request 할당               ~수 KB-수 MB (일시적)
  ──────────────────────         (GC가 빠르게 회수)
  입력 텍스트, 임베딩 벡터 (1024-dim float64 = 8 KB),
  JSON 직렬화 버퍼

  ───────────────────────────────────────────────────────────
  합계 (steady-state)            ~400-900 MB
```

### 5.2 현재 대비 절감

```
           현재 (3세션 Python)                   새 (3세션 runed)
           ────────────────────                  ──────────────────

프로세스A  [모델 300-800MB][키][gRPC][Python]
프로세스B  [모델 300-800MB][키][gRPC][Python]
프로세스C  [모델 300-800MB][키][gRPC][Python]
           ─────────────────────────────────
합계:      900-2,400 MB (모델만)                  runed:   300-800 MB (모델 1벌)
           + Python 오버헤드 각 50-100 MB          + 키/런타임 70-110 MB
           + pip 패키지 메모리 각 50-100 MB         thin client: 3 x ~10 MB
           ─────────────────────────────────      ──────────────────
           총 ~1,200-2,700 MB                     총 ~400-940 MB
```

### 5.3 메모리 증가하지 않는 구조

- **per-session state 없음**: 세션이 10개여도 데몬 메모리는 동일
- **per-request 할당은 단명**: goroutine이 끝나면 GC 대상. Go GC는 수 ms 내 회수
- **모델은 세션 수에 무관**: 1세션이든 10세션이든 모델 가중치 메모리는 동일
- **gRPC 채널은 multiplexing**: 세션이 늘어도 TCP 연결 수 불변

---

## 6. 전체 시스템 다이어그램

```
┌─ 사용자 머신 ───────────────────────────────────────────────────────────────────────┐
│                                                                                     │
│  ┌─ 에이전트 세션들 ──────────────────────────────────────────────────────────────┐  │
│  │                                                                                │  │
│  │  ┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐          │  │
│  │  │ Claude     │    │ Claude     │    │ Codex      │    │ Gemini     │          │  │
│  │  │ Code A     │    │ Code B     │    │            │    │ CLI        │          │  │
│  │  │            │    │            │    │            │    │            │          │  │
│  │  │ scribe.md  │    │ retriever  │    │ scribe.md  │    │ (MCP 지원  │          │  │
│  │  │ retriever  │    │   .md      │    │            │    │  시)       │          │  │
│  │  │   .md      │    │            │    │            │    │            │          │  │
│  │  └─────┬──────┘    └─────┬──────┘    └─────┬──────┘    └─────┬──────┘          │  │
│  │        │                 │                 │                 │                  │  │
│  └────────┼─────────────────┼─────────────────┼─────────────────┼──────────────────┘  │
│           │ MCP stdio       │ MCP stdio       │ Bash exec       │ MCP stdio           │
│           ▼                 ▼                 ▼                 ▼                     │
│  ┌─ thin client 레이어 ──────────────────────────────────────────────────────────┐    │
│  │                                                                                │    │
│  │  ┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐          │    │
│  │  │ rune-mcp   │    │ rune-mcp   │    │ rune CLI   │    │ rune-mcp   │          │    │
│  │  │ (~10MB)    │    │ (~10MB)    │    │ (ephemeral)│    │ (~10MB)    │          │    │
│  │  │ 상주       │    │ 상주       │    │ 즉시 종료  │    │ 상주       │          │    │
│  │  └─────┬──────┘    └─────┬──────┘    └─────┬──────┘    └─────┬──────┘          │    │
│  │        │                 │                 │                 │                  │    │
│  └────────┼─────────────────┼─────────────────┼─────────────────┼──────────────────┘    │
│           │ HTTP            │ HTTP            │ HTTP            │ HTTP                  │
│           │ POST            │ POST            │ POST            │ POST                  │
│           └────────┬────────┴────────┬────────┴────────┬────────┘                       │
│                    │                 │                 │                                 │
│                    ▼                 ▼                 ▼                                 │
│           ┌────────────────────────────────────────────────┐                             │
│           │          ~/.rune/sock                          │                             │
│           │       (unix domain socket, 0600)               │                             │
│           └──────────────────────┬─────────────────────────┘                             │
│                                  │                                                       │
│                                  ▼                                                       │
│  ┌─ runed (Go 데몬, PID 1개) ────────────────────────────────────────────────────────┐  │
│  │                                                                                    │  │
│  │   ┌──────────────────────────────────────────────────────────────────────────┐     │  │
│  │   │                          HTTP Router                                    │     │  │
│  │   │  POST /capture   POST /batch-capture   POST /recall   POST /reload     │     │  │
│  │   │  GET  /health    GET  /diagnostics      GET  /history  GET /vault-status│     │  │
│  │   │  DELETE /captures/:id                                                   │     │  │
│  │   └─────────────────────────────┬────────────────────────────────────────────┘     │  │
│  │                                 │                                                  │  │
│  │   ┌─────────────────────────────▼────────────────────────────────────────────┐     │  │
│  │   │                         핵심 로직                                        │     │  │
│  │   │                                                                          │     │  │
│  │   │   ┌─────────────────┐  ┌─────────────────┐  ┌────────────────────────┐  │     │  │
│  │   │   │ Config Manager  │  │ Embedding Engine │  │ FHE Key Cache         │  │     │  │
│  │   │   │                 │  │                  │  │                        │  │     │  │
│  │   │   │ config.json     │  │ ONNX/sbert 모델  │  │ EncKey.json (수 KB)   │  │     │  │
│  │   │   │ fsnotify watch  │  │ 300-800 MB       │  │ EvalKey.json (수십 MB)│  │     │  │
│  │   │   │ RWMutex 보호    │  │ read-only        │  │ DEK (32 bytes)        │  │     │  │
│  │   │   └─────────────────┘  └─────────────────┘  │ read-only              │  │     │  │
│  │   │                                              └────────────────────────┘  │     │  │
│  │   │   ┌──────────────────────────────────────────────────────────────────┐  │     │  │
│  │   │   │ Capture Pipeline                                                │  │     │  │
│  │   │   │ text → embed() → FHE encrypt(EncKey) → AES encrypt(DEK)        │  │     │  │
│  │   │   │      → enVector.insert() → capture_log.jsonl append            │  │     │  │
│  │   │   ├──────────────────────────────────────────────────────────────────┤  │     │  │
│  │   │   │ Recall Pipeline                                                 │  │     │  │
│  │   │   │ query → embed() → enVector.score() → Vault.DecryptScores()     │  │     │  │
│  │   │   │       → enVector.remind() → Vault.DecryptMetadata()            │  │     │  │
│  │   │   │       → filter → recency rerank → 결과 조립                    │  │     │  │
│  │   │   ├──────────────────────────────────────────────────────────────────┤  │     │  │
│  │   │   │ Utility: history, delete, diagnostics, vault_status, reload    │  │     │  │
│  │   │   └──────────────────────────────────────────────────────────────────┘  │     │  │
│  │   └──────────────────────────────────────────────────────────────────────────┘     │  │
│  │                                                                                    │  │
│  │   ┌────────────────────────┐          ┌──────────────────────────────────────┐     │  │
│  │   │ vault-go               │          │ envector-go                          │     │  │
│  │   │                        │          │                                      │     │  │
│  │   │ gRPC ClientConn        │          │ gRPC ClientConn                      │     │  │
│  │   │ (HTTP/2 multiplexing)  │          │ (HTTP/2 multiplexing)                │     │  │
│  │   │                        │          │                                      │     │  │
│  │   │ GetPublicKey           │          │ Score (FHE cosine similarity)        │     │  │
│  │   │ DecryptScores          │          │ Remind (메타데이터 조회)             │     │  │
│  │   │ DecryptMetadata        │          │ Insert (FHE 암호화 벡터 삽입)        │     │  │
│  │   └───────────┬────────────┘          └─────────────────────┬────────────────┘     │  │
│  │               │                                             │                      │  │
│  └───────────────┼─────────────────────────────────────────────┼──────────────────────┘  │
│                  │                                             │                         │
│  ┌─ ~/.rune/ (파일 시스템) ──────────────────────────────────────────────────────────┐  │
│  │                                                                                    │  │
│  │  config.json          상태 + Vault 자격증명                                       │  │
│  │  sock                 unix domain socket (0600)                                    │  │
│  │  keys/EncKey.json     FHE 공개키 (디스크 캐시)                                    │  │
│  │  keys/EvalKey.json    FHE 평가키 (디스크 캐시, 수십 MB)                           │  │
│  │  logs/capture_log.jsonl  캡처 이력                                                │  │
│  │  bin/rune             CLI 바이너리                                                │  │
│  │  bin/runed            데몬 바이너리                                               │  │
│  │  bin/rune-mcp         MCP shim 바이너리 (선택)                                    │  │
│  │  cache/models/        임베딩 모델 파일 캐시                                       │  │
│  │                                                                                    │  │
│  └────────────────────────────────────────────────────────────────────────────────────┘  │
│                                                                                         │
└─────────────────────────────────────────────────────────────────────────────────────────┘
                  │                                             │
                  │ gRPC / TLS                                  │ gRPC / TLS
                  ▼                                             ▼
┌─ 외부 서비스 ──────────────────────────────────────────────────────────────────────────┐
│                                                                                        │
│  ┌──────────────────────────────┐       ┌──────────────────────────────────────────┐   │
│  │ Rune-Vault                  │       │ enVector Cloud                           │   │
│  │                              │       │                                          │   │
│  │ 역할:                       │       │ 역할:                                    │   │
│  │  - FHE 키 번들 관리          │       │  - FHE 암호화 벡터 저장/검색             │   │
│  │  - FHE ciphertext 복호화    │       │  - 동형 암호 상태에서 cosine similarity   │   │
│  │  - AES 메타데이터 복호화     │       │  - 암호화된 메타데이터 저장/반환          │   │
│  │                              │       │                                          │   │
│  │ 프로토콜: gRPC + TLS         │       │ 프로토콜: gRPC + TLS (Go SDK 경유)       │   │
│  │ 인증: Vault token (evt_xxx) │       │ 인증: API key (Vault 번들에서 획득)      │   │
│  └──────────────────────────────┘       └──────────────────────────────────────────┘   │
│                                                                                        │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### 데이터 흐름 요약

```
capture 흐름:
  agent → thin client → unix sock → runed
    → embed(text) → FHE encrypt(EncKey) → AES encrypt(DEK, metadata)
    → enVector.Insert(encrypted_vec, encrypted_meta)       ←── gRPC/TLS
    → capture_log.jsonl append
    → HTTP 200 → thin client → agent

recall 흐름:
  agent → thin client → unix sock → runed
    → embed(query) → enVector.Score(encrypted_query_vec)   ←── gRPC/TLS
    → Vault.DecryptScores(fhe_ciphertext)                  ←── gRPC/TLS
    → enVector.Remind(top_k_rows)                          ←── gRPC/TLS
    → Vault.DecryptMetadata(aes_ciphertexts)               ←── gRPC/TLS
    → filter → recency rerank → 결과 조립
    → HTTP 200 → thin client → agent
```
