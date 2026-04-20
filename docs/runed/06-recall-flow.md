# Recall 데이터 플로우: 쿼리 → 검색 → 복호화 → 재랭킹

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스 실측 대조 완료.
> §2.2.2 Intent patterns **31개 확정** (문서의 "regex 8종"은 intent 종류 수,
> 총 패턴은 6+5+5+4+4+4+3=31). §3.1 FHE 1024-dim 확정. §4.3 재랭킹 공식
> `(0.7×rawScore + 0.3×decay) × statusMul` (`searcher.py:297` 실측 일치).
> §4.4 status 승수 `accepted 1.0 / proposed 0.9 / superseded 0.5 / reverted 0.3`
> 일치.

이 문서는 `runed`의 Recall(검색/조회) 파이프라인 전체를 다룬다.
HTTP 요청 수신부터 응답 반환까지, 각 단계의 입력/출력 타입과 데이터 변환을
구현 수준에서 기술한다.

---

## 1. 전체 플로우 개요

```
POST /recall {"query":"PostgreSQL 결정 이유","topk":5}
  │
  ▼
[Stage 1] 쿼리 분석 (query.go)
  │ → RecallRequest 파싱
  │ → intent 분류 (regex 8종)
  │ → entity/keyword 추출
  │ → time scope 감지
  │ → 쿼리 확장 (max 5, 검색에는 상위 3개 사용)
  │ → ParsedQuery
  ▼
[Stage 2] 멀티쿼리 벡터 검색 (search.go)
  │ → 각 확장 쿼리(최대 3개):
  │      embed.EmbedSingle(q)          → []float32 (1024-dim, L2 normalized)
  │      enVectorSDK.Score(index, vec) → []string  (FHE ciphertext, base64)
  │      vault.DecryptScores(blob, K)  → []ScoreEntry{ShardIdx, RowIdx, Score}
  │      enVectorSDK.Remind(index, entries, ["metadata"])
  │                                    → []string  (AES ciphertext JSON)
  │      vault.DecryptMetadata(list)   → []map[string]any (plaintext metadata)
  │ → dedup (record_id 기준, 높은 score 유지)
  │ → phase chain 확장 (누락된 sibling 검색)
  │ → group 조립 (phase_seq 순서)
  │ → raw records
  ▼
[Stage 3] 필터 + 재랭킹 (rerank.go)
  │ → domain/status/since 메타데이터 필터
  │ → time scope 필터 (ParsedQuery.TimeScope 기반)
  │ → recency 가중 (90일 half-life)
  │ → status 승수
  │ → 최종 정렬 (adjustedScore 내림차순)
  │ → topK 절단
  ▼
HTTP 200 {"ok":true,"found":3,"results":[...],"confidence":0.82}
```

---

## 2. Stage 1: 쿼리 분석 (query.go)

### 2.1 입력

```go
type RecallRequest struct {
    Query   string        `json:"query"`
    TopK    int           `json:"topk"`          // 기본값 5, 최대 10
    Filters RecallFilters `json:"filters"`
}

type RecallFilters struct {
    Domain string `json:"domain,omitempty"`  // e.g. "architecture", "security"
    Status string `json:"status,omitempty"`  // e.g. "accepted", "proposed"
    Since  string `json:"since,omitempty"`   // ISO 8601 날짜, e.g. "2026-01-01"
}
```

`TopK`가 10을 초과하면 즉시 에러 반환:

```go
if req.TopK > 10 {
    return ErrorResponse{Code: "invalid_input", Message: "topk must be 10 or less"}
}
```

### 2.2 처리

쿼리 프로세서는 다음 5단계를 순차 실행한다.

#### 2.2.1 텍스트 정리

```go
func cleanQuery(query string) string {
    cleaned := strings.ToLower(strings.TrimSpace(query))
    cleaned = reMultiSpace.ReplaceAllString(cleaned, " ")       // 다중 공백 → 단일
    cleaned = reTrailingPunct.ReplaceAllString(cleaned, "")     // 끝의 .!,;: 제거 (? 유지)
    return cleaned
}
```

#### 2.2.2 Intent 분류 (regex, 8종)

정리된 쿼리에 대해 패턴을 순서대로 매칭한다. 첫 번째 매치가 intent가 된다.

