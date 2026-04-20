# Rune AS-IS: End-to-End 플로우

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스 실측 대조 완료. 주요 교정: §6 bootstrap self-heal **4단계** (3 아님, `VIRTUAL_ENV unset` 누락이었음), fastembed 4단계는 stale 방어 코드, `_init_pipelines` L1544-1547에서 **dormant 조기 리턴**, `save_config` 호출이 config.json을 3섹션→7섹션 확장시키는 사이드 이펙트.

이 문서는 현재 Python 코드베이스에 존재하는 각 플로우를, Go 포팅이 중요한
부분의 동작을 그대로 재현하고 중요하지 않은 부분은 의도적으로 건너뛸 수 있을
만큼의 디테일로 서술한다.

## 1. 라이프사이클: Active vs Dormant

플러그인은 두 개의 동작 상태 — `active`와 `dormant` — 를 가지며, 상태는
`~/.rune/config.json`의 `state` 필드에 저장된다.

### Active

- 8개의 MCP tool 전부가 활성화됨.
- 에이전트 프롬프트 (`agents/{claude,gemini,codex}/*.md`)는 에이전트에게
  중요한 결정을 적극적으로 캡처하고, decision-rationale 류의 쿼리에서는
  자동으로 recall을 실행하라고 지시한다.
- 서버는 startup 시:
  1. `~/.rune/config.json`을 읽는다.
  2. Rune-Vault에 `GetPublicKey`를 호출해 `EncKey.json` + `EvalKey.json`을
     다운로드한다.
  3. Vault 번들에서 enVector endpoint + API key를 추출해 `config.envector.*`
     에 기록한다 (그래서 사용자가 직접 설정할 필요가 없다).
  4. 백그라운드 스레드에서 `EmbeddingAdapter`를 지연 초기화한다.
  5. scribe + retriever 파이프라인을 지연 초기화하고, 파이프라인이 필요한
     tool 호출은 `_ensure_pipelines()`에서 최대 120초까지 대기한다.

### Dormant

- 모든 MCP tool이 즉시 에러로 거부되며, 에러에는 `dormant_reason` (코드:
  `vault_unreachable`, `vault_token_invalid`, `user_deactivated` 등)과
  setup 힌트가 포함된다.
- 에이전트 프롬프트는 매 세션 시작에서 상태를 확인하고 dormant면 auto-capture
  / auto-recall을 완전히 스킵한다.
- **네트워크 호출을 전혀 하지 않는다** — 고의적인 설계로, 깨진 설치가
  실패하는 tool 라운드트립으로 토큰을 태우지 않도록 한다.

### 상태 전이

| From | Via | To |
|---|---|---|
| (설치 전) | `/rune:configure` | `dormant` (인프라 검증 아직 안 됨) 또는 `active` (인프라 OK) |
| `dormant` | `/rune:activate` (전체 헬스 체크 + `reload_pipelines` 실행) | 성공 시 `active`, 실패 시 `dormant` 유지 |
| `active` | `/rune:deactivate` | `dormant` (config 보존) |
| `active` | *세션 도중 인프라 실패* (예: Vault 도달 불가) | `dormant`; 플러그인이 `dormant_reason`을 기록하고, 사용자에게 한 번 알리고, 재시도를 멈춤 |
| `dormant` | `/rune:reset` | `dormant` + config 삭제 |

라이브 인프라 실패 시의 **fail-safe 동작**이 중요하다: 플러그인은 조용히
재시도하지 않는다. 자동 demote 후 사람의 개입을 기다린다.

## 2. 캡처 플로우 — Agent-Delegated (프라이머리, 모던)

Go MVP가 완전히 재현해야 하는 경로다. 호출한 에이전트 (Claude, Codex,
Gemini)가 significance 판단과 구조화된 필드 추출을 책임지고, Rune 서버는
저장만 한다.

