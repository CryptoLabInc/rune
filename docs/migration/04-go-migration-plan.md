# Rune Python → Go 마이그레이션 계획

## MVP 스코프 선언

MVP는 **한 문장**이다: Claude Code, Codex, Gemini 사용자들에게 오늘과 같은
agent-delegated `capture` / `recall` 경험을 주되, Python venv 대신 단일 Go
바이너리로, 얇은 feature set에 legacy 짐 없이.

구체적으로 MVP가 배포되는 조건:

- macOS와 Linux에 단일 `rune` Go 바이너리가 설치 가능하다.
- 바이너리가 MCP 호환 에이전트와 **stdio MCP**로 통신한다.
- 8개의 MCP tool 전부 존재: `capture`, `batch_capture`, `recall`,
  `vault_status`, `diagnostics`, `reload_pipelines`, `capture_history`,
  `delete_capture`.
- `capture`는 **agent-delegated only**로 동작한다 (legacy 3-tier 경로는
  사라진다).
- `recall`은 기본적으로 **raw 결과만** 리턴한다 (에이전트가 합성).
- Vault gRPC + enVector FHE round-trip이 동일한 `~/.rune/config.json`
  스키마(드롭된 필드 제외)로 end-to-end 작동한다.
- `rune configure`, `rune activate`, `rune deactivate`, `rune status`,
  `rune reset` 커맨드가 동작하며 현재 Claude Code 슬래시 커맨드 UX와
  일치한다.
- Claude Code에서 `claude plugin install rune`, Codex에서
  `$skill-installer install rune`로 플러그인이 설치 가능하다.

그 외 전부 — 서버 사이드 LLM 합성, phase chain 재구성, 멀티링구얼 쿼리
라우팅, 여러 임베딩 백엔드, Slack/Notion 인제스션, 리뷰 큐, 서버 사이드
Tier 2/3 — 는 **MVP 스코프 밖**이다 (판정 내역은
[03-feature-inventory.md](03-feature-inventory.md) 참고).

## 왜 Go, 왜 지금

- **배포**: 사용자들이 Python 플러그인에서 venv/pip 이슈를 끊임없이 맞는다.
  `bootstrap-mcp.sh`에 이미 pip shebang과 fastembed 캐시 오염에 대한
  self-healing 레이어가 3개 있다 — 신호다. 정적 Go 바이너리 하나로 그런
  설치 실패 클래스 전체를 제거한다.
- **시작 속도**: Python + `sentence-transformers` 초기화는 cold에서 여러
  초가 걸린다. 임베딩 모델을 장수명 백그라운드 프로세스로 옮기면 호출당
  지연시간이 줄어든다.
- **표면 축소**: ~4,500 라인의 legacy + 중복을 포팅하는 대신 드롭할 수 있다.
- **동시성 적합**: Rune의 자연스러운 형태는 모델 하나와 gRPC 클라이언트
  몇 개를 붙잡고 있으면서 매우 작은 요청 핸들러를 제공하는 장수명
  서비스다. Go가 바로 그것을 위한 언어다.

## 타겟 아키텍처

핵심 설계 분할은 **장수명 임베딩 서비스** (모델 보유, Vault와 enVector와
통신, 모든 암호학 작업 수행) 와 **MCP CLI** (에이전트와 stdio MCP를 주고받고,
tool 호출을 임베딩 서비스에 대한 로컬 HTTP 요청으로 변환) 사이에 있다.

