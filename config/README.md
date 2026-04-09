# Configuration Guide

This directory contains configuration templates for the Rune plugin.

## Configuration File

The Rune plugin stores configuration in:
```
~/.rune/config.json
```

## Configuration Structure

```json
{
  "vault": {
    "endpoint": "your-vault-host:50051",
    "token": "your-vault-token"
  },
  "state": "active"
}
```

> **Note**: enVector Cloud credentials (endpoint, API key) are delivered automatically via the Vault bundle at startup. You do not need to configure them manually.

## Fields

### `vault.endpoint` (required for shared memory)
Your team's Rune-Vault gRPC endpoint. This is the host:port where the Vault gRPC server is running.

**Example**: `vault-host:50051`

### `vault.token` (required for shared memory)
Authentication token for Vault access. Provided by your team administrator.

**Example**: `your-vault-token`

**Security**: Keep this secure. Anyone with this token can access your team's organizational memory.

> **Note**: enVector credentials and the index name for team shared memory are managed by the Vault admin and distributed automatically via the Vault bundle at startup. All tools (`capture`, `recall`) use the Vault-provided configuration.

### `state` (required)
Plugin activation state. Values:
- `"active"` - Full functionality enabled
- `"dormant"` - Waiting for configuration

### Optional Sections

The config file also supports optional sections for agent configuration:

```json
{
  "embedding": {
    "mode": "femb",
    "model": "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
  },
  "scribe": {
    "similarity_threshold": 0.35,
    "auto_capture_threshold": 0.7,
    "tier2_enabled": true
  },
  "retriever": {
    "topk": 10,
    "confidence_threshold": 0.5
  }
}
```

All optional sections have sensible defaults. The capture pipeline's LLM (Tier 2/3) automatically uses API keys inherited from the host agent's environment (e.g., `ANTHROPIC_API_KEY`). No manual LLM configuration is needed.

## Manual Configuration

If you prefer to configure manually instead of using `/rune:configure`:

1. **Create directory**:
   ```bash
   mkdir -p ~/.rune
   ```

2. **Copy template**:
   ```bash
   cp config.template.json ~/.rune/config.json
   ```

3. **Edit file**:
   ```bash
   nano ~/.rune/config.json
   # or use your preferred editor
   ```

4. **Replace placeholders**:
   - `vault.endpoint`: Your team's Vault gRPC endpoint
   - `vault.token`: Your Vault authentication token
   - `state`: Set to `"active"`

5. **Set permissions** (recommended):
   ```bash
   chmod 600 ~/.rune/config.json
   ```

6. **Verify**:
   ```
   /rune:status
   ```

## Environment Variables (Alternative)

You can also use environment variables instead of a config file:

```bash
# Vault (for shared team memory)
export RUNEVAULT_ENDPOINT="your-vault-host:50051"
export RUNEVAULT_TOKEN="your-vault-token"

# LLM for capture pipeline (optional — auto-detected from host agent environment)
# export RUNE_LLM_PROVIDER="anthropic"  # or "openai" or "google"
```

The plugin will check environment variables if `~/.rune/config.json` doesn't exist.

> **Note**: enVector credentials are delivered automatically via the Vault bundle. No enVector environment variables are needed.

## Team Configuration

### For Team Administrators

When onboarding team members, provide them with:

1. **Vault credentials** (same for all team members):
   - Vault gRPC endpoint
   - Vault Token

   enVector credentials are delivered automatically via the Vault bundle — no separate distribution needed.

2. **Optional: Pre-configured file**:
   ```bash
   # Generate pre-configured file for team member
   cat > alice-config.json << EOF
   {
     "vault": {
       "endpoint": "vault-host:50051",
       "token": "your-vault-token"
     },
     "state": "active"
   }
   EOF

   # Send to Alice
   # Alice installs: cp alice-config.json ~/.rune/config.json
   ```

### For Team Members

After receiving credentials from your admin:

1. **Option A: Interactive configuration**:
   ```
   /rune:configure
   ```
   Then enter provided credentials.

2. **Option B: Pre-configured file**:
   ```bash
   mkdir -p ~/.rune
   cp provided-config.json ~/.rune/config.json
   chmod 600 ~/.rune/config.json
   ```

## Security Best Practices

### File Permissions
```bash
# Restrict to user-only access
chmod 600 ~/.rune/config.json
```

### Token Rotation
Periodically rotate Vault tokens:
1. Admin generates new token in Vault
2. Admin distributes new token to team
3. Team members update `vault.token` in config
4. Admin revokes old token

### Backup
Back up your configuration:
```bash
cp ~/.rune/config.json ~/.rune/config.backup.json
```

## Troubleshooting

### "Cannot connect to Vault"
1. Check Vault Endpoint is correct
2. Verify Vault is running: `curl <vault-url>/health`
3. Check network connectivity
4. Verify token is valid

### "enVector authentication failed"
1. Check that your Vault is running and accessible (enVector credentials are delivered via Vault bundle)
2. Re-run `/rune:configure` to refresh credentials
3. Contact your team administrator if the issue persists

### "Permission denied"
```bash
chmod 600 ~/.rune/config.json
```

### Reset configuration
```
/rune:reset
```
Then reconfigure with `/rune:configure`.

## Support

- **Issues**: https://github.com/CryptoLabInc/rune/issues
- **Email**: zotanika@cryptolab.co.kr
- **Full docs**: https://github.com/CryptoLabInc/rune-admin
