# tests/test_server.py
import os
import sys
import pytest
from unittest.mock import patch

from typing import Union, List, Any, Dict, Optional

import numpy as np

# Add mcp directory (rune/mcp/) to import path
MCP_ROOT = os.path.dirname(os.path.dirname(__file__))
if MCP_ROOT not in sys.path:
    sys.path.insert(0, MCP_ROOT)

from fastmcp import Client
from server.server import MCPServerApp
from adapter import EnVectorSDKAdapter, EmbeddingAdapter
from adapter.vault_client import VaultClient, DecryptResult

# embedding fake adapter
class FakeEmbeddingAdapter(EmbeddingAdapter):
    def __init__(self):
        pass  # Actual initialization not needed

    # ----------- Mocked method: get_embedding ----------- #
    def get_embedding(self, texts: List[str]) -> np.ndarray:
        # Return a fake response
        #   - Expected Return Type: List[Dict[str, Any]]
        return np.array([[0.1, 0.2, 0.3] * (i+1) for i in range(len(texts))])

@pytest.fixture
def mcp_server():
    """
    Create and return a FastMCP server instance for testing.
    Inject a fake adapter to avoid using the actual enVector SDK.
    """
    class FakeAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass  # Actual initialization not needed

        # ----------- Mocked method: Get Index List ----------- #
        def invoke_get_index_list(self) -> List[str]:
            return ["index_a", "index_b"]

        # ----------- Mocked method: Get Index Info ----------- #
        def invoke_get_index_info(self, index_name: str) -> Dict[str, Any]:
            if index_name not in ("index_a", "index_b"):
                raise ValueError(f"Index '{index_name}' not found")
            return {"index_name": index_name, "dim": 128, "row_count": 42}

        # ----------- Mocked method: Create Index ----------- #
        def invoke_create_index(self, index_name: str, dim: int, index_params: Dict[str, Any] = None) -> Dict[str, Any]:
            if index_params is not None and not isinstance(index_params, dict):
                raise TypeError("index_params must be a dict or None")
            return {"index_name": index_name, "dim": dim, "index_params": index_params}

        # ----------- Mocked method: Insert ----------- #
        def invoke_insert(
                self,
                index_name: str,
                vectors: List[List[float]],
                metadata: Union[Any, List[Any]] = None
            ) -> Dict[str, Any]:
            return {"index_name": index_name, "vectors": vectors, "metadata": metadata}

        # ----------- Mocked method: Search ----------- #
        def invoke_search(self, index_name: str, query: Union[List[float], List[List[float]]], topk: int) -> List[Dict[str, Any]]:
            # Return a fake response
            #   - Expected Return Type: List[Dict[str, Any]]
            return [{"id": 1, "score": 0.9, "metadata": {"fieldA": "valueA"}}]

    app = MCPServerApp(envector_adapter=FakeAdapter(), mcp_server_name="test-mcp", embedding_adapter=FakeEmbeddingAdapter())
    return app.mcp  # FastMCP Instance


# ----------- Low-Level Tool Removal Verification ----------- #
@pytest.mark.asyncio
async def test_low_level_tools_not_registered(mcp_server):
    """Low-level enVector tools must never be exposed."""
    async with Client(mcp_server) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        forbidden = ["create_index", "get_index_list", "get_index_info",
                      "insert", "insert_documents_from_path",
                      "insert_documents_from_text", "search"]
        for tool_name in forbidden:
            assert tool_name not in names, f"{tool_name} should not be registered"


# =========================================================================== #
#  Fake Vault Client for testing remember / vault_status tools
# =========================================================================== #

class FakeVaultClient(VaultClient):
    """Fake Vault client that returns deterministic results without network calls."""
    def __init__(self):
        # Skip real __init__ to avoid network setup
        self.vault_endpoint = "http://fake-vault:50080"
        self.vault_token = "fake-token"
        self.timeout = 5.0
        self._grpc_target = "fake-vault:50051"
        self._channel = None
        self._stub = None

    async def get_public_key(self) -> dict:
        return {"EncKey.json": "{}", "EvalKey.json": "{}", "index_name": "team-decisions"}

    async def health_check(self) -> bool:
        return True

    async def decrypt_search_results(
        self,
        encrypted_blob_b64: str,
        top_k: int = 5,
    ) -> DecryptResult:
        return DecryptResult(
            ok=True,
            results=[
                {"shard_idx": 0, "row_idx": 0, "score": 0.95},
                {"shard_idx": 0, "row_idx": 1, "score": 0.80},
            ][:top_k],
        )

    async def decrypt_metadata(
        self,
        encrypted_metadata_list: List[str],
    ) -> List:
        return [f"decrypted_{i}" for i in range(len(encrypted_metadata_list))]


