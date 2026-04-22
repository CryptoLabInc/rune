# Python 코드베이스 → 새 Go 구조 매핑

현재 rune Python 코드를 직접 읽어 실측한 LoC·역할 인벤토리. 각 파일이 새 Go 아키텍처(`rune-mcp` / `rune-embedder`)에서 어디로 옮겨지는지 기록.

**측정일**: 2026-04-20  
**총 LoC**: 약 11,500 (mcp/ + agents/ + scripts/ + commands/ 합)

## 전체 매핑 요약

| Python 디렉토리 | LoC | 새 구조에서 어디로 |
|---|---|---|
| `mcp/server/server.py` | 2002 | **`rune-mcp`** 본체 (MCP 진입점, state 머신, 8 tools) |
| `mcp/adapter/envector_sdk.py` | 386 | **`rune-mcp` adapter** (envector-go SDK + AES envelope) |
| `mcp/adapter/vault_client.py` | 380 | **`rune-mcp` adapter** (Vault gRPC) |
| `mcp/adapter/embeddings.py` | 154 | **`rune-embedder`** (ONNX/llama-server 래퍼) |
| `mcp/adapter/document_preprocess.py` | 166 | `rune-mcp` (경량 텍스트 정리) |
| `agents/common/config.py` | 365 | **`rune-mcp`** config loader (3-섹션 스키마) |
| `agents/common/schemas/embedding.py` | 56 | **`rune-mcp` policy** (novelty threshold 상수) |
| `agents/common/schemas/decision_record.py` | 259 | **`rune-mcp` domain** (DecisionRecord v2.1 타입) |
| `agents/common/schemas/templates.py` | 363 | 에이전트 md 쪽으로 이관 고려 (MVP scope 밖) |
| `agents/scribe/*` | 약 2,500 | **대부분 삭제** · 트리거·추출은 에이전트 md 책임 |
| `agents/retriever/query_processor.py` | 436 | **`rune-mcp` policy** (intent 31 regex, stop 81, entity 추출) |
| `agents/retriever/searcher.py` | 576 | **`rune-mcp` service** (rerank 공식, recency) |
| `agents/retriever/synthesizer.py` | 482 | **삭제** · 에이전트가 합성 |
| `scripts/bootstrap-mcp.sh` | 167 | **삭제** · venv/pip self-heal은 Go 바이너리에 불필요 |
| `scripts/*.sh` (나머지) | 1,500+ | Go용으로 재작성 (install / configure-mcp / uninstall 등) |
| `commands/claude/*.md` · `commands/rune/*.toml` | N/A | 대부분 유지. capture·recall의 호출부만 rune-mcp 바이너리로 교체 |

## 상세 파일별

### mcp/server/server.py (2002 LoC) — Go rune-mcp로

**역할**:
- FastMCP를 통해 8개 tool (`tool_capture`, `tool_recall`, `tool_reload_pipelines`, `tool_delete_capture`, `tool_capture_history`, `tool_vault_status`, `tool_diagnostics`, `tool_batch_capture`) 노출
- `state` 머신 (`dormant` / `active`). 새 구조는 `starting` / `waiting_for_vault` 추가
- `_init_pipelines` 120s 타임아웃 (첫 activate 시 Vault fetch + 모델 로드)
- enVector pre-warm 60s + ThreadPoolExecutor(max_workers=1) SDK 직렬화
- `_classify_novelty` (0.3/0.7/0.95 임계)
- `_SensitiveFilter` 로그 마스킹
- `_append_capture_log` 0600 jsonl
- 검증: `phases[:7]`, `title[:60]`, `confidence clamp [0,1]`

**Go 포팅 방향**:
- `cmd/rune-mcp/main.go` — MCP 진입점 (stdio JSON-RPC)
- `internal/mcp/tools.go` — 8개 tool 핸들러
- `internal/mcp/lifecycle.go` — state 머신 + Vault retry
- `internal/policy/*` — novelty, 검증 상수
- `internal/obs/sensitive.go` — slog custom handler (로그 마스킹)
- 모델 로드는 **이 프로세스에서 제거**. rune-embedder로 위임

### mcp/adapter/envector_sdk.py (386 LoC) — Go adapter

**역할**:
- pyenvector SDK 래퍼
- **monkey-patch 5개** (Vault-delegated 모델 지원 — `KeyParameter.sec_key` 등)
- AES envelope 생성 (`_app_encrypt_metadata` — `{"a":agent_id, "c":base64(IV||CT)}`)
- `_with_reconnect` 재연결 래퍼 (연결 에러 패턴 10종)
- Score → CipherBlock 추출 → base64 (Vault로 보낼 blob)
- `call_remind` → metadata 조회

