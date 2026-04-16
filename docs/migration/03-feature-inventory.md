# Rune AS-IS: 기능 인벤토리 및 MVP 판정

이 문서는 현재 Python 플러그인의 **모든 기능**을 나열하고 Go MVP 기준의
판정을 붙인다:

- **KEEP** — Go로 포팅해 첫 MVP 릴리스에 포함
- **DEFER** — MVP 스코프는 아니지만 후속 버전을 위해 보존할 가치 있음
- **DROP** — 제거, Go 대응 구현 예정 없음

목표는 *agent-delegated happy path* (capture + recall + lifecycle +
diagnostics) 를 커버하는 얇은 Go MVP를 배포하는 것이며, 그 외 모든 것은
드롭 또는 연기한다. 그 시나리오에 load-bearing이 아닌 것은 전부 빠진다.

## A. MCP Tools

| Tool | 판정 | 이유 |
|---|---|---|
| `capture` (agent-delegated 모드, `extracted` 파라미터) | **KEEP** | 프라이머리 캡처 경로. 서버의 LLM 비용 zero. |
| `capture` (legacy 3-tier 모드, `extracted` 없음) | **DROP** | 서버에 Anthropic 키가 필요하고, 에이전트가 이미 한 일을 중복하며, 오직 웹훅 서비스만 필요로 한다. |
| `batch_capture` | **KEEP** | session-end 정리가 이걸 쓴다. per-item 독립 처리가 agent-delegated 모델과 이미 일치. |
| `recall` (raw-results 모드) | **KEEP** | 에이전트가 자체적으로 응답을 작성하길 원하는 경우의 프라이머리 리콜 경로. |
| `recall` (서버 사이드 합성 모드, LLM이 응답 작성) | **DEFER** | LLM aware 에이전트 없이 CLI로 쓰는 경우를 위한 nice-to-have. MVP에는 불필요 — 에이전트가 합성한다. |
| `vault_status` | **KEEP** | `/rune:status`에 필요. 작다. |
| `diagnostics` | **KEEP** | `/rune:status`에 필요. |
| `reload_pipelines` | **KEEP** | `/rune:activate`와 post-configure 플로우에 필요. |
| `capture_history` | **KEEP** | 로컬 전용, 저렴함, "제대로 캡처되었나?" 디버깅에 유용. |
| `delete_capture` | **KEEP** | 사용자 통제용으로 필요. soft-delete 시맨틱이 저렴하다. |

## B. 캡처 파이프라인 기능

| 기능 | 파일 | 판정 | 비고 |
|---|---|---|---|
| **Scribe 에이전트 프롬프트** (캡처 정책 + 추출 스키마) | `agents/claude/scribe.md`, `agents/gemini/scribe.md`, `agents/codex/scribe.md` | **KEEP** | agent-delegated 캡처의 **상류** — 에이전트에게 언제 캡처하고 어떤 JSON을 추출할지 지시. 이것 없이는 `extracted` 파라미터가 서버에 도달하지 않음. Go 마이그레이션 시 프롬프트 내용은 유지하되 MCP tool 호출부만 업데이트. |
| **Scribe 서브에이전트 자동 스폰** | `CLAUDE.md` (rune:scribe 규칙) | **KEEP** | 대화에서 결정이 감지되면 백그라운드로 `rune:scribe` 서브에이전트 스폰 → `capture` MCP 도구 호출. 호스트 에이전트(Claude Code)의 플러그인 시스템에 의존하므로 Go에서 직접 구현할 필요 없음 — 프롬프트 규칙만 유지. |
| agent-delegated `extracted` → DecisionRecord | `record_builder.py` | **KEEP** | 핵심 경로. |
| `single` 스키마 | `schemas/decision_record.py`, `record_builder.py` | **KEEP** | |
| `phase_chain` 스키마 | 동일 | **KEEP** | retriever가 활용하는 순차 추론 구조를 보존. |
| `bundle` 스키마 | 동일 | **KEEP** | 여러 facet을 가진 결정을 보존. |
| Novelty 체크 (0.95 near-duplicate 블록; 런타임 기본값) | `server.py::_classify_novelty` + searcher FHE 경로 | **KEEP** | load-bearing — 메모리 bloat 방지. ※ embedding.py 상수는 0.93이지만 server.py wrapper가 0.95로 오버라이드. |
| Tier 1 패턴 유사도 | `scribe/detector.py` + `scribe/pattern_parser.py` + `common/pattern_cache.py` + `patterns/capture-triggers.md` | **DROP** | legacy 3-tier 경로만 사용. agent-delegated에는 Tier 1이 없음. |
| Tier 2 LLM 정책 필터 | `scribe/tier2_filter.py`, `config.scribe.tier2_*` | **DROP** | production 기본값 off; 호출한 에이전트가 이미 그 판단을 한다. |
| Tier 3 LLM 필드 추출기 | `scribe/llm_extractor.py` | **DROP** | 호출한 에이전트가 이미 추출한다. |
| 낮은 확신 캡처용 리뷰 큐 | `scribe/review_queue.py`, `~/.rune/review_queue.json` | **DROP** | agent-delegated가 상류에서 significance를 보장. |
| Slack 웹훅 인제스션 | `agents/scribe/server.py` + `handlers/slack.py` + `scripts/setup-slack-app.sh` + `config.scribe.slack_*` | **DROP** | 독립된 FastAPI 서비스, MCP surface의 일부 아님, 에이전트 플로우에 불필요. |
| Notion 웹훅 인제스션 | `handlers/notion.py` + `config.scribe.notion_signing_secret` | **DROP** | 동일한 사유. |
| `capture_log.jsonl` append | `mcp/server/server.py` | **KEEP** | `capture_history`를 뒷받침. |
| per-agent DEK 메타데이터 암호화 (AES) | `vault_client.py` | **KEEP** | zero-knowledge 보장의 핵심. |

