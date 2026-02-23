# Summary of file: enVector SDK Adapter(enVector APIs Caller)

from typing import Union, List, Dict, Any
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
            query_encryption: bool,
            access_token: str = None,
            auto_key_setup: bool = True,
            agent_id: str = None,
            agent_dek: bytes = None,
        ):
        """
        Initializes the EnVectorSDKAdapter with an optional endpoint.

        Args:
            address (str): The endpoint URL for the enVector SDK.
            key_id (str): The key identifier for the enVector SDK.
            key_path (str): The path to the key files.
            eval_mode (str): The evaluation mode for the enVector SDK.
            query_encryption (bool): Whether to encrypt the query vectors.
            access_token (str, optional): The access token for the enVector SDK.
            auto_key_setup (bool): If True, generates keys automatically when not found.
                                   Set to False when keys are provided externally (e.g., from Vault).
            agent_id (str): Per-agent identifier for app-layer metadata encryption.
            agent_dek (bytes): Per-agent AES-256 DEK (32 bytes) for metadata encryption.
        """
        if not key_path:
            key_path = str(KEY_PATH)
        self.query_encryption = query_encryption
        self._agent_id = agent_id
        self._agent_dek = agent_dek
        ev.init(address=address, key_path=key_path, key_id=key_id, eval_mode=eval_mode, auto_key_setup=auto_key_setup, access_token=access_token)

    #------------------- Create Index ------------------#

    def call_create_index(self, index_name, dim, index_params) -> Dict[str, Any]:
        """
        Create a new empty index.

        Args
        ----------
            index_name (str): The name of the index.
            dim (int): The dimensionality of the index.
            index_params (dict, optional): The parameters for the index.

        Returns
        -------
            Dict[str, Any]: If succeed, converted format of the create index results. Otherwise, error message.
        """
        try:
            results = self.invoke_create_index(index_name=index_name, dim=dim, index_params=index_params)
            return self._to_json_available({"ok": True, "results": results})
        except Exception as e:
            # Handle exceptions and return an appropriate error message
            return {"ok": False, "error": repr(e)}

    def invoke_create_index(self, index_name: str, dim: int, index_params: Dict[str, Any] = None):
        """
        Invokes the enVector SDK's create_index functionality.

        Args:
            index_name (str): The name of the index.
            dim (int): The dimensionality of the index.
            index_params (dict, optional): The parameters for the index.

        Returns:
            Any: Raw create index results from the enVector SDK.
        """
        # Return the created index instance
        if self.query_encryption:
            return ev.create_index(index_name=index_name, dim=dim, index_params=index_params, query_encryption="cipher")
        else:
            return ev.create_index(index_name=index_name, dim=dim, index_params=index_params, query_encryption="plain")

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

    def invoke_get_index_list(self) -> List[str]:
        """
        Invokes the enVector SDK's get_index_list functionality.

        Returns:
            List[str]: List of index names from the enVector SDK.
        """
        return ev.get_index_list()

    #--------------- Get Index Info --------------#
    def call_get_index_info(self, index_name: str) -> Dict[str, Any]:
        """
        Calls the enVector SDK to get the information of a specific index.

        Args:
            index_name (str): The name of the index.

        Returns:
            Dict[str, Any]: If succeed, converted format of the index info. Otherwise, error message.
        """
        try:
            results = self.invoke_get_index_info(index_name=index_name)
            return self._to_json_available({"ok": True, "results": results})
        except Exception as e:
            # Handle exceptions and return an appropriate error message
            return {"ok": False, "error": repr(e)}

    def invoke_get_index_info(self, index_name: str) -> Dict[str, Any]:
        """
        Invokes the enVector SDK's get_index_info functionality.

        Args:
            index_name (str): The name of the index.

        Returns:
            Dict[str, Any]: Index information from the enVector SDK.
        """
        return ev.get_index_info(index_name=index_name)

    #------------------- Insert ------------------#

    def call_insert(self, index_name: str, vectors: List[List[float]], metadata: List[Any] = None):
        """
        Calls the enVector SDK to perform an insert operation.

        Args:
            vectors (List[List[float]]): The list of vectors to insert.
            metadata (List[Any], optional): The list of metadata associated with the vectors. Defaults to None.

        Returns:
            Dict[str, Any]: If succeed, converted format of the insert results. Otherwise, error message.
        """
        try:
            results = self.invoke_insert(index_name=index_name, vectors=vectors, metadata=metadata)
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

    def invoke_insert(self, index_name: str, vectors: List[List[float]], metadata: List[Any] = None):
        """
        Invokes the enVector SDK's insert functionality.

        Args:
            index_name (str): The name of the index to insert into.
            vectors (Union[List[List[float]], List[CipherBlock]]): The list of vectors to insert.
            metadata (List[Any], optional): The list of metadata associated with the vectors. Defaults to None.

        Returns:
            Any: Raw insert results from the enVector SDK.
        """
        # App-layer metadata encryption with per-agent DEK
        if self._agent_dek and metadata:
            if not self._agent_id:
                logger.warning("agent_dek is set but agent_id is missing — skipping metadata encryption")
            else:
                metadata = [self._app_encrypt_metadata(m) for m in metadata]

        index = ev.Index(index_name)  # Create an index instance with the given index name
        # Insert vectors with optional metadata
        return index.insert(data=vectors, metadata=metadata) # Return list of inserted vectors' IDs

    #------------------- Search ------------------#

    def call_search(self, index_name: str, query: Union[List[float], List[List[float]]], topk: int) -> Dict[str, Any]:
        """
        Calls the enVector SDK to perform a search operation.

        Args:
            index_name (str): The name of the index to search.
            query (Union[List[float], List[List[float]]]): The search query.
            topk (int): The number of top results to return.

        Returns:
            Dict[str, Any]: If succeed, converted format of the search results. Otherwise, error message.
        """
        try:
            results = self.invoke_search(index_name=index_name, query=query, topk=topk)
            return self._to_json_available({"ok": True, "results": results})
        except Exception as e:
            # Handle exceptions and return an appropriate error message
            return {"ok": False, "error": repr(e)}

    def invoke_search(self, index_name: str, query: Union[List[float], List[List[float]]], topk: int):
        """
        Invokes the enVector SDK's search functionality.

        Args:
            index_name (str): The name of the index to search.
            query (Union[List[float], List[List[float]]]): The search query.
            topk (int): The number of top results to return.

        Returns:
            Any: Raw search results from the enVector SDK.
        """
        index = ev.Index(index_name)  # Create an index instance with the given index name
        # Search with the provided query and topk. Fixed output_fields parameter for now.
        return index.search(query, top_k=topk, output_fields=["metadata"])

    #------------------- Remember (Vault-Secured Pipeline) ------------------#

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
        try:
            index = ev.Index(index_name)
            scores = index.scoring(query)  # List[CipherBlock] with is_score=True
            encoded_blobs = []
            for cb in scores:
                # Serialize the CiphertextScore protobuf and encode to base64
                serialized = cb.data.SerializeToString()
                encoded_blob = base64.b64encode(serialized).decode('utf-8')
                encoded_blobs.append(encoded_blob)
            return {"ok": True, "encrypted_blobs": encoded_blobs}
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
        try:
            if output_fields is None:
                output_fields = ["metadata"]

            index = ev.Index(index_name)
            # Indexer.get_metadata expects [{"shard_idx": int, "row_idx": int}]
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
