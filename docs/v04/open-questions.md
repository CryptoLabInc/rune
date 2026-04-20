# 미결 항목 · 블로킹

Rune v0.4.0 작업 중 아직 결정 안 된 것들과 정리된 블로킹 목록. 각 항목은 "왜 결정이 필요한가 · 검토 중인 선택지 · 다음 액션"으로 정리.

결정이 내려지면 해당 항목을 이 문서에서 제거하고, 대신 관련 `components/*.md` 또는 `architecture.md`에 반영.

---

## 🔴 블로킹 (Phase 0 완료 전 필수)

### Q1. AES envelope에 MAC 필드 추가 여부

**배경**: 현행 AES-256-CTR envelope `{"a":agent_id, "c":base64(IV||CT)}`는 인증 태그가 없다. 암호문 바이트를 flip해도 복호화 측이 감지하지 못해 **malleability 공격 취약**. 팀 메모리 품질 보호 관점에서 장기적으로 위험.

**제약**: pyenvector(Python)과 envector-go(Go) 양쪽 클라이언트가 동시 지원해야 호환. Vault는 FHE 경로라 무관.

**검토 중인 선택지**:
- **(a)** envelope에 `"m"` 필드 추가 — `HMAC-SHA256(dek, a||iv||ct)[:16]`. 기존 레코드는 `m` 없으면 verify skip + 4주 grace period
- **(b)** AES-GCM으로 마이그레이션 — 장기. 포맷 자체 변경. legacy 재암호화 경로 필요
- **(c)** 현상 유지 — 비권장

**잠정 방향**: (a) Phase 1 초반 채택. pyenvector와 envector-go 양쪽 동시 릴리스 필요.

**다음 액션**: pyenvector 팀과 릴리스 타이밍 조율 + envector-go SDK에 PR (§Q4와 함께).

---

### Q2. rune-embedder 실행 엔진 (ONNX vs llama-server)

**배경**: rune-embedder를 별도 데몬으로 분리하기로 결정한 후, 내부 실행 엔진을 Go 생태계(ONNX Runtime Go)로 할지 기성품(llama-server)으로 할지 결정해야 한다.

**검토 중인 선택지**:

| | **ONNX Runtime Go** | **llama-server** |
|---|---|---|
| 실행 엔진 | `github.com/yalue/onnxruntime_go` CGO | llama.cpp의 `llama-server` 바이너리 (C++) |
| 모델 포맷 | `.onnx` (우리가 PyTorch에서 wrapper re-export) | `.gguf` (Qwen3 공식 GGUF 이미 존재) |
| tokenize · pool · normalize | 우리가 구현(또는 wrapper에 포함) | llama-server 내장 |
| HTTP 서버 | Go `net/http`로 자체 작성 | llama-server 자체 내장 |
| Go 코드 | ~300줄 | ~50줄 (supervisor/health 정도) |
| 업스트림 | ONNX Runtime(MS) + 자체 wrapper 스크립트 | llama.cpp 커뮤니티 |
| 파리티 검증 | Python sentence-transformers와 비교 필요 | 동일. GGUF 기반이라 tokenizer 일치 기대치 높음 |
| 재사용성 | 자체 HTTP endpoint | 업계 표준 endpoint (다른 프로젝트도 이미 쓰는 경우 많음) |

**잠정 방향**: 양쪽 **1-2시간짜리 POC** 후 결정. 둘 다 PyTorch 기준치와 충분히 일치(cosine ≥0.999)하면 llama-server 선호 (코드 덜 짜고 표준).

**다음 액션**:
- POC 1: `llama-server --model qwen3-embedding-0.6b.gguf --embedding --port ...` 실행 후 curl로 `/embedding` 호출, PyTorch 기준값과 비교
- POC 2: ONNX Runtime Go로 wrapper-exported `.onnx` 로드해서 같은 비교
- 결과를 `research/onnx-vs-llama-server-poc.md`에 기록 후 이 항목 결정

---

### Q3. Multi-MCP에서 envector `ActivateKeys` 경쟁

**배경**: envector 서버는 "한 번에 한 키만 resident" 제약이 있어, `ActivateKeys` 호출 시 4-RPC 시퀀스(list → register-if-missing → **unload-others** → load-target)를 수행한다.

새 구조에서는 **세션마다 rune-mcp 프로세스가 독립적으로 `ActivateKeys` 호출**한다. 같은 유저 같은 key_id면 결과는 동일하지만:
- 프로세스 A가 자기 키 로드 중
- 프로세스 B가 동시에 같은 키 activate 시도 → "unload-others" 단계에서 A가 올리는 중인 걸 내리려 함?
- race condition 가능성