```
세션 속의 에이전트
    │
    │ (1) 에이전트가 대화에서 중요한 결정을 감지함.
    │     scribe.md 프롬프트에 나열된 패턴을 따른다
    │     ("we chose X over Y because...", "the root cause was...",
    │      한국어 equivalents, 등)
    │
    │ (2) 에이전트가 DecisionRecord 형태의 JSON을 추출한다.
    │     세 가지 스키마 중 하나: single, phase_chain, bundle
    │
    ▼
MCP 호출: capture(text, source, user, channel, extracted=<JSON>)
    │
    ▼
mcp/server/server.py::tool_capture
    │
    ├── 게이트: state != "active" → dormant 에러로 거부
    ├── _ensure_pipelines()에서 대기 (첫 호출 시 최대 120s)
    │
    ├── `extracted`가 존재 → (AGENT-DELEGATED 경로) ──┐
    │                                                  │
    │                                                  ▼
    │                                    입력 검증:
    │                                       tier2.capture == false → 즉시 거부
    │                                       phases[:7] (최대 7개)
    │                                       title[:60] (60자 절삭)
    │                                       confidence: 0.0–1.0 클램핑
    │                                       phases 1개 → single 스키마로 취급
    │                                    │
    │                                    ▼
    │                                    record_builder.build_phases(pre_extraction=<ExtractionResult>)
    │                                    DecisionRecord 생성(
    │                                        id, title, rationale,
    │                                        problem, alternatives,
    │                                        trade_offs, status,
    │                                        domain, tags,
    │                                        source, user, channel,
    │                                        created_at, ...)
    │                                    │
    │                                    ▼
    │                                    EmbeddingService.embed(record.embed_text)
    │                                    → 1024-dim float 벡터 (sbert + Qwen3-0.6B)
    │                                    (Go 스키마 확정 전에 dim 실측 필요)
    │                                    │
    │                                    ▼
    │                                    Novelty 체크:
    │                                       envector.score(query=vec)
    │                                       → FHE 유사도 ciphertext
    │                                       vault.DecryptScores(ct)
    │                                       → top-k {shard, row, score}
    │                                       if top_score ≥ 0.95 → near_duplicate
    │                                           → "duplicate, not stored" 리턴
    │                                       0.7–0.95 → related (저장, related 태그)
    │                                       0.3–0.7  → evolution
    │                                       < 0.3    → novel
    │                                       ※ 런타임 임계값은 server.py::_classify_novelty()
    │                                         기본 인자(0.3/0.7/0.95)가 embedding.py 상수
    │                                         (0.4/0.7/0.93)를 오버라이드한다.
    │                                    │
    │                                    ▼
    │                                    envector.insert(
    │                                        vec,
    │                                        metadata=AES-DEK-encrypt(
    │                                            json.dumps(record)
    │                                        )
    │                                    )
    │                                    → record_id, shard, row
    │                                    │
    │                                    ▼
    │                                    ~/.rune/capture_log.jsonl 에 append
    │                                    ※ Novelty 체크 실패는 non-fatal:
    │                                       예외 발생 → 경고 로그, 캡처 계속
    │                                    │
    │                                    ▼
    │                                    return {"ok": true, "record_id": ...}
    │
    └── 아니면 (LEGACY 3-TIER 경로) — 섹션 3 참고
```

**Go MVP가 이 경로에서 구현해야 하는 것**:
- agent-delegated JSON 입력 → DecisionRecord 매핑
- 로컬 임베딩 (임베딩 서비스 프로세스로 위임)
- `envector.score` + `vault.DecryptScores`로 novelty 체크
- per-agent DEK로 메타데이터 AES 암호화
- `envector.insert`
- `capture_log.jsonl` 에 append

입력 JSON이 표현할 수 있는 세 가지 스키마 — **전부 보존해야 한다**:

| 스키마 | 언제 | 효과 |
|---|---|---|
| `single` | 단발성 결정 | 단일 레코드 저장 |
| `phase_chain` | 여러 단계에 걸친 순차적 추론 (problem → options → pick → rationale) | 각 phase를 독립 레코드로 저장하고 `phase_seq` + 공유 `group_id`를 부여; retriever가 체인을 재구성 가능 |
| `bundle` | 여러 디테일 facet을 가진 하나의 결정 (예: 아키텍처 + 보안 + 성능) | 각 facet을 독립 레코드로 저장하고 공유 `group_id`를 부여 |

### build_phases() 내부 분기

`record_builder.build_phases(raw_event, detection, pre_extraction=...)` 호출 시:

| 조건 | 분기 | 결과 |
|------|------|------|
| `pre_extraction` 없음 (LLM도 없음) | `build()` | 최소 single 레코드 |
| `pre_extraction` 있음 + `is_multi_phase == False` | `_build_single_record_from_extraction()` | agent 추출 기반 single 레코드 |
| `pre_extraction` 있음 + `is_multi_phase == True` | `_build_multi_record_from_extraction()` | 2+ 레코드 (phase_chain 또는 bundle) |

