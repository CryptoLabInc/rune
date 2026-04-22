# Domain Types — DecisionRecord v2.1 + I/O schemas

이 문서는 Go 포팅이 필요한 **모든 domain 타입 · enum · I/O 스키마**의 단일 출처 (Single Source of Truth). 다른 flow/component 문서는 여기를 link만 하고 자체 정의는 두지 않는다.

**Python 원본**: `agents/common/schemas/decision_record.py` (260 LoC) · `agents/retriever/query_processor.py` L22-54

**Go 이관 위치**: `internal/domain/` 패키지

## 목차

1. Enums (8개: 6 schema + 2 query)
2. Sub-models (9개)
3. DecisionRecord v2.1 (main schema)
4. MCP tool I/O (CaptureRequest/Response, RecallArgs/Result)
5. Internal types (SearchHit, ParsedQuery, Detection, NoveltyInfo)
6. capture_log.jsonl entry
7. 헬퍼 · validation 규칙

---

## 1. Enums

### 1.1 Domain (19 values)

**Python**: `decision_record.py:L19-39`  
**용도**: DecisionRecord.domain · tier2 에이전트가 에이전트에서 지정

```go
type Domain string

const (
    DomainArchitecture     Domain = "architecture"
    DomainSecurity         Domain = "security"
    DomainProduct          Domain = "product"
    DomainExec             Domain = "exec"
    DomainOps              Domain = "ops"
    DomainDesign           Domain = "design"
    DomainData             Domain = "data"
    DomainHR               Domain = "hr"
    DomainMarketing        Domain = "marketing"
    DomainIncident         Domain = "incident"
    DomainDebugging        Domain = "debugging"
    DomainQA               Domain = "qa"
    DomainLegal            Domain = "legal"
    DomainFinance          Domain = "finance"
    DomainSales            Domain = "sales"
    DomainCustomerSuccess  Domain = "customer_success"
    DomainResearch         Domain = "research"
    DomainRisk             Domain = "risk"
    DomainGeneral          Domain = "general"
)

func ParseDomain(s string) (Domain, error) {
    // unknown 입력 시 DomainGeneral로 fallback (D14 에이전트-delegated 관대함)
    // 빈 문자열 시 DomainGeneral
}
```

### 1.2 Sensitivity (3 values)

**Python**: `decision_record.py:L42-46`

```go
type Sensitivity string

const (
    SensitivityPublic     Sensitivity = "public"
    SensitivityInternal   Sensitivity = "internal"
    SensitivityRestricted Sensitivity = "restricted"
)

// Default: SensitivityInternal (Python DecisionRecord default)
```

### 1.3 Status (4 values)

**Python**: `decision_record.py:L49-54`  
**용도**: recall rerank `STATUS_MULTIPLIER` (accepted=1.0, proposed=0.9, superseded=0.5, reverted=0.3)

```go
type Status string

const (
    StatusProposed   Status = "proposed"
    StatusAccepted   Status = "accepted"
    StatusSuperseded Status = "superseded"
    StatusReverted   Status = "reverted"
)

// Default: StatusProposed (Python DecisionRecord default)
```

### 1.4 Certainty (3 values)

**Python**: `decision_record.py:L57-61`  
**CRITICAL RULE**: `SUPPORTED`은 evidence.quote가 있어야 함. 없으면 강제 `UNKNOWN`으로 강등 (§7 참조)

```go
type Certainty string

const (
    CertaintySupported          Certainty = "supported"
    CertaintyPartiallySupported Certainty = "partially_supported"
    CertaintyUnknown            Certainty = "unknown"
)

// Default: CertaintyUnknown
// recall calculateConfidence weight: supported=1.0, partially=0.6, unknown=0.3
```

### 1.5 ReviewState (4 values)

**Python**: `decision_record.py:L64-69`

