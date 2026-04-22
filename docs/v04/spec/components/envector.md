# envector 통합 — envector-go SDK 채택

벡터 저장·검색. 각 `rune-mcp`가 `github.com/CryptoLabInc/envector-go-sdk`를 import해서 세션별 독립 Client·Keys·Index 인스턴스를 관리. Python `mcp/adapter/envector_sdk.py`의 monkey-patch 포함한 기능을 SDK 정식 API + rune-mcp 자체 AES envelope으로 분담.

## 역할

- FHE 암호화된 벡터의 저장·검색
- metadata(AES envelope) 저장·조회. SDK는 metadata를 **opaque string**으로 취급
- 서버에 키 등록·resident 관리 (`ActivateKeys`)

**rune-mcp가 직접 처리하는 것** (SDK 바깥):
- AES-256-CTR envelope 생성·복호화 — SDK는 metadata string을 그대로 전달
- FHE 복호화 — `Keys.Decrypt` 사용 안 함. ciphertext blob을 Vault로 전달

## Vault-delegated 보안 모델

기존 Python rune은 pyenvector의 `KeyParameter` 5개 property를 런타임에 monkey-patch해서 "SecKey 없어도 Encrypt 가능" 모드를 성립시킨다. Go에서는 monkey-patch가 언어적으로 불가능하므로 **envector-go SDK에 조건 완화 PR** 을 제출해 정식 지원 추가. 자세한 배경·근거는 Q4 (`overview/open-questions.md`).

**요청할 SDK 패치 (방법 1)** — `keys.go`의 `OpenKeysFromFile` 내부 변경:

```go
// 기존 (SecKey.json 필수)
secBytes, err := os.ReadFile(filepath.Join(o.Path, secKeyFile))
if err != nil { return nil, err }
dec, err := p.NewDecryptor(ckks, secBytes)

// 변경 후 (SecKey.json optional)
var dec crypto.Decryptor
secPath := filepath.Join(o.Path, secKeyFile)
if _, err := os.Stat(secPath); err == nil {
    secBytes, err := os.ReadFile(secPath)
    if err != nil { _ = enc.Close(); _ = ckks.Close(); return nil, err }
    dec, err = p.NewDecryptor(ckks, secBytes)
    if err != nil { _ = enc.Close(); _ = ckks.Close(); return nil, err }
}
// SecKey.json 부재 → dec == nil → Vault-delegated 모드
```

+ `Keys.Decrypt` 방어:
```go
func (k *Keys) Decrypt(blob []byte) (scores [][]float64, shardIdx []int32, err error) {
    if k == nil || k.closed { return nil, nil, ErrKeysClosed }
    if k.dec == nil { return nil, nil, ErrDecryptorUnavailable }
    return k.dec.DecryptScore(blob)
}
```

+ `errors.go`:
```go
var ErrDecryptorUnavailable = errors.New(
    "envector: decryptor unavailable (Keys opened without SecKey; use vault-delegated decrypt path)",
)
```

총 약 10줄 + 테스트 1. Non-breaking (SecKey 있는 기존 사용자는 동일 동작). 머지 예상 2-5일.

임시 fallback (SDK PR 1주+ 지연 시): `~/.rune/keys/<key_id>/SecKey.json`에 `{}` 빈 파일 작성. mock backend는 통과하나 libevi 실물에선 깨질 가능성 → Phase 2 전 반드시 제거.

## rune-mcp에서의 사용 흐름

### 초기화 (rune-mcp 부팅 시)