@pytest.fixture
def mcp_server_with_vault():
    """
    MCP server fixture with a fake Vault client injected,
    enabling the `remember` and `vault_status` tools.
    """
    class FakeAdapterWithVault(EnVectorSDKAdapter):
        def __init__(self):
            pass

        # --- existing mocked methods (same as FakeAdapter) ---
        def invoke_get_index_list(self) -> List[str]:
            return ["index_a", "index_b"]

        def invoke_get_index_info(self, index_name: str) -> Dict[str, Any]:
            if index_name not in ("index_a", "index_b"):
                raise ValueError(f"Index '{index_name}' not found")
            return {"index_name": index_name, "dim": 128, "row_count": 42}

        def invoke_create_index(self, index_name: str, dim: int, index_params: Dict[str, Any] = None) -> Dict[str, Any]:
            return {"index_name": index_name, "dim": dim, "index_params": index_params}

        def invoke_insert(self, index_name: str, vectors, metadata=None):
            return {"index_name": index_name, "vectors": vectors, "metadata": metadata}

        def invoke_search(self, index_name: str, query, topk: int):
            return [{"id": 1, "score": 0.9, "metadata": {"fieldA": "valueA"}}]

        # --- new remember (score + remind) pipeline mocks ---
        def call_score(self, index_name: str, query) -> Dict[str, Any]:
            return {"ok": True, "encrypted_blobs": ["ZmFrZV9ibG9i"]}

        def call_remind(self, index_name: str, indices: List[Dict[str, Any]], output_fields=None) -> Dict[str, Any]:
            results = [{"metadata": f"memory_{entry['shard_idx']}_{entry['row_idx']}", "score": entry.get("score", 0.0)} for entry in indices]
            return {"ok": True, "results": results}

    app = MCPServerApp(
        envector_adapter=FakeAdapterWithVault(),
        mcp_server_name="test-mcp-vault",
        embedding_adapter=FakeEmbeddingAdapter(),
        vault_client=FakeVaultClient(),
        vault_index_name="team-decisions",
    )
    return app.mcp


@pytest.fixture
def mcp_server_with_vault_no_index():
    """
    MCP server fixture with Vault but NO team index name configured.
    Tests the case where old Vault doesn't provide index_name.
    """
    class FakeAdapterWithVault(EnVectorSDKAdapter):
        def __init__(self):
            pass

        def invoke_get_index_list(self) -> List[str]:
            return ["index_a", "index_b"]

        def invoke_get_index_info(self, index_name: str) -> Dict[str, Any]:
            if index_name not in ("index_a", "index_b"):
                raise ValueError(f"Index '{index_name}' not found")
            return {"index_name": index_name, "dim": 128, "row_count": 42}

        def invoke_create_index(self, index_name: str, dim: int, index_params: Dict[str, Any] = None) -> Dict[str, Any]:
            return {"index_name": index_name, "dim": dim, "index_params": index_params}

        def invoke_insert(self, index_name: str, vectors, metadata=None):
            return {"index_name": index_name, "vectors": vectors, "metadata": metadata}

        def invoke_search(self, index_name: str, query, topk: int):
            return [{"id": 1, "score": 0.9, "metadata": {"fieldA": "valueA"}}]

        def call_score(self, index_name: str, query) -> Dict[str, Any]:
            return {"ok": True, "encrypted_blobs": ["ZmFrZV9ibG9i"]}

        def call_remind(self, index_name: str, indices: List[Dict[str, Any]], output_fields=None) -> Dict[str, Any]:
            results = [{"metadata": f"memory_{entry['shard_idx']}_{entry['row_idx']}", "score": entry.get("score", 0.0)} for entry in indices]
            return {"ok": True, "results": results}

    app = MCPServerApp(
        envector_adapter=FakeAdapterWithVault(),
        mcp_server_name="test-mcp-vault-no-index",
        embedding_adapter=FakeEmbeddingAdapter(),
        vault_client=FakeVaultClient(),
        vault_index_name=None,
    )
    return app.mcp


# ----------- Vault Status Tool Tests ----------- #

