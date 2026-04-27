package policy

import (
	"regexp"
	"strings"

	"github.com/envector/rune-go/internal/domain"
)

// Query parsing — Python: agents/retriever/query_processor.py (437 LoC).
// Flow: docs/v04/spec/flows/recall.md Phase 2.

type IntentRule struct {
	Intent   domain.QueryIntent
	Patterns []string
}

var IntentRules = []IntentRule{
	{domain.QueryIntentDecisionRationale, []string{
		`why did we (choose|decide|go with|select|pick)`,
		`what was the (reasoning|rationale|logic|thinking)`,
		`why .+ over .+`,
		`what were the (reasons|factors)`,
		`why (not|didn't we)`,
		`reasoning behind`,
	}},
	{domain.QueryIntentFeatureHistory, []string{
		`(have|did) (customers?|users?) (asked|requested|wanted)`,
		`feature request`,
		`why did we (reject|say no|decline)`,
		`(how many|which) customers`,
		`customer feedback (on|about)`,
	}},
	{domain.QueryIntentPatternLookup, []string{
		`how do we (handle|deal with|approach|manage)`,
		`what'?s our (approach|process|standard|convention)`,
		`is there (an?|existing) (pattern|standard|convention)`,
		`what'?s the (best practice|recommended way)`,
		`how should (we|I)`,
	}},
	{domain.QueryIntentTechnicalContext, []string{
		`what'?s our (architecture|design|system) for`,
		`how (does|is) .+ (implemented|built|designed)`,
		`(explain|describe) (the|our) .+ (system|architecture|design)`,
		`technical (details|overview) (of|for)`,
	}},
	{domain.QueryIntentSecurityCompliance, []string{
		`(security|compliance) (requirements?|considerations?)`,
		`what (security|privacy) (measures|controls)`,
		`(gdpr|hipaa|sox|pci) (requirements?|compliance)`,
		`audit (requirements?|trail)`,
	}},
	{domain.QueryIntentHistoricalContext, []string{
		`when did we (decide|choose|implement|launch)`,
		`(history|timeline) of`,
		`(have|did) we (ever|previously)`,
		`how long (have|has) .+ been`,
	}},
	{domain.QueryIntentAttribution, []string{
		`who (decided|chose|approved|owns)`,
		`which (team|person|group) (is responsible|decided|owns)`,
		`(owner|maintainer) of`,
	}},
}

type TimeRule struct {
	Scope    domain.TimeScope
	Patterns []string
}

var TimeRules = []TimeRule{
	{domain.TimeScopeLastWeek, []string{`last week`, `this week`, `past week`, `7 days`}},
	{domain.TimeScopeLastMonth, []string{`last month`, `this month`, `past month`, `30 days`}},
	{domain.TimeScopeLastQuarter, []string{`last quarter`, `this quarter`, `Q[1-4]`, `past 3 months`}},
	{domain.TimeScopeLastYear, []string{`last year`, `this year`, `20\d{2}`, `past year`}},
}

var StopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {}, "will": {}, "would": {}, "could": {},
	"should": {}, "may": {}, "might": {}, "must": {}, "shall": {}, "can": {}, "need": {}, "dare": {},
	"ought": {}, "used": {}, "to": {}, "of": {}, "in": {}, "for": {}, "on": {}, "with": {}, "at": {}, "by": {},
	"from": {}, "up": {}, "about": {}, "into": {}, "over": {}, "after": {}, "we": {}, "our": {}, "us": {},
	"i": {}, "me": {}, "my": {}, "you": {}, "your": {}, "it": {}, "its": {}, "they": {}, "them": {}, "their": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "what": {}, "which": {}, "who": {}, "whom": {},
	"when": {}, "where": {}, "why": {}, "how": {}, "and": {}, "or": {}, "but": {}, "if": {}, "because": {},
	"as": {}, "until": {}, "while": {}, "although": {}, "though": {}, "even": {}, "just": {}, "also": {},
}

var TechPatterns = []string{
	`\b(PostgreSQL|MySQL|MongoDB|Redis|Elasticsearch|Kafka)\b`,
	`\b(React|Vue|Angular|Next\.js|Node\.js|Python|Java|Go)\b`,
	`\b(AWS|GCP|Azure|Kubernetes|Docker|Terraform)\b`,
	`\b(REST|GraphQL|gRPC|WebSocket|HTTP|HTTPS)\b`,
}

// Pre-compiled regexes
var (
	wsRe         = regexp.MustCompile(`\s+`)
	punctRe      = regexp.MustCompile(`[.!,;:]+$`)
	quotesRe     = regexp.MustCompile(`"([^"]+)"|'([^']+)'`)
	wordRe       = regexp.MustCompile(`\b\w+\b`)
	intentRegexs [][]*regexp.Regexp
	timeRegexs   [][]*regexp.Regexp
	techRegexs   []*regexp.Regexp
)

