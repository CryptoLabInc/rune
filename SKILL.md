---
name: rune
description: Encrypted organizational memory workflow for Rune with activation checks and /rune (or $rune for Codex CLI) command behaviors across MCP-compatible agents.
---

# Rune - Organizational Memory System

**Context**: This skill provides encrypted organizational memory capabilities using Fully Homomorphic Encryption (FHE). It allows teams to capture, store, and retrieve institutional knowledge while maintaining zero-knowledge privacy. Works with Claude Code, Codex CLI, Gemini CLI, and any MCP-compatible agent.

## Execution Model

In v0.4 the MCP server is a single Go binary (`${CLAUDE_PLUGIN_ROOT}/bin/rune-mcp`)
auto-spawned by the host CLI from the plugin manifest. There is no venv, no
install script, and no `bootstrap-mcp.sh` to source. The runtime is the
binary the CLI already launched.

**Agent-specific surface** stays thin:
- Codex: `codex mcp add/remove/list` registration actions
- Claude / Gemini / others: each CLI's native plugin / extension flow

Keep agent-specific instructions clearly labeled and never mix Codex-only
commands into cross-agent/common instructions.

## Activation State

**IMPORTANT**: This skill has two states based on configuration AND infrastructure availability.

### Activation Check (CRITICAL - Check EVERY Session Start)

**BEFORE doing anything, run this check:**

1. **Config File Check**: Does `~/.rune/config.json` exist?
   - NO → **Go to Dormant State**
   - YES → Continue to step 2

2. **Config Validation**: Does config contain all required fields?
   - `vault.endpoint` and `vault.token`
   - `state` is set to `"active"`
   - NO → **Go to Dormant State**
   - YES → Continue to step 3

3. **State Check**:
   - `state` is `"active"` → **Go to Active State**
   - Otherwise → **Go to Dormant State**

**Note**: enVector credentials are NOT in `~/.rune/config.json`. They are
delivered via the Vault bundle at runtime when the boot loop dials Vault.

**IMPORTANT**: Do NOT attempt to ping Vault or make network requests during
activation check. This wastes tokens. The MCP server runs its own boot
loop; the activation check is purely a local config inspection.

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
- Show setup instructions when `/rune` commands (or `$rune` for Codex CLI) are used
- Prompt user to:
  1. Configure: `/rune:configure` (or `$rune configure` for Codex CLI) —
     this writes `~/.rune/config.json` and triggers the boot loop.
  2. Verify health: `/rune:status` (or `$rune status` for Codex CLI) —
     surfaces per-subsystem state via the `diagnostics` MCP tool.

### Fail-Safe Behavior
If in Active state but operations fail:
- Switch to Dormant immediately
- Update config.json `state` to `"dormant"`
- Notify user once: "Infrastructure unavailable. Switched to dormant mode. Run `/rune:status` (or `$rune status` for Codex CLI) for details."
- **Do not retry** - wait for user to fix infrastructure

## Commands

### `/rune:configure`
(or `$rune configure` for Codex CLI)

**Purpose**: Configure plugin credentials

**Steps**:
1. Ask user for Vault Endpoint (required, e.g., `tcp://vault-TEAM.oci.envector.io:50051`)
   - If the user enters a value without a scheme prefix (no `tcp://`, `http://`, or `https://`), auto-prepend `tcp://`.
