# 미결 항목 · 블로킹

Rune v0.4.0 작업 중 아직 결정 안 된 것들. 각 항목은 "왜 결정이 필요한가 · 검토 중인 선택지 · 다음 액션"으로 정리.

결정이 내려지면 해당 항목을 `📦 Archived` 섹션으로 이동 또는 관련 spec 문서에 반영.

**현재 상태 (2026-04-22)**: 구현 blocking 0건. 아래 2건만 active.

---

## 🔵 Deferred (Post-MVP)

### Q1. AES envelope에 MAC 필드 추가 여부

**배경**: 현행 AES-256-CTR envelope `{"a":agent_id, "c":base64(IV||CT)}`는 인증 태그가 없다. 암호문 바이트를 flip해도 복호화 측이 감지하지 못해 **malleability 공격 취약**. 팀 메모리 품질 보호 관점에서 장기적으로 위험.

**제약**: pyenvector(Python)과 envector-go(Go) 양쪽 클라이언트가 동시 지원해야 호환. Vault는 FHE 경로라 무관.

**검토 중인 선택지**:
- **(a)** envelope에 `"m"` 필드 추가 — `HMAC-SHA256(dek, a||iv||ct)[:16]`. 기존 레코드는 `m` 없으면 verify skip + 4주 grace period
- **(b)** AES-GCM으로 마이그레이션 — 장기. 포맷 자체 변경. legacy 재암호화 경로 필요
- **(c)** 현상 유지 — 비권장

**잠정 방향**: (a) Phase 1 초반 채택. pyenvector와 envector-go 양쪽 동시 릴리스 필요.

**다음 액션**: pyenvector 팀과 릴리스 타이밍 조율 + envector-go SDK에 PR (§Q4와 함께).

**구현 영향**: MVP 구현에는 영향 없음 (CTR 사용). 추가 시 envelope 포맷에 `"m"` key 추가되는 형태 예정.

---

## 🟡 외부 의존성 대기

### Q4. envector-go SDK `OpenKeysFromFile` 조건 완화

**배경**: rune은 **Vault-delegated 보안 모델**을 쓴다 — SecKey는 Vault에만, rune은 EncKey + EvalKey만 로컬에 보유. pyenvector는 rune이 monkey-patch로 이 모드를 우회하고 있으나, Go에선 언어적으로 monkey-patch 불가.

현재 `envector-go-sdk`의 `OpenKeysFromFile`이 `SecKey.json` 파일 존재를 필수 요구 → SecKey 없이는 `Keys` 객체 생성 불가 → rune이 `Insert` 용 Encryptor를 못 씀.

**검토 중인 선택지**:
- **(방법 1)** SDK에 조건 완화 PR — `SecKey.json` 없으면 `Keys.dec = nil`, `Keys.Decrypt`는 `ErrDecryptorUnavailable` 반환. 약 10줄 변경, non-breaking
- **(방법 2)** rune이 fake SecKey.json 생성으로 우회 — mock backend만 통과. libevi 붙으면 깨짐. 기술 부채

**잠정 방향**: **방법 1 채택**. SDK PR 제출 → 머지 예상 2-5일. PR 머지 대기가 1주+ 길어질 때만 방법 2 임시 적용.

**다음 액션**: envector-go SDK 팀에 PR 제출. PR 본문에 rune의 pyenvector monkey-patch 5개를 정당성 근거로 첨부.

**구현 영향**: Capture/Recall **core 로직 구현은 PR과 무관하게 진행 가능**. PR 머지 전까지는 mock backend 테스트로 충분. libevi 연동 시점에만 blocking.

**관련 파일**: `spec/components/envector.md` (방법 1 패치 명세 포함 예정)

---

## 📦 Archived (embedder 프로젝트 이관)

embedder 프로젝트가 별도 분리되면서 (D30), 이 repo 밖 결정으로 이관된 항목들.

### Q2. rune-embedder 실행 엔진 (ONNX vs llama-server)

**해소**: embedder 프로젝트가 llama-server 채택한 것으로 전달받음 (D29 Archived 참조). 이 repo 밖에서 결정.

### Q5. rune-embedder와 rune-mcp 설치 시 순서

**해소**: embedder 프로젝트 scope. rune-mcp는 embedder down 시 retry backoff (D7)만 수행. 설치 order는 embedder 팀의 `/configure` 워크플로우 책임.

### Q6. rune-embedder 버전과 rune-mcp 버전 호환

**해소**: embedder 프로젝트가 proto 버전 관리. rune-mcp는 generated Go stub을 `go.mod` import만. API spec 확정 시 `spec/components/embedder.md`의 placeholder `embedder.v1`을 실제 이름으로 교체.

### Q7. rune-embedder 보안 경계 (socket 퍼미션)

**해소**: embedder 프로젝트 책임. rune-mcp는 socket path 받아서 gRPC client dial만 수행 (unix socket, 커널 mediated).

---

## ✅ Decided (open-questions에서 spec 문서로 이관 완료)

### Q3. Multi-MCP `ActivateKeys` 경쟁 → Option (b) 채택

**배경**: envector 서버 "한 번에 한 키만 resident" 제약. 여러 rune-mcp가 동시에 같은 `ActivateKeys` 호출 시 race 가능성.

**결론**: **(b) server-side 멱등성 의존**. 같은 key_id 동시 호출은 envector 서버가 처리 가능.

