# Context Retrieval Patterns

This document contains comprehensive question patterns that indicate when users are searching for past organizational context. These patterns are organized by intent and domain.

**Usage**: Claude uses these patterns to recognize when to search organizational memory and retrieve relevant historical context.

---

## Decision Rationale Queries

### "Why" Questions
- "Why did we choose X over Y?"
- "Why did we decide to use [technology/framework]?"
- "What was the reasoning behind [decision]?"
- "What was the rationale for [approach]?"
- "What was the original rationale for implementing [feature/system]?"
- "Why did we pivot away from [approach]?"

### Trade-off Analysis
- "What were the trade-offs we discussed about [decision]?"
- "What trade-offs were evaluated before implementing [approach]?"
- "What alternatives did we consider?"
- "What were the alternative approaches we considered?"
- "What was the context when we made that call?"
- "What were the constraints we were working under?"

### Comparative Decisions
- "Why PostgreSQL over MySQL?"
- "What made us choose [X] instead of [Y]?"
- "How did we compare [option A] and [option B]?"

---

## Technical Architecture & Implementation

### Architecture Queries
- "What's our architecture for [system component]?"
- "How are we handling [technical challenge]?"
- "What was the rationale behind [API/integration pattern]?"
- "What design pattern was established for [use case]?"
- "Which architectural decision records (ADRs) are relevant to [technical area]?"

### Technology Stack
- "What database/framework/tool are we using and why?"
- "Why did we choose [specific technology]?"
- "What's our tech stack for [project/service]?"
- "What technology choices did we make for [component]?"

### Implementation Details
- "How did we implement [feature]?"
- "What patterns do we use for [common task]?"
- "What are the established patterns in our codebase for [auth/logging/caching]?"
- "How did we integrate X with Y?"
- "Is there existing code that does something similar?"
- "Who wrote this spaghetti code?"
- "Is this module deprecated or just legacy?"
- "Where are the sacred texts (docs) for this system?"
- "Is this a hack or a permanent fix?"

---

## Security & Compliance

### Security Requirements
- "What were the security considerations for [feature]?"
- "What security concerns were raised during the last architecture review?"
- "What was the outcome of the last security review for [component]?"
- "What's our encryption and key management strategy?"

### Compliance & Regulations
- "What compliance or regulatory constraints apply?"
- "What compliance requirements were documented for [feature]?"
- "For audit purposes, what did we implement?"
- "Where is the compliance audit trail for X?"
- "Did legal formalize approval for [Doc]?"
- "What is the classification level for this data?"
- "Show me the FMEA (Failure Mode) report."
- "Is this air-gap compliant?"
- "What regulatory requirements apply to [technical area]?"

---

## Performance & Scalability

### Performance Analysis
- "What performance requirements did we set?"
- "What were the performance considerations that led to [architectural decision]?"
- "What performance benchmarks did we set for query latency?"
- "How did we address performance bottlenecks in [component]?"

### Scalability Concerns
- "What scalability concerns came up?"
- "How does this scale?"
- "What are the known limitations in [component]?"

---

## Product & Business Context

### Feature Requirements
- "What were the original requirements for [feature]?"
- "What requirements did we define for [initiative]?"
- "What was the business case for [feature]?"
- "What's the customer pain point we're solving?"

### Prioritization & Roadmap
- "Which features were deprioritized from the initial roadmap?"
- "What did we decide to prioritize this quarter?"
- "Why did we move [feature] to next quarter?"
- "What were the blockers in our last release cycle?"

### Customer Feedback
- "What feedback did users give about [feature]?"
- "Which enterprise customers requested [capability]?"
- "What onboarding friction points did early adopters report?"
- "What were the customer objections to [approach]?"

---

## Marketing & Growth

### Campaign Performance
- "What was our ROI on the [campaign name] campaign?"
- "How did the [channel] campaign perform last year?"
- "What was the conversion rate for [campaign/channel]?"
- "Which creative assets had the highest engagement in [timeframe]?"

### Audience & Targeting
- "Who is our target demographic for [product/service]?"
- "What audience segments responded best to [campaign]?"
- "What were the key learnings from [initiative]?"

### Strategy & Positioning
- "What messaging did we use for the [quarter/year] launch?"
- "What was our positioning against [competitor]?"
- "What partnerships did we explore for [market/segment]?"

### Budget & Resources
- "What budget did we allocate to [channel/campaign]?"
- "Which vendor did we use for [service/tool]?"
- "What was the attribution model we used for [initiative]?"
- "What was the hypothesis for Experiment X?"
- "Show me the lift/impact of [feature] on retention."
- "Why is churn trending up in [segment]?"
- "What's the North Star impact of this initiative?"
- "Did we validate the viral loop hypothesis?"

