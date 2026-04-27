# Rune v0.4.0 — Go 전환 설계 문서

2026-04 기준 Python MCP → Go MCP 포팅 설계 문서. Python 코드베이스(`mcp/`, `agents/`)를 기반으로 Go 구현을 위한 계약·결정·흐름을 정리한다.

**이 디렉토리가 단일 진실 소스**. 이전 `docs/migration/` · `docs/runed/` 문서는 히스토리·배경 자료로만 참조.

## 핵심 아키텍처

Python의 "세션당 MCP 프로세스에 embedding model까지 포함" 구조가 갖는 **모델 메모리 중복 문제**만 제거하고 나머지는 Python 구조에 가깝게 유지:

- **`rune-mcp`** (이 프로젝트): 세션당 1개. stdio JSON-RPC. Python MCP를 Go로 포팅 (임베딩 제외)
- **`embedder`** (별도 프로세스, 가칭): 머신당 1개 상주. 임베딩 모델 전담. gRPC over unix socket
- **Vault · envector**: 각 MCP가 독립적으로 gRPC 연결

`embedder`는 외부 컴포넌트. rune-mcp는 **gRPC 클라이언트로만** 사용.

> **네이밍 히스토리**: 초기 설계에서 "runed" (Python MCP 대체 + 임베딩 내장 통합 데몬)을 구상했으나 폐기. 현재는 **rune-mcp (Go 포팅) + embedder (임베딩 별도)** 구조.

## 디렉토리 구조

```
docs/v04/
├── README.md                      # 이 파일 (index · role별 reading order)
│
├── overview/                      # 사람용 — Why & What
│   ├── architecture.md             # 3-프로세스 구조 · 메모리 모델 · 원칙 · 상태머신
│   ├── decisions.md                # D1-D30 결정 트래커 (배경·선택지·근거·재평가)
│   └── open-questions.md           # Q1-Q9 미결 사항
│
├── spec/                          # 개발자용 — How (구현 계약)
│   ├── types.md                    # 🔑 DecisionRecord v2.1 · 8 enum · I/O schemas (단일 진실 소스)
│   ├── flows/                      # Phase 단위 end-to-end 로직 + Go pseudocode
│   │   ├── capture.md               # 7-phase capture
│   │   ├── recall.md                # 7-phase recall
│   │   └── lifecycle.md             # 6 tool (vault_status · diagnostics · batch_capture · …)
│   ├── components/                 # 컴포넌트 계약 · 패키지 구조 · gRPC client 구현
│   │   ├── rune-mcp.md              # 세션별 MCP 바이너리 (메인)
│   │   ├── embedder.md              # 외부 embedder gRPC 클라이언트
│   │   ├── vault.md                 # Vault gRPC 클라이언트
│   │   └── envector.md              # envector-go SDK
│   └── python-mapping.md           # Python 파일/LoC → Go 구조 매핑
│
├── notes/                         # 내부 작업 노트 (참고용)
│   ├── verification-matrix.md      # Python↔Go bit-identical 대조 검증 로그
│   ├── implementability-report.md  # Go 개발자 진입 가능성 검증 리포트
│   └── flow-matrix.md               # 10 flow × 파일 매트릭스 + Tier S/A/B 공통 모듈
│
└── progress/                       # 실제 개발 진행 추적 (vertical slice 단위)
    ├── README.md                    # 인덱스 + 마일스톤별 상태표
    └── phase-a-mcp-boot.md          # MCP handshake + tools/list (`19b7bf6`)
```

## 읽는 순서

### 🧑‍💼 **처음 보는 사람 / 리뷰어** (overview만 읽어도 충분)

1. 이 README → 전체 요약
2. `overview/architecture.md` → 왜·무엇을·어떻게 (narrative)
3. `overview/decisions.md` → 결정 히스토리·대안 근거
4. `overview/open-questions.md` → 아직 결정 안 된 것들

### 👨‍💻 **Go로 구현할 개발자**

