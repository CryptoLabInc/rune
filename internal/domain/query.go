package domain

// Query / recall types + internal search types.
// Spec: docs/v04/spec/types.md §1.7-1.8, §4.2, §5.1-5.3.
// Python: agents/retriever/query_processor.py · searcher.py.

// ─────────────────────────────────────────────────────────────────────────────
// §1.7 QueryIntent (8 values)
// ─────────────────────────────────────────────────────────────────────────────

// QueryIntent — Python: query_processor.py:L23-32.
type QueryIntent string

const (
	QueryIntentDecisionRationale  QueryIntent = "decision_rationale"
	QueryIntentFeatureHistory     QueryIntent = "feature_history"
	QueryIntentPatternLookup      QueryIntent = "pattern_lookup"
	QueryIntentTechnicalContext   QueryIntent = "technical_context"
	QueryIntentSecurityCompliance QueryIntent = "security_compliance"
	QueryIntentHistoricalContext  QueryIntent = "historical_context"
	QueryIntentAttribution        QueryIntent = "attribution"
	QueryIntentGeneral            QueryIntent = "general"
)

// ─────────────────────────────────────────────────────────────────────────────
// §1.8 TimeScope (5 values)
// ─────────────────────────────────────────────────────────────────────────────

// TimeScope — Python: query_processor.py:L35-41.
type TimeScope string

const (
	TimeScopeLastWeek    TimeScope = "last_week"    // 7 days
	TimeScopeLastMonth   TimeScope = "last_month"   // 30 days
	TimeScopeLastQuarter TimeScope = "last_quarter" // 90 days
	TimeScopeLastYear    TimeScope = "last_year"    // 365 days
	TimeScopeAllTime     TimeScope = "all_time"     // no filter (default)
)

// ─────────────────────────────────────────────────────────────────────────────
// §4.2 RecallArgs / RecallResult
// ─────────────────────────────────────────────────────────────────────────────

// RecallArgs — §4.2. Python: server.py:L910-1034.
type RecallArgs struct {
	Query  string  `json:"query"`
	TopK   int     `json:"topk,omitempty"`   // default 5, max 10
	Domain *string `json:"domain,omitempty"` // filter
	Status *string `json:"status,omitempty"` // filter
	Since  *string `json:"since,omitempty"`  // ISO date "YYYY-MM-DD"
}

// RecallResult — §4.2. Synthesized is always false (D28 agent-delegated).
type RecallResult struct {
	OK          bool           `json:"ok"`
	Found       int            `json:"found"`
	Results     []RecallEntry  `json:"results"`
	Confidence  float64        `json:"confidence"`
	Sources     []RecallSource `json:"sources"`
	Synthesized bool           `json:"synthesized"` // fixed false
	Error       string         `json:"error,omitempty"`
}

// RecallEntry — §4.2.
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

	GroupID    *string `json:"group_id,omitempty"`
	GroupType  *string `json:"group_type,omitempty"`
	PhaseSeq   *int    `json:"phase_seq,omitempty"`
	PhaseTotal *int    `json:"phase_total,omitempty"`
}

// RecallSource — §4.2.
type RecallSource struct {
	RecordID string `json:"record_id"`
	Title    string `json:"title"`
}

// ─────────────────────────────────────────────────────────────────────────────
// §5.1 SearchHit — recall Phase 3-6 internal pipeline
// ─────────────────────────────────────────────────────────────────────────────

// SearchHit — §5.1. Python: searcher.py:L44-76 SearchResult.
type SearchHit struct {
	RecordID        string
	Title           string
	PayloadText     string
	Domain          string
	Certainty       string
	Status          string
	Score           float64
	ReusableInsight string
	AdjustedScore   float64
	Metadata        map[string]any

	GroupID    *string
	GroupType  *string
	PhaseSeq   *int
	PhaseTotal *int
}

// IsReliable — Python property mirror.
func (h *SearchHit) IsReliable() bool {
	return h.Certainty == "supported" || h.Certainty == "partially_supported"
}

// IsPhase — Python property mirror.
func (h *SearchHit) IsPhase() bool {
	return h.GroupID != nil
}

// ExtractPayloadText — §5.1. Strict v2.1 (D32). No v1/v2.0 fallback.
// Python reference: searcher.py:L487-496 (v0.4 simplified to payload.text only).
func ExtractPayloadText(metadata map[string]any) string {
	payload, ok := metadata["payload"].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := payload["text"].(string)
	return text
}

// ─────────────────────────────────────────────────────────────────────────────
// §5.2 ParsedQuery — recall Phase 2 result
// ─────────────────────────────────────────────────────────────────────────────

// ParsedQuery — §5.2. Python: query_processor.py:L44-54.
// No Language field (D21 — agent pre-translates).
type ParsedQuery struct {
	Original        string
	Cleaned         string
	Intent          QueryIntent
	TimeScope       TimeScope
	Entities        []string // max 10
	Keywords        []string // max 15
	ExpandedQueries []string // max 5
}

// ─────────────────────────────────────────────────────────────────────────────
// §5.3 Detection — capture Phase 2 result
// ─────────────────────────────────────────────────────────────────────────────

// Detection — §5.3. Built from agent data (not Python's full DetectionResult).
// Python: server.py:L70-87 _detection_from_agent_data.
type Detection struct {
	IsSignificant bool    // agent-delegated: always true (server.py:L82)
	Confidence    float64 // [0.0, 1.0] agent-provided
	Domain        string  // from extracted.tier2.domain, default "general"
}
