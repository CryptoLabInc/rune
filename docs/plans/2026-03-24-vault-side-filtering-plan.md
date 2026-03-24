# Vault-Side Filtering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move metadata filtering, recency weighting, and group assembly from the client (rune plugin) into the Vault (rune-admin), so that DecryptScores returns pre-filtered, pre-ranked, group-assembled top-k results.

**Architecture:** Extend `DecryptScoresRequest` with 3 optional filter fields. Vault internally over-fetches candidates, calls enVector Cloud `remind()` to get encrypted metadata, decrypts it, applies filters + recency weighting + group assembly, and returns exactly top-k entries. Response format unchanged. Client simplification follows in a separate PR.

**Tech Stack:** Python 3.12, gRPC/protobuf, pyenvector SDK, pytest

**Repos:**
- `rune-admin` (`/Users/sunchuljung/repo/cryptolab/Rune/rune-admin`) — Tasks 1-7
- `rune` (`/Users/sunchuljung/repo/cryptolab/Rune/rune`) — Task 8 (client simplification)

---

### Task 1: Proto — Add Filter Fields

**Files:**
- Modify: `rune-admin/vault/proto/vault_service.proto`

**Step 1: Add optional filter fields to DecryptScoresRequest**

In `vault/proto/vault_service.proto`, add 3 fields after `top_k`:

```protobuf
message DecryptScoresRequest {
  string token = 1;
  string encrypted_blob_b64 = 2;
  int32  top_k = 3;
  // v0.2.4: optional metadata filters (backward compatible)
  string filter_domain = 4;  // e.g. "architecture", empty = no filter
  string filter_status = 5;  // e.g. "accepted", empty = no filter
  string filter_since = 6;   // ISO date e.g. "2026-01-01", empty = no filter
}
```

**Step 2: Regenerate Python stubs**

Run:
```bash
cd rune-admin/vault
python -m grpc_tools.protoc \
  -I proto \
  --python_out=proto \
  --grpc_python_out=proto \
  proto/vault_service.proto
```

Expected: `proto/vault_service_pb2.py` and `proto/vault_service_pb2_grpc.py` regenerated without errors.

**Step 3: Commit**

```bash
git add vault/proto/
git commit -m "proto: add optional filter fields to DecryptScoresRequest"
```

---

### Task 2: Vault Config — Add Search Parameters

**Files:**
- Modify: `rune-admin/vault/vault_core.py` (top-level config section, lines 39-53)
- Modify: `rune-admin/vault/.env.example`

**Step 1: Add config constants to vault_core.py**

After the existing config block (line 53), add:

```python
# Over-fetch and scoring configuration
INTERNAL_FETCH_MULTIPLIER = int(os.getenv("VAULT_FETCH_MULTIPLIER", "10"))
HALF_LIFE_DAYS = float(os.getenv("VAULT_HALF_LIFE_DAYS", "90"))
SIMILARITY_WEIGHT = float(os.getenv("VAULT_SIMILARITY_WEIGHT", "0.7"))
RECENCY_WEIGHT = float(os.getenv("VAULT_RECENCY_WEIGHT", "0.3"))
STATUS_MULTIPLIER = {
    "accepted": 1.0,
    "proposed": 0.9,
    "superseded": 0.5,
    "reverted": 0.3,
}
GROUP_ASSEMBLY_MAX_DEPTH = int(os.getenv("VAULT_GROUP_MAX_DEPTH", "3"))
```

**Step 2: Add env vars to .env.example**

```bash
# ── Search Behavior ─────────────────────────────────────────────
VAULT_FETCH_MULTIPLIER=10
VAULT_HALF_LIFE_DAYS=90
VAULT_SIMILARITY_WEIGHT=0.7
VAULT_RECENCY_WEIGHT=0.3
VAULT_GROUP_MAX_DEPTH=3
```

**Step 3: Commit**

```bash
git add vault/vault_core.py vault/.env.example
git commit -m "config: add over-fetch and scoring parameters"
```

