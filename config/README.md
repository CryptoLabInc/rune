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
  "envector": {
    "endpoint": "runestone-xxx.clusters.envector.io",
    "api_key": "your-api-key",
    "collection": "rune-context"
  },
  "state": "active"
}
```

## Fields

### `vault.endpoint` (required for shared memory)
Your team's Rune-Vault gRPC endpoint. This is the host:port where the Vault gRPC server is running.

**Example**: `vault-host:50051`

### `vault.token` (required for shared memory)
Authentication token for Vault access. Provided by your team administrator.

**Example**: `your-vault-token`

**Security**: Keep this secure. Anyone with this token can access your team's organizational memory.

### `envector.endpoint` (required)
Your enVector Cloud gRPC endpoint address.

**Example**: `runestone-xxx.clusters.envector.io`

### `envector.api_key` (required)
API key for enVector Cloud authentication.

**Example**: `your-envector-api-key`

### `envector.collection` (optional)
Collection name for organizing vectors. Default: `"rune-context"`

This maps to an `index_name` in the enVector SDK. When using MCP tools directly (`insert`, `search`, `remember`), use this value as the `index_name` parameter.

> **Note**: For team shared memory via the `remember` tool, the team index name is managed by the Vault admin and distributed automatically. The `collection` field is primarily used by Scribe/Retriever agents for their own operations.

You may want to use different collections for:
- Different projects: `"project-alpha-context"`
- Different teams: `"team-frontend-context"`
- Different purposes: `"security-decisions"`

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
    "slack_webhook_port": 8080,
    "similarity_threshold": 0.5,
    "auto_capture_threshold": 0.8,
    "tier2_enabled": true,
    "tier2_model": "claude-haiku-4-5-20251001"
  },
  "retriever": {
    "topk": 10,
    "confidence_threshold": 0.5,
    "anthropic_model": "claude-sonnet-4-20250514"
  }
}
```

All optional sections have sensible defaults. See `agents/common/config.py` for the full schema.

## Manual Configuration

If you prefer to configure manually instead of using `/rune configure`:

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
   - `envector.endpoint`: Your enVector cluster endpoint
   - `envector.api_key`: Your enVector API key
   - `state`: Set to `"active"`

5. **Set permissions** (recommended):
   ```bash
   chmod 600 ~/.rune/config.json
   ```

6. **Verify**:
   ```
   /rune status
   ```

## Environment Variables (Alternative)

You can also use environment variables instead of a config file:

```bash
# Vault (for shared team memory)
export RUNEVAULT_ENDPOINT="your-vault-host:50051"
export RUNEVAULT_TOKEN="your-vault-token"

# enVector Cloud
export ENVECTOR_ENDPOINT="runestone-xxx.clusters.envector.io"
export ENVECTOR_API_KEY="your-api-key"
```

The plugin will check environment variables if `~/.rune/config.json` doesn't exist.

> **Note**: The MCP server (`server.py`) uses `ENVECTOR_ADDRESS` for the endpoint, while the agent config (`config.py`) uses `ENVECTOR_ENDPOINT`. Both are supported -- set both if running agents and MCP server separately.

## Team Configuration

### For Team Administrators

When onboarding team members, provide them with:

1. **Vault credentials** (same for all team members):
   - Vault gRPC endpoint
   - Vault Token

2. **enVector credentials** (can be shared or individual):
   - Cluster endpoint (e.g., `runestone-xxx.clusters.envector.io`)
   - API key

3. **Optional: Pre-configured file**:
   ```bash
   # Generate pre-configured file for team member
   cat > alice-config.json << EOF
   {
     "vault": {
       "url": "vault-host:50051",
       "token": "your-vault-token"
     },
     "envector": {
       "endpoint": "cluster.envector.io:443",
       "api_key": "your-api-key",
       "collection": "rune-context"
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
   /rune configure
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

### Separate Collections
For sensitive projects, use separate collections:
```json
{
  "envector": {
    "collection": "confidential-project-alpha"
  }
}
```

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
1. Check API key is correct
2. Verify enVector account is active
3. Check cluster endpoint

### "Permission denied"
```bash
chmod 600 ~/.rune/config.json
```

### Reset configuration
```
/rune reset
```
Then reconfigure with `/rune configure`.

## Support

- **Issues**: https://github.com/CryptoLabInc/rune/issues
- **Email**: zotanika@cryptolab.co.kr
- **Full docs**: https://github.com/CryptoLabInc/rune-admin
