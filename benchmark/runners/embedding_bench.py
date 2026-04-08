#!/usr/bin/env python3
"""Embedding token length benchmark.

Measures how reusable_insight token length affects:
1. Novelty classification accuracy (duplicate/evolution/unrelated detection)
2. Recall precision (retrieval quality for known decisions)

Usage:
    python benchmark/runners/embedding_bench.py
    python benchmark/runners/embedding_bench.py --model Qwen/Qwen3-Embedding-0.6B
    python benchmark/runners/embedding_bench.py --report benchmark/reports/embedding_token_length.json
"""

from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

import numpy as np

# Add rune root to path for imports
RUNE_DIR = Path(__file__).resolve().parent.parent.parent
sys.path.insert(0, str(RUNE_DIR))

from agents.common.schemas.embedding import classify_novelty

DATASET_PATH = Path(__file__).resolve().parent.parent / "datasets" / "embedding_token_length.json"
REPORTS_DIR = Path(__file__).resolve().parent.parent / "reports"

# Acceptable novelty classes for each variant type
ACCEPTABLE_CLASSES = {
    "duplicate": {"near_duplicate"},
    "evolution": {"evolution", "related"},
    "unrelated": {"novel"},
}

TOKEN_LENGTHS = ["128", "256", "512", "768"]


def load_model(model_name: str):
    """Load sentence-transformers model."""
    from sentence_transformers import SentenceTransformer
    print(f"Loading model: {model_name}")
    t0 = time.monotonic()
    model = SentenceTransformer(model_name, trust_remote_code=True)
    print(f"Model loaded in {time.monotonic() - t0:.1f}s")
    return model


def embed_all(model, texts: list[str]) -> np.ndarray:
    """Embed all texts in one batch, return L2-normalized embeddings."""
    embeddings = model.encode(texts, normalize_embeddings=True, show_progress_bar=True)
    return np.array(embeddings)


def cosine_sim(a: np.ndarray, b: np.ndarray) -> float:
    """Cosine similarity between two L2-normalized vectors."""
    return float(np.dot(a, b))


def run_benchmark(model, dataset: dict) -> dict:
    """Run full benchmark and return results."""
    topics = dataset["topics"]

    # Collect all texts for batch embedding
    text_index = {}  # (topic_id, variant, length) -> index
    all_texts = []
    for topic in topics:
        for variant in ["original", "duplicate", "evolution", "unrelated"]:
            for length in TOKEN_LENGTHS:
                key = (topic["id"], variant, length)
                text_index[key] = len(all_texts)
                all_texts.append(topic["variants"][variant][length])

    print(f"\nEmbedding {len(all_texts)} texts...")
    embeddings = embed_all(model, all_texts)
    print(f"Embedding shape: {embeddings.shape}")

    # === Novelty Classification ===
    novelty_results = []
    for topic in topics:
        for variant in ["duplicate", "evolution", "unrelated"]:
            for length in TOKEN_LENGTHS:
                orig_idx = text_index[(topic["id"], "original", length)]
                var_idx = text_index[(topic["id"], variant, length)]
                sim = cosine_sim(embeddings[orig_idx], embeddings[var_idx])
                classification = classify_novelty(sim)
                correct = classification["class"] in ACCEPTABLE_CLASSES[variant]
                novelty_results.append({
                    "topic_id": topic["id"],
                    "language": topic["language"],
                    "variant": variant,
                    "length": length,
                    "similarity": round(sim, 4),
                    "predicted_class": classification["class"],
                    "expected_classes": list(ACCEPTABLE_CLASSES[variant]),
                    "correct": correct,
                })

    # === Recall Precision ===
    recall_results = []
    for length in TOKEN_LENGTHS:
        # Build "memory DB" from all originals at this length
        orig_embeddings = []
        orig_ids = []
        for topic in topics:
            idx = text_index[(topic["id"], "original", length)]
            orig_embeddings.append(embeddings[idx])
            orig_ids.append(topic["id"])
        orig_matrix = np.array(orig_embeddings)

        for topic in topics:
            for variant in ["duplicate", "evolution", "unrelated"]:
                var_idx = text_index[(topic["id"], variant, length)]
                query_emb = embeddings[var_idx]

                # Compute similarities to all originals
                sims = orig_matrix @ query_emb
                ranked = np.argsort(-sims)
                top_ids = [orig_ids[i] for i in ranked[:3]]

                target = topic["id"]
                recall_at_1 = target == top_ids[0]
                recall_at_3 = target in top_ids

                recall_results.append({
                    "topic_id": topic["id"],
                    "language": topic["language"],
                    "variant": variant,
                    "length": length,
                    "recall_at_1": recall_at_1,
                    "recall_at_3": recall_at_3,
                    "top3": top_ids,
                    "top3_sims": [round(float(sims[ranked[i]]), 4) for i in range(3)],
                })

    return {
        "embedding_dim": int(embeddings.shape[1]),
        "total_texts": len(all_texts),
        "novelty": novelty_results,
        "recall": recall_results,
    }


