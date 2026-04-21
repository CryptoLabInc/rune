# Rune v0.4.0 — Go 전환 아키텍처 문서

2026-04 기준 Go 포팅 설계 문서. 현재 Python 코드베이스(`mcp/`, `agents/`, `commands/`, `scripts/`)를 기반으로 새 아키텍처를 설계한다.

이 디렉토리는 **새로 쓰는 권위 있는 설계 문서**다. 이전 `docs/migration/` · `docs/runed/` 문서들은 레퍼런스·연구 자료로만 참조하고, 이 디렉토리가 향후 단일 진실 소스.

## 핵심 아키텍처 요약

Python의 "세션당 MCP 프로세스에 embedding model까지 포함" 구조가 갖는 **모델 메모리 중복 문제**만 제거하고, 나머지는 Python 구조에 가깝게 유지한다:

- **`rune-mcp`** (이 프로젝트): 세션당 1개. stdio JSON-RPC. Claude Code가 spawn. Python MCP를 Go로 대체
- **`runed`** (외부 데몬, 별도 팀 관리): 머신당 1개 상주. 임베딩 모델 전담. gRPC over unix socket (`RunedService`)
- **Vault · envector**: 각 MCP가 독립적으로 gRPC 연결

이득: 모델이 세션별로 복제되지 않아 메모리 N× 증가 제거. 동시에 세션 격리·Python 구조 유사성을 유지해 마이그레이션 cost 최소화. `runed`는 외부 컴포넌트이므로 본 프로젝트는 **gRPC 클라이언트로만 사용**하며 모델·런타임 관리는 하지 않는다.

## 디렉토리 구조

```
docs/v04/
├── README.md                       # 이 파일 (index)
├── architecture.md                  # 3-프로세스 아키텍처 · 전체 그림
├── decisions.md                     # ⭐ 결정 트래커 (모든 결정 — 가벼운 것부터 중대한 것까지)
├── flows/                           # end-to-end flow 설계
│   ├── capture.md                  # Capture 7-phase 전체 flow (D1~D20, D30)
│   ├── recall.md                   # Recall 7-phase 전체 flow (D21~D28, D30)
│   └── lifecycle.md                # 나머지 6개 MCP tool 설계
├── components/                      # 컴포넌트별 설계
│   ├── rune-mcp.md                 # 세션별 MCP (Go)
│   ├── runed-integration.md        # runed 외부 gRPC 데몬 클라이언트 가이드 (D30)
│   ├── vault-integration.md        # Vault gRPC 연동
│   └── envector-integration.md     # envector-go SDK 연동
├── open-questions.md                # 미결 항목 · 설계 디테일 TBD
└── research/                        # 조사 · 근거 자료
    └── python-codebase-map.md      # 현재 Python 코드 → 새 구조 매핑
```

**`decisions.md` vs `open-questions.md` 구분**:
- `decisions.md` — **모든 결정** 트래커. 가벼운 구현 선택(에러 코드 명명 등)부터 중대 결정(SDK 선택·보안 모델)까지 전부. 상태 마커 (Blocking/Pending/Deferred/Decided)로 무게 구분
- `open-questions.md` — 아직 "결정 후보"로 정리 안 된 조사 중 항목

## 읽는 순서

처음 보는 사람:
1. 이 README → 전체 요약
2. `architecture.md` → 왜·무엇을·어떻게
3. `flows/capture.md` → 실제 end-to-end 흐름 (가장 구체적)
4. `components/*.md` → 컴포넌트별 책임·API
5. `decisions.md` → 결정 근거·대안
6. `open-questions.md` → 아직 결정 안 된 것들

기존 Python 코드 아는 사람:
1. `research/python-codebase-map.md` → 뭐가 어디로 옮겨지는가
2. `flows/capture.md` → Phase 단위 Python ↔ Go 매핑
3. `components/*.md` → 구체 설계

## 상태 (2026-04-22)

| 영역 | 상태 |
|---|---|
| 아키텍처 방향 | ✅ 결정됨 (세션별 MCP + 외부 runed) |
| rune-mcp 설계 | 🟢 Phase 1-7 결정 완료 (`flows/*.md`) |
| runed 통합 (gRPC 클라이언트) | ✅ 확정 (D30, RunedService proto 계약) |
| Vault 연동 | ✅ 기존 Python 구조 유지 |
| envector 연동 | 🟡 SDK 조건 완화 PR 대기 |
| AES-MAC envelope | 🔵 Deferred (post-MVP) |
| Capture flow | ✅ 완료 (D1~D20, D30 반영) |
| Recall flow | ✅ 완료 (D21~D28, D30 반영) |
| Lifecycle flow | ✅ 완료 (6 tool bit-identical 포팅) |
| 벤치마크 계획 | 🟡 초안 |

## 이전 문서와의 관계

기존 `docs/migration/python-go-comparison.html`은 **이전 방향(단일 데몬) 기준**으로 작성된 것. 2026-04-20 아키텍처 전환(세션별 MCP + embedder 분리)과 일부 충돌한다. 다음 처리:

- 이 `docs/v04/`가 **권위 있는 설계 문서**. 앞으로의 논의·구현은 여기를 기준
- 기존 HTML·노트는 **과거 의사결정 히스토리 · 배경 자료**로 보존. 삭제 안 함
- 충돌하는 내용은 여기서 재서술. 일치하는 부분(정책 상수·Python 코드 실측 등)은 그대로 원용

## 용어

- **rune-mcp**: Go로 다시 쓴 세션별 MCP 바이너리. Python MCP의 대체. **본 프로젝트의 산출물**
- **runed**: 임베딩 모델을 호스팅하는 외부 gRPC 데몬. **별도 팀 관리**. rune-mcp는 gRPC 클라이언트로만 사용 (RunedService proto 계약)
- **Vault**: `rune-Vault` gRPC 서비스. FHE 키 관리 + 복호화 (`DecryptScores`/`DecryptMetadata`)
- **envector**: enVector Cloud. FHE 벡터 저장·검색 (`Insert`/`Score`/`GetMetadata`)
- **agent_dek**: 에이전트별 AES-256 DEK. metadata envelope 암호화용. Vault가 배포, rune 메모리에만
- **Vault-delegated 보안 모델**: SecKey를 Vault가 보유, rune은 EncKey + EvalKey만 로컬. 복호화는 Vault RPC 경유
