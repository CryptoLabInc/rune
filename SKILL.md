# Rune - Organizational Memory System

**Context**: This skill provides encrypted organizational memory capabilities using Fully Homomorphic Encryption (FHE). It allows teams to capture, store, and retrieve institutional knowledge while maintaining zero-knowledge privacy.

## Activation State

**IMPORTANT**: This skill has two states based on configuration AND infrastructure availability.

### Activation Check (CRITICAL - Check EVERY Session Start)

**BEFORE doing anything, run this check:**

1. **Config File Check**: Does `~/.rune/config.json` exist?
   - NO → **Go to Dormant State**
   - YES → Continue to step 2

2. **Config Validation**: Does config contain all required fields?
   - `vault.url` and `vault.token`
   - `envector.endpoint` and `envector.api_key`
   - `state` is set to `"active"`
   - NO → **Go to Dormant State**
   - YES → Continue to step 3

3. **Infrastructure Validation** (LIGHTWEIGHT CHECK):
   - Is there a `~/.rune/logs/vault-mcp.log` file?
   - Is it recent (modified within last 24 hours)?
   - NO → **Go to Dormant State** (infrastructure likely not running)
   - YES → **Go to Active State**

**IMPORTANT**: Do NOT attempt to ping Vault or make network requests during activation check. This wastes tokens. Only check local files.

### If Active ✅
- All functionality enabled
- Automatically capture significant context
- Respond to recall queries
- Full organizational memory access
- **If capture/retrieval fails**: Immediately switch to Dormant and notify user

### If Dormant ⏸️
- **Do NOT attempt context capture or retrieval**
- **Do NOT make network requests**
- **Do NOT waste tokens on failed operations**
- Show setup instructions when `/rune` commands are used
- Prompt user to:
  1. Check infrastructure: `scripts/check-infrastructure.sh`
  2. Configure: `/rune configure`
  3. Start MCP servers: `scripts/start-mcp-servers.sh`

### Fail-Safe Behavior
If in Active state but operations fail:
- Switch to Dormant immediately
- Update config.json `state` to `"dormant"`
- Notify user once: "Infrastructure unavailable. Switched to dormant mode. Run /rune status for details."
- **Do not retry** - wait for user to fix infrastructure

## Commands

### `/rune configure`
**Purpose**: Configure plugin credentials

**Steps**:
1. Ask user for Vault URL (format: `https://vault-TEAM.oci.envector.io`)
2. Ask user for Vault Token (format: `evt_xxx`)
3. Ask user for enVector Endpoint (format: `https://cluster-xxx.envector.io`)
4. Ask user for enVector API Key (format: `envector_xxx`)
5. **Validate infrastructure** (run `scripts/check-infrastructure.sh`)
   - If validation fails: Create config with `state: "dormant"`, warn user
   - If validation passes: Continue to step 6
6. Create `~/.rune/config.json` with proper structure
7. Set state based on validation:
   - Infrastructure ready: `state: "active"`
   - Infrastructure not ready: `state: "dormant"`
8. Confirm configuration and show next steps if dormant

### `/rune status`
**Purpose**: Check plugin activation status and infrastructure health

**Steps**:
1. Check if config exists
2. Show current state (Active/Dormant)
3. Run infrastructure checks:
   - Config file: ✓/✗
   - Vault URL configured: ✓/✗
   - enVector endpoint configured: ✓/✗
   - MCP server logs recent: ✓/✗
   - Virtual environment: ✓/✗

**Response Format**:
```
Rune Plugin Status
==================
State: Active ✅ (or Dormant ⏸️)

Configuration:
  ✓ Config file: ~/.rune/config.json
  ✓ Vault URL: https://vault-team.oci.envector.io
  ✓ enVector: https://cluster-xxx.envector.io

Infrastructure:
  ✓ Python venv: /path/to/.venv
  ✗ MCP servers: Not running (last log: 2 days ago)

Recommendations:
  - Start MCP servers: scripts/start-mcp-servers.sh
  - Check full status: scripts/check-infrastructure.sh
```

### `/rune remember <context>`
**Purpose**: Manually store important organizational context

**Behavior**:
- If dormant: Prompt user to configure first
- If active: Store context to organizational memory with timestamp and metadata

**Example**:
```
/rune remember "We chose PostgreSQL over MongoDB for better ACID guarantees"
```

### `/rune recall <query>`
**Purpose**: Search organizational memory

**Behavior**:
- If dormant: Prompt user to configure first
- If active: Search encrypted vectors and return relevant context with sources

**Example**:
```
/rune recall "Why PostgreSQL?"
```