def print_summary(results: dict) -> None:
    """Print summary tables to terminal."""
    novelty = results["novelty"]
    recall = results["recall"]

    print(f"\n{'='*62}")
    print(f"Dim: {results['embedding_dim']}  |  Texts: {results['total_texts']}")
    print(f"{'='*62}")

    # --- Novelty by length ---
    print(f"\nNovelty Classification Accuracy")
    print(f"{'Length':<8} | {'duplicate':<12} | {'evolution':<12} | {'unrelated':<12} | {'Overall':<10}")
    print("-" * 62)
    for length in TOKEN_LENGTHS:
        row = [r for r in novelty if r["length"] == length]
        by_var = {}
        for variant in ["duplicate", "evolution", "unrelated"]:
            subset = [r for r in row if r["variant"] == variant]
            acc = sum(r["correct"] for r in subset) / len(subset) if subset else 0
            by_var[variant] = acc
        overall = sum(r["correct"] for r in row) / len(row) if row else 0
        print(f"{length:<8} | {by_var['duplicate']:>10.1%} | {by_var['evolution']:>10.1%} | {by_var['unrelated']:>10.1%} | {overall:>8.1%}")

    # --- Recall by length ---
    print(f"\nRecall Precision")
    print(f"{'Length':<8} | {'R@1 dup':<10} | {'R@3 evo':<10} | {'Unrel !@3':<10} | {'Overall':<10}")
    print("-" * 55)
    for length in TOKEN_LENGTHS:
        row = [r for r in recall if r["length"] == length]
        dup = [r for r in row if r["variant"] == "duplicate"]
        evo = [r for r in row if r["variant"] == "evolution"]
        unr = [r for r in row if r["variant"] == "unrelated"]
        r1_dup = sum(r["recall_at_1"] for r in dup) / len(dup) if dup else 0
        r3_evo = sum(r["recall_at_3"] for r in evo) / len(evo) if evo else 0
        unr_not3 = sum(not r["recall_at_3"] for r in unr) / len(unr) if unr else 0
        overall = (r1_dup + r3_evo + unr_not3) / 3
        print(f"{length:<8} | {r1_dup:>8.1%} | {r3_evo:>8.1%} | {unr_not3:>8.1%} | {overall:>8.1%}")

    # --- By language ---
    print(f"\nBy Language")
    print(f"{'Lang':<6} | {'Nov. Acc':<10} | {'R@1 dup':<10} | {'R@3 evo':<10}")
    print("-" * 42)
    langs = sorted(set(r["language"] for r in novelty))
    for lang in langs:
        nov_sub = [r for r in novelty if r["language"] == lang]
        nov_acc = sum(r["correct"] for r in nov_sub) / len(nov_sub) if nov_sub else 0
        rec_sub = [r for r in recall if r["language"] == lang]
        dup_sub = [r for r in rec_sub if r["variant"] == "duplicate"]
        evo_sub = [r for r in rec_sub if r["variant"] == "evolution"]
        r1 = sum(r["recall_at_1"] for r in dup_sub) / len(dup_sub) if dup_sub else 0
        r3 = sum(r["recall_at_3"] for r in evo_sub) / len(evo_sub) if evo_sub else 0
        print(f"{lang:<6} | {nov_acc:>8.1%} | {r1:>8.1%} | {r3:>8.1%}")

    # --- Similarity distributions ---
    print(f"\nSimilarity Distribution (mean +/- std)")
    print(f"{'Length':<8} | {'duplicate':<18} | {'evolution':<18} | {'unrelated':<18}")
    print("-" * 68)
    for length in TOKEN_LENGTHS:
        row = [r for r in novelty if r["length"] == length]
        parts = []
        for variant in ["duplicate", "evolution", "unrelated"]:
            subset = [r["similarity"] for r in row if r["variant"] == variant]
            mean = np.mean(subset) if subset else 0
            std = np.std(subset) if subset else 0
            parts.append(f"{mean:.3f} +/- {std:.3f}")
        print(f"{length:<8} | {parts[0]:<18} | {parts[1]:<18} | {parts[2]:<18}")


def main():
    parser = argparse.ArgumentParser(description="Embedding token length benchmark")
    parser.add_argument("--model", default="Qwen/Qwen3-Embedding-0.6B",
                        help="Sentence-transformers model name")
    parser.add_argument("--dataset", default=str(DATASET_PATH),
                        help="Path to dataset JSON")
    parser.add_argument("--report", default=None,
                        help="Path to save JSON report")
    args = parser.parse_args()

    with open(args.dataset) as f:
        dataset = json.load(f)

    model = load_model(args.model)
    results = run_benchmark(model, dataset)
    print_summary(results)

    if args.report:
        report_path = Path(args.report)
        report_path.parent.mkdir(parents=True, exist_ok=True)
        with open(report_path, "w") as f:
            json.dump(results, f, indent=2, ensure_ascii=False)
        print(f"\nReport saved to {report_path}")


if __name__ == "__main__":
    main()
