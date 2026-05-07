# Runed-grpc Spec Parity Audit (Task 3)

> Run on `couragehong/feat/runed-grpc` HEAD `309e76d`.
> Spec: `docs/v04/spec/components/embedder.md`.
> No Python equivalent — D30 designates runed as a Go-native sibling daemon
> replacing the in-process Python embedding pipeline. Audit baseline is the
> spec doc + the runed daemon's actual proto contract
> (`runed/proto/runed/v1/runed.proto`).

## Verdict

**Pass on the core RPC surface, two spec-promised behaviors are missing**:
(1) Info.vector_dim verification on Embed responses, (2) Health-aware
classification of first-failure (LOADING vs DEGRADED). Both are explicitly
deferred in the embedder.md spec ("classify first failure via Health"
described but never marked required for MVP). Documenting + queueing.

## Direct spec parity (✅)

| # | Spec section | Behavior | Where |
|---|---|---|---|
| 1 | §RPC 요약 | `Embed(text) → vector` | client.go:96-103 |
| 2 | §RPC 요약 | `EmbedBatch(texts) → embeddings` | client.go:106-130 |
| 3 | §RPC 요약 | `Info() → daemon_version, model_identity, vector_dim, max_text_length, max_batch_size` | info_cache.go |
| 4 | §RPC 요약 | `Health() → status, uptime, total_requests` | client.go:152-164 |
| 5 | §RPC 요약 | Shutdown intentionally NOT used (rune-mcp does not own daemon lifecycle) | (omitted) |
| 6 | §Dial | `unix://`+sockPath, insecure creds (UDS = same machine) | client.go:71-86 |
| 7 | §Retry 정책 (D7) | Backoff `[0, 500ms, 2s]` × 3 attempts | retry.go (#95) + client.go uses |
| 8 | §Retry 정책 | Retryable: Unavailable / DeadlineExceeded / ResourceExhausted | retry.go:retryable |
| 9 | §EmbedBatch with split | Split when len > MaxBatchSize, preserve order | client.go:106-130 |
| 10 | §Info 캐시 | `sync.Once` ensures single Info RPC | info_cache.go:Get |
| 11 | §Info 캐시 + D30 | `slog.Info "embedder info loaded"` w/ model_identity / vector_dim / max_batch_size | info_cache.go |
| 12 | §EmbedBatch resp count guard | `len(resp.Embeddings) != len(texts)` → error | client.go:144-147 |
| 13 | §Health 활용 status mapping | `STATUS_OK / LOADING / DEGRADED / SHUTTING_DOWN / UNSPECIFIED` enum→string | client.go:statusName |

## Acceptable divergences (⚠️)

1. **Hybrid choice — raw stub instead of `runed/client` Plan A wrapper**.
   Spec: "rune-mcp는 [runed/client] 라이브러리를 inner transport로 두고
   정책 layer만 자체 작성하는 hybrid 채택 가능." Implementation chose direct
   `runedv1.RunedServiceClient` because the wrapper exposes
   `Connect/Embed/EmbedBatch/Info/Close` but **NOT** `Health` (we need Health
   for the `rune_diagnostics` MCP tool). Documented in PR body. Net effect: a
   slightly larger surface area in our adapter, but no functional gap.

2. **Health is NOT auto-retried**. Spec §Retry 정책 lists Health among the
   retryable surface but D8 ("Boot does NOT poll Health — first embed call
   drives") implies Health is for diagnostic surfaces (vault_status,
   diagnostics) where transient-retry is misleading. We surface the raw error
   instead. Aligns with how `LifecycleService.Diagnostics` consumes it.

3. **`Close` does not call runed `Shutdown` RPC**. Spec §RPC 요약 explicitly
   "rune-mcp does not call Shutdown." Confirmed — we just close the gRPC
   conn.

## Open gaps (⚠️ should follow up)

### 1. **Info.vector_dim mismatch detection** (spec §불변 계약)

Spec: "**dim**: Qwen3-Embedding-0.6B 기준 1024. `Info.vector_dim`으로 확인 후
불일치면 에러." Not implemented — `embedBatchOnce` does not verify
`len(e.Vector) == c.info.Snapshot().VectorDim`. The spec's earlier prose code
sample (embedder.md §EmbedBatch with split) shows this guard:

```go
if len(e.Vector) != c.infoCache.Snapshot().VectorDim {
    return nil, fmt.Errorf("embedder: vector dim mismatch at index %d", i)
}
```

**Resolution**: Add the check inside `embedBatchOnce` (and a similar one
inside `EmbedSingle`). Low-risk add, ~5 LoC. Track as runed follow-up.

### 2. **Health-aware first-failure classification** (spec §Health 활용)

Spec: "첫 embed 호출 실패 시 `Health` 조회로 분류:
- LOADING → 잠시 후 재시도 (wait-and-retry 대기)
- DEGRADED → 경고 로그 + 상위 EmbedderDegradedError 전파
- SHUTTING_DOWN → 즉시 실패 + 상위 EmbedderUnavailableError"

Not implemented — we just bubble up the gRPC error. The retry helper does
N=3 attempts with code-based retryable check, but doesn't probe Health
between failures.

**Resolution**: This is a separate retry strategy refinement; a healthy MVP
can ship with code-based retryable alone (LOADING typically surfaces as
Unavailable + retryable=true → covered). Mark optional. Track as runed
follow-up.

### 3. **Error mapping to typed adapter errors** (spec §에러 매핑 table)

Spec lists 5 typed error sentinels:
- `EmbedderInvalidInputError` (gRPC InvalidArgument)
- `EmbedderBusyError` (ResourceExhausted)
- `EmbedderUnavailableError` (Unavailable)
- `EmbedderTimeoutError` (DeadlineExceeded)
- `EmbedderError(wrap)` (other)

Not implemented — `errors.go` doesn't exist in this adapter. Currently we
return the raw gRPC status error wrapped with `fmt.Errorf`. Service layer
relies on `status.FromError` to introspect the code, which works but lacks
the typed-sentinel pattern that vault uses.

**Resolution**: Add `internal/adapters/embedder/errors.go` with sentinels +
`MapGRPCError`. Symmetrical to `vault/errors.go`. Low-risk add. Track as
runed follow-up.

### 4. **Socket path resolution priority is caller's responsibility**

Comment at the top of `client.go` documents:
> 1. env RUNE_EMBEDDER_SOCKET
> 2. config.embedder.socket_path
> 3. default ~/.runed/embedding.sock

But `New(sockPath string)` takes the resolved path as input — caller must
implement the priority chain. Not implemented anywhere yet (no callsite
exists). **Resolution**: Add to `cmd/rune-mcp/main.go` `buildDeps` or in a
new `internal/adapters/embedder/socket.go` helper, alongside the boot loop
work.

## Test coverage status

`go test ./internal/adapters/embedder/` reports `[no test files]`. Mock
`RunedServiceClient` + retry / split / cache tests are queued as Task #7.

## Cross-check: runed daemon contract compatibility

- runed `go.mod` requires `go 1.26.2`; rune `go.mod` is `go 1.25.9`. Local
  `replace ../runed` bypasses the toolchain check. **Open**: bump rune to
  1.26 before publishing (also needed for runed publish).
- runed has `model_identity` placeholder behavior — slog breadcrumb captures
  whatever value it emits; we do not validate the format.
- runed gen/ is `.gitignored` in runed; first-time setup requires
  `cd ../runed && buf generate` (or `make proto`). Document in onboarding.
