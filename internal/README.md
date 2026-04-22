# `internal/` — rune-mcp Go packages

rune-mcp 본체의 skeleton. **agent-delegated path only**
([docs/v04/overview/architecture.md §Scope](../docs/v04/overview/architecture.md#scope-sot--agent-delegated-only)).

현재 stdlib-only compile 통과 상태. 외부 의존성(MCP SDK, gRPC, envector-go SDK)은
Phase 1에서 추가. 전체 로드맵은 [docs/v04/README.md §구현 로드맵](../docs/v04/README.md#구현-로드맵).

처음 보는 개발자는 먼저 [docs/v04/onboarding.md](../docs/v04/onboarding.md) (10분 안에 첫 PR 위치까지).

---

## 패키지 의존 방향

아래로 갈수록 더 "바깥" — 위 패키지가 아래 패키지를 import한다. **역방향 import 금지**.

```
cmd/rune-mcp          (entry: stdio + lifecycle bootstrap)
      │
      ▼
internal/mcp          (MCP SDK 래핑 · 8 tool handler · state gate · input validation)
      │
      ▼
internal/service      (7-phase orchestration · business logic)
      │
      ▼
internal/policy       internal/adapters       internal/lifecycle       internal/obs
 (pure functions,      (external I/O:          (state machine,          (slog +
  no I/O)              gRPC/SDK/files)         boot retry,              request_id)
      │                     │                  graceful shutdown)
      └─────────┬───────────┘                        │
                ▼                                    ▼
            internal/domain  ←──────────────────────┘
            (pure types, leaf — no imports from other internal packages)
```

**규칙**:
- `internal/domain`은 leaf. 다른 `internal/*` 패키지 import 금지 (stdlib만 허용).
- `internal/policy`는 pure functions. `internal/adapters` 호출 금지, I/O 없음.
- `internal/service`가 adapters + policy를 조합. 여기에만 business orchestration.
- `internal/mcp` handler는 service로 위임. 비즈니스 로직 직접 수행 금지.
- Adapter error는 service 레이어에서 `domain.RuneError`로 wrap (adapter 내부에서 wrap 금지).

---

## 패키지 요약

### `domain/` — core types (types.md §1-6, §3a)

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `schema.go` | 6 enums (Domain 19/Sensitivity 3/Status 4/Certainty 3/ReviewState 4/SourceType 7) + 9 sub-models + DecisionRecord v2.1 + `GenerateRecordID`/`GenerateGroupID` (word-level slug, `unicode.IsLetter`/`IsDigit`) + §7 validation helpers | `agents/common/schemas/decision_record.py` (260 LoC) | [types.md §1-3, §7](../docs/v04/spec/types.md) |
| `extraction.go` | `ExtractionResult` hierarchy (3 structs + `IsMultiPhase`/`IsBundle`) + `ParseExtractionFromAgent` stub | `agents/scribe/llm_extractor.py:L28-70` (**types only** — agent-delegated mode) | [types.md §3a](../docs/v04/spec/types.md#3a-agent-extraction-types-extractionresult-hierarchy) |
| `capture.go` | `CaptureRequest`/`CaptureResponse` + `RawEvent` | `mcp/server/server.py:L698-806, L1208-1407` | [types.md §4.1](../docs/v04/spec/types.md) |
| `query.go` | `QueryIntent` (8) + `TimeScope` (5) + `RecallArgs`/`RecallResult` + `SearchHit` + `ParsedQuery` + `Detection` + `ExtractPayloadText` (D32 strict v2.1) | `agents/retriever/query_processor.py:L22-54` + `searcher.py:L44-76, L472-521` | [types.md §1.7-8, §4.2, §5.1-3](../docs/v04/spec/types.md) |
| `novelty.go` | `NoveltyInfo` + 4 `NoveltyClass` values (inverted score) + `RelatedRecord` | `server.py:L100-108` + `agents/common/schemas/embedding.py:L33-56` | [types.md §5.4](../docs/v04/spec/types.md) |
| `errors.go` | 10 error codes (Python 7 + `EMBEDDER_UNREACHABLE` + `EMPTY_EMBED_TEXT` + `EXTRACTION_MISSING`) + `RuneError` + `MakeError` stub | `mcp/server/errors.py` (118 LoC) | [rune-mcp.md §에러 처리](../docs/v04/spec/components/rune-mcp.md#에러-처리) |
| `logio.go` | `CaptureLogEntry` (D20 bit-identical, `NoveltyScore *float64` for omitempty) | `server.py:L115-138` | [types.md §6](../docs/v04/spec/types.md) |

### `policy/` — pure functions, no I/O

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `novelty.go` | `ClassifyNovelty` + D11 thresholds (`0.3/0.7/0.95`) | `agents/common/schemas/embedding.py:L33-56` | [decisions.md D11](../docs/v04/overview/decisions.md) |
| `rerank.go` | `ApplyRecencyWeighting` + `FilterByTime` + `HalfLifeDays=90` + `SimilarityWeight=0.7` + `RecencyWeight=0.3` + `StatusMultiplier` + `TimeRanges` | `agents/retriever/searcher.py:L273-300, L523-559` | [recall.md Phase 6](../docs/v04/spec/flows/recall.md) |
| `query.go` | `Parse` + `IntentRules` (31 regex / 7 intents, insertion-ordered) + `TimeRules` (16 / 4) + `StopWords` (81) + `TechPatterns` (4) | `agents/retriever/query_processor.py` (L70-417) | [recall.md Phase 2 · decisions.md D21](../docs/v04/spec/flows/recall.md) |
| `record_builder.go` | `BuildPhases` + `MaxInputChars=12_000` + `QuotePatterns` (4) + `RationalePatterns` (5) | `agents/scribe/record_builder.py` (703 LoC) | [capture.md Phase 5 · decisions.md D13/D14](../docs/v04/spec/flows/capture.md) |
| `payload_text.go` | `RenderPayloadText` + 7 `_format_*` helpers (post-insertion · blank collapse) | `agents/common/schemas/templates.py` (364 LoC, **canonical**) | [decisions.md D15](../docs/v04/overview/decisions.md) |
| `pii.go` | `RedactSensitive` + 5 `SENSITIVE_PATTERNS` (email/phone/API key/long hex/credit card) | `agents/scribe/record_builder.py:L89-95 + L406-418` | [capture.md Phase 5](../docs/v04/spec/flows/capture.md) |

### `adapters/` — external I/O wrappers

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `config/loader.go` | `Config` 3-section (vault/state/metadata) + `Load` + `DirPerm=0700`/`FilePerm=0600` | `agents/common/config.py` (365 LoC, **7→3 sections reduced**) | [rune-mcp.md §Config](../docs/v04/spec/components/rune-mcp.md#config-로딩) |
| `vault/client.go` | 3-RPC `Client` interface (GetPublicKey/DecryptScores/DecryptMetadata) + `MaxMessageLength=256MB` + `ValidateAgentDEK` (Python missing — Go fail-fast) | `mcp/adapter/vault_client.py` (381 LoC) | [vault.md](../docs/v04/spec/components/vault.md) |
| `vault/endpoint.go` | `NormalizeEndpoint` (4-form: `tcp://`, `http(s)://`, `host:port`, `host`) | `vault_client.py:L116-140 _derive_grpc_target` | [vault.md §Endpoint](../docs/v04/spec/components/vault.md#endpoint-파싱정규화) |
| `vault/health.go` | `HealthFallback` Tier 2 HTTP `/health` (strip `/mcp` `/sse`) | `vault_client.py:L322-337` | [vault.md §Health 2-tier](../docs/v04/spec/components/vault.md#health-check-2-tier) |
| `vault/errors.go` | 5 sentinels (`ErrVaultUnavailable` 등) + `ErrNotHTTPScheme` + `MapGRPCError` | — (Go-specific typed errors) | [vault.md §에러 분류](../docs/v04/spec/components/vault.md#에러-분류) |
| `envector/client.go` | SDK `Client` interface (Insert/Score/GetMetadata/ActivateKeys/GetIndexList) + `InsertRequest`/`MetadataRef`/`MetadataEntry` | `mcp/adapter/envector_sdk.py` (387 LoC) | [envector.md](../docs/v04/spec/components/envector.md) |
| `envector/aes_ctr.go` | `Seal`/`Open` (AES-256-CTR envelope `{"a","c"}`, 16B IV, no MAC — Q1 deferred) | `envector_sdk.py:L227-234` + `pyenvector/utils/aes.py:L52-58` | [rune-mcp.md §AES envelope](../docs/v04/spec/components/rune-mcp.md#aes-envelope-capture는-rune-mcp-recall은-vault-복호화) |
| `envector/errors.go` | 4 sentinels (ConnectionLost/KeyActivationConflict/DecryptorUnavailable/InsertInconsistent) + `MapSDKError` (Python 11 string patterns NOT ported — intentional) | — (adapter-level typed errors) | [envector.md §에러 처리](../docs/v04/spec/components/envector.md#에러-처리) |
| `embedder/client.go` | gRPC `Client` interface (EmbedSingle/EmbedBatch/Info/Health) + `RetryBackoffs [0, 500ms, 2s]` (D7) | — (D30 external daemon) | [embedder.md](../docs/v04/spec/components/embedder.md) |
| `embedder/info_cache.go` | `infoCache` with `sync.Once` + `Get`/`Snapshot` | — | [embedder.md §Info 캐시](../docs/v04/spec/components/embedder.md) |
| `embedder/retry.go` | `retry[R any]` generic helper + `retryable` (gRPC codes Unavailable/DeadlineExceeded/ResourceExhausted) | — | [embedder.md §Retry 정책](../docs/v04/spec/components/embedder.md) |
| `logio/capture_log.go` | `Append` (sync.Mutex + `flock(LOCK_EX)` + O_APPEND) + `Tail` (reverse jsonl reader) | `server.py:L115-168` | [rune-mcp.md §Capture log](../docs/v04/spec/components/rune-mcp.md#capture-log) |

### `service/` — 7-phase orchestration (business logic)

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `capture.go` | `CaptureService.Handle` (Phase 2-7) + `Batch` + `BatchCaptureArgs`/`Result` + `runNoveltyCheck` + `sealMetadata` | `server.py:L1208-1407 _capture_single` + `L810-896 tool_batch_capture` | [capture.md](../docs/v04/spec/flows/capture.md) |
| `recall.go` | `RecallService.Handle` (7-phase) + `searchWithExpansions`/`searchSingle`/`resolveMetadata`/`classifyMetadata`/`toSearchHit`/`expandPhaseChains`/`assembleGroups`/`applyMetadataFilters`/`buildResult`/`calculateConfidence` | `server.py:L910-1034` + `agents/retriever/searcher.py` (576 LoC) | [recall.md](../docs/v04/spec/flows/recall.md) |
| `lifecycle.go` | `LifecycleService.{VaultStatus,Diagnostics,CaptureHistory,DeleteCapture,ReloadPipelines}` + 8 result types + `collectEnvector` (5s timeout) + `warmupEnvector` (60s) | `server.py:L496-528, L540-684, L1046-1089, L1092-1111, L1123-1206` | [lifecycle.md](../docs/v04/spec/flows/lifecycle.md) |
| `search.go` | `SearchByID` — `"ID: {id}"` hack (recall + delete_capture 공유) | `agents/retriever/searcher.py:L561-567` | [lifecycle.md §5](../docs/v04/spec/flows/lifecycle.md) |
| `diagnostics_classify.go` | `ClassifyEnvectorError` (gRPC code-based, not Python string patterns) + `EnvectorErrorType` enum (5 values) | `server.py:L655-672` (Python string patterns → Go code-based, intentional) | [lifecycle.md §2](../docs/v04/spec/flows/lifecycle.md) |

### `mcp/` — MCP SDK wiring

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `tools.go` | 8 tool handler stubs (Capture/Recall/BatchCapture/CaptureHistory/DeleteCapture/VaultStatus/Diagnostics/ReloadPipelines) + `Deps` struct + `Register` | `mcp/server/server.py` tool handlers | [rune-mcp.md §MCP Tools](../docs/v04/spec/components/rune-mcp.md#mcp-tools-8개) |
| `state.go` | `CheckState` (state-specific recovery hints) + `ValidateCaptureRequest`/`ValidateRecallArgs` + `TruncateTitle` (D3 60-rune) + `ClampConfidence` | `server.py:L1503-1518 _ensure_pipelines` + L1240-1242 validation | [rune-mcp.md §에러 처리](../docs/v04/spec/components/rune-mcp.md#에러-처리) |

### `lifecycle/` — process lifecycle

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `boot.go` | `State` enum (4 values, atomic) + `Manager` + `BootBackoffs` (1s → 60s cap) + `RunBootLoop` | `server.py main()` + `_init_pipelines` | [rune-mcp.md §부팅 시퀀스](../docs/v04/spec/components/rune-mcp.md#부팅-시퀀스) |
| `shutdown.go` | `GracefulShutdown` 3-step (drain inflight 30s + adapter Close + DEK zeroize) + `InflightTracker` + `ZeroizeDEK` | (Python signal handling) | [rune-mcp.md §프로세스 수명](../docs/v04/spec/components/rune-mcp.md#프로세스-수명) |

### `obs/` — observability

| 파일 | 담당 | Python 원본 | spec |
|---|---|---|---|
| `slog.go` | `SensitiveFilter` 2 regex placeholder (`sk-\|pk-\|api_\|...` + `token\|key\|secret\|password`) + `request_id` context (WithRequestID/RequestID/NewRequestID) | `server.py:L25-40 _SensitiveFilter` | [rune-mcp.md §Observability](../docs/v04/spec/components/rune-mcp.md#observability) |

---

## 각 Phase별 작업 위치

각 Phase 상세는 [docs/v04/README.md §구현 로드맵](../docs/v04/README.md#구현-로드맵).

| Phase | 주요 작업 파일 |
|---|---|
| **1** (external deps) | `go.mod` + 각 adapter 파일의 import 추가 |
| **2** (pure logic) | `internal/domain/*` + `internal/policy/*` TODO → 구현 |
| **3** (canonical port) | `internal/policy/{record_builder,payload_text,pii}.go` |
| **4** (adapters) | `internal/adapters/{vault,envector,embedder,logio,config}/*` |
| **5** (service) | `internal/service/{capture,recall,lifecycle,search,diagnostics_classify}.go` |
| **6** (MCP) | `internal/mcp/tools.go` + `cmd/rune-mcp/main.go` |
| **7** (test) | 각 패키지의 `*_test.go` + `testdata/` golden fixtures |

---

## 작업 규칙

1. **Python이 canonical** — 모든 bit-identical 포팅 대상 함수 위에 Python `file.py:L<n>` 참조 주석이 이미 있음. 의심되면 Python 파일 열어서 확인.
2. **결정 재정의 금지** — D1-D32는 [overview/decisions.md](../docs/v04/overview/decisions.md)의 단일 진실 소스. 변경 필요하면 PR 전에 `D<N>` 신규 항목 추가 → 리뷰 승인 → 코드 변경.
3. **빌드 baseline** — `go build ./...` 통과가 항상 baseline. 새 파일 추가 시 반드시 확인.
4. **TODO 주석 수명** — `// TODO: bit-identical to ...` 가이드는 실제 구현으로 대체되면 삭제. 남은 TODO는 "아직 구현 안 됨"을 의미.
5. **의존성 방향 준수** — 위 §패키지 의존 방향 위반 금지. 필요 시 리뷰에서 보강 제안.
6. **bit-identical 검증은 golden fixture** — Python에서 생성한 JSON/MD를 `testdata/golden/` 에 두고 byte-by-byte 비교.

---

## 관련 문서

- [docs/v04/onboarding.md](../docs/v04/onboarding.md) — **처음 보는 개발자**: 10분 안에 첫 PR 위치까지
- [docs/v04/README.md](../docs/v04/README.md) — 전체 개요 + 구현 로드맵
- [docs/v04/overview/architecture.md](../docs/v04/overview/architecture.md) — 3-프로세스 모델 + Scope SOT
- [docs/v04/overview/decisions.md](../docs/v04/overview/decisions.md) — D1-D32 결정 트래커
- [docs/v04/overview/open-questions.md](../docs/v04/overview/open-questions.md) — 미결 Q1 (AES-MAC), Q4 (envector-go SDK PR)
- [docs/v04/spec/types.md](../docs/v04/spec/types.md) — 모든 도메인 타입의 단일 진실 소스
- [docs/v04/spec/flows/](../docs/v04/spec/flows/) — capture/recall/lifecycle 7-phase
- [docs/v04/spec/components/](../docs/v04/spec/components/) — rune-mcp/vault/envector/embedder
- [docs/v04/spec/python-mapping.md](../docs/v04/spec/python-mapping.md) — Python → Go 파일/LoC 매핑
- [docs/v04/notes/](../docs/v04/notes/) — 검증 로그 (verification-matrix · implementability-report · python-parity-final)
