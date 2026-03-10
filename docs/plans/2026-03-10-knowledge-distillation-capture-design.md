# Knowledge Distillation Capture — Design Document

**Date**: 2026-03-10
**Status**: Approved
**Scope**: Scribe capture accuracy improvement for agentic coding workflows

## Problem

Current Rune scribe captures decisions expressed in explicit natural language
("We decided to use X because Y"). In agentic coding workflows (Claude Code,
Gemini CLI, Codex), significant decisions happen during code generation,
debugging, and optimization — but:

1. Raw artifacts (code, diffs, build output) live in subprocesses/tool calls,
   not in the conversation prompt
2. The "knowledge" is scattered across tool call results
3. No trigger patterns exist for code-context decisions

The scribe needs to evolve from a **decision language sensor** to a
**knowledge distiller** — capturing condensed, shareable insights from
coding workflows.

## Design

Three complementary changes, prioritized:

### 1. Scribe Prompt Enhancement (Priority 1)

**Files**: `agents/claude/scribe.md`, `agents/gemini/scribe.md`

Add "Agentic Coding Context" to Step 1 CAPTURE criteria:
- Root cause discovery (bug cause + fix approach)
- Performance insight (bottleneck + optimization + before/after impact)
- Problem reframing (initial assumption wrong, real cause found)
- Architecture pivot during implementation
- Non-obvious dependency or interaction discovered
- Pattern establishment from concrete fix

Add distillation guidance: capture the INSIGHT, not the raw artifact.
Include minimal evidence (code snippet up to 50 lines), not full diffs.

Add optional fields to Step 2 extraction formats:
- `evidence_type`: `code_change | git_bisect | benchmark | error_trace | runtime_observation`
- `evidence_snippet`: Minimal proof (up to 50 lines) at phase level for bundles
- `reusable_insight`: One-sentence generalizable lesson

Code-heavy captures should prefer **bundle format** with phases:
1. Core Insight
2. Problem Context (+ evidence)
3. Root Cause (+ evidence)
4. Solution Applied (+ evidence)
5. Impact / Verification

### 2. Capture Triggers Extension (Priority 1, parallel)

**Files**: `patterns/capture-triggers.md`, `capture-triggers.ko.md`, `capture-triggers.ja.md`

New section "Agentic Coding Context" with sub-categories:
- Root Cause Discovery patterns (EN/KO/JA)
- Performance & Optimization patterns
- Problem Reframing patterns
- Architecture Pivot patterns
- Pattern Establishment patterns
- Non-obvious Dependency patterns

### 3. Benchmark Scenarios (Priority 1, parallel)

**Directory**: `benchmark/scenarios/capture/should_capture/coding_context/`

New scenario files (~18 should_capture scenarios):
- `root_cause.jsonl` — debugging breakthroughs with code evidence
- `optimization.jsonl` — performance improvements with metrics
- `reframing.jsonl` — problem reinterpretation
- `architecture_pivot.jsonl` — implementation-time direction changes
- `pattern_establish.jsonl` — team rules derived from concrete fixes

New should_not_capture category:
- `benchmark/scenarios/capture/should_not_capture/code_noise/scenarios.jsonl` (~4 scenarios)
  - Mechanical type fixes, variable renames, dependency bumps, test-pass confirmations

### Embedding Consideration

Current model (multilingual-MiniLM-L12-v2) is NL-optimized. Strategy:
- `payload.text` renders prose (title, rationale, insight) prominently at top
- Code evidence placed below as supporting reference
- Embedding matches on prose semantics; code is for human consumption post-retrieval

### Schema Changes

`benchmark/scenarios/schema.json` — add optional fields to `expected_fields`:
- `evidence_type`: string enum
- `has_reusable_insight`: boolean

`agents/common/schemas/decision_record.py` — no changes needed (evidence fits
in existing `payload.text` Markdown rendering)

## Out of Scope (Future — Approach B)

- Breakpoint hooks (post-commit, post-PR synthesis)
- Claude Code hooks integration
- Git post-commit hook for automatic capture
- These require separate infrastructure per agent platform

## Success Criteria

Run `scribe_bench.py` before and after changes:
- New `coding_context` category: >= 80% capture accuracy
- Existing categories: no regression (maintain current accuracy)
- New `code_noise` category: >= 90% correct rejection