```go
type ReviewState string

const (
    ReviewStateUnreviewed ReviewState = "unreviewed"
    ReviewStateApproved   ReviewState = "approved"
    ReviewStateEdited     ReviewState = "edited"
    ReviewStateRejected   ReviewState = "rejected"
)

// Default: ReviewStateUnreviewed
```

### 1.6 SourceType (7 values)

**Python**: `decision_record.py:L72-80`  
**용도**: Evidence.source.type

```go
type SourceType string

const (
    SourceTypeSlack   SourceType = "slack"
    SourceTypeMeeting SourceType = "meeting"
    SourceTypeDoc     SourceType = "doc"
    SourceTypeGitHub  SourceType = "github"
    SourceTypeEmail   SourceType = "email"
    SourceTypeNotion  SourceType = "notion"
    SourceTypeOther   SourceType = "other"
)
```

### 1.7 QueryIntent (8 values)

**Python**: `query_processor.py:L23-32`  
**용도**: recall Phase 2 intent 분류

```go
type QueryIntent string

const (
    QueryIntentDecisionRationale QueryIntent = "decision_rationale"  // "Why did we choose X?"
    QueryIntentFeatureHistory    QueryIntent = "feature_history"     // "Have customers asked for X?"
    QueryIntentPatternLookup     QueryIntent = "pattern_lookup"      // "How do we handle X?"
    QueryIntentTechnicalContext  QueryIntent = "technical_context"   // "What's our architecture for X?"
    QueryIntentSecurityCompliance QueryIntent = "security_compliance" // "What are the security requirements?"
    QueryIntentHistoricalContext QueryIntent = "historical_context"  // "When did we decide X?"
    QueryIntentAttribution       QueryIntent = "attribution"         // "Who decided on X?"
    QueryIntentGeneral           QueryIntent = "general"             // fallback
)
```

### 1.8 TimeScope (5 values)

**Python**: `query_processor.py:L35-41`  
**용도**: recall Phase 6 `filterByTime` (LAST_WEEK=7일 ~ LAST_YEAR=365일)

```go
type TimeScope string

const (
    TimeScopeLastWeek    TimeScope = "last_week"    // 7 days
    TimeScopeLastMonth   TimeScope = "last_month"   // 30 days
    TimeScopeLastQuarter TimeScope = "last_quarter" // 90 days
    TimeScopeLastYear    TimeScope = "last_year"    // 365 days
    TimeScopeAllTime     TimeScope = "all_time"     // no filter (default)
)
```

---

## 2. Sub-models

### 2.1 SourceRef

**Python**: `decision_record.py:L87-91`

```go
type SourceRef struct {
    Type    SourceType `json:"type"`
    URL     *string    `json:"url,omitempty"`
    Pointer *string    `json:"pointer,omitempty"` // e.g., "channel:#arch thread_ts:123" or "timestamp:00:32:14"
}
```

### 2.2 Evidence

**Python**: `decision_record.py:L94-98`

```go
type Evidence struct {
    Claim  string    `json:"claim"`  // What is being claimed
    Quote  string    `json:"quote"`  // Direct quote (1-2 sentences)
    Source SourceRef `json:"source"`
}
```

### 2.3 Assumption

**Python**: `decision_record.py:L101-104`

```go
type Assumption struct {
    Assumption string  `json:"assumption"`
    Confidence float64 `json:"confidence"` // [0.0, 1.0], default 0.5
}
```

### 2.4 Risk

**Python**: `decision_record.py:L107-110`

```go
type Risk struct {
    Risk       string  `json:"risk"`
    Mitigation *string `json:"mitigation,omitempty"`
}
```

### 2.5 DecisionDetail

**Python**: `decision_record.py:L113-118`

```go
type DecisionDetail struct {
    What  string   `json:"what"`             // The actual decision statement
    Who   []string `json:"who,omitempty"`    // Participants (role:cto, user:alice)
    Where string   `json:"where,omitempty"`  // Channel/meeting where decided
    When  string   `json:"when,omitempty"`   // Date (YYYY-MM-DD)
}
```

