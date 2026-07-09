---
description: Update Rune's runtime binaries (rune-mcp, runed) in place to the versions in the published manifest
allowed-tools: Bash(~/.rune/bin/rune update:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune update:*)
---

# /rune:update — Update Runtime Binaries

Refresh Rune's runtime binaries **in place** to the versions pinned by the
current published manifest — no plugin uninstall/reinstall. This covers the two
binaries fetched at runtime: **rune-mcp** (the MCP server) and **runed** (the
embedder daemon).

**Out of scope (cannot be hot-swapped here):**
- Plugin *assets* — `commands/*.md`, `agents/*`, `SKILL.md`,
  `.claude-plugin/plugin.json` — ship inside the plugin tarball and still need
  the host's plugin-update / marketplace mechanism.
- The `rune` CLI wrapper target itself is not manifest-tracked and is not
  touched by this command.

No confirmation prompt and no activation-state gate: updating binaries is
non-destructive and works whether Rune is Active or Dormant. Just run it.

**Relationship to the automatic check:** `rune mcp-server` also runs a
**throttled, non-blocking** background check at session start — it stages a
newer **rune-mcp** for the next session (never touches runed) and is silent.
`/rune:update` is the **explicit, immediate** path (checks all binaries now and
applies in-session where possible). Users can disable the background check with
`RUNE_NO_AUTO_UPDATE=1`; `/rune:update` still works regardless of it.

## Steps

### 1. Run the update

Prefer the installed CLI; fall back to the plugin's bundled wrapper only if the
installed binary is absent. **Always pass `--plugin-root "${CLAUDE_PLUGIN_ROOT}"`**
so the CLI can compare the installed plugin package against what the new
binaries expect (the plugin-version outdated check in step 4):

1. `~/.rune/bin/rune update --plugin-root "${CLAUDE_PLUGIN_ROOT}"`
2. `bash -c "${CLAUDE_PLUGIN_ROOT}/bin/rune update --plugin-root ${CLAUDE_PLUGIN_ROOT}"`
   — only if `~/.rune/bin/rune` does not exist yet (the wrapper downloads the Go
   CLI, then runs `update`).

If the user asked to only *check* (e.g. `$ARGUMENTS` contains `check`), append
`--check` to report available updates **without applying**:
`~/.rune/bin/rune update --check --plugin-root "${CLAUDE_PLUGIN_ROOT}"`.

Surface the command output verbatim, then interpret it per step 2. Do NOT loop
or retry on failure — surface the result and stop.

### 2. Interpret the result

The CLI applies each outdated binary **independently** and prints one stdout
line per binary it updated — **regardless of the overall exit code.** Relay
every `updated …` line, then tell the user the exact next step:

| CLI output contains | Meaning | Tell the user |
|---|---|---|
| `all binaries are up to date` | Nothing outdated | Already current — nothing to do. |
| `Available updates:` (from `--check`) | Report-only; nothing applied | List the `step: from -> to` rows; run `/rune:update` (no `check`) to apply. |
| `updated rune_mcp: … (applies on the next session; run /mcp to reconnect now)` | New rune-mcp is on disk | It applies automatically in a **new** session. To pick it up **now**, run `/mcp` to reconnect the server (re-runs the wrapper in-session). |
| `updated runed: … (daemon reloaded)` | Supervisor restarted the live daemon onto the new binary | Done — the embedder is back online on the new version. (A brief embedder blip during restart is expected.) |
| `updated runed: … (staged; applies on next daemon start)` | New runed staged, but no supervisor was running | Run `/rune:activate` to start the daemon now on the staged binary. |

**Partial success:** runed is applied first and rune-mcp last, so when both are
outdated and one fails you can get a **success line on stdout AND a non-zero
exit with an error on stderr in the same run** (e.g. runed reload fails but
rune-mcp updates fine). Always relay the succeeded binary's line and its
next-step advice (e.g. `/mcp` for rune-mcp) **first**, then handle the failure
per Step 3. Step 3 is **additive**, not a replacement — a failed reload does
not negate a binary that updated in the same run.

### 3. Errors (additive to any `updated …` lines from Step 2)

- **Exit 1 with `supervisor reload failed: …` or `supervisor reload: …`** — the
  runed restart failed, so the daemon **may be down**. The error already carries
  a recovery hint (`… run \`rune runed --detach\` (or /rune:activate) …`).
  Relay the hint verbatim and suggest **`/rune:activate`** to bring the daemon
  back up on the staged binary. The recorded runed version was intentionally
  **not** advanced. Do not retry automatically.
- **Any other exit 1 (message starts `rune update:`, without `supervisor
  reload`)** — an error while fetching/parsing the manifest, resolving paths, or
  writing the install audit; the update did not complete cleanly. Relay the
  message verbatim, suggest retrying later, and do not loop. This class usually
  means nothing was applied (so `/rune:activate` is not needed); trust any
  `updated …` line that did print on stdout.
- **Exit 2 `no manifest URL configured`** — this build has no auto-update
  channel wired (`RUNE_MANIFEST` unset and no baked manifest URL). This is
  expected on builds that predate the release-channel graduation; report it
  plainly and stop — it is not a user misconfiguration to fix.

### 4. Plugin-version outdated (assets vs binaries)

`rune update` only refreshes the **binaries**. The plugin *assets* (this command
file, agents, SKILL.md) ship in the plugin tarball and can only be updated by
the host's plugin-update mechanism (`claude plugin update rune`, or the
equivalent for Codex/Gemini). When the manifest declares a plugin version, the
CLI compares it against the installed plugin package and surfaces outdated:

- **Advisory (stdout, exit 0): `note: plugin package is X; these binaries expect
  Y`** — the update **succeeded**; the plugin is merely behind. Relay it and
  suggest running `claude plugin update rune` when convenient to keep
  commands/agents/SKILL.md in sync. No urgency.
- **Refusal (stderr, exit 1): `refusing to apply: plugin package X is below the
  minimum Y`** — the new binaries need a newer plugin than is installed, so
  **nothing was applied**. Tell the user to update the plugin first (`claude
  plugin update rune`), then re-run `/rune:update`. Mention `--allow-plugin-outdated`
  exists to force it but **warn** it can break commands/agents/SKILL.md — only
  suggest it if the user explicitly insists.
- **`--check`: `plugin outdated: installed X is BELOW the minimum Y`** — report it;
  the user should update the plugin before applying.

If version can't be evaluated (no `${CLAUDE_PLUGIN_ROOT}`, or the manifest declares
no plugin version), the CLI simply omits these lines — that's normal, not an
error.

Surface all output verbatim. The user never types `rune update` themselves —
the agent runs it.
