# Rune 구현 상세: Go 포팅용 데이터 테이블

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스 실측 대조. 주요 교정:
> §1 Intent regex 총 수 **31개** (33 아님). §14 AES 모드 **AES-256-CTR 확정**
> (pyenvector/utils/aes.py:52-58 실측) — Open Question에서 확정으로 상태 전환,
> Go `crypto/cipher.NewCTR` 포팅 스니펫 추가.

이 문서는 Go 구현에 필요한 상세 데이터 테이블, 알고리즘, 패턴 목록을 담는다.
다른 마이그레이션 문서들이 "무엇을" 설명한다면, 이 문서는 "정확히 어떻게"를 설명한다.

> 소스 기준: `agents/retriever/query_processor.py`, `agents/scribe/record_builder.py`,
> `agents/scribe/llm_extractor.py`, `agents/common/schemas/decision_record.py`,
> `agents/retriever/searcher.py`, `mcp/adapter/envector_sdk.py`,
> `mcp/adapter/vault_client.py`, `mcp/server/server.py`

---

## 1. Intent 분류 패턴

`QueryProcessor.INTENT_PATTERNS` (query_processor.py:70-116).

8종의 `QueryIntent` enum이 있으며, `GENERAL`은 패턴 없이 fallback으로 동작한다.

| Intent | Python Name | Regex Patterns |
|--------|-------------|----------------|
| 결정 근거 | `DECISION_RATIONALE` | `why did we (choose\|decide\|go with\|select\|pick)` |
| | | `what was the (reasoning\|rationale\|logic\|thinking)` |
| | | `why .+ over .+` |
| | | `what were the (reasons\|factors)` |
| | | `why (not\|didn't we)` |
| | | `reasoning behind` |
| 기능 이력 | `FEATURE_HISTORY` | `(have\|did) (customers?\|users?) (asked\|requested\|wanted)` |
| | | `feature request` |
| | | `why did we (reject\|say no\|decline)` |
| | | `(how many\|which) customers` |
| | | `customer feedback (on\|about)` |
| 패턴 조회 | `PATTERN_LOOKUP` | `how do we (handle\|deal with\|approach\|manage)` |
| | | `what'?s our (approach\|process\|standard\|convention)` |
| | | `is there (an?\|existing) (pattern\|standard\|convention)` |
| | | `what'?s the (best practice\|recommended way)` |
| | | `how should (we\|I)` |
| 기술 맥락 | `TECHNICAL_CONTEXT` | `what'?s our (architecture\|design\|system) for` |
| | | `how (does\|is) .+ (implemented\|built\|designed)` |
| | | `(explain\|describe) (the\|our) .+ (system\|architecture\|design)` |
| | | `technical (details\|overview) (of\|for)` |
| 보안/컴플라이언스 | `SECURITY_COMPLIANCE` | `(security\|compliance) (requirements?\|considerations?)` |
| | | `what (security\|privacy) (measures\|controls)` |
| | | `(gdpr\|hipaa\|sox\|pci) (requirements?\|compliance)` |
| | | `audit (requirements?\|trail)` |
| 이력 맥락 | `HISTORICAL_CONTEXT` | `when did we (decide\|choose\|implement\|launch)` |
| | | `(history\|timeline) of` |
| | | `(have\|did) we (ever\|previously)` |
| | | `how long (have\|has) .+ been` |
| 귀속 | `ATTRIBUTION` | `who (decided\|chose\|approved\|owns)` |
| | | `which (team\|person\|group) (is responsible\|decided\|owns)` |
| | | `(owner\|maintainer) of` |
| 일반 (fallback) | `GENERAL` | (패턴 없음 -- 위 모든 패턴 불일치 시 적용) |

**총 패턴 수**: **31개** (GENERAL 제외 7개 intent에 분포). 2026-04-17 실측 분포:
DECISION_RATIONALE 6 + FEATURE_HISTORY 5 + PATTERN_LOOKUP 5 + TECHNICAL_CONTEXT 4
+ SECURITY_COMPLIANCE 4 + HISTORICAL_CONTEXT 4 + ATTRIBUTION 3 = 31.

**Go 구현 참고**: `regexp.MustCompile`로 init 시 컴파일. `(?i)` 플래그 사용
(Python은 매칭 시 `re.IGNORECASE` 적용).

---

## 2. Stop Words

