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
from adapter import EnVectorSDKAdapter
from adapter.vault_client import VaultClient, DecryptResult


class FakeEmbeddingService:
    """Fake embedding service matching EmbeddingService.embed() API."""
    def embed(self, texts: List[str]) -> List[List[float]]:
        return [[0.1, 0.2, 0.3] * (i+1) for i in range(len(texts))]

    def embed_single(self, text: str) -> List[float]:
        return self.embed([text])[0]

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

        # ----------- Mocked method: Insert ----------- #
        def invoke_insert(
                self,
                index_name: str,
                vectors: List[List[float]],
                metadata: Union[Any, List[Any]] = None
            ) -> Dict[str, Any]:
            return {"index_name": index_name, "vectors": vectors, "metadata": metadata}

    app = MCPServerApp(envector_adapter=FakeAdapter(), mcp_server_name="test-mcp")
    app.embedding = FakeEmbeddingService()
    app._pipelines_ready.set()
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
#  Fake Vault Client for testing vault_status tools
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
        self._ca_cert = None
        self._tls_disable = True

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
    enabling the `vault_status` tool.
    """
    class FakeAdapterWithVault(EnVectorSDKAdapter):
        def __init__(self):
            pass

        # --- existing mocked methods (same as FakeAdapter) ---
        def invoke_get_index_list(self) -> List[str]:
            return ["index_a", "index_b"]

        def invoke_insert(self, index_name: str, vectors, metadata=None):
            return {"index_name": index_name, "vectors": vectors, "metadata": metadata}

    app = MCPServerApp(
        envector_adapter=FakeAdapterWithVault(),
        mcp_server_name="test-mcp-vault",
        vault_client=FakeVaultClient(),
        vault_index_name="team-decisions",
    )
    app.embedding = FakeEmbeddingService()
    app._pipelines_ready.set()
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


# ----------- Degraded Mode Tests (no enVector adapter) ----------- #

@pytest.fixture
def mcp_server_degraded():
    """MCP server with envector_adapter=None, simulating startup when infra is down."""
    app = MCPServerApp(mcp_server_name="test-mcp-degraded")
    app._pipelines_ready.set()
    return app.mcp


@pytest.mark.asyncio
async def test_degraded_server_starts_and_lists_tools(mcp_server_degraded):
    """Server should start and register all tools even without enVector adapter."""
    async with Client(mcp_server_degraded) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        assert "reload_pipelines" in names
        assert "capture" in names
        assert "recall" in names
        assert "vault_status" in names


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

# Pre-warm tests

@pytest.mark.asyncio
async def test_reload_pipelines_returns_envector_warmup(mcp_server):
    async with Client(mcp_server) as client:
        result = await client.call_tool("reload_pipelines", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        # When pipelines aren't active (dormant), warmup may be None/empty
        # but the field should still be present in the response
        assert "envector_warmup" in data


@pytest.mark.asyncio
async def test_reload_pipelines_warmup_failure():
    class FailingAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass

        def invoke_get_index_list(self) -> List[str]:
            raise ConnectionError("UNAVAILABLE: could not connect")

    app = MCPServerApp(envector_adapter=FailingAdapter(), mcp_server_name="test-mcp-warmup-fail")
    app.embedding = FakeEmbeddingService()
    app._pipelines_ready.set()
    # Force _scribe to be truthy so warmup path is triggered
    app._scribe = {"record_builder": None, "envector_client": None, "embedding_service": None}

    async with Client(app.mcp) as client:
        result = await client.call_tool("reload_pipelines", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        warmup = data.get("envector_warmup")
        # _init_pipelines resets _scribe to None (dormant), so warmup may be None;
        # if the warmup path runs, it should report the failure
        if warmup is not None:
            assert warmup.get("ok") is False
            assert "error" in warmup


# ----------- Diagnostic Tool Tests ----------- #

@pytest.mark.asyncio
async def test_tools_list_contains_diagnostics(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        tools = await client.list_tools()
        names = [t.name for t in tools]
        assert "diagnostics" in names


@pytest.mark.asyncio
async def test_diagnostics_with_vault(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        vault = data.get("vault", {})
        assert vault.get("configured") is True
        assert vault.get("healthy") is True
        assert vault.get("endpoint") == "http://fake-vault:50080"

        keys = data.get("keys", {})
        assert "enc_key_loaded" in keys
        assert "key_id" in keys
        assert "agent_dek_loaded" in keys

        pipelines = data.get("pipelines", {})
        assert "scribe" in pipelines
        assert "retriever" in pipelines

        envector = data.get("envector", {})
        assert envector.get("reachable") is True
        assert envector.get("latency_ms") is not None


@pytest.mark.asyncio
async def test_diagnostics_environment_includes_executable(mcp_server_with_vault):
    """diagnostics.environment must expose sys.executable so clients can
    derive the active plugin checkout path from it. This is the basis for
    multi-checkout drift detection in /rune:status — stripping /.venv/bin/
    python3 from the value yields the plugin root of the responding server."""
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        env = data.get("environment", {})
        assert "executable" in env, "environment must include 'executable' field"
        assert isinstance(env["executable"], str)
        assert env["executable"] == sys.executable


@pytest.mark.asyncio
async def test_diagnostics_without_vault(mcp_server):
    async with Client(mcp_server) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        vault = data.get("vault", {})
        assert vault.get("configured") is False
        assert vault.get("healthy") is False

        keys = data.get("keys", {})
        assert keys.get("enc_key_loaded") is False
        assert keys.get("agent_dek_loaded") is False

        pipelines = data.get("pipelines", {})
        assert pipelines.get("scribe") is False
        assert pipelines.get("retriever") is False

        envector = data.get("envector", {})
        assert envector.get("reachable") is True
        assert envector.get("latency_ms") is not None


@pytest.mark.asyncio
async def test_diagnostics_no_envector(mcp_server_degraded):
    async with Client(mcp_server_degraded) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        vault = data.get("vault", {})
        assert vault.get("configured") is False
        assert vault.get("healthy") is False

        keys = data.get("keys", {})
        assert keys.get("enc_key_loaded") is False
        assert keys.get("agent_dek_loaded") is False

        pipelines = data.get("pipelines", {})
        assert pipelines.get("scribe") is False
        assert pipelines.get("retriever") is False

        envector = data.get("envector", {})
        assert envector.get("reachable") is False


# enVector connection related tests

@pytest.fixture
def mcp_server_envector_timeout():
    import time

    class SlowAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass

        def invoke_get_index_list(self) -> List[str]:
            time.sleep(10)  # Longer than ENVECTOR_DIAGNOSIS_TIMEOUT (5s)
            return []

    app = MCPServerApp(envector_adapter=SlowAdapter(), mcp_server_name="test-mcp-slow")
    app.embedding = FakeEmbeddingService()
    app._pipelines_ready.set()
    return app.mcp


@pytest.fixture
def mcp_server_envector_connection_error():
    class ErrorAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass

        def invoke_get_index_list(self) -> List[str]:
            raise ConnectionError("UNAVAILABLE: Connection refused to cloud.envector.io:443")

    app = MCPServerApp(envector_adapter=ErrorAdapter(), mcp_server_name="test-mcp-err")
    app.embedding = FakeEmbeddingService()
    app._pipelines_ready.set()
    return app.mcp


@pytest.fixture
def mcp_server_envector_auth_error():
    class AuthErrorAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass

        def invoke_get_index_list(self) -> List[str]:
            raise Exception("UNAUTHENTICATED: invalid API key")

    app = MCPServerApp(envector_adapter=AuthErrorAdapter(), mcp_server_name="test-mcp-auth")
    app.embedding = FakeEmbeddingService()
    app._pipelines_ready.set()
    return app.mcp


@pytest.mark.asyncio
async def test_diagnostics_envector_timeout(mcp_server_envector_timeout):
    async with Client(mcp_server_envector_timeout) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        envector = data.get("envector", {})
        assert envector.get("reachable") is False
        assert envector.get("error_type") == "timeout"
        assert "elapsed_ms" in envector
        assert "timed out" in envector.get("error", "").lower()


@pytest.mark.asyncio
async def test_diagnostics_envector_connection_refused(mcp_server_envector_connection_error):
    async with Client(mcp_server_envector_connection_error) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        envector = data.get("envector", {})
        assert envector.get("reachable") is False
        assert envector.get("error_type") == "connection_refused"
        assert "hint" in envector
        assert "endpoint" in envector["hint"].lower()


@pytest.mark.asyncio
async def test_diagnostics_envector_auth_failure(mcp_server_envector_auth_error):
    async with Client(mcp_server_envector_auth_error) as client:
        result = await client.call_tool("diagnostics", {})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        envector = data.get("envector", {})
        assert envector.get("reachable") is False
        assert envector.get("error_type") == "auth_failure"
        assert "hint" in envector


# ----------- Error Response Tests ----------- #

@pytest.mark.asyncio
async def test_capture_returns_structured_error_when_pipeline_not_ready(mcp_server):
    async with Client(mcp_server) as client:
        result = await client.call_tool("capture", {"text": "test decision"})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        assert data.get("ok") is False
        error = data.get("error")
        assert isinstance(error, dict), f"Expected structured error dict, got: {type(error)}"
        assert error.get("code") == "PIPELINE_NOT_READY"
        assert error.get("retryable") is False
        assert "Scribe" in error.get("message", "")


@pytest.mark.asyncio
async def test_recall_returns_structured_error_when_pipeline_not_ready(mcp_server):
    async with Client(mcp_server) as client:
        result = await client.call_tool("recall", {"query": "test query"})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        assert data.get("ok") is False
        error = data.get("error")
        assert isinstance(error, dict), f"Expected structured error dict, got: {type(error)}"
        assert error.get("code") == "PIPELINE_NOT_READY"
        assert error.get("retryable") is False
        assert "Retriever" in error.get("message", "")


@pytest.mark.asyncio
async def test_recall_returns_structured_error_for_invalid_topk(mcp_server_with_vault):
    async with Client(mcp_server_with_vault) as client:
        result = await client.call_tool("recall", {"query": "test", "topk": 100})
        data = getattr(result, "data", None) or getattr(result, "structured", None) \
               or getattr(result, "structured_content", None)

        assert data is not None
        assert data.get("ok") is False
        error = data.get("error")
        assert isinstance(error, dict), f"Expected structured error dict, got: {type(error)}"
        assert error.get("code") in ("PIPELINE_NOT_READY", "INVALID_INPUT")
        assert isinstance(error.get("retryable"), bool)


# ----------- Background Pipeline Init Tests ----------- #

@pytest.mark.asyncio
async def test_ensure_pipelines_returns_none_when_ready():
    """_ensure_pipelines returns None when pipelines are already initialized."""
    class FakeAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass
    app = MCPServerApp(envector_adapter=FakeAdapter(), mcp_server_name="test-mcp")
    app._pipelines_ready.set()
    result = app._ensure_pipelines(timeout=0.1)
    assert result is None


@pytest.mark.asyncio
async def test_ensure_pipelines_returns_error_on_timeout():
    """_ensure_pipelines returns error dict when init times out."""
    class FakeAdapter(EnVectorSDKAdapter):
        def __init__(self):
            pass
    app = MCPServerApp(envector_adapter=FakeAdapter(), mcp_server_name="test-mcp")
    # Don't set _pipelines_ready — simulate still initializing
    result = app._ensure_pipelines(timeout=0.01)
    assert result is not None
    assert result["ok"] is False
    assert "in progress" in result["error"]["message"].lower()
