# 아키텍처 — 3-프로세스 모델

Rune v0.4.0의 전체 아키텍처 개요. "왜 이 모양인가"를 먼저 설명하고 "어떻게 생겼나"를 보인 뒤 "각 프로세스가 뭘 하는가"로 내려간다.

## 왜 이렇게 바꾸는가 (문제 배경)

현재 Python 구조의 **단 하나의 본질적 문제**는 다음이다:

> 세션 하나당 Python MCP 프로세스 하나가 뜨고, 각 프로세스가 **자기만의 embedding 모델 사본(Qwen3-Embedding-0.6B)을 메모리에 올린다**.

즉 세션 N개면 모델 N개가 RAM에 복제된다. 세션이 늘면 메모리가 선형으로 증가한다. 이게 유일하게 **아키텍처 레벨에서 해결해야 할** 문제다.

그 외에:
- Python 런타임 자체 (venv/pip/shebang 자가복구)의 복잡도 — Go 정적 바이너리로 자연스레 해결
- gRPC 채널·FHE 키 복제 — 완전 제거하려면 단일 데몬이어야 하지만, 그러면 세션 격리·마이그레이션 비용 증가. **허용 가능한 trade-off**
- 세션별 cold start — 중앙화된 embedder가 모델을 미리 들고 있으면 세션의 cold start는 Vault GetPublicKey 1회 호출(밀리초 단위)로 줄어듦

따라서 Go 전환은:

1. **모델 중복 제거**를 위한 최소 변경만 한다 — 모델은 별도 프로세스(가칭 `embedder`)로 빼낸다 (이 프로젝트는 gRPC 클라이언트로만 사용)
2. **나머지는 Python 구조에 가깝게** 유지한다 — 세션별 MCP 프로세스, stdio JSON-RPC, 독립 Vault/envector 연결
3. **정책·계약은 bit-identical** — novelty 임계, AES envelope, DecisionRecord 스키마 등은 Python과 동일

## 그림 — 3-프로세스 모델

```
┌────────────────────────────────────────────────────────────────────┐
│  [사용자 머신]                                                      │
│                                                                    │
│   Claude 창 A ─stdio JSON-RPC─→ rune-mcp A ─┐                      │
│   Claude 창 B ─stdio JSON-RPC─→ rune-mcp B ─┼─┐                    │
│   Claude 창 C ─stdio JSON-RPC─→ rune-mcp C ─┘ │                    │
│                                               │                    │
│                                               │ gRPC               │
│                                               │ over unix socket   │
│                                               ↓                    │
│                                    ┌──────────────────────┐        │
│                                    │  embedder            │        │
│                                    │  (별도 프로세스)       │        │
│                                    │  (launchd/systemd)    │        │
│                                    │                      │        │
│                                    │  Embed · EmbedBatch  │        │
│                                    │  Info · Health        │        │
│                                    └──────────────────────┘        │
│                                                                    │
│                     rune-mcp 각자 독립 gRPC:                        │
│                       ├─ Vault (vault.token)                        │
│                       └─ envector (envector.api_key)                │
└──────────────────────────┬──────────────────┬──────────────────────┘
                           │                  │
                           ↓                  ↓
                    ┌─────────────┐     ┌──────────────┐
                    │  rune-Vault │     │  envector    │
                    │  (gRPC)      │     │  Cloud       │
                    │              │     │              │
                    │  키 브로커    │     │  FHE 벡터 DB  │
                    │  FHE 복호화   │     │              │
                    └─────────────┘     └──────────────┘
```

## 세 프로세스의 역할

### 1. rune-mcp (세션당 1개)

- **역할**: Python MCP의 Go 대체. Claude Code가 spawn. stdio JSON-RPC로 8개 tool 제공
- **수명**: Claude 창이 열려있는 동안 상주. 창 닫히면 stdio EOF로 종료 (Python과 동일)
- **보유 리소스 (메모리)**: Vault gRPC 연결 + envector gRPC 연결 + FHE 키 (EncKey/EvalKey/agent_dek) + per-request context
- **보유 리소스 (디스크)**: config.json 읽기만 · capture_log.jsonl append (0600)
- **모델 없음**: embedding 요청은 `embedder`에 gRPC로 호출 (D30)

### 2. embedder (외부 프로세스)