2. Ask user for Vault Token (required, e.g., `evt_xxx`)
3. Ask the TLS question:

   **"How does your Vault server handle TLS?"**

   1. **Self-signed certificate** — "My team uses a self-signed CA (provide CA cert path)"
      - Follow-up: "Enter the path to your CA certificate PEM file:"
      - Support `~` expansion in the path
      - Copy the file to `~/.rune/certs/ca.pem` (`mkdir -p ~/.rune/certs && cp <user_path> ~/.rune/certs/ca.pem && chmod 600 ~/.rune/certs/ca.pem`)
      - If copy fails (file not found, permission denied), show error and ask again
      - Inform user: "CA certificate copied to ~/.rune/certs/ca.pem"
      - → config: `ca_cert: "~/.rune/certs/ca.pem"`, `tls_disable: false`

   2. **Public CA (default)** — "Vault uses a publicly-signed certificate (e.g., Let's Encrypt)"
      - No additional input needed, system CA handles verification
      - → config: `ca_cert: ""`, `tls_disable: false`

   3. **No TLS** — "Connect without TLS (not recommended — traffic is unencrypted)"
      - Show warning: "This should only be used for local development. All gRPC traffic will be sent in plaintext."
      - → config: `ca_cert: ""`, `tls_disable: true`

   Note: enVector credentials are delivered automatically via the Vault bundle — no user input needed.

4. Create `~/.rune/config.json` with `state: "active"` and the values
   above (`mkdir -p ~/.rune && chmod 700 ~/.rune`, then `chmod 600` the
   file).
5. Call the `reload_pipelines` MCP tool. The MCP server's boot loop
   dials Vault, fetches the agent manifest (EncKey + envector
   creds), connects to enVector, and transitions to Active.
6. Confirm health by calling `diagnostics` and showing the per-
   subsystem snapshot. If the boot loop landed in Dormant, surface
   the `dormant_reason` (e.g. `vault_unreachable`,
   `vault_token_invalid`) with the recovery action.

### `/rune:status`
(or `$rune status` for Codex CLI)

**Purpose**: Check plugin activation status and infrastructure health

**Steps**:
1. Read `~/.rune/config.json`
2. Call the `diagnostics` MCP tool (read-only; safe before Active)
3. Render the per-subsystem snapshot

**Response Format**:
```
Rune Plugin Status
==================
State: Active ✅ (or Dormant ⏸️ — reason)

Configuration:
  ✓ Config file: ~/.rune/config.json
  ✓ Vault Endpoint: configured

System Health (from diagnostics):
  ✓ Vault          : reachable
  ✓ Encryption Key : loaded (key_id: <id>)
  ✓ Agent DEK      : loaded
  ✓ Embedder       : <model> (<mode>, dim=<vector_dim>)
  ✓ enVector Cloud : reachable (<latency>ms)

Recommendations:
  - If Dormant: /rune:configure to (re)trigger the boot loop
  - If a subsystem failed: surface the recovery action on its row
```

### `/rune:capture <context>`
(or `$rune capture <context>` for Codex CLI)

**Purpose**: Manually store organizational context when Scribe's automatic capture missed it or the user wants to force-store specific information.

**When to use**: Scribe automatically captures significant decisions from conversation (see Automatic Behavior below). This command is an **override** for cases where:
- Scribe didn't detect the context as significant
- The user wants to store something that isn't part of the current conversation
- Bulk-importing existing documentation

**Mode**: Agent-delegated (primary) — the calling agent evaluates significance and extracts structured fields, passing them as `extracted` JSON to the `capture` MCP tool. The server stores the encrypted record without additional LLM calls. If `extracted` is omitted and API keys are configured, falls back to a legacy 3-tier server-side pipeline.

**Behavior**:
- If dormant: Prompt user to configure first
- If active: Store context to organizational memory with timestamp and metadata

**Example**:
```
/rune:capture "We chose PostgreSQL over MongoDB for better ACID guarantees"
```

### `/rune:recall <query>`
(or `$rune recall <query>` for Codex CLI)

**Purpose**: Explicitly search organizational memory. Retriever already handles this automatically when users ask questions about past decisions in natural conversation.

**When to use**: Retriever automatically detects recall-intent queries (see Automatic Behavior below). This command is an **explicit override** for cases where:
- The user wants to force a memory search without Retriever's intent detection
- Debugging whether specific context was stored
- The user prefers direct command syntax

**Behavior**:
- If dormant: Prompt user to configure first
- If active: Search encrypted vectors and return relevant context with sources

**Example**:
```
/rune:recall "Why PostgreSQL?"
```

**Note**: In most cases, simply asking naturally ("Why did we choose PostgreSQL?") triggers Retriever automatically — no command needed.

