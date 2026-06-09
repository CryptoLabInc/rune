---
description: Activate Rune (resume from dormant) and verify pipelines come up healthy
allowed-tools: Bash(~/.rune/bin/rune install:*), Bash(${CLAUDE_PLUGIN_ROOT}/bin/rune install:*), mcp__plugin_rune_rune__activate, mcp__plugin_rune_rune__diagnostics, mcp__plugin_rune_rune__vault_status
---

# /rune:activate — Activate Plugin

Resume Rune from a dormant state and verify the boot loop reaches Active.

The MCP server is a Go binary at `~/.rune/bin/rune-mcp`, spawned by Claude Code
via the plugin manifest's committed wrapper `${CLAUDE_PLUGIN_ROOT}/bin/rune
mcp-server` (always present at session start; self-installs rune-mcp on first
run). `mcp__plugin_rune_rune__activate` runs the prereq checks server-side (config presence
+ runed socket reachability, plus a runed Health probe) and only triggers the
boot loop if everything's ready.

## Preflight: the first MCP call self-installs

On a fresh `claude plugin install rune`, the first `mcp__plugin_rune_rune__*` call spawns
`${CLAUDE_PLUGIN_ROOT}/bin/rune mcp-server`, which self-installs rune-mcp and
then serves — so the call is EXPECTED to succeed in-session (may be slow on a
cold download, bounded by the manifest's spawn timeout; that is normal).

You normally do NOT need to run anything here. ONLY if a `mcp__plugin_rune_rune__*` call
actually fails with a transport / connection / spawn error (e.g. the server
shows failed in `/mcp`) — a genuinely broken bootstrap — recover by running
ONE of these via the Bash tool, then retry the failed MCP call once:

1. **`~/.rune/bin/rune install`** - canonical Go binary already exists.
2. **`bash -c "${CLAUDE_PLUGIN_ROOT}/bin/rune install"`** - when
   `~/.rune/bin/rune` doesn't exist yet (the bash wrapper downloads the Go
   binary, then installs).

Surface the install output verbatim. If the retry ALSO fails, surface the
error and stop — do NOT loop. The user never types `rune install` themselves.

## Steps

### 1. Call `mcp__plugin_rune_rune__activate`

That's it - no Read, no Edit, no manual state inspection. The MCP tool
performs:

- `config.Load()`: if missing or vault block empty, returns
  `status: "configure_required"` without touching the boot loop
- `os.Stat(~/.runed/embedding.sock)`: if absent, returns
  `status: "install_pending"`
- `Health` probe to runed: if it reports `STATUS_LOADING` (runed is
  self-bootstrapping - fetching llama-server / downloading the model),
  returns `status: "waiting_for_bootstrap"` with progress detail on
  `.bootstrap` and skips the boot loop
- Otherwise calls `reload_pipelines` and mirrors its result on `.reload`

The response shape:

```jsonc
{
  "ok": true,
  "status": "configure_required" | "install_pending" |
            "waiting_for_bootstrap" |
            "active" | "waiting_for_vault" | "dormant",
  "hint": "<actionable string when status is not active>",
  "bootstrap": {                              // only when status == "waiting_for_bootstrap"
    "phase": "FETCHING_LLAMA_SERVER" | "FETCHING_MODEL" | "STARTING_LLAMA_SERVER",
    "bytes_done":  <int64>,
    "bytes_total": <int64>,                   // 0 when total not yet known
    "message":     "<free-text detail>"
  },
  "reload": { ...ReloadPipelinesResult... }   // only when status reached the boot loop
}
```

### 2. Branch on `status`

**`configure_required`** - Vault credentials missing.
- Render: `"Rune is not yet configured. Run /rune:configure to set Vault credentials."`
- Use the `hint` verbatim - it already names the exact next step.
- Stop. Do NOT call `mcp__plugin_rune_rune__diagnostics`; the agent already has the answer.

**`install_pending`** - the activate handler tried to auto-spawn the runed
daemon (via `${HOME}/.rune/bin/rune runed --detach` or the
canonical equivalent) but couldn't make the socket reachable. The
response's `hint` field already contains the specific recovery - usually
the agent-facing form is `bash -c "${CLAUDE_PLUGIN_ROOT}/bin/rune install"`
(or `~/.rune/bin/rune install` once the CLI is installed).
- Render the `hint` verbatim to the user, then invoke the hint (recovery command).
- After install succeeds, retry `/rune:activate` once.
- Do NOT instruct the user to type the underlying `rune install`
  themselves - the agent runs it. Stop. Don't call diagnostics.

**`waiting_for_bootstrap`** - runed is up but still self-bootstrapping.
This is expected on the very first activate after a fresh install: runed
downloads llama-server (~25 MB) and the embedding model (~340 MB) before
it can serve embeddings.
- Render a single line summarizing `.bootstrap`. Pick the phrasing from `.bootstrap.phase`:
  - `FETCHING_LLAMA_SERVER`: "runed is downloading llama-server"
  - `FETCHING_MODEL`: "runed is downloading the embedding model"
  - `STARTING_LLAMA_SERVER`: "runed is starting llama-server"
- If `.bootstrap.bytes_total > 0`, append the progress in MB:
  `"<phrase> (<done_mb> MB / <total_mb> MB)"`. Otherwise just the phrase
- Append `.bootstrap.message` verbatim on a second line when non-empty
- Tell the user: **no further action is needed.** The MCP server has
  already started a background watcher that polls runed's health and
  will complete activation.
  Suggest:
  - To check progress (optional): invoke `/rune:status`
    and check `embedding.phase` / `embedding.bytes_done` /
    `embedding.bytes_total`. The bootstrap fields are populated there
    while runed is `LOADING`; once they go to zero and `embedding.status`
    is `OK`, activation has auto-completed.
  - To start using Rune as soon as it's ready: just invoke
    `/rune:recall` or `/rune:capture` - they will succeed once the
    watcher has reached Active
- Stop. DO NOT poll automatically; DO NOT re-run `/rune:activate`.

**`active`** — happy path. Pipelines initialized, ready to capture/recall.
- Optionally call `mcp__plugin_rune_rune__diagnostics` ONCE to render the
  per-subsystem summary below. Skip if you only need to confirm activation;
  `response.reload.state == "active"` is authoritative.

**`waiting_for_vault`** or **`dormant`** - boot loop ran but didn't reach Active.
- Fast-fail. Look at `response.reload.last_boot_error` — the boot loop has
  already classified the root cause.
- Render the recovery as **one block**: the `hint` from
  `last_boot_error.hint` verbatim, then a single re-run suggestion
  (`/rune:configure` for credential issues, `/rune:activate` after fixing
  substrate). Do NOT retry, do NOT shell-probe with openssl/nc/etc.
- Per-`kind` reference table lives in `commands/claude/configure.md` §5
  for the rare case the hint needs supplementation.

### 3. Render success snapshot (active path only)

When `status == "active"`, call `mcp__plugin_rune_rune__diagnostics` once and
render the per-subsystem report (use ✓ for healthy, ✗ for failures with
the specific error on the same line):

```
Infrastructure Validation
=========================
  Vault           : ✓ reachable (<endpoint>)
  Encryption Key  : ✓ loaded (key_id: <id>)
  Agent DEK       : ✓ loaded
  Scribe          : ✓ initialized
  Retriever       : ✓ initialized
  Embedder        : ✓ <model> (dim=<vector_dim>)
  enVector Cloud  : ✓ reachable (<latency>ms)
  Pipeline State  : Active
```

Then: `"Rune activated. Organizational memory is now online."`

---

**Notes:**
- This is a session-local resume - the MCP server stays the same process.
  No Claude Code restart required (the reload handler re-spawns the boot
  loop on dormant to active transitions).
- For an older rune-mcp binary without the `activate` tool, fall back to
  the legacy flow: Read config, call `reload_pipelines` directly, and branch
  on `diagnostics.vault.last_boot_error`. The SDK will surface a
  `method-not-found` error to signal the missing tool.