## C. 리콜 파이프라인 기능

| 기능 | 파일 | 판정 | 비고 |
|---|---|---|---|
| 쿼리 의도 분류 (regex, 영어) | `query_processor.py` | **KEEP** | |
| 쿼리 의도 분류 (LLM, 비영어) | `query_processor.py` | **DEFER** | MVP는 regex-only; 언어 라우팅은 나중에. |
| 엔티티 + 키워드 추출 | `query_processor.py` | **KEEP** | searcher 필터가 다운스트림에서 사용. |
| 시간 범위 추출 | `query_processor.py` + `searcher.py:523` | **KEEP** | 저렴하고 유용. |
| 쿼리 확장 (멀티 쿼리 검색) | `searcher.py:106` | **KEEP** | 리콜 품질에 유의미한 기여. |
| 멀티 쿼리 dedup | `searcher.py` | **KEEP** | |
| FHE round-trip (score → DecryptScores → remind → DecryptMetadata) | `searcher.py:375`, `envector_client.py`, `vault_client.py` | **KEEP** | 핵심. |
| Phase chain 확장 | `searcher.py:306` | **DEFER** | nice-to-have; ~200 라인의 복잡도. MVP는 평탄한 리스트 리턴; chain 재구성은 v1.1. |
| 그룹 조립 / interleave | `searcher.py:178` | **DEFER** | phase chain에 의존. |
| 메타데이터 필터 (domain/status/since) | `searcher.py:228` | **KEEP** | 클라이언트 사이드 best-effort 필터링은 저렴. |
| Recency 가중 (90일 half-life) | `searcher.py:273` | **KEEP** | 쉬운 수식, 높은 효용. |
| 상태 승수 (active > superseded > reverted) | `searcher.py` | **KEEP** | |
| 서버 사이드 LLM 합성 (Claude Sonnet) | `synthesizer.py:142` | **DEFER** | MVP에서는 에이전트가 합성한다. |
| EN/KO/JA 템플릿의 마크다운 폴백 | `synthesizer.py` | **KEEP** (영어만) | MVP: 영어 템플릿만. KO/JA는 연기. |
| confidence 공식 | `synthesizer.py` | **KEEP** | 단순하고, 다운스트림 호출자가 의존. |
| `related_queries` 필드 | `synthesizer.py` | **KEEP** | |

## D. 라이프사이클 & 설정 기능

