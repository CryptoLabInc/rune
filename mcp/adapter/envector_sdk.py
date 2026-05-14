# Summary of file: enVector SDK Adapter(enVector APIs Caller)

from typing import Union, List, Dict, Any, Optional
import base64
import json
import logging
import os
import numpy as np
import pyenvector as ev  # pip install pyenvector
from pyenvector.crypto.block import CipherBlock
from pyenvector.crypto.parameter import KeyParameter
from google.protobuf.json_format import MessageToDict

from pathlib import Path

logger = logging.getLogger("rune.adapter")

SCRIPT_DIR = Path(__file__).parent.resolve()
KEY_PATH = SCRIPT_DIR.parent.parent / "keys" # Manage keys directory at project root

# ---------------------------------------------------------------------------
# Vault-model safety patches for pyenvector KeyParameter
#
# In the Vault security model SecKey and MetadataKey never leave Vault,
# so the local .json files do not exist.  pyenvector's KeyParameter
# properties call utils.get_key_stream(path) which falls through to
# ast.literal_eval(path_string) and raises SyntaxError when the file
# is missing.
#
# The patches return None for missing key files, allowing Cipher to
# initialise in encrypt-only mode — exactly what insert operations need.
# ---------------------------------------------------------------------------
_original_sec_key_fget = KeyParameter.sec_key.fget
_original_sec_key_path_fget = KeyParameter.sec_key_path.fget
_original_metadata_key_fget = KeyParameter.metadata_key.fget
_original_metadata_key_path_fget = KeyParameter.metadata_key_path.fget

def _safe_sec_key_getter(self):
    """Return None when SecKey.json is absent instead of crashing."""
    if getattr(self, 'sec_key_stream', None):
        return _original_sec_key_fget(self)
    path = _original_sec_key_path_fget(self)
    if path and not os.path.exists(path):
        return None
    return _original_sec_key_fget(self)

def _safe_sec_key_path_getter(self):
    """Return None when SecKey.json is absent so Cipher skips decryptor init."""
    path = _original_sec_key_path_fget(self)
    if path and not os.path.exists(path):
        return None
    return path

def _safe_metadata_key_getter(self):
    """Return None when MetadataKey.json is absent instead of crashing."""
    if getattr(self, 'metadata_key_stream', None):
        return _original_metadata_key_fget(self)
    path = _original_metadata_key_path_fget(self)
    if path and not os.path.exists(path):
        return None
    return _original_metadata_key_fget(self)

def _safe_metadata_key_path_getter(self):
    """Return None when MetadataKey.json is absent so Cipher skips metadata encryption."""
    path = _original_metadata_key_path_fget(self)
    if path and not os.path.exists(path):
        return None
    return path

_original_metadata_encryption_fget = KeyParameter.metadata_encryption.fget

def _safe_metadata_encryption_getter(self):
    """Return False when MetadataKey.json is absent (app-layer handles encryption)."""
    if not _original_metadata_encryption_fget(self):
        return False
    # If metadata_encryption is True but key file is missing, override to False
    path = _original_metadata_key_path_fget(self)
    if path and not os.path.exists(path):
        return False
    return True

KeyParameter.sec_key = property(_safe_sec_key_getter, KeyParameter.sec_key.fset)
KeyParameter.sec_key_path = property(_safe_sec_key_path_getter)
KeyParameter.metadata_key = property(_safe_metadata_key_getter, KeyParameter.metadata_key.fset)
KeyParameter.metadata_key_path = property(_safe_metadata_key_path_getter)
KeyParameter.metadata_encryption = property(_safe_metadata_encryption_getter, KeyParameter.metadata_encryption.fset)

# gRPC error messages related to dead/stale connection
CONNECTION_ERROR_PATTERNS = (
    "UNAVAILABLE",
    "DEADLINE_EXCEEDED",
    "Connection refused",
    "Connection reset",
    "Stream removed",
    "RST_STREAM",
    "Broken pipe",
    "Transport closed",
    "Socket closed",
    "EOF",
    "failed to connect",
)


