# Usage Patterns

This guide shows common usage patterns for the Rune plugin after activation.

---

## Automatic Context Capture

The plugin automatically captures significant organizational context without manual intervention.

### Architecture Decisions

```
Developer: "We decided to use a microservices architecture instead of a
monolith because we need independent scaling for the API gateway and the
data processing service."

Claude: [Automatically captures this architectural decision]
```

### Technical Rationale

```
Developer: "We chose React over Vue because our team has more React
experience, and the component ecosystem is more mature for our use case."

Claude: [Stores framework decision with rationale]
```

### Code Patterns

```
Developer: "We always validate user input at the controller layer, never
in the view. This ensures consistent security across all endpoints."

Claude: [Captures coding standard]
```

### Team Agreements

```
Developer: "Our deployment policy is: all changes must go through staging
first, with at least 24 hours of testing before production deploy."

Claude: [Records team process]
```

---

## Manual Context Storage

Scribe captures most decisions automatically from conversation. Use `/rune remember` when Scribe missed something or you want to store context that isn't part of the current conversation:

### Security Policies

```
/rune remember "All API keys must be stored in environment variables,
never hardcoded. Use AWS Secrets Manager for production."
```

### Design Patterns

```
/rune remember "We use the Repository pattern for data access.
Each entity has a corresponding repository interface in the domain layer."
```

### Performance Decisions

```
/rune remember "Database queries must use pagination. Maximum page size
is 100 items. Always include total count in response."
```

### Onboarding Information

```
/rune remember "New developers should read docs/ARCHITECTURE.md first,
then set up their environment with scripts/dev-setup.sh"
```

---

## Context Retrieval

### Natural Language Queries

```
Developer: "Why did we choose microservices?"

Claude: "According to organizational memory from 2 weeks ago:
You decided to use a microservices architecture instead of a monolith
because you need independent scaling for the API gateway and the data
processing service.

Source: Team discussion on 2025-01-15"
```

### Specific Topics

```
Developer: "What's our database pagination policy?"

Claude: "Based on organizational memory:
Database queries must use pagination with a maximum page size of 100 items.
Always include total count in response.

Source: Captured 1 month ago"
```

### Historical Decisions

```
Developer: "Have we discussed authentication methods?"

Claude: "Yes, organizational memory contains 3 related decisions:

1. (2 months ago) All API endpoints must use JWT authentication
2. (1 month ago) JWT tokens expire after 24 hours
3. (2 weeks ago) Refresh tokens stored in HTTP-only cookies

Would you like details on any of these?"
```

---

## Collaborative Scenarios

### Developer Onboarding

**New developer Carol joins the team:**

```
Carol: "What tech stack are we using?"

Claude: "Based on organizational memory:

Backend:
- Node.js with Express (chosen for team familiarity)
- PostgreSQL (better JSON support than MySQL)
- Redis for caching

Frontend:
- React (team experience + ecosystem maturity)
- TypeScript (type safety)
- Tailwind CSS

Architecture:
- Microservices (independent scaling requirements)

Would you like more details on any of these decisions?"
```

### Cross-Team Context

**Team member working on related feature:**

```
Developer: "How did the API team implement authentication?"

Claude: "According to organizational memory from API team:

Authentication Implementation:
1. JWT tokens with 24-hour expiration
2. Refresh tokens in HTTP-only cookies
3. Token validation middleware in auth.middleware.ts
4. Public key rotation every 90 days

Source: API team lead, captured 3 weeks ago"
```

### Avoiding Repeated Discussions

**Same question asked months later:**

```
Developer: "Should we use MySQL or PostgreSQL?"

Claude: "This was already decided 4 months ago. From organizational memory:

Decision: PostgreSQL
Reason: Better JSON support than MySQL, data model is heavily document-oriented
Decided by: Team consensus after 2-week trial
Status: Currently in production with 500GB data

The decision was successful - no issues reported.

Would you like to reconsider this, or are you looking for specific PostgreSQL usage patterns?"
```

---

## Search Patterns

Retriever handles these automatically when you ask naturally. Use `/rune recall` as an explicit override when you want to bypass Retriever's intent detection.

### By Topic

```
/rune recall "deployment process"

Returns:
1. Deployment policy (staging → production, 24h testing)
2. CI/CD pipeline configuration (GitHub Actions)
3. Rollback procedures
4. Production access controls
```

### By Time

```
Developer: "What did we discuss about security last month?"

Claude searches organizational memory with time filter and returns relevant context.
```

### By Team Member

```
Developer: "What decisions did Alice make about the database?"

Claude: "Alice captured these database-related decisions:

1. (3 months ago) PostgreSQL choice with rationale
2. (2 months ago) Database pagination policy
3. (1 month ago) Index strategy for query optimization
4. (2 weeks ago) Backup schedule and retention policy"
```

---

## Advanced Usage

### Project-Specific Context

Switch between different project contexts:

