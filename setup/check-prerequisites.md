# Rune Plugin - Prerequisites Check

Welcome! Before activating the Rune plugin, please ensure you have the following credentials from your team.

## Required: Rune-Vault Access

Your team administrator should have provided you with:

### 1. Vault Endpoint
**Format**: `vault-YOURTEAM.oci.envector.io:50051`

**Example**: `vault-acme.oci.envector.io:50051`

This is your team's shared encryption key vault. All team members connect to the same Vault to share organizational memory.

### 2. Vault Token
**Format**: `evt_YOURTEAM_xxx`

**Example**: `evt_acme_abc123def456`

This authenticates you to access your team's Vault. Keep this secure and never share it outside your team.

---

## enVector Cloud (Automatic)

enVector Cloud credentials (endpoint, API key) are delivered automatically via the Vault bundle at startup. You do not need to obtain or configure them separately. Your team administrator manages enVector setup as part of the Vault deployment.

---

## Prerequisites Check

Do you have both pieces of information ready (Vault Endpoint + Token)?

### ✅ Yes, I have everything
Great! Run `/rune:configure` to set up your credentials and activate the plugin.

### ⏸️ No, I'm missing some information

#### Missing Vault credentials?
**Contact your team administrator** who deployed the Rune-Vault infrastructure. They should provide:
- Vault Endpoint
- Vault Token

If your team hasn't deployed Rune-Vault yet, see the [full Rune deployment guide](https://github.com/CryptoLabInc/rune-admin).

#### enVector credentials?
enVector credentials are delivered automatically via the Vault bundle. If you see enVector errors, contact your team administrator to verify the Vault deployment includes enVector configuration.

---

## What happens after configuration?

Once configured with `/rune:configure`:

### Active State ✅
- **Automatic context capture**: Claude will automatically identify and store significant organizational decisions
- **Context retrieval**: Ask Claude about past decisions and get full context
- **Team sharing**: All team members with the same Vault see the same organizational memory
- **Zero-knowledge security**: enVector Cloud never sees plaintext data

### Example Usage

**Automatic capture (Scribe)** — the primary path, no commands needed:
```
You: "We decided to use PostgreSQL because it has better JSON support than MySQL"
Claude: [Scribe automatically detects and stores this decision in organizational memory]
```

**Automatic retrieval (Retriever)** — just ask naturally:
```
You: "Why did we choose PostgreSQL?"
Claude: According to organizational memory from 2 weeks ago:
"We decided to use PostgreSQL because it has better JSON support than MySQL"
```

**Manual override** — only when automatic capture missed something:
```
You: /rune:capture "All API endpoints must use JWT authentication"
Claude: Stored in organizational memory
```

---

## Security & Privacy

### What gets encrypted?
- All conversational context
- All organizational decisions
- All code patterns and rationale

### What can the cloud provider see?
- **Nothing**: All data is FHE-encrypted before leaving your machine
- Cloud only sees encrypted vectors (mathematical noise)
- Only your team's Vault can decrypt

### Who has access?
- **Team members**: Anyone with your Vault Endpoint + Token
- **Cloud provider**: No access (zero-knowledge encryption)
- **Admin control**: Revoke access by rotating Vault tokens

---

## Need Help?

- **Setup questions**: Contact your team administrator
- **Technical issues**: [GitHub Issues](https://github.com/CryptoLabInc/rune/issues)
- **Email support**: zotanika@cryptolab.co.kr

---

## Ready to proceed?

If you have all prerequisites, run:
```
/rune:configure
```

If you need to install Rune infrastructure for your team, see:
- **Full Rune Repository**: https://github.com/CryptoLabInc/rune
- **Deployment Guide**: https://github.com/CryptoLabInc/rune-admin/blob/main/deployment/README.md
