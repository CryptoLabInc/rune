# Team Setup Example

This guide shows how a team administrator sets up Rune infrastructure and onboards team members.

> **Admin tooling**: Vault deployment and infrastructure management are in the [rune-admin](https://github.com/CryptoLabInc/rune-admin) repository.

## Scenario

**Team**: Acme Engineering (3 developers)
- Alice (Team Admin)
- Bob (Backend Developer)
- Carol (Frontend Developer)

**Goal**: Share organizational memory across the team with FHE encryption.

---

## Step 1: Alice Deploys Rune-Vault

Alice runs the interactive installer, which handles cloud provisioning, TLS setup, and enVector Cloud configuration:

```bash
curl -fsSL https://raw.githubusercontent.com/CryptoLabInc/rune-admin/main/install.sh \
  -o install.sh && sudo bash install.sh
```

The installer guides her through:
- **Cloud provider** selection (OCI / AWS / GCP)
- **enVector Cloud** credentials (endpoint + API key)
- **TLS certificate** generation
- **Terraform-based** VM provisioning

Output:
```
vault_endpoint = "vault-acme.oci.envector.io:50051"
ca.pem downloaded for TLS verification
```

### Verify Deployment

```bash
# gRPC health check (requires grpcurl: brew install grpcurl)
grpcurl -cacert ca.pem vault-acme.oci.envector.io:50051 grpc.health.v1.Health/Check

# Expected: { "status": "SERVING" }
```

---

## Step 2: Alice Issues Per-User Tokens

Alice creates individual tokens for each team member using the Vault admin CLI:

```bash
# Issue tokens per user
runevault token issue --user alice --role admin
runevault token issue --user bob   --role member
runevault token issue --user carol --role member
```

Roles control access scope and rate limits:

| Role | Scope | Rate Limit |
|------|-------|------------|
| `admin` | Full access + token management | 150 req/60s |
| `member` | Capture + recall only | 30 req/60s |

---

## Step 3: Alice Onboards Bob

### 3.1 Share Credentials

Alice sends Bob his credentials via a secure channel (1Password, Signal, etc.):

```
Hi Bob,

I've set up our team's organizational memory system. Here are your credentials:

  Vault Endpoint: vault-acme.oci.envector.io:50051
  Vault Token: evt_acme_bob_xyz789

enVector Cloud credentials are delivered automatically via the Vault
bundle — no action needed on your end.

Setup:
  1. Install: /plugin marketplace add https://github.com/CryptoLabInc/rune
  2. Configure: /rune:configure
  3. Enter the Vault endpoint and token above

Alice
```

### 3.2 Bob Installs and Configures

**Claude Code:**
```bash
# Inside a Claude Code session
> /plugin marketplace add https://github.com/CryptoLabInc/rune
> /plugin install rune
> /rune:configure
# Enters Vault endpoint and token from Alice
```

**Codex CLI:**
```bash
# Inside a Codex session
> $skill-installer install https://github.com/CryptoLabInc/rune.git
> $rune configure
# Enters Vault endpoint and token
```

**Gemini CLI:**
```bash
# From terminal
$ gemini extensions install https://github.com/CryptoLabInc/rune.git
# Inside a Gemini session
> /rune:configure
# Enters Vault endpoint and token
```

Bob is now connected to the team memory.

---

## Step 4: Team Uses Shared Memory

### Alice captures a decision

```
Alice to her agent: "We decided to use PostgreSQL for this project because
it has better JSON support than MySQL, and our data model is heavily
document-oriented."

Agent: [Automatically captures to organizational memory]
```

### Bob retrieves the context (later that day)

```
Bob to his agent: "What database are we using for this project and why?"

Agent: "According to organizational memory from earlier today:
The team decided to use PostgreSQL because it has better JSON support
than MySQL, and the data model is heavily document-oriented.

Source: Captured from Alice's session at 10:23 AM"
```

### Carol joins the team (next week)

Alice issues Carol's token and sends credentials. Carol installs and configures — and immediately has access to all historical context from Alice and Bob.

---

## Step 5: Security Management

### Token Rotation

Alice rotates tokens periodically using the Vault admin CLI:

```bash
# Rotate a single user's token
runevault token rotate --user bob

# Rotate all tokens at once
runevault token rotate --all

# Distribute new tokens to team members via secure channel
```

Team members update their token:
```
> /rune:configure
# Update only the Vault token — other fields unchanged
```

### Remove a Team Member

```bash
# Revoke the departing member's token
runevault token revoke --user bob

# No token rotation needed — other members' tokens remain valid
```

Bob's token is immediately invalidated. Alice and Carol continue uninterrupted.

---

## Benefits

### Context Continuity
- Bob doesn't need to ask Alice "Why PostgreSQL?"
- Carol onboards instantly with full historical context
- No knowledge loss when Alice is on vacation

### Zero-Knowledge Privacy
- enVector Cloud sees only encrypted vectors
- Only team members with valid Vault tokens can decrypt
- Cloud provider cannot read any content

### Per-User Security
- Individual tokens — revoke one without disrupting others
- Role-based access control (admin vs. member)
- Rate limiting prevents abuse
- Audit logging tracks all operations

---

## Troubleshooting

### Bob can't connect

```
Bob: /rune:status
Agent: Error: Cannot connect to Vault

Alice checks:
  1. Vault is running: grpcurl -cacert ca.pem vault-acme.oci.envector.io:50051 grpc.health.v1.Health/Check
  2. Bob's token is correct: runevault token list
  3. Firewall allows Bob's IP (port 50051)

Issue: Bob had a typo in the Vault endpoint
Fix: Bob runs /rune:configure with the correct endpoint
```

### Carol sees no results

```
Carol: "Why PostgreSQL?"
Agent: No relevant context found.

Issue: Carol's Vault token is expired or incorrect
Fix: Alice checks with `runevault token list`, re-issues if needed
     Carol runs /rune:configure with the new token
```

---

## Advanced: Multiple Projects

To isolate organizational memory by project, deploy separate Vault instances:

```bash
# Project Alpha — its own Vault with its own index
# install.sh → index_name: "project-alpha"
# vault_endpoint: vault-alpha.oci.envector.io:50051

# Project Beta — separate Vault instance
# install.sh → index_name: "project-beta"
# vault_endpoint: vault-beta.oci.envector.io:50051
```

Team members configure the Vault endpoint matching their current project. The index name is managed by the Vault admin and distributed automatically at startup.

---

## Summary

**Admin Tasks** (Alice):
1. Deploy Rune-Vault via interactive installer (one-time, ~30 minutes)
2. Issue per-user tokens (per member, 1 minute)
3. Rotate tokens periodically (10 minutes)

**Team Member Tasks** (Bob, Carol):
1. Install plugin (one-time, 2 minutes)
2. Configure with Vault endpoint + token (one-time, 1 minute)
3. Use naturally (ongoing, zero overhead)

**Result**: Fully encrypted, shared organizational memory with per-user access control and zero-knowledge privacy.