---

### Task 3: enVector Remind Client — Add to Vault

**Files:**
- Modify: `rune-admin/vault/vault_core.py`

The Vault needs to call enVector Cloud's `remind()` to fetch encrypted metadata for over-fetched candidates. The `pyenvector` SDK is already a dependency and `ev.init()` is called at startup.

**Step 1: Write failing test**

Create `rune-admin/tests/unit/test_envector_remind.py`:

```python
"""Tests for Vault's internal enVector remind() call."""
import pytest
import sys
import os
import json
from unittest.mock import MagicMock, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../vault'))

from vault_core import _remind_and_decrypt_metadata, rate_limiter


class TestRemindAndDecryptMetadata:

    @pytest.fixture(autouse=True)
    def reset_rate_limiter(self):
        rate_limiter._requests.clear()

    def test_returns_metadata_for_indices(self, monkeypatch):
        """Should fetch and decrypt metadata for given indices."""
        indices = [
            {"shard_idx": 0, "row_idx": 5, "score": 0.9},
            {"shard_idx": 0, "row_idx": 12, "score": 0.8},
        ]

        # Mock pyenvector remind
        mock_ev = MagicMock()
        mock_ev.remind.return_value = {
            "ok": True,
            "results": [
                {"data": '{"domain":"architecture","status":"accepted","timestamp":"2026-03-01T00:00:00Z","group_id":null}'},
                {"data": '{"domain":"security","status":"proposed","timestamp":"2026-02-15T00:00:00Z","group_id":null}'},
            ],
        }
        monkeypatch.setattr('vault_core.ev', mock_ev)
        monkeypatch.setattr('vault_core.VAULT_INDEX_NAME', 'test-index')

        results = _remind_and_decrypt_metadata("valid-token", indices)

        assert len(results) == 2
        assert results[0]["domain"] == "architecture"
        assert results[1]["status"] == "proposed"

    def test_returns_empty_for_no_indices(self, monkeypatch):
        """Should return empty list for empty indices."""
        results = _remind_and_decrypt_metadata("valid-token", [])
        assert results == []
```

**Step 2: Run test to verify it fails**

Run: `cd rune-admin && python -m pytest tests/unit/test_envector_remind.py -v`
Expected: FAIL — `_remind_and_decrypt_metadata` not defined.

**Step 3: Implement _remind_and_decrypt_metadata**

Add to `vault_core.py` after `_decrypt_metadata_impl`:

```python
def _remind_and_decrypt_metadata(
    token: str,
    indices: list[dict],
) -> list[dict]:
    """
    Fetch encrypted metadata from enVector Cloud and decrypt it.

    Used internally by DecryptScores for over-fetch filtering.
    Calls ev.remind() → decrypts AES metadata → returns parsed dicts.

    Args:
        token: Auth token (for agent DEK derivation).
        indices: List of {shard_idx, row_idx, score} from FHE decryption.

    Returns:
        List of decrypted metadata dicts, one per index entry.
    """
    if not indices:
        return []

    import pyenvector as ev

    if not VAULT_INDEX_NAME:
        logger.warning("VAULT_INDEX_NAME not set, cannot fetch metadata")
        return [{} for _ in indices]

    # Fetch encrypted metadata from enVector Cloud
    remind_result = ev.remind(
        index_name=VAULT_INDEX_NAME,
        indices=indices,
        output_fields=["metadata"],
    )

    if not remind_result.get("ok"):
        logger.warning("remind() failed: %s", remind_result.get("error"))
        return [{} for _ in indices]

    # Decrypt each entry's metadata
    master_key = _load_master_key()
    agent_id = hashlib.sha256(token.encode('utf-8')).hexdigest()[:32]
    agent_dek = derive_agent_key(master_key, agent_id)

    results = []
    for entry in remind_result.get("results", []):
        data = entry.get("data", "")
        if not data:
            results.append({})
            continue
        try:
            parsed = json.loads(data)
            if isinstance(parsed, dict) and "a" in parsed and "c" in parsed:
                decrypted = aes_decrypt_metadata(parsed["c"], agent_dek)
                if isinstance(decrypted, bytes):
                    decrypted = decrypted.decode('utf-8')
                results.append(json.loads(decrypted))
            else:
                results.append(parsed)
        except Exception as e:
            logger.debug("Metadata decrypt failed for entry: %s", e)
            results.append({})

    return results
```