### 2.6 Context

**Python**: `decision_record.py:L121-130`

```go
type Context struct {
    Problem      string       `json:"problem,omitempty"`
    Scope        *string      `json:"scope,omitempty"`
    Constraints  []string     `json:"constraints,omitempty"`
    Alternatives []string     `json:"alternatives,omitempty"`
    Chosen       string       `json:"chosen,omitempty"`
    TradeOffs    []string     `json:"trade_offs,omitempty"`
    Assumptions  []Assumption `json:"assumptions,omitempty"`
    Risks        []Risk       `json:"risks,omitempty"`
}
```

### 2.7 Why

**Python**: `decision_record.py:L133-142`

```go
type Why struct {
    RationaleSummary string    `json:"rationale_summary,omitempty"`
    Certainty        Certainty `json:"certainty"` // default: CertaintyUnknown
    MissingInfo      []string  `json:"missing_info,omitempty"`
}
```

> **불변 계약** (§7.1): `Certainty=Supported`는 evidence에 quote가 있어야 함. 없으면 자동 Unknown 강등.

### 2.8 Quality

**Python**: `decision_record.py:L145-150`

```go
type Quality struct {
    ScribeConfidence float64     `json:"scribe_confidence"` // [0.0, 1.0], default 0.5
    ReviewState      ReviewState `json:"review_state"`      // default: Unreviewed
    ReviewedBy       *string     `json:"reviewed_by,omitempty"`
    ReviewNotes      *string     `json:"review_notes,omitempty"`
}
```

### 2.9 Payload

**Python**: `decision_record.py:L153-159`

```go
type Payload struct {
    Format string `json:"format"` // fixed "markdown"
    Text   string `json:"text"`   // Markdown text for embedding fallback
}
```

---

## 3. DecisionRecord v2.1 (main schema)

**Python**: `decision_record.py:L166-213`  
**용도**: envector.Insert metadata의 decrypted payload

```go
type DecisionRecord struct {
    SchemaVersion string    `json:"schema_version"` // fixed "2.1"
    ID            string    `json:"id"`             // dec_YYYY-MM-DD_<domain>_<slug>
    Type          string    `json:"type"`           // fixed "decision_record"

    Domain       Domain       `json:"domain"`
    Sensitivity  Sensitivity  `json:"sensitivity"`
    Status       Status       `json:"status"`
    SupersededBy *string      `json:"superseded_by,omitempty"`
    Timestamp    time.Time    `json:"timestamp"` // UTC, RFC3339

    Title    string         `json:"title"`    // 60-rune truncate (D3)
    Decision DecisionDetail `json:"decision"`
    Context  Context        `json:"context"`
    Why      Why            `json:"why"`
    Evidence []Evidence     `json:"evidence,omitempty"`

    Links []map[string]any `json:"links,omitempty"` // ADR, PR URLs 등
    Tags  []string         `json:"tags,omitempty"`

    // Group fields (phase_chain / bundle)
    GroupID    *string `json:"group_id,omitempty"`
    GroupType  *string `json:"group_type,omitempty"`  // "phase_chain" | "bundle"
    PhaseSeq   *int    `json:"phase_seq,omitempty"`   // 0-indexed
    PhaseTotal *int    `json:"phase_total,omitempty"` // max 7 (D의 phase cap)

    // Content preservation
    OriginalText *string `json:"original_text,omitempty"`
    GroupSummary *string `json:"group_summary,omitempty"` // 1-line topic shared across phases

    // PRIMARY embedding target (schema 2.1+)
    ReusableInsight string `json:"reusable_insight"` // 256-768 tokens, dense NL, no markdown

    Quality Quality `json:"quality"`
    Payload Payload `json:"payload"`
}
```

### Record ID 생성 규칙