| Intent | 패턴 (case-insensitive) |
|---|---|
| `decision_rationale` | `why did we (choose\|decide\|go with\|select\|pick)`, `what was the (reasoning\|rationale\|logic\|thinking)`, `why .+ over .+`, `what were the (reasons\|factors)`, `why (not\|didn't we)`, `reasoning behind` |
| `feature_history` | `(have\|did) (customers?\|users?) (asked\|requested\|wanted)`, `feature request`, `why did we (reject\|say no\|decline)`, `(how many\|which) customers`, `customer feedback (on\|about)` |
| `pattern_lookup` | `how do we (handle\|deal with\|approach\|manage)`, `what'?s our (approach\|process\|standard\|convention)`, `is there (an?\|existing) (pattern\|standard\|convention)`, `what'?s the (best practice\|recommended way)`, `how should (we\|I)` |
| `technical_context` | `what'?s our (architecture\|design\|system) for`, `how (does\|is) .+ (implemented\|built\|designed)`, `(explain\|describe) (the\|our) .+ (system\|architecture\|design)`, `technical (details\|overview) (of\|for)` |
| `security_compliance` | `(security\|compliance) (requirements?\|considerations?)`, `what (security\|privacy) (measures\|controls)`, `(gdpr\|hipaa\|sox\|pci) (requirements?\|compliance)`, `audit (requirements?\|trail)` |
| `historical_context` | `when did we (decide\|choose\|implement\|launch)`, `(history\|timeline) of`, `(have\|did) we (ever\|previously)`, `how long (have\|has) .+ been` |
| `attribution` | `who (decided\|chose\|approved\|owns)`, `which (team\|person\|group) (is responsible\|decided\|owns)`, `(owner\|maintainer) of` |
| `general` | 어떤 패턴에도 매치되지 않을 때의 기본값 |

```go
type QueryIntent string

const (
    IntentDecisionRationale QueryIntent = "decision_rationale"
    IntentFeatureHistory    QueryIntent = "feature_history"
    IntentPatternLookup     QueryIntent = "pattern_lookup"
    IntentTechnicalContext  QueryIntent = "technical_context"
    IntentSecurityCompliance QueryIntent = "security_compliance"
    IntentHistoricalContext QueryIntent = "historical_context"
    IntentAttribution       QueryIntent = "attribution"
    IntentGeneral           QueryIntent = "general"
)

func detectIntent(query string) QueryIntent {
    lower := strings.ToLower(query)
    for _, entry := range intentPatterns {  // 순서 보장된 슬라이스
        for _, pattern := range entry.Patterns {
            if pattern.MatchString(lower) {
                return entry.Intent
            }
        }
    }
    return IntentGeneral
}
```

#### 2.2.3 Entity 추출

세 가지 소스에서 엔티티를 추출한다:

1. **인용 문자열**: `"PostgreSQL"` 또는 `'Redis'` 같은 따옴표 안의 문자열
2. **대문자 단어/구**: 문장 시작이 아닌 위치의 대문자 시작 단어 연속체 (Proper noun 후보)
3. **기술 이름 패턴**: 미리 정의된 기술 이름 regex 매치

```go
var techPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)\b(PostgreSQL|MySQL|MongoDB|Redis|Elasticsearch|Kafka)\b`),
    regexp.MustCompile(`(?i)\b(React|Vue|Angular|Next\.js|Node\.js|Python|Java|Go)\b`),
    regexp.MustCompile(`(?i)\b(AWS|GCP|Azure|Kubernetes|Docker|Terraform)\b`),
    regexp.MustCompile(`(?i)\b(REST|GraphQL|gRPC|WebSocket|HTTP|HTTPS)\b`),
}

func extractEntities(query string) []string {
    var entities []string
    // 1. 인용 문자열 추출
    // 2. 대문자 구 추출 (문장 시작 제외)
    // 3. 기술 이름 패턴 매칭
    // 중복 제거, 최대 10개 반환
    return dedup(entities)[:min(len(entities), 10)]
}
```

#### 2.2.4 Keyword 추출

쿼리를 단어 단위로 분리 후, stop word와 2글자 이하 단어를 제거한다.

```go
var stopWords = map[string]bool{
    "the": true, "a": true, "an": true, "is": true, "are": true,
    "was": true, "were": true, "be": true, "been": true, "being": true,
    "have": true, "has": true, "had": true, "do": true, "does": true,
    "did": true, "will": true, "would": true, "could": true, "should": true,
    // ... (총 ~80개)
}

func extractKeywords(query string) []string {
    words := reWordBoundary.FindAllString(strings.ToLower(query), -1)
    var keywords []string
    for _, w := range words {
        if !stopWords[w] && len(w) > 2 {
            keywords = append(keywords, w)
        }
    }
    return dedup(keywords)[:min(len(keywords), 15)]
}
```

#### 2.2.5 Time Scope 감지

쿼리 텍스트에서 시간 범위 표현을 regex로 감지한다.

| TimeScope | 패턴 |
|---|---|
| `LAST_WEEK` | `last week`, `this week`, `past week`, `7 days` |
| `LAST_MONTH` | `last month`, `this month`, `past month`, `30 days` |
| `LAST_QUARTER` | `last quarter`, `this quarter`, `Q[1-4]`, `past 3 months` |
| `LAST_YEAR` | `last year`, `this year`, `20\d{2}`, `past year` |
| `ALL_TIME` | 아무 패턴에도 매치되지 않을 때 (기본값) |

```go
type TimeScopeType string