**Step 4: Run test to verify it passes**

Run: `cd rune-admin && python -m pytest tests/unit/test_envector_remind.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add vault/vault_core.py tests/unit/test_envector_remind.py
git commit -m "feat: add internal remind + metadata decrypt for over-fetch"
```

---

### Task 4: Filtering & Recency Weighting — Pure Functions

**Files:**
- Modify: `rune-admin/vault/vault_core.py`
- Create: `rune-admin/tests/unit/test_filtering.py`

**Step 1: Write failing tests**

Create `rune-admin/tests/unit/test_filtering.py`:

```python
"""Tests for metadata filtering, recency weighting, and group assembly."""
import pytest
import sys
import os
import json
from datetime import datetime, timezone, timedelta

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../vault'))

from vault_core import (
    _apply_filters,
    _apply_recency_weighting,
    rate_limiter,
)


class TestApplyFilters:

    def _entry(self, domain="general", status="accepted", timestamp="2026-03-01T00:00:00Z", **kwargs):
        return {"domain": domain, "status": status, "timestamp": timestamp, **kwargs}

    def test_filter_by_domain(self):
        candidates = [
            (0.9, self._entry(domain="architecture")),
            (0.8, self._entry(domain="security")),
            (0.7, self._entry(domain="architecture")),
        ]
        result = _apply_filters(candidates, filter_domain="architecture")
        assert len(result) == 2
        assert all(m["domain"] == "architecture" for _, m in result)

    def test_filter_by_status(self):
        candidates = [
            (0.9, self._entry(status="accepted")),
            (0.8, self._entry(status="proposed")),
        ]
        result = _apply_filters(candidates, filter_status="accepted")
        assert len(result) == 1

    def test_filter_by_since(self):
        candidates = [
            (0.9, self._entry(timestamp="2026-03-15T00:00:00Z")),
            (0.8, self._entry(timestamp="2026-01-01T00:00:00Z")),
        ]
        result = _apply_filters(candidates, filter_since="2026-03-01")
        assert len(result) == 1

    def test_no_filters_returns_all(self):
        candidates = [
            (0.9, self._entry()),
            (0.8, self._entry()),
        ]
        result = _apply_filters(candidates)
        assert len(result) == 2

    def test_combined_filters(self):
        candidates = [
            (0.9, self._entry(domain="architecture", status="accepted", timestamp="2026-03-15T00:00:00Z")),
            (0.8, self._entry(domain="architecture", status="proposed", timestamp="2026-03-15T00:00:00Z")),
            (0.7, self._entry(domain="security", status="accepted", timestamp="2026-03-15T00:00:00Z")),
        ]
        result = _apply_filters(candidates, filter_domain="architecture", filter_status="accepted")
        assert len(result) == 1
        assert result[0][0] == 0.9


class TestRecencyWeighting:

    def test_recent_scores_higher(self):
        now = datetime.now(timezone.utc).isoformat()
        old = (datetime.now(timezone.utc) - timedelta(days=365)).isoformat()
        candidates = [
            (0.8, {"timestamp": old, "status": "accepted"}),
            (0.8, {"timestamp": now, "status": "accepted"}),
        ]
        result = _apply_recency_weighting(candidates)
        # Recent entry should have higher adjusted score
        assert result[0][1]["timestamp"] == now

    def test_reverted_penalized(self):
        now = datetime.now(timezone.utc).isoformat()
        candidates = [
            (0.8, {"timestamp": now, "status": "reverted"}),
            (0.8, {"timestamp": now, "status": "accepted"}),
        ]
        result = _apply_recency_weighting(candidates)
        assert result[0][1]["status"] == "accepted"

    def test_returns_tuples_with_adjusted_score(self):
        now = datetime.now(timezone.utc).isoformat()
        candidates = [(0.9, {"timestamp": now, "status": "accepted"})]
        result = _apply_recency_weighting(candidates)
        assert len(result) == 1
        # Result is (adjusted_score, metadata)
        assert isinstance(result[0][0], float)
```

