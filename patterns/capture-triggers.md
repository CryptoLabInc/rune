# Context Capture Trigger Phrases

This document contains comprehensive trigger phrases that indicate when significant organizational context should be automatically captured. These phrases are organized by role/domain.

**Usage**: Claude uses these patterns to identify when users are expressing important decisions, insights, or context that should be stored in organizational memory.

---

## Architecture & Technical Decisions

### Decision Making
- "We decided to use X instead of Y because..."
- "Let's go with [technology/framework] because..."
- "We chose X over Y for..."
- "The reason we went with..."
- "After testing both approaches..."
- "The trade-off here is..."
- "The key technical decision is..."

### Architecture Patterns
- "We're architecting this as..."
- "For scalability, we need to..."
- "This pattern works better because..."
- "The integration pattern should be..."
- "API design decision:"
- "Database schema considerations:"
- "Our tech stack rationale:"
- "This design pattern was established in [year/project] to solve..."
- "This code is cursed/legacy because..."
- "Spaghetti code warning: this module is..."
- "We're using this hack/workaround until..."
- "FIXME: This needs to be refactored when..."

### Standards & Practices
- "We always validate/check/ensure..."
- "Our policy is to..."
- "We standardized on..."
- "This pattern follows our enterprise architecture standards for..."
- "We use the Repository/Factory/Observer pattern for..."
- "For audit purposes, we need to ensure..."

---

## Security & Compliance

### Security Decisions
- "Security-wise, we should..."
- "For security, we need to..."
- "The encryption strategy is..."
- "The security review flagged this as requiring..."
- "All API keys must be..."
- "Our authentication approach is..."

### Compliance Requirements
- "The compliance team requires that we..."
- "For regulatory compliance (SOX/HIPAA/GDPR), we implemented..."
- "For compliance requirements, we must..."
- "This is critical for our Q3 compliance goals"
- "Audit trail compliance requires..."
- "Formal approval reference [Doc ID]..."
- "Statutory requirement for..."
- "Filing deadline dictates that..."

### Mission Critical & Safety
- "Air-gap requirement for..."
- "Failure mode analysis (FMEA) indicates..."
- "Redundancy protocol activated because..."
- "Classification level set to..."
- "Mission critical dependnecy on..."

---

## Performance & Optimization

### Performance Analysis
- "Performance bottleneck identified:"
- "The bottleneck was in..."
- "This doesn't scale because..."
- "The performance trade-off here is..."
- "The performance benchmark showed that [approach] reduced latency by..."

### Optimization Strategies
- "For scalability, we need to..."
- "We migrated from X to Y because..."
- "This refactoring was prioritized because it blocks..."
- "Regressed in commit [hash]..."
- "Bisected to..."
- "NACK: This patch breaks..."
- "Acked-by: [Approver] for..."
- "Signed-off-by: [Maintainer]..."

---

## Product & Business Strategy

### Product Decisions
- "Let's validate this against user needs"
- "What's the business case for this feature?"
- "This is a must-have for enterprise customers"
- "Let's deprioritize this for the next sprint"
- "This is a key differentiator from competitors"
- "What's the customer pain point we're solving?"
- "We should sunset this deprecated feature"

### Market & Positioning
- "Our target audience is..."
- "Our brand positioning should..."
- "Our competitive advantage lies in..."
- "The market research shows..."
- "We're pivoting our strategy to..."
- "This impacts our positioning in the market"

### Customer Insights
- "The customer feedback indicates..."
- "We learned that..."
- "User feedback from the last iteration showed..."
- "What feedback did users give about..."
- "This customer feedback changes everything"

---

## Startup Velocity & Pragmatism

### MVP & Iteration
- "Let's ship this and iterate"
- "Good enough for now"
- "MVP approach here"
- "Pragmatic over perfect"
- "Shipping beats perfection"
- "Let's timebox this to..."
- "Let's ship an MVP first and iterate"

