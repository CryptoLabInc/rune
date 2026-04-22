# Python ↔ Go 최종 정합성 평가

**시작일**: 2026-04-22  
**범위**: `mcp/`, `agents/` Python 전수 (~10,626 LoC, 40 파일)  
**목적**: Python 개별 코드 라인 단위로 Go 문서와 bit-identical 정합성 최종 확인  
**방식**: 순차 검증. 파일마다 {Python 요약 · Go 매핑 · missed detail · 수정 필요}

## 진행 순서

1. [x] `agents/common/schemas/decision_record.py` — types.md ✅ (불일치 1건 수정)
2. [x] `agents/common/schemas/embedding.py` — types.md ✅ (critical 불일치 3건 수정)
3. [x] `mcp/adapter/vault_client.py` — spec/components/vault.md ✅ (critical 2건 + 보충 4건 수정)
4. [x] `mcp/adapter/envector_sdk.py` — spec/components/envector.md ✅ (critical 1 + 보충 4건 수정)
5. [x] `agents/retriever/query_processor.py` — recall.md + types.md ✅ (수정 0건 — 가장 깨끗)
6. [x] `agents/retriever/searcher.py` — recall.md ✅ (critical 2 + 보충 2건 수정)
7. [x] `agents/scribe/record_builder.py` — capture.md + policy/ ✅ (canonical-reference 섹션 신설)
8. [x] `agents/common/schemas/templates.py` — D15 canonical ✅ (포팅 주의사항 3건 추가)
9. [x] `agents/common/config.py` — rune-mcp.md ✅ (5건 보강)
10. [x] `mcp/server/errors.py` — rune-mcp.md 에러 ✅ (7종 bit-identical + 1종 Go 신규)
11. [x] `mcp/server/server.py` — 8 tools (flows/*) ✅ (4건 보충)
12. [x] Track C: `envector_client.py`, `document_preprocess.py` scope 확정 ✅
13. [x] Track B 최종 확인 (D14/D21/D28/D30 제외 파일들) ✅

## 발견 사항

### 1. `agents/common/schemas/decision_record.py` ✅ (2026-04-22)

**Python**: 260 LoC, 6 enum + 9 sub-model + DecisionRecord v2.1 + 2 helper (generate_record_id/group_id) + 2 method (validate/ensure_certainty_consistency).

**Go 매핑 (spec/types.md)**:
| Python | Go |
|---|---|
| 6 enum (L19-80) | §1.1-1.6 ✅ |
| 9 sub-model (L87-159) | §2.1-2.9 ✅ |
| DecisionRecord main (L166-213) | §3 ✅ |
| validate_evidence_certainty (L215-224) | §7.2 ✅ |
| ensure_evidence_certainty_consistency (L226-242) | §7.1 ✅ (2-branch 로직 정확) |
| generate_record_id (L245-251) | §3 ⚠️ **수정됨** |
| generate_group_id (L254-259) | §3 ✅ |

**🔴 발견 및 수정**: `generate_record_id` slug 필터 로직이 **단어 단위**인데 Go 문서는 **문자 단위**로 기술됨. bit-identical 실패 가능.

- Python: `"_".join(w for w in words if w.isalnum() or w.replace("_", "").isalnum())` — 단어 전체 판정
- 수정 후 Go 의사코드: `isPyIsalnum(w) || isPyIsalnum(w.replace("_", ""))` 단어 단위 필터
- 예시: `"Add email@foo.com support"` → `"add_support"` (email 단어 통째로 drop)
- 한글 등 unicode 지원: `unicode.IsLetter(r) || unicode.IsDigit(r)`

### 2. `agents/common/schemas/embedding.py` ✅ (2026-04-22)

**Python**: 57 LoC, 3 상수 + `embedding_text_for_record` + `classify_novelty`.

**Go 매핑**:
| Python | Go |
|---|---|
| `embedding_text_for_record` (L21-30) | types.md §3 `EmbeddingTextForRecord` ✅ |
| `classify_novelty` (L33-56) | types.md §5.4 NoveltyInfo + capture.md Phase 4 |

**🔴 발견 및 수정 (3건)**:

1. **NoveltyClass 범위 swap**: `related`와 `evolution`이 documentation에서 뒤바뀜.
   - 정확: novel(<0.3) / **evolution(0.3~0.7)** / **related(0.7~0.95)** / near_duplicate(≥0.95)
   - 직관: evolution은 "중간 유사도 = 다른 각도의 같은 주제", related는 "높은 유사도 = 같은 토픽"
   - 수정: `spec/types.md §5.4` + `spec/flows/capture.md` Phase 4 표 전부 교정

2. **`novelty.score` 의미 틀림**: 문서는 "max similarity"라 했으나 실제는 `1.0 - max_similarity`, round(4).
   - novelty_score는 **inverted** (유사도 높을수록 낮음)
   - 초기값 1.0 (기존 레코드 없을 때 최대 novelty)
   - 수정: types.md §5.4 Score 코멘트 + 주석 추가

3. **`classify_novelty` 반환 필드 명확화**: 2필드만 `{class, score}` 반환. `related`는 server.py:L1353-1360에서 caller가 추가. 문서에 명시.

**✅ 확인 완료**:
- `embedding_text_for_record`: trim + fallback to payload.text ✅
- 상수 `NOVELTY_THRESHOLD_NOVEL=0.4, RELATED=0.7, NEAR_DUPLICATE=0.93`: **dead defaults** (server.py L102-104이 0.3/0.7/0.95 명시 전달) — 이미 matrix에 기록됨

### 3. `mcp/adapter/vault_client.py` ✅ (2026-04-22)

**Python**: 381 LoC. `VaultClient` class + `create_vault_client` factory + `DecryptResult` dataclass.

**🔴 Critical 발견 및 수정**:

1. **`DecryptMetadata` 입출력 완전히 틀림**:
   - 틀린 vault.md: "입력=refs[shard_idx,row_idx], 출력=AES envelope 상태 metadata. rune-mcp가 로컬 복호화"
   - 실제: Vault가 AES envelope 문자열 받아서 **AES-256-CTR 복호화까지 수행**, plaintext JSON 문자열 반환. rune-mcp는 `json.Unmarshal`만.
   - 검증: proto docstring "Decrypts a list of AES-encrypted metadata strings" 확인
   - 영향: 구현 시 완전히 다른 path 만들게 될 bug. **capture는 rune-mcp 직접 암호화, recall은 Vault 위임** (비대칭)
   - 수정: `spec/components/vault.md` DecryptMetadata 섹션 전면 재작성 + `spec/components/rune-mcp.md` AES envelope 섹션에 비대칭 책임 분담 명시 + `spec/flows/recall.md` L24 "rune-mcp 직접 복호화" → "Vault 위임"

2. **`decrypt_search_results` default `top_k=5` 누락**: Python L220 명시값. vault.md 추가.

**🟡 보충 (4건)**:

3. **Timeout default 30s**: Python L84 전 RPC 30초. vault.md 10초로 잘못 적혔던 것 수정 (30s + health만 5s).

4. **`RUNEVAULT_GRPC_TARGET` env override** (Python L108-110): vault.md Endpoint 섹션에 우선순위 명시 추가.

5. **gRPC health check `grpc_health.v1` 표준** (Python L309-316): vault.md Health check 2-tier 재구조화 (Tier 1 grpc_health_v1 + Tier 2 HTTP fallback).

6. **Factory `create_vault_client`**: 언급 없이 넘어감 (Go는 config 기반 사용 전제).

**✅ 확인 완료**:
- GetPublicKey: bundle 구조 + JSON 파싱 (envector_endpoint, agent_dek 등)
- DecryptScores: encrypted_blob_b64 입력, `{shard_idx, row_idx, score}` 출력
- _derive_grpc_target: 4가지 형식 (host:port, tcp://, http(s)://, bare) + default 50051
- TLS credentials: ca_cert file or system bundle
- MAX_MESSAGE_LENGTH 256MB 양방향 (최근 수정됨)
- HTTP health fallback: /mcp · /sse suffix 제거 후 GET /health

### 4. `mcp/adapter/envector_sdk.py` ✅ (2026-04-22)

**Python**: 387 LoC. 5 monkey-patch + CONNECTION_ERROR_PATTERNS(11) + EnVectorSDKAdapter + `_app_encrypt_metadata` + `_with_reconnect` + `call_*` methods + `_to_json_available`.

**🔴 Critical 발견 및 수정 (1건)**:

1. **envector.md Recall 예시가 여전히 "rune-mcp 로컬 복호화"로 잘못 기술** (vault_client.py 검증 결과 반영 안 됨):
   - 틀린 코드: `aesctr.Open(bundle.AgentDEK, bundle.AgentID, m.Data)` 각 envelope 로컬 복호화
   - 수정: `vaultClient.DecryptMetadata(ctx, envelopes)` 일괄 Vault 위임 (비대칭 책임 분담)
   - 추가: "비대칭 책임 분담" 주석 — capture는 local, recall은 Vault 위임
   - Python `searcher.py:L395-470` 기준

**🟡 보충 (4건)**:

2. **Score → Vault base64 encoding** 명시: Python `envector_sdk.py:L283-284` `base64.b64encode(serialized).decode('utf-8')` 동작. proto 확인 결과 `encrypted_blob_b64 string` 필드 → Go도 `base64.StdEncoding.EncodeToString(blob)` 필요. envector.md Recall Step 2에 추가.

3. **Batch metadata 암호화** (Python L253 `[...for m in metadata]`): envector.md Capture 경로 재작성 — N개 record 리스트 암호화 예시 + D16 batch 원칙 명시.

4. **`auto_key_setup=False` 설명** (Python L116): rune은 Vault에서 외부 공급 → 자동 키 생성 끔. envector.md 초기화 예시에 `WithAutoKeySetup(false)` + 주석 추가.

5. **`agent_dek`/`agent_id` mismatch warning** (Python L250-251): envector.md Capture 경로에 safety check + `slog.Warn` 추가.

6. **GetMetadata pre-validation** (Python L316-318 row_idx nil check): Go에서는 struct field이므로 자동 보장. envector.md에 설명 추가.

**✅ 확인된 일치 / 의도적 차이**:
- 11 CONNECTION_ERROR_PATTERNS: Go typed error 의도적 차이 (이미 문서화됨)
- 5 monkey-patches (`_safe_*_getter`): SDK 조건 완화 PR (Q4)로 대체 (이미 문서화됨)
- `_with_reconnect`: Go gRPC ClientConn 자동 복구 활용 (이미 문서화됨)
- `_app_encrypt_metadata` AES envelope 포맷: rune-mcp.md 섹션에 자세히
- `call_score` / `call_remind` / `call_get_index_list`: Go SDK API로 대체

### 5. `agents/retriever/query_processor.py` ✅ (2026-04-22)

**Python**: 437 LoC. QueryIntent + TimeScope + ParsedQuery + QueryProcessor (regex + LLM multilingual paths).

**수정 필요 사항**: **0건** 🎉

**✅ 완벽 일치 (12 항목)**:
- QueryIntent 8 enum · TimeScope 5 enum
- ParsedQuery (language field 제외, D21로 의도적 드롭)
- INTENT_PATTERNS: 31 regex × 7 intents (insertion-order — Go ordered slice로 정확 이식)
- TIME_PATTERNS: 16 regex × 4 scopes
- STOP_WORDS: 81 words 전수
- `_clean_query`: lowercase · strip · `\s+` collapse · `[.!,;:]+$` 제거 (? 보존)
- `_detect_intent`: 순차 매칭, 첫 hit 반환 else GENERAL
- `_detect_time_scope`: 동일 패턴
- `_extract_entities`: 4-stage (quoted · capitalized i>0 · tech × 4 groups · dedup[:10])
- `_extract_keywords`: `\w+` · stopword filter · len>2 · dedup[:15]
- `_generate_expansions`: intent별 (DECISION=3, FEATURE_HISTORY=2, PATTERN=2, TECHNICAL=2) + entity별 (entity[:3] × 2) · lowercase-key dedup[:5]

**⚠️ 의도적 차이 (D21 기반)**:
- `language: LanguageInfo` field → Go 드롭
- `_parse_multilingual` (LLM translation path) → Go 드롭  
- `LLMClient` 의존성 → Go 드롭 (agent-side 번역 전제)
- `QUERY_PARSE_PROMPT` → Go 불필요

**🟢 사소한 누락 (조치 불필요)**:
- `format_for_search` 메서드 (L419-436): `agents/tests/test_retriever.py:L88-90` 테스트 전용. 메인 pipeline 미사용. Go 포팅 scope 아님.

**정규식 flag 대응**: Python `query.lower() + re.IGNORECASE` ↔ Go `(?i)` inline flag. 효과 동등.

### 6. `agents/retriever/searcher.py` ✅ (2026-04-22)

**Python**: 576 LoC. SearchResult dataclass + Searcher class + 12 methods.

**🔴 Critical 발견 및 수정 (2건)**:

1. **`toSearchHit` 필드 경로·default 버그** (recall.md L825-848):
   - 틀린 key: `getString(meta, "record_id")` → 실제 Python은 `metadata.get("id")` (field name "id")
   - 틀린 path: `getString(meta, "certainty")` → 실제 `metadata["why"]["certainty"]` (nested)
   - 누락 fallback: Python은 `raw.get("id", "unknown")` 까지 chain. Go 누락.
   - 누락 defaults: Python은 id="unknown"/title="Untitled"/domain="general"/status="unknown"/certainty="unknown". Go는 ""
   - 영향: **RecordID 완전히 깨짐**, certainty 잘못된 값 (calculateConfidence weight 기본 0.3으로 떨어짐), recall 응답 품질 저하
   - 수정: `toSearchHit` 전면 재작성 + Python line reference 주석 + `getStringDefault` 헬퍼 패턴 + nested `why.certainty` 경로

2. **`age_days` integer truncation 누락** (recall.md L1075):
   - Python `(now - ts).days` = integer (timedelta.days 속성이 int)
   - Go `Hours()/24` = float — bit-identical 실패
   - 수정: `math.Floor(rawDays)` 적용 + 주의 주석

**🟡 보충 (2건)**:

3. **`_search_with_expansions` original fallback 추가** (recall.md L597-625):
   - Python L167-173: `if query.original not in query.expanded_queries`면 원본 case로 추가 검색
   - 효과: 원본 대소문자 보존한 embedding 시도 (expansions는 lowercase cleaned)
   - 수정: Step 2로 original fallback 로직 추가 (embedder.EmbedSingle + searchSingle + dedup)

4. **Timestamp ISO format `+00:00` vs `Z`** (recall.md L1023-1036):
   - Python `datetime.isoformat()` UTC → "+00:00" 출력
   - Go `time.RFC3339` UTC → "Z" 출력
   - lexicographic 비교 edge case 차이 ("+" < "Z" in ASCII)
   - 수정: Go에서 `"2006-01-02T15:04:05-07:00"` 커스텀 포맷 사용 (pyIsoFormat 상수)

**✅ 이미 일치 (대부분 기존 검증됨)**:
- `search` 6-step 순차 (expand → assemble → filter → time → rerank → topk)
- `_assemble_groups`: group + standalone interleave by best_score
- `_apply_metadata_filters`: domain/status/since
- `_filter_by_time`: TimeScope → timedelta (7/30/90/365)
- `_expand_phase_chains`: max_chains=2, Group: prefix, sibling merge (D27)
- `_search_via_vault`: 4-step Vault pipeline
- 상수: HALF_LIFE_DAYS=90, SIMILARITY_WEIGHT=0.7, RECENCY_WEIGHT=0.3, STATUS_MULTIPLIER
- 에러 처리: bare except → empty results (Phase 4 non-fatal)
- AES envelope 3-way classify (D26)

**🟢 Safe to skip**:
- `get_related` (Python L569-576): **사용 안 됨** (grep 결과 dead code). Go 포팅 scope 아님

### 7. `agents/scribe/record_builder.py` ✅ (2026-04-22)

**Python**: 703 LoC. RawEvent + RecordBuilder class + 17 methods + 3 pattern constants (5 SENSITIVE · 4 QUOTE · 5 RATIONALE).

**🟡 발견 (7건 — capture.md canonical reference 섹션으로 일괄 보충)**:

1. **`MAX_INPUT_CHARS = 12_000` 누락** (Python L227) — cleanText truncate 기준
2. **`_parse_domain` `customer_escalation` → CUSTOMER_SUCCESS alias** 누락 (Python L646) — 엣지케이스 없으면 general로 떨어짐
3. **SENSITIVE_PATTERNS 5개 정확한 regex** 인라인 없음 (D15 canonical-reference 방식 적용)
4. **QUOTE_PATTERNS 4개** (double/single/일본/프랑스 min 10자) 동일
5. **RATIONALE_PATTERNS 5개** 동일
6. **`_determine_certainty` 3-rule 로직** 미기술 (no evidence / no direct quote / no rationale / all satisfied)
7. **순서 제약** (consistency → render_payload_text) 명시 필요

**🟢 수정**: `spec/flows/capture.md` Phase 5에 **"🔒 Canonical reference — `record_builder.py` (D13 Option A)"** 섹션 신설
- 4 상수 표 (MAX_INPUT_CHARS · QUOTE · RATIONALE · SENSITIVE_PATTERNS) with Python line reference
- Go 포팅 매핑표 (9 methods)
- 순서 제약 명시 (ensure_consistency → render_payload_text → reusable_insight)
- golden fixture 검증 방식
- D14 agent-delegated scope 축소 (legacy regex fallback dead code 명시)

**🟢 Safe to skip (legacy regex fallback, D14)**:
- `build()` single-record legacy path
- `_extract_context` / `_extract_rationale` (regex fallback)
- `_extract_title` 2 regex patterns (agent가 title 제공 시 미사용)
- `_determine_status` acceptance_patterns (`_status_from_hint` fallback으로 존재하나 hint 있으면 미사용)

**✅ 이미 반영**:
- D13 Option A (rune-mcp 포팅)
- D14 agent-delegated (pre_extraction 필수)
- D15 render_payload_text canonical
- D16 batch embed
- phase max 7 (server.py L1275)
- `_redact_sensitive` 호출 위치 (build_phases 진입부, agent-delegated에서도 항상)
- `original_text = raw_event.text` (redact 전 원본 보존)

### 8. `agents/common/schemas/templates.py` ✅ (2026-04-22)

**Python**: 364 LoC. PAYLOAD_TEMPLATE (16 slots) + 7 `_format_*` helpers + `render_payload_text` + `render_compact_payload` + PAYLOAD_HEADERS (en/ko/ja) + `render_display_text`.

**✅ D15 canonical-reference 구조 정확 확인**: 모든 라인 참조 일치.

**🟡 line-by-line 포팅 시 놓치기 쉬운 subtle behaviors 3건** (D15에 포팅 주의사항 섹션으로 추가):

1. **phase_line / group_summary post-insertion** (L204-216): template format 이후 "ID: " line 다음에 삽입. template 자체에 포함 안 됨 → Go에서 동일 후처리 필수.

2. **Blank line collapse + trim** (L219-222): `while "\n\n\n" in text: text = text.replace(...)` + `.strip()`. 빠뜨리면 1자 diff로 embedding 공간 다름.

3. **`_format_alternatives` 빈 `chosen` 특이 동작** (L59): `"" in alt.lower()` always True → `chosen=""` 일 때 모든 alt가 "(chosen)" marker. Python 버그지만 MVP는 bit-identical 재현. **Post-MVP에 Python 패치 + Go 재동기화 예정** (`open-questions.md` Post-MVP 6).

**🟢 Post-MVP 항목 명시**:
- `render_compact_payload` (L225-238)
- `render_display_text` (L288-363)
- `PAYLOAD_HEADERS` 한국어/일본어 locale dict

**검증 방식**: golden fixture test `testdata/payload_text/golden/*.md` 50개 샘플 byte-for-byte 비교 (D15 포팅 완료 판정 기준).

### 9. `agents/common/config.py` ✅ (2026-04-22)

**Python**: 365 LoC. 7 dataclass (Vault/EnVector/Embedding/LLM/Scribe/Retriever/RuneConfig) + parsers + load/save + env var overrides + ensure_directories.

**🟢 Python → Go 의도적 축소 (v0.4 design)**:
- `envector` section → drop (Vault bundle 매 부팅 재획득, 메모리만)
- `embedding` → drop (D30 embedder 외부 책임)
- `llm` (9 fields) → drop (D14/D21/D28 agent-delegated, rune-mcp LLM 미사용)
- `scribe` (9 fields) → drop (scribe legacy 전부 제거)
- `retriever` → drop (server.py tool default 사용)
- Python 15+ env var → Go는 `RUNE_STATE` 1개만

**🟡 rune-mcp.md Config 섹션 5건 보강**:

1. **`dormant_reason` / `dormant_since` top-level 필드**: Python L225-226 저장. Go schema 예시에 포함.
2. **Directory 0700 · File 0600 permissions**: Python L358-365 `ensure_directories`. Go 파일 시스템 레이아웃 표 신설 (CONFIG_DIR/LOGS_DIR/KEYS_DIR 0700, config.json/EncKey/EvalKey/capture_log 0600).
3. **Migration 동작 명시**: Python 시대 config.json에 있는 extra sections (`embedding`, `llm`, `scribe`, `retriever`, `envector`)은 Go가 **무시** (destructive 아님, pass-through).
4. **`RUNE_STATE` env var override** 언급 추가 (optional).
5. **Write 시점**: rune-mcp는 읽기만, `/rune:configure` 는 외부 Claude Code plugin에서 쓰기. state 전환은 메모리에만.

**✅ 이미 일치**:
- vault 4 필드 (endpoint/token/ca_cert/tls_disable)
- state "active"/"dormant"
- metadata top-level (configVersion/lastUpdated/installedFrom)
- 읽기 전용 원칙

### 10. `mcp/server/errors.py` ✅ (2026-04-22)

**Python**: 118 LoC. `RuneError` base + 6 domain errors + `make_error` helper.

**🔴 발견**: Go docs 에러 code가 Python과 완전히 다른 이름 사용 중 (`VAULT_PENDING`, `DORMANT`, `embedder_unreachable`, `metadata_corrupted` 등). bit-identical 실패.

**🟢 수정 (rune-mcp.md 에러 섹션 전면 재작성)**:

- **Python 7종 bit-identical**:
  - `INTERNAL_ERROR` (base, retryable=false)
  - `VAULT_CONNECTION_ERROR` (retryable=true)
  - `VAULT_DECRYPTION_ERROR` (retryable=false)
  - `ENVECTOR_CONNECTION_ERROR` (retryable=true)
  - `ENVECTOR_INSERT_ERROR` (retryable=true)
  - `PIPELINE_NOT_READY` (retryable=false) — state != active 모든 경우 재사용
  - `INVALID_INPUT` (retryable=false)

- **Go 신규 1종**:
  - `EMBEDDER_UNREACHABLE` (retryable=true) — D30 외부 embedder gRPC 연결 실패 (Python에 없음, process-internal embedding이었으므로)

- **state-specific recovery_hint**: internal state (starting/waiting_for_vault/dormant+reason)별로 `PIPELINE_NOT_READY` hint differentiation. code는 통일.

- **metadata corruption**: Python `searcher.py:L438` "조용히 skip" 동작 재현. 별도 code 만들지 않음 (partial degrade).

- **MakeError helper**: Python `make_error` 동등. typed `RuneError` 있으면 code/retryable/recovery_hint 포함 응답 조립. 아니면 INTERNAL_ERROR fallback.

**✅ 에이전트 호환성**: Python 7종 code 유지로 기존 에이전트 프롬프트 재사용 가능.

### 11. `mcp/server/server.py` ✅ (2026-04-22, 2002 LoC)

**Python**: MCPServerApp class + 8 tool handlers + pipeline init + main().

**🟡 발견 및 수정 (4건)**:

1. **`_SensitiveFilter` 2 regex 정확 pattern 상세화** (rune-mcp.md L298): Go doc은 `pk-` 누락 + pattern 구조 모호. Python L28-31 정확한 2개 regex + 치환 규칙 (`m.group()[:8] + "***"`) 명시.

2. **`_ensure_pipelines` 120s timeout** 명시 (rune-mcp.md 타임아웃 섹션): 모든 tool call 진입부에서 백그라운드 pipeline init 대기. Python L1503-1518 기준. 초과 시 `PipelineNotReadyError` + hint 차별화.

3. **Envector credentials 캐싱 divergence 명시** (rune-mcp.md Config): Python `_init_pipelines` L1583-1591 Vault → config.json 저장 vs Go memory-only. **의도적 divergence** (보안·단순성·비용) 주석 추가.

4. **Dormant mode 동작** 명시 (rune-mcp.md Config 섹션): Python L1544-1547 bit-identical. state != active 시 pipeline init skip, read-only tool만 동작. "degraded mode" 패턴.

**✅ 이전 pass에서 검증된 항목 (재확인)**:
- 8 tool handler (vault_status/diagnostics/capture/batch_capture/recall/reload_pipelines/capture_history/delete_capture)
- `_capture_single` L1208-1407 (tier2 / novelty / record_builder flow)
- `_calculate_confidence` L393-412
- `_append_capture_log` / `_read_capture_log` (D20)
- `_set_dormant_with_reason` (state machine)
- `fetch_keys_from_vault` (Vault bundle)
- 상수: CAPTURE_LOG_PATH, ENVECTOR_DIAGNOSIS_TIMEOUT=5s, WARMUP_TIMEOUT=60s, recall topk=5/max=10

**🟢 D31 dropped (확인)**:
- `_infer_provider_from_context` L451
- `_maybe_reload_for_auto_provider` L477
- `_legacy_standard_capture` L1409

**🟢 Signal handling (Python vs Go structural 차이)**:
- Python `os.close(0) + os._exit(0)` (anyio readline thread GC quirk 대응)
- Go는 context cancellation + graceful shutdown — 동일 의도, 다른 메커니즘

### 12. Track C: `envector_client.py` + `document_preprocess.py` + `embeddings.py` ✅ (2026-04-22)

| 파일 | LoC | Scope |
|---|---|---|
| `agents/common/envector_client.py` | 220 | ❌ Go 별도 미포팅 — 얇은 wrapper. Go `internal/adapters/envector/client.go`에 직접 노출 |
| `mcp/adapter/document_preprocess.py` | 166 | ❌ Dead code — `__init__.py` export만, 실제 사용 없음. PDF/MD chunking (langchain+pypdf). Go 포팅 scope 아님 |
| `mcp/adapter/embeddings.py` | 154 | ❌ D30 embedder 외부 데몬 분리로 이관 |

### 13. Track B: D14/D21/D28/D30 제외 파일들 ✅ (2026-04-22)

**총 4016 LoC Python → Go 포팅 scope 제외** (전수 확인):

| 카테고리 | 파일 | LoC | 제외 이유 |
|---|---|---|---|
| D14 Tier 1 | `agents/scribe/detector.py` | — | agent-delegated (detector 제거) |
| D14 Tier 2 | `agents/scribe/tier2_filter.py` | 143 | agent-delegated |
| D14 Tier 3 | `agents/scribe/llm_extractor.py` | 421 | agent-delegated |
| D14 patterns | `agents/scribe/pattern_parser.py` | 423 | Tier 1 patterns |
| D14 review | `agents/scribe/review_queue.py` | 352 | legacy |
| Legacy webhook | `agents/scribe/server.py` | 576 | scribe webhook server |
| Legacy sources | `agents/scribe/handlers/*.py` | 663 | Slack/Notion handlers (base+slack+notion) |
| D28 synthesizer | `agents/retriever/synthesizer.py` | 482 | agent-delegated response (raw results only) |
| D14 LLM | `agents/common/llm_client.py` | 139 | rune-mcp LLM 미사용 |
| D14 helper | `agents/common/llm_utils.py` | 39 | LLM utility |
| D30 embedder | `agents/common/embedding_service.py` | 178 | external embedder |
| D21 language | `agents/common/language.py` | 172 | agent-side 번역 |
| D14 pattern cache | `agents/common/pattern_cache.py` | 203 | Tier 1 pattern caching |

## 최종 통계

**Python 원본 전수 검증 완료**:
- 전체 40 파일 · ~10,626 LoC
- **포팅 대상**: ~6,070 LoC (57%)
- **포팅 제외** (D14/D21/D28/D30 + Track C dead code): ~4,556 LoC (43%)

**Go 문서 수정 건수**: **46건** (11 파일에서)

**수정 분류**:
| 유형 | 건수 |
|---|---|
| 🔴 Critical (실제 구현 버그 유발) | ~12건 |
| 🟡 보충 (누락된 상세·계약·상수) | ~28건 |
| 🟢 의도적 divergence 명시 | ~6건 |

**가장 임팩트 큰 발견 Top 5**:
1. **`searcher.py` `toSearchHit` 필드 경로 버그**: `record_id` key 틀림 + `certainty` nested path 누락 + default 값 차이. 그대로 구현했으면 recall 응답 깨짐.
2. **`embedding.py` novelty class swap**: `related` ↔ `evolution` 범위 뒤바뀜. score도 `1.0 - max_similarity` inverted.
3. **`vault_client.py` DecryptMetadata 책임 경계 완전히 틀림**: Go doc "rune-mcp 복호화"라 했으나 실제 Vault가 AES 복호화. 비대칭 책임.
4. **`decision_record.py` slug 필터**: 단어 단위 vs 문자 단위 혼동.
5. **`errors.py` error code vocabulary**: Go doc이 Python과 완전히 다른 이름 사용 중 (`VAULT_PENDING`, `metadata_corrupted` 등). 7종 bit-identical 재정렬.