const (
    ScopeLastWeek    TimeScopeType = "LAST_WEEK"
    ScopeLastMonth   TimeScopeType = "LAST_MONTH"
    ScopeLastQuarter TimeScopeType = "LAST_QUARTER"
    ScopeLastYear    TimeScopeType = "LAST_YEAR"
    ScopeAllTime     TimeScopeType = "ALL_TIME"
)

// cutoff 계산
var timeScopeDeltas = map[TimeScopeType]time.Duration{
    ScopeLastWeek:    7 * 24 * time.Hour,
    ScopeLastMonth:   30 * 24 * time.Hour,
    ScopeLastQuarter: 90 * 24 * time.Hour,
    ScopeLastYear:    365 * 24 * time.Hour,
}
```

#### 2.2.6 쿼리 확장

Intent와 entity를 기반으로 대안 쿼리를 생성한다. 원본 포함 최대 5개.

```go
func generateExpansions(query string, intent QueryIntent, entities []string) []string {
    expansions := []string{query}  // 원본 항상 포함

    switch intent {
    case IntentDecisionRationale:
        expansions = append(expansions,
            "decision "+query,
            "rationale "+query,
            "trade-off "+query,
        )
    case IntentFeatureHistory:
        expansions = append(expansions,
            "customer request "+query,
            "feature rejected "+query,
        )
    case IntentPatternLookup:
        expansions = append(expansions,
            "standard approach "+query,
            "best practice "+query,
        )
    case IntentTechnicalContext:
        expansions = append(expansions,
            "architecture "+query,
            "implementation "+query,
        )
    }

    // entity 기반 확장
    for _, entity := range entities[:min(len(entities), 3)] {
        expansions = append(expansions, entity+" decision", "why "+entity)
    }

    return dedupLower(expansions)[:min(len(expansions), 5)]
}
```

**비영어 쿼리 처리**: 비영어 쿼리가 감지되면 LLM을 호출하여 intent 분류 + 영어 번역을 수행한다. 이 경우 확장 쿼리는 원본(비영어) + 영어 번역 + 영어 기반 확장으로 구성되며, 최대 7개까지 생성된다. Go 포팅 시에도 이 이중 경로(regex / LLM)를 유지해야 한다.

### 2.3 출력

```go
type ParsedQuery struct {
    Original        string        // 원본 쿼리 텍스트
    Cleaned         string        // 정리된 쿼리 (lowercase, 공백 정규화)
    Intent          QueryIntent   // "decision_rationale", "general", etc.
    Entities        []string      // ["PostgreSQL"]
    Keywords        []string      // ["결정", "이유", "postgresql"]
    TimeScope       *TimeScope    // nil이면 ALL_TIME
    ExpandedQueries []string      // 최대 5개, 검색에는 상위 3개만 사용
    Language        *LanguageInfo // 감지된 언어 정보 (nil이면 영어)
}

type TimeScope struct {
    Type   TimeScopeType // "LAST_WEEK", "LAST_MONTH", "LAST_QUARTER", "LAST_YEAR"
    Cutoff time.Time     // now - delta로 계산
}
```

---

## 3. Stage 2: 멀티쿼리 벡터 검색 (search.go)

### 3.1 FHE Round-Trip 상세

확장 쿼리 상위 3개 각각에 대해 다음 4단계 파이프라인을 실행한다.
각 단계에서의 정확한 데이터 변환:

```
q = "PostgreSQL 결정 이유"
  │
  ▼ embed.EmbedSingle(q)
vec = []float32{0.12, -0.34, 0.56, ...}  // 1024-dim, L2 normalized
  │                                        // 모델: Qwen/Qwen3-Embedding-0.6B (로컬)
  │
  ▼ enVectorSDK.Score(indexName, vec)
blobs = []string{"aGVsbG8gd29ybGQ=..."}  // FHE result ciphertext (base64)
  │                                        // runed는 이 blob의 내용을 모름 (opaque)
  │                                        // enVector Cloud에서 FHE cosine similarity 연산 수행
  │
  ▼ vault.DecryptScores(blobs[0], topK=5)
entries = []ScoreEntry{
    {ShardIdx: 0, RowIdx: 42, Score: 0.87},
    {ShardIdx: 0, RowIdx: 15, Score: 0.73},
    {ShardIdx: 0, RowIdx: 91, Score: 0.65},
}
  │  // Score는 이제 plaintext cosine similarity (0.0 ~ 1.0)
  │  // Vault가 FHE secret key로 복호화 → top-K 선택 → 인덱스+점수 반환
  │  // Secret key는 Vault 밖으로 나가지 않음
  │
  ▼ enVectorSDK.Remind(indexName, entries, ["metadata"])