@pytest.mark.asyncio
async def test_tools_list_contains_vault_status(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        assert "vault_status" in names


@pytest.mark.asyncio
async def test_vault_status_with_vault_configured(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool("vault_status", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True
        assert data.get("vault_configured") is True
        assert data.get("secure_search_available") is True
        assert data.get("mode") == "secure (Vault-backed)"


@pytest.mark.asyncio
async def test_vault_status_without_vault(mcp_server):
    """When no vault_client is injected the tool should report standard mode."""
    async with Client(mcp_server) as client:
        result = await client.call_tool("vault_status", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True
        assert data.get("vault_configured") is False
        assert data.get("secure_search_available") is False


# ----------- Remember Tool Tests ----------- #

@pytest.mark.asyncio
async def test_tools_list_contains_remember(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        assert "remember" in names


@pytest.mark.asyncio
async def test_remember_happy_path(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool(
            "remember",
            {
                "index_name": "index_a",
                "query": [0.1, 0.2, 0.3],
                "topk": 2,
                "request_id": "test_req_001",
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True
        assert isinstance(data.get("results"), list)
        assert len(data["results"]) == 2
        assert data.get("request_id") == "test_req_001"


@pytest.mark.asyncio
async def test_remember_with_text_query(mcp_server_with_vault):
    """When a text query is given, the embedding adapter should be used."""
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool(
            "remember",
            {
                "index_name": "index_a",
                "query": "what was the last decision?",
                "topk": 1,
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True


@pytest.mark.asyncio
async def test_remember_topk_exceeds_limit(mcp_server_with_vault):
    """topk > 10 should be rejected by policy."""
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool(
            "remember",
            {
                "index_name": "index_a",
                "query": [0.1, 0.2, 0.3],
                "topk": 11,
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is False
        assert "max" in data.get("error", "").lower() or "10" in data.get("error", "")


@pytest.mark.asyncio
async def test_remember_without_vault(mcp_server):
    """When vault is not configured, remember should return an error."""
    async with Client(mcp_server) as client:
        result = await client.call_tool(
            "remember",
            {
                "index_name": "index_a",
                "query": [0.1, 0.2, 0.3],
                "topk": 3,
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is False
        assert "vault" in data.get("error", "").lower()


# ----------- Vault Index Name Tests ----------- #

@pytest.mark.asyncio
async def test_remember_uses_vault_index_when_omitted(mcp_server_with_vault):
    """When index_name is omitted, the vault-provisioned team index should be used."""
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool(
            "remember",
            {
                "query": [0.1, 0.2, 0.3],
                "topk": 2,
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True
        assert isinstance(data.get("results"), list)


@pytest.mark.asyncio
async def test_remember_explicit_index_overrides_vault(mcp_server_with_vault):
    """When index_name is explicitly provided, it should override the vault default."""
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool(
            "remember",
            {
                "index_name": "index_a",
                "query": [0.1, 0.2, 0.3],
                "topk": 2,
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True


@pytest.mark.asyncio
async def test_remember_fails_when_no_index_and_no_vault(mcp_server_with_vault_no_index):
    """When no vault index is configured and index_name is omitted, should return error."""
    async with Client(mcp_server_with_vault_no_index) as client:
        result = await client.call_tool(
            "remember",
            {
                "query": [0.1, 0.2, 0.3],
                "topk": 2,
            }
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is False
        assert "index_name" in data.get("error", "").lower() or "required" in data.get("error", "").lower()


@pytest.mark.asyncio
async def test_vault_status_includes_team_index_name(mcp_server_with_vault):
    """vault_status should expose the team_index_name field."""
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool("vault_status", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None, "No data returned from tool call"
        assert data.get("ok") is True
        assert data.get("team_index_name") == "team-decisions"


# ----------- Remember / Vault Status / Index Name Tool Tests Finished ----------- #


# ----------- Degraded Mode Tests (no enVector adapter) ----------- #

@pytest.fixture
def mcp_server_degraded():
    """MCP server with envector_adapter=None, simulating startup when infra is down."""
    app = MCPServerApp(mcp_server_name="test-mcp-degraded")
    return app.mcp


@pytest.mark.asyncio
async def test_degraded_server_starts_and_lists_tools(mcp_server_degraded):
    """Server should start and register all tools even without enVector adapter."""
    async with Client(mcp_server_degraded) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        assert "reload_pipelines" in names
        assert "remember" in names
        assert "capture" in names
        assert "recall" in names
        assert "vault_status" in names


@pytest.mark.asyncio
async def test_remember_returns_error_when_no_adapter(mcp_server_degraded):
    """remember tool should return graceful error when enVector adapter is None."""
    async with Client(mcp_server_degraded) as client:
        result = await client.call_tool(
            "remember",
            {"index_name": "test", "query": [0.1, 0.2], "topk": 1},
        )
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)
        assert data is not None
        assert data.get("ok") is False
        assert "adapter" in data.get("error", "").lower()


# ----------- Reload Pipelines Tool Tests ----------- #

@pytest.mark.asyncio
async def test_tools_list_contains_reload_pipelines(mcp_server):
    async with Client(mcp_server) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        assert "reload_pipelines" in names


@pytest.mark.asyncio
async def test_reload_pipelines_without_active_config(mcp_server):
    """When state is not active, pipelines should remain None."""
    async with Client(mcp_server) as client:
        with patch.dict(os.environ, {"RUNE_STATE": "dormant"}):
            result = await client.call_tool("reload_pipelines", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        assert data.get("scribe_initialized") is False
        assert data.get("retriever_initialized") is False
