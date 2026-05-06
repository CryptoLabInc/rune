# Rune Benchmark

Benchmark suite for evaluating Rune's capture, recall, and extraction quality.

## Architecture

Since v0.2.0, Rune operates in **agent-delegated mode**: the calling agent (Claude/Codex/Gemini) performs LLM reasoning using the scribe prompt, then passes pre-extracted JSON to the MCP server. The server handles only embedding, encryption, and storage.

This means **capture quality is determined by the scribe prompt**, not by Rune's internal modules. The benchmark reflects this:

| Benchmark                     | What it tests                                                  | Method                                                                 |
| ----------------------------- | -------------------------------------------------------------- | ---------------------------------------------------------------------- |
| **scribe_bench** (capture)    | Does the scribe prompt correctly guide capture/skip decisions? | Feed scribe prompt + scenario input to LLM, score the output JSON      |
| **scribe_bench** (extraction) | Is the extracted JSON well-structured?                         | Same LLM call, score field coverage and extraction type                |
| **retriever_bench**           | Can stored decisions be retrieved with various queries?        | Offline embedding similarity (FHE is transparent to scores)            |
| **latency_bench**             | How long does each pipeline phase take?                        | Direct adapter calls against live envector-msa-1.4.0, per-phase timing |

## Running

### Quality benchmarks (scribe / retriever)

```bash
# Default: uses claude CLI (no API key needed)
python benchmark/runners/scribe_bench.py

# Use a different agent CLI
python benchmark/runners/scribe_bench.py --agent gemini
python benchmark/runners/scribe_bench.py --agent codex

# Capture only / extraction only
python benchmark/runners/scribe_bench.py --mode capture
python benchmark/runners/scribe_bench.py --agent gemini --mode extraction

# Filter by category
python benchmark/runners/scribe_bench.py --category pr_review

# Retriever benchmark (no API key or CLI needed — uses local embeddings)
python benchmark/runners/retriever_bench.py
python benchmark/runners/retriever_bench.py --category semantic_match

# Save report
python benchmark/runners/scribe_bench.py --report benchmark/reports/scribe.json

# Fallback: direct API call (for CI without CLI auth)
python benchmark/runners/scribe_bench.py --api-key $ANTHROPIC_API_KEY --provider anthropic
python benchmark/runners/scribe_bench.py --api-key $OPENAI_API_KEY --provider openai --model gpt-4o
```

### Latency benchmark (envector-msa-1.4.0)

Requires: Vault running (`tcp://localhost:50051`) and envector-msa-1.4.0 endpoint reachable.

```bash
# All scenarios (10 runs, 2 warmup)
python benchmark/runners/latency_bench.py

# Single feature
python benchmark/runners/latency_bench.py --feature capture
python benchmark/runners/latency_bench.py --feature recall
python benchmark/runners/latency_bench.py --feature batch_capture
python benchmark/runners/latency_bench.py --feature vault_status

# Custom run count (20 total, 3 warmup → 17 effective)
python benchmark/runners/latency_bench.py --runs 20 --warmup 3

# Save Markdown report
python benchmark/runners/latency_bench.py \
    --report benchmark/reports/latency_results_v1.4.0.md \
    --format md

# Save JSON report
python benchmark/runners/latency_bench.py \
    --report benchmark/reports/latency_results_v1.4.0.json \
    --format json
```

Pipeline phases measured per feature:

| Feature         | Phases                                                   |
| --------------- | -------------------------------------------------------- |
| `capture`       | embed → score (FHE search) → vault_topk → insert → total |
| `recall`        | embed → score → vault_topk → remind → total              |
| `batch_capture` | total_batch, per_item (embed+score per item)             |
| `vault_status`  | vault_health_check                                       |

## Scenarios (104 total)

```
scenarios/
├── capture/
│   ├── should_capture/         55 scenarios
│   │   ├── architecture/       10  (incl. mixed-language edge cases)
│   │   ├── debugging/           9  (incl. implicit decision in FYI)
│   │   ├── incident/            6
│   │   ├── product/             8
│   │   ├── tradeoff/            6
│   │   ├── process/             6
│   │   └── pr_review/          10  (incl. cross-team, subtle standard-setting)
│   └── should_not_capture/     23 scenarios
│       ├── casual/              4
│       ├── status_update/       4
│       ├── question/            4
│       ├── slop/                6  (AI fluff, non-committal, verbose nothing)
│       └── pr_noise/            5  (LGTM, nitpick, lint, merge conflict)
├── recall/                     16 scenarios
│   ├── exact_match/             4
│   ├── semantic_match/          6  (incl. cross-language Korean→English)
│   ├── cross_domain/            3
│   └── temporal/                3  (superseded decisions)
└── extraction/                 10 scenarios
    ├── single/                  4
    ├── phase_chain/             3
    └── bundle/                  3
```

## Scoring

### Capture Accuracy
- **True Positive**: should_capture scenario correctly flagged
- **True Negative**: should_not_capture scenario correctly skipped
- **False Positive**: noise incorrectly captured (worse than FN for storage cost)
- **False Negative**: decision missed

### Extraction Quality
- **Type accuracy**: correct extraction mode (single / phase_chain / bundle)
- **Title keyword match**: extracted title contains expected keywords
- **Status accuracy**: correct status_hint (accepted / proposed / rejected)
- **Field coverage**: sufficient alternatives and trade-offs extracted
- **Phase count**: reasonable number of phases for multi-phase/bundle

### Retriever Quality
- **Hit@K**: target record appears in results above min_score threshold
- **MRR**: Mean Reciprocal Rank of target records

## Adding Your Own Scenarios

When Rune doesn't behave as expected — a decision was missed, noise was captured, or the extraction structure was wrong — paste the actual conversation into a scenario to turn it into a regression test.