```
┌──────────────────────────────────────────────────────────────────────┐
│                        개발자의 머신                                │
│                                                                      │
│  ┌────────────┐   stdio MCP    ┌──────────────────────────────────┐  │
│  │ 에이전트 CLI│◄──────────────►│ rune mcp                         │  │
│  │ (Claude,   │                │   Go 바이너리, 단수명             │  │
│  │  Codex,    │                │   • MCP 서버 (stdio)             │  │
│  │  Gemini)   │                │   • 각 tool 호출을 데몬에         │  │
│  └────────────┘                │     HTTP POST로 포워드           │  │
│                                │   • 필요 시 데몬을 on-demand로   │  │
│                                │     시작                          │  │
│                                └─────────────┬────────────────────┘  │
│                                              │                       │
│                                              │ 유닉스 소켓 위         │
│                                              │ HTTP + JSON           │
│                                              │ /tmp/rune.sock        │
│                                              ▼                       │
│                                ┌──────────────────────────────────┐  │
│                                │ runed                            │  │
│                                │   Go 바이너리, 장수명 데몬        │  │
│                                │   • /capture  /batch-capture     │  │
│                                │   • /recall                      │  │
│                                │   • /vault-status /diagnostics   │  │
│                                │   • /reload      /history /delete│  │
│                                │   • SBERT 모델을 메모리에 보유    │  │
│                                │   • gRPC 클라이언트 → Rune-Vault │  │
│                                │   • SDK 클라이언트 → enVector    │  │
│                                │   • 파일 I/O  → ~/.rune/*        │  │
│                                └──────────┬──────────────┬────────┘  │
│                                           │              │           │
└───────────────────────────────────────────┼──────────────┼───────────┘
                                            │              │
                          ┌─────────────────┘              └─────────────┐
                          ▼                                              ▼
              ┌──────────────────────┐                        ┌───────────────────────┐
              │  Rune-Vault (Go)     │                        │  enVector Cloud       │
              │  gRPC 50051          │                        │  (hosted FHE)         │
              │  (변경 없음)          │                        │  (변경 없음)           │
              └──────────────────────┘                        └───────────────────────┘
```

### 왜 Go 바이너리 둘인가?

에이전트가 세션마다 새 MCP 서버 프로세스를 스폰하기 때문이고, **매 스폰마다
임베딩 모델을 다시 로드하고 싶지 않기** 때문이다. 분할은:

- `rune mcp`는 시작이 저렴하다. 하는 일은 stdio MCP 프레임을 파싱해서
  포워드하는 것 뿐. 에이전트가 연결을 끊으면 warm state를 잃지 않고 죽어도
  된다.
- `runed`는 시작이 느리고 (모델 로드) 살아있기는 저렴하다. 임베딩 모델,
  커넥션 풀, 작은 인메모리 캐시를 소유한다.

`rune mcp`는 데몬이 안 떠 있으면 on-demand로 실행시키고 (`systemctl --user`,
launchd, 또는 그냥 `runed &`), 소켓을 기다린 뒤 포워드한다. 데몬이 이미
떠 있으면 이후의 `rune mcp` 스폰은 밀리초 안에 연결된다.

대안은 모든 걸 `rune mcp`에서 하고 스폰마다 모델을 로드하는 것이다 — 더
단순하지만 1024-dim 임베딩 모델로는 첫 호출 지연시간이 수용 불가능하다.
그걸 피하려고 데몬의 복잡도를 감수한다.

### 왜 유닉스 소켓 위의 HTTP이고 gRPC가 아닌가?

- 로컬 전용 IPC가 필요하다; 유닉스 소켓이 맞는 전송.
- HTTP + JSON은 디버깅이 더없이 쉽다 (`curl --unix-socket /tmp/rune.sock
  ...`).
- 언어 교차 요구가 없다 — 양쪽 다 Go, 양쪽 다 우리 것이라 "gRPC로 크로스
  랭귀지에서 강타입을 쓰자"는 보통의 논리가 해당되지 않는다.
- HTTP/1.1 on 유닉스 소켓의 호출당 오버헤드는 sub-millisecond다; 임베딩
  연산이 2–3 자릿수(order of magnitude)로 지배적.
- Rune-Vault와는 여전히 gRPC로 통신 — 변경 없음.

## 제안된 Go 패키지 레이아웃