SDK의 `activationMu sync.Mutex`는 **intra-process**만 보호. inter-process(여러 rune-mcp 간)는 비보호.

**검토 중인 선택지**:
- **(a)** 첫 MCP만 ActivateKeys 호출, 나머지는 skip — 파일 lock 또는 소켓 기반 조율
- **(b)** 매 MCP가 호출하되 server-side 멱등성에 의존 — 키 ID 같으면 문제 없다고 믿음
- **(c)** rune-mcp 밖에 브로커 프로세스 하나 더 두기 — 구조 복잡도 증가

**잠정 방향**: 실제 envector 서버의 동시 activate 동작을 테스트로 확인 필요. 같은 유저·같은 key_id 시나리오에서 race가 실제 문제를 일으키는지 실측.

**다음 액션**: envector 서버에 2개 rune-mcp에서 동시 `ActivateKeys` 호출하고 최종 상태 검증. 결과에 따라 (a)/(b)/(c) 중 선택.

---

### Q4. envector-go SDK `OpenKeysFromFile` 조건 완화

**배경**: rune은 **Vault-delegated 보안 모델**을 쓴다 — SecKey는 Vault에만, rune은 EncKey + EvalKey만 로컬에 보유. pyenvector는 rune이 monkey-patch로 이 모드를 우회하고 있으나, Go에선 언어적으로 monkey-patch 불가.

현재 `envector-go-sdk`의 `OpenKeysFromFile`이 `SecKey.json` 파일 존재를 필수 요구 → SecKey 없이는 `Keys` 객체 생성 불가 → rune이 `Insert` 용 Encryptor를 못 씀.

**검토 중인 선택지**:
- **(방법 1)** SDK에 조건 완화 PR — `SecKey.json` 없으면 `Keys.dec = nil`, `Keys.Decrypt`는 `ErrDecryptorUnavailable` 반환. 약 10줄 변경, non-breaking
- **(방법 2)** rune이 fake SecKey.json 생성으로 우회 — mock backend만 통과. libevi 붙으면 깨짐. 기술 부채

**잠정 방향**: **방법 1 채택**. SDK PR 제출 → 머지 예상 2-5일. PR 머지 대기가 1주+ 길어질 때만 방법 2 임시 적용.

**다음 액션**: envector-go SDK 팀에 PR 제출. PR 본문에 rune의 pyenvector monkey-patch 5개를 정당성 근거로 첨부.

**관련 파일**: `components/envector-integration.md` (방법 1 패치 명세 포함 예정)

---

## 🟡 설계 디테일 미정 (Phase 1 전)

### Q5. rune-embedder와 rune-mcp 설치 시 순서

**배경**: `/rune:configure` 실행 시 두 바이너리 설치·launchd 등록 순서 결정. embedder가 먼저 떠있어야 첫 rune-mcp가 embedding 요청 가능.

**검토**:
- rune-embedder 바이너리 다운로드 → launchd unit 등록 + load → embedder가 모델 로드 시작 (몇 초)
- rune-mcp 바이너리 다운로드 → Claude Code에 MCP 등록
- 첫 Claude 창 열 때 rune-mcp 기동 → embedder `/health` 확인 후 ready

**다음 액션**: `/rune:configure` 워크플로우 재작성 시 명시.

---

### Q6. rune-embedder 버전과 rune-mcp 버전 호환

**배경**: 두 바이너리가 독립 배포되면 버전 mismatch 가능. 예를 들어 embedder가 response schema 바꾸면 구 MCP가 파싱 실패.

**검토 중인 선택지**:
- API에 `/v1/embed` 같은 버전 prefix
- response에 `model_version` 필드 포함하고 mismatch면 경고
- 둘 다 같은 releases에서 번들 배포하고 "rune toolchain 1.0" 같은 단일 버전으로 관리

**잠정 방향**: 초기엔 **단일 버전 번들 배포** (복잡도 최소). `/v1/` 네임스페이스는 예방책으로 처음부터 삽입.

**다음 액션**: `components/rune-embedder.md`에 API 스펙 작성 시 확정.

---

### Q7. rune-embedder 보안 경계

**배경**: embedder는 unix socket (`~/.rune/embedder.sock`)으로 노출. 같은 유저의 어떤 프로세스든 소켓에 접근 가능.

**검토**:
- 0600 퍼미션만으로 충분한가? (같은 uid 프로세스는 전부 접근 가능)
- peer credential 검증(`SO_PEERCRED`/`getpeereid`)으로 특정 uid만 제한
- HMAC 기반 auth 토큰 추가?

**잠정 방향**: 0600 + peer credential(같은 uid만). embedding은 민감 데이터 아니지만 로그 주입·DoS 방지 관점에서 기본 방어.