`QueryProcessor.STOP_WORDS` (query_processor.py:127-137). 키워드 추출 시 필터링용.

총 81개:

```
the, a, an, is, are, was, were, be, been, being,
have, has, had, do, does, did, will, would, could,
should, may, might, must, shall, can, need, dare,
ought, used, to, of, in, for, on, with, at, by,
from, up, about, into, over, after, we, our, us,
i, me, my, you, your, it, its, they, them, their,
this, that, these, those, what, which, who, whom,
when, where, why, how, and, or, but, if, because,
as, until, while, although, though, even, just, also
```

**Go 구현 참고**: `map[string]struct{}` 로 O(1) lookup. 쿼리 토큰화 후 이 set에
없는 단어만 keywords로 채택.

---

## 3. Time Scope 패턴

`QueryProcessor.TIME_PATTERNS` (query_processor.py:119-124).

| TimeScope | Python Name | Regex Patterns |
|-----------|-------------|----------------|
| 지난 1주 | `LAST_WEEK` | `last week`, `this week`, `past week`, `7 days` |
| 지난 1개월 | `LAST_MONTH` | `last month`, `this month`, `past month`, `30 days` |
| 지난 분기 | `LAST_QUARTER` | `last quarter`, `this quarter`, `Q[1-4]`, `past 3 months` |
| 지난 1년 | `LAST_YEAR` | `last year`, `this year`, `20\d{2}`, `past year` |
| 전체 (기본값) | `ALL_TIME` | (패턴 없음 -- 위 모든 패턴 불일치 시 기본값) |

**총 패턴 수**: 16개.

**Go 구현 참고**: `ALL_TIME`은 enum의 zero value로 설정하면 패턴 미매칭 시
자연스럽게 기본값이 된다.

---

## 4. Entity 추출 알고리즘

`QueryProcessor._extract_entities()` (query_processor.py:318-356).

4단계 파이프라인:

### 4.1 Quoted String 추출

```
패턴: "([^"]+)"|'([^']+)'
```

- 더블 쿼트와 싱글 쿼트 모두 캡처
- 두 그룹 중 비어있지 않은 쪽 사용 (`q[0] or q[1]`)
- 길이 1 이하 필터링 (`len(entity) > 1`)

### 4.2 Capitalized Word 감지

- `words = query.split()`으로 토큰화
- **인덱스 0 (문장 시작) 건너뜀** (`i > 0`)
- 첫 글자가 대문자이고 길이 2 이상인 단어 감지
- 연속 대문자 단어를 하나의 multi-word entity로 결합:
  ```
  "how does Azure Kubernetes Service work"
  → entity: "Azure Kubernetes Service"
  ```
- 중복 체크 후 추가 (`entity not in entities`)

### 4.3 Technology Name 패턴

4개의 카테고리별 정규식 (대소문자 무시):

| 카테고리 | 패턴 |
|----------|-------|
| DB/Infra | `\b(PostgreSQL\|MySQL\|MongoDB\|Redis\|Elasticsearch\|Kafka)\b` |
| Framework/언어 | `\b(React\|Vue\|Angular\|Next\.js\|Node\.js\|Python\|Java\|Go)\b` |
| Cloud/DevOps | `\b(AWS\|GCP\|Azure\|Kubernetes\|Docker\|Terraform)\b` |
| Protocol | `\b(REST\|GraphQL\|gRPC\|WebSocket\|HTTP\|HTTPS)\b` |

### 4.4 Dedup 및 제한

- `dict.fromkeys(entities)` 로 순서 보존 중복 제거
- **최대 10개** 반환 (`[:10]`)

---

## 5. Query Expansion 템플릿

`QueryProcessor._generate_expansions()` (query_processor.py:372-417).

### 5.1 Intent별 확장 문구

| Intent | 확장 접두사 |
|--------|-------------|
| `DECISION_RATIONALE` | `"decision {query}"`, `"rationale {query}"`, `"trade-off {query}"` |
| `FEATURE_HISTORY` | `"customer request {query}"`, `"feature rejected {query}"` |
| `PATTERN_LOOKUP` | `"standard approach {query}"`, `"best practice {query}"` |
| `TECHNICAL_CONTEXT` | `"architecture {query}"`, `"implementation {query}"` |
| 그 외 4종 | 확장 없음 (원본 쿼리만) |

### 5.2 Entity 기반 확장