class EnVectorSDKAdapter:
    """
    Adapter class to interact with the enVector SDK.
    """
    def __init__(
            self,
            address: str,
            key_id: str,
            key_path: str,
            eval_mode: str,
            query_encryption: Union[str, bool, None],
            access_token: str = None,
            secure: Optional[bool] = None,
            auto_key_setup: bool = True,
            agent_id: str = None,
            agent_dek: bytes = None,
            index_type: str = None,
        ):
        """
        Initializes the EnVectorSDKAdapter with an optional endpoint.

        Args:
            address (str): The endpoint URL for the enVector SDK.
            key_id (str): The key identifier for the enVector SDK.
            key_path (str): The path to the key files.
            eval_mode (str): The evaluation mode for the enVector SDK.
            query_encryption (Union[str, bool, None]): pyenvector 1.4 accepts "plain"/"cipher"; legacy bool is normalized.
            access_token (str, optional): The access token for the enVector SDK.
            secure (bool, optional): TLS toggle for pyenvector 1.4 (None = SDK default).
            auto_key_setup (bool): If True, generates keys automatically when not found.
                                   Set to False when keys are provided externally (e.g., from Vault).
            agent_id (str): Per-agent identifier for app-layer metadata encryption.
            agent_dek (bytes): Per-agent AES-256 DEK (32 bytes) for metadata encryption.
            index_type (str, optional): Index structure type passed to ev.init() ("ivf_vct", "flat", etc.).
        """
        if not key_path:
            key_path = str(KEY_PATH)
        self.query_encryption = query_encryption
        self._agent_id = agent_id
        self._agent_dek = agent_dek

        # Store init params for reinitialization on connection loss
        self._init_params = {
            "address": address,
            "key_path": key_path,
            "key_id": key_id,
            "eval_mode": eval_mode,
            "auto_key_setup": auto_key_setup,
            "access_token": access_token,
            "secure": secure,
            "query_encryption": self._normalize_query_encryption(query_encryption),
        }
        if index_type is not None:
            self._init_params["index_type"] = index_type

        ev.init(**self._init_params)

    @staticmethod
    def _normalize_query_encryption(value: Union[str, bool, None]) -> str:
        """Normalize Rune's legacy bool flag to pyenvector 1.4 string values."""
        if value is None:
            return "plain"
        if isinstance(value, bool):
            return "cipher" if value else "plain"
        return value

    #--------------- Get Index List --------------#
    def call_get_index_list(self) -> Dict[str, Any]:
        """
        Calls the enVector SDK to get the list of indexes.

        Returns:
            Dict[str, Any]: If succeed, converted format of the index list. Otherwise, error message.
        """
        try:
            results = self.invoke_get_index_list()
            return self._to_json_available({"ok": True, "results": results})
        except Exception as e:
            # Handle exceptions and return an appropriate error message
            return {"ok": False, "error": repr(e)}

    #--------------- Connection resilience helpers --------------#

    @staticmethod
    def _is_connection_error(exc: Exception) -> bool:
        if isinstance(exc, (ConnectionError, OSError)):
            return True
        msg = str(exc)
        return any(pattern in msg for pattern in CONNECTION_ERROR_PATTERNS)

    def _reinit(self) -> None:
        logger.warning(
            "enVector connection lost - reconnecting to %s ...",
            self._init_params["address"],
        )
        ev.init(**self._init_params)
        logger.info("enVector reconnection complete.")

    def _with_reconnect(self, fn):
        try:
            return fn()
        except Exception as exc:
            if not self._is_connection_error(exc):
                raise
            logger.warning(
                "enVector operation failed (%s: %s). Attempting reconnect...",
                type(exc).__name__, exc,
            )
            self._reinit()
            return fn()

    def invoke_get_index_list(self) -> List[str]:
        """
        Invokes the enVector SDK's get_index_list functionality.

        Returns:
            List[str]: List of index names from the enVector SDK.
        """
        return self._with_reconnect(ev.get_index_list)

    #------------------- Insert ------------------#

    def call_insert(
        self,
        index_name: str,
        vectors: List[List[float]],
        metadata: List[Any] = None,
        await_completion: bool = False,
        use_row_insert: bool = False,
        load: bool = True,
        request_ids: Optional[List[str]] = None,
    ):
        """
        Calls the enVector SDK to perform an insert operation.

        Args:
            vectors (List[List[float]]): The list of vectors to insert.
            metadata (List[Any], optional): The list of metadata associated with the vectors. Defaults to None.
            await_completion (bool): Forwarded to pyenvector 1.4.3 Index.insert(await_completion=...).
                If True, block until the server-side stage (MERGED_SAVED for the default
                execute_until="segmentation") is reached.
            use_row_insert (bool): If True, use single-row insert API path instead of batch path.
            load (bool): If True, the SDK triggers Index.load() after submission (or after
                stage-wait when await_completion=True). Set False for fully async insert paths
                that handle load() out-of-band (e.g., pre-loaded index in logic-1 benchmarks).
            request_ids (Optional[List[str]]): Out parameter. If provided, the SDK clears
                this list and appends one server-generated split request_id per underlying
                async split RPC. Required to later call wait_for_insert_stage.

        Returns:
            Dict[str, Any]: If succeed, converted format of the insert results. Otherwise, error message.
        """
        try:
            results = self.invoke_insert(
                index_name=index_name,
                vectors=vectors,
                metadata=metadata,
                await_completion=await_completion,
                use_row_insert=use_row_insert,
                load=load,
                request_ids=request_ids,
            )
            return self._to_json_available({"ok": True, "results": results})
        except Exception as e:
            # Handle exceptions and return an appropriate error message
            return {"ok": False, "error": repr(e)}

    def _app_encrypt_metadata(self, metadata_str: str) -> str:
        """
        App-layer metadata encryption using per-agent DEK.
        Returns JSON: {"a": "<agent_id>", "c": "<base64_ciphertext>"}
        """
        from pyenvector.utils.aes import encrypt_metadata as aes_encrypt
        ct = aes_encrypt(metadata_str, self._agent_dek)
        return json.dumps({"a": self._agent_id, "c": ct})

    def invoke_insert(
        self,
        index_name: str,
        vectors: List[List[float]],
        metadata: List[Any] = None,
        await_completion: bool = False,
        use_row_insert: bool = False,
        load: bool = True,
        request_ids: Optional[List[str]] = None,
    ):
        """
        Invokes the enVector SDK's insert functionality.

        Args:
            index_name (str): The name of the index to insert into.
            vectors (Union[List[List[float]], List[CipherBlock]]): The list of vectors to insert.
            metadata (List[Any], optional): The list of metadata associated with the vectors. Defaults to None.
            await_completion (bool): Forwarded to pyenvector 1.4.3 Index.insert(await_completion=...).
                If True, block until the server-side stage (MERGED_SAVED for the default
                execute_until="segmentation") is reached.
            use_row_insert (bool): If True, use single-row insert API path instead of batch path.
            load (bool): Forwarded to SDK Index.insert(load=...). Defaults to True (SDK default).
            request_ids (Optional[List[str]]): Forwarded to SDK Index.insert(request_ids=...) as
                an out parameter; the SDK fills it with server-generated split request_ids.

        Returns:
            Any: Raw insert results from the enVector SDK.
        """
        # App-layer metadata encryption with per-agent DEK
        if self._agent_dek and metadata:
            if not self._agent_id:
                logger.warning("agent_dek is set but agent_id is missing — skipping metadata encryption")
            else:
                metadata = [self._app_encrypt_metadata(m) for m in metadata]

        def _do_insert():
            index = ev.Index(index_name)
            return index.insert(
                data=vectors,
                metadata=metadata,
                request_ids=request_ids,
                await_completion=await_completion,
                load=load,
                use_row_insert=use_row_insert,
            )

        return self._with_reconnect(_do_insert)

    #------------------- Wait for insert stage ------------------#

    def call_wait_for_insert_stage(
        self,
        index_name: str,
        request_ids: List[str],
        target_stage: str = "segmentation",
        timeout_s: float = 60.0,
        poll_interval_s: float = 0.5,
    ) -> Dict[str, Any]:
        """
        Block until all given async-insert request_ids reach target_stage on the server.

        target_stage="segmentation" maps to MERGED_SAVED — the condition for searchability
        when the index is already loaded.

        Returns:
            Dict[str, Any]: {"ok": True} on success, else {"ok": False, "error": "..."}.
        """
        try:
            self.invoke_wait_for_insert_stage(
                index_name=index_name,
                request_ids=request_ids,
                target_stage=target_stage,
                timeout_s=timeout_s,
                poll_interval_s=poll_interval_s,
            )
            return {"ok": True}
        except Exception as e:
            return {"ok": False, "error": repr(e)}

    def invoke_wait_for_insert_stage(
        self,
        index_name: str,
        request_ids: List[str],
        target_stage: str = "segmentation",
        timeout_s: float = 60.0,
        poll_interval_s: float = 0.5,
    ) -> None:
        """
        Raw call to ev.Index(name).wait_for_insert_stage(...). Raises on failure.
        """
        def _do_wait():
            index = ev.Index(index_name)
            return index.wait_for_insert_stage(
                request_ids=request_ids,
                target_stage=target_stage,
                timeout_s=timeout_s,
                poll_interval_s=poll_interval_s,
            )

        return self._with_reconnect(_do_wait)

    #------------------- Load ------------------#

    def call_load_index(self, index_name: str) -> Dict[str, Any]:
        """
        Trigger Index.load() out-of-band. Safe to call repeatedly — the SDK treats
        "already loaded" as a no-op.
        """
        try:
            def _do_load():
                ev.Index(index_name).load()
            self._with_reconnect(_do_load)
            return {"ok": True}
        except Exception as e:
            return {"ok": False, "error": repr(e)}

    #------------------- Scoring (Vault-Secured Pipeline) ------------------#

    def call_score(
        self, index_name: str, query: Union[List[float], List[List[float]]]
    ) -> Dict[str, Any]:
        """
        Query against the encrypted index and returns the result ciphertext for Vault decryption.

        Args:
            index_name: Index to search.
            query: Query vector(s).

        Returns:
            Dict with ok, encrypted_blobs (List[str] of base64-encoded CiphertextScore protobuf), or error.
        """
        def _do_score():
            index = ev.Index(index_name)
            scores = index.scoring(query)  # List[CipherBlock] with is_score=True
            encoded_blobs = []
            for cb in scores:
                # Serialize the CiphertextScore protobuf and encode to base64
                serialized = cb.data.SerializeToString()
                encoded_blob = base64.b64encode(serialized).decode('utf-8')
                encoded_blobs.append(encoded_blob)
            return {"ok": True, "encrypted_blobs": encoded_blobs}

        try:
            return self._with_reconnect(_do_score)
        except Exception as e:
            return {"ok": False, "error": repr(e)}

    def call_remind(
        self,
        index_name: str,
        indices: List[Dict[str, Any]],
        output_fields: List[str] = None,
    ) -> Dict[str, Any]:
        """
        Retrieves metadata for indices returned by Vault after decryption.

        Args:
            index_name: Index to fetch metadata from.
            indices: List of dicts with "shard_idx", "row_idx", "score".
            output_fields: Fields to include (default: ["metadata"]).

        Returns:
            Dict with ok, results (List[dict]), or error.
        """
        if output_fields is None:
            output_fields = ["metadata"]

        # Pre-validate before network call
        idx_list = []
        for entry in indices:
            row_idx = entry.get("row_idx")
            if row_idx is None:
                raise ValueError("Missing required 'row_idx' in index entry: " + repr(entry))
            idx_list.append(
                {
                    "shard_idx": entry.get("shard_idx", 0),
                    "row_idx": row_idx,
                }
            )

        def _do_remind():
            index = ev.Index(index_name)
            results = index.indexer.get_metadata(
                index_name=index_name,
                idx=idx_list,
                fields=output_fields,
            )
            # Convert protobuf Metadata objects to dicts and attach scores
            results_with_scores = []
            for i, entry in enumerate(indices):
                if i < len(results):
                    metadata_obj = results[i]
                    # Protobuf objects: use MessageToDict for proper field extraction
                    if hasattr(metadata_obj, 'ListFields'):
                        result_dict = MessageToDict(metadata_obj, preserving_proto_field_name=True)
                    elif hasattr(metadata_obj, '_asdict'):
                        result_dict = metadata_obj._asdict()
                    elif hasattr(metadata_obj, '__dict__'):
                        result_dict = metadata_obj.__dict__.copy()
                    else:
                        result_dict = {"metadata": str(metadata_obj)}

                    # Attach score from Vault
                    result_dict["score"] = entry.get("score", 0.0)
                    results_with_scores.append(result_dict)
            return self._to_json_available({"ok": True, "results": results_with_scores})

        try:
            return self._with_reconnect(_do_remind)
        except Exception as e:
            return {"ok": False, "error": repr(e)}

    @staticmethod
    def _to_json_available(obj: Any) -> Any:
        """
        Converts an object to a JSON-serializable format if possible.

        Args:
            obj (Any): The object to convert.

        Returns:
            Any: The JSON-serializable representation of the object, or the original object if conversion is not possible.
        """
        if obj is None or isinstance(obj, (str, int, float, bool)):
            return obj
        if isinstance(obj, dict):
            return {str(k): EnVectorSDKAdapter._to_json_available(v) for k, v in obj.items()}
        if isinstance(obj, (list, tuple, set)):
            return [EnVectorSDKAdapter._to_json_available(item) for item in obj]
        for attr in ("model_dump", "dict", "to_dict"):
            if hasattr(obj, attr):
                try:
                    return EnVectorSDKAdapter._to_json_available(getattr(obj, attr)())
                except Exception:
                    pass
        if hasattr(obj, "__dict__"):
            try:
                return {k: EnVectorSDKAdapter._to_json_available(v) for k, v in obj.__dict__.items() if not k.startswith("_")}
            except Exception:
                pass
        return repr(obj)