**Python**: `decision_record.py:L245-251` `generate_record_id`

```go
func GenerateRecordID(ts time.Time, domain Domain, title string) string {
    // date_str = ts.UTC().Format("2006-01-02")
    // 제목 첫 3 단어, lowercase, 영숫자·underscore만 필터, "_" join
    // 형식: "dec_{date}_{domain}_{slug}"
    // Python: w.isalnum() or w.replace("_", "").isalnum() — unicode 포함
    // Go 동등: unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
```

**주의**: Python `str.isalnum()`은 유니코드 포함이므로 한글도 통과. Go는 `unicode.IsLetter`/`IsDigit` 사용해야 파리티.

### Group ID 생성 규칙

**Python**: `decision_record.py:L254-259` `generate_group_id`

```go
func GenerateGroupID(ts time.Time, domain Domain, title string) string {
    // 동일 로직이지만 prefix "grp_"
    // 형식: "grp_{date}_{domain}_{slug}"
}
```

### Embedding 텍스트 선택

**Python**: `agents/common/schemas/embedding.py` `embedding_text_for_record`

```go
func EmbeddingTextForRecord(r *DecisionRecord) string {
    if s := strings.TrimSpace(r.ReusableInsight); s != "" {
        return s
    }
    return r.Payload.Text
}
```

---

## 4. MCP tool I/O

### 4.1 CaptureRequest / CaptureResponse

**Python 핸들러**: `server.py:L698-806` (entry) · L1208-1407 (`_capture_single`)  
**결정**: D4 (extracted=map[string]any) · D14 (agent-delegated)

```go
type CaptureRequest struct {
    Text      string         `json:"text"`
    Source    string         `json:"source"`
    User      string         `json:"user,omitempty"`
    Channel   string         `json:"channel,omitempty"`
    Extracted map[string]any `json:"extracted"` // agent-delegated 전수 payload
}

// extracted 내 계약 필드 (overview/decisions.md D4 참조):
//   tier2.capture:  bool (default true)  — false면 rejection path
//   tier2.reason:   string                — rejection reason
//   tier2.domain:   string (default "general")
//   confidence:     number [0.0, 1.0]     — 에이전트 신뢰도
//   title:          string                 — 60-rune truncate
//   reusable_insight: string               — 임베딩 우선 source
//   phases:         [...]                  — max 7 records
//   payload.text:   string                 — fallback 임베딩 source
//   기타 DecisionRecord v2.1 필드
```

```go
type CaptureResponse struct {
    OK       bool   `json:"ok"`
    Captured bool   `json:"captured"`
    RecordID string `json:"record_id,omitempty"` // captured=true 시
    Title    string `json:"title,omitempty"`
    Domain   Domain `json:"domain,omitempty"`

    Reason   string       `json:"reason,omitempty"`   // captured=false 시 사유
    Novelty  *NoveltyInfo `json:"novelty,omitempty"`  // near_duplicate 시 포함
    SimilarTo string      `json:"similar_to,omitempty"` // D10: record_id 또는 "shard=X,row=Y"

    Error string `json:"error,omitempty"` // ok=false 시
}
```

### 4.2 RecallArgs / RecallResult

**Python 핸들러**: `server.py:L910-1034`

```go
type RecallArgs struct {
    Query  string  `json:"query"`
    TopK   int     `json:"topk,omitempty"`   // default 5, max 10
    Domain *string `json:"domain,omitempty"` // filter by domain enum
    Status *string `json:"status,omitempty"` // filter by status
    Since  *string `json:"since,omitempty"`  // ISO date "YYYY-MM-DD"
}
```