```
rune/
├── cmd/
│   ├── rune/                      # 얇은 CLI: `rune configure`, `rune status`, 등
│   │   └── main.go
│   ├── rune-mcp/                  # stdio MCP 어댑터 (플러그인 매니페스트가 호출)
│   │   └── main.go
│   └── runed/                     # 장수명 데몬
│       └── main.go
│
├── internal/
│   ├── config/                    # RuneConfig, JSON load/save, env override
│   │   ├── config.go
│   │   └── state.go               # active/dormant 전이, dormant_reason 코드
│   │
│   ├── mcp/                       # `github.com/mark3labs/mcp-go` 등 위의 MCP 서버 shim
│   │   ├── server.go              # tool 등록, HTTP 호출 마샬링
│   │   └── tools.go               # tool당 함수 하나
│   │
│   ├── daemon/                    # runed 서비스 구현
│   │   ├── http.go                # 유닉스 소켓 HTTP 핸들러
│   │   ├── capture.go
│   │   ├── recall.go
│   │   ├── history.go
│   │   ├── diagnostics.go
│   │   └── lifecycle.go
│   │
│   ├── embed/                     # 임베딩 서비스 (모델 보유자)
│   │   ├── service.go
│   │   └── sbert/                 # sentence-transformers 로더
│   │       └── sbert.go
│   │
│   ├── vault/                     # Rune-Vault gRPC 클라이언트
│   │   ├── client.go
│   │   └── pb/                    # 생성된 protobuf 코드, upstream에서 복제
│   │
│   ├── envector/                  # enVector SDK 클라이언트
│   │   ├── client.go
│   │   └── fhe/                   # CGO 바인딩 또는 HTTP shim (open questions 참고)
│   │
│   ├── record/                    # DecisionRecord + 스키마 (single/phase_chain/bundle)
│   │   ├── record.go
│   │   └── novelty.go             # novelty 임계값 + 로직
│   │
│   ├── retriever/                 # query_processor + searcher 대응
│   │   ├── query.go               # 의도/엔티티/시간 범위 추출
│   │   ├── search.go              # 멀티 쿼리 검색 + dedup
│   │   └── rerank.go              # recency + status 승수
│   │
│   ├── crypt/                     # AES-DEK 메타데이터 암호화
│   │   └── dek.go
│   │
│   ├── errs/                      # RuneError 대응
│   │   └── errors.go
│   │
│   └── logio/                     # capture_log.jsonl append + read
│       └── jsonl.go
│
├── testdata/                      # 픽스처 (EncKey.json 샘플, golden 벡터 등)
├── plugin/                        # Claude Code / Codex / Gemini 매니페스트
├── scripts/                       # 대체 인스톨러 (짧게!)
└── docs/
```

## 와이어 프로토콜

### `rune mcp` ↔ `runed` (유닉스 소켓 위 로컬 HTTP)

각 MCP tool에는 1:1 HTTP 엔드포인트가 있다. 요청/응답 형태가 현재 MCP
파라미터 스키마와 정확히 일치하므로 MCP shim은 자명한 디스패처다.

```
POST /v1/capture       { text, source, user, channel, extracted } → { ok, record_id, novelty }
POST /v1/batch-capture { items: [...] }                            → { results: [...] }
POST /v1/recall        { query, topk, domain, status, since }     → { results, confidence, related_queries, warnings }
GET  /v1/vault-status                                              → { ok, vault_configured, vault_endpoint, secure_search_available, mode, vault_healthy, team_index_name }
GET  /v1/diagnostics                                               → { vault, envector, embedding, pipelines }
POST /v1/reload                                                    → { ok, state }
GET  /v1/history?limit=...&domain=...&since=...                    → { entries: [...] }
POST /v1/delete        { record_id }                               → { ok }
```

auth 없음 — 유닉스 소켓이 파일시스템 퍼미션 (0600, 사용자 소유)으로 보호된다.
로컬 머신에서 이 소켓을 읽을 수 있는 것은 `runed` 자신보다 덜 신뢰되지
않는다.

### `runed` ↔ Rune-Vault (gRPC, 변경 없음)

