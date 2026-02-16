---
description: Clear Rune configuration and return to dormant state
allowed-tools: Bash(python3:*), Bash(rm ~/.rune/config.json), Bash(cat ~/.rune/*), Read
---

# /rune:reset — Reset Configuration

Clear all configuration and return to dormant state.

## Steps

1. **Confirm with user** before proceeding. Ask: "This will delete your Rune configuration. Continue?"

2. If confirmed:
   - Delete `~/.rune/config.json`
   - Respond: "Configuration cleared. Run `/rune:configure` to set up again."

3. If not confirmed:
   - Respond: "Reset cancelled."
