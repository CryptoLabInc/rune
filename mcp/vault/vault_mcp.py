from fastmcp import FastMCP
import base64
import pickle
import numpy as np
import os
import json
from pyenvector.crypto import KeyGenerator, Cipher
from pyenvector.crypto.block import CipherBlock, Query

# Configuration
KEY_DIR = "vault_keys"
KEY_ID = "vault-key"
DIM = 32 # Using small dim for demo

# Initialize Keys on Startup
def ensure_keys():
    if not os.path.exists(KEY_DIR):
        print(f"Generating keys in {KEY_DIR}...")
        keygen = KeyGenerator(key_path=KEY_DIR, key_id=KEY_ID, dim_list=[DIM])
        keygen.generate_keys()
    else:
        print(f"Keys found in {KEY_DIR}")

ensure_keys()
enc_key_path = os.path.join(KEY_DIR, "EncKey.json")
sec_key_path = os.path.join(KEY_DIR, "SecKey.json")

# Initialize shared Cipher instance (thread-safety? FastMCP uses asyncio/uvicorn workers)
# Usually safe for read-ops.
cipher = Cipher(enc_key_path=enc_key_path, dim=DIM)

# Authorization
VALID_TOKENS = {
    "envector-team-alpha", 
    "envector-admin-001"
}

def validate_token(token: str):
    if token not in VALID_TOKENS:
        raise ValueError(f"Access Denied: Invalid Authentication Token '{token}'")

# MCP Server
mcp = FastMCP("enVector-Vault")

@mcp.tool()
def get_public_key(token: str) -> str:
    """
    Returns the public key bundle (EncKey, EvalKey, MetadataKey).
    This bundle allows the Agent to encrypt data/queries and register keys with the Cloud.
    
    Args:
        token: Authentication token issued by Vault Admin.
        
    Returns:
        JSON string containing:
        {
            "EncKey.json": "...",
            "EvalKey.json": "...",
            "MetadataKey.json": "..."
        }
    """
    validate_token(token)
    
    bundle = {}
    for filename in ["EncKey.json", "EvalKey.json", "MetadataKey.json"]:
        path = os.path.join(KEY_DIR, filename)
        if os.path.exists(path):
            with open(path, "r") as f:
                bundle[filename] = f.read()
        else:
            # Should not happen if ensure_keys ran
            pass
            
    return json.dumps(bundle)

@mcp.tool()
def decrypt_scores(token: str, encrypted_blob_b64: str, top_k: int = 5) -> str:
    """
    Decrypts a blob of encrypted scores using the Vault's Secret Key.
    Applies Top-K filtering and returns the result.
    
    Args:
        token: Authentication token issued by Vault Admin.
        encrypted_blob_b64: Base64 string of the serialized CipherBlock (Query) from the Cloud.
        top_k: Number of top results to return (max 10 allowed).
    
    Returns:
        JSON string containing the list of scores (and implicitly indices).
    """
    validate_token(token)
    
    # Policy Enforcement
    if top_k > 10:
        return json.dumps({"error": "Rate Limit Exceeded: Max top_k is 10"})

    try:
        # 1. Deserialize
        blob_bytes = base64.b64decode(encrypted_blob_b64)
        
        # Native deserialization
        # `CipherBlock.serialize` returns the raw bytes compatible with Query/Score serialization.
        # Since we encrypted a vector ("item" or "query"), it's likely a Query object internally (or CiphertextScore if score).
        # We assume it's a Query object for now as observed in verify_crypto.
        
        try:
             query_obj = Query.deserializeFrom(blob_bytes)
             encrypted_result = CipherBlock(data=query_obj)
        except Exception as e:
             # Fallback: maybe it was pickling? No, we moved to native.
             return json.dumps({"error": f"Deserialization failed: {str(e)}"})
        
        # 2. Decrypt
        decrypted_vector = cipher.decrypt(encrypted_result, sec_key_path=sec_key_path)
        
        # unwrapping list logic (from verification script)
        if isinstance(decrypted_vector, list) and len(decrypted_vector) > 0 and (isinstance(decrypted_vector[0], list) or isinstance(decrypted_vector[0], np.ndarray)):
            decrypted_vector = decrypted_vector[0]
            
        # 3. Top-K
        scores = np.array(decrypted_vector)
        # Get indices of top_k scores (descending)
        # Note: argpartition is faster but argsort is easier for full sort
        if len(scores) <= top_k:
            top_indices = np.argsort(scores)[::-1]
        else:
            top_indices = np.argpartition(scores, -top_k)[-top_k:]
            top_indices = top_indices[np.argsort(scores[top_indices])[::-1]]
            
        params = []
        for idx in top_indices:
            params.append({
                "index": int(idx),
                "score": float(scores[idx])
            })
            
        return json.dumps(params)

    except Exception as e:
        return json.dumps({"error": str(e)})

if __name__ == "__main__":
    import sys
    import argparse
    
    parser = argparse.ArgumentParser(description="Run the enVector-Vault MCP server.")
    parser.add_argument("command", nargs="?", choices=["server"], help="Command to run")
    parser.add_argument("--mode", choices=["sse", "http"], default="sse", help="Transport mode")
    parser.add_argument("--port", type=int, default=50080, help="Port to bind")
    parser.add_argument("--host", default="0.0.0.0", help="Host to bind")
    
    args = parser.parse_args()

    if args.command == "server":
        print(f"Starting enVector-Vault MCP Server (SSE) on {args.host}:{args.port}...")
        
        # SSE is standard for FastMCP
        import uvicorn
        # Note: sse_app() returns a Starlette/FastAPI app
        app = mcp.sse_app()
        uvicorn.run(app, host=args.host, port=args.port)
            
    else:
        # Default to stdio for CLI / Inspector usage
        mcp.run()