**Step 2: Run tests to verify they fail**

Run: `cd rune-admin && python -m pytest tests/unit/test_filtering.py -v`
Expected: FAIL — functions not defined.

**Step 3: Implement _apply_filters and _apply_recency_weighting**

Add to `vault_core.py`:

```python
def _apply_filters(
    candidates: list[tuple[float, dict]],
    filter_domain: str = "",
    filter_status: str = "",
    filter_since: str = "",
) -> list[tuple[float, dict]]:
    """
    Filter (score, metadata) candidates by optional metadata fields.

    Args:
        candidates: List of (similarity_score, metadata_dict) tuples.
        filter_domain: Keep only this domain (empty = no filter).
        filter_status: Keep only this status (empty = no filter).
        filter_since: Keep only records after this ISO date (empty = no filter).

    Returns:
        Filtered list of (score, metadata) tuples.
    """
    result = candidates
    if filter_domain:
        result = [(s, m) for s, m in result if m.get("domain") == filter_domain]
    if filter_status:
        result = [(s, m) for s, m in result if m.get("status") == filter_status]
    if filter_since:
        filtered = []
        for s, m in result:
            ts = m.get("timestamp", "")
            if ts >= filter_since:
                filtered.append((s, m))
        result = filtered
    return result


def _apply_recency_weighting(
    candidates: list[tuple[float, dict]],
) -> list[tuple[float, dict]]:
    """
    Re-score candidates with time decay and status multiplier.

    Formula: adjusted = (SIMILARITY_WEIGHT * score + RECENCY_WEIGHT * decay) * status_mult
    Where: decay = 0.5^(age_days / HALF_LIFE_DAYS)

    Args:
        candidates: List of (similarity_score, metadata_dict) tuples.

    Returns:
        List of (adjusted_score, metadata_dict) sorted descending by adjusted_score.
    """
    now = datetime.now(timezone.utc)
    scored = []
    for sim_score, meta in candidates:
        age_days = 0
        ts_str = meta.get("timestamp", "")
        if ts_str:
            try:
                ts = datetime.fromisoformat(ts_str.replace("Z", "+00:00"))
                age_days = max(0, (now - ts).days)
            except (ValueError, TypeError):
                pass
        decay = 0.5 ** (age_days / HALF_LIFE_DAYS) if HALF_LIFE_DAYS > 0 else 1.0
        status_mult = STATUS_MULTIPLIER.get(meta.get("status", ""), 1.0)
        adjusted = (SIMILARITY_WEIGHT * sim_score + RECENCY_WEIGHT * decay) * status_mult
        scored.append((adjusted, meta))
    scored.sort(key=lambda x: x[0], reverse=True)
    return scored
```

Add `from datetime import datetime, timezone` to imports at top of `vault_core.py`.

**Step 4: Run tests to verify they pass**

Run: `cd rune-admin && python -m pytest tests/unit/test_filtering.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add vault/vault_core.py tests/unit/test_filtering.py
git commit -m "feat: add metadata filtering and recency weighting"
```

---

### Task 5: Group Assembly with Recursion

**Files:**
- Modify: `rune-admin/vault/vault_core.py`
- Create: `rune-admin/tests/unit/test_group_assembly.py`

**Step 1: Write failing tests**

Create `rune-admin/tests/unit/test_group_assembly.py`:

