package policy

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/envector/rune-go/internal/domain"
)

// Record builder — Python canonical: agents/scribe/record_builder.py (703 LoC).
// D13 Option A: Go ports all logic (not delegated to agent).
// D14: pre_extraction required (no LLM fallback).
// Spec: docs/v04/spec/flows/capture.md Phase 5 + canonical-reference section.

// MAX_INPUT_CHARS — Python L227. Truncate cleanText before extraction.
const MaxInputChars = 12_000

// QuotePatterns — 4 regex (Python L72-77): double "", single ”, Japanese 「」,
// French «». Min 10 chars.
var QuotePatterns = []*regexp.Regexp{
	regexp.MustCompile(`"([^"]{10,})"`),
	regexp.MustCompile(`'([^']{10,})'`),
	regexp.MustCompile(`「([^」]{10,})」`),
	regexp.MustCompile(`«([^»]{10,})»`),
}

// RationalePatterns — 5 regex (Python L80-86): because / reason / rationale /
// since / due to.
var RationalePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)because\s+(.{10,}?)(?:\.|$)`),
	regexp.MustCompile(`(?i)reason(?:ing)?(?:\s+is)?[:\s]+(.{10,}?)(?:\.|$)`),
	regexp.MustCompile(`(?i)rationale[:\s]+(.{10,}?)(?:\.|$)`),
	regexp.MustCompile(`(?i)since\s+(.{10,}?)(?:\.|$)`),
	regexp.MustCompile(`(?i)due to\s+(.{10,}?)(?:\.|$)`),
}

var acceptancePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:approved|accepted|confirmed|finalized|agreed|decided)\b`),
	regexp.MustCompile(`(?i)\b(?:final decision|it's decided|we're going with)\b`),
}

// BuildPhases — Python: record_builder.py build_phases(rawEvent, detection, pre_extraction).
//
// Agent-delegated mode (D14) requires pre_extraction != nil; otherwise returns
// ErrExtractionMissing.
//
// Order-critical (Python L196-199, L310-311, L395-396):
//  1. Redact PII from rawEvent.Text → cleanText (ALWAYS, even in agent-delegated)
//  2. Assemble record(s) with payload.text = ""
//  3. EnsureEvidenceCertaintyConsistency per record (§7.1)
//  4. Render payload.text = RenderPayloadText(record)
//  5. Set reusable_insight = pre_extraction.group_summary (if present)
//
// Returns 1-7 records (single / phase_chain / bundle per ExtractionResult.GroupType).
func BuildPhases(
	rawEvent *domain.RawEvent,
	detection *domain.Detection,
	preExtraction *domain.ExtractionResult,
	now time.Time,
) ([]domain.DecisionRecord, error) {
	if preExtraction == nil {
		return nil, domain.ErrExtractionMissing
	}

	// PII redaction
	cleanText, redactionNotes := RedactSensitive(rawEvent.Text)

	// Dispatch
	if !preExtraction.IsMultiPhase() {
		fields := preExtraction.Single
		if fields == nil {
			return nil, &domain.RuneError{
				Code:    domain.CodeInvalidInput,
				Message: "extraction has no single fields and no phases",
			}
		}
		rec := buildSingleRecord(fields, rawEvent, cleanText, detection, preExtraction, redactionNotes, now)
		return []domain.DecisionRecord{rec}, nil
	}

	records := buildMultiRecord(preExtraction, rawEvent, cleanText, detection, redactionNotes, now)
	return records, nil
}

func buildSingleRecord(
	fields *domain.ExtractedFields,
	rawEvent *domain.RawEvent,
	cleanText string,
	detection *domain.Detection,
	extraction *domain.ExtractionResult,
	redactionNotes string,
	now time.Time,
) domain.DecisionRecord {
	title := fields.Title
	if title == "" {
		title = extractTitle(cleanText)
	}

	evidence := extractEvidence(rawEvent, cleanText)
	certainty, missingInfo := determineCertainty(evidence, fields.Rationale)
	status := statusFromHint(fields.StatusHint, evidence, cleanText)
	d := domain.ParseDomain(detection.Domain)
	recordID := domain.GenerateRecordID(now, d, title)

	confidence := detection.Confidence
	if extraction.Confidence != nil {
		confidence = *extraction.Confidence
	}

	var reviewNotes *string
	if redactionNotes != "" {
		reviewNotes = &redactionNotes
	}

	alts := fields.Alternatives
	if len(alts) > 5 {
		alts = alts[:5]
	}
	tos := fields.TradeOffs
	if len(tos) > 5 {
		tos = tos[:5]
	}

	record := domain.DecisionRecord{
		SchemaVersion: "2.1", // XXX: should we reset this?
		ID:            recordID,
		Type:          "decision_record",
		Domain:        d,
		Sensitivity:   domain.SensitivityInternal,
		Status:        status,
		Timestamp:     now.UTC(),
		Title:         title,
		Decision: domain.DecisionDetail{
			What:  truncRunes(cleanText, 500),
			Who:   userWho(rawEvent),
			Where: whereStr(rawEvent),
		},
		Context: domain.Context{
			Problem:      fields.Problem,
			Alternatives: alts,
			TradeOffs:    tos,
		},
		Why: domain.Why{
			RationaleSummary: fields.Rationale,
			Certainty:        certainty,
			MissingInfo:      missingInfo,
		},
		Evidence:     evidence,
		Tags:         fields.Tags,
		OriginalText: &rawEvent.Text,
		Quality: domain.Quality{
			ScribeConfidence: confidence,
			ReviewState:      domain.ReviewStateUnreviewed,
			ReviewNotes:      reviewNotes,
		},
		Payload: domain.Payload{Format: "markdown", Text: ""},
	}

	domain.EnsureEvidenceCertaintyConsistency(&record)
	record.Payload.Text = RenderPayloadText(&record)

	if extraction.GroupSummary != "" {
		record.ReusableInsight = extraction.GroupSummary
	}

	return record
}

