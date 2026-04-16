# Rune AS-IS: 에이전트-서버 인터페이스 계약

이 문서는 에이전트 프롬프트(scribe.md, retriever.md)와 MCP 서버(server.py) 사이의
인터페이스 계약을 정의한다. 원래 서베이에서 서버 구현 중심으로 분석하면서
에이전트 측 계약이 누락되었다.

이 계약은 Go 마이그레이션 시 반드시 보존해야 한다:
- 서버가 이 스키마를 바꾸면 에이전트 프롬프트가 깨진다
- 에이전트 프롬프트를 바꾸면 서버 파싱이 깨진다

---

## 1. Capture 입력: Agent JSON 스키마

에이전트(scribe)가 `capture` MCP 툴을 호출할 때 `extracted` 파라미터로 전달하는
JSON 스키마. 세 가지 포맷이 존재한다.

**MCP 툴 파라미터** (`mcp/server/server.py:698-704`):

| 파라미터 | 타입 | 필수 | 기본값 | 설명 |
|---|---|---|---|---|
| `text` | str | Y | - | 캡처 대상 원문 텍스트 |
| `source` | str | N | `"claude_agent"` | 소스 식별자 |
| `user` | str | N | `None` | 작성자 |
| `channel` | str | N | `None` | 채널/컨텍스트 |
| `extracted` | str | N | `None` | 에이전트가 추출한 JSON **문자열** (agent-delegated 모드) |

### 1.1 공통 필드: `tier2` (에이전트 게이트)

모든 포맷에 최상위로 포함되는 에이전트 측 캡처 판정 필드.
이것은 레거시 Tier 2 LLM 필터가 **아니라**, 에이전트가 직접 수행하는 정책 평가다.

```
tier2: {
  capture: bool,      // true → 캡처 진행, false → 거절
  reason:  str,       // 캡처/거절 사유 (1문장)
  domain:  str        // 도메인 분류 (아래 enum 참조)
}
```

**서버 처리** (`server.py:1244-1251`): `tier2.capture == false`이면 즉시 리턴하고
서버 파이프라인을 일절 실행하지 않는다.

**Domain enum** (`agents/claude/scribe.md:76`, 전 에이전트 공통):
`architecture`, `security`, `product`, `exec`, `ops`, `design`, `data`, `hr`,
`marketing`, `incident`, `debugging`, `qa`, `legal`, `finance`, `sales`,
`customer_success`, `research`, `risk`, `general`

### 1.2 Format A: Single Decision

단일 의사결정. `phases` 필드 없음.
(`agents/claude/scribe.md:79-92`, `server.py:1294-1324`)

| 필드 | 타입 | 필수 | 서버 처리 |
|---|---|---|---|
| `tier2` | object | Y | 위 참조 |
| `title` | str | Y | `[:60]` 절단 (`server.py:1309`) |
| `reusable_insight` | str | Y | 256-768 토큰 권장. 임베딩의 **주 대상** — 리콜 품질을 결정하는 핵심 필드. markdown 금지, self-contained (`server.py:1319`, `embedding.py:21-30`) |
| `rationale` | str | Y | 그대로 저장 |
| `problem` | str | N | 그대로 저장 |
| `alternatives` | str[] | N | 빈 문자열 필터 (`server.py:1312`). 서버에서 길이 제한 없음 |
| `trade_offs` | str[] | N | 빈 문자열 필터 (`server.py:1313`). 서버에서 길이 제한 없음 |
| `status_hint` | str | N | `.lower()` 정규화. 값: `accepted` \| `proposed` \| `rejected` (`server.py:1314`) |
| `tags` | str[] | N | `.lower()` 정규화, 빈 문자열 필터 (`server.py:1315`) |
| `confidence` | float | N | `[0.0, 1.0]` 클램프 (`server.py:1258-1259`). 0.7+ 권장 |

**선택적 코드 컨텍스트 필드** (`agents/claude/scribe.md:96-100`):

| 필드 | 타입 | 설명 |
|---|---|---|
| `evidence_type` | str | `code_change` \| `git_bisect` \| `benchmark` \| `error_trace` \| `runtime_observation` |
| `evidence_snippet` | str | 증거 코드/로그 (50줄 이하) |

### 1.3 Format B: Multi-Phase (Phase Chain)

