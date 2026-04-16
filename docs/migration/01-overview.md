# Rune AS-IS: 시스템 개요

## Rune이란

Rune은 **AI 코딩 에이전트 (Claude Code, Codex CLI, Gemini CLI)를 위한 Python
플러그인**이며, 팀에게 **FHE 암호화된 조직 메모리**를 공유시켜 준다. 팀원의
세션에서 중요한 결정 — 대안 중의 선택, 근본 원인 분석, 커밋된 트레이드오프
— 이 내려지면, 에이전트는 그것의 짙은 자연어 "요지(gist)"를 캡처한다.
나중에 팀의 누군가가 관련된 질문을 하면, 에이전트는 요청받지 않고도 자동으로
이전 컨텍스트를 리콜한다.

"암호화" 부분은 load-bearing이다: **클라우드는 plaintext를 절대 보지 못한다**.
메모리는 enVector Cloud에 FHE ciphertext 벡터로 저장되며, 오직 팀의 Rune-Vault
— 비밀키를 보관하는 self-hosted gRPC 서비스 — 만이 유사도 점수와 레코드별
메타데이터를 복호화할 수 있다. LLM 추론이 필요한 작업(significance 판단, 필드
추출, 응답 합성)은 호출한 에이전트 자신이 전부 처리한다; Rune은 지능이 아니라
메커니즘이다.

배포는 Claude Code 플러그인, Codex CLI 스킬, Gemini CLI 익스텐션으로 이루어진다.
세 경로 모두 같은 Python 코드베이스를, 단일 **MCP stdio 서버**를 통해 사용한다.

## 큰 그림

```
┌─────────────────────────────────────────────────────────────────────┐
│  개발자의 머신                                                      │
│                                                                     │
│  ┌─────────────┐   stdio MCP   ┌────────────────────────────────┐   │
│  │  에이전트 CLI│◄─────────────►│  mcp/server/server.py          │   │
│  │ (Claude,    │               │  (FastMCP, Python)             │   │
│  │  Codex,     │               │                                │   │
│  │  Gemini)    │               │  - capture / batch_capture     │   │
│  └─────────────┘               │  - recall                      │   │
│                                │  - vault_status / diagnostics  │   │
│                                │  - reload_pipelines            │   │
│                                │  - capture_history             │   │
│                                │  - delete_capture              │   │
│                                └──┬──────────────┬───────────┬──┘   │
│                                   │              │           │      │
│                    ┌──────────────┘              │           │      │
│                    ▼                             ▼           ▼      │
│           ┌────────────────┐          ┌────────────────┐  ~/.rune/  │
│           │ EmbeddingAdapter│          │ VaultClient    │  config.  │
│           │ (SBERT/Qwen3)   │          │ (gRPC stub)    │  json     │
│           │ 프로세스 내      │          └────────┬───────┘  log.    │
│           └────────────────┘                   │          jsonl    │
│                                                │                    │
└────────────────────────────────────────────────┼────────────────────┘
                                                 │
                                    ┌────────────┴──────────┐
                                    │                       │
                                    ▼                       ▼
                        ┌──────────────────────┐  ┌────────────────────┐
                        │  enVector Cloud      │  │  Rune-Vault (Go)   │
                        │  (managed FHE)       │  │  gRPC, port 50051  │
                        │                      │  │                    │
                        │  - insert(ciphertext)│  │  - GetPublicKey    │
                        │  - score()           │  │  - DecryptScores   │
                        │  - remind()          │  │  - DecryptMetadata │
                        └──────────────────────┘  └────────────────────┘
                                                   (비밀키를 보관하며,
                                                    절대 노출하지 않음)
```

**신뢰 경계 (trust boundary)**

- **enVector Cloud**는 honest-but-curious로 취급된다. FHE ciphertext와
  동형 스코어링 결과만 볼 뿐, plaintext는 전혀 보지 못한다.
- **Rune-Vault**는 신뢰된다: FHE 비밀키와 per-agent DEK (메타데이터용
  256-bit AES)를 보관한다. 팀이 self-host한다.
- **개발자의 머신**은 완전히 신뢰된다. 임베딩은 로컬에서 생성되고, 임베딩
  벡터의 FHE 암호화는 startup 시 Vault에서 가져온 public key를 사용해
  `pyenvector` SDK가 로컬에서 수행한다.

## 컴포넌트