| 기능 | 파일 | 판정 | 비고 |
|---|---|---|---|
| Config 스키마 (vault/envector/embedding/retriever/state) | `agents/common/config.py` | **KEEP** | 핵심. |
| `scribe.tier2_*` 필드 | 동일 | **DROP** | legacy 3-tier와 결합. |
| `scribe.slack_*` / `scribe.notion_signing_secret` 필드 | 동일 | **DROP** | 웹훅 인제스션과 결합. |
| `scribe.similarity_threshold` / `auto_capture_threshold` 필드 | 동일 | **DROP** | legacy 경로만 읽음. |
| `scribe.patterns_path` 필드 | 동일 | **DROP** | 삭제된 `capture-triggers.md`를 가리킴. |
| `llm.openai_*` / `llm.google_*` 필드 | 동일 | **DROP** | production 미사용; Anthropic이 프라이머리. |
| `llm.anthropic_*` 필드 | 동일 | **DEFER** | 서버 사이드 합성을 유지할 경우만 필요; MVP는 합성을 연기하므로 함께 드롭. |
| 상태 머신 (`active`/`dormant`) | 서버 startup + 모든 tool | **KEEP** | 핵심 UX. |
| `dormant_reason` / `dormant_since` | config | **KEEP** | |
| 라이브 실패 시 자동 demote fail-safe | 서버 런타임 | **KEEP** | 치명적 — 깨진 설치가 토큰을 태우지 않도록. |
| 환경 변수 오버라이드 (`RUNEVAULT_ENDPOINT` 등) | `config.py` | **KEEP** | 배포 유연성. |
| TLS 모드 (self-signed CA / public CA / tls_disable) | `vault_client.py` + `configure` 커맨드 | **KEEP** | self-hosted Vault가 일반 케이스. |
| gRPC `tcp://` endpoint 파싱 | `vault_client.py` | **KEEP** | |
| gRPC legacy `http://...:50080/mcp` endpoint 파싱 | `vault_client.py:93,124` | **DROP** | pre-gRPC legacy, 명시적으로 표기되어 있음. |
| MCP clientInfo 기반 자동 프로바이더 추론 | `server.py:450` | **DROP** | 단순화: config의 provider만 읽는다. |

## E. CLI / 커맨드 / 통합

| 기능 | 파일 | 판정 | 비고 |
|---|---|---|---|
| Claude Code 슬래시 커맨드 (9개) | `commands/claude/*.md` | **KEEP** (마크다운 그대로 포팅) | 런타임은 에이전트 프롬프트지 코드 아님. |
| Codex CLI 슬래시 커맨드 (9개) | `commands/rune/*.toml` | **KEEP** | 동일. |
| Gemini CLI 통합 | `gemini-extension.json`, `agents/gemini/*.md` | **KEEP** | 동일. |
| `plugin.json` + `marketplace.json` | `.claude-plugin/` | **KEEP** | Go 바이너리를 가리키도록 업데이트. |
| `hooks/hooks.json` (비어 있는 플레이스홀더) | | **DROP** | 내용 없음. |
| `scripts/bootstrap-mcp.sh` | | **REPLACE** | Go는 venv가 필요 없다; Go 바이너리를 찾고, 첫 실행 시 임베딩 모델을 다운로드하고, exec하는 작은 런처로 대체. |
| `scripts/install.sh` | | **REPLACE** | prebuilt Go 바이너리를 `~/.rune/bin/`에 설치하거나 `go install`. |
| `scripts/install-codex.sh` | | **REPLACE** | 새 인스톨러 + `codex mcp add`의 얇은 래퍼. |
| `scripts/ensure-codex-ready.sh` | | **REPLACE** | 동일. |
| `scripts/configure-claude-mcp.sh` | | **REPLACE** | |
| `scripts/check-infrastructure.sh` | | **REPLACE** | Go로 재작성, `rune doctor`로 노출. |
| `scripts/start-mcp-servers.sh` | | **KEEP** (사소) | 바이너리 exec만. |
| `scripts/uninstall.sh` | | **KEEP** (사소) | |
| `scripts/dev-reinstall-claude.sh` | | **DROP** | Python 플러그인을 위한 dev 전용. |
| `scripts/register-plugin.sh` | | **DROP** | dev 툴링. |
| `scripts/bundle-rune-core.sh` | | **DROP** | dev 툴링. |
| `scripts/smoke-test-agents.sh` | | **REPLACE** | 여전히 유용하면 Go 바이너리용으로 재작성. |
| `scripts/setup-slack-app.sh` | | **DROP** | Slack 인제스션이 드롭됨. |
| `scripts/migrate_embeddings.py` | | **DROP** | 일회성 관리 도구; 모델 변경 시 다시 작성. |

## F. 암호화 / 스토리지

