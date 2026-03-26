# tests/test_vault_client.py
import json
import pytest
from dataclasses import dataclass, field
from typing import List, Optional
from unittest.mock import AsyncMock, MagicMock

import os
import sys

MCP_ROOT = os.path.dirname(os.path.dirname(__file__))
if MCP_ROOT not in sys.path:
    sys.path.insert(0, MCP_ROOT)

from adapter.vault_client import VaultClient, VaultError, DecryptResult


# Fake protobuf response

@dataclass
class FakeGetPublicKeyResponse:
    key_bundle_json: str = ""
    error: str = ""


@dataclass
class FakeScoreEntry:
    shard_idx: int = 0
    row_idx: int = 0
    score: float = 0.0


@dataclass
class FakeDecryptScoresResponse:
    results: List[FakeScoreEntry] = field(default_factory=list)
    error: str = ""


@dataclass
class FakeDecryptMetadataResponse:
    decrypted_metadata: List[str] = field(default_factory=list)
    error: str = ""


# Helpers

def _make_client_with_mock_stub() -> tuple[VaultClient, MagicMock]:
    client = VaultClient(
        vault_endpoint="tcp://fake-vault:50051",
        vault_token="test-token",
        tls_disable=True,
    )
    mock_stub = MagicMock()
    client._channel = MagicMock()
    client._stub = mock_stub
    return client, mock_stub


# Tests

class TestGetPublicKey:

    @pytest.mark.asyncio
    async def test_valid_json(self):
        client, stub = _make_client_with_mock_stub()
        bundle = {"EncKey.json": "enc...", "EvalKey.json": "eval...", "index_name": "team-idx"}
        stub.GetPublicKey = AsyncMock(return_value=FakeGetPublicKeyResponse(
            key_bundle_json=json.dumps(bundle),
            error="",
        ))

        result = await client.get_public_key()
        assert result == bundle

    @pytest.mark.asyncio
    async def test_invalid_json_raises_vault_error(self):
        client, stub = _make_client_with_mock_stub()
        stub.GetPublicKey = AsyncMock(return_value=FakeGetPublicKeyResponse(
            key_bundle_json="NOT VALID JSON {{{",
            error="",
        ))

        with pytest.raises(VaultError, match="GetPublicKey returned invalid JSON"):
            await client.get_public_key()

    @pytest.mark.asyncio
    async def test_invalid_json_preserves_cause(self):
        client, stub = _make_client_with_mock_stub()
        stub.GetPublicKey = AsyncMock(return_value=FakeGetPublicKeyResponse(
            key_bundle_json="<<<broken>>>",
            error="",
        ))

        with pytest.raises(VaultError) as exc_info:
            await client.get_public_key()
        assert isinstance(exc_info.value.__cause__, (json.JSONDecodeError, ValueError))

    @pytest.mark.asyncio
    async def test_empty_json_string_raises_vault_error(self):
        client, stub = _make_client_with_mock_stub()
        stub.GetPublicKey = AsyncMock(return_value=FakeGetPublicKeyResponse(
            key_bundle_json="",
            error="",
        ))

        with pytest.raises(VaultError, match="GetPublicKey returned invalid JSON"):
            await client.get_public_key()

    @pytest.mark.asyncio
    async def test_server_error_field(self):
        client, stub = _make_client_with_mock_stub()
        stub.GetPublicKey = AsyncMock(return_value=FakeGetPublicKeyResponse(
            key_bundle_json="",
            error="token expired",
        ))

        with pytest.raises(VaultError, match="GetPublicKey failed: token expired"):
            await client.get_public_key()

    @pytest.mark.asyncio
    async def test_grpc_error(self):
        import grpc.aio
        client, stub = _make_client_with_mock_stub()

        rpc_error = grpc.aio.AioRpcError(
            code=grpc.StatusCode.UNAVAILABLE,
            initial_metadata=grpc.aio.Metadata(),
            trailing_metadata=grpc.aio.Metadata(),
            details="Connection refused",
        )
        stub.GetPublicKey = AsyncMock(side_effect=rpc_error)

        with pytest.raises(VaultError, match="gRPC GetPublicKey failed"):
            await client.get_public_key()