```python
"""Tests for group assembly with recursion."""
import pytest
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '../../vault'))

from vault_core import _assemble_groups_from_pool, rate_limiter


class TestGroupAssembly:

    def _entry(self, score, group_id=None, phase_seq=None, phase_total=None, **kwargs):
        meta = {"group_id": group_id, "group_type": "phase_chain" if group_id else None,
                "phase_seq": phase_seq, "phase_total": phase_total, "status": "accepted",
                "timestamp": "2026-03-01T00:00:00Z", **kwargs}
        return (score, meta)

    def test_no_groups_returns_topk(self):
        pool = [self._entry(0.9 - i * 0.1) for i in range(5)]
        result = _assemble_groups_from_pool(pool, top_k=3)
        assert len(result) == 3

    def test_group_siblings_pulled_in(self):
        """When phase 2 is in top-k, phases 1 and 3 should be pulled from pool."""
        pool = [
            self._entry(0.95),  # standalone, rank 1
            self._entry(0.90, group_id="g1", phase_seq=1, phase_total=3),  # rank 2
            self._entry(0.50, group_id="g1", phase_seq=0, phase_total=3),  # rank low
            self._entry(0.40, group_id="g1", phase_seq=2, phase_total=3),  # rank low
            self._entry(0.85),  # standalone, rank 3
        ]
        result = _assemble_groups_from_pool(pool, top_k=5)
        # Should contain: standalone(0.95) + all 3 group members + standalone(0.85)
        group_members = [m for _, m in result if m.get("group_id") == "g1"]
        assert len(group_members) == 3
        # Group should be in phase_seq order
        seqs = [m["phase_seq"] for m in group_members]
        assert seqs == [0, 1, 2]

    def test_group_fills_remaining_with_recursion(self):
        """After pulling n group members, remaining k-n slots filled from pool."""
        pool = [
            self._entry(0.95, group_id="g1", phase_seq=0, phase_total=2),
            self._entry(0.50, group_id="g1", phase_seq=1, phase_total=2),
            self._entry(0.85),  # standalone
            self._entry(0.80),  # standalone
            self._entry(0.75),  # standalone
        ]
        result = _assemble_groups_from_pool(pool, top_k=4)
        assert len(result) == 4
        # 2 group members + 2 standalone
        group_count = sum(1 for _, m in result if m.get("group_id") == "g1")
        assert group_count == 2

    def test_respects_max_depth(self):
        """Recursion should stop at GROUP_ASSEMBLY_MAX_DEPTH."""
        # Create nested groups that would trigger deep recursion
        pool = [
            self._entry(0.95, group_id="g1", phase_seq=0, phase_total=2),
            self._entry(0.50, group_id="g1", phase_seq=1, phase_total=2),
            self._entry(0.85, group_id="g2", phase_seq=0, phase_total=2),
            self._entry(0.40, group_id="g2", phase_seq=1, phase_total=2),
        ]
        result = _assemble_groups_from_pool(pool, top_k=3)
        assert len(result) <= 4  # Should not exceed pool size
```

**Step 2: Run tests to verify they fail**

Run: `cd rune-admin && python -m pytest tests/unit/test_group_assembly.py -v`
Expected: FAIL

**Step 3: Implement _assemble_groups_from_pool**

Add to `vault_core.py`:

