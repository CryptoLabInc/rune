# Vault-Side Filtering, Recency Weighting & Group Assembly

**Date**: 2026-03-24
**Status**: Approved
**Target repo**: rune-admin (Vault)
**Client repo**: rune (plugin)

## Problem

The client (Searcher) currently performs metadata filtering, group assembly, recency weighting, and phase chain expansion after Vault returns top-k results. This has two issues:

1. **Metadata filters reduce result count below top-k** — if user requests `topk=10` with `domain="architecture"` and only 3 of the top 10 match, client returns 3.
2. **Group expansion requires extra round-trips** — missing phase chain siblings trigger additional search queries from the client.

These operations belong inside Vault, where internal over-fetch can maintain result count without exposing extra decrypted records to the agent.

## Security Rationale

The agent must NEVER see more than top-k decrypted records. If the client over-fetches (e.g., requests 100 to filter down to 10), a compromised plugin gains bulk access to organizational memory. Vault controls the tap: only filtered, ranked top-k flows out.

## Design

### Proto Change

Extend `DecryptScoresRequest` with 3 optional filter fields:

```protobuf
message DecryptScoresRequest {
  bytes encrypted_blob = 1;
  int32 top_k = 2;
  // v0.2.4: optional metadata filters
  optional string filter_domain = 3;  // e.g. "architecture"
  optional string filter_status = 4;  // e.g. "accepted"
  optional string filter_since = 5;   // ISO date e.g. "2026-01-01"
}
```

Response format unchanged. List order = display order.

All fields are optional — omitting them produces current behavior (no filters). This is fully backward compatible.

### Vault Internal Pipeline

When Vault receives `DecryptScores`:

```
1. Decrypt full score ciphertext
2. Internal over-fetch: select top (top_k * INTERNAL_MULTIPLIER) candidates
3. For each candidate, decrypt metadata (already in Vault memory)
4. Apply metadata filters (domain, status, since) → filtered pool
5. Apply recency weighting:
   - decay = 0.5^(age_days / HALF_LIFE_DAYS)
   - status_mult = {accepted: 1.0, proposed: 0.9, superseded: 0.5, reverted: 0.3}
   - adjusted_score = (SIMILARITY_WEIGHT * score + RECENCY_WEIGHT * decay) * status_mult
6. Group assembly with recursion:
   - If a result has group_id, pull all siblings from the pool
   - n siblings assembled → fill remaining k-n slots from pool
   - Recurse if new group_ids found in the fill
   - Groups ordered by phase_seq
7. Return exactly top_k results
```

### Vault Configuration

Scoring and assembly parameters live in Vault config (not sent by client):

```yaml
search:
  internal_fetch_multiplier: 10
  recency:
    half_life_days: 90
    similarity_weight: 0.7
    recency_weight: 0.3
  status_multiplier:
    accepted: 1.0
    proposed: 0.9
    superseded: 0.5
    reverted: 0.3
  group_assembly:
    enabled: true
    max_recursion_depth: 3
```

### Client Changes (rune plugin)

After Vault deployment, `searcher.py` simplifies:

**Remove:**
- `_apply_metadata_filters()`
- `_expand_phase_chains()`
- `_assemble_groups()`
- `_apply_recency_weighting()`
- Constants: `HALF_LIFE_DAYS`, `SIMILARITY_WEIGHT`, `RECENCY_WEIGHT`, `STATUS_MULTIPLIER`

**Modify:**
- `_search_via_vault()` — pass filters to `decrypt_search_results()`
- `search()` pipeline — reduce to: search with expansions → dedup → return top-k

**Keep (for non-Vault fallback):**
- `_search_direct()` path retains current client-side logic for deployments without Vault.

### Migration

1. Proto fields are optional → existing clients work without changes
2. Deploy Vault with new logic first
3. Client simplification is a separate PR after Vault rollout confirmed
4. Vault config ships with sensible defaults (values above)

## Files Referenced

- `agents/retriever/searcher.py` — current client-side workarounds
- `mcp/adapter/vault_client.py` — gRPC client (`decrypt_search_results`)
- `mcp/server/server.py` — MCP `recall` tool (lines 757-800)
- `mcp/adapter/vault_proto/vault_service.proto` — proto definition (in rune-admin)