Multi-record 시:
- 각 phase에 독립 DecisionRecord 생성
- 공유 `group_id` (`grp_YYYY-MM-DD_domain_slug`)
- `phase_seq` (0-indexed), `phase_total`
- Record ID suffix: phase_chain → `_p0, _p1`, bundle → `_b0, _b1`
- `reusable_insight`: agent JSON의 `group_summary` 또는 `reusable_insight`에서 매핑

## 3. 캡처 플로우 — Legacy 3-Tier (드롭 대상)

이 경로는 `extracted`가 전달되지 **않고** API 키가 구성되어 있을 때 실행된다.
주로 `agents/scribe/server.py`의 Slack/Notion 웹훅 인제스션 서비스를 위해
존재한다 — 이 서비스는 원시 메시지를 받아서 전부 스스로 처리해야 한다.

```
raw text
    │
    ▼
Tier 1 — scribe/detector.py
    pattern_cache.pattern_similarity(text)
    → patterns/capture-triggers.md에서 파싱한 18+개의 사전 임베딩된
      capture trigger와의 cosine 비교
    임계값: scribe.similarity_threshold (기본 0.35, 넓은 그물)
    │
    │ 임계값 미만 → drop
    │
    ▼
Tier 2 — scribe/tier2_filter.py
    LLM 정책 필터 (Claude Haiku via llm_client)
    ~200 토큰: 이게 저장할 가치가 있는 조직 지식인가?
    │
    │ 거부 → drop
    │
    ▼
Tier 3 — scribe/llm_extractor.py
    LLM 필드 추출 (Claude Sonnet via llm_client)
    ~500–2048 토큰: 구조화된 DecisionRecord JSON을 emit
    │
    ▼
scribe/record_builder.py → DecisionRecord
    │
    ├── record.confidence < scribe.auto_capture_threshold (0.7) →
    │       scribe/review_queue.py → ~/.rune/review_queue.json
    │       (저장 전 인간 승인 필요)
    │
    ▼
(여기서부터는 agent-delegated와 동일: embed → novelty → insert → log)
```

**Tier 1 + Tier 2 + Tier 3 경로 전체를 드롭한다.** `detector.py`,
`pattern_parser.py`, `tier2_filter.py`, `llm_extractor.py`, `review_queue.py`,
`patterns/capture-triggers.md`, 그리고 `agents/scribe/server.py` +
`handlers/` 웹훅 서비스 전체가 포함된다. 세부 drop 목록은
[03-feature-inventory.md](03-feature-inventory.md) 참고.

## 4. 리콜 플로우

단일 recall 경로다. 명시적 `/rune:recall` 커맨드뿐 아니라, 에이전트가
decision-rationale 류 질문을 감지하면 자동으로도 호출된다. 에이전트는 원시
결과만 요청하거나(`agent-delegated synthesis`) 서버사이드 LLM 합성을 요청할
수 있다.

