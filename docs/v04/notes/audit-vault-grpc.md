# Vault-grpc Python Parity Audit (Task 4)

> Run on `couragehong/feat/vault-grpc` HEAD `0012afb` against
> `mcp/adapter/vault_client.py` (380 LoC).
> Spec: `docs/v04/spec/components/vault.md`.

## Verdict

**Pass with one open env-override gap.** All RPCs (GetPublicKey, DecryptScores,
DecryptMetadata, HealthCheck Tier 1) match Python semantics; Go adds 3
intentional safety/typing improvements. One Python feature (RUNEVAULT_GRPC_TARGET
env override) is not in the Go adapter and should be wired by the config
loader / Deps construction layer.

## Direct parity (✅)

| # | Behavior | Python | Go |
|---|---|---|---|
| 1 | gRPC channel options 256 MB max msg | L166-169 | client.go:84-87 (`MaxMessageLength`) |
| 2 | TLS w/ system CA default | L142-161 `_build_tls_credentials` | client.go:99-101 (`credentials.NewTLS(nil)`) |
| 3 | TLS w/ custom CA PEM | L150-158 | client.go:93-98 (`NewClientTLSFromFile`) |
| 4 | `tls_disable` insecure mode | L170-177 | client.go:90-92 (`insecure.NewCredentials`) |
| 5 | Endpoint normalization 4-form | L116-140 `_derive_grpc_target` | endpoint.go:21-40 |
| 6 | Bearer token in **request body** (NOT metadata) | L204 `pb2.GetPublicKeyRequest(token=...)` | client.go:144 same |
| 7 | GetPublicKey: json.loads bundle, error string check | L208-213 | client.go:147-167 |
| 8 | DecryptScores: token + blob + top_k → ScoreEntry list | L237-257 | client.go:172-198 |
| 9 | DecryptMetadata: token + list → strings | L280-295 | client.go:201-216 |
| 10 | HealthCheck Tier 1 grpc_health.v1 service="" | L307-316 | client.go:155-167 |
| 11 | All-RPC default 30 s, health 5 s | L84, L315 | `DefaultTimeout`, `HealthTimeout` |
| 12 | Channel `Close` | L185-190 | client.go:Close |

## Go-only improvements (✅)

1. **gRPC keepalive params** (Time=30 s, Timeout=10 s, PermitWithoutStream=true)
   per `vault.md §Keepalive`. Detects stale conns after sleep/wake / NAT timeout.
   Python has none — vulnerable to dead-conn first-RPC failures.
2. **`ValidateAgentDEK` length check** (`len(dek) != 32` → fail-fast). Python
   `envector_sdk.py:L139` `self._agent_dek = agent_dek` accepts any size silently
   (security gap noted in `vault.md §agent_dek`).
3. **Typed `*Error` with Code / Retryable / Cause** via `MapGRPCError` (gRPC
   status → 5 sentinels: `VAULT_UNAVAILABLE`, `VAULT_AUTH_FAILED`,
   `VAULT_KEY_NOT_FOUND`, `VAULT_TIMEOUT`, `VAULT_INTERNAL`). Python wraps as
   plain `VaultError(str)` and the service layer reparses by string. Go is more
   robust for service-level error classification.

## Acceptable divergences (⚠️)

1. **DecryptResult wrapper vs plain error**. Python L221 returns
   `DecryptResult{ok=false, results=[], error=resp.error}` for application-level
   failures; Go returns Go `error` (`*Error`). Service-layer behavior is
   equivalent — both halt the RPC and surface the message — but the wire-shape
   differs. Not user-visible (service translates to `domain.RuneError`).
2. **DecryptMetadata JSON parse location**. Python L293 parses each item in the
   client. Go returns raw `[]string` and the service does `json.Unmarshal` per
   envelope. **Spec preference**: `vault.md` L47-48 ("rune-mcp는 각 문자열을
   `json.Unmarshal`로 parse만") — Go matches spec.
3. **HealthCheck Tier 2 (HTTP /health) auto-fallback**. Python L320-336 chains
   Tier 2 inside `health_check()`. Go exposes `HealthFallback` as a standalone
   function; `HealthCheck()` is Tier 1 only. Per `vault.md §Health 2-tier`
   ("진단용") Tier 2 is for diagnostic messaging, not regular liveness — caller
   (service/lifecycle.go) decides when to invoke.
4. **Per-instance timeout configurability**. Python L84 `timeout=30.0` is a
   ctor param. Go uses package-level `DefaultTimeout` constant; per-call ctx
   timeout is the override path. Either fine for our use; if we ever need
   per-Vault timeout, switch to a ClientOpts field.

## Open gap (⚠️ should follow up)

**`RUNEVAULT_GRPC_TARGET` env override is not honored**. Python L108-110:

```python
self._grpc_target = os.getenv("RUNEVAULT_GRPC_TARGET")
if not self._grpc_target:
    self._grpc_target = self._derive_grpc_target(self.vault_endpoint)
```

This is a documented escape hatch for ops to redirect Vault traffic without
editing config.json (used in incident response). Go adapter does not check the
env var — `NewClient` always normalizes from the passed-in endpoint.

**Resolution**: env priority belongs in the config loader / Deps construction
layer (caller of `NewClient`). Adding it inside `NewClient` would mix
concerns. Open as a config-layer task (likely belongs in
`internal/adapters/config/loader.go` or `cmd/rune-mcp/main.go` boot
construction). Track in production-readiness audit (Task #5).

## Bug fixed in this PR

Commit `7a8199c` `fix(vault): use grpc/status + codes for MapGRPCError type
assertion` — #95's hand-rolled `grpcStatuser` interface required `Code() int`
but real gRPC `*status.Status.Code()` returns `codes.Code` (uint32). The
assertion silently failed for every gRPC error and every status fell through
to `ErrVaultInternal`. Fixed by importing `google.golang.org/grpc/{codes,status}`
and using `status.FromError`. Either keep this commit in this PR or fold back
into #95's branch — reviewer's call.

## Test coverage status

`go test ./internal/adapters/vault/` reports `[no test files]`. Bufconn-based
unit tests for the 4 RPCs + error paths are queued as Task #6.