encryptedMeta = []struct{Data string}{
    {Data: `{"a":"agent_xyz","c":"SGVsbG8gV29ybGQ=..."}`},  // AES ciphertext
    {Data: `{"a":"agent_xyz","c":"dGVzdCBkYXRh..."}`},
    {Data: `{"a":"agent_xyz","c":"ZXhhbXBsZQ==..."}`},
}
  │  // "a" = agent ID, "c" = AES-encrypted metadata (base64)
  │  // enVector Cloud는 평문 메타데이터를 모름
  │
  ▼ vault.DecryptMetadata(encryptedMeta)
plainMeta = []map[string]any{
    {"id":"dec_2024-pg","title":"PostgreSQL 선택","domain":"architecture",
     "status":"accepted","why":{"certainty":"supported"},"payload":{"text":"..."},
     "reusable_insight":"PostgreSQL을 선택한 이유는...","timestamp":"2025-12-01T..."},
    {"id":"dec_2024-bench","title":"DB 벤치마크 결과","domain":"performance",
     "status":"accepted",...},
    {"id":"dec_2024-mongo","title":"MongoDB 평가","domain":"architecture",
     "status":"superseded",...},
}
```

Go 구현에서의 Vault 호출 시그니처:

```go
// DecryptScores: FHE ciphertext → plaintext scores
type DecryptScoresRequest struct {
    Token             string // Vault 인증 토큰
    EncryptedBlobB64  string // base64-encoded FHE result ciphertext
    TopK              int    // 반환할 상위 결과 수
}

type ScoreEntry struct {
    ShardIdx int     `json:"shard_idx"`
    RowIdx   int     `json:"row_idx"`
    Score    float64 `json:"score"`     // plaintext cosine similarity
}

type DecryptResult struct {
    OK      bool         // 성공 여부
    Results []ScoreEntry // top-K score entries
    Error   string       // 실패 시 에러 메시지
}

