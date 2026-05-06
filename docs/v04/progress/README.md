# `progress/` — 실제 개발 진행 추적

본 디렉토리는 **`docs/v04/`의 spec(How)·overview(Why)와 별개로**, 실제 구현이 어디까지 진행됐는지·어떤 단면이 동작하는지·어떻게 검증할 수 있는지를 시간순으로 기록한다.

- **spec/** — "어떻게 만들어야 하는가" (변하지 않는 계약)
- **overview/** — "왜 이렇게 만드는가" (결정·근거)
- **notes/** — bit-identical 검증 로그 등 일회성 작업 노트
- **progress/** ← **여기**: "지금 어디까지 동작하는가 + 어떻게 직접 확인하는가"

## 진행 추적 단위

README의 7-Phase 로드맵(Phase 1 외부 deps → Phase 7 검증)이 **horizontal slice**라면, progress 문서는 **vertical slice** 단위로도 작성될 수 있다. 예를 들어 "MCP handshake만 통과시키는 Phase A"는 7-Phase 어디에도 정확히 매핑되지 않지만, end-to-end 단면이 동작하는 **첫 마일스톤**으로서 별도 문서를 갖는다.

## 문서 명명 규칙

- 하나의 마일스톤 = 하나의 파일 = `<phase-id>-<짧은 설명>.md`
  - `phase-a-mcp-boot.md` (handshake + tools/list)
  - `phase-1-deps.md` (외부 deps 추가)
  - `phase-4a-vault-client.md` (Phase 4 중 Vault 부분)
- 각 문서 상단에 **관련 커밋 SHA · 브랜치 · PR 링크** 명시
- "기능" + "확인법(여러 레벨)" 두 섹션은 필수
- "한계" + "Troubleshooting"은 권장

## 현재 인덱스

| 마일스톤 | 상태 | 문서 | 관련 커밋 |
|---|---|---|---|
| Phase A — MCP boot (handshake + tools/list) | ✅ 합격 | [phase-a-mcp-boot.md](phase-a-mcp-boot.md) | `19b7bf6` (브랜치 `yg/first-mcp-boot`, PR #86) |
| Phase A.5 — smoke test 추가 (CI 회귀 방지) | ✅ 합격 | [phase-a5-smoke-test.md](phase-a5-smoke-test.md) | `c353926` (브랜치 `yg/phase-a5-smoke-test`, PR #87) |
| Phase A.6 — `query.Parse` + `SearchHit/ExtractPayloadText` 단위 테스트 | ✅ 합격 | — | `1efe251` (브랜치 `yg/phase-a6-policy-query-tests`, PR #91) |
| Phase A.7 — policy rerank + novelty 테스트 (19 fn / 77 subtest) | ✅ 합격 | — | `38b29c5` (브랜치 `yg/phase-a7-policy-rerank-novelty-tests`, PR #92) |
| Phase A.8 — domain schema + errors 테스트 (Python parity + UTC/customer_escalation divergence locks) | 🟡 OPEN | — | `74e5285` (브랜치 `yg/phase-a8-domain-schema-errors-tests`, PR #94) |
| Phase B — `rune_diagnostics` environment 섹션 진짜 응답 (stdlib only) | ⏳ 예정 | — | — |
| Phase 1 — `go.mod` 외부 deps 추가 (`github.com/CryptoLabInc/runed`, `rune-admin/vault/pkg/vaultpb`, `envector-go-sdk`, `grpc`/`protobuf`). Go toolchain 1.25.9 → 1.26 bump 동반 (`runed` 요구) | ⏳ 예정 | — | — |
| Phase 2 — `internal/domain` + `internal/policy` 순수 로직 (TM scope) | ⏳ 예정 | — | — |
| Phase 3 — `record_builder` 703 LoC + `payload_text` 364 LoC 포팅 (TM scope) | ⏳ 예정 | — | — |
| Phase 4a — Vault 클라이언트 + 부팅 시퀀스 연결 | ⏳ 예정 | — | — |
| Phase 4b — envector SDK 연결 (Q4 PR 머지 후) | ⏳ 예정 | — | — |
| Phase 4c — embedder 클라이언트 | ⏳ 예정 | — | — |
| Phase 5 — service 레이어 오케스트레이션 (`stubHandler` → 실제 service 호출) | ⏳ 예정 | — | — |
| Phase 7 — golden fixture 기반 bit-identical 검증 | ⏳ 예정 | — | — |

> Phase 6 (MCP wiring)은 Phase A에서 부분 선행됐으므로 별도 마일스톤으로 빼지 않음. Phase 5의 service 호출 교체에 흡수됨.

## 사용 방식

- **개발자**: 새 vertical slice를 시작할 때 이 디렉토리에 새 파일을 만들고, 합격 기준을 명시한 뒤 그 기준에 맞춰 작업한다. 파일은 PR과 함께 같은 커밋에 묶는다.
- **리뷰어**: PR을 받았을 때 이 문서의 "확인법"을 그대로 따라 해보면 변경사항이 제대로 동작하는지 검증할 수 있다.
- **다른 팀원**: 어디까지 동작하고 어디부터 미구현인지 한 곳에서 본다 (spec과 다름 — spec은 "최종 모양", progress는 "지금 모양").