func init() {
	for _, rule := range IntentRules {
		var rx []*regexp.Regexp
		for _, p := range rule.Patterns {
			rx = append(rx, regexp.MustCompile("(?i)"+p))
		}
		intentRegexs = append(intentRegexs, rx)
	}
	for _, rule := range TimeRules {
		var rx []*regexp.Regexp
		for _, p := range rule.Patterns {
			rx = append(rx, regexp.MustCompile("(?i)"+p))
		}
		timeRegexs = append(timeRegexs, rx)
	}
	for _, p := range TechPatterns {
		techRegexs = append(techRegexs, regexp.MustCompile("(?i)"+p))
	}
}

func cleanQuery(q string) string {
	c := strings.TrimSpace(strings.ToLower(q))
	c = wsRe.ReplaceAllString(c, " ")
	c = punctRe.ReplaceAllString(c, "")
	return c
}

func detectIntent(qLower string) domain.QueryIntent {
	for i, rule := range IntentRules {
		for _, rx := range intentRegexs[i] {
			if rx.MatchString(qLower) {
				return rule.Intent
			}
		}
	}
	return domain.QueryIntentGeneral
}

func detectTimeScope(qLower string) domain.TimeScope {
	for i, rule := range TimeRules {
		for _, rx := range timeRegexs[i] {
			if rx.MatchString(qLower) {
				return rule.Scope
			}
		}
	}
	return domain.TimeScopeAllTime
}

func extractEntities(q string) []string {
	var entities []string
	addUnique := func(e string) {
		if len(e) <= 1 {
			return
		}
		for _, ex := range entities {
			if ex == e {
				return
			}
		}
		entities = append(entities, e)
	}

	matches := quotesRe.FindAllStringSubmatch(q, -1)
	for _, m := range matches {
		if m[1] != "" {
			addUnique(m[1])
		} else if m[2] != "" {
			addUnique(m[2])
		}
	}

	words := strings.Fields(q)
	for i := 1; i < len(words); i++ {
		if words[i] != "" && words[i][0] >= 'A' && words[i][0] <= 'Z' && len(words[i]) > 1 {
			phrase := []string{words[i]}
			j := i + 1
			for j < len(words) && len(words[j]) > 0 && words[j][0] >= 'A' && words[j][0] <= 'Z' {
				phrase = append(phrase, words[j])
				j++
			}
			addUnique(strings.Join(phrase, " "))
		}
	}

	for _, rx := range techRegexs {
		for _, m := range rx.FindAllString(q, -1) {
			addUnique(m)
		}
	}

	if len(entities) > 10 {
		entities = entities[:10]
	}
	return entities
}

func extractKeywords(qLower string) []string {
	words := wordRe.FindAllString(qLower, -1)
	var keywords []string
	seen := make(map[string]bool)
	for _, w := range words {
		if _, isStop := StopWords[w]; !isStop && len(w) > 2 {
			if !seen[w] {
				seen[w] = true
				keywords = append(keywords, w)
			}
		}
	}
	if len(keywords) > 15 {
		keywords = keywords[:15]
	}
	return keywords
}

func generateExpansions(qLower string, intent domain.QueryIntent, entities []string) []string {
	var exp []string
	exp = append(exp, qLower)

	switch intent {
	case domain.QueryIntentDecisionRationale:
		exp = append(exp, "decision "+qLower, "rationale "+qLower, "trade-off "+qLower)
	case domain.QueryIntentFeatureHistory:
		exp = append(exp, "customer request "+qLower, "feature rejected "+qLower)
	case domain.QueryIntentPatternLookup:
		exp = append(exp, "standard approach "+qLower, "best practice "+qLower)
	case domain.QueryIntentTechnicalContext:
		exp = append(exp, "architecture "+qLower, "implementation "+qLower)
	}

	for i := 0; i < len(entities) && i < 3; i++ {
		exp = append(exp, entities[i]+" decision", "why "+entities[i])
	}

	var unique []string
	seen := make(map[string]bool)
	for _, e := range exp {
		lower := strings.ToLower(e)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, e)
		}
	}
	if len(unique) > 5 {
		unique = unique[:5]
	}
	return unique
}

func Parse(q string) domain.ParsedQuery {
	cleaned := cleanQuery(q)
	intent := detectIntent(cleaned)
	timeScope := detectTimeScope(cleaned)
	entities := extractEntities(q)
	keywords := extractKeywords(cleaned)
	expanded := generateExpansions(cleaned, intent, entities)

	return domain.ParsedQuery{
		Original:        q,
		Cleaned:         cleaned,
		Intent:          intent,
		TimeScope:       timeScope,
		Entities:        entities,
		Keywords:        keywords,
		ExpandedQueries: expanded,
	}
}
