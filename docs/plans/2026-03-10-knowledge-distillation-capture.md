# Knowledge Distillation Capture — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve scribe capture accuracy for agentic coding workflows by adding knowledge distillation patterns, new benchmark scenarios, and extraction format extensions.

**Architecture:** TDD approach — write benchmark scenarios first (the "tests"), then modify scribe prompts and triggers (the "implementation"), then run benchmarks to validate. All changes are prompt/data-only; no Python code changes needed except schema.json.

**Tech Stack:** JSONL scenarios, Markdown prompts, JSON Schema

---

## Task 1: Run Baseline Benchmark

Capture current scribe_bench results before any changes.

**Files:**
- Read: `benchmark/runners/scribe_bench.py`

**Step 1: Run capture benchmark with default agent (claude)**

Run: `cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python benchmark/runners/scribe_bench.py --mode capture --report benchmark/reports/baseline-capture.json`

If claude CLI auth is unavailable, use API:
Run: `python benchmark/runners/scribe_bench.py --mode capture --api-key $ANTHROPIC_API_KEY --provider anthropic --report benchmark/reports/baseline-capture.json`

Save the output — this is our regression baseline.

**Step 2: Record baseline numbers**

Note overall accuracy and per-category breakdown. We must not regress on existing categories.

---

## Task 2: Update Schema

Add optional fields to the capture scenario schema for coding context validation.

**Files:**
- Modify: `benchmark/scenarios/schema.json`

**Step 1: Add new optional fields to CaptureScenario's expected_fields**

In the `CaptureScenario` definition's `expected_fields.properties`, add:

```json
"evidence_type": {
  "type": "string",
  "enum": ["code_change", "git_bisect", "benchmark", "error_trace", "runtime_observation"]
},
"has_reusable_insight": {
  "type": "boolean"
}
```

These are optional — existing scenarios are unaffected.

**Step 2: Commit**

```bash
git add benchmark/scenarios/schema.json
git commit -m "feat(schema): add evidence_type and has_reusable_insight to capture scenario schema"
```

---

## Task 3: Write should_capture Benchmark Scenarios — Root Cause Discovery

**Files:**
- Create: `benchmark/scenarios/capture/should_capture/coding_context/root_cause.jsonl`

**Step 1: Create directory**

```bash
mkdir -p benchmark/scenarios/capture/should_capture/coding_context
```

**Step 2: Write root_cause.jsonl**

Write 4 scenarios as single-line JSONL. Each scenario represents a debugging breakthrough with code evidence that should be captured.

Scenario patterns:
1. **Memory leak + listener cleanup** (EN) — WebSocket handler registering listeners without removal, fix with `removeAllListeners()`. `evidence_type: "code_change"`, `has_reusable_insight: true`
2. **Race condition + lock fix** (EN) — TOCTOU in order creation, fix with `SELECT FOR UPDATE SKIP LOCKED`. `evidence_type: "code_change"`
3. **Git bisect regression** (EN) — bisected to specific commit that broke auth flow, includes git output. `evidence_type: "git_bisect"`
4. **Stack trace misdirection** (KO) — 스택 트레이스는 API layer를 가리켰지만 실제 원인은 캐시 무효화. `evidence_type: "error_trace"`, language: "ko"

Each scenario follows this exact format (one line per scenario in JSONL):
```json
{"id": "coding-root-cause-001", "category": "capture/should_capture/coding_context", "language": "en", "input": "<realistic text with code snippets>", "expected_capture": true, "expected_fields": {"domain": "debugging", "status_hint": "accepted", "title_keywords": ["memory leak", "listener"], "evidence_type": "code_change", "has_reusable_insight": true}, "recall_queries": [{"query": "WebSocket memory leak fix", "should_match": true}]}
```

Input text should be 100-300 words, include realistic code snippets (diff format or inline), and end with a generalizable insight.

**Step 3: Validate JSONL format**

```bash
python3 -c "
import json
with open('benchmark/scenarios/capture/should_capture/coding_context/root_cause.jsonl') as f:
    for i, line in enumerate(f, 1):
        obj = json.loads(line)
        assert 'id' in obj and 'input' in obj and 'expected_capture' in obj
        print(f'  OK: {obj[\"id\"]}')
print(f'Total: {i} scenarios')
"
```

Expected: 4 scenarios, all parse successfully.