// DecryptMetadata: AES ciphertext → plaintext metadata JSON
type DecryptMetadataRequest struct {
    Token                 string   // Vault 인증 토큰
    EncryptedMetadataList []string // AES-encrypted metadata JSON strings
}
// → []map[string]any (각 메타데이터 객체의 복호화된 JSON)
```

### 3.2 멀티쿼리 Dedup

여러 확장 쿼리에서 겹치는 결과가 나올 때의 처리:

```go
func searchWithExpansions(parsed ParsedQuery, topK int) []SearchResult {
    var allResults []SearchResult
    seen := make(map[string]int)  // record_id → allResults 내 인덱스

    // 확장 쿼리 상위 3개 순회
    for _, q := range parsed.ExpandedQueries[:min(len(parsed.ExpandedQueries), 3)] {
        results := searchSingle(q, topK)
        for _, r := range results {
            if idx, exists := seen[r.RecordID]; exists {
                // 이미 존재: 더 높은 score 유지
                if r.Score > allResults[idx].Score {
                    allResults[idx].Score = r.Score
                }
            } else {
                seen[r.RecordID] = len(allResults)
                allResults = append(allResults, r)
            }
        }
    }

    // 원본 쿼리가 확장 쿼리에 없으면 추가 검색
    if !contains(parsed.ExpandedQueries, parsed.Original) {
        results := searchSingle(parsed.Original, topK)
        for _, r := range results {
            if _, exists := seen[r.RecordID]; !exists {
                seen[r.RecordID] = len(allResults)
                allResults = append(allResults, r)
            }
        }
    }

    // score 내림차순 정렬
    sort.Slice(allResults, func(i, j int) bool {
        return allResults[i].Score > allResults[j].Score
    })
    return allResults
}
```

### 3.3 Phase Chain 확장

검색 결과에 phase chain 레코드(group_id가 있는 레코드)가 포함되어 있으나
sibling이 누락된 경우, 추가 검색을 수행한다:

```go
func expandPhaseChains(results []SearchResult, maxChains int) []SearchResult {
    // maxChains = 2 (최대 2개 체인만 확장)
    // 1. 이미 존재하는 결과에서 group_id별 present/total 비교
    // 2. present < total인 group_id에 대해 "Group: <group_id>" 쿼리로 sibling 검색
    // 3. 기존 결과와 sibling을 phase_seq 순서로 조립
    // 4. 중복 record_id 제거
    return expanded
}
```

### 3.4 Group 조립

phase chain과 bundle 결과를 올바른 순서로 조립한다:

```go
func assembleGroups(results []SearchResult) []SearchResult {
    // 1. group_id 있는 결과와 standalone 결과 분리
    // 2. 같은 group_id의 결과를 phase_seq 순서로 정렬
    // 3. 각 그룹의 최고 score를 기준으로 standalone과 interleave
    //    → 그룹의 best score 위치에 그룹 전체를 삽입
    return assembled
}
```

### 3.5 에러 처리

각 외부 호출 실패 시의 동작:

| 실패 지점 | 동작 | 근거 |
|---|---|---|
| `embed.EmbedSingle` 실패 | 빈 결과 반환 | 벡터 없이 검색 불가 |
| `enVector.Score` 실패 | 빈 결과 반환 | FHE 연산 결과 없이 진행 불가 |
| `vault.DecryptScores` 실패 | 에러 반환 + 상태 dormant 전환 | Vault 인증/연결 문제 → `vault_unreachable` |
| `enVector.Remind` 실패 | 빈 결과 반환 | 메타데이터 없으면 의미 있는 결과 구성 불가 |
| `vault.DecryptMetadata` 실패 | per-entry fallback 시도 → 실패 시 빈 metadata로 처리 | batch 실패 시 개별 복호화 시도 |
| 네트워크 에러 (`ConnectionError`, `OSError`) | 에러 반환 + 상태 dormant 전환 | `envector_unreachable` |

**Vault 에러 시 dormant 전환**: Vault 또는 enVector 연결 실패 시 `config.json`의 state를 `"dormant"`로 변경하고 `dormant_reason` 필드를 설정한다. 이후 recall 호출 시 파이프라인 미초기화 에러를 반환한다.

**메타데이터 복호화 fallback 순서**:

```go
// 1. JSON 파싱 시도 → AES ciphertext 패턴 확인 {"a":..., "c":...}
// 2. AES ciphertext면 vault.DecryptMetadata batch 호출
// 3. batch 실패 시 per-entry 개별 복호화
// 4. 개별 복호화도 실패 시 빈 metadata로 처리
// 5. JSON이지만 AES가 아니면 그 자체가 평문 metadata
// 6. JSON 파싱 실패 시 base64 디코딩 후 JSON 파싱 시도
```

---

## 4. Stage 3: 필터 + 재랭킹 (rerank.go)

### 4.1 메타데이터 필터

Best-effort 클라이언트 사이드 필터링. Vault가 반환한 top-K 결과에 대해서만
적용하므로, 필터링 후 결과 수가 topK 미만이 될 수 있다.

```go
func applyMetadataFilters(results []SearchResult, filters RecallFilters) []SearchResult {
    filtered := results

    if filters.Domain != "" {
        var tmp []SearchResult
        for _, r := range filtered {
            if r.Metadata.Domain == filters.Domain {
                tmp = append(tmp, r)
            }
        }
        filtered = tmp
    }

    if filters.Status != "" {
        var tmp []SearchResult
        for _, r := range filtered {
            if r.Metadata.Status == filters.Status {
                tmp = append(tmp, r)
            }
        }
        filtered = tmp
    }

    if filters.Since != "" {
        var tmp []SearchResult
        for _, r := range filtered {
            ts := r.Metadata.Timestamp
            if ts == "" || ts >= filters.Since {
                tmp = append(tmp, r)  // timestamp 없으면 유지
            }
        }
        filtered = tmp
    }

    return filtered
}
```

**주의**: 이 필터링은 Vault가 이미 반환한 결과에 대한 후처리다. 이상적으로는
Vault 측에서 over-fetch + 필터링을 수행해야 하지만, 현재는 클라이언트 측에서
best-effort로 처리한다. 결과 수 감소는 응답의 `warnings` 필드로 보고한다.

### 4.2 시간 범위 필터

`ParsedQuery.TimeScope`이 설정된 경우 (쿼리 텍스트에서 시간 표현이 감지된 경우):

```go
func filterByTime(results []SearchResult, scope TimeScope) []SearchResult {
    if scope.Type == ScopeAllTime {
        return results
    }

    cutoff := scope.Cutoff  // now - delta

    var filtered []SearchResult
    for _, r := range results {
        ts, err := parseTimestamp(r.Metadata.Timestamp)
        if err != nil {
            filtered = append(filtered, r)  // 파싱 실패 시 유지
            continue
        }
        if !ts.Before(cutoff) {
            filtered = append(filtered, r)
        }
    }
    return filtered
}
```

**timestamp 파싱**: ISO 8601 문자열과 Unix timestamp 숫자 모두 지원한다.
`"Z"` suffix는 `"+00:00"`으로 치환 후 파싱한다.

### 4.3 Recency 가중

시간 감쇠 공식 (지수적 감쇠, half-life 90일):

```
ageDays = now.Sub(record.Timestamp).Hours() / 24
decay = 0.5 ^ (ageDays / 90)
```

감쇠 값 예시:

| 경과일 | decay 값 |
|---|---|
| 0일 | 1.000 |
| 30일 | 0.794 |
| 90일 | 0.500 |
| 180일 | 0.250 |
| 365일 | 0.062 |

```go
const (
    HalfLifeDays     = 90
    SimilarityWeight = 0.7
    RecencyWeight    = 0.3
)