- **역할**: Qwen3-Embedding-0.6B 모델을 메모리에 상시 로드해두고 **gRPC**로 임베딩 요청 처리. 이 프로젝트의 관심사 **밖** (별도 프로세스)
- **수명·모델·런타임**: embedder 프로젝트 책임. launchd/systemd 등록 도구도 embedder 쪽이 제공
- **노출 인터페이스**: unix domain socket에서 gRPC
  - `Embed(text) → vector`
  - `EmbedBatch(texts) → embeddings`
  - `Info() → {daemon_version, model_identity, vector_dim, max_text_length, max_batch_size}`
  - `Health() → {status, uptime, total_requests}`
  - `Shutdown(grace_seconds)` — rune-mcp는 호출 안 함
- **proto 계약**: embedder 프로젝트가 정의. 실제 패키지/서비스 이름은 embedder 팀 결정 (placeholder: `embedder.v1`)
- **책임 경계**: rune-mcp는 `embedder`를 띄우지 **않음**. 이미 떠있는 데몬에 gRPC 클라이언트로만 접근. 모델 선택·런타임(llama-server 등)·모델 파일 관리 전부 embedder 내부
- 상세: `components/embedder-integration.md`

### 3. 외부 서비스 (Vault + envector)

기존 Python과 동일 서비스·동일 프로토콜:
- **Vault**: FHE 키 브로커 + 복호화. `GetPublicKey` · `DecryptScores` · `DecryptMetadata`. auth = `vault.token`
- **envector**: FHE 벡터 DB. `Insert` · `Score` · `GetMetadata` (+ 키 lifecycle `ActivateKeys` 등). auth = `envector.api_key`

이 두 서비스는 Go 전환과 무관하게 유지된다.

## 주요 흐름 (end-to-end)

### 부팅 시퀀스 (Claude 창 하나 여는 경우)

```
1. 사용자가 Claude 창 연다
2. Claude Code가 plugin.json을 보고 rune-mcp 바이너리 spawn (stdio)
3. rune-mcp 시작
   ├─ config.json 로드 (vault.endpoint, vault.token 확보 — 디스크에서)
   ├─ state = starting
   ├─ Vault.GetPublicKey(vault_token) 호출
   │    ├─ 성공: 번들 메모리 세팅 (EncKey, EvalKey, envector_endpoint, envector_api_key, agent_id, agent_dek)
   │    │       EncKey.json / EvalKey.json은 ~/.rune/keys/<key_id>/에 캐시 (재부팅 빠른 복구용)
   │    │       state = active
   │    └─ 실패: state = waiting_for_vault → exp backoff retry (1s → 60s cap)
   ├─ envector-go SDK 초기화 (client + keys + index 핸들)
   └─ MCP tool handler 등록
4. Claude에게 "ready" 신호 (MCP initialize 응답)
```

`embedder`는 **별개 프로세스로 이미 떠 있음** (embedder 프로젝트가 launchd/systemd 자동 기동 도구 제공). rune-mcp는 `embedder`의 존재를 전제하고 필요 시 gRPC 호출.

### Capture 경로

```
Claude 창에서 scribe 에이전트가 capture tool 호출
  │
  │ stdio JSON-RPC: tools/call rune_capture {text_to_embed, metadata}
  ↓
rune-mcp 프로세스
  ├─ 검증 (phases[:7], title[:60], confidence clamp)
  ├─ gRPC embedder.Embed(text_to_embed)
  │    ↓
  │  embedder가 임베딩 계산 (외부 프로세스, 모델·런타임은 embedder 책임)
  │    ↓ vector[1024]
  ├─ envector.Score(vector)로 top-1 유사도 조회 → Vault.DecryptScores(blob)
  │    → similarity 값 확보
  ├─ policy.ClassifyNovelty(similarity) → novel/related/evolution/near_duplicate
  │    └─ near_duplicate(≥0.95)면 Insert 생략하고 에러 반환
  ├─ AES envelope 생성: {"a":agent_id, "c":base64(IV||CT(metadata_json))}
  │    (rune-mcp 자체 구현, pyenvector/envector-go SDK는 opaque로 받음)
  ├─ envector.Insert(vector, envelope) → vector_id 반환
  ├─ capture_log.jsonl append (0600)
  └─ stdio 응답: {"ok":true, "record_id":"...", "novelty":"..."}
```

### Recall 경로

```
Claude 창에서 retriever 에이전트가 recall tool 호출
  │
  │ stdio JSON-RPC: tools/call rune_recall {query, topk, filters}
  ↓
rune-mcp
  ├─ policy.ParseQuery(query) → intent 분류, entity 추출, time scope, expanded_queries
  ├─ gRPC embedder.EmbedBatch(expanded[:3])
  │    → 벡터 3개 (batch)
  ├─ 병렬 (errgroup):
  │    ├─ envector.Score(vec[0]) → blob → Vault.DecryptScores → top-k 점수
  │    ├─ envector.Score(vec[1]) → ...
  │    └─ envector.Score(vec[2]) → ...
  ├─ dedup + filter
  ├─ envector.GetMetadata(refs) → envelope 배열
  ├─ 각 envelope AES 복호화 (rune-mcp 자체 · agent_dek 사용)
  ├─ policy.Rerank: (0.7×raw + 0.3×decay) × status_mul · half-life 90일
  └─ stdio 응답: {"results":[...], "confidence":..., "sources":...}
```