```python
def _assemble_groups_from_pool(
    pool: list[tuple[float, dict]],
    top_k: int,
    _depth: int = 0,
) -> list[tuple[float, dict]]:
    """
    Select top_k results from pool, prioritizing group completeness.

    When a group member is in the top-k candidates, all siblings from the
    pool are pulled in. If n siblings are included, the remaining k-n slots
    are filled recursively from the remaining pool.

    Args:
        pool: List of (adjusted_score, metadata) sorted descending by score.
        top_k: Number of results to return.
        _depth: Recursion depth (internal, for safety limit).

    Returns:
        List of (score, metadata) tuples, length <= top_k.
    """
    if not pool or top_k <= 0:
        return []
    if _depth >= GROUP_ASSEMBLY_MAX_DEPTH:
        return pool[:top_k]

    # Take initial top_k candidates
    candidates = pool[:top_k]
    remainder = pool[top_k:]

    # Find group_ids in candidates
    group_ids = set()
    for _, meta in candidates:
        gid = meta.get("group_id")
        if gid:
            group_ids.add(gid)

    if not group_ids:
        return candidates

    # Pull all siblings from remainder into candidates
    pulled = []
    still_remaining = []
    for item in remainder:
        gid = item[1].get("group_id")
        if gid in group_ids:
            pulled.append(item)
        else:
            still_remaining.append(item)

    if not pulled:
        return candidates

    # Merge candidates + pulled siblings
    all_selected = candidates + pulled
    n_selected = len(all_selected)

    # Separate groups and standalone
    groups: dict[str, list] = {}
    standalone = []
    for item in all_selected:
        gid = item[1].get("group_id")
        if gid and gid in group_ids:
            groups.setdefault(gid, []).append(item)
        else:
            standalone.append(item)

    # Sort each group by phase_seq
    for gid in groups:
        groups[gid].sort(key=lambda x: x[1].get("phase_seq") or 0)

    # Interleave: groups at best-score position, standalone by score
    result = []
    used_groups = set()
    all_items = []
    for item in standalone:
        all_items.append(("standalone", item[0], item))
    for gid, members in groups.items():
        best = max(s for s, _ in members)
        all_items.append(("group", best, gid))
    all_items.sort(key=lambda x: x[1], reverse=True)

    for kind, _, payload in all_items:
        if kind == "standalone":
            result.append(payload)
        elif kind == "group" and payload not in used_groups:
            used_groups.add(payload)
            result.extend(groups[payload])

    # If we pulled siblings, we may have more than top_k
    # Fill remaining slots recursively
    if len(result) < top_k and still_remaining:
        fill_count = top_k - len(result)
        fill = _assemble_groups_from_pool(still_remaining, fill_count, _depth + 1)
        result.extend(fill)

    return result[:top_k] if len(result) > top_k else result
```

**Step 4: Run tests to verify they pass**

Run: `cd rune-admin && python -m pytest tests/unit/test_group_assembly.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add vault/vault_core.py tests/unit/test_group_assembly.py
git commit -m "feat: add group assembly with recursion for phase chains"
```

---

### Task 6: Wire It All — Modify _decrypt_scores_impl

**Files:**
- Modify: `rune-admin/vault/vault_core.py` — `_decrypt_scores_impl()`
- Modify: `rune-admin/vault/vault_grpc_server.py` — `DecryptScores()`
- Modify: `rune-admin/tests/unit/test_decrypt_scores.py`

**Step 1: Write failing test for filtered DecryptScores**

Add to `tests/unit/test_decrypt_scores.py`:

```python
def test_filter_domain_reduces_results(self, monkeypatch):
    """DecryptScores with filter_domain should only return matching records."""
    scores = [0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1, 0.05]
    blob = _make_fake_blob()
    self._patch_cipher_and_proto(monkeypatch, _mock_decrypt_score_flat(scores))

    # Mock remind + metadata: alternating architecture/security
    metadata = [
        {"domain": "architecture", "status": "accepted", "timestamp": "2026-03-01T00:00:00Z"},
        {"domain": "security", "status": "accepted", "timestamp": "2026-03-01T00:00:00Z"},
    ] * 5  # 10 entries

    mock_remind = MagicMock(return_value=metadata)
    monkeypatch.setattr('vault_core._remind_and_decrypt_metadata', mock_remind)

    result = decrypt_scores(
        "TOKEN-FOR-DEMONSTRATION-PURPOSES-ONLY-DO-NOT-USE-IN-PRODUCTION",
        blob, top_k=5,
        filter_domain="architecture",
    )
    data = json.loads(result)
    assert isinstance(data, list)
    assert len(data) == 5

def test_no_filters_backward_compatible(self, monkeypatch):
    """DecryptScores without filters should work exactly as before."""
    scores = [0.9, 0.8, 0.7]
    blob = _make_fake_blob()
    self._patch_cipher_and_proto(monkeypatch, _mock_decrypt_score_flat(scores))

    result = decrypt_scores(
        "TOKEN-FOR-DEMONSTRATION-PURPOSES-ONLY-DO-NOT-USE-IN-PRODUCTION",
        blob, top_k=3,
    )
    data = json.loads(result)
    assert isinstance(data, list)
    assert len(data) == 3
```