순차적 추론 과정을 여러 phase로 기록.
(`agents/claude/scribe.md:103-134`, `server.py:1271-1293`)

| 필드 | 타입 | 필수 | 서버 처리 |
|---|---|---|---|
| `tier2` | object | Y | 위 참조 |
| `group_title` | str | Y | `[:60]` 절단 (`server.py:1286`) |
| `group_type` | str | Y | `"phase_chain"` 고정 |
| `reusable_insight` | str | Y | 체인 전체를 요약. 임베딩 대상 (`server.py:1288`) |
| `status_hint` | str | N | `.lower()` 정규화 |
| `tags` | str[] | N | `.lower()`, 빈 문자열 필터 |
| `confidence` | float | N | `[0.0, 1.0]` 클램프 |
| `phases` | object[] | Y | **2-7개**. `[:7]` 슬라이스 (`server.py:1275`) |

**Phase 객체 스키마** (`PhaseExtractedFields`, `server.py:1276-1284`):

| 필드 | 타입 | 서버 처리 |
|---|---|---|
| `phase_title` | str | `[:60]` 절단 |
| `phase_decision` | str | 그대로 |
| `phase_rationale` | str | 그대로 |
| `phase_problem` | str | 그대로 |
| `alternatives` | str[] | 빈 문자열 필터 |
| `trade_offs` | str[] | 빈 문자열 필터 |
| `tags` | str[] | `.lower()`, 빈 문자열 필터 |

### 1.4 Format C: Bundle

하나의 의사결정에 대한 다각적 분석 (핵심 결정 + 대안 분석 + 임팩트 등).
(`agents/claude/scribe.md:137-168`, `server.py:1271-1293` — phase_chain과 동일 경로)

Format B와 구조가 동일하되 `group_type: "bundle"`, phases는 **2-5개**.
첫 번째 phase는 항상 `"Core Decision"` (`agents/claude/scribe.md:229`).

**Code-Context Bundle 변형** (`agents/claude/scribe.md:171-214`):
phase 레벨에 `evidence_snippet` (50줄 이하) 추가 가능. `evidence_type`은 최상위에 지정.

### 1.5 Rejection Format

Step 1 정책 평가에서 캡처 거절 시:
```json
{
  "tier2": {"capture": false, "reason": "Casual discussion without decision", "domain": "general"}
}
```
서버는 `tier2.capture == false`를 확인하고 즉시 `{ok: true, captured: false}` 리턴
(`server.py:1246-1251`).

---

## 2. Capture 응답: 서버 → 에이전트

### 2.1 성공 (Single Record)

단일 레코드 캡처 성공 시 (`server.py:1387-1397`):

```json
{
  "ok": true,
  "captured": true,
  "record_id": "dec_2026-04-16_arch_postgres",
  "summary": "Adopt PostgreSQL for JSON support",
  "domain": "architecture",
  "certainty": "supported",
  "mode": "agent-delegated",
  "novelty": {
    "class": "novel",
    "score": 0.85,
    "related": [
      {"id": "dec_...", "title": "...", "similarity": 0.342}
    ]
  }
}
```

### 2.2 성공 (Multi-Record: phase_chain / bundle)

2개 이상의 레코드가 생성된 경우 추가 필드 (`server.py:1398-1401`):

```json
{
  "ok": true,
  "captured": true,
  "record_id": "dec_..._p0",
  "summary": "...",
  "domain": "...",
  "certainty": "...",
  "mode": "agent-delegated",
  "novelty": {"class": "...", "score": 0.0, "related": []},
  "record_count": 3,
  "group_id": "grp_...",
  "group_type": "phase_chain"
}
```

### 2.3 거절: 에이전트 판정 (tier2)

에이전트가 `tier2.capture: false`로 보낸 경우 (`server.py:1247-1251`):

```json
{
  "ok": true,
  "captured": false,
  "reason": "Agent rejected: Casual discussion without decision"
}
```

### 2.4 거절: Near-Duplicate

Novelty 체크에서 near_duplicate 판정 시 (`server.py:1362-1369`):

```json
{
  "ok": true,
  "captured": false,
  "reason": "Near-duplicate — virtually identical insight already stored",
  "novelty": {
    "class": "near_duplicate",
    "score": 0.05,
    "related": [
      {"id": "dec_...", "title": "...", "similarity": 0.97}
    ]
  }
}
```