**Go 포팅 방향**:
- `internal/adapters/envector/sdk.go` — envector-go SDK 호출 wrapper
- `internal/adapters/envector/aes_ctr.go` — AES envelope 생성 (rune 자체 구현)
- monkey-patch 5개는 **envector-go SDK에 조건 완화 PR로 정식 지원 요청** (spec/components/envector.md 참조)
- `_with_reconnect`는 Go SDK의 `grpc.ClientConn` keepalive로 대체 가능성 → 실측

### mcp/adapter/vault_client.py (380 LoC) — Go adapter

**역할**:
- Rune-Vault gRPC 클라이언트
- `GetPublicKey` · `DecryptScores` · `DecryptMetadata` 3개 RPC
- endpoint 정규화 (`tcp://` / `http(s)://` / bare hostname → `:50051`)
- Health fallback: gRPC 실패 시 http scheme이면 `/mcp`·`/sse` 제거 후 `GET /health`
- Legacy HTTP endpoint 분기 (L70, L93-94, L117-140)

**Go 포팅 방향**:
- `internal/adapters/vault/client.go` — gRPC client + 3 RPC
- `internal/adapters/vault/endpoint.go` — endpoint 파싱·정규화
- `internal/adapters/vault/health.go` — health fallback
- Legacy HTTP 경로는 제거 (결정 필요)

### mcp/adapter/embeddings.py (154 LoC) — rune-embedder로

**역할**:
- `EmbeddingAdapter` (4 backends: sbert / fastembed / huggingface / openai)
- L2 normalize
- Production은 `sbert` + `Qwen/Qwen3-Embedding-0.6B`만 사용

**Go 포팅 방향**:
- **rune-embedder 데몬으로 완전 이관**. rune-mcp에는 없음
- rune-embedder가 ONNX Runtime Go 또는 llama-server 기반으로 동일 기능 제공
- fastembed / huggingface / openai backend는 포팅 안 함 (production 미사용)

### agents/common/config.py (365 LoC) — rune-mcp config loader

**역할**:
- `RuneConfig` dataclass + `save_config()` · `load_config()`
- 7 섹션 필드: vault / envector / embedding / llm / scribe / retriever / state
- 현재 `save_config`가 envector 섹션을 plaintext로 쓰는 drift 존재

**Go 포팅 방향**:
- `internal/adapters/config/loader.go` — **3-섹션만** (`vault` + `state` + `metadata`)
- `envector` / `embedding` / `llm` / `scribe` / `retriever` 섹션 파일 저장 제거
- Vault 번들의 envector 자격증명은 **메모리만**
- metadata는 `map[string]any`로 라운드트립 보존

### agents/common/schemas/ — rune-mcp policy + domain

**embedding.py (56 LoC)**:
- novelty 임계값 상수 (0.4/0.7/0.93 benchmark 또는 0.3/0.7/0.95 runtime)
- → `internal/policy/novelty.go`

**decision_record.py (259 LoC)**:
- DecisionRecord v2.1 enum (Domain 19 / Status 4 / Certainty 3 / ReviewState 4 / Sensitivity 3 / SourceType 7)
- `generate_record_id` (L245-259)
- → `internal/domain/decision_record.go`

**templates.py (363 LoC)**:
- 에이전트 프롬프트 템플릿
- → **MVP scope 밖** · 에이전트 md로 이관 고려

### agents/scribe/* (약 2,500 LoC) — 대부분 삭제

**현 상태**: 원래 3-tier capture 파이프라인 (detector → tier2 LLM filter → llm_extractor → record_builder). 현재 production은 agent-delegated 모드라 **detector/tier2/llm_extractor는 전부 None** (initialize 안 됨).

