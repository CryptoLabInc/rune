# Rune
**Encrypted shared memory for AI agents.**

Rune gives every AI agent on your team the **collective experience** of the entire organization — automatically, privately, and without anyone searching for it.

```
  Without Rune                         With Rune
  ━━━━━━━━━━━━                        ━━━━━━━━━

  Developer: "Should we use MongoDB?"   Developer: "Should we use MongoDB?"

  Agent: "MongoDB is great for          Agent: "Your team chose PostgreSQL
         flexible schemas..."                   over MongoDB in March. ACID
                                                compliance was non-negotiable
  (generic advice, no team context)             for transaction integrity."

  Two weeks later: "Wait, the team      The developer never searched.
  already rejected MongoDB??"           The agent just knew.
```

Works with **Claude Code, Codex CLI, Gemini CLI**, and any MCP-compatible agent.

---

## Quick Start

### Install

**Claude Code:**
```bash
# From terminal (local clone)
$ claude plugin marketplace add ./
$ claude plugin install rune

# From inside a Claude Code session (remote)
> /plugin marketplace add https://github.com/CryptoLabInc/rune
> /plugin install rune
```

**Gemini CLI:**
```bash
# From terminal
$ gemini extensions install https://github.com/CryptoLabInc/rune.git
```

**Codex CLI:**
```bash
# From inside a Codex session
> $skill-installer install https://github.com/CryptoLabInc/rune.git
```

### Configure

```
> /rune:configure        # Claude Code, Gemini CLI
> $rune configure        # Codex CLI
```

You'll need from your team admin:
- **Vault endpoint** + **token**

That's all. enVector Cloud credentials are delivered automatically via the Vault bundle.

Don't have these? See [rune-admin](https://github.com/CryptoLabInc/rune-admin) for deployment, or [examples/team-setup-example.md](examples/team-setup-example.md) for a walkthrough.

### That's It

Once configured, Rune works automatically. No commands to memorize. No queries to write. Just work with your agent as usual.

Your agent will:
- **Capture** significant decisions, trade-offs, and lessons during your work
- **Recall** relevant team knowledge when it matters — without being asked

---

## What This Looks Like

**Debugging that saves 4 hours:**
```
Dev: "The payment service is timing out"

Agent: "A similar timeout was investigated last month — root
       cause was connection pool exhaustion under concurrent
       webhook processing. Fix was async batch processing
       with a 50-connection pool limit."

Knowledge from a teammate's past session, surfaced automatically.
```

**Onboarding that just works:**
```
New hire: "I'm thinking of using Redis for the job queue"

Agent: "Your team evaluated Redis vs SQS for the job queue
       in February. SQS was chosen because Redis required
       dedicated ops capacity the team doesn't have."

The new hire's agent already has institutional knowledge
they haven't been taught yet.
```

You don't "query" Rune. Your agent draws from it the way an experienced engineer draws from years of past projects — the relevant context just surfaces.

---

## How Rune Is Different

| Approach | Limitation | Rune |
|----------|-----------|------|
| **Built-in memory** (Claude, ChatGPT, Copilot) | Siloed per vendor. Your team's Claude memory and Codex memory never connect. | One shared memory across all agents. Vendor-independent. |
| **RAG pipelines** | Chunks documents into fragments. Destroys reasoning structure. Requires ongoing pipeline maintenance. | Agent judges significance and stores *decisions*, not document chunks. No pipeline to maintain. |
| **Wikis & docs** | Manual. Nobody updates the wiki after the meeting. | Captures automatically during work, not after. |
| **Plaintext vector DBs** | Your organizational knowledge is readable by the cloud provider. | FHE encryption — the cloud stores and searches *only ciphertext*. Mathematically guaranteed. |

---

## Architecture

```
  Agent Swarm (your team)              Cloud Infrastructure
  ━━━━━━━━━━━━━━━━━━━━━━              ━━━━━━━━━━━━━━━━━━━━

  Alice's Agent ─┐
  Bob's Agent ───┤── MCP ──► enVector Cloud (encrypted vectors)
  Carol's Agent ─┘               │
                            Rune-Vault (secret key holder)
                            decrypts similarity scores only
```

**Capture:** Agent judges significance → generates reusable insight → novelty check against existing memory → FHE encrypt → store

**Recall:** Semantic query → encrypted similarity scoring → Vault decrypts scores only → metadata retrieved and decrypted locally

### Privacy: Zero-Knowledge Encryption

Every memory is encrypted **before leaving your machine** using Fully Homomorphic Encryption (FHE).

- **enVector Cloud** stores and searches **only encrypted vectors** — it cannot read your data
- **Rune-Vault** holds the secret key and decrypts **only similarity scores** — it never sees the content
- **Plaintext never leaves your machine**

Even if the cloud is compromised, your organizational knowledge remains mathematically protected.

---

## MCP Tools