상위 3개 entity에 대해 (`entities[:3]`):

```
"{entity} decision"
"why {entity}"
```

즉 entity가 3개면 6개 확장 추가.

### 5.3 Dedup 및 제한

- 원본 쿼리가 항상 첫 번째 (`expansions = [query]`)
- lowercase 기준 중복 제거 (`exp.lower()` 비교)
- **최대 5개** 반환 (`unique[:5]`)

---

## 6. ExtractionResult 스키마

`llm_extractor.py:28-70` + `server.py:1271-1324`.

### 6.1 데이터 클래스 계층

```
ExtractedFields          -- 단일 레코드 (single)
  title: str
  rationale: str
  problem: str
  alternatives: List[str]
  trade_offs: List[str]
  status_hint: str          ("proposed" | "accepted" | "rejected")
  tags: List[str]

PhaseExtractedFields     -- 다단계 레코드의 한 phase
  phase_title: str
  phase_decision: str
  phase_rationale: str
  phase_problem: str
  alternatives: List[str]
  trade_offs: List[str]
  tags: List[str]

ExtractionResult         -- 추출 결과 래퍼
  group_title: str
  group_type: str           ("phase_chain" | "bundle" | "")
  group_summary: str        (1-line semantic anchor)
  status_hint: str
  tags: List[str]
  confidence: Optional[float]   (0.0-1.0)
  single: Optional[ExtractedFields]
  phases: Optional[List[PhaseExtractedFields]]
  -- computed properties --
  is_multi_phase: bool      (phases is not None and len(phases) > 1)
  is_bundle: bool           (group_type == "bundle" and len(phases) > 1)
```

### 6.2 Agent JSON → ExtractionResult 매핑

server.py:1271-1324에서 agent가 반환한 JSON을 ExtractionResult로 변환:

**다단계 (phases > 1)**:
- `data["phases"]` 순회 (최대 7개: `[:7]`)
- 각 phase → `PhaseExtractedFields` (title 60자 제한)
- `group_type` 기본값: `"phase_chain"`
- `group_summary`: `data["reusable_insight"]` 우선, fallback으로 `data["group_title"]`

**단일 (phases == 1 또는 없음)**:
- phases가 정확히 1개: phase 필드에서 추출, fallback으로 flat 필드
- phases 없음: flat 필드에서 직접 추출
- `ExtractionResult.single`에 `ExtractedFields` 할당
- `group_title = single.title`

**공통 정규화**:
- title: `[:60]` 잘라냄
- tags: `str(t).lower()`
- status_hint: `.lower()`
- confidence: `float()` 변환 (None이면 0.0)

### 6.3 Phase 분할 임계값

```python
PHASE_SPLIT_THRESHOLD = 800    # chars → multi-phase
BUNDLE_SPLIT_THRESHOLD = 1500  # chars → bundle
```

---

## 7. DecisionRecord 필드 전체 목록

`agents/common/schemas/decision_record.py`. Pydantic BaseModel 기반.

### 7.1 Enum 정의

**Domain** (19종):

| 값 | 설명 |
|----|------|
| `architecture` | 아키텍처 |
| `security` | 보안 |
| `product` | 제품 |
| `exec` | 경영 |
| `ops` | 운영 |
| `design` | 디자인 |
| `data` | 데이터 |
| `hr` | 인사 |
| `marketing` | 마케팅 |
| `incident` | 장애 |
| `debugging` | 디버깅 |
| `qa` | QA |
| `legal` | 법무 |
| `finance` | 재무 |
| `sales` | 영업 |
| `customer_success` | 고객 성공 |
| `research` | 연구 |
| `risk` | 리스크 |
| `general` | 일반 (기본값) |

**Status** (4종): `proposed`, `accepted`, `superseded`, `reverted`

**Certainty** (3종): `supported`, `partially_supported`, `unknown`

**ReviewState** (4종): `unreviewed`, `approved`, `edited`, `rejected`

**Sensitivity** (3종): `public`, `internal`, `restricted`

**SourceType** (7종): `slack`, `meeting`, `doc`, `github`, `email`, `notion`, `other`

### 7.2 Sub-model 필드

**SourceRef**:
| 필드 | 타입 | 설명 |
|------|------|------|
| `type` | SourceType | 소스 유형 |
| `url` | Optional[str] | URL |
| `pointer` | Optional[str] | 예: `"channel:#arch thread_ts:123"` |