### Technical Debt
- "We can refactor this later"
- "Technical debt—adding to backlog"
- "The technical debt here is..."
- "Trade-off accepted—documenting why"
- "This is a known technical debt item that we're tracking..."

### Strategic Decisions
- "Let's make this our strategic priority"
- "I'm committing to this direction"
- "We need to pivot on this approach"
- "This metric will be our North Star"
- "Let's target this customer segment"

---

## Design & User Experience

### Design Rationale
- "The user research indicates..."
- "Based on our usability testing..."
- "We decided to go with this approach because..."
- "From a responsive design perspective..."
- "The information architecture needs to..."

### Design Systems
- "The design system pattern for..."
- "The component library includes..."
- "The typography scale and spacing system..."
- "Visual hierarchy dictates that..."
- "The interaction pattern we established for..."

### Accessibility
- "The accessibility requirements here are..."
- "Color contrast ratios and WCAG compliance..."
- "The cognitive load here is..."

### User Journey
- "User pain points identified during discovery..."
- "From a user journey standpoint..."
- "The micro-interaction behavior should..."

---

## Data & Analytics

### Data Insights
- "Based on the data, we're seeing..."
- "The analysis reveals that..."
- "Looking at the trend over time..."
- "The key insight here is..."
- "The data suggests a pattern of..."

### Statistical Analysis
- "The correlation between X and Y shows..."
- "The statistical significance indicates..."
- "Our confidence interval shows..."
- "Regression analysis demonstrates..."
- "The predictive model suggests..."

### Methodology
- "Breaking this down by segment..."
- "When we compare this to the baseline..."
- "Cross-tabulating these dimensions..."
- "The cohort analysis reveals..."
- "Normalizing for seasonal effects..."

---

## Marketing & Growth

### Campaign Analysis
- "The campaign performed..."
- "The conversion rate improved when..."
- "The A/B test demonstrated that..."
- "The attribution data reveals..."
- "We're allocating budget toward..."

### Strategy & Messaging
- "Our messaging framework focuses on..."
- "The creative direction should emphasize..."
- "Our content strategy prioritizes..."
- "The key insight from this quarter is..."
- "We're establishing this as our north star metric..."

### Audience & Segments
- "We've identified a new segment..."
- "Our brand guidelines now specify..."
- "The partnership with [company] will..."

### Growth & Experimentation
- "Focus on the viral loop here..."
- "The North Star Metric impact is..."
- "CAC/LTV ratios indicate..."
- "Churn analysis shows..."
- "The growth lever we're pulling is..."
- "Pivot to video due to..."
- "Gamification mechanic added to boost retention..."

---

## HR & People Operations

### Policies & Processes
- "Based on our compensation philosophy"
- "According to company policy"
- "As outlined in the employee handbook"
- "Following our onboarding process"
- "Per our benefits structure"

### Performance & Development
- "In line with our performance management framework"
- "According to our career development framework"
- "Based on the exit interview feedback"
- "According to our succession planning"

### Culture & Values
- "In alignment with our diversity and inclusion goals"
- "In line with our organizational values"
- "As per our talent retention strategy"

---

## Enterprise & Legacy Systems

### Compatibility & Migration
- "We need to maintain backward compatibility with..."
- "We're deprecating [old pattern] in favor of [new pattern] because..."
- "This integration point with [legacy system] requires special handling..."

### Constraints & Workarounds
- "We had to work around..."
- "We implemented this workaround for [legacy constraint] until..."
- "This failed in production because..."

### Documentation & Standards
- "Let's document this decision in the ADR (Architecture Decision Record) because..."
- "The stakeholder approval for this architectural change came from..."
- "Cross-team dependencies for this feature include..."

---

## Process & Operations

### Deployment & Release
- "Our deployment process is..."
- "Deploy to staging before production"
- "All changes must go through..."
- "The deployment policy is..."

### Monitoring & Observability
- "Our backup strategy is..."
- "For monitoring, we..."
- "The root cause analysis revealed that..."