```go
client, err := envector.NewClient(ctx,
    envector.WithAddress(bundle.EnvectorEndpoint),
    envector.WithAccessToken(bundle.EnvectorAPIKey),
)
// SDK는 연결 lazy (첫 RPC에서 dial)

keys, err := envector.OpenKeysFromFile(
    envector.WithKeyPath("~/.rune/keys/" + bundle.KeyID),
    envector.WithKeyID(bundle.KeyID),
    envector.WithKeyPreset("FGb"),
    envector.WithKeyEvalMode("rmp"),
    envector.WithKeyDim(1024),
    // Python 대응: auto_key_setup=False (rune은 Vault에서 키 받으므로 자동 생성 끔)
    envector.WithAutoKeySetup(false),
)
// 조건 완화 PR 머지 후에는 SecKey.json 없어도 OK
defer keys.Close()

// 서버에 키 resident
if err := client.ActivateKeys(ctx, keys); err != nil {
    // Q3: multi-MCP 경쟁 시 여기서 에러 가능. 상세는 아래
}

idx, err := client.Index(ctx,
    envector.WithIndexName(bundle.IndexName),
    envector.WithIndexKeys(keys),
    envector.WithIndexDim(1024),
)
// idx 객체를 rune-mcp 수명 동안 재사용
```

### Capture 경로 (batch 지원 · Python `envector_sdk.py:L236-260` bit-identical)

```go
// Step 1: rune-mcp가 각 record의 metadata를 AES envelope로 암호화
// Python envector_sdk.py:L253 동작: [self._app_encrypt_metadata(m) for m in metadata]
// → 리스트 전체 암호화 (multi-record capture per D16)
envelopes := make([]string, len(records))
for i, r := range records {
    metadataJSON, _ := json.Marshal(r)
    envelopes[i], _ = aesctr.Seal(bundle.AgentDEK, bundle.AgentID, metadataJSON)
}

// Safety check (Python L250-251 warning): agent_dek 있는데 agent_id 없으면 암호화 skip
if bundle.AgentDEK != nil && bundle.AgentID == "" {
    slog.Warn("agent_dek set but agent_id missing — skipping metadata encryption")
    // envelopes는 raw JSON 전달 (envector가 opaque로 저장)
}

// Step 2: vectors는 `embedder` 프로세스에서 받았다고 가정 (D30 gRPC)
result, err := idx.Insert(ctx, envector.InsertRequest{
    Vectors:  vectors,      // [][]float32, N개
    Metadata: envelopes,    // []string, N개 AES envelope
})
// result.ItemIDs: []int64 (서버 할당 ID)
```

**배치 원칙** (D16): N개 record가 있으면 `Insert` 1회 호출 (개별 N번 아님). atomicity는 D17 (조건부 가정).

### Recall 경로 (Python bit-identical, 비대칭 복호화 책임)

```go
// Step 1: Score (encrypted similarity)
blobs, err := idx.Score(ctx, queryVec)
// blobs: [][]byte CiphertextScore protos (SDK raw bytes)

// Step 2: base64 encode 후 Vault.DecryptScores 호출
// (Python envector_sdk.py:L283-284 bit-identical: base64.b64encode(serialized).decode('utf-8'))
blob0B64 := base64.StdEncoding.EncodeToString(blobs[0])
scoreEntries, err := vaultClient.DecryptScores(ctx, blob0B64, /*top_k=*/ 5)
// scoreEntries: []{shard_idx int32, row_idx int32, score float64}

// Step 3: top-k 선별 후 metadata 조회
refs := buildRefs(scoreEntries)
metas, err := idx.GetMetadata(ctx, refs, []string{"metadata"})
// metas[i].Data = 저장된 AES envelope string "{"a":...,"c":...}" (envector opaque 보관)

// Step 4: AES envelope 목록을 Vault.DecryptMetadata로 위임 복호화
// (Python searcher.py:L395-470 bit-identical — 로컬 복호화 안 함. Vault에 위임해서 audit trail 보존)
envelopes := make([]string, 0, len(metas))
for _, m := range metas {
    envelopes = append(envelopes, string(m.Data))
}
plaintextJSONs, err := vaultClient.DecryptMetadata(ctx, envelopes)
// plaintextJSONs: []string — Vault가 AES 복호화한 plaintext JSON 문자열들
// rune-mcp는 json.Unmarshal만 수행 (로컬 AES 복호화 없음)
```

