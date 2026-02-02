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
    "url": "https://vault-YOURTEAM.oci.envector.io",
    "token": "evt_YOURTEAM_xxx"
  },
  "envector": {
    "endpoint": "https://cluster-xxx.envector.io",
    "api_key": "envector_xxx",
    "collection": "sealed-hive-context"
  },
  "state": "active"
}
```

## Fields

### `vault.url` (required)
Your team's Rune-Vault URL. Format: `https://vault-TEAM.{oci|aws|gcp}.envector.io`

**Example**: `https://vault-acme.oci.envector.io`

### `vault.token` (required)
Authentication token for Vault access. Format: `evt_xxx`

**Example**: `evt_acme_abc123def456`

**Security**: Keep this secure. Anyone with this token can access your team's organizational memory.

### `envector.endpoint` (required)
Your enVector Cloud cluster endpoint. Format: `https://cluster-xxx.envector.io`

**Example**: `https://cluster-us-west-2.envector.io`

### `envector.api_key` (required)
API key for enVector Cloud authentication. Format: `envector_xxx`

**Example**: `envector_abc123def456`

### `envector.collection` (optional)
Collection name for storing vectors. Default: `"sealed-hive-context"`

You may want to use different collections for:
- Different projects: `"project-alpha-context"`
- Different teams: `"team-frontend-context"`
- Different purposes: `"security-decisions"`

### `state` (required)
Plugin activation state. Values:
- `"active"` - Full functionality enabled
- `"dormant"` - Waiting for configuration

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
   - `vault.url`: Your team's Vault URL
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
export RUNE_VAULT_URL="https://vault-acme.oci.envector.io"
export RUNE_VAULT_TOKEN="evt_acme_xxx"
export ENVECTOR_ENDPOINT="https://cluster-xxx.envector.io"
export ENVECTOR_API_KEY="envector_xxx"
```

The plugin will check environment variables if `~/.rune/config.json` doesn't exist.

## Team Configuration

### For Team Administrators

When onboarding team members, provide them with:

1. **Vault credentials** (same for all team members):
   - Vault URL
   - Vault Token

2. **enVector credentials** (can be shared or individual):
   - Cluster endpoint
   - API key

3. **Optional: Pre-configured file**:
   ```bash
   # Generate pre-configured file for team member
   cat > alice-config.json << EOF
   {
     "vault": {
       "url": "https://vault-acme.oci.envector.io",
       "token": "evt_acme_xxx"
     },
     "envector": {
       "endpoint": "https://cluster-us-west-2.envector.io",
       "api_key": "envector_xxx",
       "collection": "sealed-hive-context"
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
1. Check Vault URL is correct
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
