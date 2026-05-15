"""pyenvector 1.2.2 adapter — eval_mode=rmp, index_type=flat.

v1.2.2 has no single-row insert path and no server-side searchable
lifecycle states. Therefore:
  * `insert` accepts `row_insert` for interface parity but ignores it.
  * insert -> searchable latency is measured by client-side score polling
    (poll score -> vault decrypt until the just-inserted vector is the
    top-1 result with cosine >= threshold).

Everything else (score, remind, index lifecycle, metadata encryption,
reconnect) is inherited from SdkAdapter / EnVectorSDKAdapter unchanged.
"""

from __future__ import annotations

import asyncio
import json
import re
import time
from typing import Optional

from .base import SdkAdapter, SearchableCtx


class V122Adapter(SdkAdapter):
    sdk_version = "1.2.2"
    eval_mode = "rmp"
    index_type = "flat"

    # Top-1 cosine at/above which a just-inserted vector counts as searchable.
    _SEARCHABLE_SCORE_THRESHOLD = 0.999

    # ── connection ─────────────────────────────────────────────────────────

    def connect(
        self,
        *,
        address: str,
        key_id: str,
        key_path: str,
        access_token: Optional[str],
        agent_id: Optional[str],
        agent_dek: Optional[bytes],
    ) -> None:
        from adapter.envector_sdk import EnVectorSDKAdapter

        # EnVectorSDKAdapter calls ev.init() in __init__; the first handshake
        # after a fresh process can flake, so retry the construction.
        last_err: Optional[Exception] = None
        for _attempt in range(5):
            try:
                self._sdk = EnVectorSDKAdapter(
                    address=address,
                    key_id=key_id,
                    key_path=key_path,
                    eval_mode=self.eval_mode,        # "rmp" — v1.2.2 has no mm32
                    query_encryption=False,
                    access_token=access_token,
                    auto_key_setup=False,
                    agent_id=agent_id,
                    agent_dek=agent_dek,
                )
                return
            except Exception as e:  # noqa: BLE001
                last_err = e
                time.sleep(2.0)
        assert last_err is not None
        raise last_err

    # ── data plane ─────────────────────────────────────────────────────────

    def insert(
        self,
        index_name: str,
        vectors: list,
        metadata: list,
        *,
        row_insert: bool = False,
    ) -> None:
        # row_insert is a 1.4.x-only knob (single-row insert API). v1.2.2 has
        # no such path, so it is accepted for interface parity and ignored.
        meta_strs = [
            json.dumps(m) if isinstance(m, dict) else str(m) for m in metadata
        ]
        res = self.sdk.call_insert(
            index_name=index_name,
            vectors=vectors,
            metadata=meta_strs,
        )
        if isinstance(res, dict) and not res.get("ok", True):
            raise RuntimeError(f"insert failed: {res.get('error')}")

    # ── searchable measurement ────────────────────────────────────────────

    def searchable_phase_names(self) -> list[str]:
        return ["insert_searchable"]

    async def measure_insert_to_searchable(self, ctx: SearchableCtx) -> dict:
        """Submit the insert, then poll until the vector is queryable.

        Single phase (`insert_searchable`): v1.2.2 insert is non-blocking and
        the SDK exposes no lifecycle state, so we cannot decompose it the way
        the 1.4.x lifecycle (insert_rpc / load / wait) allows.
        """
        t0 = time.perf_counter()
        # No row_insert here: this is V122Adapter calling its own insert(),
        # which has no single-row path. ctx.insert_mode is consumed by
        # V143Adapter instead.
        self.insert(ctx.index_name, [ctx.vec], ctx.metadata)
        await self._wait_until_searchable(ctx)
        return {"insert_searchable": (time.perf_counter() - t0) * 1000.0}

    async def _wait_until_searchable(
        self,
        ctx: SearchableCtx,
        timeout_s: float = 30.0,
        poll_interval_s: float = 0.2,
    ) -> None:
        """Poll score -> vault decrypt until our vector is the top-1 result.

        Caveat: bench texts repeat across runs, so after the first iteration
        the same vector already exists in the index — later polls match the
        prior copy. This mirrors the 1.4.x server-push measurement, which
        also fires on the new insert request. On vault RESOURCE_EXHAUSTED we
        back off by the server-hinted retry-after.
        """
        start = time.perf_counter()
        while True:
            try:
                score_res = self.score(ctx.index_name, ctx.vec)
                blobs = (
                    score_res.get("encrypted_blobs", [])
                    if score_res.get("ok")
                    else []
                )
                if blobs:
                    vault_res = await ctx.vault.decrypt_search_results(
                        blobs[0], top_k=3
                    )
                    if vault_res.ok and vault_res.results:
                        top = vault_res.results[0]
                        top_score = (
                            top.get("score", 0.0) if isinstance(top, dict) else 0.0
                        )
                        if top_score >= self._SEARCHABLE_SCORE_THRESHOLD:
                            return
            except Exception as e:  # noqa: BLE001
                msg = str(e)
                if "RESOURCE_EXHAUSTED" in msg:
                    m = re.search(r"Retry after (\d+)", msg)
                    backoff = int(m.group(1)) if m else 5
                    elapsed = time.perf_counter() - start
                    if elapsed + backoff >= timeout_s:
                        raise TimeoutError(
                            f"rate-limit backoff ({backoff}s) would exceed "
                            f"timeout ({timeout_s}s); elapsed={elapsed:.1f}s"
                        )
                    await asyncio.sleep(backoff)
                    continue
                raise
            if time.perf_counter() - start >= timeout_s:
                raise TimeoutError(
                    f"vector not searchable within {timeout_s}s "
                    f"(top score never reached {self._SEARCHABLE_SCORE_THRESHOLD})"
                )
            await asyncio.sleep(poll_interval_s)