> **비대칭 책임 분담** (중요): capture는 rune-mcp가 local `agent_dek`으로 직접 암호화 (Seal). recall은 Vault.DecryptMetadata에 위임. rune-mcp는 recall 경로에서 AES 복호화 하지 않음. 상세는 `spec/components/vault.md` DecryptMetadata · `spec/components/rune-mcp.md` AES envelope 섹션 참조.

## ActivateKeys · Multi-MCP 경쟁 (Q3)

envector 서버는 "한 번에 한 키만 resident" 제약. `ActivateKeys`가 4-RPC 수행:
1. `GetKeysList` — 서버 등록된 키 목록
2. 내 `keys.id` 없으면 `RegisterKeys` — EvalKey 업로드 (1MiB 청크)
3. 모든 다른 키 `UnloadKeys`
4. 내 키 `LoadKeys`

**문제**: 새 구조에서는 **세션마다 rune-mcp가 독립적으로 `ActivateKeys` 호출**. 같은 유저·같은 key_id면 결과는 동일해야 하지만:
- 프로세스 A가 자기 키 로드 중
- 프로세스 B가 동시에 같은 키 activate 시도 → 3단계에서 A가 올리는 걸 내리려는 race
- SDK의 `activationMu sync.Mutex`는 **intra-process만** 보호

**선택지** (open-questions.md Q3):
- (a) 첫 MCP만 activate, 나머지는 skip (파일 lock 조율)
- (b) 모두 호출하되 server-side 멱등성에 의존
- (c) 별도 브로커 프로세스

**다음 액션**: 실 envector 서버에서 동시 activate race test. key_id 동일 시 어떤 동작인지 확인. 결과에 따라 최소 구조로 선택.

**임시 가정 (MVP 초기)**: 대부분의 경우 사용자는 하나의 세션을 열고 짧은 간격으로 추가 세션을 여므로 race 발생 확률 낮음. 문제 관찰되면 그때 파일 lock 도입.

## GetMetadata 사전 검증

Python `envector_sdk.py:L314-324`의 `call_remind` 동작:

```python
for entry in indices:
    row_idx = entry.get("row_idx")
    if row_idx is None:
        raise ValueError("Missing required 'row_idx' in index entry: ...")
    idx_list.append({"shard_idx": entry.get("shard_idx", 0), "row_idx": row_idx})
```

Go 대응:

```go
// internal/adapters/envector/client.go
func (c *Client) buildRefs(entries []VaultScoreEntry) ([]MetadataRef, error) {
    refs := make([]MetadataRef, 0, len(entries))
    for _, e := range entries {
        // Python L316-318 bit-identical: row_idx 없으면 error
        // (shard_idx는 default 0)
        refs = append(refs, MetadataRef{
            ShardIdx: e.ShardIdx,
            RowIdx:   e.RowIdx,
        })
    }
    return refs, nil
}
```

Go에서는 `VaultScoreEntry`가 struct이므로 missing 검증이 불필요 (zero value 보장). Python에서만 dict key check 필요했던 것.

## Metadata opaque 경계

SDK 명시 (`doc.go`): *"Metadata is stored verbatim — the SDK never interprets it"*.

의미:
- `InsertRequest.Metadata []string`에 전달한 값은 바이트 그대로 저장됨
- `GetMetadata` 응답의 `Metadata[i].Data`는 저장된 바이트 그대로 반환
- **rune-mcp가 AES envelope JSON을 string으로 넣고, 꺼낼 때 parse + 복호화**. SDK는 관여 안 함

AES envelope 포맷·코드는 `spec/components/rune-mcp.md`의 "AES envelope" 섹션 참조. 결정 #4 (AES-256-CTR) + Q1 (MAC 추가).

## Build tags

SDK는 빌드 태그로 crypto provider 분기:

```
기본           → deterministic mock (cross-compile OK, wire-fidelity test용)
-tags libevi  → 실 libevi_crypto CGO 바인딩 (prod)
```

### 현재 상태 (2026-04-20)

- `third_party/evi/{darwin_amd64,darwin_arm64,linux_amd64,linux_arm64}/` 네 디렉토리 **모두 빈 상태**. PROVENANCE 파일만
- libevi 태그 구현도 모든 메서드가 `ErrCryptoUnavailable` 반환
- **Phase 2 전까지 mock backend만 사용 가능**