**Evidence**:
| 필드 | 타입 | 설명 |
|------|------|------|
| `claim` | str (required) | 주장 내용 |
| `quote` | str (required) | 직접 인용 (1-2문장) |
| `source` | SourceRef | 출처 |

**Assumption**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `assumption` | str | (required) |
| `confidence` | float [0.0, 1.0] | 0.5 |

**Risk**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `risk` | str | (required) |
| `mitigation` | Optional[str] | None |

**DecisionDetail**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `what` | str | (required) |
| `who` | List[str] | `[]` (형식: `"role:cto"`, `"user:alice"`) |
| `where` | str | `""` |
| `when` | str | `""` (형식: YYYY-MM-DD) |

**Context**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `problem` | str | `""` |
| `scope` | Optional[str] | None |
| `constraints` | List[str] | `[]` |
| `alternatives` | List[str] | `[]` |
| `chosen` | str | `""` |
| `trade_offs` | List[str] | `[]` |
| `assumptions` | List[Assumption] | `[]` |
| `risks` | List[Risk] | `[]` |

**Why**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `rationale_summary` | str | `""` |
| `certainty` | Certainty | `UNKNOWN` |
| `missing_info` | List[str] | `[]` |

**Quality**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `scribe_confidence` | float [0.0, 1.0] | 0.5 |
| `review_state` | ReviewState | `UNREVIEWED` |
| `reviewed_by` | Optional[str] | None |
| `review_notes` | Optional[str] | None |

**Payload**:
| 필드 | 타입 | 기본값 |
|------|------|--------|
| `format` | Literal["markdown"] | `"markdown"` |
| `text` | str | `""` |

### 7.3 DecisionRecord 최상위 필드

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `schema_version` | str | `"2.1"` | |
| `id` | str | (required) | `dec_YYYY-MM-DD_domain_slug` |
| `type` | Literal | `"decision_record"` | |
| `domain` | Domain | `GENERAL` | |
| `sensitivity` | Sensitivity | `INTERNAL` | |
| `status` | Status | `PROPOSED` | |
| `superseded_by` | Optional[str] | None | |
| `timestamp` | datetime | `now(utc)` | |
| `title` | str | (required) | |
| `decision` | DecisionDetail | (required) | |
| `context` | Context | `Context()` | |
| `why` | Why | `Why()` | |
| `evidence` | List[Evidence] | `[]` | |
| `links` | List[dict] | `[]` | ADR, PR 등 |
| `tags` | List[str] | `[]` | |
| `group_id` | Optional[str] | None | 그룹 공유 ID |
| `group_type` | Optional[str] | None | `"phase_chain"` 또는 `"bundle"` |
| `phase_seq` | Optional[int] | None | 0-indexed |
| `phase_total` | Optional[int] | None | |
| `original_text` | Optional[str] | None | 추출 전 원문 |
| `group_summary` | Optional[str] | None | 의미 앵커 |
| `reusable_insight` | str | `""` | 임베딩 대상 (256-768 토큰) |
| `quality` | Quality | `Quality()` | |
| `payload` | Payload | `Payload()` | |

### 7.4 ID 생성 규칙

**Record ID** (`generate_record_id`):
```
dec_{YYYY-MM-DD}_{domain}_{slug}
slug = title의 처음 3단어, lowercase, underscore 결합
```

**Group ID** (`generate_group_id`):
```
grp_{YYYY-MM-DD}_{domain}_{slug}
```

---

## 8. Certainty 판정 알고리즘

`RecordBuilder._determine_certainty()` (record_builder.py:543-576).

```
입력: evidence: List[Evidence], rationale: str
출력: (Certainty, List[str] missing_info)

1. evidence가 비어 있으면
   → UNKNOWN, ["No evidence found"]

2. evidence는 있으나 모든 claim에 "paraphrase"가 포함되어 있으면
   → PARTIALLY_SUPPORTED, ["No direct quotes - evidence is paraphrased"]

3. direct quote는 있으나 rationale이 비어 있으면
   → PARTIALLY_SUPPORTED, ["Explicit rationale not found"]

4. direct quote + rationale 모두 존재
   → SUPPORTED, []
```

**direct quote 판정 기준**: `"paraphrase" not in e.claim.lower()` 인 evidence가
하나라도 있으면 direct quote 있음으로 간주.