**Step 2: Run tests to verify they fail**

Run: `cd rune-admin && python -m pytest tests/unit/test_decrypt_scores.py::TestDecryptScores::test_filter_domain_reduces_results -v`
Expected: FAIL — `_decrypt_scores_impl` doesn't accept filter params.

**Step 3: Modify _decrypt_scores_impl signature and logic**

Replace the function in `vault_core.py`:

```python
def _decrypt_scores_impl(
    token: str,
    encrypted_blob_b64: str,
    top_k: int = 5,
    filter_domain: str = "",
    filter_status: str = "",
    filter_since: str = "",
) -> str:
    """
    Core implementation: Decrypts CiphertextScore, applies filtering,
    recency weighting, and group assembly.

    When filters are provided, Vault internally over-fetches
    (top_k * INTERNAL_FETCH_MULTIPLIER), fetches metadata from enVector
    Cloud, applies filters + recency weighting + group assembly, and
    returns exactly top_k results.

    When no filters are provided, behaves identically to the original
    implementation (backward compatible).
    """
    validate_token(token)

    if top_k > 10:
        return json.dumps({"error": "Rate Limit Exceeded: Max top_k is 10"})

    has_filters = bool(filter_domain or filter_status or filter_since)

    try:
        blob_bytes = base64.b64decode(encrypted_blob_b64)
        try:
            score_proto = CiphertextScore()
            score_proto.ParseFromString(blob_bytes)
            encrypted_result = CipherBlock(data=score_proto)
        except Exception as e:
            return json.dumps({"error": f"Deserialization failed: {str(e)}"})

        decrypted = cipher.decrypt_score(encrypted_result, sec_key_path=sec_key_path)
        score_2d = decrypted["score"]
        shard_indices = decrypted.get("shard_idx", list(range(len(score_2d))))

        # Flatten all scores
        all_scores = [
            (shard_indices[i], j, float(v))
            for i, row in enumerate(score_2d)
            for j, v in enumerate(row)
        ]

        if not has_filters:
            # No filters: original fast path (no metadata fetch)
            topk_results = heapq.nlargest(top_k, all_scores, key=lambda x: x[2])
            return json.dumps([
                {"shard_idx": s, "row_idx": r, "score": sc}
                for s, r, sc in topk_results
            ])

        # Over-fetch candidates
        internal_k = min(top_k * INTERNAL_FETCH_MULTIPLIER, len(all_scores))
        candidates = heapq.nlargest(internal_k, all_scores, key=lambda x: x[2])

        # Fetch and decrypt metadata for candidates
        indices = [{"shard_idx": s, "row_idx": r, "score": sc} for s, r, sc in candidates]
        metadata_list = _remind_and_decrypt_metadata(token, indices)

        # Pair scores with metadata
        paired = list(zip([sc for _, _, sc in candidates], metadata_list))

        # Apply filters
        paired = _apply_filters(paired, filter_domain, filter_status, filter_since)

        # Apply recency weighting
        paired = _apply_recency_weighting(paired)

        # Group assembly
        paired = _assemble_groups_from_pool(paired, top_k)

        # Map back to indices for response
        # Build lookup: metadata -> original index entry
        meta_to_index = {}
        for idx_entry, meta in zip(indices, metadata_list):
            meta_to_index[id(meta)] = idx_entry

        result = []
        for score, meta in paired:
            idx_entry = meta_to_index.get(id(meta))
            if idx_entry:
                result.append({
                    "shard_idx": idx_entry["shard_idx"],
                    "row_idx": idx_entry["row_idx"],
                    "score": score,
                })

        return json.dumps(result[:top_k])

    except Exception as e:
        return json.dumps({"error": str(e)})
```