**근거**: 
- Python `ev.init()` single call (no intra-process locking) 기반 production에서 문제 발생 안 함
- SDK `activationMu sync.Mutex`는 intra-process만 보호. inter-process 경쟁은 server에 위임
- (a) 파일 lock이나 (c) 브로커 프로세스는 premature optimization

**재평가 트리거**: staging에서 다중 세션 동시 startup 시 실제 race 관찰 시 (a) 파일 lock 도입 고려.

**이관 위치**: `spec/components/envector.md` 내 언급.

---

### Q8. `capture_log.jsonl` 저장 · 동시 쓰기 → flock + Mutex 채택

**배경**: 세션별 rune-mcp가 동시에 같은 `~/.rune/capture_log.jsonl`에 append.

**결론**: **(a) 한 파일 + `sync.Mutex` (intra-process) + `flock(LOCK_EX)` (inter-process)** + atomic append (fsync).

**Rotation**: 초기 없음. 수집 빈도 관찰 후 lumberjack 등 검토 (post-MVP).

**이관 위치**: `spec/components/rune-mcp.md` "Capture log" 섹션에 반영됨.

---

### Q9. Vault 주소 오타 · 영구 실패 UX → tool_vault_status 상세화

**배경**: 부팅 시 Vault 실패 계속 → `waiting_for_vault` 상태 retry 지속. 사용자가 원인 파악 어려움.

**결론**: 다층 힌트 제공
- capture 시도 → 503 `VAULT_PENDING` + "run `/rune:vault_status` 진단 힌트"
- `/rune:vault_status` → `last_error` · `attempt_count` · `elapsed` 반환
- 영구 실패 의심 시 "`config.vault.endpoint` 확인" 제안

**이관 위치**: `spec/flows/lifecycle.md` §1 `tool_vault_status` 스펙에 반영됨.

---

## 📅 Post-MVP 고려 항목

아래는 **MVP scope 밖**이지만 로드맵에 기록해둘 항목들. Phase 2 이후 상황 보고 재검토.

### Post-MVP 1. Novelty check (c) ADVISORY 전환

MVP는 "≥0.95 near_duplicate면 저장 거부" (Python 동일). "0.97인데 진짜 update하고 싶었던 상황"에서 사용자 좌절 가능. Phase 2+에 에이전트 md(scribe.md) 재작성 시 advisory 전환 고려 — rune-mcp는 novelty class + related만 반환, 저장 결정은 에이전트가.

### Post-MVP 2. Vault `DecryptBundle` RPC 통합

현재 recall은 `DecryptScores` + `DecryptMetadata` 두 번 왕복. 하나로 합치면 latency 절감. rune-Vault proto 변경 필요 → cross-team coordination.

### Post-MVP 3. Scientist pattern shadow run

Python/Go 병렬 실행 + diff 기록. cutover 리스크 완화용. Phase 3 전에 1주 이상 shadow 돌려 diff 분포 수집.

### Post-MVP 4. mTLS Vault 연결

현재 server TLS only. Prod 배포 전 mTLS로 전환 — cross-team cert 프로비저닝 필요.

### Post-MVP 5. Release signing · SBOM

Go 바이너리 cosign + syft SBOM 첨부. macOS codesign + notarize. supply-chain 방어.

### Post-MVP 6. `_format_alternatives` 빈 `chosen` 버그 수정 (templates.py)

Python `agents/common/schemas/templates.py:L59`:
```python
if alt.lower() == chosen.lower() or chosen.lower() in alt.lower():
    lines.append(f"- {alt} (chosen)")
```

**버그**: `chosen=""` 이면 `"" in alt.lower()` 항상 True → 모든 alternative가 "(chosen)" marker로 표시됨. 의도와 정반대 (선택 안 했는데 전부 선택된 것처럼 렌더).

**MVP 현재**: Python bit-identical 유지 (D15 canonical-reference). Go 구현도 동일 버그 재현 (golden fixture 일관성).

**Post-MVP 방침**: Python에서 `if chosen and (...)` guard 추가하여 수정. Go는 수정된 Python 따라감. 영향:
- 새로 capture되는 레코드의 `payload.text`는 "(chosen)" marker 없게 렌더 (chosen 빈 경우)
- 기존 저장된 레코드는 `reusable_insight`가 primary embedding source (D14 에이전트 생성)이므로 재인덱싱 불필요. `payload.text` 표시는 recall 응답 시점에 새 렌더 룰 적용
- Golden fixture 업데이트 필요

**다음 액션**: MVP 완료 후 Python PR → Go 재동기화 → golden fixture regen.

### Post-MVP 7. embedder model_identity 변경 자동 감지

현재 MVP는 slog 로깅만 (D30). embedder가 모델 바꾸면 기존 벡터와 쿼리 벡터 공간 불일치. 자동 감지 · 재임베딩 migration tool 필요할 수 있음. 실사용 패턴 관찰 후 설계.

---

## 결정되면 어디로 옮기나

| 항목 | 결정 후 이관처 |
|---|---|
| Q1 AES-MAC | `spec/components/envector.md` + 관련 코드 주석 |
| Q4 SDK 조건 완화 PR | `spec/components/envector.md` (PR 머지 후 제거) |

나머지는 이미 spec 또는 archived로 이관 완료.