func recencyDecay(ageDays float64) float64 {
    if HalfLifeDays <= 0 {
        return 1.0
    }
    return math.Pow(0.5, ageDays/float64(HalfLifeDays))
}
```

### 4.4 Status 승수

레코드의 status에 따른 점수 승수:

```go
var statusMultipliers = map[string]float64{
    "accepted":   1.0,
    "proposed":   0.9,
    "superseded": 0.5,
    "reverted":   0.3,
}

func statusMultiplier(status string) float64 {
    if mult, ok := statusMultipliers[status]; ok {
        return mult
    }
    return 1.0  // 알 수 없는 status는 감쇠 없음
}
```

**주의**: Python 코드의 기본값은 `1.0`이다. 이는 `"active"` 등의 status가
`statusMultipliers` 맵에 없을 때 감쇠 없이 원점수를 유지한다는 의미다.

### 4.5 최종 점수 계산

```go
func applyRecencyWeighting(results []SearchResult) {
    now := time.Now().UTC()

    for i := range results {
        r := &results[i]
        ageDays := 0.0

        if ts, err := parseTimestamp(r.Metadata.Timestamp); err == nil {
            ageDays = math.Max(0, now.Sub(ts).Hours()/24)
        }

        decay := recencyDecay(ageDays)
        statusMult := statusMultiplier(r.Status)

        // adjustedScore = (similarity * weight + recency * weight) * statusMultiplier
        r.AdjustedScore = (SimilarityWeight*r.Score + RecencyWeight*decay) * statusMult
    }

    // adjustedScore 내림차순 정렬
    sort.Slice(results, func(i, j int) bool {
        return results[i].AdjustedScore > results[j].AdjustedScore
    })
}
```

**점수 공식 요약**:

```
adjustedScore = (0.7 * rawScore + 0.3 * recencyDecay) * statusMultiplier
```

이는 순수 cosine similarity (`rawScore`)와 시간 감쇠(`recencyDecay`)를 7:3
비율로 혼합한 후, status에 따른 페널티를 적용하는 구조다.

예시: rawScore=0.87, 30일 경과, status="accepted"일 때:

```
decay = 0.5^(30/90) = 0.794
adjustedScore = (0.7 * 0.87 + 0.3 * 0.794) * 1.0
              = (0.609 + 0.238) * 1.0
              = 0.847
```

---

## 5. 응답 포맷

### 5.1 정상 응답 (agent-side synthesis, 기본 경로)

LLM 키가 없거나 synthesizer가 초기화되지 않은 경우. 에이전트가 직접 합성한다.

```go
type RecallResponse struct {
    OK          bool           `json:"ok"`           // true
    Found       int            `json:"found"`        // 결과 수
    Results     []RecallResult `json:"results"`      // 복호화된 결과 목록
    Confidence  float64        `json:"confidence"`   // 전체 신뢰도 (0.0~1.0)
    Sources     []Source       `json:"sources"`      // 상위 5개 요약
    Synthesized bool           `json:"synthesized"`  // false (agent가 합성)
}

type RecallResult struct {
    RecordID   string `json:"record_id"`
    Title      string `json:"title"`
    Content    string `json:"content"`      // payload.text (합성 원본)
    Domain     string `json:"domain"`
    Certainty  string `json:"certainty"`    // "supported" | "partially_supported" | "unknown"
    Score      float64 `json:"score"`       // raw cosine similarity

    // phase chain / bundle인 경우에만 포함
    GroupID    string `json:"group_id,omitempty"`
    GroupType  string `json:"group_type,omitempty"`   // "phase_chain" | "bundle"
    PhaseSeq   *int   `json:"phase_seq,omitempty"`
    PhaseTotal *int   `json:"phase_total,omitempty"`
}

