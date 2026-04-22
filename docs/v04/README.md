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
└── notes/                         # 내부 작업 노트 (참고용)
    ├── verification-matrix.md      # Python↔Go bit-identical 대조 검증 로그
    └── implementability-report.md  # Go 개발자 진입 가능성 검증 리포트
```

## 읽는 순서

### 🧑‍💼 **처음 보는 사람 / 리뷰어** (overview만 읽어도 충분)

1. 이 README → 전체 요약
2. `overview/architecture.md` → 왜·무엇을·어떻게 (narrative)
3. `overview/decisions.md` → 결정 히스토리·대안 근거
4. `overview/open-questions.md` → 아직 결정 안 된 것들

### 👨‍💻 **Go로 구현할 개발자**

1. `overview/architecture.md` → 맥락 파악 (한 번만)
2. `spec/types.md` → **모든 도메인 타입·enum·I/O schema** (항상 옆에 두기)
3. `spec/python-mapping.md` → Python 파일 → Go 패키지 매핑
4. `spec/flows/capture.md` → 7-phase 상세 흐름 (가장 구체적)
5. `spec/flows/recall.md` → 동일
6. `spec/flows/lifecycle.md` → 나머지 6 tool
7. `spec/components/*.md` → 각 컴포넌트 gRPC 계약·패키지 구조
8. 헷갈리면 `overview/decisions.md` D_N 참조 (spec 문서들이 D_N으로 link)

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