**Novelty 분류 임계값** (런타임 기본값: `server.py::_classify_novelty()` 인자):

| 클래스 | similarity 범위 | 동작 |
|---|---|---|
| `near_duplicate` | >= 0.95 | **캡처 차단** (유일한 blocking 케이스) |
| `related` | >= 0.7 | annotation only, 캡처 진행 |
| `evolution` | >= 0.3 | annotation only, 캡처 진행 |
| `novel` | < 0.3 | annotation only, 캡처 진행 |

※ `embedding.py` 상수는 0.4/0.7/0.93이지만, `server.py::_classify_novelty()`의
기본 인자(0.3/0.7/0.95)가 런타임에서 오버라이드한다. 자세한 내용은
02-flows.md §2 참고.

### 2.5 에러

구조화된 에러 응답 (`mcp/server/errors.py:93-118`):

```json
{
  "ok": false,
  "error": {
    "code": "VAULT_CONNECTION_ERROR",
    "message": "Cannot reach Vault",
    "retryable": true,
    "recovery_hint": "Run /rune:status for diagnostics."
  }
}
```

**에러 코드 테이블** (`errors.py:19-90`):

| 코드 | retryable | 발생 조건 |
|---|---|---|
| `VAULT_CONNECTION_ERROR` | true | Vault 서버 연결 불가 |
| `VAULT_DECRYPTION_ERROR` | false | Vault 토큰 만료/권한 없음 |
| `ENVECTOR_CONNECTION_ERROR` | true | enVector 엔드포인트 연결 불가 |
| `ENVECTOR_INSERT_ERROR` | true | enVector 저장 실패 (일시적) |
| `PIPELINE_NOT_READY` | false | 파이프라인 미초기화 |
| `INVALID_INPUT` | false | 입력 파라미터 오류 |
| `INTERNAL_ERROR` | false | 예상치 못한 예외 (fallback) |

---

## 3. Batch Capture 입력/응답

세션 종료 시 미캡처된 결정을 한꺼번에 처리하는 툴.

### 3.1 입력

**MCP 툴 파라미터** (`server.py:819-824`):

| 파라미터 | 타입 | 필수 | 기본값 | 설명 |
|---|---|---|---|---|
| `items` | str | Y | - | JSON 배열 문자열. 각 항목은 `capture`의 `extracted`와 동일한 포맷 |
| `source` | str | N | `"claude_agent"` | 소스 식별자 |
| `user` | str | N | `None` | 작성자 |
| `channel` | str | N | `None` | 채널/컨텍스트 |

### 3.2 응답

**정상 응답** (`server.py:889-896`):

```json
{
  "ok": true,
  "total": 3,
  "results": [
    {"index": 0, "title": "...", "status": "captured", "novelty": "novel"},
    {"index": 1, "title": "...", "status": "near_duplicate", "novelty": "near_duplicate"},
    {"index": 2, "title": "...", "status": "error", "error": "Insert failed: ..."}
  ],
  "captured": 1,
  "skipped": 1,
  "errors": 1
}
```

**Status enum** (`server.py:861-869`):

| status | 의미 |
|---|---|
| `captured` | 정상 캡처 완료 |
| `near_duplicate` | novelty 체크에서 중복으로 스킵 |
| `skipped` | 에이전트 거절 등으로 스킵 |
| `error` | 처리 중 예외 발생. `error` 필드에 메시지 포함 |

각 항목은 독립적으로 처리된다 — 하나의 실패가 나머지를 중단하지 않음
(`server.py:848-883`).

**빈 배열 입력 시** (`server.py:845-846`):
```json
{"ok": true, "total": 0, "results": [], "captured": 0, "skipped": 0, "errors": 0}
```

---

## 4. Recall 입력/응답

### 4.1 입력

**MCP 툴 파라미터** (`server.py:910-916`):

| 파라미터 | 타입 | 필수 | 기본값 | 유효 범위 | 설명 |
|---|---|---|---|---|---|
| `query` | str | Y | - | - | 자연어 질문 또는 토픽 (문장/키워드 모두 가능) |
| `topk` | int | N | 5 | max 10 (`server.py:931`) | 검색 결과 수 |
| `domain` | str | N | `None` | Domain enum | 도메인 필터 |
| `status` | str | N | `None` | `accepted` \| `proposed` \| `superseded` | 상태 필터 |
| `since` | str | N | `None` | ISO date (e.g. `"2026-01-01"`) | 날짜 필터 |