---

## Task 4: Write should_capture Scenarios — Optimization

**Files:**
- Create: `benchmark/scenarios/capture/should_capture/coding_context/optimization.jsonl`

**Step 1: Write optimization.jsonl**

4 scenarios:
1. **O(n^2) to O(n) hash set** (EN) — profiling result + diff + before/after latency. `evidence_type: "benchmark"`
2. **N+1 query elimination** (EN) — added eager loading, query count from 50 to 1. `evidence_type: "code_change"`
3. **Bundle size reduction** (EN) — tree-shaking + code splitting, 2MB to 400KB. `evidence_type: "benchmark"`
4. **DB index optimization** (KO) — 풀스캔에서 인덱스 스캔으로, EXPLAIN 결과 포함. `evidence_type: "benchmark"`, language: "ko"

**Step 2: Validate JSONL**

Same validation as Task 3.

---

## Task 5: Write should_capture Scenarios — Reframing, Pivot, Pattern

**Files:**
- Create: `benchmark/scenarios/capture/should_capture/coding_context/reframing.jsonl`
- Create: `benchmark/scenarios/capture/should_capture/coding_context/architecture_pivot.jsonl`
- Create: `benchmark/scenarios/capture/should_capture/coding_context/pattern_establish.jsonl`

**Step 1: Write reframing.jsonl (3 scenarios)**

1. **API vs cache** (EN) — thought it was API timeout, actually cache invalidation race
2. **Frontend vs backend** (EN) — UI lag blamed on React rerenders, actually slow API serialization
3. **Network vs application** (KO) — 네트워크 문제인 줄 알았는데 커넥션 풀 고갈

**Step 2: Write architecture_pivot.jsonl (3 scenarios)**

1. **REST to WebSocket** (EN) — polling approach couldn't meet latency requirement
2. **Monolith extract** (EN) — planned microservice extraction, kept as module instead
3. **ORM to raw SQL** (EN) — ORM couldn't express the query, switched to raw SQL with builder

**Step 3: Write pattern_establish.jsonl (3 scenarios)**

1. **Error boundary pattern** (EN) — discovered crash cascade, established team rule
2. **Idempotency key pattern** (EN) — duplicate payment bug led to team-wide idempotency standard
3. **Config validation pattern** (KO) — 런타임 에러 → 시작 시 config 검증 규칙 확립

**Step 4: Validate all JSONL files**

```bash
for f in benchmark/scenarios/capture/should_capture/coding_context/*.jsonl; do
  echo "=== $f ==="
  python3 -c "
import json, sys
with open('$f') as fh:
    for i, line in enumerate(fh, 1):
        obj = json.loads(line)
        print(f'  OK: {obj[\"id\"]}')
    print(f'Total: {i}')
"
done
```

Expected: 17 total scenarios across 5 files.

---

## Task 6: Write should_not_capture Scenarios — Code Noise

**Files:**
- Create: `benchmark/scenarios/capture/should_not_capture/code_noise/scenarios.jsonl`

**Step 1: Create directory and write scenarios**

```bash
mkdir -p benchmark/scenarios/capture/should_not_capture/code_noise
```

4 scenarios:
1. **Type fix** — `any` → `ApiResponse`, no decision. Notes: "Mechanical type annotation"
2. **Variable rename** — refactored `getData` → `fetchUserProfile`, no architectural decision
3. **Dependency bump** — updated lodash 4.17.20 → 4.17.21, security patch, no decision
4. **Test pass confirmation** — "All 47 tests pass. CI green." No decision content.

Format: same JSONL schema, `expected_capture: false`, `expected_fields: {}`.

**Step 2: Validate JSONL**

Same validation pattern. Expected: 4 scenarios.

**Step 3: Commit all scenarios**

```bash
git add benchmark/scenarios/
git commit -m "feat(bench): add coding_context capture scenarios and code_noise rejection scenarios

17 should_capture scenarios across 5 categories:
- root_cause (4), optimization (4), reframing (3), architecture_pivot (3), pattern_establish (3)

4 should_not_capture code_noise scenarios"
```

---

## Task 7: Update capture-triggers.md (English)

**Files:**
- Modify: `patterns/capture-triggers.md`

**Step 1: Add new section before "## Usage Guidelines"**

Insert after the "## QA & Testing" section (line ~413), before "## Legal & Regulatory":

