package domain

import (
	"fmt"
	"math"
	"strings"
)

// Agent extraction types (ExtractionResult hierarchy).
// Spec: docs/v04/spec/types.md §3a.
// Python: agents/scribe/llm_extractor.py:L28-70 — types only in agent-delegated mode.
//
// Wire format: agent sends flat JSON in CaptureRequest.Extracted.
// Internal: rune-mcp splits into Detection (see query.go) + ExtractionResult.
// Mapping table: see types.md §3a.4.

// ExtractedFields — §3a.1. Single-record extraction (no phases).
// Python: llm_extractor.py:L28-37.
type ExtractedFields struct {
	Title        string   `json:"title,omitempty"` // 60-rune truncate
	Rationale    string   `json:"rationale,omitempty"`
	Problem      string   `json:"problem,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"`
	TradeOffs    []string `json:"trade_offs,omitempty"`
	StatusHint   string   `json:"status_hint,omitempty"` // "proposed" | "accepted" | "rejected"
	Tags         []string `json:"tags,omitempty"`
}

// PhaseExtractedFields — §3a.2. One phase in phase_chain or bundle.
// Python: llm_extractor.py:L40-49.
type PhaseExtractedFields struct {
	PhaseTitle     string   `json:"phase_title,omitempty"` // 60-rune truncate
	PhaseDecision  string   `json:"phase_decision,omitempty"`
	PhaseRationale string   `json:"phase_rationale,omitempty"`
	PhaseProblem   string   `json:"phase_problem,omitempty"`
	Alternatives   []string `json:"alternatives,omitempty"`
	TradeOffs      []string `json:"trade_offs,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// ExtractionResult — §3a.3. Top-level (single / phase_chain / bundle).
// Python: llm_extractor.py:L52-70.
type ExtractionResult struct {
	GroupTitle   string                 `json:"group_title,omitempty"`
	GroupType    string                 `json:"group_type,omitempty"`    // "phase_chain" | "bundle" | "" (single)
	GroupSummary string                 `json:"group_summary,omitempty"` // shared across all phases
	StatusHint   string                 `json:"status_hint,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Confidence   *float64               `json:"confidence,omitempty"` // agent-provided [0.0, 1.0]
	Single       *ExtractedFields       `json:"single,omitempty"`
	Phases       []PhaseExtractedFields `json:"phases,omitempty"` // cap 7 (phase) / 5 (bundle)
}

// IsMultiPhase — Python property: llm_extractor.py:L64-66.
func (r *ExtractionResult) IsMultiPhase() bool {
	return len(r.Phases) > 1
}

// IsBundle — Python property: llm_extractor.py:L68-70.
func (r *ExtractionResult) IsBundle() bool {
	return r.GroupType == "bundle" && len(r.Phases) > 1
}