### 4.2 응답: Agent-Delegated (primary path)

에이전트가 직접 합성하는 경로 — LLM 키 불필요 (`server.py:953-990`):

```json
{
  "ok": true,
  "found": 3,
  "results": [
    {
      "record_id": "dec_2026-01-15_arch_postgres",
      "title": "Adopt PostgreSQL",
      "content": "The team decided to adopt PostgreSQL...",
      "domain": "architecture",
      "certainty": "supported",
      "score": 0.87,
      "group_id": "grp_...",
      "group_type": "phase_chain",
      "phase_seq": 0,
      "phase_total": 3
    }
  ],
  "confidence": 0.72,
  "sources": [
    {"record_id": "...", "title": "...", "domain": "...", "certainty": "...", "score": 0.87}
  ],
  "synthesized": false
}
```

**결과 항목 필드** (`server.py:957-970`):

| 필드 | 타입 | 조건 |
|---|---|---|
| `record_id` | str | 항상 |
| `title` | str | 항상 |
| `content` | str | 항상 (`payload_text` 매핑) |
| `domain` | str | 항상 |
| `certainty` | str | 항상. `supported` \| `partially_supported` \| `unknown` |
| `score` | float | 항상. similarity score |
| `group_id` | str | 그룹 레코드인 경우만 (`server.py:965-969`) |
| `group_type` | str | 그룹 레코드인 경우만 |
| `phase_seq` | int | 그룹 레코드인 경우만 |
| `phase_total` | int | 그룹 레코드인 경우만 |

**최상위 메타 필드**:
- `confidence` (float): 상위 5개 결과의 certainty-weighted score (`server.py:393-412`)
- `sources` (array): 상위 5개 결과의 요약 (`server.py:972-981`)
- `synthesized` (bool): agent-delegated path에서 항상 `false` (`server.py:989`)

**참고**: `adjusted_score`와 `reusable_insight`는 서버 내부
`SearchResult` 객체 (`agents/retriever/searcher.py:54,53`)에는 존재하지만
MCP 응답에는 포함되지 않는다. 에이전트는 `score`만 받는다.

### 4.3 응답: Server-Side Synthesis (fallback)

LLM 키가 있을 때의 서버 사이드 합성 경로 (`server.py:993-1003`):

```json
{
  "ok": true,
  "found": 3,
  "answer": "The team decided to adopt PostgreSQL because...",
  "confidence": 0.72,
  "sources": [...],
  "warnings": ["Limited evidence for secondary claim"],
  "related_queries": ["What alternatives were considered?"],
  "synthesized": true
}
```

이 경로는 LLM API 키가 설정된 경우에만 활성화되며, agent-delegated 모드에서는
에이전트가 직접 합성하므로 primary path (`synthesized: false`)가 사용된다.

### 4.4 에러

Capture와 동일한 구조화된 에러 응답 (Section 2.5 참조).
추가로 `VAULT_DECRYPTION_ERROR` 발생 시 서버가 자동으로 dormant 상태로 전환한다
(`server.py:1007`).

---

## 5. Scribe 행동 규칙 (CLAUDE.md + scribe.md에서 추출)

### 5.1 캡처 트리거 패턴

에이전트가 캡처해야 하는 패턴 (`agents/claude/scribe.md:31-53`, 15개 주요 항목 + 6개 코딩 세션 하위 항목 = 21종):

**의사결정 계열**:
1. 구체적 결정 + 근거 (기술 선택, 아키텍처, 프로세스 변경)
2. 정책/표준 수립 또는 변경
3. 트레이드오프 분석 또는 대안 거절
4. 인시던트/실패/디버깅에서의 교훈
5. 팀에 영향을 미치는 합의/커밋먼트

**운영 계열**:
6. 인시던트 포스트모템 결과, 근본 원인 분석, 시정 조치
7. 디버깅 브레이크스루: 근본 원인 식별, 수정 적용, 워크어라운드
8. 버그 트리아지 결과: 심각도, 담당자, 수정 전략
9. QA 발견으로 테스트 전략/인수 기준 변경