func buildMultiRecord(
	extraction *domain.ExtractionResult,
	rawEvent *domain.RawEvent,
	cleanText string,
	detection *domain.Detection,
	redactionNotes string,
	now time.Time,
) []domain.DecisionRecord {
	phases := extraction.Phases
	d := domain.ParseDomain(detection.Domain)
	groupTitle := extraction.GroupTitle
	if groupTitle == "" {
		groupTitle = extractTitle(cleanText)
	}
	groupID := domain.GenerateGroupID(now, d, groupTitle)
	groupType := extraction.GroupType
	if groupType == "" {
		groupType = "phase_chain"
	}
	phaseTotal := len(phases)

	confidence := detection.Confidence
	if extraction.Confidence != nil {
		confidence = *extraction.Confidence
	}

	var reviewNotes *string
	if redactionNotes != "" {
		reviewNotes = &redactionNotes
	}

	var records []domain.DecisionRecord
	for seq, phase := range phases {
		phaseTitle := phase.PhaseTitle
		if phaseTitle == "" {
			phaseTitle = fmt.Sprintf("Phase %d", seq+1)
		}
		suffix := fmt.Sprintf("_p%d", seq)
		if groupType == "bundle" {
			suffix = fmt.Sprintf("_b%d", seq)
		}
		recordID := domain.GenerateRecordID(now, d, phaseTitle) + suffix

		decision := truncRunes(phase.PhaseDecision, 500)

		evidence := extractEvidence(rawEvent, cleanText)
		certainty, missingInfo := determineCertainty(evidence, phase.PhaseRationale)
		status := statusFromHint(extraction.StatusHint, evidence, cleanText)

		alts := phase.Alternatives
		if len(alts) > 5 {
			alts = alts[:5]
		}
		tos := phase.TradeOffs
		if len(tos) > 5 {
			tos = tos[:5]
		}

		// Fallback phase.Tags to extractions.Tags
		tags := phase.Tags
		if len(tags) == 0 {
			tags = extraction.Tags
		}

		var groupSummary *string
		if extraction.GroupSummary != "" {
			groupSummary = &extraction.GroupSummary
		}

		record := domain.DecisionRecord{
			SchemaVersion: "2.1", // XXX: should we reset this?
			ID:            recordID,
			Type:          "decision_record",
			Domain:        d,
			Sensitivity:   domain.SensitivityInternal,
			Status:        status,
			Timestamp:     now.UTC(),
			Title:         phaseTitle,
			Decision: domain.DecisionDetail{
				What:  decision,
				Who:   userWho(rawEvent),
				Where: whereStr(rawEvent),
			},
			Context: domain.Context{
				Problem:      phase.PhaseProblem,
				Alternatives: alts,
				TradeOffs:    tos,
			},
			Why: domain.Why{
				RationaleSummary: phase.PhaseRationale,
				Certainty:        certainty,
				MissingInfo:      missingInfo,
			},
			Evidence:     evidence,
			Tags:         tags,
			OriginalText: &rawEvent.Text,
			GroupID:      &groupID,
			GroupType:    &groupType,
			PhaseSeq:     &seq,
			PhaseTotal:   &phaseTotal,
			GroupSummary: groupSummary,
			Quality: domain.Quality{
				ScribeConfidence: confidence,
				ReviewState:      domain.ReviewStateUnreviewed,
				ReviewNotes:      reviewNotes,
			},
			Payload: domain.Payload{Format: "markdown", Text: ""},
		}

		domain.EnsureEvidenceCertaintyConsistency(&record)
		record.Payload.Text = RenderPayloadText(&record)

		if extraction.GroupSummary != "" {
			record.ReusableInsight = extraction.GroupSummary
		}

		records = append(records, record)
	}

	return records
}

