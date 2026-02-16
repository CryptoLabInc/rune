"""
Vault Client for Rune-Vault gRPC Integration

This client handles communication between envector-mcp-server and Rune-Vault.
All decryption operations are delegated to Vault, which holds the secret key.

Uses grpc.aio for async communication with Vault's gRPC server.
Maintains a persistent channel (connection pooled internally by grpcio).

Security Model:
- MCP server NEVER has access to secret key
- All decryption requests go through Vault
- Audit trail maintained by Vault
"""

import os
import json
import logging
from typing import Dict, Any, Optional, List
from dataclasses import dataclass
from urllib.parse import urlparse

import grpc
import grpc.aio
import httpx

logger = logging.getLogger(__name__)

# Import generated stubs
from .vault_proto import vault_service_pb2 as pb2
from .vault_proto import vault_service_pb2_grpc as pb2_grpc

MAX_MESSAGE_LENGTH = 256 * 1024 * 1024  # 256 MB (EvalKey can be tens of MB)


@dataclass
class DecryptResult:
    """Result from Vault decryption of the result ciphertext."""
    ok: bool
    results: List[Dict[str, Any]]  # [{shard_idx: int, row_idx: int, score: float}, ...]
    error: Optional[str] = None

    @classmethod
    def from_vault_response(cls, raw: Any) -> "DecryptResult":
        """
        Parse response from non-gRPC paths (kept for backward compatibility).
        The gRPC path constructs DecryptResult directly from typed ScoreEntry messages.
        """
        if isinstance(raw, list):
            return cls(ok=True, results=raw)
        if isinstance(raw, dict) and "error" in raw:
            return cls(ok=False, results=[], error=raw["error"])
        raise VaultError(f"Unexpected vault response format: {type(raw).__name__}: {raw}")


class VaultError(Exception):
    """Error communicating with Vault."""
    pass