---

## Data & Analytics

### Methodology & Baselines
- "What were the baseline metrics we established for this KPI?"
- "What was the methodology we used last time for this calculation?"
- "How did we handle missing data in the previous analysis?"
- "What statistical test did we apply to validate significance?"

### Data Quality & Sources
- "Which data sources did we validate and approve for this metric?"
- "What was the data quality score for this dataset?"
- "How did we address the outliers in the last quarterly report?"

### Analysis & Insights
- "What were the key assumptions in our previous forecast model?"
- "How did we define the success criteria for this experiment?"
- "Which segments showed the highest conversion rates previously?"
- "What were the identified confounding variables in our A/B test?"

---

## Design & User Experience

### Design Decisions
- "What design decisions did we make for [specific feature/component]?"
- "Why did we choose [specific design pattern] over alternatives?"
- "What iteration history exists for [component/feature]?"
- "What design principles or heuristics apply to [interaction]?"

### Game Design & Balance
- "Why was [Item/Character] nerfed?"
- "What's the drop rate curve for [Loot]?"
- "How does this event affect the economy balance?"
- "What was the player feedback on the new level difficulty?"
- "Is this mechanic considered an exploit?"

### User Research
- "What user feedback did we receive about [specific interaction]?"
- "What user research insights influenced [design decision]?"
- "What user personas or scenarios guided [feature design]?"
- "What usability issues came up during testing of [feature]?"

### Design System
- "What does our design system specify for [component type]?"
- "What design tokens or variables are used for [visual element]?"
- "What responsive breakpoints and behaviors did we define?"

### Accessibility & Requirements
- "What accessibility considerations were documented for [feature]?"
- "What cross-functional requirements affected [design choice]?"
- "What technical constraints influenced [design implementation]?"
- "Where is the latest comp/redline for this flow?"
- "Is this component in the design system token list?"
- "Did we test this prototype flow with users?"

---

## HR & People Operations

### Compensation & Benefits
- "What compensation range did we offer for similar roles last quarter?"
- "How did we structure the compensation package for their peer?"
- "What benefits changes did we communicate during open enrollment?"

### Performance & Development
- "What performance issues were documented in their last review cycle?"
- "What were the action items from the last performance review?"
- "What training programs did this employee complete?"
- "Which employees are currently on performance improvement plans?"

### Recruiting & Onboarding
- "What was the candidate's feedback from their interview panel?"
- "How many candidates did we interview for this position?"
- "Who did we decide to hire for [role]?"

### Culture & Policy
- "How did we handle this type of employee relations issue previously?"
- "What accommodations have we approved for similar requests?"
- "What were the common themes from recent exit interviews?"
- "Which departments have the highest attrition rates?"

---

## Executive & Strategic

### Strategic Decisions
- "What did we decide about our pricing strategy last month?"
- "What were our Q1 OKRs and how did we perform?"
- "What strategic priorities did we set at the last offsite?"
- "When did we decide to change our go-to-market approach?"

### Funding & Budget
- "What was our rationale for passing on that funding offer?"
- "What was the budget we allocated to [department/initiative]?"
- "What were the concerns raised about our burn rate?"

### Partnerships & Deals
- "When did we last discuss this partnership opportunity?"
- "What partnership discussions happened around [topic]?"
- "What were the terms we negotiated with [partner]?"

### Team & Hiring
- "Which candidates did we interview for the VP role?"
- "Who are the key stakeholders for [initiative]?"
- "What hiring decisions did we make last quarter?"

---

## Process & Operations

### Deployment & Release
- "What's our [deployment/testing/backup] process?"
- "What was the timeline for [release/deployment]?"
- "Are there any documented workarounds for [legacy system/edge case]?"

### Incident Response
- "What broke the last time we touched this?"
- "What was the outcome of [incident/outage]?"
- "What lessons were learned from [past incident/project]?"
- "What was the root cause of [issue]?"

### Team Coordination
- "Which teams or systems have dependencies on [service/API]?"
- "Who are the key stakeholders or approvers for changes to [critical system]?"
- "What cross-team dependencies exist for [initiative]?"

---

## Technical Debt & Maintenance

### Known Issues
- "What are the known limitations or technical debt in [component]?"
- "What technical debt did we knowingly take on?"
- "What technical debt items have we identified?"
- "Did we document why we skipped X feature?"