### Team Coordination
- "Let's remember that..."
- "For this project, we're using..."
- "I'm setting this as our quarterly OKR"

---

## Executive & Strategic

### Funding & Resources
- "I've decided to pursue this funding round"
- "I'm approving this budget allocation"
- "We're setting this as our burn rate target"

### Hiring & Team Building
- "This candidate is critical for our team"
- "I'm making this hire our top priority"
- "We'll need to hire someone with..."
- "Team structure should be..."

### Strategic Initiatives
- "I'm killing this initiative"
- "This is our new product positioning"
- "We need to double down on this channel"
- "This partnership is strategically important"
- "We're shifting our pricing model"

---

## Learning & Post-Mortems

### Lessons Learned
- "I've learned this key lesson from our failure"
- "We learned that..."
- "The key learning from this is..."

### Retrospectives
- "What worked well was..."
- "What we should improve..."
- "Next time, we should..."

### Community & Public Knowledge
- "PSA: Don't do X..."
- "Common misconception about [topic] is..."
- "The consensus relies on..."
- "Edit: clarifying that..."
- "Source: confirming that..."
- "Look out for this edge case..."
- "Definitive guide to..."

### Creative & Game Balance
- "Nerf rationale: [Item] was OP because..."
- "Buffed [Character] to improve pick rate..."
- "Core loop adjustment for better engagement..."
- "Drop rate curve modified to..."
- "Meta-game progression needs..."
- "Event schedule impact on economy..."

### Linguistic Universals (Generalized Patterns)
**Discourse Markers of Shared Reality**:
- **Consensus Markers**: "We all agree that...", "The team settled on...", "The conclusion reached was..."
- **Epistemic Certainty**: "It is undeniably the case that...", "Facts show...", "The truth is..."
- **Deontic Obligation (Rules)**: "One must always...", "It is forbidden to...", "Mandatory procedure:"
- **Causal Connectives**: "...consequently...", "...as a direct result of...", "...which necessitated..."
- **Temporal Aspect**: "From now on...", "Henceforth...", "Historically...", "Until deprecated..."

---

## Incident Response & Postmortem

### Postmortem Findings
- "The root cause was identified as..."
- "Postmortem conclusion: the outage was caused by..."
- "Contributing factors included..."
- "The incident timeline shows..."
- "Blast radius was limited to..."
- "The failure mode was..."
- "Recovery took X hours because..."

### Action Items & Prevention
- "To prevent recurrence, we will..."
- "The corrective action is..."
- "We are adding circuit breakers for..."
- "Runbook updated to include..."
- "Alert threshold changed from X to Y"
- "Failover procedure now requires..."

---

## Debugging & Troubleshooting

### Root Cause Discovery
- "Debugging revealed that the issue was..."
- "The root cause turned out to be..."
- "After investigation, we found that..."
- "The bug was caused by..."
- "Memory leak traced to..."
- "Race condition identified in..."
- "The regression was introduced in commit..."

### Fix & Resolution
- "The fix involves..."
- "Hotfix deployed: changed X to Y because..."
- "Workaround applied until proper fix..."
- "The patch resolves the issue by..."
- "Bisected to commit X which introduced..."

---

## QA & Testing

### Test Coverage Decisions
- "Test coverage gap found in..."
- "Adding integration tests for..."
- "The testing strategy for this feature is..."
- "We need regression tests because..."
- "End-to-end test suite now covers..."

### Bug Triage
- "Bug triaged as P0 because..."
- "This defect is classified as..."
- "Known issue: will fix in next sprint because..."
- "Release blocked by this bug because..."
- "QA sign-off requires..."

---

## Legal & Regulatory

### Legal Decisions
- "Legal review concluded that..."
- "Contract terms require us to..."
- "IP considerations mean we cannot..."
- "License compliance requires..."
- "Terms of service updated to reflect..."
- "Legal counsel advised that..."