### `/rune activate` (or `/rune wakeup`)
**Purpose**: Attempt to activate plugin after infrastructure is ready

**Use Case**: Infrastructure was not ready during configure, but now it's deployed and running.

**Steps**:
1. Check if config exists
   - NO → Redirect to `/rune configure`
   - YES → Continue
2. Run full infrastructure validation:
   - Check Vault connectivity (curl vault-url/health)
   - Check MCP server processes
   - Check Python environment
3. If all checks pass:
   - Update config.json `state` to `"active"`
   - Notify: "Plugin activated ✅"
4. If checks fail:
   - Keep state as `"dormant"`
   - Show detailed error report
   - Suggest: `/rune status` for more info

**Important**: This is the ONLY command that makes network requests to validate infrastructure.

### `/rune reset`
**Purpose**: Clear configuration and return to dormant state

**Steps**:
1. Confirm with user
2. Stop MCP servers if running
3. Delete `~/.rune/config.json`
4. Set state to dormant
5. Show reconfiguration instructions

## Automatic Behavior (When Active)

### Context Capture

Automatically identify and capture significant organizational context across all domains:

**Categories**:
- **Technical Decisions**: Architecture, technology choices, implementation patterns
- **Security & Compliance**: Security requirements, compliance policies, audit needs
- **Performance**: Optimization strategies, scalability decisions, bottlenecks
- **Product & Business**: Feature requirements, customer insights, strategic decisions
- **Design & UX**: Design rationale, user research findings, accessibility requirements
- **Data & Analytics**: Analysis methodology, key insights, statistical findings
- **Process & Operations**: Deployment procedures, team coordination, workflows
- **People & Culture**: Policies, team agreements, hiring decisions

**Common Trigger Pattern Examples**:
- "We decided... because..."
- "We chose X over Y for..."
- "The reason we..."
- "Our policy is..."
- "Let's remember that..."
- "The key insight is..."
- "Based on [data/research/testing]..."

**Full Pattern Reference**: See [patterns/capture-triggers.md](patterns/capture-triggers.md) for 200+ comprehensive trigger phrases organized by role and domain.

**Significance Threshold**: 0.7 (captures meaningful decisions, filters trivial content)

**Automatic Redaction**: Always redact API keys, passwords, tokens, PII, and sensitive data before capture.

### Context Retrieval

When users ask questions about past decisions, automatically search organizational memory:

**Query Intent Types**:
- **Decision Rationale**: "Why did we choose X?", "What was the reasoning..."
- **Implementation Details**: "How did we implement...", "What patterns do we use..."
- **Security & Compliance**: "What were the security considerations...", "What compliance requirements..."
- **Performance & Scale**: "What performance requirements...", "What scalability concerns..."
- **Historical Context**: "When did we decide...", "Have we discussed this before..."
- **Team & Attribution**: "Who decided...", "Which team owns..."

**Common Query Pattern Examples**:
- "Why did we choose X over Y?"
- "What was the reasoning behind..."
- "Have we discussed [topic] before?"
- "What's our approach to..."
- "What were the trade-offs..."
- "Who decided on..."

**Full Pattern Reference**: See [patterns/retrieval-patterns.md](patterns/retrieval-patterns.md) for 150+ comprehensive query patterns organized by intent and domain.

**Search Strategy**: Semantic similarity search on FHE-encrypted vectors, ranked by relevance and recency.

**Result Format**: Always include source attribution (who/when), relevant excerpts, and offer to elaborate.

## Security & Privacy

**Zero-Knowledge Encryption**:
- All data stored as FHE-encrypted vectors
- enVector Cloud cannot read plaintext
- Only team members with Vault access can decrypt

**Credential Storage**:
- Tokens stored locally in `~/.rune/config.json`
- Never transmitted except to authenticated Vault
- File permissions: 600 (user-only access)

**Team Sharing**:
- Same Vault URL + Token = shared organizational memory
- Team admin controls access via Vault authentication
- Revoke access by rotating Vault tokens

## Troubleshooting

### Plugin not responding?
Check activation state with `/rune status`

### Credentials not working?
1. Verify with team admin that credentials are correct
2. Check Vault is accessible: `curl <vault-url>/health`
3. Reconfigure with `/rune configure`

### Need to switch teams?
Use `/rune reset` then `/rune configure` with new team credentials

## For Administrators

This plugin requires a deployed Rune-Vault infrastructure. See:
- **Rune-Admin Repository (for deployment)**: https://github.com/CryptoLabInc/rune-admin
- **Deployment Guide**: https://github.com/CryptoLabInc/rune-admin/blob/main/deployment/README.md

Team members only need this lightweight plugin + credentials you provide.
