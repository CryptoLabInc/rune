# Implementability Report — Go 포팅 구현가능성 검증

**작성일**: 2026-04-22  
**질문**: "Go 개발자가 `docs/v04/`만 읽고 Python 동작을 bit-identical로 Go에 구현할 수 있는가?"  
**방법**: 4-domain 병렬 검증 (Capture / Recall / Lifecycle-adapters / Bootstrap-state-error) + 주요 주장 직접 코드 확인

> **해석 주의**: "bit-identical 포팅"이 목표지만 Python 소스는 **설계 레퍼런스**로 항상 사용된다. "docs only" 순수 테스트는 불가능한 수준의 완결성이 필요하므로, 실제 기준은 "docs + python 소스 read"로 판정한다.

---

## 요약

| 영역 | 구현가능성 | 블로커 | 수정 필요 |
|---|---|---|---|
| **Recall flow** | ✅ 95% Full | 0 | AES envelope 필드 의미 footnote |
| **Lifecycle 6 tools** | ✅ 100% Full | 0 | 없음 |
| **Vault/envector/embedder adapter** | ✅ 95% Full | 0 | Q3 ActivateKeys race, envector-go SDK PR (외부 대기) |
| **Bootstrap · state machine · error** | 🟡 85% Full | 0 | `_maybe_reload_for_auto_provider` 결정 |
| **Capture flow** | 🟡 **70% Full** | **2** | **tier2 필드 + templates.py 상세** |
| **DecisionRecord schema** | 🟡 Partial | 0 | 6 enum 값 전수 명시 |

**종합**: 구현 시작 가능. **2개 P0 블로커** (capture 쪽) 해소 후 병행 구현 권장.

---

## A. 실제 구현 블로커 (2건)

### 🔴 BLOCKER #1: `tier2` 필드 처리 전부 누락

**Python 동작** (`server.py:L1240-1267`):
```python
data = parse_llm_json(extracted)
if not data:
    return {"ok": False, "error": "Invalid extracted JSON — could not parse."}

# Tier 2 check: agent already evaluated
tier2 = data.get("tier2", {})
if not tier2.get("capture", True):
    return {
        "ok": True,
        "captured": False,
        "reason": f"Agent rejected: {tier2.get('reason', 'no reason')}",
    }

agent_domain = tier2.get("domain", "general")
agent_confidence = data.get("confidence")  # 0-1 clamp

detection = _detection_from_agent_data(
    domain=agent_domain,
    confidence=float(agent_confidence) if agent_confidence is not None else 0.0,
)
```

**Go 문서 검색 결과**: `tier2` 문자열 **zero hits** (verification-matrix.md 제외)

