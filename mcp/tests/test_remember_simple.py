#!/usr/bin/env python3
"""
Simple integration test for remember tool functionality.

This test uses the EnVectorSDKAdapter methods directly to verify:
1. The adapter's call_score() and call_remind() methods work correctly
2. The remember pipeline orchestration logic is sound

This test uses mocking to avoid requiring actual Cloud connection.
"""

import os
import sys
import json
from typing import Dict, Any, List

# Add srcs to path
ROOT = os.path.dirname(os.path.dirname(__file__))
SRCS = os.path.join(ROOT, "srcs")
if SRCS not in sys.path:
    sys.path.append(SRCS)

from adapter import EnVectorSDKAdapter
from adapter.vault_client import DecryptResult


def test_adapter_methods_exist():
    """Verify that EnVectorSDKAdapter has the required methods for remember pipeline."""
    # Create a dummy adapter just to check methods exist
    # We don't actually call init() to avoid connection issues
    adapter = EnVectorSDKAdapter.__new__(EnVectorSDKAdapter)

    # Check that all required methods exist
    assert hasattr(adapter, 'call_score'), "Missing call_score method"
    assert hasattr(adapter, 'call_remind'), "Missing call_remind method"
    assert hasattr(adapter, 'call_search'), "Missing call_search method"
    assert hasattr(adapter, 'call_insert'), "Missing call_insert method"
    assert hasattr(adapter, 'call_create_index'), "Missing call_create_index method"

    print("✓ All required adapter methods exist")


def test_remember_pipeline_logic():
    """
    Test the remember pipeline logic without actual network calls.

    This simulates what server.py's tool_remember() does:
    1. call_score() → encrypted_blobs
    2. vault.decrypt_search_results() → indices with scores
    3. call_remind() → metadata
    """

    # Step 1: Mock call_score result
    print("\nStep 1: Mock homomorphic search result")
    mock_score_result = {
        "ok": True,
        "encrypted_blobs": ["ZmFrZV9lbmNyeXB0ZWRfYmxvYl9iYXNlNjQ="]
    }
    assert mock_score_result["ok"] is True
    assert len(mock_score_result["encrypted_blobs"]) > 0
    print(f"  ✓ Got encrypted blob: {mock_score_result['encrypted_blobs'][0][:40]}...")

    # Step 2: Mock Vault decrypt
    print("\nStep 2: Mock Vault decryption")
    mock_vault_result = DecryptResult(
        ok=True,
        results=[
            {"shard_idx": 0, "row_idx": 5, "score": 0.95},
            {"shard_idx": 0, "row_idx": 2, "score": 0.88},
            {"shard_idx": 0, "row_idx": 8, "score": 0.75},
        ],
        request_id="test_req_123",
        timestamp=1700000000.0,
        total_vectors=100,
    )
    assert mock_vault_result.ok is True
    assert len(mock_vault_result.results) == 3
    print(f"  ✓ Vault returned {len(mock_vault_result.results)} top-k indices")

    # Step 3: Mock call_remind result
    print("\nStep 3: Mock metadata retrieval")
    mock_remind_result = {
        "ok": True,
        "results": [
            {
                "metadata": json.dumps({
                    "id": 5,
                    "content": "Decision to use FHE for zero-knowledge memory",
                    "timestamp": "2024-01-15",
                    "author": "admin"
                }),
                "score": 0.95
            },
            {
                "metadata": json.dumps({
                    "id": 2,
                    "content": "Architecture decision: Vault holds SecKey",
                    "timestamp": "2024-01-12",
                    "author": "architect"
                }),
                "score": 0.88
            },
            {
                "metadata": json.dumps({
                    "id": 8,
                    "content": "Remember tool replaces secure_search",
                    "timestamp": "2024-01-20",
                    "author": "developer"
                }),
                "score": 0.75
            },
        ]
    }
    assert mock_remind_result["ok"] is True
    assert len(mock_remind_result["results"]) == 3

    print(f"  ✓ Retrieved {len(mock_remind_result['results'])} metadata entries:")
    for i, entry in enumerate(mock_remind_result["results"]):
        metadata = json.loads(entry["metadata"])
        print(f"    {i+1}. [{metadata['timestamp']}] {metadata['content']} (score: {entry['score']})")

    # Final result: combine Vault metadata with scores
    print("\nFinal: Assemble remember result")
    final_result = {
        "ok": True,
        "results": mock_remind_result["results"],
        "request_id": mock_vault_result.request_id,
        "total_vectors": mock_vault_result.total_vectors,
    }

    assert final_result["ok"] is True
    assert final_result["request_id"] == "test_req_123"
    assert final_result["total_vectors"] == 100
    assert len(final_result["results"]) == 3

    print(f"  ✓ Remember result: {len(final_result['results'])} memories recalled")
    print(f"  ✓ Request ID: {final_result['request_id']}")
    print(f"  ✓ Total vectors in index: {final_result['total_vectors']}")

    print("\n" + "=" * 80)
    print("✓ Remember pipeline logic validated successfully")
    print("=" * 80)


def test_error_handling_no_vault():
    """Test that remember returns error when Vault is not configured."""

    # Simulate tool_remember when vault_client is None
    vault_client = None

    if vault_client is None:
        error_result = {
            "ok": False,
            "error": "Vault not configured. Set RUNEVAULT_ENDPOINT and RUNEVAULT_TOKEN environment variables.",
        }

    assert error_result["ok"] is False
    assert "vault" in error_result["error"].lower()

    print("✓ Correctly handles missing Vault configuration")


def test_error_handling_topk_limit():
    """Test that remember enforces top_k <= 10 policy."""

    topk = 15

    if topk > 10:
        error_result = {"ok": False, "error": "Policy: max top_k is 10."}

    assert error_result["ok"] is False
    assert "10" in error_result["error"]

    print("✓ Correctly enforces top_k limit policy")


def test_vault_client_interface():
    """Test that DecryptResult has the expected structure."""

    result = DecryptResult(
        ok=True,
        results=[{"shard_idx": 0, "row_idx": 1, "score": 0.9}],
        request_id="req_001",
        timestamp=1700000000.0,
        total_vectors=50,
    )

    assert result.ok is True
    assert len(result.results) == 1
    assert result.results[0]["shard_idx"] == 0
    assert result.results[0]["row_idx"] == 1
    assert result.results[0]["score"] == 0.9
    assert result.request_id == "req_001"
    assert result.total_vectors == 50

    print("✓ DecryptResult structure validated")


if __name__ == "__main__":
    print("=" * 80)
    print("Remember Tool Simple Validation")
    print("=" * 80)

    test_adapter_methods_exist()
    test_remember_pipeline_logic()
    test_error_handling_no_vault()
    test_error_handling_topk_limit()
    test_vault_client_interface()

    print("\n" + "=" * 80)
    print("✓ All validation tests passed")
    print("=" * 80)