| 기능 | 파일 | 판정 | 비고 |
|---|---|---|---|
| Vault gRPC `GetPublicKey` | `vault_proto/`, `vault_client.py` | **KEEP** | |
| Vault gRPC `DecryptScores` | 동일 | **KEEP** | |
| Vault gRPC `DecryptMetadata` | 동일 | **KEEP** | |
| 시작 시 Vault 번들 → enVector 자격증명 | `server.py::fetch_keys_from_vault` | **KEEP** | 사용자가 enVector 자격증명을 입력하지 않음. |
| `EncKey.json` / `EvalKey.json` 온디스크 캐시 | `~/.rune/keys/` | **KEEP** | 부팅 가속. |
| `pyenvector` FHE SDK | `envector_sdk.py` | **REPLACE** (Go 바인딩) | plan 문서의 미결 이슈 참고. |
| enVector `insert` / `score` / `remind` | `envector_sdk.py` | **KEEP** | |
| 임베딩 백엔드 — `sbert` | `mcp/adapter/embeddings.py::SBERTSDKAdapter` | **KEEP** (프라이머리) | production 기본값. |
| 임베딩 백엔드 — `fastembed` | 동일 | **DEFER** | config 플래그 뒤의 대체 백엔드로 유지. |
| 임베딩 백엔드 — `huggingface` | 동일 | **DROP** | production 미사용. |
| 임베딩 백엔드 — `openai` | 동일 | **DROP** | 네트워크 의존성이 있어 "로컬 임베딩" 스토리와 배치됨. |
| 모델: `Qwen/Qwen3-Embedding-0.6B` | config 기본값 | **KEEP** | ⚠️ Go envector 스키마 확정 전 **출력 차원 검증** 필수 (Qwen3-0.6B는 1024-dim; pyenvector 인덱스 스키마가 반드시 일치해야 함). |

## G. Observability / Errors

| 기능 | 판정 | 비고 |
|---|---|---|
| `retryable` + `recovery_hint`를 가진 `RuneError` 계층 | **KEEP** | Go에서 타입 에러로 포팅. |
| `prometheus_client` 메트릭 | **DEFER** | MVP 필수 아님. 나중에 연결. |
| `python-json-logger` 구조화 로그 | **REPLACE** | Go에서는 `log/slog` 사용. |
| `capture_log.jsonl` | **KEEP** | |
| `~/.rune/logs/` 로테이트 파일 | **KEEP** | |

## H. 테스트

현재 테스트 스위트는 크다 (`agents/tests/`에 ~17개, `mcp/tests/`에 4개).
Go MVP를 재작성하면서 Python 테스트를 라인별로 포팅하지는 않는다.

| 테스트 파일 | 판정 | 비고 |
|---|---|---|
| `mcp/tests/test_server.py` | **REUSE AS SPEC** | MCP surface에 대한 가장 좋은 구조적 가이드. |
| `mcp/tests/test_vault_client.py` / `test_vault_direct.py` | **REUSE AS SPEC** | Vault gRPC 기대 동작을 정의. |
| `mcp/tests/test_errors.py` | **REUSE AS SPEC** | 에러 계약. |
| `agents/tests/test_retriever.py` | **REUSE AS SPEC** | 리콜 플로우의 정본. |
| `agents/tests/test_novelty_check.py` | **REUSE AS SPEC** | novelty 시맨틱의 정본. |
| `agents/tests/test_agent_delegated.py` | **REUSE AS SPEC** | MVP happy path. |
| `agents/tests/test_batch_capture.py` | **REUSE AS SPEC** | |
| `agents/tests/test_record_builder.py` | **REUSE AS SPEC** | DecisionRecord 형태. |
| `agents/tests/test_schemas.py` | **REUSE AS SPEC** | |
| `agents/tests/test_detector.py` | **DROP** | legacy Tier 1. |
| `agents/tests/test_pattern_parser.py` | **DROP** | legacy Tier 1. |
| `agents/tests/test_tier2_filter.py` | **DROP** | legacy Tier 2. |
| `agents/tests/test_pipeline_scenario.py` | **DROP** | 드롭된 웹훅 파이프라인의 end-to-end 테스트. |
| `agents/tests/test_team_day_scenario.py` | **DROP** | 동일. |
| `agents/tests/test_llm_client.py` / `test_llm_utils.py` | **DROP** | legacy LLM 클라이언트 (OpenAI/Google). |
| `agents/tests/test_config.py` | **REUSE AS SPEC** | Config 계약. |
| `agents/tests/test_language.py` | **DEFER** | 언어 식별 아이디어는 post-MVP용으로 보존. |

## I. 문서

| 문서 | 판정 | 비고 |
|---|---|---|
| `README.md` | **UPDATE** | Go 바이너리 배포로 바뀌면서 설치 절차가 변한다. |
| `SKILL.md` | **UPDATE** | legacy 3-tier 표현과 Slack/Notion 참조 제거. |
| `AGENT_INTEGRATION.md` | **UPDATE** | |
| `GEMINI.md` / `CLAUDE.md` | **UPDATE** | |
| `CONTRIBUTING.md` | **REWRITE** | Go 전용 dev 루프로. |
| `patterns/capture-triggers.md` | **DROP** | |
| `patterns/retrieval-patterns.md` | **KEEP** | retriever 프롬프트에서 참조; 문서 전용. |
| `rune-onepager.drawio.png` | **KEEP** | 릴리스 전에 새 아키텍처로 업데이트. |
| `benchmark/` | **DEFER** | 유용하지만 MVP 필수 아님. 나중에 Go로 재작성. |
| `examples/` | **KEEP** | 사용자용 문서; 구식 정보 확인. |

