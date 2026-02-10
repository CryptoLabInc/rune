# Team Setup Example

This guide shows how a team administrator sets up Rune infrastructure and onboards team members.

## Scenario

**Team**: Acme Engineering (3 developers)
- Alice (Team Admin)
- Bob (Backend Developer)
- Carol (Frontend Developer)

**Goal**: Share organizational memory across the team with FHE encryption.

---

## Step 1: Alice Deploys Rune Infrastructure

Alice is the team administrator and needs to deploy:
1. Rune-Vault (team-shared key management)
2. enVector Cloud account (encrypted vector storage)

### 1.1 Deploy Rune-Vault

```bash
# Alice clones full Rune repository (not this plugin)
git clone https://github.com/CryptoLabInc/rune.git
cd rune

# Deploy to Oracle Cloud Infrastructure
cd deployment/oci

# Edit terraform.tfvars
cat > terraform.tfvars << EOF
team_name = "acme"
region = "us-ashburn-1"
vault_instance_shape = "VM.Standard.E4.Flex"
EOF

# Deploy
terraform init
terraform apply

# Note the outputs:
# vault_url = "https://vault-acme.oci.envector.io"
# vault_token = "evt_acme_abc123def456"
```

### 1.2 Sign up for enVector Cloud

```bash
# Alice visits https://envector.io
# Creates account: alice@acme.com
# Creates cluster: acme-us-west-2
# Generates API key: envector_xyz789

# Note the credentials:
# endpoint = "https://cluster-us-west-2.envector.io"
# api_key = "envector_xyz789"
```

---

## Step 2: Alice Configures Her Own Plugin

```bash
# Alice installs the lightweight plugin
cd ~/workspace
/plugin install github.com/CryptoLabInc/rune-plugin

# Configure with her credentials
/rune configure

# Enter:
# Vault URL: https://vault-acme.oci.envector.io
# Vault Token: evt_acme_abc123def456
# enVector Endpoint: https://cluster-us-west-2.envector.io
# enVector API Key: envector_xyz789

# Plugin is now active ✅
```

---

## Step 3: Alice Onboards Bob

### 3.1 Share Credentials

Alice sends Bob an email:

```
Subject: Rune Plugin Setup - Acme Team

Hi Bob,

I've set up our team's organizational memory system. Here are your credentials:

**Rune-Vault**:
- URL: https://vault-acme.oci.envector.io
- Token: evt_acme_abc123def456

**enVector Cloud** (shared):
- Endpoint: https://cluster-us-west-2.envector.io
- API Key: envector_xyz789

**Setup Instructions**:
1. Run: /plugin install github.com/CryptoLabInc/rune-plugin
2. Run: /rune configure
3. Enter the credentials above

Let me know if you have any issues!

Alice
```

### 3.2 Bob Installs and Configures

```bash
# Bob receives email and installs
/plugin install github.com/CryptoLabInc/rune-plugin

# Configure
/rune configure
# Enters credentials from Alice's email

# Plugin is now active ✅
```

---

## Step 4: Team Uses Shared Memory

### Alice captures a decision

```
Alice to Claude: "We decided to use PostgreSQL for this project because
it has better JSON support than MySQL, and our data model is heavily
document-oriented."

Claude: [Automatically captures to organizational memory]
```

### Bob retrieves the context (later that day)

```
Bob to Claude: "What database are we using for this project and why?"

Claude: "According to organizational memory from earlier today:
The team decided to use PostgreSQL because it has better JSON support
than MySQL, and the data model is heavily document-oriented.

Source: Captured from Alice's session at 10:23 AM"
```

### Carol joins the team (next week)

Alice sends Carol the same credentials email. Carol installs and configures:

```bash
/plugin install github.com/CryptoLabInc/rune-plugin
/rune configure
# Enters same credentials
```

Now Carol has access to all historical context from Alice and Bob.

---

## Step 5: Security Management

### 5.1 Token Rotation (Monthly)

Alice rotates the Vault token:

```bash
# Alice generates new token in Vault admin panel
# New token: evt_acme_xyz789new

# Alice emails team:
"Team, please update your Vault token to: evt_acme_xyz789new
Run: /rune configure (just update the token field)"

# Team members update:
/rune configure
# Only need to update vault token, other fields unchanged

# Alice revokes old token in Vault admin panel
```

### 5.2 Remove Team Member

If someone leaves the team:

```bash
# Alice rotates Vault token
# Sends new token only to current team members
# Old token is revoked -> departed member loses access
```

---

## Benefits Realized

### Context Continuity
- Bob doesn't need to ask Alice "Why PostgreSQL?"
- Carol onboards instantly with full historical context
- No knowledge loss when Alice is on vacation

### Zero-Knowledge Privacy
- enVector Cloud sees only encrypted vectors
- Only team members with Vault access can decrypt
- Cloud provider cannot read any conversations

### Team Collaboration
- All 3 developers share the same organizational memory
- Decisions captured once, available to everyone
- Consistent knowledge across the team

---

## Troubleshooting

### Bob can't connect

```
Bob: /rune status
Claude: Error: Cannot connect to Vault

Alice checks:
1. Vault is running: curl https://vault-acme.oci.envector.io/health
2. Bob's token is correct
3. Firewall allows Bob's IP

Issue: Bob had typo in Vault URL
Fix: Bob runs /rune configure with correct URL
```

### Carol sees encrypted results

```
Carol: "Why PostgreSQL?"
Claude: Returns encrypted gibberish

Issue: Carol's Vault token is incorrect
Fix: Alice verifies token, Carol runs /rune configure
```

---

## Advanced: Multiple Projects

### Separate Collections

Alice sets up separate collections for different projects:

```bash
# Project Alpha (confidential)
/rune configure
# Collection: "project-alpha-context"

# Project Beta (public)
/rune configure
# Collection: "project-beta-context"
```

Team members can switch between collections by reconfiguring.

---

## Summary

**Admin Tasks** (Alice):
1. Deploy Rune-Vault (one-time, 1 hour)
2. Sign up enVector Cloud (one-time, 10 minutes)
3. Onboard team members (per member, 5 minutes)
4. Rotate tokens (monthly, 10 minutes)

**Team Member Tasks** (Bob, Carol):
1. Install plugin (one-time, 2 minutes)
2. Configure credentials (one-time, 3 minutes)
3. Use naturally (ongoing, zero overhead)

**Result**: Fully encrypted, shared organizational memory with zero knowledge privacy.