**새 개발자라면 먼저 [`onboarding.md`](onboarding.md)** — 10분 안에 첫 PR 위치까지 안내 (환경 셋업 · 읽기 순서 · Phase별 starter task · 컨벤션 · FAQ).

그 다음 (필요 시점에):

1. `overview/architecture.md` → 맥락 + **§Scope** (agent-delegated only 전제가 프로젝트 전반을 좌우)
2. [`../../internal/README.md`](../../internal/README.md) → 패키지 지도 (Go 파일 ↔ spec ↔ Python 원본 매핑표)
3. `spec/types.md` → 모든 도메인 타입·enum·I/O schema (항상 옆에 두기)
4. `spec/python-mapping.md` → Python 파일 → Go 패키지 LoC 매핑
5. `spec/flows/{capture,recall,lifecycle}.md` → 담당 Phase에 해당하는 7-phase 흐름
6. `spec/components/*.md` → 각 컴포넌트 gRPC 계약·패키지 구조
7. 헷갈리면 `overview/decisions.md` D_N 참조 (spec 문서들이 D_N으로 link)

### 🔍 **검증 상태 확인하고 싶은 사람**

- `notes/verification-matrix.md` — 28 상수 · Phase 순서 · 함수 라인 전수 대조 결과
- `notes/implementability-report.md` — "docs만으로 Go 구현 가능한가" 검증

## overview vs spec vs notes

