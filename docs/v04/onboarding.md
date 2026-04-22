# Onboarding — rune-mcp Go 개발자 진입 가이드

**목표**: 이 문서를 10분 읽으면, 어디를 보고 어디를 고쳐야 할지 판단하고 첫 PR 위치까지 도달.

이 문서는 `docs/v04/`의 모든 세부 문서를 대체하지 않는다. **네비게이션 맵**이다.

---

## 사전 조건

- Go 1.24+ (`go version`로 확인)
- Git + 이 repo 접근 권한
- Python rune 코드베이스에 대한 최소한의 친숙함 (or 의지) — **Python이 canonical source**라 계속 참조

---

## 10분 루트

### 0~3분: 컨텍스트

1. **[docs/v04/README.md](README.md)** — 핵심 아키텍처(3-프로세스 모델 rune-mcp / embedder / Vault+envector) + 현재 상태 (1분)
2. **[docs/v04/overview/architecture.md §Scope](overview/architecture.md#scope-sot--agent-delegated-only)** — 가장 중요. **agent-delegated path only**가 프로젝트 전제다. legacy LLM 경로(detector/tier2_filter/llm_extractor/synthesizer/auto-provider) 전부 **scope 밖** (2분)

### 3~7분: 네비게이션

3. **[internal/README.md](../../internal/README.md)** — 패키지 지도. 어느 Go 파일이 어느 spec / Python 원본에 매핑되는지 (2분)
4. **`go build ./...`** — 로컬에서 컴파일 확인 (stdlib-only 상태, 외부 deps 없이 통과) (1분)
5. **[docs/v04/README.md §구현 로드맵](README.md#구현-로드맵)** — 7 Phase 중 본인이 시작할 Phase 선택 (1분)

### 7~10분: 첫 target 선택

6. 아래 [§첫 PR 추천](#첫-pr-추천-phase별-starter-task)에서 작은 task 하나 집기
7. 해당 파일 열어 TODO 주석 확인 → Python 원본 파일 라인 확인 → 브랜치 생성

이 시점에 **어디를 고치면 되는지 명확**해야 한다. 그렇지 않으면 본 가이드의 § 중 놓친 링크가 있는 것. 뒤로 돌아가서 재확인.

---

## 환경 셋업

### IDE

- **VS Code + Go extension** (권장): `gopls` 자동 설치
- **GoLand**: 상용, 풀 기능
- 공통: `golangci-lint` 설치 권장 (`brew install golangci-lint` 또는 `go install`)

### Python 참조용

Python rune 코드는 같은 repo 안에 있다 (`./mcp/`, `./agents/`). 별도 checkout 불필요.
- IDE에서 Python 파일을 옆 탭에 열어두고 같이 보는 습관
- Go 파일 주석에 `Python: file.py:L<n>` 라인 참조가 있음 → 바로 점프

### 빌드·검사 명령

```bash
go build ./...      # 모든 패키지 빌드. Phase 0 baseline
go vet ./...        # 표준 정적 분석
gofmt -l .          # 포맷 체크 (출력 없으면 OK)
golangci-lint run   # 린터 (설치됐다면)
go test ./...       # 테스트 (Phase 2 이후부터 실효성)
```

---

## 문서 읽기 순서

### 반드시 먼저 (진입 순서)

1. [docs/v04/README.md](README.md) — 아키텍처 · 상태 · 구현 로드맵
2. [docs/v04/overview/architecture.md §Scope](overview/architecture.md#scope-sot--agent-delegated-only) — **agent-delegated only** 전제
3. [internal/README.md](../../internal/README.md) — 패키지 지도

### Phase 시작 전 (해당 Phase만 읽어도 됨)

| Phase | 필수 읽기 |
|---|---|
| **2** (domain/policy 구현) | [spec/types.md](spec/types.md) · [decisions.md](overview/decisions.md) D11 / D21 / D22 / D23 |
| **3** (record_builder + payload_text 포팅) | [spec/flows/capture.md](spec/flows/capture.md) Phase 5 canonical section · [decisions.md](overview/decisions.md) D13 / D14 / D15 · `agents/common/schemas/templates.py` (**canonical**) · `agents/scribe/record_builder.py` |
| **4** (adapter 실 클라이언트) | [spec/components/vault.md](spec/components/vault.md) · [envector.md](spec/components/envector.md) · [embedder.md](spec/components/embedder.md) · [decisions.md](overview/decisions.md) D26 / D30 |
| **5** (service orchestration) | [spec/flows/capture.md](spec/flows/capture.md) · [recall.md](spec/flows/recall.md) · [lifecycle.md](spec/flows/lifecycle.md) |
| **6** (MCP SDK 연결) | [spec/components/rune-mcp.md §MCP 서버 구현](spec/components/rune-mcp.md) · [decisions.md](overview/decisions.md) D2 |

### 필요할 때 참조 (lookup 용도)

- 결정 근거: [overview/decisions.md](overview/decisions.md) — D1-D32. 왜 이렇게 했는지 의심될 때
- 미결 사항: [overview/open-questions.md](overview/open-questions.md) — Q1 (AES-MAC) · Q4 (envector-go SDK PR)
- 검증 로그: [notes/verification-matrix.md](notes/verification-matrix.md) · [python-parity-final.md](notes/python-parity-final.md)
- Python 파일 매핑: [spec/python-mapping.md](spec/python-mapping.md)

---

## 첫 PR 추천 (Phase별 starter task)

**원칙**: 파일 1-2개, 50-150줄 변경, 테스트 포함. 첫 PR로 patterns 익히기.

### Phase 2 — 가장 쉬움

**🟢 추천 A**: `internal/domain/schema.go:ParseDomain` 19-enum map 구현
- 현재 상태: stub (`return DomainGeneral`)
- Python 참조: `agents/scribe/record_builder.py:L621-655 _parse_domain` (+ L646 `customer_escalation → CUSTOMER_SUCCESS` alias)
- 작업량: ~30줄 Go + 20줄 테이블 테스트
- 학습 포인트: Python bit-identical 포팅 패턴 첫 체험

**🟢 추천 B**: `internal/policy/novelty.go:ClassifyNovelty` 구현
- 현재 상태: stub (`return NoveltyClassNovel, 1.0`)
- Python 참조: `agents/common/schemas/embedding.py:L33-56 classify_novelty`
- 작업량: ~20줄 + 테이블 테스트 (4 class boundary)
- 학습 포인트: **score inverted** (1.0 - max_sim) + round(4) 주의

**🟢 추천 C**: `internal/domain/schema.go:GenerateRecordID` 테스트 추가
- 현재 상태: **구현 완료** (테스트 없음)
- 테스트 케이스: unicode (한글) · alphanumeric only · empty title · punctuation mixed
- 작업량: ~40줄 테스트 (Python bit-identical 결과와 비교)
- 학습 포인트: `isPyIsalnum` unicode 동작 확인 — Python 결과 golden으로 비교

**🟢 추천 D**: `internal/domain/errors.go:MakeError` 구현
- 현재 상태: stub (`return nil`)
- Python 참조: `mcp/server/errors.py:L93-118 make_error`
- 작업량: ~30줄 + 테스트
- 학습 포인트: `errors.As` type assertion 패턴

### Phase 3 — regex 포팅 입문

**🟡 추천**: `internal/policy/pii.go:RedactSensitive` 구현
- 현재 상태: stub (truncate만 함)
- Python 참조: `agents/scribe/record_builder.py:L89-95` (5 regex) + `L406-418 _redact_sensitive` + `L227 MAX_INPUT_CHARS=12_000`
- 작업량: ~50줄 Go (5 regex 컴파일 + 치환) + golden fixture 테스트 (redact 전후 텍스트 쌍)
- 학습 포인트: `regexp.MustCompile` + `ReplaceAllString` + unicode 고려

### Phase 4 — adapter 부분 완성

**🟡 추천**: `internal/adapters/vault/endpoint.go:NormalizeEndpoint` 테스트 + 엣지 케이스
- 현재 상태: **부분 구현** (4 형식 basic 처리)
- Python 참조: `mcp/adapter/vault_client.py:L116-140 _derive_grpc_target`
- 작업량: ~40줄 테스트 (env var override · trailing slash · IPv6 brackets 등)
- 학습 포인트: URL parsing edge case + Python 동작 정확 매칭

### 큰 task (Phase 3 본격 진입)

**🔴 heavy**: `internal/policy/payload_text.go:RenderPayloadText` 전체 포팅
- Python 참조: `agents/common/schemas/templates.py` (364 LoC, **canonical**)
- 작업량: ~250줄 Go + 50+ golden fixture
- **주의 사항**: phase_line post-insertion + blank line collapse + `_format_alternatives` bug 유지 (bit-identical) — [decisions.md D15 포팅 주의사항](overview/decisions.md) 참조
- 학습 포인트: 대형 canonical port. 본격 들어가기 전에 팀과 먼저 논의

---

## 코드 컨벤션

### Python bit-identical 원칙

- 포팅 대상 함수 위에 `// Python: <file>.py:L<start>-<end>` 형식 주석 (skeleton에 이미 들어가 있음)
- 상수값은 Python 동일 (novelty `0.3/0.7/0.95` · half-life `90일` · title `60자` · `MAX_INPUT_CHARS=12_000` 등)
- 검증은 `testdata/golden/` 의 JSON/MD 파일로 byte-identical 비교

### TODO 관리

- 스켈레톤의 기존 TODO는 구현 시 **제거**
- 새 TODO 포맷: `// TODO(<phase>): <무엇> — <참조>`
  - 예: `// TODO(Phase 3): implement _extract_evidence — record_builder.py:L498-531`
- 장기 유보 항목은 TODO 대신 decisions.md에 D_N 항목으로

### 테스트

- 순수 함수(domain/, policy/)는 Go 표준 `_test.go` + 테이블 테스트
- Python ↔ Go bit-identical이 필요한 경우 `testdata/golden/` 고정 후 byte-compare
- Python 측 골든 생성 스크립트는 향후 `scripts/gen_golden.py` 추가 예정 (없으면 해당 PR에서 같이 추가)

### 디렉토리 간 경계 (internal/README.md §패키지 의존 방향 참조)

- `internal/domain`은 leaf. 다른 `internal/*` 패키지 import 금지 (stdlib만)
- `internal/policy`는 I/O 없음. `adapters/` 호출 금지
- Adapter error는 service 레이어에서만 `domain.RuneError` 로 wrap

### 커밋 메시지

Conventional Commits 스타일:
- `feat(go): ...` — 새 기능
- `fix(go): ...` — 버그 수정
- `test(go): ...` — 테스트만 추가
- `docs(v04): ...` — 문서 변경
- `refactor(go): ...` — 동작 변경 없이 구조만

본문에 **Python 원본 파일:라인 참조** + **영향 받는 spec 문서** 언급 권장.

### PR 단위

- Phase 단위로 크게 묶지 말고, 하나의 파일 / 하나의 함수 단위로 작게
- 골든 픽스처 추가/변경은 별도 PR
- `go build ./...` · `go test ./...` · `golangci-lint run` 통과 확인 후 PR

---

## FAQ

**Q. Python 코드를 꼭 읽어야 하나?**
예. 모든 포팅은 Python이 canonical. Go 주석에 라인 번호 있으니 점프하기 쉬움. 읽지 않고 구현하면 bit-identical 실패 가능성 큼.

**Q. 결정을 바꾸고 싶다 (예: novelty threshold 조정)**
PR 전에 [decisions.md](overview/decisions.md)에 새 `D<N>` 항목 추가 → 근거 기록 → 리뷰 승인 후 코드 변경. 갑자기 코드에서 상수 바꾸기 금지.

**Q. 외부 의존성을 직접 추가해도 되나?**
Phase 1이 이 역할. 임의 추가 금지. `go.mod require` 변경은 팀 리뷰 필요.

**Q. 테스트가 Python과 float 오차로 실패한다**
`math.Floor` 적용 확인 (예: `searcher.py:L291` `(now-ts).days`는 integer truncation. Go에서는 float 연산 후 Floor 필수). 자세한 bit-identical 주의사항은 [recall.md Phase 6](spec/flows/recall.md) 참조.

**Q. agent-delegated가 아닌 경로를 구현해야 하나?**
아니오. Python에 남아있는 legacy LLM 경로(detector/tier2_filter/llm_extractor/synthesizer/_legacy_standard_capture)는 **Go scope 밖**. 자세한 drop 목록은 [architecture.md §Scope 표](overview/architecture.md#scope-sot--agent-delegated-only).

**Q. 에이전트가 `extracted.tier2`를 안 보내면?**
D14에 따라 `EXTRACTION_MISSING` 에러. agent-delegated 전제이므로 `pre_extraction` 필수. legacy regex fallback은 포팅 안 함.

**Q. envector SDK가 아직 없으면 Phase 4를 어떻게 진행?**
Mock backend + stub 구현으로 Phase 4 starting 가능. Q4 envector-go SDK `OpenKeysFromFile` 조건 완화 PR 머지 전까지는 libevi 통합 불가, mock으로만 테스트. [open-questions.md Q4](overview/open-questions.md) 참조.

**Q. Python과 완전히 같은 동작이 맞는지 어떻게 검증?**
Golden fixture (Python에서 생성한 JSON/MD를 `testdata/golden/`에 고정) + Go 출력과 byte-compare. 대형 포팅(record_builder/payload_text)은 이게 유일한 신뢰 보증 수단.

**Q. 어디서 막히면?**
1. Python 원본 파일 + 해당 spec 문서를 옆에 놓고 다시 읽기
2. [open-questions.md](overview/open-questions.md) 확인 — 이미 논의된 미결 사항일 수도
3. [decisions.md D1-D32](overview/decisions.md) 관련 결정 확인
4. 그래도 안 풀리면 team channel 질문

---

## 다음 단계

- Phase 선택 후 이 가이드의 §첫 PR 추천 에서 starter 골라서 브랜치 생성
- 첫 PR 리뷰 후 배운 점 팀 Slack에 공유 (다음 사람 도움)
- 전체 로드맵은 [docs/v04/README.md §구현 로드맵](README.md#구현-로드맵) 에서 추적

환영합니다. 🚀
