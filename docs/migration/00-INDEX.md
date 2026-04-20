# Rune Python → Go 마이그레이션 문서

이 폴더는 Rune 플러그인의 Python → Go 마이그레이션에 대한 **단일 진실 소스
(single source of truth)** 다. 현재 Python 코드베이스의 AS-IS 상태를 포착하고,
MVP에서 무엇을 유지/드롭/연기할지 결정하고, 목표 Go 아키텍처를 제안한다.

## 읽는 순서

1. **[01-overview.md](01-overview.md)** — 오늘의 Rune이 무엇인지, 큰 그림
   다이어그램, 컴포넌트, 기술 스택, 배포 토폴로지, 설정 모델.
2. **[02-flows.md](02-flows.md)** — 현재 존재하는 캡처/리콜/라이프사이클
   플로우의 end-to-end 워크스루 (legacy 3-tier 캡처 경로 포함).
3. **[03-feature-inventory.md](03-feature-inventory.md)** — 모든 기능에 Go MVP
   기준 **KEEP / DROP / DEFER** 판정을 붙인 목록, 그리고 `file:line` 레퍼런스를
   가진 세부 legacy 인벤토리.
4. **[04-go-migration-plan.md](04-go-migration-plan.md)** — 제안된 Go 타겟
   아키텍처 (임베딩 서버 + CLI), 와이어 프로토콜, 패키지 레이아웃, 단계별
   마이그레이션 계획, 미결 이슈, 리스크.
5. **[05-architecture-comparison.md](05-architecture-comparison.md)** — MCP+CLI+runed
   (안 A) vs CLI+runed (안 B) 아키텍처 비교. runed 내부 구조는 양안 공통.
6. **[06-runed-implementation-spec.md](06-runed-implementation-spec.md)** — Go 데몬
   서브시스템별 구현 명세: Vault/enVector gRPC, AES-256-CTR, 임베딩, Capture/Recall,
   config, lifecycle, 에러 응답, 환경변수, 보안.
7. **[07-implementation-details.md](07-implementation-details.md)** — Go 포팅에 필요한
   상세 데이터 테이블: intent regex 패턴, stop words, entity 추출 알고리즘,
   ExtractionResult 스키마, Domain/Certainty/Status 판정 로직, enVector SDK
   안전 패치, AES 암호화 모드 확정(CTR).
8. **[08-agent-contract.md](08-agent-contract.md)** — 에이전트-서버 간 인터페이스
   계약: agent JSON 입력 스키마 (Format A/B/C), capture/recall 응답 포맷,
   scribe 트리거 패턴, retriever 합성 규칙, 에이전트별 차이, phase chain 현황.
   원래 서베이에서 서버 구현 중심으로 분석하면서 에이전트 측 계약이 누락되어
   별도 보충한 문서.

## TL;DR

- Rune은 **Python MCP 플러그인** (`mcp/` + `agents/` 합쳐 ~17.6k LOC) 으로,
  AI 에이전트들에게 FHE 암호화된 공유 메모리를 제공한다.
- 캡처 경로는 **두 개의 병렬 구현**을 갖는다: *agent-delegated* 경로(모던,
  프라이머리)와 *legacy 3-tier 서버사이드 LLM 파이프라인*. 3-tier 경로는 거의
  전적으로 *Slack/Notion 웹훅 인제스션 서비스* (`agents/scribe/server.py`)를
  위해 존재하며, 이 서비스는 **MVP에 포함되지 않는다**.
- 리콜 경로는 단일하며 ~1.5k LOC, `query_processor → searcher →
  vault FHE round-trip → synthesizer` 로 조직되어 있다.
- Go MVP에서는 **3-tier 캡처 경로 전체, 두 웹훅 핸들러, FastAPI scribe
  서비스, 리뷰 큐, 다국어 capture-trigger 캐시, 문서 전처리,
  OpenAI/Google LLM 클라이언트, 임베딩 마이그레이션 스크립트**를 드롭한다
  — ~17.6k 중 대략 **~4,500 라인**.
- 타겟 Go 아키텍처: 장수명 **임베딩 서비스** (SBERT/Qwen3 모델을 보유하며,
  enVector + Vault와 통신)와 얇은 **CLI** (에이전트 — Claude Code, Codex,
  Gemini — 가 capture와 recall을 호출할 때 사용). 둘 사이의 와이어
  프로토콜은 유닉스 소켓 위의 로컬 **HTTP + JSON**; CLI는 에이전트와 **stdio
  MCP**로 통신한다.
- MCP tool surface는 호환성을 유지한다: `capture`, `batch_capture`, `recall`,
  `vault_status`, `diagnostics`, `reload_pipelines`, `capture_history`,
  `delete_capture` — 다만 더 얇은 시맨틱으로.
- 에이전트-서버 인터페이스 계약은 [08-agent-contract.md](08-agent-contract.md)에
  정리되어 있다. agent JSON 입력 스키마(Format A/B/C), capture/recall 응답 포맷,
  scribe 캡처 트리거 21종(15 주요 + 6 코딩 하위), retriever certainty별 합성 규칙이 포함된다.

## 이 문서 세트의 상태

이 문서들은 마이그레이션 시작 시점의 AS-IS 분석이다. Python 코드베이스의
이후 변경과 **동기화되지 않는다** — Go 구현이 시작되면 canonical 레퍼런스는
Go 코드 자체다.

**2026-04-17 전체 재검증 통과**: 전체 Python 코드베이스(`agents/`, `mcp/`, `commands/`,
`scripts/`, `config/`)를 직접 대조하여 `file:line` 참조와 사실 주장을 교정했다.
주요 수정: Intent regex 31개(33 아님), 테스트 15개(17 아님), bootstrap self-heal
4단계(3 아님, fastembed 단계는 stale 방어 코드), `_init_pipelines` dormant 조기 리턴
(L1544-1547), `_maybe_reload_for_auto_provider`는 DROP 표기에도 코드상 live, legacy
HTTP endpoint 파싱도 live, AES-256-CTR 확정(pyenvector/utils/aes.py:52-58).
자세한 교정 내역: **[python-go-comparison.html](python-go-comparison.html)**의
"문서 검증 상태" callout + Part 4 §4.5 결정 #34/#35.

## 서베이 과정에서 해소된 충돌

병렬 서브에이전트 서베이들 사이에 몇 가지 충돌이 드러났고, 다음과 같이
해소되었다 (관련 문서에 명시):

- `pattern_cache`는 **캡처 전용**이며 retriever는 쓰지 않는다 (grep으로
  검증 — `agents/retriever/*.py` 중 어느 것도 import하지 않음). legacy
  캡처 경로와 함께 드롭.
- production 임베딩 설정은 **`sbert` 모드 + `Qwen/Qwen3-Embedding-0.6B`**
  이다 (`agents/common/config.py:38-39`, 사용자의 `~/.rune/config.json`).
  일부 보고에 언급된 "fastembed default"는 실제로는 실행되지 않는 dead-code
  SDK 어댑터 default다. Qwen3-0.6B는 일부 서베이가 인용한 384가 아니라
  **1024차원** 벡터를 출력한다 (그 384 수치는 SBERT MiniLM 클래스 default).
- `agents/scribe/server.py`는 **독립된 FastAPI 프로세스**이며
  `scripts/setup-slack-app.sh`에서만 실행된다. `mcp/server/server.py`와
  독립적이고 Slack/Notion 인제스션 경로와 함께 수명을 다한다.