```
MCP 호출: recall(query, topk=10, domain?, status?, since?)
    │
    ▼
mcp/server/server.py::tool_recall
    │
    ├── 게이트: state == "active" 필수
    ├── _ensure_pipelines() 대기
    │
    ▼
retriever/query_processor.py::parse()                     [query_processor.py:187]
    │
    ├── 언어 감지: 영어 → regex 경로,
    │              비영어 → (선택적) LLM 경로
    ├── 의도 분류: 8종
    │   (rationale, implementation, security, performance,
    │    historical, team, definition, other)
    ├── 엔티티 추출 (경량 NER)
    ├── 시간 범위 추출 (LAST_WEEK/MONTH/QUARTER/YEAR)
    ├── 쿼리 확장: 최대 5개의 expanded_queries
    │
    ▼
ParsedQuery → retriever/searcher.py::search()
    │
    ▼
[Stage 2] 멀티 쿼리 벡터 검색                             [searcher.py:106-151]
    상위 3개의 expanded_queries 각각에 대해:
        vec = embedding_service.embed(q)
        encrypted_scores = envector.score(vec)
        top-k = vault.DecryptScores(encrypted_scores)
        metadata_ct = envector.remind([row_ids])
        metadata = vault.DecryptMetadata(metadata_ct)
    record_id로 dedup, 첫 등장 유지 (최고 점수가 아님; 이후 score 내림차순 정렬)
    │
    ▼
[Stage 3] Phase chain 확장                                [searcher.py:306-365]
    group_id를 가진 결과에 대해 (phase_chain, bundle 모두 group_id 사용),
    누락된 siblings를 추가 envector 쿼리로 가져옴 (최대 2 체인)
    │
    ▼
[Stage 4] 그룹 조립                                       [searcher.py:178-226]
    phase siblings와 bundles 수집, phase_seq 순으로 정렬,
    standalone 결과와 best score 기준으로 interleave
    │
    ▼
[Stage 5] 메타데이터 필터                                 [searcher.py:228-252]
    클라이언트 사이드의 domain / status / since 필터링
    (best-effort — 결과 수가 topk 아래로 떨어질 수 있음)
    │
    ▼
[Stage 6] 시간 범위 필터                                  [searcher.py:523-559]
    감지된 time window 밖의 결과 제거
    ※ Timestamp 파싱: ISO 8601 문자열 또는 Unix float 지원.
      누락/파싱 실패 시 **포함** (배제 아님) — optimistic fallback.
      Recency 가중에서도 동일: 누락 시 age_days=0 (감쇠 없음).
    │
    ▼
[Stage 7] (FHE round-trip은 위의 per-query 호출 안에서 이미 일어남)
    │
    ▼
[Stage 8] Recency 가중 + 재랭킹                           [searcher.py:273-300]
    adjusted_score = (0.7 * raw_score + 0.3 * recency_decay(half-life 90d))
                     * status_multiplier (accepted > proposed > superseded > reverted)
    재정렬
    ※ Dedup 시맨틱: 같은 record_id가 여러 expanded query에서 나오면
      **첫 등장 기준** (최고 점수가 아님). 원본 쿼리는 expanded_queries에
      없을 때만 별도 검색.
    │
    ▼
[Stage 9] 합성                                             [synthesizer.py:142-175]
    │
    ├── agent-delegated 모드 (프라이머리 MVP 경로):
    │       raw 결과 + confidence + related_queries 리턴
    │       호출한 에이전트가 응답을 작성
    │
    └── 서버 사이드 모드 (legacy-ish지만 여전히 live):
            if llm.anthropic_api_key 세팅됨:
                prompt = templates.recall_prompt(records, query)
                answer = claude_sonnet(prompt)  # confidence + sources 추출
            else:
                answer = markdown_table(records)  # EN/KO/JA 템플릿
    ※ Confidence 계산 (server.py:393-412):
      top 5 결과에 대해:
        position_weight = 1.0 / (i + 1)
        certainty_weight = {supported: 1.0, partially_supported: 0.6, unknown: 0.3}
        weight = position × certainty × score
      confidence = min(1.0, sum(weights) / 2.0)
    │
    ▼
return {
  "query": query,
  "results": [...],                # 원시 DecisionRecord dict + 점수
  "answer": "..." | null,          # 서버 사이드 합성 모드에서만 채워짐
  "confidence": float,             # 재랭킹 이후
  "sources": [record_ids],         # 서버 사이드 합성 모드에서만 채워짐
  "warnings": [...],               # 예: "filtered below topk"
  "related_queries": [...],
}
```

**단일 recall에서의 FHE 접점**:

| 단계 | 어디서 실행 | 누가 무엇을 보는가 |
|---|---|---|
| 쿼리 임베딩 | 개발자 머신 (EmbeddingAdapter) | plaintext 쿼리 → plaintext 벡터 |
| 벡터 암호화 | 개발자 머신 (pyenvector SDK) | plaintext 벡터 → FHE ciphertext (public key만 사용) |
| 유사도 스코어링 | enVector Cloud | FHE ciphertext만; 동형 계산 |
| 점수 복호화 | Rune-Vault (비밀키) | FHE 점수 ciphertext → plaintext top-k |
| 메타데이터 페치 | enVector Cloud | AES ciphertext만 |
| 메타데이터 복호화 | Rune-Vault (per-agent DEK) | AES ciphertext → plaintext DecisionRecord JSON |
| 합성 | 개발자 머신 *또는* Anthropic API | plaintext DecisionRecord → plaintext 응답 |

## 5. 유틸리티 / 진단 플로우

### `vault_status`
Vault `GetPublicKey`를 호출해 도달 가능성을 확인하고 번들의 `index_name`을
읽는다. enVector I/O 없음. 연결 상태 + 팀 인덱스 이름을 리턴.