```go
type RecallResult struct {
    OK         bool          `json:"ok"`
    Found      int           `json:"found"`      // post-filter 결과 수
    Results    []RecallEntry `json:"results"`    // top-K entries
    Confidence float64       `json:"confidence"` // calculateConfidence 결과 (0.00-1.00)
    Sources    []RecallSource `json:"sources"`   // top-5 source refs

    Synthesized bool   `json:"synthesized"` // fixed false (D28 agent-delegated)
    Error       string `json:"error,omitempty"`
}

type RecallEntry struct {
    RecordID        string  `json:"record_id"`
    Title           string  `json:"title"`
    Domain          string  `json:"domain"`
    Certainty       string  `json:"certainty"`
    Status          string  `json:"status"`
    Score           float64 `json:"score"`
    AdjustedScore   float64 `json:"adjusted_score"`
    ReusableInsight string  `json:"reusable_insight,omitempty"`
    PayloadText     string  `json:"payload_text,omitempty"`
    // Group fields
    GroupID    *string `json:"group_id,omitempty"`
    GroupType  *string `json:"group_type,omitempty"`
    PhaseSeq   *int    `json:"phase_seq,omitempty"`
    PhaseTotal *int    `json:"phase_total,omitempty"`
}

type RecallSource struct {
    RecordID string `json:"record_id"`
    Title    string `json:"title"`
}
```

---

## 5. Internal types

### 5.1 SearchHit

**Python**: `searcher.py:L44-76` `SearchResult`  
**용도**: recall Phase 3-6 내부 파이프라인

```go
type SearchHit struct {
    RecordID        string
    Title           string
    PayloadText     string // for synthesis display
    Domain          string
    Certainty       string
    Status          string
    Score           float64
    ReusableInsight string
    AdjustedScore   float64 // after recency weighting + status multiplier
    Metadata        map[string]any

    // Group fields
    GroupID    *string
    GroupType  *string
    PhaseSeq   *int
    PhaseTotal *int
}

// Python properties 포팅
func (h *SearchHit) IsReliable() bool {
    return h.Certainty == "supported" || h.Certainty == "partially_supported"
}

func (h *SearchHit) IsPhase() bool {
    return h.GroupID != nil
}
```

### 5.2 ParsedQuery

**Python**: `query_processor.py:L44-54`  
**용도**: recall Phase 2 결과

```go
type ParsedQuery struct {
    Original        string
    Cleaned         string
    Intent          QueryIntent
    TimeScope       TimeScope
    Entities        []string // max 10
    Keywords        []string // max 15
    ExpandedQueries []string // max 5
    // Language: D21에서 agent-side 번역, Go에서는 English 전제
}
```

### 5.3 Detection

**Python**: `scribe/detector.py:L15-23`  
**용도**: capture Phase 2에서 `_detection_from_agent_data` 결과

```go
type Detection struct {
    IsSignificant bool    // agent-delegated에서 항상 true (server.py:L82)
    Confidence    float64 // [0.0, 1.0] agent-provided
    Domain        string  // from extracted.tier2.domain, default "general"
}
```

### 5.4 NoveltyInfo

**Python**: `server.py:L100-108` `_classify_novelty`  
**용도**: capture Phase 4 결과

```go
type NoveltyInfo struct {
    Class   NoveltyClass    `json:"class"`   // novel | related | evolution | near_duplicate
    Score   float64         `json:"score"`   // max similarity
    Related []RelatedRecord `json:"related"` // top-3 비슷한 record (Python L1353-1360)
}

type NoveltyClass string
const (
    NoveltyClassNovel         NoveltyClass = "novel"         // < 0.3
    NoveltyClassRelated       NoveltyClass = "related"       // 0.3 ~ 0.7
    NoveltyClassEvolution     NoveltyClass = "evolution"     // 0.7 ~ 0.95
    NoveltyClassNearDuplicate NoveltyClass = "near_duplicate" // >= 0.95 (blocks capture)
)

type RelatedRecord struct {
    ID         string  `json:"id"`
    Title      string  `json:"title"`
    Similarity float64 `json:"similarity"` // round 3
}
```

---

## 6. capture_log.jsonl entry