class TestDecryptMetadata:

    @pytest.mark.asyncio
    async def test_valid_json_entries(self):
        client, stub = _make_client_with_mock_stub()
        entries = [
            json.dumps({"decision": "use Postgres", "confidence": 0.9}),
            json.dumps({"decision": "adopt gRPC", "confidence": 0.85}),
        ]
        stub.DecryptMetadata = AsyncMock(return_value=FakeDecryptMetadataResponse(
            decrypted_metadata=entries,
            error="",
        ))

        result = await client.decrypt_metadata(["enc1", "enc2"])
        assert len(result) == 2
        assert result[0]["decision"] == "use Postgres"
        assert result[1]["decision"] == "adopt gRPC"

    @pytest.mark.asyncio
    async def test_one_bad_entry_raises_vault_error(self):
        client, stub = _make_client_with_mock_stub()
        entries = [
            json.dumps({"decision": "valid"}),
            "NOT JSON AT ALL",  # bad entry
        ]
        stub.DecryptMetadata = AsyncMock(return_value=FakeDecryptMetadataResponse(
            decrypted_metadata=entries,
            error="",
        ))

        with pytest.raises(VaultError, match="DecryptMetadata returned invalid JSON in metadata entry"):
            await client.decrypt_metadata(["enc1", "enc2"])

    @pytest.mark.asyncio
    async def test_bad_entry_preserves_cause(self):
        client, stub = _make_client_with_mock_stub()
        stub.DecryptMetadata = AsyncMock(return_value=FakeDecryptMetadataResponse(
            decrypted_metadata=["{{broken}}"],
            error="",
        ))

        with pytest.raises(VaultError) as exc_info:
            await client.decrypt_metadata(["enc1"])
        assert isinstance(exc_info.value.__cause__, (json.JSONDecodeError, ValueError))

    @pytest.mark.asyncio
    async def test_empty_list(self):
        client, stub = _make_client_with_mock_stub()
        stub.DecryptMetadata = AsyncMock(return_value=FakeDecryptMetadataResponse(
            decrypted_metadata=[],
            error="",
        ))

        result = await client.decrypt_metadata([])
        assert result == []

    @pytest.mark.asyncio
    async def test_server_error_field(self):
        client, stub = _make_client_with_mock_stub()
        stub.DecryptMetadata = AsyncMock(return_value=FakeDecryptMetadataResponse(
            decrypted_metadata=[],
            error="decryption key not found",
        ))

        with pytest.raises(VaultError, match="DecryptMetadata failed: decryption key not found"):
            await client.decrypt_metadata(["enc1"])

    @pytest.mark.asyncio
    async def test_grpc_error(self):
        import grpc.aio
        client, stub = _make_client_with_mock_stub()

        rpc_error = grpc.aio.AioRpcError(
            code=grpc.StatusCode.DEADLINE_EXCEEDED,
            initial_metadata=grpc.aio.Metadata(),
            trailing_metadata=grpc.aio.Metadata(),
            details="Deadline exceeded",
        )
        stub.DecryptMetadata = AsyncMock(side_effect=rpc_error)

        with pytest.raises(VaultError, match="gRPC DecryptMetadata failed"):
            await client.decrypt_metadata(["enc1"])


class TestDecryptSearchResults:

    @pytest.mark.asyncio
    async def test_valid_response(self):
        client, stub = _make_client_with_mock_stub()
        stub.DecryptScores = AsyncMock(return_value=FakeDecryptScoresResponse(
            results=[
                FakeScoreEntry(shard_idx=0, row_idx=3, score=0.95),
                FakeScoreEntry(shard_idx=0, row_idx=7, score=0.82),
            ],
            error="",
        ))

        result = await client.decrypt_search_results("base64blob", top_k=2)
        assert result.ok is True
        assert len(result.results) == 2
        assert result.results[0]["score"] == 0.95

    @pytest.mark.asyncio
    async def test_server_error(self):
        client, stub = _make_client_with_mock_stub()
        stub.DecryptScores = AsyncMock(return_value=FakeDecryptScoresResponse(
            results=[],
            error="invalid ciphertext",
        ))

        result = await client.decrypt_search_results("bad-blob")
        assert result.ok is False
        assert result.error == "invalid ciphertext"