### ensure_evidence_certainty_consistency()

`decision_record.py:226-242`. 레코드 생성 후 사후 보정:

```
1. evidence 중 quote가 있는 항목이 하나도 없으면:
   - certainty가 SUPPORTED였으면 → UNKNOWN으로 강등
   - missing_info에 "No direct quotes found in evidence" 추가

2. evidence가 아예 없으면:
   - status가 ACCEPTED였으면 → PROPOSED로 강등
```

**Go 구현 참고**: 이 두 함수는 서로 다른 시점에 실행된다.
`_determine_certainty`는 빌드 시, `ensure_evidence_certainty_consistency`는
최종 validation 시. 둘 다 구현 필요.

---

## 9. Status 판정 알고리즘

`RecordBuilder._determine_status()` (record_builder.py:578-602) 및
`_status_from_hint()` (record_builder.py:604-619).

### 9.1 규칙 기반 판정 (`_determine_status`)

```
1. evidence가 비어 있으면 → PROPOSED

2. acceptance 패턴 매칭 (text.lower() 대상):
   패턴 그룹 1: \b(?:approved|accepted|confirmed|finalized|agreed|decided)\b
   패턴 그룹 2: \b(?:final decision|it's decided|we're going with)\b
   매칭되면 → ACCEPTED

3. 기본값 → PROPOSED (보수적)
```

### 9.2 LLM 힌트 기반 판정 (`_status_from_hint`)

```
hint_lower = hint.lower().strip()

"accepted"  → ACCEPTED
"rejected"  → PROPOSED  (거부된 제안도 여전히 proposed)
"proposed"  → PROPOSED
그 외       → _determine_status() fallback (규칙 기반)
```

**Go 구현 참고**: `"rejected"` → `PROPOSED` 매핑은 의도적. 거부된 결정은
`superseded`가 아니라 채택되지 않은 제안으로 취급.

---

## 10. Domain 매핑 테이블

`RecordBuilder._parse_domain()` (record_builder.py:621-655).

입력 문자열을 lowercase로 변환 후 **부분 문자열 매칭** (`key in domain_lower`).

| 키 문자열 | Domain enum |
|-----------|-------------|
| `"architecture"` | `ARCHITECTURE` |
| `"security"` | `SECURITY` |
| `"product"` | `PRODUCT` |
| `"exec"` | `EXEC` |
| `"ops"` | `OPS` |
| `"design"` | `DESIGN` |
| `"data"` | `DATA` |
| `"hr"` | `HR` |
| `"marketing"` | `MARKETING` |
| `"incident"` | `INCIDENT` |
| `"debugging"` | `DEBUGGING` |
| `"qa"` | `QA` |
| `"legal"` | `LEGAL` |
| `"finance"` | `FINANCE` |
| `"sales"` | `SALES` |
| `"customer_success"` | `CUSTOMER_SUCCESS` |
| `"customer_escalation"` | `CUSTOMER_SUCCESS` |
| `"research"` | `RESEARCH` |
| `"risk"` | `RISK` |

**총 19개 키** (18개 Domain 값 -- `customer_escalation`은 `CUSTOMER_SUCCESS`에 alias).

Fallback: 어떤 키와도 매칭되지 않으면 → `GENERAL`.

**Go 구현 참고**: `strings.Contains(domainLower, key)` 순회. 순서에 따라 첫
매칭이 반환되므로 Python dict 순서를 보존할 것.

---

## 11. Evidence 추출

`RecordBuilder.QUOTE_PATTERNS` (record_builder.py:72-77) 및
`_extract_evidence()` (record_builder.py:498-531).

### 11.1 Quote 패턴 (4종)

| 종류 | 패턴 | 최소 길이 |
|------|------|-----------|
| 더블 쿼트 | `"([^"]{10,})"` | 10자 |
| 싱글 쿼트 | `'([^']{10,})'` | 10자 |
| 일본어 괄호 | `「([^」]{10,})」` | 10자 |
| 프랑스어 괄호 | `«([^»]{10,})»` | 10자 |

### 11.2 추출 흐름

```
1. 4개 QUOTE_PATTERNS 순회하여 정규식 매칭
   - 매칭된 quote가 10자 이상이면 Evidence 생성
   - quote 길이 200자 제한 ([:200])
   - claim = "Quoted statement from discussion"

2. quote가 하나도 없고 text가 20자 이상이면 paraphrase fallback:
   - text[:150] + "..." (150자 초과 시) 또는 text 전체
   - claim = "Decision statement (paraphrased)"

3. 최대 3개 반환 ([:3])
```