| 레이어 | 경로 | 역할 |
|---|---|---|
| **MCP server** | `mcp/server/server.py` | FastMCP stdio 서버, tool 등록, orchestration, `_ensure_pipelines()` 지연 초기화 |
| **Vault client** | `mcp/adapter/vault_client.py` | `GetPublicKey`, `DecryptScores`, `DecryptMetadata`에 대한 gRPC stub 래퍼; startup 시 `EncKey.json` + `EvalKey.json`을 가져옴 |
| **enVector SDK adapter** | `mcp/adapter/envector_sdk.py` | `pyenvector` SDK 호출 (`insert`, `score`, `remind`) 래핑; 현재는 에러 dict를 반환(예외 throw 안 함) |
| **Embedding adapter** | `mcp/adapter/embeddings.py` | 4개 백엔드(fastembed, sbert, huggingface, openai)를 가진 `EmbeddingAdapter`; production은 `sbert` + `Qwen/Qwen3-Embedding-0.6B` 사용 |
| **Document preprocess** | `mcp/adapter/document_preprocess.py` | LangChain 기반 chunking — **`mcp/adapter/__init__.py`에서만 import, 실제 사용처 없음** |
| **Errors** | `mcp/server/errors.py` | `code`, `retryable`, `recovery_hint`을 갖는 `RuneError` 기저 클래스 |
| **Embedding service** | `agents/common/embedding_service.py` | `EmbeddingAdapter`의 싱글턴 래퍼 + cosine 헬퍼 |
| **enVector client** | `agents/common/envector_client.py` | SDK 어댑터 위의 고수준 래퍼 |
| **LLM client** | `agents/common/llm_client.py` | 멀티 프로바이더 (Anthropic, OpenAI, Google); production에서는 Anthropic만 실제로 사용 |
| **Schemas** | `agents/common/schemas/{decision_record,embedding,templates}.py` | `DecisionRecord` dataclass + LLM 프롬프트 템플릿 |
| **Language detection** | `agents/common/language.py` | 리트리벌 라우팅용 언어 식별 |
| **Pattern cache** | `agents/common/pattern_cache.py` | 사전 임베딩된 capture trigger — **캡처 전용, retriever는 사용 안 함** |
| **Config** | `agents/common/config.py` | `RuneConfig` + dataclass들, env override, `~/.rune/config.json` 영속화 |
| **Scribe — detector** | `agents/scribe/detector.py` | Tier 1 패턴 유사도 (capture trigger) |
| **Scribe — pattern parser** | `agents/scribe/pattern_parser.py` | `patterns/capture-triggers.md`를 파싱해 패턴 리스트로 |
| **Scribe — tier2 filter** | `agents/scribe/tier2_filter.py` | Tier 2 LLM 정책 필터 (Claude Haiku) — **legacy** |
| **Scribe — LLM extractor** | `agents/scribe/llm_extractor.py` | Tier 3 필드 추출 (Claude Sonnet) — **legacy** |
| **Scribe — record builder** | `agents/scribe/record_builder.py` | 추출된 필드 → `DecisionRecord` 변환 |
| **Scribe — review queue** | `agents/scribe/review_queue.py` | 낮은 확신의 캡처에 대한 인간 리뷰 버퍼 — **legacy** |
| **Scribe — FastAPI server** | `agents/scribe/server.py` | Slack/Notion 웹훅용 독립 uvicorn 서비스 — **legacy** |
| **Scribe — handlers** | `agents/scribe/handlers/{slack,notion}.py` | 웹훅 수신기 — **고아, 위 FastAPI 서버에서만 사용** |
| **Retriever — query processor** | `agents/retriever/query_processor.py` | 의도 분류, 엔티티 추출, 쿼리 확장 |
| **Retriever — searcher** | `agents/retriever/searcher.py` | 멀티 쿼리 검색, FHE round-trip, phase chain 조립, 재랭킹 |
| **Retriever — synthesizer** | `agents/retriever/synthesizer.py` | LLM 응답 합성 (Claude Sonnet) 또는 마크다운 폴백 |

## 배포 토폴로지

Rune은 세 부분으로 배포된다. 이 중 둘은 팀 전역 인프라고, 나머지 하나는
개발자별로 있다.

1. **Rune-Vault** (팀 전역, self-hosted). 팀의 FHE 비밀키와 per-agent DEK를
   보관하는 Go gRPC 서비스. 세 개의 RPC: `GetPublicKey` (EncKey.json +
   EvalKey.json + config 번들 반환), `DecryptScores` (FHE 유사도 ciphertext
   복호화), `DecryptMetadata` (AES 암호화된 레코드 메타데이터 복호화).
   `rune-admin`의 Terraform 모듈로 OCI/AWS/GCP에 배포.
2. **enVector Cloud** (managed, 팀 전역). 호스팅되는 FHE 벡터 DB. 각 팀은
   Vault 관리자가 프로비저닝해 주는 인덱스를 하나 받는다. SDK endpoint +
   API key는 **Vault 번들의 일부로** 클라이언트에 전달된다 — 사용자가 직접
   enVector 자격증명을 입력할 일이 없다.