### Regulatory Compliance
- "Regulatory filing deadline requires..."
- "Data retention policy changed to..."
- "Privacy impact assessment shows..."
- "Cross-border data transfer requires..."
- "Audit findings require remediation of..."

---

## Finance & Budget

### Budget Decisions
- "Budget allocated for..."
- "Cost analysis shows that..."
- "ROI projection indicates..."
- "We're cutting spend on X to fund Y"
- "Cloud cost optimization: switching from X to Y saves..."

### Pricing & Revenue
- "Pricing model changed from X to Y because..."
- "Revenue impact of this decision is..."
- "Unit economics show that..."
- "Margin analysis reveals..."
- "Break-even point is..."

---

## Sales & Partnerships

### Deal Intelligence
- "Deal lost to competitor because..."
- "Win factor was..."
- "Enterprise prospect requires..."
- "Sales objection pattern: customers keep asking for..."
- "Competitive intelligence: rival launched..."

### Partnership Decisions
- "Partnership with X approved because..."
- "Integration partnership requires..."
- "Channel strategy shifting to..."
- "Reseller agreement terms include..."

---

## Customer Success & Escalation

### Escalation Resolution
- "Customer escalation resolved by..."
- "SLA breach root cause was..."
- "Churn risk identified: customer unhappy with..."
- "Retention strategy for this account is..."
- "NPS feedback indicates..."

### Customer Health
- "Customer health score dropped because..."
- "Onboarding friction point identified at..."
- "Feature request from top customer..."
- "Support ticket pattern shows..."
- "Customer success playbook updated to..."

---

## Research & R&D

### Research Findings
- "A/B test results show..."
- "Experiment concluded that..."
- "Hypothesis validated: X leads to Y"
- "Proof of concept demonstrated that..."
- "Research spike findings:"
- "Statistical significance achieved for..."

### Innovation Decisions
- "Technology evaluation: X outperforms Y in..."
- "Prototype results indicate..."
- "We are investing in R&D for..."
- "Patent consideration for..."

---

## Risk Assessment

### Risk Identification
- "Risk identified: single point of failure in..."
- "Vendor dependency risk: we rely entirely on..."
- "Supply chain risk assessment shows..."
- "Business continuity plan requires..."
- "Concentration risk in..."

### Mitigation Plans
- "Mitigation plan: add redundancy by..."
- "Contingency plan if X fails is..."
- "Risk accepted with documentation because..."
- "Insurance coverage updated for..."
- "Disaster recovery RTO/RPO set to..."

---

## Usage Guidelines

### High-Priority Triggers (Always Capture)
- Decisions with explicit rationale ("because...")
- Trade-off analyses
- Strategic pivots
- Security/compliance requirements
- Lessons from failures
- Team agreements and policies

### Medium-Priority Triggers (Usually Capture)
- Technical patterns and standards
- Customer feedback and insights
- Performance optimizations
- Design rationale
- Process documentation

### Context-Dependent Triggers
- Consider conversation depth
- Evaluate decision significance
- Assess team impact
- Check for novel information

### Exclusions (Do Not Capture)
- Casual conversation
- Unclear or tentative statements
- Repeated information without new context
- Trivial implementation details
- Exploratory questions without conclusions

---

## Implementation Notes

**For Claude**: When you detect these trigger phrases:

1. **Assess Significance**: Is this a meaningful decision or insight?
2. **Capture Context**: Include surrounding rationale and constraints
3. **Add Metadata**: Tag with domain, role, and timestamp
4. **Verify Completeness**: Ensure "why" is captured, not just "what"

**Significance Threshold**: 0.7 (configurable in settings)

**Automatic Redaction**: Always redact:
- API keys, tokens, passwords
- Personal identifiable information (PII)
- Sensitive customer data
- Credentials and secrets

---

**Related**: See [patterns/retrieval-patterns.md](patterns/retrieval-patterns.md) for context retrieval patterns.