### 11.3 Rationale 추출 패턴

`RATIONALE_PATTERNS` (record_builder.py:80-86):

| 패턴 |
|------|
| `because\s+(.{10,}?)(?:\.\|$)` |
| `reason(?:ing)?(?:\s+is)?[:\s]+(.{10,}?)(?:\.\|$)` |
| `rationale[:\s]+(.{10,}?)(?:\.\|$)` |
| `since\s+(.{10,}?)(?:\.\|$)` |
| `due to\s+(.{10,}?)(?:\.\|$)` |

첫 매칭만 반환. 매칭 없으면 빈 문자열.

---

## 12. Metadata 추출 fallback 경로

`Searcher._to_search_result()` (searcher.py:472-521).

enVector 검색 결과 raw dict에서 메타데이터 추출 시 다단계 fallback 사용.

### 12.1 payload_text 추출

```
1차: metadata.payload.text     (payload가 dict인 경우)
2차: metadata.text             (payload가 dict가 아닌 경우)
3차: raw.text                  (metadata에 text가 없는 경우)
4차: metadata.decision.what    (위 3개 모두 비어 있는 경우, decision이 dict인 경우)
```

### 12.2 certainty 추출

```
1차: metadata.why.certainty    (why가 dict인 경우)
2차: "unknown"                 (why가 dict가 아니거나 없는 경우)
```

### 12.3 단순 필드

```
record_id: metadata.id → raw.id → "unknown"
title:     metadata.title → "Untitled"
domain:    metadata.domain → "general"
status:    metadata.status → "unknown"
```

### 12.4 그룹 필드

```
group_id:    metadata.group_id    (Optional)
group_type:  metadata.group_type  (Optional)
phase_seq:   metadata.phase_seq   (Optional)
phase_total: metadata.phase_total (Optional)
```

**Go 구현 참고**: 모든 중첩 접근에서 `isinstance(x, dict)` 타입 체크가 필요.
Go에서는 `map[string]interface{}` assertion으로 대체.

---

## 13. enVector SDK 안전 패치

`mcp/adapter/envector_sdk.py:33-86`.

### 13.1 문제

pyenvector SDK의 `KeyParameter` 클래스는 초기화 시 `SecKey.json`과
`MetadataKey.json` 파일을 디스크에서 로드하려 시도한다. Vault 보안 모델에서는
비밀키를 디스크에 두지 않으므로, 파일이 없으면 SDK가 크래시한다.

### 13.2 Monkey-Patch 상세

5개의 property getter를 교체:

| Property | 원래 동작 | 패치 동작 |
|----------|-----------|-----------|
| `sec_key` | 파일 로드 후 반환 | 파일 없으면 `None` 반환 |
| `sec_key_path` | 경로 반환 | 파일 없으면 `None` (Cipher가 decryptor init 건너뜀) |
| `metadata_key` | 파일 로드 후 반환 | 파일 없으면 `None` 반환 |
| `metadata_key_path` | 경로 반환 | 파일 없으면 `None` |
| `metadata_encryption` | `True`/`False` | 키 파일 없으면 강제 `False` (앱 레이어에서 암호화) |

**Go에서의 함의**: enVector SDK Go 바인딩 사용 시, 비밀키 파일 로드를 시도하지
않는 초기화 경로를 사용해야 한다. encrypt-only 모드로 동작하면 충분.

### 13.3 gRPC Connection Error 패턴

`CONNECTION_ERROR_PATTERNS` (envector_sdk.py:89-101). 재연결 판단 기준:

```
UNAVAILABLE, DEADLINE_EXCEEDED, Connection refused, Connection reset,
Stream removed, RST_STREAM, Broken pipe, Transport closed,
Socket closed, EOF, failed to connect
```

---

## 14. AES 암호화 모드 (확정: AES-256-CTR)

`envector_sdk.py:227-234`.

### 14.1 코드 인용

```python
def _app_encrypt_metadata(self, metadata_str: str) -> str:
    from pyenvector.utils.aes import encrypt_metadata as aes_encrypt
    ct = aes_encrypt(metadata_str, self._agent_dek)
    return json.dumps({"a": self._agent_id, "c": ct})
```