3. **Rune 플러그인** (개발자별). 에이전트 CLI가 stdio로 실행하는 Python
   프로세스. Entrypoint: `scripts/bootstrap-mcp.sh`가 venv 생성, 의존성 설치,
   self-healing (pip shebang 오염, stale `fastembed` 캐시)을 처리한 뒤
   `mcp/server/server.py --mode stdio`를 exec한다. startup 시 서버는 Vault
   키를 가져오고 enVector 자격증명을 `~/.rune/config.json`에 캐시하며,
   capture + retrieval 파이프라인을 백그라운드에서 지연 초기화한다
   (첫 tool 호출 시 120s 타임아웃).

## 기술 스택

| 관심사 | 기술 | 비고 |
|---|---|---|
| Language | Python 3.11+ | |
| MCP framework | `fastmcp` ≥ 2.2.0 | stdio만; HTTP/SSE 미지원 |
| FHE SDK | `pyenvector` ≥ 1.2.0 | enVector FHE 라이브러리를 감싸는 C 익스텐션 |
| Vault RPC | gRPC (Python 생성 stub) | proto: `mcp/adapter/vault_proto/` |
| Embedding | `sentence-transformers` (sbert) + `Qwen/Qwen3-Embedding-0.6B` | production 기본값; `fastembed`, `transformers`, `openai`는 대체 백엔드 |
| LLM | Anthropic SDK | `claude-sonnet-4-20250514` (추출 + 합성), `claude-haiku-4-5-20251001` (tier 2 필터, legacy) |
| Webhook 인제스션 | `fastapi` + `uvicorn` + `slack-sdk` | legacy 경로만 (`agents/scribe/server.py`) |
| Config | 디스크 JSON + dataclass 모델 | `~/.rune/config.json` |
| Observability | `prometheus-client`, `python-json-logger` | |
| 로컬 파일 | `~/.rune/logs/`, `~/.rune/keys/`, `~/.rune/capture_log.jsonl`, `~/.rune/review_queue.json`, `~/.rune/certs/ca.pem` | |

### 로그 보안

`mcp/server/server.py`의 `_SensitiveFilter`가 모든 로그 출력에서 민감 토큰을
자동 마스킹한다. `sk-`, `pk-`, `api_`, `envector_`, `evt_` 접두사가 붙은 10자
이상 문자열과 `token|key|secret|password` 뒤의 20자 이상 문자열을 감지하여
처음 8자만 남기고 `***`로 대체한다. Go 구현에서도 동일한 필터 필요.

## 설정 모델

`agents/common/config.py`에 정의된 `~/.rune/config.json`의 전체 스키마:

```json
{
  "vault": {
    "endpoint": "tcp://vault-team.oci.envector.io:50051",
    "token": "evt_...",
    "ca_cert": "",
    "tls_disable": false
  },
  "envector": {
    "endpoint": "",
    "api_key": ""
  },
  "embedding": {
    "mode": "sbert",
    "model": "Qwen/Qwen3-Embedding-0.6B"
  },
  "llm": {
    "provider": "anthropic",
    "tier2_provider": "anthropic",
    "anthropic_api_key": "",
    "anthropic_model": "claude-sonnet-4-20250514",
    "openai_api_key": "",
    "openai_model": "gpt-4o-mini",
    "openai_tier2_model": "",
    "google_api_key": "",
    "google_model": "gemini-2.0-flash-exp",
    "google_tier2_model": ""
  },
  "scribe": {
    "slack_webhook_port": 8080,
    "similarity_threshold": 0.35,
    "auto_capture_threshold": 0.7,
    "tier2_enabled": false,
    "tier2_model": "claude-haiku-4-5-20251001",
    "patterns_path": "<PROJECT_ROOT>/patterns/capture-triggers.md",
    "slack_signing_secret": "",
    "notion_signing_secret": ""
  },
  "retriever": {
    "topk": 10,
    "confidence_threshold": 0.5
  },
  "state": "active",
  "dormant_reason": "",
  "dormant_since": ""
}
```

### 실제로 소비되는 필드

- `vault.*` — 모든 요청에서.
- `envector.endpoint`, `envector.api_key` — Vault 번들로부터 *서버가* 채운다;
  사용자가 직접 설정하지 않는다.
- `embedding.mode` + `embedding.model` — 파이프라인 init 시 한 번 읽힘.
- `llm.provider` + `llm.anthropic_*` — retriever synthesizer(선택적)와
  legacy tier 3 extractor가 사용.
- `llm.openai_*` + `llm.google_*` — **실전에서는 사용되지 않음**; 이전의
  멀티 프로바이더 실험의 잔해.
- `scribe.similarity_threshold` + `scribe.auto_capture_threshold` —
  legacy 3-tier 파이프라인만 읽는다.
- `scribe.tier2_*` + `scribe.slack_*` + `scribe.notion_*` +
  `scribe.patterns_path` — **legacy 전용** (3-tier 파이프라인 + FastAPI
  웹훅 서버).