**gap**:
- agent-delegated CaptureRequest에 `tier2.{capture, reason, domain}` 필드가 있다는 것 자체가 미기술
- rejection path (`tier2.capture=false` → `captured=false` with agent's reason) 없음
- `confidence` 필드 0-1 clamping 누락

**영향**: Python 에이전트가 보내는 실제 JSON shape와 Go 서버가 기대하는 shape 불일치 → agent integration 실패.

**권고**: `spec/flows/capture.md` Phase 2에 다음 추가:
1. `CaptureRequest.extracted` JSON shape에 `tier2: {capture: bool, reason: string, domain: string}` 명시
2. Phase 2 분기: `tier2.capture=false` → rejection response 조립
3. `confidence` 필드 0-1 클램프 규칙
4. `agent_domain = tier2.domain || "general"` fallback

---

### 🔴 BLOCKER #2: `templates.py` (render_payload_text) 363 LoC 미상세

**Python 실재**: `agents/common/schemas/templates.py` (363 LoC, `wc -l` 확인)

구성:
- `PAYLOAD_TEMPLATE` 멀티라인 템플릿 문자열 (L14~)
- 7개 헬퍼 함수:
  - `_format_alternatives(alternatives, chosen)` L52 — "chosen" marker 삽입
  - `_format_trade_offs` L66
  - `_format_assumptions` L73
  - `_format_risks` L85
  - `_format_evidence` L97 — quote 번호 매김
  - `_format_links` L118
  - `_format_tags` L131
- `render_payload_text` L138 (메인 진입점, PAYLOAD_TEMPLATE.format(...) at L183)
- `render_compact_payload` L225
- `render_display_text` L288 (다국어 지원)

**Go 문서 상태**:
- `overview/decisions.md` D15 (L830): "Python templates.py 363 LoC 전체 Go 포팅"
- `spec/flows/capture.md:L295`: "`payload_text.go` # RenderPayloadText (templates.py 이식)"
- **알고리즘·템플릿 문자열·헬퍼 로직 전부 Go 문서에 없음**

**영향**: payload.text는 **embedding 대상 텍스트**. Python과 다르게 렌더하면 vector 공간이 달라져 recall 품질 변함. Bit-identical 테스트 통과 불가.

**권고** (옵션 둘 중 하나):
- A. Python `templates.py`를 **설계 레퍼런스로 명시**하고 "line-by-line 포팅, golden fixture로 검증" 원칙 못박기
- B. Go 문서에 PAYLOAD_TEMPLATE 전문 + 7 헬퍼 알고리즘 인라인 (300+ LoC 추가)

→ B는 doc 유지 cost 큼. A (Python source를 canonical reference로 지정 + golden test) 권장.

---

## B. 구현 친화성 문제 (블로커 아니지만 갭)

### ✅ B.1 Enum 값 전수 — **해소됨** (2026-04-22)

**Python** (`agents/common/schemas/decision_record.py:L19-80`): Domain 19 · Sensitivity 3 · Status 4 · Certainty 3 · ReviewState 4 · SourceType 7 = 40값  
**Python** (`agents/retriever/query_processor.py:L23-41`): QueryIntent 8 · TimeScope 5

**Go 문서**: `spec/types.md` §1.1-1.8 — 8 enum 전수 Go const 블록 + `ParseDomain` 시그니처 등 포함.

**해소 방식**: P1 #1로 `spec/types.md` 신규 작성 (2026-04-22). 모든 도메인 타입의 단일 진실 소스로 설정.

### 🟡 B.2 `_maybe_reload_for_auto_provider` 미명시

**Python**: `server.py:L451-488`
- `_infer_provider_from_context(ctx)` — MCP clientInfo.name에서 provider 추론 (claude/openai/gemini)
- `_maybe_reload_for_auto_provider(ctx)` — 추론된 provider가 override와 다르면 `_init_pipelines()` 재호출
- 호출 위치: L706 (capture), L826 (batch_capture), L918 (recall)

**분석**:
- 이 기능은 **legacy tier2/llm_extractor LLM 클라이언트**를 위한 것
- D14 (agent-delegated) 모드에서 rune-mcp는 내부 LLM 사용 안 함
- 따라서 **v0.4 Go에서는 실제로 dead code**일 가능성 높음

**Go 문서**: 언급 **zero**

**권고**: `overview/decisions.md`에 결정 추가:
- "D14 agent-delegated로 내부 LLM 제거됨에 따라 auto-provider reload 기능 **Go에서 제거**"
- 또는 runtime에 tier2 LLM 남겨둔다면 포팅 필요 명시

### 🟡 B.3 Phase 5 `extractPayloadText` fallback chain

**Python** (`searcher.py:L487-496`):
```python
payload = metadata.get("payload", {})
if isinstance(payload, dict):
    payload_text = payload.get("text", "")
else:
    payload_text = metadata.get("text", raw.get("text", ""))

if not payload_text:
    decision = metadata.get("decision", {})
    if isinstance(decision, dict):
        payload_text = decision.get("what", "")
```

**Go 문서**: `recall.md:L848` — 한 줄 참조만

**권고**: recall.md Phase 5 또는 SearchHit 변환 섹션에 4-step fallback 알고리즘 인라인.

### 🟡 B.4 AES envelope 필드 의미 미설명

**Python** (`searcher.py:L424`): `if "a" in parsed and "c" in parsed:` — 필드 의미 주석 없음  
**Go `spec/flows/capture.md:L299`**: `{"a": "agent_xyz", "c": "base64(IV||CT)"}` — 예시로만 설명

**권고**: `spec/flows/capture.md` Phase 5c와 `spec/flows/recall.md` Phase 5에 footnote:  
> `"a"` = agent_id (16-32자), `"c"` = base64(IV(16B) ‖ AES-CTR ciphertext)

### 🟡 B.5 Phase 4 near_duplicate 응답 shape

**Python** (`server.py:L1363-1369`):
```python
if novelty_info["class"] == "near_duplicate":
    return {
        "ok": True,
        "captured": False,
        "reason": "Near-duplicate — virtually identical insight already stored",
        "novelty": novelty_info,
    }
```

**Go `capture.md:L48`**: "near_duplicate (similar_to, OK=false)" — 필드 부분적 기술

**권고**: 정확한 JSON shape 명시 (특히 `ok=True` vs `OK=false` 상충 해소).

---

## C. Agent들이 오인한 주장 (내가 실측으로 확인)

### ❌ 오인 #1: Novelty threshold 불일치 (Capture agent 주장)

**주장**: Python `0.4/0.7/0.93` vs Go docs `0.3/0.7/0.95` → 블로커

**실측**:
- `embedding.py:L16-18` 모듈 상수: `0.4/0.7/0.93` (있음)
- `server.py:L102-104` `_classify_novelty` defaults: `0.3/0.7/0.95`
- `server.py:L1352` 호출: `_classify_novelty(max_sim)` — **인자 없이 호출**
- 즉 실제 runtime: server.py defaults `0.3/0.7/0.95` 사용

**결론**: embedding.py 모듈 상수는 **dead defaults** (runtime 미사용). 실제 동작은 `0.3/0.7/0.95`이며 Go 문서와 일치. **블로커 아님**.

> 단, Python 자체에 inconsistency는 있음 (상수와 call site defaults가 다름). Python bug로 별도 리포트 가치.

### ❌ 오인 #2: Lifecycle adapters에 envector 11 error patterns 누락

**주장**: "envector-integration.md에 매핑 미상세"

**실측**: envector-integration.md는 **구조적으로 다른 접근** (gRPC status code + SDK typed error). Python의 string pattern matching과 의도적으로 다른 설계. **일치시킬 필요 없음** (Go에서는 상위 추상화가 맞음).

---

## D. 우선순위 정리

### 🔴 P0 블로커 (구현 시작 전 해소 필수)

| # | 항목 | 수정 위치 | 예상 시간 |
|---|---|---|---|
| 1 | tier2 필드 처리 누락 | spec/flows/capture.md Phase 2 + rune-mcp.md CaptureRequest | 30분 |
| 2 | render_payload_text 포팅 전략 | decisions.md D15 + spec/flows/capture.md Phase 5 | 15분 (A안: Python 참조 지정) |

### 🟡 P1 (구현 병행 정비)

| # | 항목 | 수정 위치 |
|---|---|---|
| 3 | 6 enum 값 전수 | spec/components/rune-mcp.md |
| 4 | _maybe_reload_for_auto_provider 결정 (drop/port) | decisions.md 신규 결정 |
| 5 | extractPayloadText fallback | spec/flows/recall.md Phase 5 |
| 6 | AES envelope `"a"`, `"c"` 의미 | spec/flows/capture.md + recall.md footnote |
| 7 | near_duplicate 응답 JSON shape | spec/flows/capture.md Phase 4 |
| 8 | Vault MAX_MESSAGE_LENGTH 256MB gRPC 옵션 | spec/components/vault.md (verification-matrix C.4 동일) |

### 🟢 P2 (Post-구현 가능)

- PII redaction 책임 경계 명확화 (verification-matrix C.1)
- `render_compact_payload`, `render_display_text` 2 추가 함수 포팅 scope 결정
- Q3 Multi-MCP ActivateKeys race 실측
- envector-go SDK `OpenKeysFromFile` PR 머지 대기

---

## E. 최종 판정

### "Go 개발자가 docs/v04/만 보고 구현 가능한가?"

**엄격 기준 (docs only)**: ❌ 불가  
- tier2, templates.py, enum 전수, auto-provider reload 모두 Python 소스 참조 필요

**실용 기준 (docs + Python source as reference)**: 🟡 조건부 가능  
- P0 블로커 2건 수정 후 구현 진행 가능
- Python은 설계 레퍼런스로 항시 옆에 두는 것이 정상

### Go 구현 진입 로드맵

1. **Day 0 (오늘)**: P0 2건 수정 (45분 작업)
2. **Week 1**: scaffolding — cmd/rune-mcp, config, Vault client, state machine (B.2 병행)
3. **Week 2-3**: Capture flow (Phase 1-7) + Lifecycle 6 tools
4. **Week 3-4**: Recall flow (Phase 1-7)
5. **Week 4**: Integration test + golden fixture (Python ↔ Go bit-identical 검증)

### 추정 구현가능성: 🟡 → ✅ 

**P0 해소 후 실제 구현 blocking 없음.** 모든 나머지는 병행 또는 post-구현 정비 가능.