| 유형 | 누가 읽나 | 성격 |
|---|---|---|
| **overview/** | PM, 아키텍트, 신규 참여자 | narrative prose, "왜"에 집중, 결정 히스토리 |
| **spec/** | Go 개발자 | 구체 인터페이스·pseudocode·상수·에러 매핑, "어떻게"에 집중 |
| **notes/** | 작성자·검증자 | 작업 중 발견·검증 로그, 참고용 |

**`overview/decisions.md` vs `overview/open-questions.md` 구분**:
- `decisions.md` — 모든 결정 트래커 (가벼운 구현 선택부터 중대 결정까지). 상태 마커 (Blocking/Pending/Deferred/Decided/Archived)로 무게 구분
- `open-questions.md` — 아직 "결정 후보"로 정리 안 된 조사 중 항목

## 상태 (2026-04-22)

| 영역 | 상태 |
|---|---|
| 아키텍처 방향 | ✅ 결정됨 (세션별 rune-mcp + 외부 embedder) |
| rune-mcp 설계 (7-phase) | 🟢 완료 (`spec/flows/*.md`) |
| embedder gRPC 통합 | ✅ 확정 (D30) |
| Vault 연동 | ✅ 기존 Python 구조 유지 |
| envector 연동 | 🟡 SDK 조건 완화 PR 대기 (Q4) |
| AES-MAC envelope | 🔵 Deferred Post-MVP (Q1) |
| Python↔Go 대조 검증 | ✅ 완료 (`notes/verification-matrix.md`) |
| Go 구현 진입 가능성 | 🟢 Ready (`notes/implementability-report.md`, P0 blocker 0건) |
| Go skeleton | ✅ 완료 (`2eb167d`, 37 파일, stdlib-only compile) |
| 개발자 onboarding 가이드 | ✅ 완료 (`onboarding.md` · `internal/README.md`) |

## 구현 로드맵

Skeleton(`2eb167d`) 이후 단계. 각 Phase는 **별도 PR** 단위이며, 이전 Phase가 완료되기 전에 다음 Phase로 진입하지 않는다 (하위 → 상위 build-order 존중).

| Phase | 범위 | 비고 |
|---|---|---|
| 1 | 외부 의존성 추가 | `modelcontextprotocol/go-sdk` v1.5+, `google.golang.org/grpc`, `google.golang.org/protobuf`, envector-go SDK(Q4 PR 머지 후 연결), embedder proto stub import |
| 2 | `internal/domain/` + `internal/policy/` 순수 로직 | `ParseDomain` 19-enum map + `customer_escalation` alias · 상수·regex 이식 (81 stopwords · 31 intent · 16 time · 4 tech) · `GenerateRecordID` unicode slug · `ClassifyNovelty` · `ApplyRecencyWeighting` · `FilterByTime` · golden fixture 테스트 harness |
| 3 | `record_builder` (703 LoC) + `payload_text` (364 LoC) 라인 단위 포팅 | D13 Option A · D15 canonical. 5 SENSITIVE + 4 QUOTE + 5 RATIONALE regex, `_parse_domain` 19-map + alias, `ensure_evidence_certainty_consistency` → `render_payload_text` 순서. `testdata/` 50+ 샘플로 byte-identical 검증 (Python 원본이 canonical source) |
| 4 | `internal/adapters/` 실제 클라이언트 | Vault gRPC (3 RPC + keepalive + 256MB msg + TLS + health 2-tier + endpoint 4-form 정규화) · envector SDK + AES envelope Seal/Open (AES-256-CTR, 16B IV) · embedder gRPC + Info `sync.Once` 캐시 + D7 retry · capture_log `flock` append + 역순 `Tail` |
| 5 | `internal/service/` orchestration | 7-phase capture (novelty non-fatal, D17 atomicity probe) · 7-phase recall (D25 순차, D26 Vault 위임 decrypt, D27 phase_chain expansion) · 6 lifecycle tools · Vault/envector 실패 시 `state=dormant` 전환 side-effect |
| 6 | `internal/mcp/` SDK 연결 | 공식 Go SDK로 8 tool 등록 · stdio transport · `Deps` 주입 · adapter error → `domain.RuneError` → MCP response wrap · state-specific recovery hints |
| 7 | 검증 | policy unit (golden fixture byte-identical) · bufconn Vault integration · libevi/mock backend envector contract tests · `synctest` (Go 1.25) boot retry 결정적 테스트 · Python ↔ Go shadow run (cutover 리스크 완화) |

**우선순위 결정 팁**:
- Phase 2·3 완료 전에는 외부 의존성(Phase 1) 없이도 순수 로직 테스트 가능 → 빠른 confidence 구축
- Phase 4는 envector-go SDK `OpenKeysFromFile` 조건 완화 PR(Q4) 머지에 블로킹 가능 — mock backend로 우회 개발
- Phase 5·6는 adapter interface 확정 후 진입. 미리 시작하면 signature 변경 비용 큼
- Phase 7 shadow run은 Post-MVP에도 유지 — 장기 품질 보증 수단

## 이전 문서와의 관계

- 기존 `docs/migration/python-go-comparison.html` — 이전 방향(단일 데몬) 기준. 일부 충돌
- 기존 `docs/runed/` — 폐기된 "runed" 통합 데몬 설계. 히스토리 보존
- 이 `docs/v04/`가 권위 있는 설계 문서

## 용어

- **rune-mcp**: Python MCP를 Go로 포팅한 세션별 바이너리 (임베딩 제외). **본 프로젝트 산출물**
- **embedder**: 임베딩 모델 호스팅 외부 gRPC 프로세스 (가칭). rune-mcp는 gRPC 클라이언트로만 사용
- **runed**: ❌ 폐기된 이름. 초기 "Python MCP 대체 + 임베딩 내장" 통합 데몬 의미
- **Vault**: `rune-Vault` gRPC 서비스. FHE 키 관리 + 복호화 (`GetPublicKey`/`DecryptScores`/`DecryptMetadata`)
- **envector**: enVector Cloud. FHE 벡터 저장·검색 (`Insert`/`Score`/`GetMetadata`)
- **agent_dek**: 에이전트별 AES-256 DEK. metadata envelope 암호화용. Vault가 배포, rune-mcp 메모리에만
- **Vault-delegated 보안 모델**: SecKey는 Vault만 보유. rune-mcp는 EncKey + EvalKey만 로컬. 복호화는 Vault RPC 경유
- **agent-delegated**: 에이전트(Claude Code 등)가 extraction·판정을 수행하고 rune-mcp는 저장·검색 파이프라인만 담당 (D14/D21/D28)
