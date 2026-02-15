#!/usr/bin/env python3
"""
Re-embedding Migration Script

Re-embeds existing records using the new multilingual embedding model.
Run this after switching from all-MiniLM-L6-v2 to paraphrase-multilingual-MiniLM-L12-v2
if existing search quality degrades.

Usage:
    python scripts/migrate_embeddings.py [--dry-run] [--batch-size 50]
"""

import sys
import argparse
from pathlib import Path

# Add project root to path
PROJECT_ROOT = Path(__file__).parent.parent
sys.path.insert(0, str(PROJECT_ROOT))
sys.path.insert(0, str(PROJECT_ROOT / "mcp"))


def main():
    parser = argparse.ArgumentParser(description="Re-embed existing records with multilingual model")
    parser.add_argument("--dry-run", action="store_true", help="Print what would be done without executing")
    parser.add_argument("--batch-size", type=int, default=50, help="Number of records to process per batch")
    parser.add_argument("--collection", type=str, default="rune-context", help="enVector collection name")
    args = parser.parse_args()

    from agents.common.config import load_config
    from agents.common.embedding_service import EmbeddingService
    from agents.common.envector_client import EnVectorClient

    config = load_config()

    print(f"[Migration] Initializing embedding service...")
    print(f"[Migration] Model: {config.embedding.model}")
    embedding_svc = EmbeddingService(mode=config.embedding.mode, model=config.embedding.model)

    if not embedding_svc.is_available:
        print("[Migration] ERROR: Embedding service not available")
        sys.exit(1)

    # Verify dimension
    test_vec = embedding_svc.embed_single("test")
    print(f"[Migration] Embedding dimension: {len(test_vec)}")

    if args.dry_run:
        print("[Migration] DRY RUN - no changes will be made")
        print(f"[Migration] Would re-embed records in collection '{args.collection}'")
        print(f"[Migration] Batch size: {args.batch_size}")
        return

    print(f"[Migration] Connecting to enVector at {config.envector.endpoint}...")
    client = EnVectorClient(
        address=config.envector.endpoint,
        access_token=config.envector.api_key or None,
    )

    if not client.is_connected:
        print("[Migration] ERROR: Could not connect to enVector")
        sys.exit(1)

    # Fetch all records
    print("[Migration] Fetching existing records...")
    try:
        records = client.list_all(limit=10000)
    except Exception as e:
        print(f"[Migration] ERROR: Failed to fetch records: {e}")
        sys.exit(1)

    total = len(records)
    print(f"[Migration] Found {total} records to re-embed")

    if total == 0:
        print("[Migration] No records to migrate")
        return

    # Process in batches
    migrated = 0
    errors = 0

    for i in range(0, total, args.batch_size):
        batch = records[i:i + args.batch_size]
        texts = []
        ids = []

        for record in batch:
            payload_text = record.get("payload", {}).get("text", "")
            if payload_text:
                texts.append(payload_text)
                ids.append(record.get("id", "unknown"))

        if not texts:
            continue

        try:
            print(f"[Migration] Re-embedding batch {i // args.batch_size + 1} ({len(texts)} records)...")
            embeddings = embedding_svc.embed(texts)

            for record_id, embedding in zip(ids, embeddings):
                try:
                    client.update_embedding(record_id, embedding)
                    migrated += 1
                except Exception as e:
                    print(f"[Migration] WARNING: Failed to update {record_id}: {e}")
                    errors += 1

        except Exception as e:
            print(f"[Migration] ERROR: Batch embedding failed: {e}")
            errors += len(texts)

    print(f"[Migration] Complete: {migrated} migrated, {errors} errors, {total} total")


if __name__ == "__main__":
    main()