```markdown
---

## Agentic Coding Context

### Root Cause Discovery
- "The root cause was [X] — here's the fix..."
- "Found it — the issue was in..."
- "After bisecting, commit [hash] introduced..."
- "The stack trace pointed to [X] but the real issue was..."
- "Traced the bug to [X], the fix is..."
- "The regression was introduced when [X] changed..."
- "Memory leak traced to [X] — listeners/handlers never cleaned up"
- "Race condition: [X] and [Y] both writing to [Z] without locking"

### Performance & Optimization
- "Before: [metric], After: [metric] — changed [what]"
- "Profiling showed [X]% of time spent in..."
- "The bottleneck was [X], replaced with [Y]"
- "Query went from [X]ms to [Y]ms after adding index on..."
- "Bundle size reduced from [X] to [Y] by..."
- "O(n²) → O(n) by switching from [X] to [Y]"
- "EXPLAIN showed full table scan, added index on..."

### Problem Reframing
- "I initially thought [X] but it turned out to be [Y]"
- "The real problem isn't [X], it's [Y]"
- "Misdiagnosed as [X] — actually [Y] because..."
- "The error pointed to [X] but the root cause was in [Y]"
- "Spent hours looking at [X], turns out [Y] was the culprit"

### Architecture Pivot During Implementation
- "Planned to use [X] but switched to [Y] because..."
- "The original approach didn't work because..."
- "[X] approach couldn't handle [constraint], pivoted to [Y]"
- "Started with [X] but discovered [limitation], rewrote using [Y]"

### Pattern Establishment (from concrete fix)
- "From now on, always [X] when [Y]"
- "This pattern should be applied across all [components]"
- "New team rule: [X] must always [Y]"
- "Adding this to our checklist: [X]"
- "Established convention: [X] for all [Y]"

### Non-obvious Dependency
- "Changing [X] breaks [Y] because..."
- "Discovered that [A] depends on [B] through..."
- "[X] silently depends on [Y] — not documented anywhere"
- "Side effect: modifying [X] triggers [Y] due to [Z]"
```

**Step 2: Commit**

```bash
git add patterns/capture-triggers.md
git commit -m "feat(triggers): add agentic coding context patterns to capture-triggers.md"
```

---

## Task 8: Update capture-triggers.ko.md (Korean)

**Files:**
- Modify: `patterns/capture-triggers.ko.md`

**Step 1: Add Korean equivalent section**

Insert matching section with Korean trigger phrases:

```markdown
---

## 에이전틱 코딩 컨텍스트

### 근본 원인 발견
- "원인을 찾았습니다 — [X] 때문이었습니다"
- "이 버그의 원인은 [X]이고, 수정은..."
- "git bisect 결과 [커밋]에서 도입된 문제였습니다"
- "스택 트레이스는 [X]를 가리켰지만 실제 원인은 [Y]"
- "메모리 누수 원인: [X]에서 리스너가 정리되지 않음"
- "레이스 컨디션: [X]와 [Y]가 동시에 [Z]에 쓰기"

### 성능 및 최적화
- "변경 전: [수치], 변경 후: [수치] — [변경 내용]"
- "프로파일링 결과 [X]%가 [Y]에서 소비"
- "병목이 [X]에 있었고, [Y]로 교체"
- "쿼리 [X]ms에서 [Y]ms로 개선 — 인덱스 추가"
- "O(n²)에서 O(n)으로 — [X]를 [Y]로 전환"

### 문제 재해석
- "[X] 문제인 줄 알았는데 실제로는 [Y]였다"
- "처음에 [X]라고 생각했지만 알고 보니 [Y]"
- "[X]로 오진했는데 실제 원인은 [Y]"
- "에러가 [X]를 가리켰지만 근본 원인은 [Y]에 있었다"

### 구현 중 아키텍처 전환
- "[X] 방식이 안 돼서 [Y]로 전환했습니다"
- "원래 접근이 [제약] 때문에 불가능해서 [Y]로 변경"
- "[X]로 시작했지만 [한계]를 발견하고 [Y]로 재작성"

### 패턴 확립 (구체적 수정에서 도출)
- "앞으로 [X] 할 때는 항상 [Y] 방식으로"
- "이 패턴을 모든 [컴포넌트]에 적용해야 합니다"
- "새로운 팀 규칙: [X]는 반드시 [Y]"
- "체크리스트에 추가: [X]"

### 비자명 의존성
- "[X]를 수정하면 [Y]가 깨지는데, 이유는..."
- "[A]가 [B]에 의존하고 있었음 — 문서화 안 되어 있었음"
- "부작용: [X] 변경 시 [Z] 때문에 [Y]가 트리거됨"
```