//--- Helper ---//

// extractTitle returns the first sentence (up to first ".") capped at
// MaxTitleLen runes. text[:idx] is safe because "." is ASCII single-byte.
func extractTitle(text string) string {
	firstSentence := text
	if idx := strings.Index(text, "."); idx > 0 {
		firstSentence = text[:idx]
	}
	firstSentence = truncRunes(firstSentence, domain.MaxTitleLen)
	firstSentence = strings.TrimSpace(firstSentence)
	if len(firstSentence) > 10 {
		return firstSentence
	}
	return "General decision"
}

func extractEvidence(rawEvent *domain.RawEvent, text string) []domain.Evidence {
	var evidence []domain.Evidence
	sourceRef := makeSourceRef(rawEvent)

	for _, rx := range QuotePatterns {
		matches := rx.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) > 1 && len(m[1]) >= 10 {
				evidence = append(evidence, domain.Evidence{
					Claim:  "Quoted statement from discussion",
					Quote:  truncRunes(m[1], 200),
					Source: sourceRef,
				})
			}
		}
	}

	// Fallback paraphrase
	if len(evidence) == 0 && len(text) >= 20 {
		quote := text
		if truncated := truncRunes(quote, 150); truncated != quote {
			quote = truncated + "..."
		}
		evidence = append(evidence, domain.Evidence{
			Claim:  "Decision statement (paraphrased)",
			Quote:  quote,
			Source: sourceRef,
		})
	}

	if len(evidence) > 3 {
		evidence = evidence[:3]
	}
	return evidence
}

func determineCertainty(evidence []domain.Evidence, rationale string) (domain.Certainty, []string) {
	var missingInfo []string

	if len(evidence) == 0 {
		missingInfo = append(missingInfo, "No evidence found")
		return domain.CertaintyUnknown, missingInfo
	}

	hasDirectQuotes := false
	for _, e := range evidence {
		if !strings.Contains(strings.ToLower(e.Claim), "paraphrase") {
			hasDirectQuotes = true
			break
		}
	}

	if !hasDirectQuotes {
		missingInfo = append(missingInfo, "No direct quotes - evidence is paraphrased")
		return domain.CertaintyPartiallySupported, missingInfo
	}

	if rationale == "" {
		missingInfo = append(missingInfo, "Explicit rationale not found")
		return domain.CertaintyPartiallySupported, missingInfo
	}

	return domain.CertaintySupported, missingInfo
}

func statusFromHint(hint string, evidence []domain.Evidence, text string) domain.Status {
	hintLower := strings.TrimSpace(strings.ToLower(hint))
	switch hintLower {
	case "accepted":
		return domain.StatusAccepted
	case "rejected":
		return domain.StatusProposed // Rejected: proposed, not superseded
	case "proposed":
		return domain.StatusProposed
	}

	return determineStatus(evidence, text)
}

func determineStatus(evidence []domain.Evidence, text string) domain.Status {
	if len(evidence) == 0 {
		return domain.StatusProposed
	}
	textLower := strings.ToLower(text)
	for _, rx := range acceptancePatterns {
		if rx.MatchString(textLower) {
			return domain.StatusAccepted
		}
	}
	return domain.StatusProposed
}

// RawEvent -> SourceRef
func makeSourceRef(rawEvent *domain.RawEvent) domain.SourceRef {
	var pointer *string
	if rawEvent.Channel != "" {
		p := "channel:" + rawEvent.Channel
		pointer = &p
	}
	return domain.SourceRef{
		Type:    parseSourceType(rawEvent.Source),
		Pointer: pointer,
	}
}

func parseSourceType(source string) domain.SourceType {
	s := strings.ToLower(source)
	switch {
	case strings.Contains(s, "slack"):
		return domain.SourceTypeSlack
	case strings.Contains(s, "github"):
		return domain.SourceTypeGitHub
	case strings.Contains(s, "notion"):
		return domain.SourceTypeNotion
	case strings.Contains(s, "meeting"):
		return domain.SourceTypeMeeting
	case strings.Contains(s, "email"):
		return domain.SourceTypeEmail
	case strings.Contains(s, "doc"):
		return domain.SourceTypeDoc
	}
	return domain.SourceTypeOther
}

func userWho(rawEvent *domain.RawEvent) []string {
	if rawEvent.User != "" {
		return []string{fmt.Sprintf("user:%s", rawEvent.User)}
	}
	return nil
}

func whereStr(rawEvent *domain.RawEvent) string {
	if rawEvent.Channel != "" {
		return fmt.Sprintf("%s:%s", rawEvent.Source, rawEvent.Channel)
	}
	return rawEvent.Source
}