type Source struct {
    RecordID  string  `json:"record_id"`
    Title     string  `json:"title"`
    Domain    string  `json:"domain"`
    Certainty string  `json:"certainty"`
    Score     float64 `json:"score"`
}
```

### 5.2 정상 응답 (server-side synthesis)

LLM 키가 설정되어 synthesizer가 활성화된 경우:

```go
type RecallSynthesizedResponse struct {
    OK             bool     `json:"ok"`
    Found          int      `json:"found"`
    Answer         string   `json:"answer"`           // LLM 합성 답변
    Confidence     float64  `json:"confidence"`
    Sources        []Source `json:"sources"`
    Warnings       []string `json:"warnings,omitempty"`
    RelatedQueries []string `json:"related_queries,omitempty"`
    Synthesized    bool     `json:"synthesized"`       // true
}
```

### 5.3 Confidence 계산

```go
func calculateConfidence(results []SearchResult) float64 {
    if len(results) == 0 {
        return 0.0
    }

    certaintyWeights := map[string]float64{
        "supported":           1.0,
        "partially_supported": 0.6,
        "unknown":             0.3,
    }

    totalScore := 0.0
    for i, r := range results[:min(len(results), 5)] {
        positionWeight := 1.0 / float64(i+1)   // 1위=1.0, 2위=0.5, 3위=0.33, ...
        certWeight := certaintyWeights[r.Certainty]
        if certWeight == 0 {
            certWeight = 0.3
        }
        totalScore += positionWeight * certWeight * r.Score
    }

    return math.Round(math.Min(1.0, totalScore/2.0)*100) / 100
}
```

이 공식은 상위 5개 결과의 가중 합을 2로 나눠 0~1 범위로 정규화한다.
position weight는 상위 결과에 더 큰 가중치를 부여한다.

### 5.4 에러 응답

```go
type RecallErrorResponse struct {
    OK           bool   `json:"ok"`            // false
    Error        string `json:"error"`         // 에러 메시지
    ErrorType    string `json:"error_type"`    // "vault_decryption", "envector_connection", etc.
    RecoveryHint string `json:"recovery_hint"` // 복구 방법 안내
}
```

에러 유형별 응답:

| ErrorType | 상황 | RecoveryHint |
|---|---|---|
| `pipeline_not_ready` | 파이프라인 미초기화 | "Run /rune:activate to reinitialize pipelines" |
| `invalid_input` | topK > 10, 빈 쿼리 등 | 입력 관련 안내 |
| `vault_decryption` | Vault 인증/연결 실패 | "Is your Vault token valid?" + 진단 안내 |
| `envector_connection` | enVector 연결 실패 | "Is the enVector endpoint reachable?" + 진단 안내 |

---

## 6. 성능 특성

### 6.1 단계별 지연

| 단계 | 예상 지연 | 네트워크 호출 | 비고 |
|---|---|---|---|
| query processor | < 1ms | 없음 | regex + 문자열 처리 (영어 경로) |
| query processor (비영어) | 500-2000ms | 1 LLM API | LLM intent 분류 + 번역 |
| embed (per query) | 10-50ms | 없음 | 로컬 ONNX/fastembed |
| enVector.Score | 50-200ms | 1 gRPC call | FHE cosine similarity 연산 |
| vault.DecryptScores | 20-100ms | 1 gRPC call | FHE 복호화 + top-K 선택 |
| enVector.Remind | 30-100ms | 1 gRPC call | 메타데이터 조회 |
| vault.DecryptMetadata | 20-100ms | 1 gRPC call | AES 복호화 (batch) |
| reranker | < 1ms | 없음 | 수학 연산 + 정렬 |

### 6.2 총 지연 추정

| 시나리오 | 예상 총 지연 |
|---|---|
| 1 쿼리 (순차) | ~130-550ms |
| 3 확장 쿼리 (순차) | ~350-1500ms |
| 3 확장 쿼리 (병렬 goroutine) | ~130-550ms |
| 3 확장 쿼리 + phase chain 확장 (순차) | ~500-2000ms |

### 6.3 병렬화 기회

각 확장 쿼리의 `searchSingle` 호출은 독립적이므로 goroutine으로 병렬 실행 가능:

```go
func searchWithExpansionsParallel(parsed ParsedQuery, topK int) []SearchResult {
    queries := parsed.ExpandedQueries[:min(len(parsed.ExpandedQueries), 3)]

    type queryResult struct {
        results []SearchResult
        err     error
    }

    ch := make(chan queryResult, len(queries))

    for _, q := range queries {
        go func(query string) {
            results, err := searchSingle(query, topK)
            ch <- queryResult{results, err}
        }(q)
    }

    var allResults []SearchResult
    seen := make(map[string]int)

    for range queries {
        qr := <-ch
        if qr.err != nil {
            continue  // 개별 쿼리 실패는 무시
        }
        for _, r := range qr.results {
            if idx, exists := seen[r.RecordID]; exists {
                if r.Score > allResults[idx].Score {
                    allResults[idx].Score = r.Score
                }
            } else {
                seen[r.RecordID] = len(allResults)
                allResults = append(allResults, r)
            }
        }
    }

    sort.Slice(allResults, func(i, j int) bool {
        return allResults[i].Score > allResults[j].Score
    })
    return allResults
}
```

**주의**: 병렬화 시 Vault와 enVector의 동시 연결 수 제한을 고려해야 한다.
gRPC 채널은 내부적으로 HTTP/2 다중화를 사용하므로 단일 채널로도 병렬 요청이 가능하다.

---

## 7. 전체 Go 코드 구조

```
retriever/
├── handler.go         # HTTP handler: request 파싱 → pipeline 호출 → response 포맷
│                      #   POST /recall 엔드포인트
│                      #   RecallRequest 검증 (topK <= 10)
│                      #   에러 매핑 (VaultError, ConnectionError → 적절한 HTTP status)
│                      #   confidence 계산
│
├── query.go           # Stage 1: ParsedQuery 생성
│                      #   cleanQuery()        — 텍스트 정리
│                      #   detectIntent()      — regex 기반 8종 intent 분류
│                      #   extractEntities()   — entity 추출 (인용/대문자/기술명)
│                      #   extractKeywords()   — keyword 추출 (stop word 제거)
│                      #   detectTimeScope()   — 시간 범위 감지
│                      #   generateExpansions() — 쿼리 확장 (intent + entity 기반)
│                      #   parseMultilingual() — LLM 기반 비영어 쿼리 처리
│
├── search.go          # Stage 2: 멀티쿼리 FHE 검색
│                      #   searchWithExpansions() — 확장 쿼리별 검색 + dedup
│                      #   searchSingle()         — 단일 쿼리 Vault 파이프라인
│                      #   expandPhaseChains()    — 누락 sibling 검색
│                      #   assembleGroups()       — phase chain/bundle 조립
│                      #   toSearchResult()       — raw entry → SearchResult 변환
│
└── rerank.go          # Stage 3: 필터 + 재랭킹
                       #   applyMetadataFilters() — domain/status/since 필터
                       #   filterByTime()          — TimeScope 기반 시간 필터
                       #   applyRecencyWeighting() — 시간 감쇠 + status 승수
                       #   calculateConfidence()   — 전체 신뢰도 계산