**Go 포팅 방향**:
- **record_builder.py (703 LoC)**: PII 마스킹 5 regex + quote strip 4 patterns + certainty/status/domain 판정. 이건 **에이전트 md로 이관**이 원래 방향. rune-mcp는 metadata opaque로 받음 (결정 #13)
- **detector, tier2_filter, llm_extractor, pattern_parser, review_queue**: 삭제
- **scribe/server.py (576 LoC)**: 독립 에이전트 서버. MVP scope 밖
- **handlers/ (Slack·Notion 663 LoC)**: 외부 통합. 유지 여부 별도 결정

### agents/retriever/* — rune-mcp service + policy

**query_processor.py (436 LoC)**:
- intent 31 regex (8 intent)
- stop words 81개
- entity 추출 4-stage (quoted → capitalized → tech-name regex)
- time scope 16 patterns
- expanded queries (상위 3 + 원본, lowercase dedup, 최대 5)
- → **rune-mcp `internal/policy/query.go`**

**searcher.py (576 LoC)**:
- recency 공식 `(0.7×raw + 0.3×decay) × status_mul`, half-life 90일
- status multiplier `accepted 1.0 / proposed 0.9 / superseded 0.5 / reverted 0.3`
- phase chain expansion (MVP 유지 — D27)
- group assembly
- → **rune-mcp `internal/policy/rerank.go` + `internal/service/recall.go`**

**synthesizer.py (482 LoC)**:
- EN/KO/JA 3언어 마크다운 템플릿
- 서버 측 LLM 합성 (옵션)
- → **삭제** · 에이전트가 합성 담당

### scripts/bootstrap-mcp.sh (167 LoC) — 삭제

**4단계 self-heal**:
1. `VIRTUAL_ENV` unset
2. Python 버전 불일치 감지·재생성
3. pip shebang 오염 수정
4. fastembed `.incomplete` 캐시 정리 (stale 방어 코드 — production 미사용)

**Go 포팅 방향**: **전면 삭제**. Go 정적 바이너리에는 venv/pip 개념 자체 없음.

### commands/claude/*.md (9 파일) · commands/rune/*.toml (9 파일) — 유지

Claude Code 슬래시 명령 정의. 대부분 호환:
- `configure.md` — vault endpoint/token 입력 로직 유지. 3-섹션 스키마로 갱신
- `activate.md` · `deactivate.md` · `reset.md` — 유지
- `capture.md` · `recall.md` · `history.md` · `delete.md` · `status.md` — 명령 자체 유지, 호출부(Bash exec 또는 mcp tool 호출)만 재작성

## 새 Go 구조 제안 레이아웃

```
rune/ (Go 모듈)
├── go.mod                            # module github.com/envector/rune-go
├── cmd/
│   ├── rune-mcp/main.go              # 세션별 MCP 진입점 (stdio)
│   └── rune-embedder/main.go         # 상주 embedder 진입점 (HTTP)
│
├── internal/
│   ├── mcp/                          # rune-mcp 고유 로직
│   │   ├── server.go                 # stdio JSON-RPC dispatch
│   │   ├── tools.go                  # 8 tool 핸들러
│   │   ├── lifecycle.go              # state 머신 + Vault retry
│   │   └── validate.go               # 입력 검증 (phases/title/confidence)
│   │
│   ├── embedder/                     # rune-embedder 고유 로직
│   │   ├── server.go                 # HTTP /embed /health
│   │   ├── onnx.go                   # ONNX backend (옵션)
│   │   ├── llama.go                  # llama-server backend (옵션)
│   │   └── tokenizer.go              # tokenize (backend 의존)
│   │
│   ├── policy/                       # 수학·상수 (공유, pure)
│   │   ├── novelty.go                # 0.3/0.7/0.95
│   │   ├── rerank.go                 # 0.7×raw + 0.3×decay
│   │   ├── recency.go                # half-life 90일
│   │   ├── query.go                  # 31 intent regex, stop 81
│   │   ├── record_id.go              # dec_YYYY-MM-DD_domain_slug
│   │   └── pii.go                    # PII 마스킹 5 regex (참조)
│   │
│   ├── domain/                       # 타입
│   │   ├── decision_record.go        # v2.1 enum
│   │   ├── capture.go                # Request/Response
│   │   └── query.go                  # Query/SearchResult
│   │
│   ├── adapters/
│   │   ├── vault/                    # Vault gRPC
│   │   ├── envector/                 # envector-go + AES envelope
│   │   ├── config/                   # config.json loader
│   │   └── embedder/                 # rune-embedder HTTP client
│   │
│   └── obs/                          # Observability
│       ├── slog.go                   # _SensitiveFilter 포팅
│       └── metrics.go
│
└── scripts/
    ├── install.sh                    # 바이너리 2개 + launchd unit 설치
    └── export_qwen3_onnx.py          # (ONNX 선택 시) wrapper re-export
```

## 수치 요약

| Python LoC | 새 구조 대응 | 비고 |
|---|---|---|
| ~2,500 (scribe) | 삭제 | 대부분 legacy agent tier |
| ~2,000 (server.py) | rune-mcp 본체 | Go 재작성 |
| ~1,500 (retriever) | rune-mcp service + policy | 로직 이식 |
| ~1,000 (adapters) | adapters 패키지 | wrapping 재작성 |
| ~500 (config + schemas) | policy + domain + config | 축소 |
| ~150 (embeddings.py) | rune-embedder | 별도 데몬 |
| **~7,650 실질 이관 대상** | **~3,000-4,000 Go LoC 예상** | 절반 이하로 축소 (scribe 삭제 + synthesizer 삭제 덕분) |

scribe/synthesizer 삭제만으로 약 2,500 LoC 제거 → **Go 코드베이스는 Python보다 눈에 띄게 작을 것**.

## 참조

- 기존 분석: `docs/migration/python-go-comparison.html` — 이전 "단일 데몬" 방향 기준이라 구조 부분은 참조 시 주의. 정책·코드 실측 데이터는 유효
- 기존 `docs/runed/00-index.md` ~ `07-mcp-cli-layer.md` — 세부 플로우 설명. 단일 데몬 전제라 일부 무효화
