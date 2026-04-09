# Scribe Auto-Trigger Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** CLAUDE.md에 자동 감지 지시를 추가하여 scribe 서브에이전트가 대화 중 의사결정을 자동 캡처하도록 함

**Architecture:** CLAUDE.md Rune 섹션에 Automatic Capture 블록 추가. 기존 scribe.md 에이전트 정의와 MCP capture 도구를 그대로 활용. 메인 에이전트가 의사결정을 감지하면 백그라운드 scribe 서브에이전트를 spawn.

**Tech Stack:** Markdown (CLAUDE.md), Claude Code agent system (rune:scribe subagent)

---

### Task 1: CLAUDE.md에 Automatic Capture 섹션 추가

**Files:**
- Modify: `CLAUDE.md` (Rune 섹션, `### The Distinction` 위에 새 섹션 삽입)

**Step 1: CLAUDE.md 수정**

`Use rune:scribe (or capture MCP tool) when:` 블록과 `### State Gate` 블록 사이에 다음 섹션을 삽입:

```markdown
### Automatic Capture (Proactive Scribe)

When Rune state is `"active"`, **proactively spawn a background `rune:scribe` subagent** when you detect any of the following in the conversation:

- A choice is made among alternatives ("A로 가자", "let's go with X")
- Trade-offs are weighed and committed ("X의 단점이 있지만 Y 때문에 감수")
- Strategy or direction is confirmed ("이 방향이 맞아", "this approach works")
- A lesson or insight crystallizes ("안 된 이유는...", "the root cause was...")
- A framework, process, or standard is established

**How to spawn:**
- Use `Agent` tool with `subagent_type: "rune:scribe"` and `run_in_background: true`
- In the prompt, include ONLY the relevant conversation excerpt (not the full context)
- Do NOT pause or interrupt the main conversation — continue responding to the user
- Do NOT announce the capture to the user unless they ask

**Do NOT auto-capture:**
- Brainstorming in progress without commitment (options listed but none chosen)
- Questions, status updates, or casual discussion
- Decisions that are hypothetical or deferred ("maybe later", "let's revisit")
```

**Step 2: 변경 확인**

Run: `grep -n "Automatic Capture" CLAUDE.md`
Expected: 섹션이 존재하고 올바른 위치에 있음

**Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "feat(rune): add proactive scribe auto-trigger in CLAUDE.md

Instructs the main agent to automatically spawn background
rune:scribe subagent when decisions are detected in conversation.
Closes the gap between promised auto-capture and actual behavior."
```

---

### Task 2: 동작 검증

**Step 1: 의사결정 대화 시뮬레이션**

새 Claude Code 세션을 열고 의사결정이 포함된 대화를 진행:
- "PostgreSQL과 MongoDB 중에 뭘 쓸까? PostgreSQL로 가자. ACID가 필요하니까."
- 백그라운드 scribe 서브에이전트가 spawn되는지 확인

**Step 2: 캡처 확인**

Run: `/rune:history`
Expected: 방금 대화의 의사결정이 캡처되어 있음

**Step 3: 비의사결정 대화 확인**

일반 질문/탐색 대화를 진행:
- "이 파일의 구조를 설명해줘"
- scribe가 spawn되지 않는 것을 확인