**Step 2: Commit**

```bash
git add patterns/capture-triggers.ko.md
git commit -m "feat(triggers): add agentic coding context patterns to capture-triggers.ko.md"
```

---

## Task 9: Update capture-triggers.ja.md (Japanese)

**Files:**
- Modify: `patterns/capture-triggers.ja.md`

**Step 1: Add Japanese equivalent section**

```markdown
---

## エージェンティックコーディングコンテキスト

### 根本原因の発見
- "原因が判明しました — [X]が原因でした"
- "このバグの原因は[X]で、修正方法は..."
- "git bisectの結果、[コミット]で導入された問題でした"
- "スタックトレースは[X]を指していましたが、実際の原因は[Y]"
- "メモリリークの原因：[X]でリスナーがクリーンアップされていない"
- "レースコンディション：[X]と[Y]が同時に[Z]に書き込み"

### パフォーマンスと最適化
- "変更前：[数値]、変更後：[数値] — [変更内容]"
- "プロファイリングの結果、[X]%が[Y]で消費"
- "ボトルネックは[X]にあり、[Y]に置き換え"
- "クエリが[X]msから[Y]msに改善 — インデックス追加"
- "O(n²)からO(n)へ — [X]を[Y]に切り替え"

### 問題の再解釈
- "[X]の問題だと思っていたが、実際は[Y]だった"
- "最初は[X]だと考えていたが、実は[Y]"
- "[X]と誤診していたが、実際の原因は[Y]"

### 実装中のアーキテクチャ転換
- "[X]のアプローチがうまくいかず、[Y]に切り替えました"
- "元のアプローチが[制約]のため不可能で、[Y]に変更"

### パターンの確立（具体的な修正から導出）
- "今後、[X]する際は必ず[Y]の方法で"
- "このパターンをすべての[コンポーネント]に適用すべき"
- "新しいチームルール：[X]は必ず[Y]"

### 非自明な依存関係
- "[X]を変更すると[Y]が壊れる。理由は..."
- "[A]が[B]に依存していた — ドキュメント化されていなかった"
```

**Step 2: Commit**

```bash
git add patterns/capture-triggers.ja.md
git commit -m "feat(triggers): add agentic coding context patterns to capture-triggers.ja.md"
```

---

## Task 10: Update Claude scribe.md — Step 1 (Policy Evaluation)

**Files:**
- Modify: `agents/claude/scribe.md`

**Step 1: Add agentic coding criteria to CAPTURE list**

After line 43 (the last `- Risk assessments...` bullet), add:

```markdown
- **Agentic coding discoveries** — significant insights from coding, debugging, or optimization sessions:
  - Root cause discovery: bug cause identified with fix approach
  - Performance insight: bottleneck found, optimization applied, before/after impact
  - Problem reframing: initial assumption proved wrong, real cause discovered
  - Architecture pivot: planned approach failed, switched to working alternative
  - Non-obvious dependency: component A unexpectedly affects B
  - Pattern establishment: team rule derived from a concrete fix
```

**Step 2: Add distillation guidance after the DO NOT CAPTURE list**

After line 51 (last DO NOT CAPTURE bullet), add:

```markdown

### Distillation Rule for Code-Heavy Context
When capturing from coding sessions, distill the **knowledge essence** — not raw artifacts:
- WHAT was the insight (1-2 sentences)
- WHY it matters beyond this session (reusable lesson)
- EVIDENCE: minimal code snippet, diff hunk, command output, or metric (up to 50 lines)
Do NOT paste full files, entire diffs, or verbose build logs.
```

**Step 3: Commit**

```bash
git add agents/claude/scribe.md
git commit -m "feat(scribe): add agentic coding context capture criteria to claude scribe.md"
```

---

## Task 11: Update Claude scribe.md — Step 2 (Extraction Formats)

**Files:**
- Modify: `agents/claude/scribe.md`

**Step 1: Add optional fields to Format A (Single Decision)**

