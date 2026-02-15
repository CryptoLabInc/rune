---
description: Configure Rune team credentials (Vault Endpoint, enVector endpoint)
allowed-tools: Bash(python3:*), Bash(cat ~/.rune/*), Bash(mkdir:*), Bash(chmod:*), Bash(scripts/*), Read, Write
---

# /rune:configure — Configure Team Credentials

Collect credentials and write `~/.rune/config.json`.

## Steps

1. Ask user for each credential:
   - **Vault Endpoint** (format: `vault-TEAM.oci.envector.io:50051`)
   - **Vault Token** (format: `evt_xxx`)
   - **enVector Endpoint** (format: `https://cluster-xxx.envector.io`)
   - **enVector API Key** (format: `envector_xxx`)

2. Validate infrastructure by running `scripts/check-infrastructure.sh` from the plugin root.
   - If validation fails: set `state: "dormant"`, warn user
   - If validation passes: set `state: "dormant"` (user must run `/rune:activate` explicitly)

3. Write `~/.rune/config.json`:
   ```json
   {
     "vault": {"endpoint": "...", "token": "..."},
     "envector": {"endpoint": "...", "api_key": "...", "collection": "rune-context"},
     "state": "dormant",
     "metadata": {"configVersion": "1.0", "lastUpdated": "..."}
   }
   ```

4. Set file permissions: `chmod 600 ~/.rune/config.json`

5. Show confirmation and suggest: "Run `/rune:activate` to enable the plugin."
