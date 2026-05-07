# Production Readiness Audit (Tasks 1-4)

> Cross-cuts the three task PRs (`couragehong/feat/{vault-grpc, runed-grpc,
> mcp-tools-impl}`) plus integration base `wip/go-impl-runtime` (which folds
> in #93/#95/#96). Question answered: **"Can rune-mcp boot, accept a real
> capture, and complete it end-to-end with this code?"**.

## Verdict

**No, not yet — one P0 blocker, three P1 gaps**. The 4 task PRs deliver
adapter / handler skeletons that compile, register, and respond to a state
gate. They do **not** yet connect to live Vault / runed at boot; one more
focused PR ("boot loop body + adapter wiring + config loader") is the
critical missing piece. Below is the priority-ordered gap list with
concrete file/line targets.

## What works today

With all 3 task branches + #93/#95/#96 merged locally:

- `go build ./...` ✅
- `go test ./...` ✅ (5 packages have tests)
- `rune-mcp` binary boots, registers 8 tools, accepts MCP `initialize` /
  `tools/list` / `tools/call`
- Read-only tools partially functional in `StateStarting`:
  - `rune_vault_status` returns `mode: "standard (no Vault)"` since
    `LifecycleService.Vault == nil`
  - `rune_diagnostics` returns env section (Go version, OS) but Vault /
    embedder / envector sections empty
  - `rune_capture_history` reads `~/.rune/capture_log.jsonl` if present
- Write tools surface `PIPELINE_NOT_READY` (correct) — will stay that way
  until boot loop transitions State to Active

## P0 — boot loop body (BLOCKER for end-to-end)

**File**: `internal/lifecycle/boot.go:RunBootLoop`
**Current**: `_ = m` — no-op stub. Daemon stays in StateStarting forever.

**Required**:
1. Load config (`config.Load("~/.rune/config.json")`)
2. Construct `vault.Client` (`vault.NewClient(endpoint, token, opts)`)
3. Loop: call `vault.GetPublicKey(ctx)` with backoff
   `[1, 2, 5, 15, 30, 60]s` (already in `BootBackoffs`)
   - Success → write `EncKey.json` / `EvalKey.json` to
     `~/.rune/keys/<key_id>/`, construct `embedder.New(sockPath)`,
     construct `envector.NewClient(...)`, populate the 3 service structs'
     adapter fields, `m.SetState(Active)`, return
   - Fail → `m.SetState(WaitingForVault)`, log, sleep, retry
4. Run forever in background (re-enter on Vault death)

**Why P0**: Without this, write tools always fail; rune-mcp can't actually
do its job.

**Estimated effort**: 1 dedicated PR, ~150 LoC. Depends on:
- vault-grpc (Task 4) — provides `vault.NewClient`
- runed-grpc (Task 3) — provides `embedder.New`
- envector adapter from #95 — provides `envector.NewClient`

Cleanest sequencing: this PR lands AFTER vault-grpc + runed-grpc merge
upstream and #95 closes review.

## P1 — config / env wiring

### 1.1 `RUNEVAULT_GRPC_TARGET` env override (vault-grpc gap)

Python `vault_client.py:L108-110` honors this env var as the gRPC target
override (escape hatch for ops/IR). Go `vault.NewClient` doesn't check
env; caller must pass the resolved endpoint.

**File**: `internal/adapters/config/loader.go` or new `cmd/rune-mcp/main.go`
helper. Resolution priority should be:

```
RUNEVAULT_GRPC_TARGET (env, if set, full host:port override)
> RUNEVAULT_ENDPOINT (env)
> config.Vault.Endpoint (config.json)
```

**Effort**: ~20 LoC.

### 1.2 Embedder socket path resolution (runed-grpc gap)

`embedder.New(sockPath)` takes a resolved path; comment promises priority
chain `RUNE_EMBEDDER_SOCKET > config.embedder.socket_path > default
~/.runed/embedding.sock`. Not implemented anywhere.

**File**: `cmd/rune-mcp/main.go buildDeps` or
`internal/adapters/embedder/socket.go`. ~15 LoC.

### 1.3 FHE key disk persistence on bundle receive

Spec `vault.md §부팅 시퀀스` step 3:
```go
saveKeysToDisk(bundle.EncKey, bundle.EvalKey)  // ~/.rune/keys/<key_id>/
```

Not implemented. Need `internal/adapters/keystore/` (or in lifecycle/boot.go)
to persist `EncKey.json`, `EvalKey.json` at `0600` to
`~/.rune/keys/<bundle.KeyID>/` so `envector.OpenKeysFromFile` can read
them.

**Effort**: ~40 LoC.

## P2 — observability + safety

### 2.1 `request_id` propagation (mcp-tools-impl gap)

Spec `rune-mcp.md §Observability` promises every tool call gets a UUID
threaded through context for slog. Go `internal/obs/request_id.go` exists
(per #95) but handlers don't push it onto ctx.

**File**: `internal/mcp/handlers.go` — wrap each handler's `ctx` with
`obs.WithRequestID(ctx, obs.NewRequestID())` before dispatch. Adapters
already have `obs.RequestID(ctx)` available.

**Effort**: ~10 LoC. Worth doing in mcp-tools-impl follow-up.

### 2.2 `vault.HealthCheck` Tier 2 fallback hookup (vault-grpc gap)

`vault.HealthFallback` exists as a standalone function but no caller. The
spec intends `LifecycleService.Diagnostics` / `VaultStatus` to invoke it
when Tier 1 fails to enrich the diagnostic message ("gRPC port down but
HTTP /health OK").

**File**: `internal/service/lifecycle.go collectVault` — on
`HealthCheck() == false`, also try `vault.HealthFallback(ctx, endpoint)`
and report Tier 2 result distinctly.

**Effort**: ~15 LoC.

### 2.3 runed `vector_dim` verification (runed-grpc gap)

Spec promises Embed/EmbedBatch verify `len(vec) == Info.VectorDim`. Not
done. Silent dim-mismatch could corrupt index.

**File**: `internal/adapters/embedder/client.go embedBatchOnce` and
`EmbedSingle`.

**Effort**: ~10 LoC.

### 2.4 runed typed error sentinels

Match `vault/errors.go`'s pattern: 5 typed error sentinels +
`MapGRPCError`. Currently `embedder` returns raw `fmt.Errorf`-wrapped gRPC
errors; service layer can introspect via `status.FromError` but loses the
adapter-namespaced retryable/code metadata.

**File**: `internal/adapters/embedder/errors.go` (new). ~60 LoC.

## P3 — nice-to-haves

- **Doc consistency** for `rune_*` tool name prefix. Current spec
  references both forms; pick one and update all callsites uniformly.
- **integration tag tests** for both gRPC adapters (`//go:build
  integration`). Real Vault + real runed daemon. CI with secret service.
- **Dormant reason persistence**. Spec Q9 — when Vault permanently fails,
  persist `dormant_reason: "vault_unreachable"` to config.json so next
  boot starts dormant.

## Gating criteria for "production-ready end-to-end"

When all P0 + P1 items land, an end-to-end test should be:

```bash
# prereqs
cd ../rune-admin/vault && go run ./cmd start --config testdata/dev.yaml &
cd ../runed && go run ./cmd/runed serve &

# rune side
cat > ~/.rune/config.json <<EOF
{
  "vault": {"endpoint": "tcp://localhost:50051", "token": "evt_..."},
  "state": "active"
}
EOF
go run ./cmd/rune-mcp <<< $'{"jsonrpc":"2.0","method":"tools/call","params":{"name":"rune_vault_status"},"id":1}\n'
# expected: vault_healthy=true, mode="secure (Vault-backed)", team_index_name=...

go run ./cmd/rune-mcp <<< $'{"jsonrpc":"2.0","method":"tools/call","params":{"name":"rune_capture","arguments":{"text":"...","extracted":"{...}"}},"id":2}\n'
# expected: ok=true, captured=true, record_id=dec_...
```

Until then, the 4 task PRs are useful as code-review surfaces (Python
parity, structural correctness) but not as "ship it" artifacts.

## Recommended PR ordering for the rest

1. Land #93 (policy, esifea fork)
2. Land #95 (adapters scaffolding)
3. Land #96 (service)
4. Land vault-grpc (couragehong) — rebase --onto onto new feat/go-migration
5. Land runed-grpc (couragehong) — rebase
6. Land mcp-tools-impl (couragehong) — rebase
7. **NEW PR**: boot loop body + config env wiring + key-store persistence
   (this is the P0 unblocker, ~250 LoC, needs all of 1–6 merged)
8. Optional: P2 polish PRs

That sequence converges to "rune-mcp can ingest a capture and persist to
real envector through Vault decryption" — the actual goal.