| Tool | What It Does |
|------|-------------|
| `capture` | Store a decision in encrypted team memory |
| `recall` | Search team memory semantically |
| `batch_capture` | Bulk-capture multiple decisions (session-end sweep) |
| `vault_status` | Check Vault connection and security mode |
| `diagnostics` | System health check |
| `reload_pipelines` | Re-read config and reinitialize |
| `capture_history` | View recent captures |
| `delete_capture` | Soft-delete a record |

See [SKILL.md](SKILL.md) for the full reference and agent integration protocol.

---

## How The Capture Pipeline Works

Rune's capture system is modeled on how the brain forms long-term memories:

```
  EXPERIENCE                HIPPOCAMPUS               LONG-TERM MEMORY
  ━━━━━━━━━━               ━━━━━━━━━━━               ━━━━━━━━━━━━━━━━

  Full conversation   ──►   Agent judges:     ──►    Stores the GIST:
  with all the              "Is this significant?"
  tangents, greetings,                                "PostgreSQL for
  weather chat...           enVector checks:          financial data.
                            "Is this novel?"          ACID required.
                                                      MongoDB rejected."
                            Filters ~99% out.
                            Keeps only the insight.   Not the conversation.
                                                      The INSIGHT.
```

| Brain | Rune |
|-------|------|
| Prefrontal cortex judges significance | Agent evaluates decisions using full context |
| Hippocampus detects novelty | Embedding similarity check against existing memories |
| Gist extraction (verbatim fades, meaning persists) | Agent writes a `reusable_insight` — a dense NL paragraph |
| Consolidation (sleep filters and stores) | Capture pipeline encrypts and stores only novel insights |
| Associative recall (cue → memory surfaces) | Semantic search on encrypted vectors |

The memory itself acts as the filter. An empty memory captures aggressively (everything is novel). A rich memory becomes selective (most things are already known). **The filter improves as the memory grows.**

---

## For Team Administrators

Rune requires two infrastructure components:

1. **Rune-Vault** — Holds the team's secret key. Decrypts only similarity scores, never content. Deploy via [rune-admin](https://github.com/CryptoLabInc/rune-admin).
2. **enVector Cloud** — Encrypted vector storage and search. Sign up at [envector.io](https://envector.io).

### Deploying

See [rune-admin](https://github.com/CryptoLabInc/rune-admin):
1. Deploy Rune-Vault (OCI/AWS/GCP via Terraform)
2. Create enVector Cloud account and cluster
3. Provision team index on Vault

### Onboarding Members

Give each member their **Vault endpoint + token**. enVector credentials are bundled automatically.

They install the plugin, run `/rune:configure` (or `$rune configure` in Codex), and they're connected.

### Security

- **Token rotation**: New token → distribute → revoke old. Departed members lose access immediately.
- **Project isolation**: Separate Vault instances per project for isolated memory spaces.

---

## Configuration

`~/.rune/config.json`:

```json
{
  "vault": {
    "endpoint": "tcp://vault-myteam.oci.envector.io:50051",
    "token": "your-vault-token",
    "ca_cert": "",
    "tls_disable": false
  },
  "state": "active"
}
```

| State | Behavior |
|-------|----------|
| **Active** | Full functionality — capture and recall enabled |
| **Dormant** | No network requests — shows setup instructions |

---

## Upgrading & Uninstalling

Agent CLIs do not yet support in-place plugin upgrades. To upgrade, uninstall first and reinstall.

### Uninstall

**Claude Code:**
```bash
$ claude plugin remove --scope user rune
$ claude plugin marketplace remove cryptolab
$ rm -rf ~/.claude/plugins/cache/cryptolab   # remove plugin cache
```

**Codex CLI:**
```bash
$ codex mcp remove rune                      # deregister MCP server

```

**Gemini CLI:**
```bash
$ gemini extensions uninstall rune           # remove extension
```

To also remove local configuration and keys:
```bash
$ rm -rf ~/.rune
```

Then reinstall from the [Install](#install) section above.

---

## Troubleshooting

```
/rune:status              # or: $rune status
```

```bash
# Check infrastructure
cd rune && ./scripts/check-infrastructure.sh

# Reset and reconfigure
/rune:reset               # or: $rune reset
/rune:configure           # or: $rune configure
```

## Related Projects

- [Rune-Admin](https://github.com/CryptoLabInc/rune-admin) — Infrastructure deployment and admin tools
- [pyenvector](https://socket.dev/pypi/package/pyenvector) — FHE encryption SDK
- [enVector Cloud](https://envector.io) — Encrypted vector database

## Support

- **Issues**: [GitHub Issues](https://github.com/CryptoLabInc/rune/issues)
- **Docs**: [Full Documentation](https://github.com/CryptoLabInc/rune-admin/tree/main/docs)
- **Email**: zotanika@cryptolab.co.kr

## License

Apache License 2.0 — See [LICENSE](LICENSE)

---

Built by [CryptoLab](https://github.com/CryptoLabInc) — where FHE meets AI agent memory.