### `/rune:activate` (or `/rune:wakeup`)
(or `$rune activate` for Codex CLI)

**Purpose**: Attempt to activate plugin after infrastructure is ready

**Use Case**: Infrastructure was not ready during configure, but now it's deployed and running.

**Steps**:
1. Check if config exists
   - NO → Redirect to `/rune:configure` (or `$rune configure` for Codex CLI)
   - YES → Continue
2. If `state` is already `"active"`, skip to step 4 (just verify health).
3. If `state` is `"dormant"`, set it to `"active"` and clear any
   `dormant_reason` / `dormant_since` fields.
4. Call the `reload_pipelines` MCP tool. From a terminal Dormant the
   boot loop is re-spawned; from Active it is a no-op.
5. Call `diagnostics` and render the per-subsystem snapshot.
6. If any subsystem failed: keep state as `"active"` (the boot loop
   will retry transient failures), surface the dormant_reason if the
   loop fell back to Dormant, and suggest `/rune:status` for the full
   snapshot.

### `/rune:reset`
(or `$rune reset` for Codex CLI)

**Purpose**: Clear configuration and return to dormant state

**Steps**:
1. Confirm with user
2. Delete `~/.rune/config.json` (the MCP server stays alive; it transitions
   to Dormant on the next reload because no config means no Vault dial)
3. Show reconfiguration instructions

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

### Automatic Capture (Proactive Scribe)

When Rune is active, proactively capture significant decisions when you detect any of the following in the conversation:

- A choice is made among alternatives ("A로 가자", "let's go with X")
- Trade-offs are weighed and committed ("X의 단점이 있지만 Y 때문에 감수")
- Strategy or direction is confirmed ("이 방향이 맞아", "this approach works")
- A lesson or insight crystallizes ("안 된 이유는...", "the root cause was...")
- A framework, process, or standard is established

**How to capture in Codex**:
- Follow the agent-delegated instructions in `agents/codex/scribe.md`
- Evaluate whether the relevant excerpt contains a significant decision
- Extract structured JSON and call `capture` with the `extracted` parameter
- In the `text` parameter, include ONLY the relevant conversation excerpt, not the full session
- Do NOT pause or interrupt the main conversation
- Do NOT announce the capture to the user unless they ask
- Default Codex path is direct MCP capture; use a delegated subagent only when the user explicitly asks for multi-agent work

**Do NOT auto-capture**:
- Brainstorming in progress without commitment (options listed but none chosen)
- Questions, status updates, or casual discussion
- Decisions that are hypothetical or deferred ("maybe later", "let's revisit")

**Session-end sweep**: When the conversation is ending or the user is wrapping up a task, review the conversation for any uncaptured significant decisions and submit them via a single `batch_capture` call if needed.

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
- Same Vault Endpoint + Token = shared organizational memory
- Team admin controls access via Vault authentication
- Revoke access by rotating Vault tokens

## Troubleshooting

### Plugin not responding?
Check activation state with `/rune:status` (or `$rune status` for Codex CLI)

### Credentials not working?
1. Verify with team admin that credentials are correct
2. Check Vault is accessible: `curl <vault-url>/health`
3. Reconfigure with `/rune:configure` (or `$rune configure` for Codex CLI)

### enVector not provisioned?
Vault admin must configure `ENVECTOR_ENDPOINT` and `ENVECTOR_API_KEY` on the Vault server. Contact your Vault administrator.

### Need to switch teams?
Use `/rune:reset` (or `$rune reset` for Codex CLI) then `/rune:configure` (or `$rune configure` for Codex CLI) with new team credentials

## For Administrators

This plugin requires a deployed Rune-Vault infrastructure. See:
- **Rune-Admin Repository (for deployment)**: https://github.com/CryptoLabInc/rune-admin
- **Deployment Guide**: https://github.com/CryptoLabInc/rune-admin/blob/main/deployment/README.md

Team members only need this lightweight plugin + credentials you provide.