```
# Configure for Project Alpha
/rune configure
# Collection: "project-alpha-context"

# Work on Project Alpha...
# All context stored in alpha collection

# Switch to Project Beta
/rune configure
# Collection: "project-beta-context"

# Work on Project Beta...
# Separate context, no cross-contamination
```

### Confidential Information

The plugin automatically redacts secrets:

```
Developer: "We use AWS access key AKIAIOSFODNN7EXAMPLE for deployment"

Claude: [Captures context but redacts the actual key]
Stored: "We use AWS access key [REDACTED] for deployment"
```

Supported redaction patterns:
- API keys
- AWS credentials
- JWT tokens
- Passwords
- SSH keys
- Database connection strings with credentials

### Bulk Import

Import existing documentation:

```
/rune remember "$(cat docs/ARCHITECTURE.md)"
```

Import decision records:

```
for file in docs/decisions/*.md; do
  /rune remember "$(cat $file)"
done
```

---

## Best Practices

### 1. Be Specific with Decisions

**Bad**:
```
"We're using React"
```

**Good**:
```
"We chose React over Vue because our team has more React experience,
and the component ecosystem is more mature for data visualization,
which is critical for our dashboard."
```

### 2. Include Context

**Bad**:
```
"Use PostgreSQL"
```

**Good**:
```
"We decided to use PostgreSQL instead of MySQL because:
1. Better JSON support (critical for our document-heavy data model)
2. More robust indexing for text search
3. Team has production PostgreSQL experience
Decision made after 2-week trial with both databases."
```

### 3. Update Decisions

When circumstances change:

```
/rune remember "Updated database decision: We migrated from PostgreSQL
to MongoDB in Q2 2025. Reason: Document model evolved to be too complex
for relational schema. PostgreSQL worked well initially but became
bottleneck at scale."
```

### 4. Regular Reviews

Periodically review organizational memory:

```
Developer: "What major technical decisions did we make this quarter?"

Claude: [Summarizes captured decisions from last 3 months]
```

---

## Troubleshooting Common Issues

### Context Not Being Captured

**Issue**: You said something important but Scribe's automatic detection didn't capture it.

**Why**: Scribe uses pattern matching and significance scoring (threshold 0.7) to decide what to capture. Some context may fall below the threshold if it lacks clear decision-language triggers like "we decided...", "we chose X over Y because...", etc.

**Solution**: Use `/rune remember` as a manual override:
```
/rune remember "Previous decision: [repeat the important context]"
```

### Too Much Noise

**Issue**: Too many trivial captures.

**Solution**: The plugin uses significance threshold (0.7). Truly trivial
content should be filtered. If you see noise, report it as feedback.

### Wrong Context Retrieved

**Issue**: Claude returned irrelevant context for your query.

**Solution**: Be more specific in your query:

Instead of: "Why database?"
Use: "Why did we choose PostgreSQL over MySQL for the user service?"

### Outdated Information

**Issue**: Retrieved context is outdated.

**Solution**: State the updated decision in conversation (Scribe will capture it), or use `/rune remember` to force-store:
```
/rune remember "Updated: [new information that supersedes old decision]"
```

---

## Integration with Development Workflow

### During Code Review

```
Reviewer: "Why did we use this pattern here?"

Claude: [Searches organizational memory for relevant pattern decisions]

Result: "According to organizational memory from 2 months ago:
We use the Repository pattern for all data access. Each entity has
a corresponding repository interface in the domain layer for better
testability and dependency injection."
```

### During Sprint Planning

```
Team Lead: "What technical debt did we identify last sprint?"

Claude: [Searches organizational memory for tech debt mentions]

Returns:
1. Authentication system needs token refresh logic
2. Database queries missing pagination
3. API error handling inconsistent across services
```

### During Incident Response

```
Developer: "What's our database backup strategy?"

Claude: "According to organizational memory:
- Full backup daily at 2 AM UTC
- Incremental backups every 6 hours
- Retention: 30 days
- Stored in: AWS S3 bucket 'prod-db-backups'
- Recovery tested: Monthly

Source: Captured from infra team 2 months ago"
```

---

## Summary

**Automatic Capture (Scribe)**: Automatically identifies and captures significant decisions from conversation — the primary capture path.
**Automatic Retrieval (Retriever)**: Automatically detects recall-intent queries in natural conversation — the primary retrieval path.
**Manual Override (`/rune remember`)**: Force-store context that Scribe missed — use sparingly.
**Manual Override (`/rune recall`)**: Explicitly search memory bypassing Retriever's intent detection — use when natural language doesn't trigger recall.
**Team Collaboration**: All team members share same organizational memory.
**Security**: FHE encryption ensures zero-knowledge privacy.

The plugin works best when teams:
1. Discuss decisions openly with Claude present (Scribe captures automatically)
2. Be specific about rationale and context (improves Scribe's detection)
3. Update decisions when they change
4. Ask questions naturally (Retriever handles recall automatically)