### `diagnostics`
전체 헬스 프로브: Vault 도달 가능성, 키 존재 여부, 임베딩 모델 로드 여부,
enVector round-trip 지연. `/rune:status`용 구조화된 리포트를 리턴.

### `capture_history`
`~/.rune/capture_log.jsonl`을 역시간순으로 읽는다. `limit`, `domain`,
`since` 필터 지원. **네트워크 호출 없음.** 이 머신이 캡처한 로컬 로그일 뿐,
팀 메모리 전체 뷰가 아니다.

### `delete_capture`
Soft delete: 레코드 상태를 `reverted`로 표시하고, 강등된 점수로 재삽입한다.
원래 ciphertext는 enVector에 그대로 남는다 (진짜 삭제는 별도 관리 작업).
retriever의 status 승수가 `reverted`를 아래로 눌러 대부분 결과 페이지에서
밀려나게 한다.

### `reload_pipelines`
`~/.rune/config.json`을 다시 읽고, scribe + retriever 파이프라인을 재초기화,
enVector 연결을 pre-warm. `/rune:activate`(상태 전환 검증)와 `/rune:configure`
이후(자격 증명 변경) 사용된다.

## 6. 시작 / 부트스트랩 플로우

```
scripts/bootstrap-mcp.sh
    │
    ├── 플러그인 루트 감지 (env, 알려진 캐시 경로, cwd walk-up)
    │
    ├── 4단계 self-heal (bootstrap-mcp.sh L37-136, 2026-04-17 실측):
    │       1. VIRTUAL_ENV 환경변수 unset (L41) — 외부 venv pip shebang 오염 사전 차단
    │       2. Python 버전 불일치 감지 (L53-67) — interpreter vs site-packages 버전 차이 시 venv 재생성
    │       3. Pip shebang 오염 감지 (L69-96) — Claude Code 플러그인 캐시 복사로 경로 깨진 shebang 재작성 (sed BSD/GNU 분기)
    │       4. Fastembed .incomplete 캐시 정리 (L117-136) ⚠️ **STALE 방어 코드**
    │          - 과거 fastembed가 기본 백엔드였던 시절의 잔재
    │          - 현재 production은 sbert + Qwen3-0.6B이라 fastembed 모델 다운로드가 발생하지 않음
    │          - sentence-transformers/HuggingFace 캐시 쪽 incomplete 방어가 오히려 필요
    │
    ├── pip install -r requirements.txt (22 패키지, root requirements.txt 실측)
    │       - fastmcp, pyenvector, sentence-transformers, fastembed(미사용), numpy
    │       - fastapi, uvicorn, slack-sdk (agents/scribe/server.py 웹훅 서비스용)
    │       - anthropic, openai, google-generativeai, langdetect
    │       - prometheus-client, python-json-logger, psutil, httpx, pydantic, 등
    │
    ├── SETUP_ONLY=1 → install-deps 모드로 진입 (backward compat, L23-25)
    ├── --local-only 플래그 → deps 설치 skip, preflight only
    ├── --install-deps 플래그 → deps 설치 + server 실행 안 함
    │
    ├── L144-162: config fallback — ~/.rune/config.json 부재 + RUNEVAULT_* env var 존재 시
    │             최소 config 자동 생성 (vault + state="dormant" + metadata.configVersion="1.1")
    │
    └── exec .venv/bin/python3 mcp/server/server.py --mode stdio
            │
            ▼
    mcp/server/server.py::main() (L1805-2001 실측)
            │
            ├── argparse — ENVECTOR_CONFIG/ENDPOINT/KEY_* env vars 로딩
            │
            ├── ENVECTOR_CONFIG가 가리키는 config.json에서 vault 블록 읽어 RUNEVAULT_* env var로 승격
            │
            ├── VAULT_CONFIGURED면 fetch_keys_from_vault(RUNEVAULT_ENDPOINT, RUNEVAULT_TOKEN, ...)
            │       → EncKey.json, EvalKey.json 디스크 캐시
            │       → index_name, key_id, agent_id, agent_dek, envector_endpoint, envector_api_key 추출
            │
            ├── EnVectorSDKAdapter 초기화 (실패 시 degraded 모드로 경고)
            ├── VaultClient 초기화
            │
            ├── **MCPServerApp(...) 생성자 — @self.mcp.tool 데코레이터로 8개 tool 모두 무조건 등록**
            │       ⚠ 중요: 조건부 등록 아님. dormant/active와 무관하게 항상 8개 전부 등록됨.
            │       runtime에 각 tool 본문이 state 체크로 거절 (self._ensure_pipelines + dormant 에러)
            │
            ├── threading.Thread(target=app._init_pipelines_background).start()
            │       (백그라운드 파이프라인 초기화. _ensure_pipelines가 120s 대기.)
            │
            └── FastMCP.run_stdio() + SIGINT/SIGTERM 핸들러

### _init_pipelines (L1520-1798) 내부 순서
    1. L1541: load_rune_config() 재로드
    2. L1544-1547: **state != "active" 즉시 return** (self._scribe = None, self._retriever = None)
       → dormant 상태면 embedding 모델 로드도, Vault fetch도 skip
    3. L1549-1552: EmbeddingService 초기화 (active일 때만)
    4. L1559-1595: Vault fetch → enVector credentials 추출
    5. L1584-1591: save_config() 호출 (envector.endpoint/api_key 캐시)
       → 이 순간 config.json이 3섹션(vault+state+metadata) → 7섹션으로 확장됨
    6. L1631-1642: EnVectorSDKAdapter 재초기화 (새 endpoint/api_key 반영)
    7. L1702: RecordBuilder(llm_extractor=llm_extractor or None) — LLM key 없으면 llm_extractor=None
    8. L1707-1726: has_llm_key면 legacy Tier1/2 pipeline (pattern_cache, detector, tier2_filter) 초기화
    9. L1728-1735: self._scribe dict 완성
    10. L1748-1772: retriever pipeline (QueryProcessor/Searcher/Synthesizer) 초기화
```