// ParseExtractionFromAgent builds Detection + ExtractionResult from the flat
// CaptureRequest.Extracted dict sent by the agent. Wire → internal conversion.
//
// Mapping table: docs/v04/spec/types.md §3a.4.
// Python reference: mcp/server/server.py:L1244-1324 (_capture_single).
func ParseExtractionFromAgent(extracted map[string]any) (*Detection, *ExtractionResult, error) {
	if extracted == nil {
		return nil, nil, fmt.Errorf("extracted JSON is nil")
	}

	// Tier 2 detection
	tier2, _ := extracted["tier2"].(map[string]any)
	detection := &Detection{
		IsSignificant: true, // agent-delegated: always true
		Domain:        "general",
	}

	if tier2 != nil {
		// capture=false: early rejection
		if capture, ok := tier2["capture"].(bool); ok && !capture {
			reason, _ := tier2["reason"].(string)
			return detection, nil, &CaptureRejection{Reason: reason}
		}
		if dom, ok := tier2["domain"].(string); ok && dom != "" {
			detection.Domain = dom
		}
	}

	// Confidence: top-level or 0.0
	agentConfidence := 0.0
	if c, ok := extracted["confidence"].(float64); ok {
		agentConfidence = math.Max(0.0, math.Min(1.0, c))
	}
	detection.Confidence = agentConfidence

	var conf *float64
	if agentConfidence > 0.0 {
		conf = &agentConfidence
	}

	phasesRaw, hasPhases := extracted["phases"].([]any)

	if hasPhases && len(phasesRaw) > 1 {
		// Multi-phase or record bundle
		var phases []PhaseExtractedFields
		for i, pRaw := range phasesRaw {
			if i >= 7 {
				break
			}
			p, ok := pRaw.(map[string]any)
			if !ok {
				continue
			}
			phases = append(phases, PhaseExtractedFields{
				PhaseTitle:     truncRunes(strVal(p, "phase_title"), MaxTitleLen),
				PhaseDecision:  strVal(p, "phase_decision"),
				PhaseRationale: strVal(p, "phase_rationale"),
				PhaseProblem:   strVal(p, "phase_problem"),
				Alternatives:   strSlice(p, "alternatives"),
				TradeOffs:      strSlice(p, "trade_offs"),
				Tags:           lowerStrSlice(p, "tags"),
			})
		}

		reusableInsight := strVal(extracted, "reusable_insight")
		if reusableInsight == "" {
			reusableInsight = strVal(extracted, "group_title")
		}

		return detection, &ExtractionResult{
			GroupTitle:   truncRunes(strVal(extracted, "group_title"), MaxTitleLen),
			GroupType:    strVal(extracted, "group_type"),
			GroupSummary: reusableInsight,
			StatusHint:   strings.ToLower(strVal(extracted, "status_hint")),
			Tags:         lowerStrSlice(extracted, "tags"),
			Confidence:   conf,
			Phases:       phases,
		}, nil
	}

	// Single record
	var single ExtractedFields
	if hasPhases && len(phasesRaw) == 1 {
		p, ok := phasesRaw[0].(map[string]any)
		if ok {
			single = ExtractedFields{
				Title:        truncRunes(strVal2(p, "phase_title", strVal(extracted, "title")), MaxTitleLen),
				Rationale:    strVal2(p, "phase_rationale", strVal(extracted, "rationale")),
				Problem:      strVal2(p, "phase_problem", strVal(extracted, "problem")),
				Alternatives: strSlice(p, "alternatives"),
				TradeOffs:    strSlice(p, "trade_offs"),
				StatusHint:   strings.ToLower(strVal(extracted, "status_hint")),
				Tags:         lowerStrSliceOr(p, "tags", lowerStrSlice(extracted, "tags")),
			}
		}
	} else {
		single = ExtractedFields{
			Title:        truncRunes(strVal(extracted, "title"), MaxTitleLen),
			Rationale:    strVal(extracted, "rationale"),
			Problem:      strVal(extracted, "problem"),
			Alternatives: strSlice(extracted, "alternatives"),
			TradeOffs:    strSlice(extracted, "trade_offs"),
			StatusHint:   strings.ToLower(strVal(extracted, "status_hint")),
			Tags:         lowerStrSlice(extracted, "tags"),
		}
	}

	reusableInsight := strVal(extracted, "reusable_insight")

	return detection, &ExtractionResult{
		GroupTitle:   single.Title,
		GroupSummary: reusableInsight,
		StatusHint:   single.StatusHint,
		Tags:         single.Tags,
		Confidence:   conf,
		Single:       &single,
	}, nil
}

type CaptureRejection struct {
	Reason string
}

func (e *CaptureRejection) Error() string {
	if e.Reason != "" {
		return "capture rejected: " + e.Reason
	}
	return "capture rejected by agent"
}

//--- Helper ---//

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func strVal2(m map[string]any, key, fallback string) string {
	v := strVal(m, key)
	if v != "" {
		return v
	}
	return fallback
}

func strSlice(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func lowerStrSlice(m map[string]any, key string) []string {
	raw := strSlice(m, key)
	for i, s := range raw {
		raw[i] = strings.ToLower(s)
	}
	return raw
}

func lowerStrSliceOr(m map[string]any, key string, fallback []string) []string {
	result := lowerStrSlice(m, key)
	if len(result) > 0 {
		return result
	}
	return fallback
}

func truncRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}
