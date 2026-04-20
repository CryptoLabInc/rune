# runed 개발 레퍼런스

> **검증 상태 (2026-04-17)**: 전체 Python 코드베이스(~11,500 LoC) 직접 읽기 완료.
> 본 세트의 8개 문서가 모두 실제 Python 소스와 대조되어 교정됨. 주요 업데이트는
> 각 문서 상단 스탬프 참고. 새롭게 확정/명시된 사실은 아래 "실제 코드와 다르게
> 알려졌던 것" 표 참고.

이 디렉토리는 Go `runed` 데몬 구현을 위한 개발자 참고 문서 세트다.
직접 구현할 때 옆에 놓고 보는 용도.

## 문서 목록

| # | 파일 | 내용 |
|---|---|---|
| 01 | [01-architecture-overview.md](01-architecture-overview.md) | 현재(Python) → 새(Go) 아키텍처 전환 큰 그림. 설치 변화, 메모리 비교, 멀티세션, 전체 시스템 다이어그램 |
| 02 | [02-installation-and-lifecycle.md](02-installation-and-lifecycle.md) | 플러그인 설치, Go 바이너리 배포, launchd/systemd 등록, startup/shutdown 시퀀스, config fsnotify, sleep/wake 복구, 업그레이드 |
| 03 | [03-external-communication.md](03-external-communication.md) | Vault gRPC + enVector Go SDK 통신 전체. proto 정의, TLS 3모드, 키 번들, Score/GetMetadata/Insert 연산, AES-256-CTR 암호화, 연결 복구, 시퀀스 다이어그램 |
| 04 | [04-envector-go-sdk-contract.md](04-envector-go-sdk-contract.md) | enVector Go SDK (팀원이 별도 개발)와 runed 사이 인터페이스 계약. Score/GetMetadata/Insert 경계, agent_id/agent_dek 미전달(runed가 AES 담당), KeyParameter 몽키패치 불필요, CipherBlock 호환성, CGO 경계, 팀 논의 필요 항목 |
| 05 | [05-capture-flow.md](05-capture-flow.md) | Capture 데이터 플로우 end-to-end. embed → AES encrypt → SDK Insert → log. 각 단계 입출력 타입, 에러 경로, batch, Go 코드 구조 |
| 06 | [06-recall-flow.md](06-recall-flow.md) | Recall 데이터 플로우 end-to-end. 쿼리 분석 → 멀티쿼리 FHE round-trip → 필터 → 재랭킹. 실제 코드 기준 공식, 성능 특성, 병렬화 |
| 07 | [07-mcp-cli-layer.md](07-mcp-cli-layer.md) | 데몬 위의 얇은 껍질: CLI + MCP shim 설계. 서브커맨드 매핑, on-demand 기동, 에러 처리, JSON escape, plugin.json |

## 설계 전제

- **enVector Go SDK는 팀원이 별도 개발**. runed는 이 SDK를 의존성으로 소비.
  SDK 인터페이스 계약은 04-envector-go-sdk-contract.md 전체와 03-external-communication.md §3.2에 정의.
- **Vault gRPC 프로토콜은 변경 없음**. 기존 proto를 Go stub으로 재생성.
  proto 정의 전문은 03-external-communication.md §2.1에.
- **MCP + CLI 양립** 구조. runed HTTP API를 공유하므로 둘 다 지원 가능하되,
  MVP는 CLI 우선. 07-mcp-cli-layer.md에 상세.
- **AES 모드는 AES-256-CTR** (`pyenvector/utils/aes.py:52-58` 소스 확인 결과).
  와이어 포맷: `IV(16B) || ciphertext → base64`. 봉투: `{"a":agent_id,"c":base64(IV||CT)}`.
  docstring의 "AES-GCM" 언급은 오래된 주석. 05와 03에 상세.
- **재랭킹 공식은 가중 합**: `(0.7 × rawScore + 0.3 × decay) × statusMul`.
  06-recall-flow.md가 실제 Python 코드(`searcher.py:297`)에서 추출한 정확한 공식.
- **임베딩 차원은 1024** (Qwen/Qwen3-Embedding-0.6B). `embedding.py:14-18` 주석에서
  "Calibrated for Qwen3-Embedding-0.6B (1024dim) via benchmark 2026-04-08" 확정.
  Phase 0 블로킹 #2 이미 해결.

## 실제 코드와 다르게 알려졌던 것 (수정됨)

| 항목 | 이전 분석 | 실제 코드 | 수정된 문서 |
|---|---|---|---|
| AES 모드 | GCM (추측) | **CTR** (pyenvector 소스 확인) | 05, 03 |
| superseded 승수 | 0.6 | **0.5** | 06 |
| 재랭킹 공식 | rawScore × recencyMul × statusMul | **(0.7×raw + 0.3×decay) × statusMul** | 06 |
| 임베딩 차원 | 384 (MiniLM default 오인) | **1024** (Qwen3-0.6B, benchmark 2026-04-08 확정) | 05 |
| dormant 시 tool 등록 | "최소 3개만" 추정 | **8개 모두 무조건 등록**, runtime state 체크로 거절 | 07 |
| keys/ 디렉토리 구조 | 평면 (`keys/EncKey.json`) | **`keys/<key_id>/EncKey.json`** (server.py:258 실측) | 02 |
| legacy HTTP endpoint 파싱 | "DROP 판정" | DROP 판정이지만 `vault_client.py:70,138-140`에 여전히 live | 03 |
| clientInfo auto-provider | "DROP 판정" | DROP 판정이지만 `server.py:477-489`에서 여전히 호출됨 | (migration 03) |
| Novelty 임계값 | 단일 값 가정 | **이중 상수**: `embedding.py:16-18` 0.4/0.7/0.93 vs `server.py:100-108` default 0.3/0.7/0.95 (런타임은 server.py 기본값 사용) | 05 |

**2026-04-17 full code re-verification**: 전체 Python 코드베이스(~11,500 LoC) 직접 읽기
완료. 위 표의 7개 항목은 모두 소스 실측으로 확정됨.