rune 측 대응:
- CI 기본: mock tag로 fast test
- Integration/E2E: libevi tag (바이너리 준비되면 자동 활성화)
- nightly job: libevi 빌드 시도 · 바이너리 생기면 알림

## libevi 바이너리 관리

배포 시:
- rune 릴리스 bundle에 `libevi_crypto.{dylib,so}` 포함
- 설치 스크립트가 `~/.rune/lib/`에 배치
- rune-mcp 실행 시 `LD_LIBRARY_PATH` 또는 `DYLD_LIBRARY_PATH` 또는 `-rpath` 설정

SDK의 `scripts/refresh-evi.sh`가 upstream `CryptoLabInc/evi-crypto`에서 빌드해서 SDK repo에 commit하는 구조. 이 commit SHA를 rune의 `go.mod require ... envector-go-sdk <version>`에서 pin.

## 연결·세션 독립

세션마다 rune-mcp 프로세스가 자기 `envector.Client` 관리:
- Client 내부는 gRPC `ClientConn` → 자동 keepalive
- 세션 3개 = envector 서버에 gRPC 연결 3개
- envector 서버 입장: 여러 token 동시 접속. 동일 유저 동일 token일 가능성 높음

**연결 재사용**: rune-mcp 내에서 Client·Keys·Index 객체를 **부팅 시 1회 생성**하고 이후 재사용. 매 capture마다 새로 만들지 않음 (Python에서 `ev.Index(index_name)` 매번 호출하던 패턴은 pyenvector가 cheap해서 가능. Go SDK는 `client.Index()`가 서버 lookup 수행하므로 재사용 권장).

## 에러 처리

### SDK가 정의한 typed errors

- `ErrAddressRequired`
- `ErrClientClosed`
- `ErrKeysClosed`, `ErrKeysNotFound`, `ErrKeysAlreadyExist`, `ErrKeysAlreadyActivated`, `ErrKeysRequired`
- `ErrCryptoUnavailable` (libevi 없이 libevi 태그로 빌드)
- `ErrNotImplemented`
- **`ErrDecryptorUnavailable`** (조건 완화 PR 머지 후 추가)

### rune-mcp에서의 매핑

rune 도메인 에러로 감쌈:
```go
// internal/adapters/envector/client.go
if errors.Is(err, envector.ErrKeysAlreadyActivated) {
    return &domain.Error{Code: "KEY_ACTIVATION_CONFLICT", Retryable: true, Cause: err}
}
```

### gRPC status 매핑

SDK 내부에서 gRPC 에러는 `fmt.Errorf("envector: ... : %w", err)` 형태로 감싸서 나옴. `errors.Is(err, ...)` 체크 + `gRPC status.Code()` 재검사로 retry 가능성 판단.

### Python 대비 (의도적 차이)

Python `mcp/adapter/envector_sdk.py:L89-101`에는 연결 에러 감지용 **11개 string pattern** (`CONNECTION_ERROR_PATTERNS`): `"UNAVAILABLE"`, `"DEADLINE_EXCEEDED"`, `"Connection refused"`, `"Connection reset"`, `"Stream removed"`, `"RST_STREAM"`, `"Broken pipe"`, `"Transport closed"`, `"Socket closed"`, `"EOF"`, `"failed to connect"` — 자유 텍스트 에러 메시지에서 substring matching.

**Go는 이 11 패턴을 포팅하지 않음**. 대신:
- envector-go SDK가 **typed error** 제공 (`ErrKeysNotFound` 등) → `errors.Is()`로 판정
- gRPC 에러는 `status.Code()` (`Unavailable`, `DeadlineExceeded` 등 enum) → code 비교로 판정

**이유**: Go는 SDK 수준에서 structured error가 이미 노출됨. string matching은 취약 (메시지 변경 시 깨짐). Python은 SDK가 structured error 노출 안 해서 fallback으로 string matching 했던 것. 기능 동등, **구현 전략 차이**.