upstream의 `vault_service.proto`로부터 `internal/vault/pb/` 아래의 Go
스텁을 재생성한다. 사용되는 RPC:

- `GetPublicKey` — 데몬 시작 시 한 번 호출, 결과를 인메모리 + 디스크
  (`~/.rune/keys/*.json`)에 캐시.
- `DecryptScores` — 모든 recall round-trip에서 호출 (그리고 capture novelty
  체크에서도).
- `DecryptMetadata` — 모든 recall round-trip에서 호출.

TLS 모드는 현재 Python 클라이언트와 일치:
- `tls_disable: true` → `grpc.WithTransportCredentials(insecure.NewCredentials())`
- `ca_cert: "..."` → PEM 로드, `credentials.NewTLS(&tls.Config{RootCAs: pool})`
  빌드
- 그 외 → 시스템 cert pool.

### `runed` ↔ enVector Cloud (FHE SDK)

아래 open questions 참고 — 가장 불확실한 부분이다. 옵션:

1. **SDK를 Go로 네이티브 포팅** — 진지한 엔지니어링 작업, MVP를 막는다.
2. **`pyenvector`를 CGO로 wrap** — 정적 배포에 고통스럽다.
3. **FHE 암호화/insert/score/remind만 위한 작은 Python 사이드카 유지** —
   실용적 대안, 단 "단일 바이너리" 목표는 무너진다.
4. **만약 pyenvector가 얇은 HTTP 래퍼라면 enVector Cloud API와 직접 HTTP
   통신** — SDK의 wire format을 확인해야 한다.
5. **enVector 팀에게 공식 Go SDK 요청** — 1–4 중 하나에 commit하기 전에
   확인할 가치 있음.

**권장**: Phase 1의 **첫 주**를 spike에 써서 1/4/5 중 어느 게 실행 가능한지
결정한다. 아무것도 안 되면 Phase 1은 옵션 3 (FHE만 Python 사이드카, 나머지는
Go 데몬)으로 출시하고 Phase 2에서 제거한다.

## 단계별 계획

### Phase 0 — 결정 (≤ 1주)

1. **FHE 바인딩 spike**. `pyenvector` 소스를 읽는다. `insert` / `score` /
   `remind`의 wire format을 조사한다. 우리가 Go로 재작성할 수 있는 깨끗한
   HTTP 클라이언트인지, CGO 래핑된 C 라이브러리인지, 아니면 정말로
   Python 전용인지 판단한다. enVector 팀에게 Go SDK에 대해 문의한다.
   위 1/3/4/5 중 선택.
2. **임베딩 출력 차원 확정**. `Qwen/Qwen3-Embedding-0.6B`를 현재 Python
   `SBERTSDKAdapter`에 태워서 `vec.shape`를 출력한다. 이 사실이 Go 쪽에서
   enVector 인덱스 스키마를 commit하는 데 있어 가장 중요한 단일 사실이다.
   ⚠️ 여러 AS-IS 서베이가 384 dim으로 보고했는데, 이건 MiniLM 클래스
   default 인자일 뿐 — 실제로 사용되는 Qwen3 모델이 **아니다**. commit 전
   실측 필수.
3. **Go 임베딩 라이브러리 선택**. 옵션:
   - `sentence-transformers`를 Python으로 CGO wrap (추하다).
   - Go 네이티브 ONNX 런타임 (`github.com/yalue/onnxruntime_go`) 과
     Qwen3-0.6B ONNX export 사용.
   - 작은 `runed-embed` Python 서브프로세스 호출 (실용적 대안).
   ONNX가 장기적으로 가장 강한 답이다; 기대하는 토크나이저를 가진 Qwen3
   ONNX export가 존재하는지 확인.
4. **위 세 결정 각각에 대해 작성된 ADR 만들기**. `docs/migration/adr/`에
   아카이브. 이게 나머지 전부를 unblock한다.

Exit 기준: 세 질문에 대해 팀이 사인오프한 서면 답이 있음.