**비즈니스 계열**:
10. 법무/컴플라이언스 결정, 규제 해석
11. 예산 배분, 가격 변경, 비용 최적화 결정
12. 영업 인텔리전스: 딜 결과, 경쟁 인사이트, 고객 요구사항
13. 고객 에스컬레이션 해결, 이탈 분석 인사이트
14. 연구 결과, 실험 결과, PoC 결론
15. 리스크 평가 + 완화 전략

**코딩 세션 계열** (Agentic coding discoveries):
16. Root cause discovery: 버그 원인 식별 + 수정 접근법
17. Performance insight: 병목 발견, 최적화 적용, before/after
18. Problem reframing (문제 재정의로 해결 방향 전환)
19. Architecture pivot (구조적 방향 전환 + 근거)
20. Non-obvious dependency (숨겨진 의존성 발견)
21. Pattern establishment (재사용 가능한 패턴 정립)

### 5.2 DO NOT CAPTURE 규칙

캡처 금지 대상 7종 (`agents/claude/scribe.md:55-61`):

1. 잡담, 인사, 소셜 채팅
2. 답변/결정 없는 질문
3. 결정/인사이트 없는 상태 업데이트 ("still working on X")
4. 커밋먼트 없는 막연한 의견 ("maybe we should...")
5. 결론 없는 초안/WIP 논의
6. 결정/학습 없는 루틴 알림/배포
7. 중대한 결정 없는 루틴 코드 변경 (타입 수정, 변수 리네임, 의존성 범프)

**추가 원칙** (`agents/claude/scribe.md:261`): 캡처 여부가 불확실할 때는
캡처하지 **않는** 쪽으로 판단 — false negative은 `/rune:capture`로 복구 가능하지만,
false positive은 사용자 신뢰를 훼손한다.

### 5.3 자동 스폰 (Proactive Scribe)

CLAUDE.md에 정의된 자동 캡처 트리거 (`CLAUDE.md:24-43`).
Rune state가 `"active"`일 때만 작동한다.

**5가지 자동 스폰 트리거**:
1. 대안 중 선택이 이루어짐 ("A로 가자", "let's go with X")
2. 트레이드오프를 저울질하고 커밋 ("X의 단점이 있지만 Y 때문에 감수")
3. 전략/방향이 확인됨 ("이 방향이 맞아", "this approach works")
4. 교훈/인사이트가 결정화 ("안 된 이유는...", "the root cause was...")
5. 프레임워크, 프로세스, 표준이 수립됨

**스폰 방법** (`CLAUDE.md:33-36`):
- `Agent` 툴: `subagent_type: "rune:scribe"`, `run_in_background: true`
- 프롬프트에는 관련 대화 발췌만 포함 (전체 컨텍스트 X)
- 메인 대화를 중단하지 않음
- 사용자에게 캡처 사실을 알리지 않음 (명시적 질문 시에만)

**3가지 비캡처 조건** (`CLAUDE.md:38-40`):
1. 커밋먼트 없는 브레인스토밍 진행 중 (옵션 나열만, 선택 없음)
2. 질문, 상태 업데이트, 일상 대화
3. 가설적이거나 보류된 결정 ("maybe later", "let's revisit")

**세션 종료 스윕** (`CLAUDE.md:43`):
사용자가 종료 신호를 보내면 (goodbye, thanks, wrapping up) 대화 내
미캡처된 중요 결정을 검토하고 `batch_capture`로 일괄 제출.

### 5.4 비영어 입력 번역 규칙

에이전트 프롬프트의 번역 규칙 (`agents/claude/scribe.md:233`):

> 원본 메시지가 비영어인 경우, **추출된 필드 값 전체를 영어로 번역**한다.
> 원본 텍스트는 `text` 파라미터에 그대로 전달된다.

즉 `extracted` JSON의 `title`, `rationale`, `reusable_insight` 등은 영어,
`text` 파라미터는 원문 언어 그대로.

---

## 6. Retriever 행동 규칙 (retriever.md에서 추출)

### 6.1 Recall 트리거

에이전트가 `recall`을 호출해야 하는 7가지 상황 (`agents/claude/retriever.md:32-40`):