### Deprecation & Migration
- "What are the deprecation timelines for [old system/pattern]?"
- "When did we plan to revisit this decision?"
- "What changed when we migrated from X to Y?"
- "What backward compatibility constraints exist for [API/interface]?"
- "Who signed off on this commit?"
- "Which PR introduced this regression?"
- "Is this backported to the stable branch?"
- "What is the diffstat for the merge?"
- "Why was this patch NACKed?"

---

## Historical Context & Attribution

### Timeline Queries
- "When did we last discuss [topic]?"
- "What did we discuss about [topic] last month?"
- "What major technical decisions did we make this quarter?"
- "What happened in [timeframe]?"

### Attribution
- "Who decided on [technology/pattern] and when?"
- "Who made the original decision and why?"
- "Which team made this decision?"
- "What was decided by [person/team]?"

### Previous Attempts
- "Have we discussed [topic] before?"
- "Have we tried this approach before?"
- "Did we have a faster solution we shelved?"
- "Have we solved [problem] before?"
- "Has this been asked/answered before?"
- "Is there a mega-thread or summary for this?"
- "What's the community consensus on [topic]?"
- "Is there a definitive guide/PSA for this?"

---

## Competitive & Market Intelligence

### Competitive Analysis
- "What competitive analysis informed our positioning?"
- "How do we compare to [competitor]?"
- "What differentiates us from [competitor]?"

### Market Positioning
- "What customer segments showed the strongest PMF signals?"
- "What market research influenced [decision]?"
- "What did we learn from [market test/pilot]?"

---

## Compliance & Standards Queries

### Policy & Guidelines
- "What were our brand guidelines around [element]?"
- "What policy applies to [situation]?"
- "According to our guidelines, how should we handle [case]?"

### Audit & Documentation
- "What documentation exists for [system/process]?"
- "Where is this decision documented?"
- "What audit requirements apply to [component]?"

---

## Query Patterns by Intent

### Understanding Past Decisions
- "Why did we..."
- "What was the reasoning..."
- "What led to..."
- "How did we arrive at..."

### Seeking Context
- "What was happening when..."
- "What were the circumstances..."
- "What constraints existed..."
- "What was the background..."

### Looking for Precedent
- "Have we done this before?"
- "Is there a pattern for..."
- "What's our standard approach..."
- "How have we handled similar..."

### Checking Status
- "What's the current state of..."
- "Where are we with..."
- "What happened to..."
- "Did we ever..."

### Linguistic Universals (Generalized Patterns)
**Information Seeking Speech Acts**:
- **Canonical Status**: "What is the canonical way to...", "Is there a standard for..."
- **Origin/Provenance**: "What is the genesis of...", "Derivation of..."
- **Teleological (Purpose matches)**: "For what purpose was X created?", "To what end..."
- **Consensus Verification**: "Do we have consensus on...", "Is it agreed that..."
- **Counterfactuals**: "What if we had chosen...", "Would X have worked if..."

### Finding Ownership
- "Who decided..."
- "Which team owns..."
- "Who's responsible for..."
- "Whose decision was..."

---

## Natural Language Variations

### Conversational Forms
- "I'm wondering why we chose X?"
- "Can you remind me what we decided about Y?"
- "Do we have a standard for Z?"
- "Wasn't there a decision about this?"

### Implicit Queries
- "Tell me about [topic]"
- "What's our approach to [challenge]?"
- "How do we handle [situation]?"
- "What do we know about [subject]?"

---

## Query Optimization Strategies

### Specific vs. Broad
**Specific** (Preferred):
- "Why did we choose PostgreSQL over MySQL for the user service?"
- "What were the performance requirements for the API gateway?"

**Too Broad** (Needs refinement):
- "Why database?"
- "Tell me about architecture"

### Time-Scoped Queries
- "What did we decide last quarter?"
- "What were recent discussions about X?"
- "What changed since [date/event]?"

### Domain-Scoped Queries
- "What technical decisions..."
- "What security policies..."
- "What customer feedback on..."

---

## Implementation Notes

**For Claude**: When you detect these retrieval patterns:

1. **Parse Intent**: Understand what type of context is being requested
2. **Scope Search**: Determine relevant domains, timeframes, and people
3. **Execute Query**: Search FHE-encrypted organizational memory
4. **Rank Results**: Prioritize by relevance and recency
5. **Present Context**: Include sources, timestamps, and attributions

**Search Strategies**:
- Semantic similarity for concept matching
- Exact phrase matching for specific terms
- Temporal filtering for time-scoped queries
- Domain tagging for scoped searches

**Result Presentation**:
- Always include source (who/when)
- Provide relevant excerpts, not full dumps
- Offer to elaborate if user needs more detail
- Link related decisions when relevant

---

**Related**: See [capture-triggers.md](capture-triggers.md) for context capture patterns.