- `retriever.topk` + `retriever.confidence_threshold` — searcher와
  synthesizer가 읽는다.
- `state` — 모든 tool 호출의 게이트: `"active"`가 아니면 tool이 setup
  안내를 돌려주며 거부.
- `dormant_reason` / `dormant_since` — 서버가 자동 demote될 때 세팅됨
  (예: vault_unreachable, vault_token_invalid, user_deactivated).

### 환경변수 오버라이드

서버는 config.json 외에도 다수의 환경변수를 읽는다. 전체 목록은
[06-runed-implementation-spec.md §14](06-runed-implementation-spec.md) 참고.
주요 변수: `ENVECTOR_CONFIG` (config 경로), `RUNEVAULT_ENDPOINT` / `RUNEVAULT_TOKEN`
(Vault 자격증명), `ANTHROPIC_API_KEY` (LLM), `RUNEVAULT_GRPC_TARGET` (gRPC
endpoint 직접 지정).

## 주요 파일 인덱스

```
mcp/
├── server/
│   ├── server.py                 # FastMCP stdio 서버, ~2000 라인
│   └── errors.py                 # RuneError 계층
├── adapter/
│   ├── embeddings.py             # EmbeddingAdapter (4개 백엔드)
│   ├── vault_client.py           # Rune-Vault용 gRPC 클라이언트
│   ├── envector_sdk.py           # pyenvector SDK 래퍼
│   ├── document_preprocess.py    # 고아 — 드롭
│   └── vault_proto/              # 생성된 gRPC stub
└── tests/                        # test_server.py, test_vault_*.py, test_errors.py

agents/
├── common/
│   ├── config.py                 # RuneConfig dataclass + IO
│   ├── embedding_service.py      # 싱글턴 래퍼
│   ├── envector_client.py        # 고수준 SDK 래퍼
│   ├── llm_client.py             # 멀티 프로바이더; Anthropic만 live
│   ├── language.py               # 언어 식별
│   ├── pattern_cache.py          # 캡처 전용; LEGACY
│   └── schemas/
│       ├── decision_record.py    # DecisionRecord dataclass
│       ├── embedding.py
│       └── templates.py          # LLM 프롬프트 템플릿
├── scribe/
│   ├── detector.py               # Tier 1 (legacy)
│   ├── pattern_parser.py         # Tier 1 지원 (legacy)
│   ├── tier2_filter.py           # Tier 2 (legacy)
│   ├── llm_extractor.py          # Tier 3 (legacy)
│   ├── record_builder.py         # 활성 — 두 경로 모두에서 사용
│   ├── review_queue.py           # legacy
│   ├── server.py                 # FastAPI uvicorn — legacy
│   └── handlers/
│       ├── slack.py              # legacy
│       └── notion.py             # legacy
├── retriever/
│   ├── query_processor.py        # KEEP
│   ├── searcher.py               # KEEP
│   └── synthesizer.py            # KEEP (단, 다이어트)
├── claude/{scribe,retriever}.md  # 에이전트 프롬프트
├── gemini/{scribe,retriever}.md  # 에이전트 프롬프트
└── codex/scribe.md               # 에이전트 프롬프트

commands/
├── claude/*.md                   # Claude Code용 슬래시 커맨드 정의
└── rune/*.toml                   # Codex CLI용 슬래시 커맨드 정의

scripts/
├── bootstrap-mcp.sh              # 런타임 준비의 단일 진실 소스
├── install.sh                    # Claude Code 인스톨러
├── install-codex.sh              # Codex 인스톨러
├── ensure-codex-ready.sh         # Codex 어댑터
├── check-infrastructure.sh       # 사전 검증
├── configure-claude-mcp.sh       # Claude Desktop MCP 등록
├── start-mcp-servers.sh          # 수동 MCP 기동
├── uninstall.sh                  # 제거
├── setup-slack-app.sh            # LEGACY — agents/scribe/server.py 실행
├── migrate_embeddings.py         # LEGACY — 일회성 관리 도구
├── dev-reinstall-claude.sh       # dev 편의
├── register-plugin.sh            # dev 툴링
├── bundle-rune-core.sh           # dev 툴링
└── smoke-test-agents.sh          # CI/dev

patterns/
├── capture-triggers.md           # LEGACY — Tier 1 pattern cache만 사용
└── retrieval-patterns.md         # 문서 (retriever 프롬프트에서 참조)

config/
├── config.template.json          # configure가 설치하는 템플릿
└── README.md

hooks/hooks.json                  # 비어 있는 플레이스홀더

.claude-plugin/
├── plugin.json                   # Claude Code 플러그인 매니페스트
└── marketplace.json              # Claude Code 마켓플레이스 엔트리

gemini-extension.json             # Gemini CLI 익스텐션 매니페스트
```