**Python**: `server.py:L115-138` `_append_capture_log`  
**결정**: D20 (Python bit-identical format)

```go
type CaptureLogEntry struct {
    TS           string  `json:"ts"`            // RFC3339 UTC
    RecordID     string  `json:"record_id"`
    Title        string  `json:"title"`
    Domain       string  `json:"domain"`
    Mode         string  `json:"mode"`          // "agent-delegated" | "soft-delete" | ...
    Action       string  `json:"action"`        // "captured" (default) | "deleted"
    NoveltyClass string  `json:"novelty_class,omitempty"`
    NoveltyScore float64 `json:"novelty_score,omitempty"`
}
```

- 파일: `~/.rune/capture_log.jsonl` (0600 perms)
- Append-only. 실패 시 degrade (D19, capture 성공 응답 유지)

---

## 7. Validation · 헬퍼

### 7.1 `EnsureEvidenceCertaintyConsistency`

**Python**: `decision_record.py:L226-242`  
**CRITICAL RULE**: evidence에 quote 하나도 없으면 `Certainty=Supported` → 강제 `Unknown` 강등 + `missing_info`에 "No direct quotes found in evidence" 추가. evidence 자체가 없으면 `Status=Accepted` → `Proposed` 강등.

```go
func EnsureEvidenceCertaintyConsistency(r *DecisionRecord) {
    hasQuotes := false
    for _, e := range r.Evidence {
        if e.Quote != "" { hasQuotes = true; break }
    }

    if !hasQuotes {
        if r.Why.Certainty == CertaintySupported {
            r.Why.Certainty = CertaintyUnknown
            const marker = "No direct quotes found in evidence"
            if !contains(r.Why.MissingInfo, marker) {
                r.Why.MissingInfo = append(r.Why.MissingInfo, marker)
            }
        }
    }

    if len(r.Evidence) == 0 {
        if r.Status == StatusAccepted {
            r.Status = StatusProposed
        }
    }
}
```

### 7.2 `ValidateEvidenceCertainty` (read-only)

**Python**: `decision_record.py:L215-224`

```go
func ValidateEvidenceCertainty(r *DecisionRecord) bool {
    hasQuotes := false
    for _, e := range r.Evidence {
        if e.Quote != "" { hasQuotes = true; break }
    }
    return !(r.Why.Certainty == CertaintySupported && !hasQuotes)
}
```

---

## 8. Defaults 요약

| 필드 | 기본값 | 이유 |
|---|---|---|
| `Domain` | `general` | 분류 불명 시 |
| `Sensitivity` | `internal` | 보수적 기본 |
| `Status` | `proposed` | evidence 없을 때 |
| `Certainty` | `unknown` | evidence quote 없을 때 강제 |
| `ReviewState` | `unreviewed` | |
| `TimeScope` | `all_time` | 시간 제약 없음 |
| `QueryIntent` | `general` | intent 미검출 시 |
| `Payload.Format` | `markdown` | fixed |
| `Schema Version` | `2.1` | fixed |
| `Type` | `decision_record` | fixed |
| `Quality.ScribeConfidence` | `0.5` | 중립 |

---

## 9. 참조

- Python 원본: `agents/common/schemas/decision_record.py` (260 LoC) · `agents/retriever/query_processor.py:L22-54`
- 관련 flow: `spec/flows/capture.md` Phase 2·5 · `spec/flows/recall.md` Phase 2·6·7
- 관련 컴포넌트: `spec/components/rune-mcp.md` (tool 정의)
- 관련 결정: D3 (title 60자), D4 (extracted map), D13 (DecisionRecord 조립 책임), D14 (agent-delegated), D16 (batch embed), D20 (capture_log 포맷)
- Embedding 텍스트 선택: `agents/common/schemas/embedding.py` `embedding_text_for_record`
- Render logic: `agents/common/schemas/templates.py` (D15 canonical reference)