1. **직접 질문**: "Why did we choose Redis?"
2. **숙의/옵션 평가**: "We're considering PostgreSQL vs MongoDB"
3. **아키텍처 논의**: "Let's think about the caching layer"
4. **트레이드오프 분석**: "The options are X, Y, Z -- each has pros and cons"
5. **관련 이력이 있는 계획**: "We need to decide on an auth approach"
6. **과거 작업 참조**: "Last time we tried microservices..."
7. **결정 전 컨텍스트 수집**: "Before we commit to this, what's the background?"

**핵심 원칙**: 팀이 결정을 향해 작업 중이거나 조직 메모리가 결과에 영향을 줄 수 있는
토픽을 탐구 중이면 `recall`을 호출한다 — 명시적 질문뿐 아니라 숙의 과정에서도.

### 6.2 합성 규칙

#### Certainty-to-Tone 매핑

에이전트가 결과의 `certainty` 필드에 따라 어조를 조정한다
(`agents/claude/retriever.md:66-72`):

| Certainty | 어조 | 예시 |
|---|---|---|
| `supported` | Confident, assertive | "The team decided to adopt PostgreSQL because..." |
| `partially_supported` | Qualified, hedged | "Based on available evidence, the team likely chose..." |
| `unknown` | Uncertain, caveated | "There's a reference to this, but the context is unclear..." |

#### Confidence 임계값

전체 검색 결과의 `confidence` 값에 따른 제시 방법
(`agents/claude/retriever.md:87-89`):

| confidence | 행동 |
|---|---|
| >= 0.6 | 정상 제시 |
| 0.3 - 0.6 | caveat 추가: "Evidence is limited, but..." |
| < 0.3 | strong caveat: "Very little evidence was found. The following is tentative..." |

#### 출처 귀속 규칙

(`agents/claude/retriever.md:93-103`)

- **결과 있을 때**: "Based on organizational memory:" 또는 "From org memory:"로 시작.
  건수 명시, confidence 레벨 표시.
- **결과 없을 때**: "No relevant records found in organizational memory.
  The following is based on general knowledge only — consider using `/rune:capture`
  to save this discussion if a decision is made."
- **부분적 결과** (confidence < 0.6): "Limited evidence in organizational memory.
  The following combines what was found with general knowledge."

### 6.3 인용 형식

에이전트는 결과의 `record_id`로 인라인 인용한다
(`agents/claude/retriever.md:76-78`):

```
The team adopted PostgreSQL for its superior JSON support
[dec_2024-01-15_arch_postgres]. This was later complemented by
Redis for caching [dec_2024-01-20_arch_redis].
```

#### Phase Chain / Bundle 제시

(`agents/claude/retriever.md:80-84`)

- **Phase chain** (`group_type: "phase_chain"`): `phase_seq` 순서대로 내러티브
  진행으로 제시. "The decision evolved through three phases: first..."
- **Bundle** (`group_type: "bundle"`): 하나의 결정의 여러 측면으로 함께 제시.
  `phase_seq=0`이 핵심 결정, 이후는 보조 디테일.
- 동일 `group_id`를 공유하는 레코드를 함께 그룹핑.

### 6.4 결과 없음 처리

(`agents/claude/retriever.md:109-112`)

- `found == 0`: 관련 레코드가 없음을 사용자에게 고지. 가능하면 대안 쿼리 제안.
- `ok: false`: 에러를 간략 보고.

**관련 쿼리 제안** (`agents/claude/retriever.md:106-108`):
결과가 질문에 부분적으로만 답할 때 후속 쿼리를 제안한다:
> "To learn more, you might also ask: 'What alternatives were considered for the
> caching layer?' or 'What were the performance benchmarks?'"

---

## 7. 에이전트별 차이

세 에이전트(Claude, Gemini, Codex)의 scribe/retriever 프롬프트 비교.

### 7.1 Scribe 차이