class VaultClient:
    """
    Async gRPC client for Rune-Vault decryption service.

    Maintains a persistent gRPC channel (connection pooled internally by grpcio).
    The channel is created lazily on first use.

    Usage:
        client = VaultClient(
            vault_endpoint="http://vault:50080/mcp",
            vault_token="your-token"
        )
        result = await client.decrypt_search_results(
            encrypted_blob_b64="base64...",
            top_k=5
        )
        await client.close()
    """

    def __init__(
        self,
        vault_endpoint: str,
        vault_token: str,
        timeout: float = 30.0,
    ):
        """
        Initialize Vault client.

        Args:
            vault_endpoint: Vault gRPC target. Accepts multiple formats:
                "host:port" (direct), "tcp://host:port", or legacy
                "http://host:50080/mcp". RUNEVAULT_GRPC_TARGET overrides.
            vault_token: Authentication token for Vault
            timeout: Request timeout in seconds
        """
        self.vault_endpoint = vault_endpoint.rstrip("/")
        self.vault_token = vault_token
        self.timeout = timeout

        # Derive gRPC target from endpoint URL or use explicit override
        self._grpc_target = os.getenv("RUNEVAULT_GRPC_TARGET")
        if not self._grpc_target:
            self._grpc_target = self._derive_grpc_target(self.vault_endpoint)

        # Lazy channel creation (created on first use in async context)
        self._channel: Optional[grpc.aio.Channel] = None
        self._stub: Optional[pb2_grpc.VaultServiceStub] = None

    @staticmethod
    def _derive_grpc_target(endpoint: str) -> str:
        """
        Derive gRPC host:port from endpoint string.

        Accepts multiple formats:
            "host:port"                    -> "host:port"       (direct gRPC target)
            "tcp://host:port"              -> "host:port"       (tcp scheme stripped)
            "http://vault:50080/mcp"       -> "vault:50051"     (legacy HTTP, port replaced)
            "https://vault.example.com"    -> "vault.example.com:50051"
        """
        parsed = urlparse(endpoint)

        # Direct gRPC target: no scheme or tcp:// scheme
        if not parsed.scheme or parsed.scheme == "tcp":
            host = parsed.hostname or endpoint.split(":")[0].split("/")[0]
            port = parsed.port
            if port:
                return f"{host}:{port}"
            # bare hostname without port — assume default gRPC port
            return f"{host}:50051"

        # Legacy HTTP/HTTPS endpoint — extract host, use gRPC default port
        host = parsed.hostname or endpoint.split(":")[0].split("/")[0]
        return f"{host}:50051"

    def _ensure_channel(self):
        """Create the async gRPC channel if not yet created."""
        if self._channel is None:
            self._channel = grpc.aio.insecure_channel(
                self._grpc_target,
                options=[
                    ("grpc.max_send_message_length", MAX_MESSAGE_LENGTH),
                    ("grpc.max_receive_message_length", MAX_MESSAGE_LENGTH),
                ],
            )
            self._stub = pb2_grpc.VaultServiceStub(self._channel)

    async def close(self):
        """Close the gRPC channel."""
        if self._channel is not None:
            await self._channel.close()
            self._channel = None
            self._stub = None

    async def get_public_key(self) -> dict:
        """
        Fetch the public key bundle via gRPC.

        Returns:
            Parsed dict: {"EncKey.json": "...", "EvalKey.json": "...", "index_name": "..."}

        Raises:
            VaultError: If the call fails
        """
        self._ensure_channel()
        try:
            request = pb2.GetPublicKeyRequest(token=self.vault_token)
            response = await self._stub.GetPublicKey(
                request, timeout=self.timeout
            )
            if response.error:
                raise VaultError(f"GetPublicKey failed: {response.error}")
            return json.loads(response.key_bundle_json)
        except grpc.aio.AioRpcError as e:
            raise VaultError(f"gRPC GetPublicKey failed: {e.code()} {e.details()}")

    async def decrypt_search_results(
        self,
        encrypted_blob_b64: str,
        top_k: int = 5,
    ) -> DecryptResult:
        """
        Decrypt result ciphertext from encrypted similarity search.

        Args:
            encrypted_blob_b64: Base64-encoded result ciphertext from
                encrypted similarity search on enVector Cloud
            top_k: Number of top results to return (max 10)

        Returns:
            DecryptResult with top-k indices and similarity values

        Raises:
            VaultError: If the call fails
        """
        self._ensure_channel()
        try:
            request = pb2.DecryptScoresRequest(
                token=self.vault_token,
                encrypted_blob_b64=encrypted_blob_b64,
                top_k=top_k,
            )
            response = await self._stub.DecryptScores(
                request, timeout=self.timeout
            )
            if response.error:
                return DecryptResult(ok=False, results=[], error=response.error)

            results = [
                {
                    "shard_idx": entry.shard_idx,
                    "row_idx": entry.row_idx,
                    "score": entry.score,
                }
                for entry in response.results
            ]
            return DecryptResult(ok=True, results=results)
        except grpc.aio.AioRpcError as e:
            raise VaultError(
                f"gRPC DecryptScores failed: {e.code()} {e.details()}"
            )

    async def decrypt_metadata(
        self,
        encrypted_metadata_list: List[str],
    ) -> List:
        """
        Decrypt AES-encrypted metadata via Vault.

        Args:
            encrypted_metadata_list: List of Base64-encoded encrypted metadata strings.

        Returns:
            List of decrypted metadata objects (dicts, strings, etc.)

        Raises:
            VaultError: If the call fails
        """
        self._ensure_channel()
        try:
            request = pb2.DecryptMetadataRequest(
                token=self.vault_token,
                encrypted_metadata_list=encrypted_metadata_list,
            )
            response = await self._stub.DecryptMetadata(
                request, timeout=self.timeout
            )
            if response.error:
                raise VaultError(f"DecryptMetadata failed: {response.error}")

            # Parse each JSON string back to Python object
            return [json.loads(s) for s in response.decrypted_metadata]
        except grpc.aio.AioRpcError as e:
            raise VaultError(
                f"gRPC DecryptMetadata failed: {e.code()} {e.details()}"
            )

    async def health_check(self) -> bool:
        """
        Check if Vault is reachable.
        Tries gRPC health check first, falls back to HTTP /health.
        """
        # Try gRPC health check
        try:
            self._ensure_channel()
            from grpc_health.v1 import health_pb2 as health_proto
            from grpc_health.v1 import health_pb2_grpc as health_grpc
            health_stub = health_grpc.HealthStub(self._channel)
            resp = await health_stub.Check(
                health_proto.HealthCheckRequest(service=""),
                timeout=5.0,
            )
            return resp.status == health_proto.HealthCheckResponse.SERVING
        except Exception:
            pass

        # Fallback to HTTP /health (only if endpoint looks like an HTTP URL)
        parsed = urlparse(self.vault_endpoint)
        if parsed.scheme in ("http", "https"):
            try:
                async with httpx.AsyncClient(timeout=httpx.Timeout(5.0)) as client:
                    base_url = self.vault_endpoint
                    for suffix in ("/mcp", "/sse"):
                        if base_url.endswith(suffix):
                            base_url = base_url[:-len(suffix)]
                            break
                    response = await client.get(f"{base_url}/health")
                    return response.status_code == 200
            except Exception as e:
                logger.warning(f"Vault health check failed: {e}")

        logger.warning("Vault health check failed: gRPC unreachable")
        return False


def create_vault_client(
    vault_endpoint: Optional[str] = None,
    vault_token: Optional[str] = None,
) -> Optional[VaultClient]:
    """
    Factory function to create Vault client from environment variables.

    Environment variables:
    - RUNEVAULT_ENDPOINT: Vault gRPC target (e.g., "vault:50051" or "tcp://host:port")
    - RUNEVAULT_TOKEN: Authentication token for Vault
    - RUNEVAULT_GRPC_TARGET: Optional explicit gRPC target override

    Args:
        vault_endpoint: Override for RUNEVAULT_ENDPOINT
        vault_token: Override for RUNEVAULT_TOKEN

    Returns:
        VaultClient if configured, None otherwise
    """
    endpoint = vault_endpoint or os.getenv("RUNEVAULT_ENDPOINT")
    token = vault_token or os.getenv("RUNEVAULT_TOKEN")

    if not endpoint or not token:
        logger.info("Rune-Vault not configured (RUNEVAULT_ENDPOINT or RUNEVAULT_TOKEN missing)")
        return None

    return VaultClient(vault_endpoint=endpoint, vault_token=token)