### Phase 1 — Walking skeleton (2–3주)

목표: 임베딩과 vault 경로를 제외한 모든 곳에 스텁을 두고 아키텍처를
end-to-end로 증명.

- [ ] `cmd/runed`, `cmd/rune-mcp`, `cmd/rune`이 컴파일되어 바이너리 산출.
- [ ] `runed`이 유닉스 소켓을 listen하고 8개 엔드포인트 전부에 하드코딩
      스텁 데이터로 응답.
- [ ] `rune-mcp`가 각 stdio MCP 호출을 소켓으로 포워드. Claude Code 세션이
      `capture` / `recall` / `diagnostics`를 호출하고 스텁 응답을 받는다.
- [ ] `rune configure`가 축소된 스키마 (no tier2/slack/notion/openai/google)
      에 맞게 `~/.rune/config.json`을 쓴다.
- [ ] `internal/vault`가 실제 Rune-Vault 인스턴스에 대해 실제
      `GetPublicKey` / `DecryptScores` / `DecryptMetadata`를 구현.
- [ ] `internal/embed`이 선택된 임베딩 백엔드와 Qwen3-0.6B 모델을 로드;
      `daemon/diagnostics.go`가 로드 시간 + dim 보고.
- [ ] 플러그인 매니페스트가 `bootstrap-mcp.sh` 대신 `rune-mcp`를 실행하도록
      업데이트.

Exit 기준: `rune status`가 실제 인프라에 대해 green 헬스 체크 출력; 스텁
capture/recall 응답이 end-to-end로 통과.

### Phase 2 — Capture happy path (1–2주)

- [ ] `internal/record`이 DecisionRecord를 single + phase_chain + bundle
      스키마로 구현.
- [ ] `internal/envector`이 `insert` 구현 (Phase 0 FHE spike 결과에 의존).
- [ ] `daemon/capture.go`의 full flow: embed → novelty 체크 → insert →
      `capture_log.jsonl` append.
- [ ] Novelty 임계값 wiring (< 0.3 novel, 0.3–0.7 evolution, 0.7–0.95
      related, ≥ 0.95 near_duplicate → 블록).
      ※ 런타임 기본값은 server.py::_classify_novelty()의 인자(0.3/0.7/0.95).
      embedding.py의 상수(0.4/0.7/0.93)와 불일치 — Go 구현 시 어떤 값을 canonical로
      삼을지 결정 필요.
- [ ] `batch_capture`가 per-item 에러 격리로 `capture`를 반복.
- [ ] Claude Code에서 `/rune:capture`가 end-to-end round-trip.
- [ ] 테스트용 Vault + 테스트용 enVector 인덱스에 대한 integration 테스트.

Exit 기준: Claude Code 세션이 결정을 캡처했을 때
`~/.rune/capture_log.jsonl`과 테스트 enVector 인덱스에 나타남.

### Phase 3 — Recall happy path (1–2주)

- [ ] `internal/retriever/query.go`의 regex 기반 의도 + 엔티티 + 시간 범위
      추출 (영어만; 한/일 LLM 라우팅은 연기).
- [ ] `internal/envector`이 `score` + `remind` 구현.
- [ ] `daemon/recall.go`의 full flow: 쿼리 파싱 → 임베딩 → 멀티 쿼리 검색
      (MVP는 단일 쿼리, 확장은 연기) → Vault 점수/메타데이터 복호 →
      필터 → recency rerank → raw 결과 리턴.
- [ ] Claude Code에서 `/rune:recall`이 round-trip; 에이전트가 최종 응답을
      합성.
- [ ] 픽스처 인덱스에 대한 golden-result 테스트.

Exit 기준: Claude Code 세션이 이전에 캡처된 결정에 대해 물었을 때 에이전트의
응답이 올바른 레코드를 인용.

### Phase 4 — 라이프사이클 + diagnostics + 설치 (1주)

- [ ] `internal/config/state.go`가 모든 `dormant_reason` 코드를 포함한
      active/dormant 상태 머신 구현.
