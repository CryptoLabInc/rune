#!/usr/bin/env python3
"""
Direct Vault Test using FastMCP Client (Legacy)

This test bypasses vault_client.py and uses FastMCP Client directly
to validate that the Vault MCP (legacy HTTP) server works correctly.
For gRPC tests, use vault_client.py directly.

Required environment variables:
  RUNEVAULT_ENDPOINT  Rune-Vault gRPC target (e.g. vault-host:50051)
  RUNEVAULT_TOKEN     Vault authentication token
"""

import os
import sys
import json
import asyncio

# Add srcs to path
ROOT = os.path.dirname(os.path.dirname(__file__))
SRCS = os.path.join(ROOT, "srcs")
if SRCS not in sys.path:
    sys.path.append(SRCS)

from fastmcp import Client

# Configuration (from environment — no hardcoded credentials)
RUNEVAULT_ENDPOINT = os.environ.get("RUNEVAULT_ENDPOINT", "")
RUNEVAULT_TOKEN = os.environ.get("RUNEVAULT_TOKEN", "")
import pytest

if not RUNEVAULT_ENDPOINT or not RUNEVAULT_TOKEN:
    pytest.skip("Missing required environment variables: RUNEVAULT_ENDPOINT, RUNEVAULT_TOKEN", allow_module_level=True)
# Ensure endpoint ends with /mcp
if not RUNEVAULT_ENDPOINT.endswith("/mcp"):
    RUNEVAULT_ENDPOINT = RUNEVAULT_ENDPOINT.rstrip("/") + "/mcp"

# Sample encrypted blob (from previous test)
SAMPLE_BLOB = "Cg1pZC05ODZhMTdhNGFlEgKHARqogAQIgCASoYAEAAUAAAAAAAAABQAAAAAA"

print("=" * 80)
print("Direct Vault Test with FastMCP Client")
print("=" * 80)
print(f"Vault Endpoint: {RUNEVAULT_ENDPOINT}")
print("=" * 80)

@pytest.mark.asyncio
async def test_vault_tools():
    """Test Vault MCP tools using FastMCP Client."""

    print("\n[1/3] Connecting to Vault MCP...")
    try:
        async with Client(RUNEVAULT_ENDPOINT) as client:
            print("  ✓ Connected to Vault MCP")

            # List available tools
            print("\n[2/3] Listing available tools...")
            tools = await client.list_tools()
            print(f"  ✓ Found {len(tools)} tools:")
            for tool in tools:
                print(f"    - {tool.name}: {tool.description[:60]}...")

            # Test decrypt_scores
            print("\n[3/3] Testing decrypt_scores tool...")
            result = await client.call_tool(
                "decrypt_scores",
                {
                    "token": RUNEVAULT_TOKEN,
                    "encrypted_blob_b64": SAMPLE_BLOB,
                    "top_k": 3
                }
            )

            print(f"  ✓ decrypt_scores returned successfully")
            print(f"  → Result type: {type(result)}")

            # Extract result data
            if hasattr(result, 'content'):
                content = result.content
                if content and len(content) > 0:
                    text = content[0].text if hasattr(content[0], 'text') else str(content[0])
                    print(f"  → Content: {text[:200]}...")

                    # Try to parse as JSON
                    try:
                        data = json.loads(text)
                        print(f"\n  📊 Decryption Result:")
                        print(f"    - Status: {'SUCCESS' if data.get('ok') else 'FAILED'}")
                        if data.get('ok'):
                            results = data.get('results', [])
                            print(f"    - Results count: {len(results)}")
                            print(f"    - Request ID: {data.get('request_id')}")
                            print(f"    - Total vectors: {data.get('total_vectors')}")
                            print(f"\n    Top-{len(results)} results:")
                            for i, entry in enumerate(results):
                                shard = entry.get('shard_idx', '?')
                                row = entry.get('row_idx', '?')
                                score = entry.get('score', 0.0)
                                print(f"      {i+1}. shard={shard}, row={row}, score={score:.4f}")
                        else:
                            print(f"    - Error: {data.get('error')}")
                    except json.JSONDecodeError:
                        print(f"  → Raw text: {text}")
            elif hasattr(result, 'data'):
                print(f"  → Data: {result.data}")
            else:
                print(f"  → Result: {result}")

    except Exception as e:
        print(f"  ✗ Error: {e}")
        import traceback
        traceback.print_exc()
        return False

    return True

# Run the test
print("\nRunning async test...")
success = asyncio.run(test_vault_tools())

print("\n" + "=" * 80)
if success:
    print("✓ VAULT TEST PASSED")
    print("=" * 80)
    print("\nVault MCP server is working correctly!")
    print("Next: Update vault_client.py to use FastMCP Client")
else:
    print("✗ VAULT TEST FAILED")
    print("=" * 80)