After the existing Format A JSON block, add a note:

```markdown
**Optional fields for code-context captures:**
```json
{
  "evidence_type": "code_change | git_bisect | benchmark | error_trace | runtime_observation",
  "evidence_snippet": "Minimal proof: diff hunk, error message, or metric (up to 50 lines)",
  "reusable_insight": "One sentence: the generalizable lesson for the team"
}
```
Include these fields when capturing from coding/debugging/optimization context. Omit for non-code decisions.
```

**Step 2: Add coding context bundle guidance**

After the Format C (Bundle) JSON block, add:

```markdown
### Format C variant: Code-Context Bundle
For code-heavy discoveries, use bundle format with evidence at phase level:
```json
{
  "tier2": {"capture": true, "reason": "...", "domain": "debugging"},
  "group_title": "Short insight title",
  "group_type": "bundle",
  "evidence_type": "code_change",
  "reusable_insight": "One sentence generalizable lesson",
  "status_hint": "accepted",
  "tags": ["debugging", "websocket"],
  "confidence": 0.85,
  "phases": [
    {
      "phase_title": "Core Insight",
      "phase_decision": "What was discovered and decided",
      "phase_rationale": "Why this matters",
      "phase_problem": "What was failing",
      "alternatives": [],
      "trade_offs": [],
      "tags": []
    },
    {
      "phase_title": "Root Cause",
      "phase_decision": "Technical explanation",
      "phase_rationale": "How it was identified",
      "phase_problem": "",
      "evidence_snippet": "```diff\n- old code\n+ new code\n```",
      "alternatives": [],
      "trade_offs": [],
      "tags": []
    },
    {
      "phase_title": "Impact",
      "phase_decision": "Before/after metrics or outcome",
      "phase_rationale": "",
      "phase_problem": "",
      "alternatives": [],
      "trade_offs": [],
      "tags": []
    }
  ]
}
```
Phases may include `evidence_snippet` (up to 50 lines each). Use 2-5 phases.
```

**Step 3: Commit**

```bash
git add agents/claude/scribe.md
git commit -m "feat(scribe): add evidence_snippet and reusable_insight extraction fields"
```

---

## Task 12: Update Gemini scribe.md

**Files:**
- Modify: `agents/gemini/scribe.md`

**Step 1: Apply identical changes from Tasks 10-11**

The gemini scribe.md is structurally identical to claude version (only `source` field differs: `gemini_agent` vs `claude_agent`). Apply the same additions:
- Agentic coding criteria in Step 1
- Distillation Rule
- Optional fields for Format A
- Code-Context Bundle variant for Format C

**Step 2: Commit**

```bash
git add agents/gemini/scribe.md
git commit -m "feat(scribe): add agentic coding context capture to gemini scribe.md"
```

---

## Task 13: Run Benchmark — Validate Improvements

**Files:**
- Read: `benchmark/runners/scribe_bench.py`

**Step 1: Run capture benchmark on new + existing scenarios**

```bash
cd /Users/sunchuljung/repo/cryptolab/Rune/rune && python benchmark/runners/scribe_bench.py --mode capture --report benchmark/reports/post-capture.json
```

**Step 2: Compare results**

Check:
- `coding_context` category: target >= 80% accuracy
- Existing categories (debugging, architecture, pr_review, etc.): no regression
- `code_noise` rejection: target >= 90%

**Step 3: If failures, iterate**

Read the failure details from the report JSON. Common fixes:
- Adjust scenario input text if too ambiguous
- Strengthen scribe prompt guidance for missed patterns
- Refine trigger phrases

**Step 4: Final commit when targets met**

```bash
git add benchmark/reports/
git commit -m "bench: baseline capture results with coding_context scenarios"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Baseline benchmark | (read only) |
| 2 | Schema update | `schema.json` |
| 3-5 | 17 should_capture scenarios | `coding_context/*.jsonl` |
| 6 | 4 should_not_capture scenarios | `code_noise/scenarios.jsonl` |
| 7-9 | Trigger patterns (EN/KO/JA) | `capture-triggers.{md,ko.md,ja.md}` |
| 10-11 | Claude scribe prompt | `agents/claude/scribe.md` |
| 12 | Gemini scribe prompt | `agents/gemini/scribe.md` |
| 13 | Validate + iterate | benchmark run |