- [ ] `rune configure` 플로우: vault endpoint, token, TLS 모드 수집; 도달
      가능성 검증; config 쓰기; 적절히 active 또는 dormant로 전이.
- [ ] `rune activate` / `rune deactivate` / `rune reset` 구현.
- [ ] `daemon/diagnostics.go`의 전체 헬스 프로브; `rune status`에서 사용.
- [ ] `rune doctor`가 `check-infrastructure.sh`를 대체.
- [ ] 크로스플랫폼 인스톨러: prebuilt darwin/amd64, darwin/arm64,
      linux/amd64 바이너리를 GitHub 릴리스에 게시; 설치 스크립트가 릴리스
      자산을 가리킴.
- [ ] 플러그인 매니페스트 + 마켓플레이스 엔트리 업데이트.
- [ ] 내부 유저에게 소프트 런치; 첫 주 동안 `capture_log.jsonl` 과
      구조화 로그를 모니터링.

Exit 기준: 새 사용자가 `claude plugin install rune`으로 60초 안에 active
상태가 됨.

### Phase 5 — 폴리시 + 핸드오프 (1주)

- [ ] README / SKILL.md / AGENT_INTEGRATION.md / GEMINI.md / CLAUDE.md를
      Go 플러그인용으로 재작성.
- [ ] 기여 가이드를 Go dev 루프용으로 재작성.
- [ ] Python 코드를 `legacy/` 브랜치로 옮기고 태그; `main`에서 삭제.
- [ ] 기존 유저용 릴리스 노트 + 마이그레이션 가이드 (그들의
      `~/.rune/config.json`은 그대로 동작; 플러그인만 재설치).

Exit 기준: 공지.

### Post-MVP (스케줄은 없지만 추적)

- 서버 사이드 LLM 합성 (`synthesizer.py` 대응) — 미리 합성된 응답을
  원하는 에이전트용.
- 리콜에서 phase chain 확장.
- 리콜에서 멀티 쿼리 확장.
- 비영어 쿼리용 멀티링구얼 LLM 라우팅.
- fastembed 대체 백엔드 (config 플래그 뒤에).
- Prometheus 메트릭.
- 언어별 결과 템플릿 (KO / JA).
- 누군가 요청하면: 로컬 데몬의 `/capture` 엔드포인트에 POST하는 독립된 Go
  바이너리로서의 Slack / Notion 인제스션.

## 미결 이슈 (Open Questions)

1. **FHE Go 바인딩.** pyenvector SDK는 C 익스텐션이고, PyPI의 공식
   `pyenvector` 패키지가 우리가 아는 유일한 public entry point다. Phase 1
   시작 전 결정 필요: 공식 Go SDK (enVector 팀에 문의), CGO wrap, HTTP
   클라이언트 재작성, 또는 Python 사이드카. 가장 큰 단일 마이그레이션
   리스크.
2. **임베딩 출력 차원.** `Qwen/Qwen3-Embedding-0.6B`가 end-to-end에서
   실제로 384가 아니라 1024차원 벡터를 출력하는지 확인. enVector 인덱스
   스키마가 이것에 의존하고, 데이터 삽입 후에는 바꾸기 어렵다.
3. **enVector API 호환성.** SDK를 재작성할 때 Python 1.2.x 시리즈와 새 Go
   클라이언트 사이에서 wire protocol이 안정적인지 확인 필요. 버전 skew
   스토리는 enVector 팀과 TBD.
4. **Rune-Vault 호환성.** 기존 Vault는 이미 Go다; Go 클라이언트가 같은
   바이너리에 연결 가능할지, 아니면 매칭해야 할 프로토콜 버전이 있는지
   `rune-admin`과 확인. 무사통과여야 하지만 확인 가치 있음.
5. **macOS에서의 데몬 라이프사이클.** launchd vs 그냥 백그라운드 프로세스
   vs `rune mcp` fork-exec? 각각 크래시 복구와 퍼미션에 트레이드오프가
   있다. Phase 4 결정 사항.