**다음 액션**: `components/rune-embedder.md`에 socket 보안 정책 명시.

---

### Q8. capture_log.jsonl 저장 위치 · rotation

**배경**: Python은 `~/.rune/capture_log.jsonl`에 append (0600). 세션별 MCP가 동시에 같은 파일에 쓰면 경쟁 가능.

**검토 중인 선택지**:
- **(a)** 한 파일 + `sync.Mutex` + file lock (`flock`) — 여러 프로세스 동시 append 안전. 표준 Unix 패턴
- **(b)** 세션별 파일 `capture_log.<pid>.jsonl` — 나중에 합쳐야 함. 복잡
- **(c)** 로그 수집용 별도 프로세스 — 과함

**잠정 방향**: (a) — OS-level `flock` + Go `sync.Mutex` 조합. lumberjack 등 rotation은 수집 빈도 본 후 결정.

**다음 액션**: `components/rune-mcp.md`에 저장 정책 명시.

---

### Q9. Vault 주소 오타 · 영구 실패 UX

**배경**: 부팅 시 Vault 호출이 계속 실패하면 rune-mcp는 `waiting_for_vault` 상태로 retry 지속. 20회 지나도 실패면 경고 로그 + `/health` flag. 그런데 **사용자가 이 상태를 어떻게 알아채는가**?

**검토**:
- Claude 창에서 capture 시도 → 503 VAULT_PENDING 받음 → 메시지에 "run /rune:vault_status로 진단" 힌트
- `/rune:vault_status` 명령이 `last_error` · `attempt_count` · `elapsed` 보여줌
- 영구 실패 의심 시 사용자에게 "config.vault.endpoint 확인해보세요" 제안

**잠정 방향**: 위 흐름. `tool_vault_status` Go 구현 시 상세 진단 정보 포함.

**다음 액션**: `components/rune-mcp.md`의 tool 스펙에 반영.

---

## 📅 Post-MVP 고려 항목

아래는 **MVP scope 밖**이지만 로드맵에 기록해둘 항목들. Phase 2 이후 상황 보고 재검토.

### Post-MVP 1. Novelty check (c) ADVISORY 전환

MVP는 "≥0.95 near_duplicate면 저장 거부" (Python 동일). 그런데 "0.97인데 진짜 update하고 싶었던 상황"에서 사용자 좌절 발생 가능. Phase 2+에 에이전트 md(scribe.md) 재작성 시 (c) advisory로 전환 고려 — runed는 novelty class + similar_to만 반환하고, 저장 결정은 에이전트가. 실제 피드백 축적 후 판단.

### Post-MVP 2. Vault `DecryptBundle` RPC 통합

현재 recall은 `DecryptScores` + `DecryptMetadata` 두 번 왕복. 하나로 합치면 latency 절감. rune-Vault proto 변경 필요 → cross-team coordination.

### Post-MVP 3. Phase chain expansion · Group assembly

Python retriever의 phase_chain 기능 (group_id 공유 sibling fetch). MVP는 DEFER, flat list 반환. Phase chain이 실제로 recall 품질에 중요한지 데이터 확인 후 복귀.

### Post-MVP 4. Scientist pattern shadow run

Python/Go 병렬 실행 + diff 기록. cutover 리스크 완화용. Phase 3 전에 1주 이상 shadow 돌려 diff 분포 수집.

### Post-MVP 5. mTLS Vault 연결

현재 server TLS only. Prod 배포 전 mTLS로 전환 — cross-team cert 프로비저닝 필요.

### Post-MVP 6. Release signing · SBOM

Go 바이너리 cosign + syft SBOM 첨부. macOS codesign + notarize. supply-chain 방어.

---

## 결정되면 어디로 옮기나

| 항목 | 결정 후 이관처 |
|---|---|
| Q1 AES-MAC | `components/envector-integration.md` + 관련 코드 주석 |
| Q2 embedder 엔진 | `components/rune-embedder.md` (해당 엔진 섹션 확정) |
| Q3 ActivateKeys 경쟁 | `components/envector-integration.md` |
| Q4 SDK 조건 완화 PR | `components/envector-integration.md` (PR 머지 후 제거) |
| Q5 설치 순서 | `architecture.md` 설치 섹션 (신규) |
| Q6 버전 호환 | `components/rune-embedder.md` |
| Q7 embedder 보안 | `components/rune-embedder.md` |
| Q8 capture_log | `components/rune-mcp.md` |
| Q9 Vault 오타 UX | `components/rune-mcp.md` tool 스펙 |

Post-MVP 항목은 결정되더라도 별도 `roadmap.md` (필요 시)에 수집.