**Step 4: Update gRPC server to pass filter fields**

In `vault_grpc_server.py`, modify the `DecryptScores` method to pass new fields:

```python
result_json = _decrypt_scores_impl(
    request.token,
    request.encrypted_blob_b64,
    request.top_k,
    filter_domain=getattr(request, 'filter_domain', ''),
    filter_status=getattr(request, 'filter_status', ''),
    filter_since=getattr(request, 'filter_since', ''),
)
```

Use `getattr` with defaults for backward compatibility with old proto stubs.

**Step 5: Run all tests**

Run: `cd rune-admin && python -m pytest tests/unit/test_decrypt_scores.py -v`
Expected: ALL PASS (existing tests still pass, new filter tests pass)

**Step 6: Commit**

```bash
git add vault/vault_core.py vault/vault_grpc_server.py tests/unit/test_decrypt_scores.py
git commit -m "feat: wire over-fetch, filtering, weighting, and group assembly into DecryptScores"
```

---

### Task 7: Update MCP Server Comment & Version

**Files:**
- Modify: `rune-admin/vault/README.md` — document new filter fields
- Modify: `rune-admin/CHANGELOG.md` — add entry

**Step 1: Add CHANGELOG entry**

```markdown
## [0.2.4] - 2026-03-24

### Added
- `DecryptScoresRequest` optional filter fields: `filter_domain`, `filter_status`, `filter_since`
- Vault-side over-fetch with configurable multiplier (`VAULT_FETCH_MULTIPLIER`)
- Recency weighting inside Vault (configurable via env vars)
- Group assembly with recursion for phase chains

### Changed
- DecryptScores now internally fetches metadata from enVector Cloud when filters are provided
- Backward compatible: no filters = original fast path
```

**Step 2: Commit**

```bash
git add vault/README.md CHANGELOG.md
git commit -m "docs: document Vault-side filtering in CHANGELOG and README"
```

---

### Task 8: Client Simplification (rune plugin — separate PR)

**Files:**
- Modify: `rune/mcp/adapter/vault_client.py` — pass filters to `decrypt_search_results()`
- Modify: `rune/agents/retriever/searcher.py` — simplify pipeline
- Modify: `rune/mcp/server/server.py` — update comment (line 792)

> **Note:** This task runs in the `rune` repo, not `rune-admin`.
> Only execute after Vault v0.2.4 is deployed and confirmed working.

**Step 1: Update vault_client.py**

Add filter params to `decrypt_search_results()`:

```python
async def decrypt_search_results(
    self,
    encrypted_blob_b64: str,
    top_k: int = 5,
    filter_domain: str = "",
    filter_status: str = "",
    filter_since: str = "",
) -> DecryptResult:
```

Pass these to the gRPC request.

**Step 2: Simplify searcher.py**

When `self._vault` is present, the `search()` method becomes:

```python
async def search(self, query, topk=None, filters=None):
    topk = topk or 10
    all_results = await self._search_with_expansions(query, topk, filters)
    return all_results[:topk]
```

Pass filters through to `_search_via_vault()` → `decrypt_search_results()`.

Remove (or gate behind `if not self._vault`):
- `_apply_metadata_filters()`
- `_expand_phase_chains()`
- `_assemble_groups()`
- `_apply_recency_weighting()`

Keep these methods for the `_search_direct()` (non-Vault) fallback path.

**Step 3: Update MCP server comment**

`mcp/server/server.py` line 792: change "over-fetch, post-filter" to "Vault-side filtering".

**Step 4: Commit**

```bash
git add mcp/adapter/vault_client.py agents/retriever/searcher.py mcp/server/server.py
git commit -m "feat: simplify client pipeline — Vault handles filtering and group assembly"
```
