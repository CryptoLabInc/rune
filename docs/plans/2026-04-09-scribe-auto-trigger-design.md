# Scribe 자동 발동 (Proactive Capture)

## 문제

Rune scribe의 모든 부품(에이전트 정의, 패턴 카탈로그, detector, MCP 도구)은 존재하지만, 대화 중 자동으로 발동시키는 배선이 없다. README는 "자동 캡처"를 약속하지만 실제로는 수동 `/rune:capture`만 작동.

## 해결

CLAUDE.md의 Rune 섹션에 자동 감지 행동 규칙 추가. Claude가 대화 중 의사결정을 감지하면 백그라운드 scribe 서브에이전트를 spawn하여 캡처.

## 설계 원칙

- **UX 최우선**: 메인 대화 흐름 끊기지 않음 (백그라운드 spawn)
- **에이전트 위임**: 의사결정 판단을 에이전트에게 맡김 — 100% 감지율 불필요, 에이전트별 개성도 데이터
- **최소 변경**: CLAUDE.md 수정만, 기존 scribe.md/MCP 도구 변경 없음

## 변경 사항

### 변경: `CLAUDE.md`

Rune 섹션에 `### Automatic Capture (Proactive Scribe)` 블록 추가:
- 감지 조건: 선택 채택, 트레이드오프 커밋, 전략 확정, 교훈 도출
- spawn 규칙: `run_in_background: true`, 캡처 대상만 프롬프트에 포함
- state gate: `~/.rune/config.json`이 active일 때만

### 미변경

- `scribe.md` — 이미 완비
- `hooks.json` — Claude Code hook 검증 후 별도
- `capture.md` — 수동 경로 유지

## 멀티에이전트

CLAUDE.md는 Claude Code 전용. Gemini/Codex는 각자 설정 파일에 동일 원칙을 에이전트 스타일로 작성. 의사결정 판단 기준은 에이전트에게 위임.

## 구현 범위

1. CLAUDE.md에 Automatic Capture 섹션 추가
2. 동작 검증 (실제 의사결정 대화에서 scribe spawn 확인)
