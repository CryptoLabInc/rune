# mcp-tools-impl Python Parity Audit (Tasks 1 + 2)

> Run on `couragehong/feat/mcp-tools-impl` HEAD `c52b814`
> against `mcp/server/server.py` (~2002 LoC).
> Spec: `docs/v04/spec/components/rune-mcp.md`,
>       `docs/v04/overview/architecture.md §Scope`.

## Verdict

**Pass on the agent-delegated path that v0.4 targets**. Tool catalog,
state-gating, validation, and error-shape match. Two intentional Go
divergences: `rune_` prefix on tool names (matches docs/spec, not Python),
and atomic state machine instead of asyncio Event for readiness gating.

## Tool catalog parity

| Python tool name | Go tool name | Gated? |
|---|---|---|
| `capture` (server.py:L688) | `rune_capture` | ✅ both gated |
| `batch_capture` (L810) | `rune_batch_capture` | ✅ both gated |
| `recall` (L901) | `rune_recall` | ✅ both gated |
| `delete_capture` (L1115) | `rune_delete_capture` | ✅ both gated |
| `capture_history` (L1093) | `rune_capture_history` | ✅ both bypass |
| `vault_status` (L492) | `rune_vault_status` | ✅ both bypass |
| `diagnostics` (L532) | `rune_diagnostics` | ✅ both bypass |
| `reload_pipelines` (L1038) | `rune_reload_pipelines` | ✅ both bypass |

**Naming divergence**: Python uses bare names (`capture`, `recall`, …); Go
uses `rune_` prefix. The spec `docs/v04/spec/components/rune-mcp.md` L124
shows the Go form (`rune_capture`) in the SDK code sample but section
headers (L161 `tool_capture`, L173 `tool_recall`) cite the Python name.
**Decision**: keep `rune_` prefix (better tool namespacing in MCP — many
servers may expose a `capture` tool, only one a `rune_capture`). Document
this in spec as intentional. Track as a doc consistency followup.

## Behavior parity

### State gate (Python `_ensure_pipelines` ↔ Go `CheckState`)

| Aspect | Python (server.py:L1503-1518) | Go (state.go:CheckState) |
|---|---|---|
| Mechanism | `Event.wait(timeout=120s)` blocks until `_pipelines_ready` set | atomic state machine via `lifecycle.Manager.Current()` |
| State branches | (init not done) / (init failed) | starting / waiting_for_vault / dormant / active |
| Recovery hints | static ("embedding model may still be downloading", "Run /rune:activate") | state-specific ("Wait 1-2 seconds", "Run /rune:vault_status", "Run /rune:activate") |
| Error code | `PipelineNotReadyError` → `code="PIPELINE_NOT_READY"` | `domain.ErrPipelineNotReady` (same code string) |

Python has finer-grained timeout logic (block for up to 120 s); Go just
returns immediately with current state. **Different paradigms, same
contract from the agent's perspective** — both surface `PIPELINE_NOT_READY`
when the pipeline isn't usable. Go is simpler and doesn't tie up a
goroutine waiting for boot.

### Input validation