6. **바이너리 크기.** SBERT 모델을 임베드하면 바이너리가 부풀어 오른다.
   모델을 따로 배포하고 첫 실행 시 다운로드할 수도 있다. Phase 1 결정.
7. **기존 유저의 `~/.rune/` 내용.** `keys/`, `capture_log.jsonl`,
   `review_queue.json`, `certs/`. Go 바이너리는 같은 파일들을 읽어야 한다.
   `review_queue.json`은 MVP에서 소비자가 없다 — 기존 유저가 제거를
   알아채지 못하도록 건드리지 말고 그대로 둔다.
8. **CI.** Python 코드가 사라지면 현재의 `ci/tests/test_ci_changed_tests.py`
   는 어디로 가는가? Phase 5의 일부로 Go로 재작성.
9. **컷오버 중의 observability.** 동일한 입력에 대해 Go 구현과 Python
   구현의 capture/recall 결과를 비교할 방법이 필요. 처음 며칠 동안 둘을
   병렬로 돌리고 shadow-log diff를 보는 것을 고려.

## 리스크

| 리스크 | 발생 가능성 | 임팩트 | 완화 |
|---|---|---|---|
| 실행 가능한 Go FHE 바인딩이 없음 → Python 사이드카에 갇힘 | 중 | 높음 | Phase 0 spike; 옵션 3 (Python FHE 사이드카) 을 fallback으로 예비 |
| 임베딩 모델 dim 불일치로 기존 enVector 인덱스 깨짐 | 낮음 | 높음 | Phase 0 실측; 현재 Python 인덱스가 이미 틀렸다면 그건 pre-existing 버그로 triage |
| Go의 SBERT 로딩이 macOS에서 Python보다 유의미하게 느림 | 중 | 중 | Phase 1에서 벤치; 필요하면 Python 임베딩 사이드카로 폴백 |
| 기존 유저의 `~/.rune/config.json` 에 있는 드롭된 필드가 load 에러 일으킴 | 낮음 | 낮음 | Config 로더는 알 수 없는 필드를 *무시* (에러 아님) |
| 새 세션의 첫 부팅에서 데몬 auto-start 레이스 | 중 | 낮음 | `rune mcp`의 2초 connect 예산을 둔 retry 루프; 동기 시작으로 fallback |
| Python `fastmcp` 와 Go MCP 라이브러리 사이의 stdio MCP 프레이밍 엣지 케이스 차이 | 중 | 중 | Phase 3–4 중 병렬 구동; tool 출력 diff |
| 에이전트 프롬프트 (`agents/claude/scribe.md` 등) 가 드롭된 동작을 참조 | 높음 | 낮음 | Phase 5의 일부로 프롬프트 재작성; 그냥 마크다운임 |
| `pyenvector` 버전 업그레이드가 Go 구현과 diverge | 중 | 중 | CI에서 enVector 클라이언트 버전 잠금; 의도적으로 bump |

## MVP에서 명시적으로 제외

- 어떤 신기능도. MVP는 **포트**지 rewrite-plus-features가 아니다.
- 리콜 응답의 서버 사이드 LLM 합성 (MVP에서는 에이전트가 합성).
- 리콜에서 phase chain 재구성 (평탄한 결과 리턴).
- 리콜에서 멀티 쿼리 확장 (단일 쿼리만).
- 비영어 쿼리 라우팅.
- Slack / Notion 인제스션.
- 리뷰 큐 + 인간 승인 UX.
- Tier 2 / Tier 3 LLM 파이프라인.
- OpenAI / Google LLM 백엔드.
- Prometheus 메트릭.
- 관리자 툴링 (프로비저닝, 로테이션, 감사).

모두 가치 있지만, 더 얇은 MVP를 먼저 배포하고 하나씩 다시 얹는 것이 현재
Python 코드베이스가 더 이상 주지 못하는 **실제 이터레이션 기점**을 준다.