| | Claude | Gemini | Codex |
|---|---|---|---|
| 프롬프트 파일 | `agents/claude/scribe.md` | `agents/gemini/scribe.md` | `agents/codex/scribe.md` |
| MCP tool name | `mcp__plugin_rune_envector__capture` | `capture` | `capture` |
| source 값 | `claude_agent` | `gemini_agent` | `codex_agent` |
| batch source 기본값 | `"claude_agent"` | `"gemini_agent"` (defaults to `"claude_agent"`) | `"codex_agent"` (defaults to `"claude_agent"`) |
| 활성화 실패 안내 | `/rune:configure`, `/rune:activate`, `/rune:status` | `/rune:configure`, `/rune:activate`, `/rune:status` | `$rune configure`, `$rune activate`, `$rune status` |
| 캡처 패턴 | 21종 (동일) | 21종 (동일) | 21종 (동일) |
| JSON 스키마 | Format A/B/C (동일) | Format A/B/C (동일) | Format A/B/C (동일) |
| Session-end sweep | batch_capture 지원 | batch_capture 지원 | batch_capture 지원 |

**핵심 차이**: Claude만 `mcp__plugin_rune_envector__` 접두사를 사용한다.
이는 Claude Code의 MCP 플러그인 네이밍 규칙이며, Gemini/Codex는 단순 `capture`/`recall`을 사용한다.
Codex는 슬래시 커맨드(`/rune:...`) 대신 CLI 스타일(`$rune ...`)을 안내한다.

### 7.2 Retriever 차이

| | Claude | Gemini |
|---|---|---|
| 프롬프트 파일 | `agents/claude/retriever.md` | `agents/gemini/retriever.md` |
| MCP tool name | `mcp__plugin_rune_envector__recall` | `recall` |
| recall 필터 | `domain`, `status`, `since` 지원 (`retriever.md:52-59`) | 필터 없음 — `query`와 `topk`만 (`retriever.md:44-48`) |
| 출처 귀속 규칙 | 있음 (`retriever.md:93-103`) | 없음 |
| Codex | retriever 프롬프트 없음 (N/A) | - |

**주의**: Gemini retriever는 두 가지 기능이 Claude 대비 누락되어 있다:
1. recall 호출 시 `domain`, `status`, `since` 필터를 프롬프트에 포함하지 않는다
   (`agents/gemini/retriever.md:44-48`). 서버는 지원하지만 Gemini에게 안내 안 됨.
2. "Source Attribution Rule" 섹션이 없다 (`agents/claude/retriever.md:91-103`).
   Claude는 "Based on organizational memory:" 접두사, 결과 수 명시, confidence 표시,
   결과 없을 때 `/rune:capture` 안내 등을 수행하지만, Gemini는 이 규칙이 없다.

---

## 8. Phase Chain 확장 현황

### 현재 상태 (AS-IS)

Python 코드에서 phase chain 확장은 **구현되어 있고 동작한다**.

`agents/retriever/searcher.py:306-365`의 `_expand_phase_chains()` 메서드가
검색 결과 중 phase chain/bundle에 속하는 레코드를 발견하면:

1. `group_id`를 공유하는 형제(sibling) phase가 결과에 모두 포함되어 있는지 확인
2. 누락된 형제가 있으면 `group_id`로 추가 검색 실행 (topk=10)
3. 같은 그룹의 형제를 `phase_seq` 순서로 정렬하여 삽입
4. 최대 **2개 체인**까지 확장 (`max_chains=2`, `searcher.py:309`)

### 문서 간 모순

`03-feature-inventory.md:60`에서 phase chain 확장을 **DEFER**로 판정:

> Phase chain 확장 | `searcher.py:306` | **DEFER** | nice-to-have; ~200 라인의
> 복잡도. MVP는 평탄한 리스트 리턴; chain 재구성은 v1.1.

이것은 Go MVP의 구현 범위 결정이다. AS-IS 사실은 다음과 같다:

- Python 코드는 phase chain 확장을 **구현하고 있다** (`searcher.py:306-365`)
- retriever 프롬프트는 에이전트에게 phase chain/bundle 결과를 그룹핑하여
  제시하도록 지시한다 (`agents/claude/retriever.md:80-84`)
- 서버는 `group_id`, `group_type`, `phase_seq`, `phase_total`을 recall 응답에
  포함한다 (`server.py:965-969`)

Go MVP에서 이 기능을 DEFER하더라도 **응답 스키마의 그룹 필드는 보존해야 한다**
— 에이전트 프롬프트가 이 필드에 의존하여 합성 규칙을 적용하기 때문이다.
확장 검색 없이도 단일 검색에서 같은 그룹의 여러 phase가 반환될 수 있으며,
에이전트는 이를 `group_id`로 그룹핑한다.
