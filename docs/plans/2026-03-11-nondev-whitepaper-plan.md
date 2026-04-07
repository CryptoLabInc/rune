# Non-Developer Whitepaper & Slides Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rune 백서 및 슬라이드의 비개발자 버전 (한글). 스타트업 전략 기획자 관점, 가격 전략 시나리오, 3막 구조.

**Architecture:** 3-Act structure (문제 → 해결 → 원리). 기존 개발자 백서의 동일한 기술적 사실을 비유 중심으로 재구성. drawio 다이어그램 3종, reveal.js 슬라이드.

**Tech Stack:** Markdown (백서), drawio XML (다이어그램), reveal.js HTML (슬라이드)

---

### Task 1: 백서 — 1막 (문제)

**Files:**
- Create: `tmp/whitepaper-rune-nondev-ko.md`

**Reference:**
- `tmp/whitepaper-rune-ko.md` (개발자 백서 — Section 1, 2의 논리 구조 참조)
- `docs/plans/2026-03-11-nondev-whitepaper-design.md` (설계 문서)

**Step 1: 1막 작성**

`tmp/whitepaper-rune-nondev-ko.md` 생성. 제목, 부제, 1막 전체 포함:

- **1.1 도입 시나리오**: 전략 기획자가 AI 에이전트와 가격 전략 수립. 30분 세션의 구체적 추론 과정 (경쟁사 A 월 29,000원, 타겟 프리랜서/소규모팀, 가격 감수성, 침투 가격 9,900원 결정). 세션 후 슬랙 한 줄만 남음.
- **1.2 컨텍스트 증발**: 결정 vs 추론 구분. 규모의 산수 (8명 x 10건/일 = 분기 5,200건). "왜 9,900원?" 상황.
- **1.3 기존 접근 한계**: 회의록/노션 (사후 작성, AI Slop), 개인 에이전트 메모리 (팀 공유 불가), 사내 검색 (추론 ≠ 문서). 비교 매트릭스.

톤: 읽기 쉬운 에세이. 과도한 확신 없이. 고등학생 이해 가능 수준.

**Step 2: 검토 후 커밋**

```bash
git add tmp/whitepaper-rune-nondev-ko.md
git commit -m "docs: add non-developer whitepaper act 1 (problem)"
```

---

### Task 2: 백서 — 2막 (해결)

**Files:**
- Modify: `tmp/whitepaper-rune-nondev-ko.md`

**Reference:**
- `tmp/whitepaper-rune-ko.md` (개발자 백서 — Section 3의 캡처/리콜 시나리오 참조)
- `agents/claude/scribe.md` (Scribe 에이전트 스펙)
- `patterns/capture-triggers.md` (200+ 캡처 트리거 패턴)

**Step 1: 2막 작성**

백서에 2막 추가:

- **2.1 두 연산**: 캡처/리콜을 한 문장으로 설명. 별도 명령 불필요, 에이전트가 자동 수행.
- **2.2 시나리오 재방문**: 가격 전략의 캡처 장면 (Scribe가 결정/근거/기각 대안 구조화). 리콜 장면은 **Before/After 대비**로 구성: (A) Rune 없는 에이전트 — 같은 질문에 교과서적 일반론 ("SaaS에서는 보통 20-30% 할인이..."), (B) Rune 있는 에이전트 — 팀의 구체적 맥락이 주입된 답변 ("2주 전 전략팀이 침투 가격을 채택했고, 프리미엄은 기각..."). 비유: "첫날 출근한 천재 컨설턴트" vs "모든 회의에 참석했던 팀원". 에이전트를 써보지 않은 독자도 조직 기억 주입의 의미를 체감할 수 있게.
- **2.3 에이전트 비종속**: MCP를 USB 포트 비유로. Claude/ChatGPT/Gemini 어디서든 같은 기억. 도구보다 오래 가는 조직 기억.

**Step 2: 커밋**

```bash
git add tmp/whitepaper-rune-nondev-ko.md
git commit -m "docs: add non-developer whitepaper act 2 (solution)"
```

---

### Task 3: 백서 — 3막 (원리) + 마무리

**Files:**
- Modify: `tmp/whitepaper-rune-nondev-ko.md`

**Reference:**
- `tmp/whitepaper-rune-ko.md` (개발자 백서 — Section 3 아키텍처, Section 4 신뢰 모델)
- `mcp/server/server.py` (MCP 서버 캡처/리콜 구현)
- `mcp/adapter/vault_client.py` (Vault 클라이언트)

**Step 1: 3막 작성**

백서에 3막 추가:

- **3.1 동형암호(FHE)**: 투표함 비유. 일반 암호화와의 차이 (꺼내서 열기 vs 잠긴 채로 비교). 고등학생 수준.
- **3.2 세 구성 요소**: 비서/잠긴 창고/금고지기 비유. 역할/아는 것/모르는 것 테이블.
- **3.3 데이터 이동 경로**: 캡처/리콜을 비유 기반 단계별 서술. 핵심: 평문은 로컬을 떠나지 않음.
- **3.4 신뢰 모델**: 4가지 위협 (창고 털림/배달 가로챔/비서 해킹/금고지기 털림). Vault 침해는 솔직하게 치명적 위험.
- **3.5 구현 과제**: 시간순 검색 불가, 다단계 결정 자동 결합 불가. 전향적 프레이밍.
- **마무리**: 요약 + 링크.

**Step 2: 커밋**

```bash
git add tmp/whitepaper-rune-nondev-ko.md
git commit -m "docs: add non-developer whitepaper act 3 (how it works) and conclusion"
```

---

### Task 4: drawio 다이어그램 3종

**Files:**
- Create: `tmp/diagrams/rune-nondev-capture-recall.drawio`
- Create: `tmp/diagrams/rune-nondev-components.drawio`
- Create: `tmp/diagrams/rune-nondev-trust-model.drawio`

**Step 1: 캡처/리콜 플로우 다이어그램**

`rune-nondev-capture-recall.drawio` — 비서/창고/금고지기 비유 기반.
- 캡처 흐름: 대화 → 비서(감지, 구조화) → 암호화(금고에 넣기) → 잠긴 창고에 보관
- 리콜 흐름: 질문 → 암호화된 검색 → 창고(잠긴 채로 비교) → 금고지기(점수만 해독) → 비서(해석)

**Step 2: 구성 요소 다이어그램**

`rune-nondev-components.drawio` — 세 구성 요소의 위치와 관계.
- 당신의 컴퓨터: AI 에이전트 + 비서(MCP 서버)
- 클라우드: 잠긴 창고 (enVector Cloud)
- 팀 인프라: 금고지기 (Rune-Vault)
- 각 구성 요소가 보유/미보유 정보 표시

**Step 3: 신뢰 모델 다이어그램**

`rune-nondev-trust-model.drawio` — 4가지 위협 시나리오 시각화.
- 각 경계에서 데이터 형태 (평문/암호문/점수만)
- 위협 4 (금고지기 침해)를 빨간색으로 강조

**Step 4: 커밋**

```bash
git add tmp/diagrams/
git commit -m "docs: add drawio diagrams for non-developer whitepaper"
```

---

### Task 5: 슬라이드 (reveal.js HTML)

**Files:**
- Create: `tmp/slides-rune-nondev-ko.html`

**Reference:**
- `tmp/slides-rune-ko.html` (개발자 슬라이드 — reveal.js 구조, CSS 스타일 참조)
- `tmp/whitepaper-rune-nondev-ko.md` (비개발자 백서 — 콘텐츠 소스)

**Step 1: 슬라이드 작성**

~20 slides, 3막 구조:

1. 타이틀
2. "30분의 추론, 1줄의 결과" (도입 시나리오)
3. 세션 후 남는 것 (슬랙 한 줄)
4. 컨텍스트 증발 — 규모의 산수
5. 기존 접근 1: 회의록/노션/AI Slop
6. 기존 접근 2: 개인 메모리, 사내 검색
7. 비교 매트릭스
8. — 2막 전환 —
9. Rune: 캡처와 리콜
10. 캡처 장면 (가격 전략)
11. 무엇이 포착되는가 (구조화된 필드)
12. 리콜 장면 (2주 후 마케팅)
13. 에이전트 비종속 (USB 포트 비유)
14. — 3막 전환 —
15. 동형암호(FHE) — 투표함 비유
16. 일반 암호화 vs FHE
17. 세 구성 요소 (비서/창고/금고지기)
18. 데이터 이동 경로
19. 신뢰 모델 — 4가지 시나리오
20. 구현 과제
21. 마무리 + 링크

Self-serving: 발표자 없이 읽을 수 있도록 각 슬라이드에 충분한 텍스트.
코드 블록 없음. 비유와 테이블 중심. drawio 다이어그램은 참조 설명으로 대체 (drawio를 SVG로 내보내기 전까지).

**Step 2: 커밋**

```bash
git add tmp/slides-rune-nondev-ko.html
git commit -m "docs: add non-developer slides (Korean, reveal.js)"
```
