#!/usr/bin/env python3
"""
Integration test for the remember pipeline (Cloud + Vault).

All 3 parties are required: Client, enVector Cloud, Rune-Vault.

Required environment variables:
  ENVECTOR_ENDPOINT    enVector Cloud endpoint
  ENVECTOR_API_KEY     enVector Cloud API key
  RUNEVAULT_ENDPOINT   Rune-Vault endpoint
  RUNEVAULT_TOKEN      Rune-Vault access token
"""

import os
import sys
import json
import asyncio
import pytest
from typing import Dict, Any, List

# Add srcs to path
ROOT = os.path.dirname(os.path.dirname(__file__))
SRCS = os.path.join(ROOT, "srcs")
if SRCS not in sys.path:
    sys.path.append(SRCS)

from adapter import EnVectorSDKAdapter
from fastmcp import Client

# --- Credentials (all required) ---
ENVECTOR_ENDPOINT = os.environ.get("ENVECTOR_ENDPOINT") or os.environ.get("ENVECTOR_ADDRESS", "")
ENVECTOR_API_KEY = os.environ.get("ENVECTOR_API_KEY", "")
RUNEVAULT_ENDPOINT = os.environ.get("RUNEVAULT_ENDPOINT", "")
RUNEVAULT_TOKEN = os.environ.get("RUNEVAULT_TOKEN", "")

_missing = [k for k, v in {
    "ENVECTOR_ENDPOINT": ENVECTOR_ENDPOINT,
    "ENVECTOR_API_KEY": ENVECTOR_API_KEY,
    "RUNEVAULT_ENDPOINT": RUNEVAULT_ENDPOINT,
    "RUNEVAULT_TOKEN": RUNEVAULT_TOKEN,
}.items() if not v]
if _missing:
    pytest.skip(f"Missing required env vars: {', '.join(_missing)}", allow_module_level=True)

TEST_INDEX = "rune_integ_test"
TEST_DIM = 128

VAULT_MCP_URL = RUNEVAULT_ENDPOINT.rstrip("/")
if not VAULT_MCP_URL.endswith("/mcp"):
    VAULT_MCP_URL += "/mcp"


# ---- helpers ----

async def vault_decrypt(blob: str, top_k: int) -> list:
    """Decrypt scoring ciphertext via Vault, return list of {shard_idx, row_idx, score}."""
    async with Client(VAULT_MCP_URL) as client:
        result = await client.call_tool("decrypt_scores", {
            "token": RUNEVAULT_TOKEN,
            "encrypted_blob_b64": blob,
            "top_k": top_k,
        })
        text = result[0].text if isinstance(result, list) else result.content[0].text
        data = json.loads(text)
        return data["results"] if isinstance(data, dict) and "results" in data else data


# ---- fixtures ----

@pytest.fixture(scope="module")
def adapter():
    """Real EnVectorSDKAdapter connected to Cloud."""
    try:
        return EnVectorSDKAdapter(
            address=ENVECTOR_ENDPOINT,
            key_id="integ_key",
            key_path="/tmp/integ_keys",
            eval_mode="rmp",
            query_encryption=False,
            access_token=ENVECTOR_API_KEY,
            auto_key_setup=True,
        )
    except Exception as e:
        pytest.skip(f"Cannot connect to enVector Cloud: {e}")


@pytest.fixture(scope="module")
def index(adapter):
    """Create index, insert vectors, yield name, then cleanup."""
    adapter.call_create_index(index_name=TEST_INDEX, dim=TEST_DIM, index_params={"index_type": "FLAT"})

    vectors = [[0.1 * (i + 1)] * TEST_DIM for i in range(5)]
    metadata = [json.dumps({"id": i, "content": f"Test memory {i}", "ts": f"2024-01-{i+1:02d}"}) for i in range(5)]
    adapter.call_insert(index_name=TEST_INDEX, vectors=vectors, metadata=metadata)

    yield TEST_INDEX

    # cleanup
    import pyenvector as ev
    from pyenvector.client.client import pyenvector_client as pc
    for fn, args in [
        (pc.indexer.unload_index, (TEST_INDEX,)),
        (pc.unload_key, ()),
        (ev.drop_index, (TEST_INDEX,)),
        (ev.delete_key, ("integ_key",)),
        (ev.disconnect, ()),
    ]:
        try:
            fn(*args)
        except Exception:
            pass


# ---- tests ----

def test_call_score(adapter, index):
    """call_score returns base64 encrypted blob."""
    result = adapter.call_score(index_name=index, query=[0.25] * TEST_DIM)
    assert result["ok"] is True
    blobs = result["encrypted_blobs"]
    assert len(blobs) > 0 and len(blobs[0]) > 0


def test_call_remind(adapter, index):
    """score → vault decrypt → call_remind returns metadata."""
    blob = adapter.call_score(index_name=index, query=[0.25] * TEST_DIM)["encrypted_blobs"][0]
    indices = asyncio.run(vault_decrypt(blob, top_k=2))
    assert len(indices) >= 1

    result = adapter.call_remind(index_name=index, indices=indices, output_fields=["metadata"])
    assert result["ok"] is True
    assert len(result["results"]) == len(indices)


def test_full_pipeline(adapter, index):
    """Full 3-step remember pipeline: score → vault → remind."""
    # Step 1
    score_result = adapter.call_score(index_name=index, query=[0.35] * TEST_DIM)
    assert score_result["ok"] is True
    blob = score_result["encrypted_blobs"][0]

    # Step 2
    indices = asyncio.run(vault_decrypt(blob, top_k=3))
    assert len(indices) > 0

    # Step 3
    remind_result = adapter.call_remind(index_name=index, indices=indices, output_fields=["metadata"])
    assert remind_result["ok"] is True
    assert len(remind_result["results"]) == len(indices)


def test_score_nonexistent_index(adapter):
    """call_score gracefully fails for nonexistent index."""
    result = adapter.call_score(index_name="no_such_index_99999", query=[0.1] * TEST_DIM)
    assert result["ok"] is False
    assert "error" in result


def test_remind_empty_indices(adapter, index):
    """call_remind handles empty indices list."""
    result = adapter.call_remind(index_name=index, indices=[], output_fields=["metadata"])
    if result.get("ok"):
        assert result.get("results", []) == []
    else:
        assert "error" in result


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-s"])