### Field Reference

#### Capture scenario fields

| Field                            | Required | Description                                                                                                                                                      |
| -------------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`                             | yes      | Unique identifier. Format: `{category}-{description}-{number}` (e.g., `debug-grpc-timeout-009`)                                                                  |
| `category`                       | yes      | Directory path under `scenarios/`. Must match where the file lives (e.g., `capture/should_capture/debugging`)                                                    |
| `language`                       | yes      | Language of the input: `en`, `ko`, `ja`, or `mixed`                                                                                                              |
| `input`                          | yes      | The actual conversation text to evaluate. Paste the real message as-is                                                                                           |
| `expected_capture`               | yes      | `true` if this should be captured, `false` if it should be skipped                                                                                               |
| `expected_fields`                | no       | Expected metadata when captured (see below). Use `{}` for should_not_capture scenarios                                                                           |
| `expected_fields.domain`         | no       | Expected domain classification: `architecture`, `security`, `product`, `debugging`, `incident`, `ops`, `process`, `qa`, `hr`, `data`, `finance`, `general`, etc. |
| `expected_fields.status_hint`    | no       | Expected decision status: `accepted`, `proposed`, or `rejected`                                                                                                  |
| `expected_fields.title_keywords` | no       | List of keywords that should appear in the extracted title (case-insensitive, any match passes)                                                                  |
| `recall_queries`                 | no       | Optional queries that should (or should not) retrieve this decision after capture                                                                                |
| `notes`                          | no       | Free-text annotation explaining why this scenario is interesting or tricky                                                                                       |

#### Retriever scenario fields

| Field                   | Required | Description                                                                                             |
| ----------------------- | -------- | ------------------------------------------------------------------------------------------------------- |
| `id`                    | yes      | Format: `recall-{subcategory}-{description}-{number}`                                                   |
| `category`              | yes      | One of: `recall/exact_match`, `recall/semantic_match`, `recall/cross_domain`, `recall/temporal`         |
| `language`              | yes      | Language of the query                                                                                   |
| `seed_records`          | yes      | Array of records to index before querying. Each has `title`, `domain`, `content`, and optionally `tags` |
| `query`                 | yes      | The recall query to test                                                                                |
| `expected_match_titles` | yes      | Titles of seed records that should appear in results above `min_score`                                  |
| `min_score`             | no       | Minimum cosine similarity threshold (default: 0.35). Lower for semantic/cross-domain tests              |
| `notes`                 | no       | Free-text annotation                                                                                    |

#### Extraction scenario fields

| Field                              | Required | Description                                                                |
| ---------------------------------- | -------- | -------------------------------------------------------------------------- |
| `id`                               | yes      | Format: `extract-{type}-{description}-{number}`                            |
| `category`                         | yes      | One of: `extraction/single`, `extraction/phase_chain`, `extraction/bundle` |
| `language`                         | yes      | Language of the input                                                      |
| `input`                            | yes      | The conversation text to extract from                                      |
| `expected_extraction_type`         | yes      | Expected extraction mode: `single`, `phase_chain`, or `bundle`             |
| `expected_fields.title_keywords`   | no       | Keywords expected in extracted title                                       |
| `expected_fields.status_hint`      | no       | Expected status: `accepted`, `proposed`, `rejected`                        |
| `expected_fields.min_alternatives` | no       | Minimum number of alternatives that should be extracted                    |
| `expected_fields.min_trade_offs`   | no       | Minimum number of trade-offs that should be extracted                      |
| `expected_fields.min_phases`       | no       | Minimum phase count (for phase_chain/bundle)                               |
| `expected_fields.max_phases`       | no       | Maximum phase count (for phase_chain/bundle)                               |
| `notes`                            | no       | Free-text annotation                                                       |

### 1. Capture Scenarios

A decision that should have been captured but wasn't — append a line to the matching category's JSONL:

```bash
# e.g., a debugging decision that was missed
vi benchmark/scenarios/capture/should_capture/debugging/scenarios.jsonl
```

```json
{"id": "debug-your-case-009", "category": "capture/should_capture/debugging", "language": "en", "input": "Paste the actual conversation here", "expected_capture": true, "expected_fields": {"domain": "debugging", "status_hint": "accepted", "title_keywords": ["key", "terms"]}, "recall_queries": [{"query": "A question you'd use to find this decision later", "should_match": true}]}
```

Noise that was incorrectly captured:

```json
{"id": "slop-your-case-007", "category": "capture/should_not_capture/slop", "language": "en", "input": "Content that should not have been captured", "expected_capture": false, "expected_fields": {}, "recall_queries": []}
```

### 2. Retriever Scenarios

A query that failed to surface a known decision:

```json
{"id": "recall-semantic-your-case-007", "category": "recall/semantic_match", "language": "en", "seed_records": [{"title": "Title of the stored record", "domain": "architecture", "content": "Body of the stored record"}], "query": "The query that should have matched but didn't", "expected_match_titles": ["Title of the stored record"], "min_score": 0.3}
```

### 3. Extraction Scenarios

Extraction produced the wrong structure (e.g., single decision extracted as phase_chain):

```json
{"id": "extract-single-your-case-005", "category": "extraction/single", "language": "en", "input": "Content to extract from", "expected_extraction_type": "single", "expected_fields": {"title_keywords": ["expected", "keywords"], "status_hint": "accepted", "min_alternatives": 1, "min_trade_offs": 1}}
```

### Tips

- **Avoid duplicate ids** — increment the number past the highest existing one
- **Paste real conversations** as input — real-world data makes better benchmarks than synthetic examples
- **Redact sensitive data**: mask API keys, passwords, and PII before adding
- Verify immediately after adding:

  ```bash
  python benchmark/runners/scribe_bench.py --category debugging -v

  ```