## 설계 원칙

1. **정책 상수는 bit-identical**: novelty 0.3/0.7/0.95, rerank 공식, half-life 90일, status mul, stop words 81개, intent 31 regex — 전부 Python과 동일. `internal/policy/`에 단일 진실 소스.

2. **세션 격리**: rune-mcp 프로세스가 세션마다 다르므로 한 세션의 panic/OOM이 다른 세션에 전파되지 않는다. Python 현재와 동일한 격리 수준.

3. **공유 리소스는 embedder 하나**: 모델은 외부 embedder가 공유 제공. Vault 연결, envector 연결, FHE 키는 세션별 독립. 공유 복잡도 최소화.

4. **Vault-delegated 복호화**: SecKey는 절대 rune-mcp 디스크에 안 둔다. Vault가 보유하고 rune-mcp는 ciphertext blob을 Vault로 RPC 보내 복호화. `DecryptScores` · `DecryptMetadata` 그대로 사용.

5. **config.json 영구 3-섹션**: `vault` + `state` + `metadata`만 디스크에. envector 자격증명 등은 매 부팅마다 Vault 번들에서 재획득 → 메모리만.

6. **재사용 가능한 embedder**: `embedder`가 노출하는 gRPC 인터페이스는 **범용**. 다른 프로젝트가 같은 소켓에 gRPC 붙이면 즉시 쓸 수 있다 (rune-mcp는 한 명의 클라이언트일 뿐).

## 메모리 모델 (정성적)

세션 N개일 때:

**Python (현재)**:
- 세션당 프로세스 1개 × (Python 런타임 + PyTorch + Qwen3 모델 + FHE 키 + gRPC 채널)
- 세션 수에 정비례로 메모리 증가

**새 Go 구조**:
- `embedder` 1개 × (외부 프로세스, 모델 + 런타임) — **세션 수와 무관 상수** (rune-mcp 프로젝트 외부 메모리)
- `rune-mcp` N개 × (Go 런타임 + FHE 키 + gRPC 채널들) — 세션 수에 비례
- 각 rune-mcp에는 모델이 없으므로 Python MCP보다 가벼움

정확한 수치는 벤치 이후 `benchmark/results.md`에 기록 (현재 문서에는 의도적으로 수치 없음). 단 구조적으로 "모델 복제 없음"은 확정.

## 상태 머신 (rune-mcp)

```
         (boot)                     (Vault OK)
starting ──────→ waiting_for_vault ──────────→ active ←──┐
                        ↑                        ↓       │
                        │                  /rune:deactivate│
                        │ (Vault 실패 시)    dormant  ─────┘
                        │                        ↑
                        └────(/rune:activate)────┘
                                                  
 (Vault exp backoff retry는 항상 백그라운드로)
```

각 상태에서 capture/recall 요청 응답:
- `starting` → 503 `{"status":"starting"}`
- `waiting_for_vault` → 503 `{"code":"VAULT_PENDING", "retry_after":..., "last_error":...}`
- `active` → 정상 처리
- `dormant` → 503 `{"code":"DORMANT","hint":"run /rune:activate"}`

## 아직 결정 안 된 항목

`open-questions.md` 참조. 주요:

- **AES envelope에 MAC 필드 추가 여부**: CTR 단독은 malleability 취약. pyenvector + envector-go 동시 업데이트 필요 (D1 Deferred)
- **Multi-MCP에서 envector `ActivateKeys` 경쟁**: 여러 rune-mcp가 동시에 같은 키를 activate 시도할 때 서버 측 "한 키만 resident" 제약과의 상호작용 (Q3, Python 실측상 server-side 멱등성 충분해 보임)
- **envector-go SDK의 `OpenKeysFromFile` 조건 완화 PR**: SecKey.json 없을 때도 Encryptor만 로드되도록 (pyenvector와 파리티)

## 참조

- 컴포넌트 세부 설계: `components/rune-mcp.md`, `components/embedder-integration.md`, `components/vault-integration.md`, `components/envector-integration.md`
- Python 코드 실측 매핑: `research/python-codebase-map.md`
- 이전 단일 데몬 설계 (과거 문서): `docs/migration/python-go-comparison.html` · `docs/runed/`