- 앱 레이어 메타데이터 암호화에 per-agent DEK 사용
- 봉투 포맷: `{"a": "<agent_id>", "c": "<base64_ciphertext>"}`
- `agent_id`는 복호화 시 올바른 DEK를 찾기 위한 키

### 14.2 확정된 사항 (2026-04-17, pyenvector 소스 실측)

`pyenvector/utils/aes.py:52-58`에서 확인:
- **AES-256-CTR 모드** 사용 (docstring은 "AES-GCM" 언급이 있으나 **오래된 주석**,
  실제 구현은 `AESHelper.encrypt_with_aes()` → CTR)
- DEK는 32바이트 (AES-256), config.py L244-252에서 base64 디코드 후 길이 검증
- CTR 모드이므로 padding 불필요
- 와이어 포맷: `IV(16바이트) || ciphertext → base64`
- 봉투는 `{"a": agent_id, "c": base64(IV || CT)}`로 JSON 직렬화

### 14.3 Go 구현

```go
// Go 포팅: crypto/cipher.NewCTR
import "crypto/aes"
import "crypto/cipher"
import "crypto/rand"

func encryptMetadata(plaintext []byte, key []byte) (string, error) {
    block, _ := aes.NewCipher(key)   // key 32 bytes = AES-256
    iv := make([]byte, aes.BlockSize) // 16 bytes
    rand.Read(iv)
    stream := cipher.NewCTR(block, iv)
    ciphertext := make([]byte, len(plaintext))
    stream.XORKeyStream(ciphertext, plaintext)
    // IV || CT → base64
    return base64.StdEncoding.EncodeToString(append(iv, ciphertext...)), nil
}
```

Vault의 `DecryptMetadata` RPC가 동일 포맷을 기대하므로 비트 단위 호환 필요.
기존 v0.3.x로 저장된 레코드 역호환을 위해 IV 파싱 로직은 `IV(16) || CT` 순서 유지.

---

## 15. Vault Health Check Fallback

`mcp/adapter/vault_client.py:301-337`.

### 15.1 Primary: gRPC Health Check

```
서비스: grpc.health.v1.Health
RPC:    Check(HealthCheckRequest{service: ""})
응답:   status == SERVING이면 healthy
타임아웃: 5초
```

### 15.2 Fallback: HTTP Health Check

gRPC 실패 시, endpoint가 `http://` 또는 `https://`인 경우만 시도:

```
1. Endpoint cleanup:
   - "/mcp" suffix 제거
   - "/sse" suffix 제거

2. GET {base_url}/health
   - TLS: tls_disable이면 verify=False, 아니면 ca_cert 또는 True
   - 타임아웃: 5초
   - 200이면 healthy

3. 둘 다 실패 → False 반환
```

**Go 구현 참고**: gRPC health check는 `google.golang.org/grpc/health/grpc_health_v1`
패키지 사용. HTTP fallback은 `net/http` 로 간단히 구현.

---

## 16. enVector Pre-Warm

`mcp/server/server.py:1051-1080`.

### 16.1 목적

`reload_pipelines` 직후 enVector gRPC 채널을 미리 예열하여, 후속
`RegisterKey` 호출 시 cold-start 지연을 방지한다. 예열하지 않으면
`/rune:status` 진단 검사에서 enVector가 "unreachable"로 보고될 수 있다.

### 16.2 동작

```
전제조건: scribe 파이프라인 초기화 성공 AND self.envector is not None

1. ThreadPoolExecutor(max_workers=1) 생성
2. self.envector.invoke_get_index_list() 를 별도 스레드에서 실행
3. 60초 타임아웃으로 대기

성공 시: {"ok": True, "latency_ms": ...}
타임아웃 시: {"ok": False, "error": "Pre-warm timed out after 60s"}
예외 시: {"ok": False, "error": str(e)}

4. pool.shutdown(wait=False)
```

### 16.3 Non-Fatal 특성

Pre-warm 실패는 `reload_pipelines` 자체의 성공/실패에 영향을 주지 않는다.
결과는 응답의 `envector_warmup` 필드에 포함될 뿐이다.

**Go 구현 참고**: `go func()` 고루틴에 `context.WithTimeout(60s)` 를 걸면 동일
패턴. reload 응답에 warmup 결과를 포함시킬 것.