파이프라인 init에 의존하는 tool 호출은 `_ensure_pipelines()` (L1503-1518,
120s 타임아웃)에서 대기한다. `vault_status`, `diagnostics`, `reload_pipelines`는
대기 로직을 별도 실행하지 **않는다** — 파이프라인이 끝나기 전에도 실행되어
사용자가 startup 정지를 진단할 수 있도록 한다.

### save_config의 사이드 이펙트 (2026-04-17 신규 확인)

`_init_pipelines`가 Vault fetch 성공 시 `save_config()`를 호출(L1585-1591)하는데,
`config.py::save_config` 구현이 **dataclass 전체 섹션을 unconditional하게 직렬화**
한다(L311-342). 결과:

- `/rune:configure`로 쓰여진 T1 상태의 3섹션 파일(`vault + state + metadata`)이
  `/rune:activate` 후 첫 Vault fetch 시 T2 상태의 7섹션으로 확장됨
  (envector + embedding + llm + scribe + retriever 추가)
- `metadata` 섹션(configVersion/lastUpdated/installedFrom)은 **save_config이
  보존하지 않아 증발**
- `configure.md:98`의 "enVector credentials are no longer stored locally" 주석과
  drift. git log 참고: `5e5033e`에서 EnVectorConfig 제거 → `95abbbb`에서 envector
  캐싱을 다시 도입. 주석만 stale

## 7. 슬래시 커맨드 → MCP Tool 매핑

| 슬래시 커맨드 (Claude) | 슬래시 커맨드 (Codex) | 호출되는 MCP tool | 비고 |
|---|---|---|---|
| `/rune:capture <text>` | `$rune capture <text>` | `capture` with `extracted` JSON | agent-delegated 모드 |
| `/rune:recall <query>` | `$rune recall <query>` | `recall` | agent-delegated 합성이 기본값 |
| `/rune:configure` | `$rune configure` | 없음 (파일 I/O만) | `~/.rune/config.json` 작성 |
| `/rune:activate` | `$rune activate` | `reload_pipelines` | 인프라 검증; active로 전이 |
| `/rune:deactivate` | `$rune deactivate` | `reload_pipelines` | dormant로 전이 |
| `/rune:reset` | `$rune reset` | 없음 | `~/.rune/config.json` 삭제 |
| `/rune:status` | `$rune status` | `diagnostics` + `vault_status` | |
| `/rune:history` | `$rune history` | `capture_history` | |
| `/rune:delete <id>` | `$rune delete <id>` | `delete_capture` + `capture_history` | |

Claude Code 버전은 `commands/claude/*.md`에, Codex 버전은
`commands/rune/*.toml`에 위치한다. 프롬프트 내용은 ~99% 공유되며, 차이는
에러 메시지 문구 (`/rune:...` vs `$rune ...`)와 Codex 전용 플러그인 루트
감지 정도다.