## J. 세부 Legacy 인벤토리 (file:line drop 리스트)

legacy 서브에이전트 서베이에서 통합. 이 표의 모든 항목은 MVP에서 **DROP**.

| 항목 | 위치 | 라인 수 | 영향 |
|---|---|---|---|
| 1 | `agents/scribe/server.py` | ~577 | FastAPI 웹훅 서비스, `setup-slack-app.sh`에서만 실행됨 |
| 2 | `agents/scribe/handlers/slack.py` | ~236 | 웹훅 핸들러 |
| 3 | `agents/scribe/handlers/notion.py` | ~260 | 웹훅 핸들러 |
| 4 | `agents/scribe/handlers/base.py` | ~140 | 핸들러 베이스 클래스 (slack/notion 드롭 후 확장하는 게 없음) |
| 5 | `agents/scribe/tier2_filter.py` | ~140 | LLM 정책 필터 |
| 6 | `agents/scribe/llm_extractor.py` | ~420 | Tier 3 추출기 |
| 7 | `agents/scribe/detector.py` | ~225 | Tier 1 패턴 디텍터 |
| 8 | `agents/scribe/pattern_parser.py` | ~420 | 마크다운 → 패턴 리스트 |
| 9 | `agents/scribe/review_queue.py` | ~350 | 인간 승인 버퍼 |
| 10 | `agents/common/pattern_cache.py` | ~200 | 사전 임베딩된 capture trigger |
| 11 | `mcp/server/server.py::_legacy_standard_capture()` | 1409–1487 | MCP 서버 내 인라인 3-tier fallback |
| 12 | `mcp/server/server.py` auto-provider 로직 | 451–489 | clientInfo 기반 LLM 프로바이더 스위치 |
| 13 | `mcp/server/server.py` API key 로딩 | 1625–1660 | OpenAI/Google env var wiring |
| 14 | `mcp/adapter/document_preprocess.py` | ~165 | LangChain chunking, 어디서도 import 안 됨 |
| 15 | `mcp/adapter/vault_client.py` legacy HTTP 분기 | 93–94, 122–125 | `http://vault:50080/mcp` fallback |
| 16 | `patterns/capture-triggers.md` (+ 언어 변종) | — | `pattern_parser`의 유일한 입력 |
| 17 | `scripts/migrate_embeddings.py` | ~117 | 일회성 관리 도구 |
| 18 | `scripts/setup-slack-app.sh` | — | Slack 기동 |
| 19 | `scripts/dev-reinstall-claude.sh`, `register-plugin.sh`, `bundle-rune-core.sh` | — | Python 플러그인용 dev 툴링 |
| 20 | `agents/common/llm_client.py` OpenAI + Google 분기 | 51–79 | 멀티 프로바이더 스캐폴딩 |
| 21 | Config: `scribe.tier2_*` | `config.py:70-74` | |
| 22 | Config: `scribe.slack_*` | `config.py:67,73` | |
| 23 | Config: `scribe.notion_signing_secret` | `config.py:74` | |
| 24 | Config: `scribe.similarity_threshold`, `auto_capture_threshold`, `patterns_path` | `config.py:68-72` | |
| 25 | Config: `llm.openai_*`, `llm.google_*` | `config.py:49-54` | |
| 26 | Config: `llm.tier2_provider`, `llm.tier2_model`, 등 | `config.py` | |
| 27 | 테스트: `test_tier2_filter.py`, `test_pipeline_scenario.py`, `test_team_day_scenario.py`, `test_detector.py`, `test_pattern_parser.py`, `test_llm_client.py`, `test_llm_utils.py` | `agents/tests/` | legacy 전용 테스트 |
| 28 | `hooks/hooks.json` | — | 비어 있는 플레이스홀더 |

**대략적 임팩트**: ~4,500 라인의 Python 삭제, ~20개의 config 필드 제거,
이중 서버 아키텍처를 단일 MCP surface로 collapse, LLM 프로바이더 3개 → 1개
(Anthropic), 캡처 경로가 두 구현에서 정확히 하나(agent-delegated)로 감소.
