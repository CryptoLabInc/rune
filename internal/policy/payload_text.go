package policy

import (
	"fmt"
	"strings"

	"github.com/envector/rune-go/internal/domain"
)

// Payload text renderer — Python canonical: agents/common/schemas/templates.py (364 LoC).
// D15: line-by-line port. Verified via byte-for-byte golden fixture test.
//
// Subtle behaviors (easy to miss in porting):
//  1. phase_line / group_summary post-insertion (L204-216) — inserted AFTER
//     template.format(), not in the template string itself
//  2. Blank line collapse + strip (L219-222): while "\n\n\n" in text:
//     text = text.replace("\n\n\n", "\n\n"); then .strip()
//  3. _format_alternatives "chosen" marker bug (L59): chosen=="" makes all
//     alternatives marked "(chosen)" — Python current behavior, keep bit-identical

const payloadTemplate = `
# Decision Record: %s
ID: %s
Status: %s | Sensitivity: %s | Domain: %s
When/Where: %s | %s

## Decision
%s

## Problem
%s

## Alternatives Considered
%s

## Why (Rationale)
%s
Certainty: %s

## Trade-offs
%s

## Assumptions
%s

## Risks & Mitigations
%s

## Evidence (Quotes)
%s

## Links
%s

## Tags
%s
`

// FIXME: if chosen == "", every alternatievs matches since "".lower() == alt.lower() always true
func formatAlternatives(alternatives []string, chosen string) string {
	if len(alternatives) == 0 {
		return "- (none documented)"
	}
	var lines []string
	chosenLower := strings.ToLower(chosen)
	for _, alt := range alternatives {
		altLower := strings.ToLower(alt)
		if altLower == chosenLower || strings.Contains(altLower, chosenLower) {
			lines = append(lines, fmt.Sprintf("- %s (chosen)", alt))
		} else {
			lines = append(lines, fmt.Sprintf("- %s", alt))
		}
	}
	return strings.Join(lines, "\n")
}

func formatTradeOffs(tradeOffs []string) string {
	if len(tradeOffs) == 0 {
		return "- (none documented)"
	}
	var lines []string
	for _, t := range tradeOffs {
		lines = append(lines, fmt.Sprintf("- %s", t))
	}
	return strings.Join(lines, "\n")
}

func formatAssumptions(assumptions []domain.Assumption) string {
	if len(assumptions) == 0 {
		return "- (none documented)"
	}
	var lines []string
	for _, a := range assumptions {
		lines = append(lines, fmt.Sprintf("- %s (confidence: %.1f)", a.Assumption, a.Confidence))
	}
	return strings.Join(lines, "\n")
}

func formatRisks(risks []domain.Risk) string {
	if len(risks) == 0 {
		return "- (none documented)"
	}
	var lines []string
	for _, r := range risks {
		mitigation := "TBD"
		if r.Mitigation != nil && *r.Mitigation != "" {
			mitigation = *r.Mitigation
		}
		lines = append(lines, fmt.Sprintf("- Risk: %s\n  Mitigation: %s", r.Risk, mitigation))
	}
	return strings.Join(lines, "\n")
}

func formatEvidence(evidence []domain.Evidence) string {
	if len(evidence) == 0 {
		return "(no evidence recorded)"
	}
	var lines []string
	for i, e := range evidence {
		sourceType := string(e.Source.Type)
		sourceURL := "(no url)"
		if e.Source.URL != nil && *e.Source.URL != "" {
			sourceURL = *e.Source.URL
		}

		lines = append(lines, fmt.Sprintf("%d) Claim: %s", i+1, e.Claim))
		lines = append(lines, fmt.Sprintf("   Quote: \"%s\"", e.Quote))
		lines = append(lines, fmt.Sprintf("   Source: %s %s", sourceType, sourceURL))
		if e.Source.Pointer != nil && *e.Source.Pointer != "" {
			lines = append(lines, fmt.Sprintf("   Pointer: %s", *e.Source.Pointer))
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func formatLinks(links []map[string]any) string {
	if len(links) == 0 {
		return "- (none)"
	}
	var lines []string
	for _, link := range links {
		rel := "link"
		if r, ok := link["rel"].(string); ok {
			rel = r
		}
		url := ""
		if u, ok := link["url"].(string); ok {
			url = u
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", rel, url))
	}
	return strings.Join(lines, "\n")
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "(none)"
	}
	return strings.Join(tags, ", ")
}

func RenderPayloadText(r *domain.DecisionRecord) string {
	domainStr := string(r.Domain)
	sensitivity := string(r.Sensitivity)
	status := string(r.Status)
	certainty := string(r.Why.Certainty)

	alternatives := formatAlternatives(r.Context.Alternatives, r.Context.Chosen)
	tradeOffs := formatTradeOffs(r.Context.TradeOffs)
	assumptions := formatAssumptions(r.Context.Assumptions)
	risks := formatRisks(r.Context.Risks)
	evidenceBlock := formatEvidence(r.Evidence)
	links := formatLinks(r.Links)
	tags := formatTags(r.Tags)

	rationale := r.Why.RationaleSummary
	if rationale == "" {
		rationale = "(no rationale documented)"
	}
	if len(r.Why.MissingInfo) > 0 {
		var missingLines []string
		for _, m := range r.Why.MissingInfo {
			missingLines = append(missingLines, fmt.Sprintf("- %s", m))
		}
		rationale += "\n\nMissing Information:\n" + strings.Join(missingLines, "\n")
	}

	when := r.Decision.When
	if when == "" {
		when = "(unknown)"
	}
	where := r.Decision.Where
	if where == "" {
		where = "(unknown)"
	}
	problem := r.Context.Problem
	if problem == "" {
		problem = "(not documented)"
	}

	text := fmt.Sprintf(payloadTemplate,
		r.Title,
		r.ID,
		status, sensitivity, domainStr,
		when, where,
		r.Decision.What,
		problem,
		alternatives,
		rationale,
		certainty,
		tradeOffs,
		assumptions,
		risks,
		evidenceBlock,
		links,
		tags,
	)

	// Post insertion
	if r.GroupID != nil && *r.GroupID != "" {
		seq := 0
		if r.PhaseSeq != nil {
			seq = *r.PhaseSeq
		}
		seq++

		totalStr := "?"
		if r.PhaseTotal != nil {
			totalStr = fmt.Sprintf("%d", *r.PhaseTotal)
		}
		gtype := "phase_chain"
		if r.GroupType != nil && *r.GroupType != "" {
			gtype = *r.GroupType
		}
		phaseLine := fmt.Sprintf("Part: %d of %s | Type: %s | Group: %s", seq, totalStr, gtype, *r.GroupID)

		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "ID: ") {
				insertPos := i + 1
				// Insert phase line after ID line
				newLines := make([]string, 0, len(lines)+2)
				newLines = append(newLines, lines[:insertPos]...)
				newLines = append(newLines, phaseLine)
				// Insert group summary if present
				if r.GroupSummary != nil && *r.GroupSummary != "" {
					newLines = append(newLines, fmt.Sprintf("Group Summary: %s", *r.GroupSummary))
				}
				newLines = append(newLines, lines[insertPos:]...)
				lines = newLines
				break
			}
		}
		text = strings.Join(lines, "\n")
	}

	// Clean up blank lines
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(text)
}
