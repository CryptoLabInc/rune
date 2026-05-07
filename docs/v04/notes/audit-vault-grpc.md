# Vault gRPC adapter audit (post-#103 contract)

> Run on `couragehong/feat/vault-fixups` against `internal/adapters/vault/`
> after #103 merged the `GetPublicKey` → `GetAgentManifest` rename and the
> EvalKey ownership shift to Vault server-side.
>
> Spec: `docs/v04/spec/components/vault.md`.
> Python reference: `mcp/adapter/vault_client.py` (~381 LoC).

## Verdict

**Pass after this PR's three fixups**. The vault adapter as merged in #103
had three regressions that this branch addresses; with them applied the
adapter is at full Python-parity for the new agent-manifest contract,
plus three Go-only safety/observability improvements.

## What this PR fixes

### 1. `MapGRPCError` typed-error fix + complete coverage of server-emitted codes

Two interlocking issues in one fix:

**(1a)** #103 ported the typed-error pattern in `envector/errors.go`
(`MapSDKError` using `status.FromError + codes.*`) but did not touch
`vault/errors.go`, which still carried #95's hand-rolled `grpcStatuser`
interface:

```go
type grpcStatuser interface {
    GRPCStatus() interface {
        Code() int          // ← real *status.Status returns codes.Code (uint32)
        Message() string
    }
}
```

Go interface satisfaction is by exact signature match. `Code() codes.Code`
does NOT satisfy `Code() int` (named type vs basic type), so
`err.(grpcStatuser)` was `false` for every gRPC error. Every status fell
into the `!ok` branch as `ErrVaultInternal/Retryable: true`, hiding the
actual code.

Worst impact: `Unauthenticated` (token revoked/expired) gets mis-flagged
as retryable, triggering infinite retry on auth failure.

**(1b)** While auditing `rune-admin/vault/internal/server/grpc.go`, found
that the server actually returns three codes the original mapping never
covered — they all fell to the `default` branch as
`VAULT_INTERNAL/Retryable: true`:

- `codes.PermissionDenied` (role scope check, role TopK limit) — should
  be **non-retryable** (role can't be promoted by retry) and a distinct
  code so callers see "permission denied" not "internal error".
- `codes.InvalidArgument` (deserialization failure, base64 decode fail,
  validator rejection) — should be **non-retryable** (same input won't
  succeed on retry) and a distinct code.
- `codes.ResourceExhausted` (token rate limit via `mapTokenError`) —
  retryable=true is fine after backoff, but the code "INTERNAL" is
  misleading; should be a distinct rate-limit code.

Fix: switched to `status.FromError(err)` + `codes.*` constants, removed
the hand-rolled `grpcStatus` struct + `statusFromError` helper + `code*`
constants block, and added three new sentinels:

```go
ErrVaultPermissionDenied = &Error{Code: "VAULT_PERMISSION_DENIED", Retryable: false}
ErrVaultInvalidInput     = &Error{Code: "VAULT_INVALID_INPUT",     Retryable: false}
ErrVaultRateLimited      = &Error{Code: "VAULT_RATE_LIMITED",      Retryable: true}
```

Now the full code → sentinel table covers everything the server emits
(`Unauthenticated`, `PermissionDenied`, `InvalidArgument`,
`ResourceExhausted`, `Internal`) plus transport-layer codes
(`Unavailable`, `DeadlineExceeded`) plus a legacy `NotFound` mapping
that's reserved for future server changes.

### 2. `MaxMessageLength` 256MB restored (commit `21a07e2`)

#103 lowered the cap from 256MB → 16MB without justification. Spec
(`vault.md` §256MB) and Python parity (`vault_client.py:L33`) both call
for 256MB. Even with EvalKey no longer in the manifest (Vault now owns
EvalKey/SecKey server-side), the manifest_json still carries EncKey JSON
content which can run multiple MBs depending on FHE parameters. 16MB
risks `ResourceExhausted` on future deployments; 256MB matches Python
without measurable cost.

### 3. bufconn unit tests + `NewBufconnClient` injector

#103 added envector unit tests (`errors_test.go`, `client_integration_test.go`)
but no vault coverage. Adds **34 test functions / 31 subtest cases** (65
runs total) covering all four RPC paths, the error-mapping matrix, edge
cases the original audit caught, and lifecycle invariants:

| Surface | Coverage |
|---|---|
| `GetAgentManifest` happy / errors | full bundle equality, omitted `index_name`, response.error string, malformed JSON, ctx cancellation |
| `decodeAgentDEK` paths | empty agent_dek, invalid base64, length matrix (0 / 16 / 31 / 32 / 33 / 64) |
| `GetAgentManifest` ↔ MapGRPCError integration | per-RPC matrix for Unauthenticated / PermissionDenied / InvalidArgument / ResourceExhausted / Internal — verifies typed `*Error` reaches the caller |
| `DecryptScores` | happy path, gRPC error, response.error string, empty results |
| `DecryptMetadata` | happy path, gRPC error, response.error string, empty list |
| `HealthCheck` | SERVING / NOT_SERVING / gRPC error → `VAULT_UNAVAILABLE` |
| `ParseManifestJSON` direct | empty JSON, missing `agent_dek`, not-JSON, forward-compat with stray `EvalKey.json` field |
| `MapGRPCError` matrix | full 9-code table (Unauthenticated / PermissionDenied / InvalidArgument / ResourceExhausted / NotFound / Unavailable / DeadlineExceeded / Internal / Aborted default), non-gRPC fallback, nil |
| `MapGRPCError` invariants | `Cause` preservation across all 9 codes, `errors.Is(*Error, original)` chain, `Message` carry-through |
| `*Error.Error()` formatter | code-only vs `code: message` |
| Lifecycle | `Endpoint()` returns constructor value, `Close()` is nil-safe (idempotent) |

Plus `NewBufconnClient(*grpc.ClientConn, token) Client` constructor
factored through `newWithConn` so `NewClient` and `NewBufconnClient`
share struct init. Useful for tests + production callers that pool
conns externally.

Run time ~0.4s. No real Vault dependency.

## Spec parity (post-fixups)

| Item | Status | Where |
|---|---|---|
| `GetAgentManifest` RPC + 7-field manifest_json parse | ✅ #103 | `client.go:200` |
| `EvalKey` removed from plugin-side Bundle | ✅ #103 | `client.go:48` |
| `agent_dek` length-32 validation | ✅ #103 | `client.go:decodeAgentDEK` |
| `DecryptScores` (token + blob + top_k) | ✅ #103 | `client.go:212` |
| `DecryptMetadata` (token + list) | ✅ #103 | `client.go:239` |
| `HealthCheck` Tier 1 grpc_health_v1 service="" | ✅ #103 | `client.go:257` |
| `MaxMessageLength = 256 MB` | ✅ this PR | `client.go:38` |
| Endpoint normalization (4-form) | ✅ inherited (`endpoint.go`) | unchanged |
| TLS modes (system CA / custom CA / insecure) | ✅ #103 | `client.go:160-176` |
| Keepalive (Time=30s, Timeout=10s, PermitWithoutStream) | ✅ #103 | `client.go:142` |
| `MapGRPCError` typed mapping (8 sentinels — full coverage of server-emitted + transport codes) | ✅ this PR | `errors.go` |
| `withTimeout` helper preserves shorter caller deadline | ✅ #103 | `client.go:198` |

## Go-only safety/observability still in place

1. `ValidateAgentDEK`-equivalent length-32 check (now inlined as
   `decodeAgentDEK`). Python `envector_sdk.py:L139` accepts any size silently.
2. Typed `*Error` with Code / Retryable / Cause via `MapGRPCError`. Python
   uses opaque `VaultError(str)` and re-parses by string at the service layer.
3. `keepalive` params. Python has none — vulnerable to dead-conn first-RPC
   failures after sleep/wake / NAT timeout.

## Acceptable divergences (⚠️, not blocking)

1. **DecryptResult wrapper vs plain error**. Python wraps app-level failures
   as `DecryptResult{ok=false, error=…}`; Go returns Go `error` (`*Error`).
   Service-layer behavior is equivalent.
2. **DecryptMetadata JSON parse location**. Python parses each item in the
   client; Go returns raw `[]string` and the service does `json.Unmarshal`
   per envelope. Spec (`vault.md` L47-48) says caller parses — Go matches
   spec.
3. **HealthCheck Tier 2 (HTTP /health) auto-fallback**. Python chains Tier 2
   inside `health_check()`; Go exposes `HealthFallback` as a standalone
   function. Per `vault.md §Health 2-tier`, Tier 2 is for diagnostic
   messaging only — caller (service/lifecycle) decides when to invoke.
4. **Per-instance timeout configurability**. Python `timeout=30.0` is a
   ctor param; Go uses package-level `DefaultTimeout` constant. Per-call
   ctx timeout is the override path.
5. **Bearer token in metadata header**. #103 sends `authorization: Bearer
   <token>` as outgoing metadata via `authCtx`, but Vault server reads the
   token only from `req.GetToken()`. Harmless; possibly future-proofing for
   header-based auth migration.

## Open follow-up items (not in this PR)

1. **`RUNEVAULT_GRPC_TARGET` env override**. Python `vault_client.py:L108-110`
   honors this as a gRPC target escape hatch for ops. Belongs in the config
   loader / Deps construction layer, not the adapter. Track separately.
2. **Cache `grpc_health_v1.HealthClient` as a struct field**. #103 allocates
   a fresh client per `HealthCheck` call. Trivial perf, not blocking.
3. **`response.error` string is uniformly mapped to `Retryable: true`**.
   Some app-level errors (invalid input, role denied) shouldn't be
   retryable. Consider a switch on the message — or, better, the server
   returning a typed code in addition to the message.
4. **MetadataRef type asymmetry**: `vault.ScoreEntry.ShardIdx` is `int32`
   while `envector.MetadataRef.ShardIdx` is `uint64`. Same logical value
   crossing a boundary — minor cleanup if/when it bites.

## Test coverage status

`go test ./internal/adapters/vault/...` reports **34 top-level functions /
31 subtest cases (65 individual runs), all passing in ~0.4s**. No real
Vault dependency; bufconn-backed in-process server.

Coverage areas (post this PR):

- All four RPCs (`GetAgentManifest`, `DecryptScores`, `DecryptMetadata`,
  `HealthCheck`) — happy path + gRPC error path + response.error string
  path + boundary inputs (empty list, empty results)
- `decodeAgentDEK` — three error paths exercised (empty / invalid base64 /
  length matrix 0–64)
- `MapGRPCError` — full 9-code matrix matching every code rune-admin/vault
  emits + transport-layer codes + non-gRPC fallback + nil
- `MapGRPCError` invariants — `Cause` preservation, `errors.Is/As` chain,
  `Message` carry-through
- `ParseManifestJSON` direct — empty JSON, missing required field, not-JSON,
  forward-compat with stale `EvalKey.json`
- Lifecycle — `Endpoint()` constructor value, `Close()` idempotency,
  context cancellation propagation

For end-to-end testing against a real Vault (post boot loop landing),
add a `//go:build integration` test similar to `envector/client_integration_test.go`.
