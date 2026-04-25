# Flow × Package Matrix — 공통 모듈 분리 작업용

**작성 목적**: Python→Go 포팅 팀 분업을 위해 rune-mcp가 완성됐을 때 돌아갈 **모든 end-to-end 플로우**를 식별하고, 복수 플로우가 공유하는 **공통 모듈**을 분리한다. 구현 전 설계 단계 산출물.

**전제**:
- Scope는 agent-delegated path only ([overview/architecture.md §Scope](../overview/architecture.md#scope-sot--agent-delegated-only))
- Skeleton 커밋 `2eb167d` 시점 파일 37개 기준
- Policy(`internal/policy/`)와 Domain 타입(`internal/domain/`) 포팅은 타 담당자 scope — 이 문서는 그 영역의 내부는 건드리지 않고 "어디서 호출하는지"만 표시한다.

---

## 0. 한 페이지 요약 (TL;DR)

> 이 문서가 길어서 처음 보면 부담스럽다. **우선 이 섹션만 읽으면 누가 뭘 해야 하는지와 내가 어디를 봐야 하는지 30초 안에 잡힌다**. 상세는 §1 이후.

### 0-a. 결론 4줄

- **플로우 10개** — MCP tool 8 (capture · batch_capture · recall · capture_history · delete_capture · vault_status · diagnostics · reload_pipelines) + infra 2 (boot · shutdown)
- **Tier S 공통 모듈 11개** — 3개 이상 플로우가 공유. 인터페이스 변경 비용이 제일 커서 선행 확정 필요
- **팀원 scope 제외** — `internal/policy/` 6파일 · `internal/domain/` 7파일은 이 문서에서 호출 사실만 표시하고 내부는 건드리지 않음
- **지금 당장 블로커 없이 시작 가능** — `obs/slog` · `lifecycle/boot` State machine · `mcp.Deps` 필드 확정 · `mcp/state` 테스트 (전부 stdlib-only, policy/domain 작업과 완전 병렬)

### 0-b. 10개 플로우 × 외부 I/O — 한눈 요약

| 플로우 | Vault | Envector | Embedder | AES Seal | Log 쓰기 | Log 읽기 | State gate |
|---|---|---|---|---|---|---|---|
| F1 BOOT | GetPublicKey | ActivateKeys | — | — | — | — | **set** |
| F2 SHUT | Close | Close | Close | — | — | — | — |
| T1 CAP | DecryptScores | Score · Insert | EmbedSingle+Batch | ● | ● | — | ● |
| T2 BAT | DecryptScores | Score · Insert | EmbedSingle+Batch | ● | ● | — | ● |
| T3 REC | DecryptScores+Meta | Score · GetMetadata | EmbedBatch | — | — | — | ● |
| T4 HIS | — | — | — | — | — | ● | — |
| T5 DEL | DecryptScores+Meta | Score · Insert | EmbedSingle | ● | ● | — | ● |
| T6 VST | HealthCheck | — | — | — | — | — | — |
| T7 DIA | HealthCheck + Key | GetIndexList(5s) | Info | — | — | — | — |
| T8 REL | GetPublicKey | GetIndexList(60s) | — | — | — | — | **reset** |

> **패턴 1** — write tool 3개(CAP/BAT/DEL) + BOOT는 Vault·Envector·Embedder 전부 필요 → 이 3개 adapter 확정 전에는 write 플로우 시작 불가
> **패턴 2** — HIS는 네트워크 I/O 0 (완전 독립), VST는 Vault 단독 → 경량, 병렬 개발 자유도 최상
> **패턴 3** — AES Seal은 write tool 전용 (recall의 metadata decrypt는 Vault가 대행)

### 0-c. 핵심 인사이트 4개

1. **T5 DEL ≈ T3 REC의 `searchSingle` + T1 CAP의 `Seal`·`Insert` 합성** — REC 완성 후에 DEL 착수하는 게 자연스럽고 싸다
2. **T8 REL ≈ F1 BOOT 재실행** — `lifecycle.RunBootLoop`을 `AwaitInitDone` + `ReinitPipelines`로 감싸면 코드 중복 없이 처리 가능
3. **T7 DIA는 orchestration뿐, 실제 진단은 adapter가 담당** — adapter 완성도가 DIA의 가치를 그대로 결정
4. **BOOT 진입점 교체 예정** — Python `scripts/bootstrap-mcp.sh`는 삭제 ([python-mapping.md:154](../spec/python-mapping.md)) → `.claude-plugin/plugin.json`이 `rune-mcp` 바이너리를 직접 실행 (cutover 시점)

### 0-d. 역할별 읽기 가이드

| 당신이 … | 먼저 볼 섹션 | 건너뛸 섹션 |
|---|---|---|
| Infra / Adapter 담당 (나) | §4 Tier S → §5-b 우선순위 → §6 체크리스트 | §2 T별 상세 (필요 시 lookup) |
| T1·T2 (Capture/Batch) 담당 | §2 T1·T2 → §3-b/3-c 매트릭스 CAP·BAT 열 → §4 Tier S | §2 T3 이후 |
| T3 (Recall) 담당 | §2 T3 → §3-b/3-c REC 열 → §5-c (T5와의 접점) | §2 T4 이후 |
| T5 (Delete) 담당 | §2 T5 (searchSingle 재사용 주목) → §3-b REC·DEL 열 비교 | §2 T6 이후 |
| T4/T6/T7/T8 (lifecycle tools) 담당 | §2 해당 플로우 → §3-b `service/lifecycle.go` 행 | §2 CAP/REC |
| Policy / Domain 담당 (현 팀원) | §1 인벤토리 · §5-a 만 — 호출 의도 확인용 | 나머지 전부 |
| PM · 리뷰어 | §1 인벤토리 → §5-c 오너 분배 → §7 미해결 항목 | §3 매트릭스 전부 |

### 0-e. Tier S 공통 모듈 11개 — 한 줄 요약

| # | 모듈 | 사용 플로우 | 왜 Tier S? |
|---|---|---|---|
| 1 | `internal/mcp/tools.go::Deps` 구조체 | 10/10 | 모든 핸들러 주입 대상 |
| 2 | `internal/obs/slog.go` | 10/10 | request_id · SensitiveFilter 전역 |
| 3 | `internal/domain/errors.go` RuneError·MakeError **(TM)** | 10/10 | 모든 에러 wrap 경로 |
| 4 | `internal/lifecycle/boot.go` State·Manager·RunBootLoop | 10/10 | 모든 tool이 state 조회 |
| 5 | `internal/adapters/vault/client.go` | 8/10 | BOOT·CAP·BAT·REC·DEL·VST·DIA·REL·SHUT (마지막은 `Close()`) |
| 6 | `adapters/vault/errors.go::MapGRPCError` | 8/10 | 모든 Vault 호출 경로 + SHUT Close 에러 매핑 |
| 7 | `internal/adapters/envector/client.go` | 8/10 | BOOT·CAP·BAT·REC·DEL·DIA·REL·SHUT |
| 8 | `adapters/envector/errors.go::MapSDKError` | 8/10 | 동일 목적 (SHUT Close 포함) |
| 9 | `internal/adapters/embedder/{client,info_cache,retry}.go` | 6/10 | CAP·BAT·REC·DEL·DIA·SHUT |
| 10 | `adapters/envector/aes_ctr.go::Seal` | 3/10 | write tool 3개 (CAP·BAT·DEL) |
| 11 | `adapters/logio/capture_log.go::Append` | 3/10 | write tool 3개 (CAP·BAT·DEL) |

> **TM** = teammate scope. 타입 시그니처는 팀원 확정에 의존하지만, 내 구현 코드의 거의 모든 경로가 이 에러 타입을 참조한다.

---

## 1. 플로우 인벤토리 (10개 — 8 tool + 2 infra)

| # | 코드 | 이름 | 종류 | 진입점 | 오케스트레이션 파일 |
|---|---|---|---|---|---|
| F1 | BOOT | Boot sequence | infra | `cmd/rune-mcp/main.go` | `lifecycle/boot.go` |
| F2 | SHUT | Graceful shutdown | infra | signal/EOF | `lifecycle/shutdown.go` |
| T1 | CAP | `rune_capture` | write tool | `mcp.ToolCapture` | `service/capture.go` |
| T2 | BAT | `rune_batch_capture` | write tool | `mcp.ToolBatchCapture` | `service/capture.go` (Batch) |
| T3 | REC | `rune_recall` | read tool | `mcp.ToolRecall` | `service/recall.go` |
| T4 | HIS | `rune_capture_history` | read tool | `mcp.ToolCaptureHistory` | `service/lifecycle.go` |
| T5 | DEL | `rune_delete_capture` | write tool (soft) | `mcp.ToolDeleteCapture` | `service/lifecycle.go` + `service/search.go` |
| T6 | VST | `rune_vault_status` | read tool | `mcp.ToolVaultStatus` | `service/lifecycle.go` |
| T7 | DIA | `rune_diagnostics` | read tool | `mcp.ToolDiagnostics` | `service/lifecycle.go` + `service/diagnostics_classify.go` |
| T8 | REL | `rune_reload_pipelines` | mutation (state) | `mcp.ToolReloadPipelines` | `service/lifecycle.go` |

> 출처: `internal/mcp/tools.go` (8 tool 스텁), `docs/v04/spec/flows/{capture,recall,lifecycle}.md`.

---

## 2. 플로우별 요약 (Phase 개요 + 주요 의존성)

### F1. BOOT — Boot sequence

- **Phase**: Config.Load → state=starting → Vault.GetPublicKey (backoff [1s..60s]) → envector.ActivateKeys → state=active → mcp.Register → stdio serve
- **Disk I/O**: `~/.rune/config.json` (read), `~/.rune/keys/<key_id>/{EncKey,EvalKey}.json` (cache write)
- **gRPC I/O**: Vault.GetPublicKey, envector.ActivateKeys
- **상태 전이**: starting → waiting_for_vault → active (or dormant on fatal)
- **Spec**: `spec/components/rune-mcp.md §부팅 시퀀스`, `architecture.md §부팅`

### F2. SHUT — Graceful shutdown

- **Phase**: stdin EOF 또는 SIGTERM/SIGINT → inflight drain (30s) → adapter Close() × 3 (Vault/Envector/Embedder) → DEK zeroize → exit
- **Disk I/O**: none (capture_log는 per-append flush라 fd 유지 안 함)
- **의존**: `lifecycle.InflightTracker`, `lifecycle.ZeroizeDEK`, 각 adapter의 `Close()`
- **Spec**: `spec/components/rune-mcp.md §프로세스 수명`

### T1. CAP — `rune_capture` (7-phase)

- **P1** state gate → `mcp.CheckState`
- **P2** validate + tier2 reject → `mcp.ValidateCaptureRequest`, agent extracted.tier2.capture 확인
- **P3** embed single → `embedder.EmbedSingle(text_to_embed)`
- **P4** novelty check → `envector.Score` → `vault.DecryptScores(topk=3)` → `policy.ClassifyNovelty` (D11 0.3/0.7/0.95) → near_duplicate면 조기종료
- **P5a** record build → `policy.BuildPhases`, `policy.RedactSensitive`, `policy.RenderPayloadText`, `domain.GenerateRecordID`
- **P5b** batch embed + seal → `embedder.EmbedBatch` + `envector.Seal` (AES-256-CTR)
- **P6** insert → `envector.Insert` (atomic batch D17)
- **P7** log append → `logio.Append` (best-effort D19)
- **Response**: `domain.CaptureResponse`

### T2. BAT — `rune_batch_capture`

- 입력 JSON array 파싱 → 각 item에 대해 `captureSingle` 재사용 → 상태 분류 (captured/skipped/near_duplicate/error) → aggregate
- CAP 플로우를 N회 호출 + summary assemble. **고유 로직**은 item parse + summarize뿐, 나머지 전부 CAP과 공용
- Python: `server.py:L810-896 tool_batch_capture`

### T3. REC — `rune_recall` (7-phase)

- **P1** state gate + topk∈[1,10] → `mcp.CheckState`, `mcp.ValidateRecallArgs`
- **P2** query parse → `policy.Parse` (31 intent / 16 time / 81 stopwords / 4 tech regex, D21 English-only)
- **P3** batch embed expansions[:3] → `embedder.EmbedBatch` (D22/D23)
- **P4** per-expansion 4-RPC 순차 (D25): `envector.Score` → `vault.DecryptScores` → `envector.GetMetadata` → `vault.DecryptMetadata` (batch + per-entry fallback D26)
- **P5** metadata 3-way classify (AES envelope / plain JSON / legacy base64) → `service/recall.classifyMetadata` → `domain.ExtractPayloadText` (D32 strict v2.1)
- **P6** phase-chain expand (D27, 최대 2 group) + group assemble + filter + `policy.ApplyRecencyWeighting` + `policy.FilterByTime` + topk
- **P7** confidence calc + sources + synthesized=false (D28)

### T4. HIS — `rune_capture_history`

- **state gate 없음** (read-only)
- **Phase**: args normalize (limit default 20, max 100) → `logio.Tail(path, limit, domain, since)` (reverse JSONL) → return
- **Disk I/O only**. 네트워크 I/O 없음
- 파일 없으면 empty entries 반환 (degrade)

### T5. DEL — `rune_delete_capture` (soft-delete)

- **P1** search by id → `service/search.SearchByID` = `embedder.EmbedSingle("ID: {id}")` + `recall.searchSingle` 재사용 + id 매칭 필터
- **P2** metadata["status"]="reverted"
- **P3** re-embed + seal → `embedder.EmbedSingle` + `envector.Seal`
- **P4** re-insert → `envector.Insert` (실패 시 `state.SetDormant("envector_unreachable")`)
- **P5** log append action="deleted" → `logio.Append`
- Vault/envector 오류 경로에서 **상태를 dormant로 전이** — CAP과 달리 fatal

### T6. VST — `rune_vault_status`

- **state gate 없음** (진단용)
- Config 로드 여부 확인 → `vault.HealthCheck` (2-tier: gRPC `/grpc.health.v1.Health` → HTTP GET `/health` fallback)
- `service/lifecycle.VaultStatus` 단일 함수. 5개 필드 응답

### T7. DIA — `rune_diagnostics`

- **state gate 없음**
- 7 subsystem 병렬/순차 수집:
  1. Environment (OS/Go runtime/cwd) — stdlib
  2. State (lifecycle.Manager.Current + dormant_reason/since)
  3. Vault (configured + `vault.HealthCheck` 2-tier + endpoint)
  4. Keys (enc_key_loaded, key_id, agent_dek_loaded)
  5. Pipelines (scribe/retriever init, LLM provider always empty)
  6. Embedding (`embedder.Info()` via sync.Once cache)
  7. Envector (`envector.GetIndexList` with **5s timeout** via `context.WithTimeout` + goroutine + select; 실패 시 `service.ClassifyEnvectorError` → `EnvectorErrorType`)
- 최상위 `ok=false` 조건: vault unhealthy **또는** enc_key_loaded=false

### T8. REL — `rune_reload_pipelines`

- `state.AwaitInitDone()` → race 방지
- `state.ClearError()` + `state.ReinitPipelines(ctx)` = 현재 boot loop 로직 재실행
- 성공 시 `envector.GetIndexList` 로 **60s warmup** (RegisterKey pre-resolve)
- 응답에 errors[], envector_warmup 포함. **상태 변경 가능** (reload 후 active로 전이)

---

## 3. 플로우 × 파일 매트릭스

> 표시: `●` = 직접 호출, `◐` = 간접/공유 helper 경유, `-` = 미사용
>
> 열 순서는 파일 경로 기준. TEAMMATE 스코프 파일은 회색 배경 대신 "TM" 표시 (본 문서는 그 파일들을 건드리지 않음).

### 3-a. Infra + MCP 레이어

| 파일 | F1 BOOT | F2 SHUT | T1 CAP | T2 BAT | T3 REC | T4 HIS | T5 DEL | T6 VST | T7 DIA | T8 REL |
|---|---|---|---|---|---|---|---|---|---|---|
| `cmd/rune-mcp/main.go` | ● | ● | - | - | - | - | - | - | - | - |
| `internal/mcp/tools.go` (Deps · Register · 8 handler 외피) | ● | - | ● | ● | ● | ● | ● | ● | ● | ● |
| `internal/mcp/state.go::CheckState` | - | - | ● | ● | ● | - | ● | - | - | - |
| `internal/mcp/state.go::ValidateCaptureRequest` | - | - | ● | ● | - | - | - | - | - | - |
| `internal/mcp/state.go::ValidateRecallArgs` | - | - | - | - | ● | - | - | - | - | - |
| `internal/mcp/state.go::TruncateTitle` | - | - | ● | ● | - | - | - | - | - | - |
| `internal/mcp/state.go::ClampConfidence` | - | - | ● | ● | - | - | - | - | - | - |
| `internal/lifecycle/boot.go` | ● | - | ◐(state 조회) | ◐ | ◐ | - | ◐ | ◐ | ● | ● |
| `internal/lifecycle/shutdown.go` | - | ● | - | - | - | - | - | - | - | - |
| `internal/obs/slog.go` (WithRequestID · SensitiveFilter) | ● | ● | ● | ● | ● | ● | ● | ● | ● | ● |

### 3-b. Service 레이어 (오케스트레이션)

| 파일 | F1 | F2 | T1 CAP | T2 BAT | T3 REC | T4 HIS | T5 DEL | T6 VST | T7 DIA | T8 REL |
|---|---|---|---|---|---|---|---|---|---|---|
| `internal/service/capture.go::CaptureService.Handle` | - | - | ● | ◐(via Batch) | - | - | - | - | - | - |
| `internal/service/capture.go::Batch` | - | - | - | ● | - | - | - | - | - | - |
| `internal/service/recall.go::RecallService.Handle` | - | - | - | - | ● | - | - | - | - | - |
| `internal/service/recall.go::searchSingle` | - | - | - | - | ● | - | ◐(via SearchByID) | - | - | - |
| `internal/service/recall.go::classifyMetadata` | - | - | - | - | ● | - | ◐(via SearchByID) | - | - | - |
| `internal/service/recall.go::expandPhaseChains` | - | - | - | - | ● | - | - | - | - | - |
| `internal/service/search.go::SearchByID` | - | - | - | - | - | - | ● | - | - | - |
| `internal/service/lifecycle.go::VaultStatus` | - | - | - | - | - | - | - | ● | - | - |
| `internal/service/lifecycle.go::Diagnostics` | - | - | - | - | - | - | - | - | ● | - |
| `internal/service/lifecycle.go::collectEnvector` | - | - | - | - | - | - | - | - | ● | - |
| `internal/service/lifecycle.go::CaptureHistory` | - | - | - | - | - | ● | - | - | - | - |
| `internal/service/lifecycle.go::DeleteCapture` | - | - | - | - | - | - | ● | - | - | - |
| `internal/service/lifecycle.go::ReloadPipelines` | - | - | - | - | - | - | - | - | - | ● |
| `internal/service/lifecycle.go::warmupEnvector` | - | - | - | - | - | - | - | - | - | ● |
| `internal/service/diagnostics_classify.go::ClassifyEnvectorError` | - | - | - | - | - | - | - | - | ● | - |

### 3-c. Adapter 레이어 (외부 I/O)

| 파일 | F1 | F2 | T1 CAP | T2 BAT | T3 REC | T4 HIS | T5 DEL | T6 VST | T7 DIA | T8 REL |
|---|---|---|---|---|---|---|---|---|---|---|
| `adapters/config/loader.go::Load` | ● | - | - | - | - | - | - | - | - | ● |
| `adapters/config/loader.go::EnsureDirectories` · perm consts | ● | - | - | - | - | - | - | - | - | - |
| `adapters/vault/client.go::GetPublicKey` | ● | - | - | - | - | - | - | - | - | ● |
| `adapters/vault/client.go::DecryptScores` | - | - | ● | ● | ● | - | ● | - | - | - |
| `adapters/vault/client.go::DecryptMetadata` | - | - | - | - | ● | - | ● | - | - | - |
| `adapters/vault/client.go::HealthCheck` | - | - | - | - | - | - | - | ● | ● | - |
| `adapters/vault/client.go::Close` | - | ● | - | - | - | - | - | - | - | - |
| `adapters/vault/endpoint.go::NormalizeEndpoint` | ● | - | - | - | - | - | - | - | - | - |
| `adapters/vault/health.go::HealthFallback` | - | - | - | - | - | - | - | ● | ● | - |
| `adapters/vault/errors.go::MapGRPCError` | ● | - | ● | ● | ● | - | ● | ● | ● | ● |
| `adapters/envector/client.go::Insert` | - | - | ● | ● | - | - | ● | - | - | - |
| `adapters/envector/client.go::Score` | - | - | ● | ● | ● | - | ● | - | - | - |
| `adapters/envector/client.go::GetMetadata` | - | - | - | - | ● | - | ● | - | - | - |
| `adapters/envector/client.go::ActivateKeys` | ● | - | - | - | - | - | - | - | - | ● |
| `adapters/envector/client.go::GetIndexList` | - | - | - | - | - | - | - | - | ● | ● |
| `adapters/envector/client.go::Close` | - | ● | - | - | - | - | - | - | - | - |
| `adapters/envector/aes_ctr.go::Seal` | - | - | ● | ● | - | - | ● | - | - | - |
| `adapters/envector/errors.go::MapSDKError` | ● | - | ● | ● | ● | - | ● | - | ● | ● |
| `adapters/embedder/client.go::EmbedSingle` | - | - | ● | ● | ◐(P4 fallback) | - | ● | - | - | - |
| `adapters/embedder/client.go::EmbedBatch` | - | - | ● | ● | ● | - | - | - | - | - |
| `adapters/embedder/client.go::Info` | - | - | ● | ● | ● | - | ● | - | ● | - |
| `adapters/embedder/client.go::Health` | - | - | - | - | - | - | - | - | ● | - |
| `adapters/embedder/client.go::Close` | - | ● | - | - | - | - | - | - | - | - |
| `adapters/embedder/info_cache.go::sync.Once` | - | - | ● | ● | ● | - | ● | - | ● | - |
| `adapters/embedder/retry.go::retry[R]` | - | - | ● | ● | ● | - | ● | - | - | - |
| `adapters/logio/capture_log.go::Append` | - | - | ● | ● | - | - | ● | - | - | - |
| `adapters/logio/capture_log.go::Tail` | - | - | - | - | - | ● | - | - | - | - |

> `adapters/envector/aes_ctr.go::Open`은 skeleton에 존재하지만 MVP에서는 어느 플로우도 호출하지 않음 — Vault가 `DecryptMetadata`로 복호화 대행 (Q1 AES-MAC 해소 이후 재평가). 표에서는 생략.

### 3-d. Policy / Domain (TM = TEAMMATE scope — 본 매트릭스는 호출 사실만 기록)

| 파일 | F1 | F2 | T1 | T2 | T3 | T4 | T5 | T6 | T7 | T8 |
|---|---|---|---|---|---|---|---|---|---|---|
| `domain/schema.go` (TM: DecisionRecord, enums, GenerateRecordID) | - | - | ● | ● | ● | - | ● | - | - | - |
| `domain/extraction.go` (TM: ExtractionResult) | - | - | ● | ● | - | - | - | - | - | - |
| `domain/capture.go` (TM: CaptureRequest/Response) | - | - | ● | ● | - | - | - | - | - | - |
| `domain/query.go` (TM: RecallArgs/Result, ExtractPayloadText D32) | - | - | - | - | ● | - | ● | - | - | - |
| `domain/novelty.go` (TM: NoveltyInfo, class) | - | - | ● | ● | - | - | - | - | - | - |
| `domain/errors.go` (TM: RuneError, 10 codes, MakeError) | ● | - | ● | ● | ● | ● | ● | ● | ● | ● |
| `domain/logio.go` (TM: CaptureLogEntry) | - | - | ● | ● | - | ● | ● | - | - | - |
| `policy/record_builder.go` (TM) | - | - | ● | ● | - | - | - | - | - | - |
| `policy/payload_text.go` (TM) | - | - | ● | ● | ◐(ExtractPayloadText 참조) | - | - | - | - | - |
| `policy/pii.go` (TM) | - | - | ● | ● | - | - | - | - | - | - |
| `policy/novelty.go` (TM: ClassifyNovelty) | - | - | ● | ● | - | - | - | - | - | - |
| `policy/query.go` (TM: Parse + 31/16/81/4 regex) | - | - | - | - | ● | - | - | - | - | - |
| `policy/rerank.go` (TM: ApplyRecencyWeighting · FilterByTime) | - | - | - | - | ● | - | - | - | - | - |

---

## 4. 공통 모듈 분류 (TEAMMATE 제외, 내 조사·구현 대상)

**기준**:
- **Tier S** — 3+ 플로우에서 직접 호출. 반드시 선행 개발. 인터페이스 변경이 가장 비싸다.
- **Tier A** — 2 플로우에서 공유. 함께 구현하면 효율.
- **Tier B** — 1 플로우 고유. 해당 플로우 담당자가 함께 개발.

### 4-a. Tier S — 전역 공통 인프라 (선행 개발 최우선)

| 파일 | 사용처 (플로우 수) | 비고 |
|---|---|---|
| `internal/mcp/tools.go::Deps` 구조체 확정 | **10/10** (모든 MCP 핸들러 + main) | 현재 스켈레톤에 필드 전부 주석 처리. 이거 먼저 확정해야 다른 팀원이 service 작성 가능 |
| `internal/obs/slog.go` (WithRequestID · SensitiveFilter · NewHandler) | **10/10** | 모든 핸들러 진입에서 request_id 할당, 모든 slog 호출에 필터 |
| `internal/domain/errors.go` (RuneError · MakeError · 10 codes) | **10/10** | TM scope이지만 모든 플로우가 의존. Deps 확정과 함께 이 타입이 고정돼야 adapter 에러 wrap 코드가 나옴 |
| `internal/adapters/vault/client.go` (5 RPC + Close) | **8** (BOOT, CAP, BAT, REC, DEL, VST, DIA, REL, SHUT) | EvalKey 256MB gRPC opt, keepalive 30s, Bearer auth metadata. SHUT은 Close()만 |
| `internal/adapters/envector/client.go` (6 RPC + Close) | **8** (BOOT, CAP, BAT, REC, DEL, DIA, REL, SHUT) | SDK 래퍼. Q4 PR 전에는 mock 대체 |
| `internal/adapters/embedder/{client,info_cache,retry}.go` | **6** (CAP, BAT, REC, DEL, DIA, SHUT) | D7 retry, sync.Once Info, batch auto-split. SHUT은 Close()만 |
| `internal/adapters/envector/aes_ctr.go::Seal` | **3** (CAP, BAT, DEL) | AES-256-CTR `{"a","c"}` envelope. 16B IV. MAC 없음 (Q1 Deferred) |
| `internal/adapters/logio/capture_log.go::Append` | **3** (CAP, BAT, DEL) | flock + O_APPEND + 0600 |
| `internal/lifecycle/boot.go` (State 4-enum · Manager · RunBootLoop) | **전체 참조** (BOOT 자체 + 7개 tool의 CheckState, DIA/VST는 state 필드 report 용도) | atomic.Int32 state, 1s→60s cap backoff |
| `internal/adapters/vault/errors.go::MapGRPCError` | **8** | gRPC code → domain code 매핑. 모든 Vault 호출 경로 + SHUT Close |
| `internal/adapters/envector/errors.go::MapSDKError` | **8** | 동일 목적 (SHUT Close 포함) |

### 4-b. Tier A — 2 플로우 공유

| 파일/함수 | 공유 플로우 | 비고 |
|---|---|---|
| `mcp.CheckState` | T1 CAP / T2 BAT / T3 REC / T5 DEL (4개) | read-only tool은 bypass |
| `mcp.withHint` (state.go private helper) | CheckState가 부르므로 위 4개 flow에서 간접 호출 | state별 recovery hint 문구. 단순 getter지만 메시지 표준화 지점 |
| `mcp.ValidateCaptureRequest` · `TruncateTitle` · `ClampConfidence` | T1 CAP / T2 BAT | 같이 구현 |
| `adapters/vault.HealthCheck` + `HealthFallback` | T6 VST / T7 DIA | 2-tier 로직 공유 |
| `adapters/vault.GetPublicKey` | F1 BOOT / T8 REL | REL은 BOOT 재실행 |
| `envector.ActivateKeys` | F1 BOOT / T8 REL | 동일 |
| `envector.GetIndexList` | T7 DIA (5s) / T8 REL (60s warmup) | timeout만 다름 |
| `config.Load` + `EnsureDirectories` | F1 BOOT / T8 REL | REL에서도 config 재읽음 |
| `adapters/embedder.Info` (sync.Once) | 4개 플로우 상호작용하지만 T7 DIA에서 snapshot 노출 | info_cache 단일 instance 공유 |
| `service/search.SearchByID` | T3 REC (간접) / T5 DEL (직접) | recall.searchSingle 재사용 기반 |
| `service/recall.classifyMetadata` + `recall.toSearchHit` | T3 REC / T5 DEL (SearchByID 경유) | metadata format 3-way classify (AES/plain/base64) |
| `logio.Tail` vs `logio.Append` | 동일 파일 파서. T4 HIS는 read, T1/T2/T5는 write | 파일 포맷(D20)이 공통 |
| `lifecycle.shutdown.go` adapter Close() 호출 경로 | F2 SHUT가 유일한 caller이지만 **모든 adapter가 Close 인터페이스 필요** | adapter 구현 시점에 Close 반드시 포함 |
| `lifecycle.SetDormant(reason)` helper | **skeleton 미구현 (gap)** — T5 DEL 에러 경로 / F1 BOOT 치명 실패 / 향후 `/rune:deactivate` 전용 | config.json의 state/dormant_reason/dormant_since 필드를 read-modify-write. Python `server.py:L171-189 _set_dormant_with_reason`. 지금 내가 추가해야 할 helper |

### 4-c. Tier B — 단일 플로우 고유 (해당 담당자 단독 작업)

| 파일 | 전용 플로우 |
|---|---|
| `service/capture.go` 전체 | T1 CAP (+ T2 BAT가 재사용) |
| `service/recall.go` 전체 (searchSingle 제외) | T3 REC |
| `service/lifecycle.go` per-tool 메서드 | T4/T5/T6/T7/T8 각자 |
| `service/diagnostics_classify.go` | T7 DIA |

---

## 5. 팀 분업 제안

### 5-a. TEAMMATE 담당 (현재 작업 중) — 건드리지 말 것

- `internal/policy/` 6개 파일 전부 (record_builder · payload_text · pii · novelty · query · rerank)
- `internal/domain/` 7개 파일 (schema · extraction · capture · query · novelty · errors · logio)
- Python bit-identical 포팅 + golden fixture

**나와의 접점**: `domain.RuneError` · `domain.Deps에 들어가는 타입들` · `domain.CaptureRequest/RecallArgs/CaptureLogEntry` — 타입 시그니처가 내 adapter/service/mcp 코드의 입력. **팀원이 domain 타입 확정 전에 내가 service/mcp 내부 로직을 깊게 짜면 타입 충돌 발생**. 순서 주의.

### 5-b. 내가 선행 개발할 공통 모듈 (우선순위 순)

> **원칙**: Tier S 먼저, 외부 의존성 없는 것부터. Phase 1(`go.mod` 외부 deps 추가)과 Phase 2·3(policy/domain) 대기하지 않고 병렬 진행 가능한 항목 위주.

**Step 1 — 외부 deps 없이 즉시 가능** (팀원 policy/domain 작업과 완전 병렬):
1. **`internal/mcp/tools.go::Deps` 구조체 확정** — 필드 목록 고정, 주석 풀기. 팀원이 service 내부 쓰게 될 때 필요. 구체 타입은 interface로 놓아 adapter 구현 전이라도 서명 고정 가능.
2. **`internal/obs/slog.go`** — SensitiveFilter 2 regex, WithRequestID/NewRequestID 구현. stdlib만 필요.
3. **`internal/mcp/state.go`의 `CheckState` helper 마무리** — 이미 대부분 구현됨, 테스트 추가만.
4. **`internal/lifecycle/boot.go` State 전이 + backoff 로직** (Vault 호출은 interface로 스텁 의존). stdlib + atomic.

**Step 2 — Phase 1(go.mod) 의존 후 즉시**:
5. **`internal/adapters/config/loader.go`** — JSON 스키마 파싱 + 권한 enforce + env var override.
6. **`internal/adapters/logio/capture_log.go`** — flock + O_APPEND + Tail 역순 리더. syscall/x/sys 필요.
7. **`internal/adapters/envector/aes_ctr.go::Seal/Open`** — crypto/aes + crypto/cipher + crypto/rand. 외부 deps 없음.
8. **`internal/adapters/vault/endpoint.go::NormalizeEndpoint`** — `net/url` stdlib.
9. **`internal/adapters/vault/errors.go::MapGRPCError`** + **`envector/errors.go::MapSDKError`** — 인터페이스 확정만 먼저 가능.

**Step 3 — Phase 4 본격 adapter 구현**:
10. `vault/client.go` gRPC 클라이언트 전체 (Phase 1 머지 필요).
11. `envector/client.go` SDK 래퍼 (Q4 PR 머지 조건부).
12. `embedder/{client,info_cache,retry}.go` — embedder proto stub import 필요.

**Step 4 — adapter 확정 후**:
13. `internal/lifecycle/shutdown.go` 최종 완성 (adapter.Close signature 확정 후).
14. `service/search.go::SearchByID` — recall.searchSingle 공유 기반 (T3/T5 양쪽 담당자에게 먼저 delivery).

### 5-c. 각 플로우 오너 분배 (Step 4 이후)

| 플로우 | 오너 후보 | 이유 |
|---|---|---|
| F1 BOOT / F2 SHUT | 인프라 담당 (나) | lifecycle/adapter 경계 전체 관여 |
| T1 CAP + T2 BAT | 한 명 | capture 로직 전체 공유 |
| T3 REC | 한 명 | query parse + reranking 복잡도 큼 |
| T4 HIS + T6 VST | 한 명 | 모두 read-only, 작음 |
| T5 DEL | REC 담당 or 독립 | recall.searchSingle 재사용하므로 REC 뒤 |
| T7 DIA + T8 REL | 한 명 | lifecycle.go 내부 상호 의존, warmup 등 envector 진단 로직 공유 |

### 5-d. 블로커/의존성 그래프

```
(Phase 1: go.mod deps)
   │
   ├─→ obs/slog.go (stdlib만, Phase 1 전에도 가능)
   ├─→ mcp/state.go (stdlib)
   ├─→ lifecycle/boot.go State machine (stdlib)
   │
   ▼
(Phase 4 adapter 구현: vault/envector/embedder)
   │    ⇐ Q4 envector-go SDK PR 머지 필요
   │
   ├─→ logio/capture_log.go (단독)
   ├─→ envector/aes_ctr.go (단독 crypto)
   ├─→ config/loader.go (단독 JSON)
   │
   ▼
(Phase 5 service orchestration)
   │    ⇐ policy/domain 완료 필요 (TEAMMATE)
   │    ⇐ adapter interface 확정 필요
   │
   ├─→ service/capture.go (CAP/BAT)
   ├─→ service/recall.go (REC)
   ├─→ service/search.go (DEL 선행 필요)
   └─→ service/lifecycle.go (HIS/DEL/VST/DIA/REL)
   │
   ▼
(Phase 6 MCP wiring)
   └─→ mcp/tools.go Register + cmd/rune-mcp/main.go
```

---

## 6. 다음 액션 (내 쪽)

**즉시 가능** (팀원 policy/domain 작업 기다리지 않음):
- [ ] `mcp.Deps` 필드 목록 확정 (adapter interface는 skeleton에 이미 정의돼 있음 — 그대로 import)
- [ ] `obs/slog.go` SensitiveFilter 2 regex 컴파일 + NewHandler wrap + request_id 구현 + 테스트 (현 skeleton은 패턴 상수만 선언, wrap 로직 TODO)
- [ ] `lifecycle/boot.go` State machine (atomic) + backoff 테이블 구현 (Vault 호출은 interface)
- [ ] `lifecycle.SetDormant(reason)` helper 신규 추가 — config.json의 state/dormant_reason/dormant_since 필드 read-modify-write (Python `server.py:L171-189` 참고). skeleton에 gap
- [ ] `mcp/state.go` 테스트 작성 (이미 로직은 있음)

**Phase 1 블로커 해소 대기**:
- [ ] `go.mod` 외부 deps 추가 PR (modelcontextprotocol/go-sdk · grpc · protobuf)

**Phase 1 머지 후**:
- [ ] `config/loader.go` 전체 구현 + 테스트
- [ ] `envector/aes_ctr.go` Seal/Open + round-trip 테스트 + Python bit-identical 검증
- [ ] `logio/capture_log.go` Append/Tail + 동시성 테스트
- [ ] `vault/endpoint.go` NormalizeEndpoint 엣지케이스 보강 + `errors.go` MapGRPCError

**Q4 머지 대기**:
- [ ] `envector/client.go` SDK 연결

**embedder proto 획득 후**:
- [ ] `embedder/client.go` + `info_cache.go` + `retry.go`

---

## 7. 미해결·확인 필요 항목

- **Q1 (AES-MAC)**: 현재 CTR 단독 → MAC 추가 여부 Deferred Post-MVP. Seal/Open 구현 시 향후 `"m":hmac` 필드 추가 가능하도록 envelope 포맷 확장성 확보.
- **Q3 (envector ActivateKeys race)**: 여러 rune-mcp 동시 기동 시. MVP에서는 server-side 멱등성 의존. 보완 필요 시 Tier S Vault/envector에 로직 추가.
- **Q4 (envector-go SDK PR)**: `OpenKeysFromFile` 조건 완화 머지 전까지 `envector/client.go` 실제 동작 검증 불가. Mock 백엔드 설계 선행 가능.
- **Deps 구체 타입 vs interface**: 테스트 용이성을 위해 interface가 일반적이나 Go 관례상 "caller가 interface 정의" 패턴을 따를지 팀 컨벤션 확정 필요.
- **Q10 (Shutdown 순서)**: adapter Close 순서가 vault → envector → embedder인지, 병렬인지 spec에 명시 없음. 안전 기본은 순차 (Vault 마지막, DEK zeroize 직전). `open-questions.md`에 미등재 — 등록 후 해소 필요.
- **Q11 (SetDormant 저장 경로)**: `/rune:deactivate` 외에 rune-mcp 자체가 config.json `state` 필드를 write해야 할 경우 (T5 DEL Vault/envector 실패 등), write 시 atomic rename + perm 0600 유지 전략 확정 필요. 현재 skeleton에 helper 미구현.

---

## 8. 참조 (본 분석 근거)

- `docs/v04/spec/flows/capture.md` — 7-phase capture
- `docs/v04/spec/flows/recall.md` — 7-phase recall
- `docs/v04/spec/flows/lifecycle.md` — 6 lifecycle tools
- `docs/v04/spec/components/{rune-mcp,vault,envector,embedder}.md` — 컴포넌트 계약
- `docs/v04/overview/architecture.md §Scope` — agent-delegated only 전제
- `docs/v04/overview/decisions.md` — D1-D32 결정 (D7/D11/D14/D15/D17/D19/D20/D21/D22/D23/D25/D26/D27/D28/D30 등)
- `internal/README.md` — 패키지 의존 방향 + Python ↔ Go 매핑표
- `cmd/rune-mcp/main.go`, `internal/mcp/tools.go`, `internal/mcp/state.go`, `internal/service/lifecycle.go` 스켈레톤 실측