| Tool | Python check | Go check |
|---|---|---|
| capture | `parse_llm_json(extracted)` returns None → "Invalid extracted JSON" (L1240-1242) | `req.Extracted == nil` → ErrInvalidInput (state.go ValidateCaptureRequest) |
| capture | text trim — implicit (Python doesn't enforce; rejects later) | `strings.TrimSpace(req.Text) == ""` → ErrInvalidInput (Go is **stricter** — fail-fast) |
| capture | tier2.capture=false rejection (L1246-1251) | done in `domain.ParseExtractionFromAgent` (extraction.go) |
| recall | topk > 10 → InvalidInputError (L930-931) | `args.TopK > 10` → ErrInvalidInput (state.go ValidateRecallArgs) |
| recall | (topk default 5 in signature) | `args.TopK == 0 → 5` (state.go) |
| recall | (no empty-query check) | `strings.TrimSpace(args.Query) == ""` → ErrInvalidInput (Go **stricter**) |

Two Go-stricter checks (capture text empty, recall query empty) — both
defensive additions, match D24 spec early-reject intent.

### Error response shape

Python `make_error` (server.py-imported from `mcp/server/errors.py`):
```python
{
    "ok": false,
    "error": {
        "code": "VAULT_CONNECTION_ERROR",
        "message": "...",
        "retryable": true,
        "recovery_hint": "..."  # optional
    }
}
```

Go `domain.MakeError` (errors.go:L48-72) produces the **identical** map
shape, then `internal/mcp/result.go errorResult()` JSON-marshals it into
`TextContent` with `IsError: true`.

✅ Wire-shape bit-identical.

### Agent-delegated mode (v0.4 architecture-level decision)

Python `tool_capture` has TWO modes (server.py:L737-775):

```python
# ===== PRIMARY: Agent-delegated mode =====
if extracted is not None:
    return await self._capture_single(...)

# ===== FALLBACK: Legacy 3-tier pipeline (requires API keys) =====
# Retained for backward compatibility.
```

v0.4 architecture (`docs/v04/overview/architecture.md §Scope`) commits to
**agent-delegated only** (D14/D21/D28). Go drops the legacy fallback —
`extracted=None` is treated as a hard `ExtractionMissing` error rather
than triggering server-side LLM extraction.

✅ This is the intentional v0.4 narrowing of scope.

## Specific Go-only improvements

1. **Type-safe handler signatures**. Python tool decorator forwards args
   through `Annotated[T, Field(description=...)]` Pydantic; Go uses
   `ToolHandlerFor[In, Out]` with concrete struct types. Compile-time
   schema inference vs runtime introspection.
2. **Compile-time tool name validation** (`mustAdd` panic-guard). The
   official Go SDK only logs on bad names; Python's FastMCP enforces.
   `mustAdd` brings Go up to Python's safety level.
3. **Nil-safe `CheckState`**. `Deps.State == nil` returns
   `PIPELINE_NOT_READY` with hint instead of panicking — useful for tests
   and during boot before Manager is constructed.
4. **State-specific recovery hints**. Python uses a single static hint
   ("Run /rune:activate"); Go branches by state (starting / waiting_for_vault
   / dormant) for more actionable messages.

## Acceptable divergences (⚠️)

1. **Tool name prefix**. `rune_*` (Go) vs bare names (Python). See above.
2. **Capture path 1-mode**. Drops Python's legacy 3-tier fallback. Per D14.
3. **Validation strictness**. Go rejects empty text/query early; Python lets
   them flow further before failing. Strict-fail is friendlier.

## Open gaps (⚠️ should follow up)

### 1. **State.go validation does NOT cover phases / title truncation / confidence clamp**

Python (server.py:L1271-1280, L1252-1261) does these inline in `_capture_single`:
- `phases_data[:7]` (max 7 phases)
- `phase_title[:60]` (max 60 chars)
- `confidence = max(0.0, min(1.0, ...))` clamp

Go has helpers (`TruncateTitle`, `ClampConfidence` in state.go) but they
are NOT called from any handler — they are referenced in
`docs/v04/spec/components/rune-mcp.md §state.go` but actual call sites are
in `internal/domain/extraction.go` `ParseExtractionFromAgent`. Need to
verify the parser does these consistently. (Out of mcp-tools-impl scope —
domain/extraction.go is in #96.)

### 2. **`_maybe_reload_for_auto_provider`** is dropped (D31, archived)

Python L484-488 optionally reloads pipelines based on MCP clientInfo header.
v0.4 explicitly drops this (D31). Go has no equivalent. ✅ Intentional.

### 3. **`request_id` propagation**

Python adds context-bound request IDs via the FastMCP context arg. Go's
handlers don't yet thread `request_id` through. Spec promises slog +
request_id for observability (`rune-mcp.md §Observability`). Track as a
follow-up.

### 4. **Boot loop is a stub**

`internal/lifecycle/boot.go` `RunBootLoop` body is `_ = m`. Until it dials
Vault and populates services with adapter clients, write tools always fail
with `PIPELINE_NOT_READY`. End-to-end behavior requires:
- vault-grpc PR (this is its #98 partner)
- runed-grpc PR (#99 partner)
- Boot loop body (separate followup)

**This is the largest gap** — flagged for production-readiness audit (Task #5).

### 5. **`buildDeps` does NOT wire State into `RecallService`**

`RecallService` does not have a `State` field per spec
(`internal/service/recall.go`). It uses `s.Vault` etc. but doesn't gate on
state internally — relies on the handler's CheckState. ✅ Working as
intended; just noting.

### 6. **`CaptureService.Now` clock injection**

Python uses `datetime.now(timezone.utc)` directly. Go injects `Now func()
time.Time` for testability. ✅ Go improvement.

## Test coverage status

`go test ./internal/mcp/` covers:
- `TestRegister_All8ToolsListed` — catalog presence + alphabetical order
- `TestRegister_SchemasInferred` — input/output schema inference per tool
- `TestRegister_WriteToolsGated` — 4 write tools surface PIPELINE_NOT_READY in starting state
- `TestIsValidToolName`, `TestMustAdd_PanicsOnInvalidName` — name validation
- `TestRegister_AllHardcodedNamesValid` — Register doesn't panic on any name

Gaps:
- read-only tool happy-paths (vault_status nil-Vault, diagnostics, capture_history)
- error result wrapping fidelity (errorResult preserves RuneError code/retryable/hint)
- okResult JSON-marshal correctness

Track these as Task #8.