```

### 7.1 SearchResult 구조체

```go
type SearchResult struct {
    RecordID       string            // 레코드 고유 ID
    Title          string            // 레코드 제목
    PayloadText    string            // payload.text (합성용 핵심 텍스트)
    Domain         string            // "architecture", "security", etc.
    Certainty      string            // "supported", "partially_supported", "unknown"
    Status         string            // "accepted", "proposed", "superseded", "reverted"
    Score          float64           // raw cosine similarity (0.0~1.0)
    ReusableInsight string           // Schema 2.1+: 밀도 높은 NL gist (임베딩 텍스트)
    AdjustedScore  float64           // recency + status 가중 적용 후
    Metadata       map[string]any    // 전체 복호화된 메타데이터

    // Group 필드 (phase_chain 또는 bundle)
    GroupID    *string // nil이면 standalone
    GroupType  *string // "phase_chain" | "bundle"
    PhaseSeq   *int    // 그룹 내 순서 (0-based)
    PhaseTotal *int    // 그룹 내 총 항목 수
}

// 편의 메서드
func (r SearchResult) IsPhase() bool { return r.GroupID != nil }
func (r SearchResult) IsReliable() bool {
    return r.Certainty == "supported" || r.Certainty == "partially_supported"
}
```

### 7.2 메타데이터 → SearchResult 매핑

Vault에서 복호화된 메타데이터 JSON을 SearchResult로 변환하는 규칙:

```go
func toSearchResult(raw map[string]any, score float64) SearchResult {
    metadata := raw["metadata"].(map[string]any)  // 또는 raw 자체

    // ID: metadata.id > raw.id > "unknown"
    recordID := getStr(metadata, "id", getStr(raw, "id", "unknown"))

    // Title: metadata.title > "Untitled"
    title := getStr(metadata, "title", "Untitled")

    // PayloadText: metadata.payload.text > metadata.text > raw.text
    //              없으면 metadata.decision.what
    payloadText := ""
    if payload, ok := metadata["payload"].(map[string]any); ok {
        payloadText = getStr(payload, "text", "")
    }
    if payloadText == "" {
        payloadText = getStr(metadata, "text", getStr(raw, "text", ""))
    }
    if payloadText == "" {
        if decision, ok := metadata["decision"].(map[string]any); ok {
            payloadText = getStr(decision, "what", "")
        }
    }

    // Certainty: metadata.why.certainty > "unknown"
    certainty := "unknown"
    if why, ok := metadata["why"].(map[string]any); ok {
        certainty = getStr(why, "certainty", "unknown")
    }

    // ReusableInsight: metadata.reusable_insight > ""
    reusableInsight := getStr(metadata, "reusable_insight", "")

    return SearchResult{
        RecordID:        recordID,
        Title:           title,
        PayloadText:     payloadText,
        Domain:          getStr(metadata, "domain", "general"),
        Certainty:       certainty,
        Status:          getStr(metadata, "status", "unknown"),
        Score:           score,
        ReusableInsight: reusableInsight,
        AdjustedScore:   score,   // 초기값은 rawScore와 동일
        Metadata:        metadata,
        GroupID:          getStrPtr(metadata, "group_id"),
        GroupType:        getStrPtr(metadata, "group_type"),
        PhaseSeq:         getIntPtr(metadata, "phase_seq"),
        PhaseTotal:       getIntPtr(metadata, "phase_total"),
    }
}
```