## 재연결

Python의 `_with_reconnect` (`envector_sdk.py:185-196`)는 연결 끊김 감지 시 `ev.init(...)` 전체 재호출. 매우 공격적.

Go SDK는 `grpc.ClientConn` 내장 keepalive + 자동 reconnect가 있으므로 **rune 측 별도 reconnect 래퍼는 대부분 불필요**:
- gRPC 채널이 일시 장애에서 자동 복구
- `Unavailable` / `DeadlineExceeded` 시 rune-mcp의 exp backoff 2-retry만

단 **sleep/wake 직후** 같은 극단 상황에서 keepalive가 stale을 못 잡는 경우 가능. 이때만 `client.Close() + NewClient()` 재생성. 실측 후 필요 시 구현 (Part 7.5 부하/장애 시나리오 테스트 대상).

## 타임아웃

- `NewClient` dial: 3s (SDK 기본값)
- Insert: 30s (큰 배치 대비)
- Score / GetMetadata: 10s
- ActivateKeys (4-RPC): 30s (EvalKey 업로드가 수십 MB 스트리밍)

context.WithTimeout으로 각 RPC call site에 적용.

## Keys · Index 수명

### Keys

- 부팅 시 `OpenKeysFromFile` 1회
- 프로세스 종료 시 `keys.Close()` (Encryptor·Decryptor·CKKSContext 리소스 해제)
- agent_dek 같은 AES 키는 Keys와 별개 — rune-mcp가 직접 zeroize

### Index

- 부팅 시 `client.Index(...)` 1회. idempotent (서버에 없으면 생성)
- 프로세스 수명 내내 재사용
- `Drop`은 호출 안 함 (데이터 삭제는 별도 tool인 `tool_delete_capture`에서 신중히)

## 패키지 레이아웃

```
internal/adapters/envector/
├── client.go           # Client + Index 래퍼
├── keys.go             # OpenKeysFromFile + 재시도
├── aes_ctr.go          # AES envelope Seal / Open (rune 자체 구현)
├── errors.go           # rune 도메인 에러 매핑
└── client_test.go      # SDK mock backend 기반 unit test
```

## 테스트 전략

### Unit (SDK mock backend)

기본 mock은 deterministic → wire-fidelity 확인 가능:
```go
func TestInsert_RoundTrip(t *testing.T) {
    client := setupTestEnvector(t)  // mock backend
    idx := setupTestIndex(t, client)
    ids, err := idx.Insert(ctx, envector.InsertRequest{
        Vectors: [][]float32{{0.1, 0.2, ...}},
        Metadata: []string{`{"a":"agent1","c":"base64..."}`},
    })
    require.NoError(t, err)
    require.Len(t, ids.ItemIDs, 1)
}
```

mock이 deterministic이라 같은 입력 → 같은 출력. 단 mock 숫자는 실 libevi와 다름. 실 정확도 검증은 integration 테스트에서.

### Integration (`//go:build integration`)

libevi 태그로 실 staging envector 연결:
- Insert → Score → DecryptScores 왕복 → 입력 벡터와 높은 cosine similarity 확인
- metadata round-trip (AES envelope 보존 확인)
- 동시 `ActivateKeys` 경쟁 테스트 (Q3 검증)

### Contract (SDK mock backend와 real backend)

같은 테스트 케이스를 mock과 libevi 양쪽에 돌려서 API 계약 일치 확인. libevi 붙기 전엔 mock만.

### 부하 테스트

Part 7.5 benchmark plan. 동시 5 rune-mcp이 capture 뿌릴 때 envector 서버 응답 시간·에러율·ActivateKeys race 관찰.

## 제약 · 미결

- SDK 조건 완화 PR 제출·머지 (Q4) — **즉시 액션**
- libevi 바이너리 부재 (Phase 2 전까지 mock만)
- Multi-MCP `ActivateKeys` 경쟁 (Q3) — race test 필요
- AES envelope MAC 필드 (Q1) — pyenvector와 동시 릴리스
- SDK 버전 pin 정책 (commit SHA vs semver)
