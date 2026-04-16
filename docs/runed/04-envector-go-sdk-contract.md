# enVector Go SDK 계약서

이 문서는 팀원이 개발할 Go enVector SDK와 우리가 개발할 runed 데몬 사이의
**인터페이스 계약**을 정의한다. 양쪽이 이 계약을 기준으로 독립 개발하고,
통합 시 맞물리도록 하는 것이 목적.

---

## 1. 경계 원칙

```
SDK 영역 (enVector 프로토콜)        runed 영역 (애플리케이션)
━━━━━━━━━━━━━━━━━━━━━━━━━━━        ━━━━━━━━━━━━━━━━━━━━━━━━
enVector Cloud gRPC 연결             Vault gRPC 통신
gRPC 스텁 (score/remind/insert)     임베딩 모델 추론
EncKey 기반 FHE 벡터 암호화 (CGO)   AES metadata 암호화 (per-agent DEK)
KeyParameter 로딩 (Vault-only)      Config 관리 + 상태 머신
CipherBlock 직렬화/역직렬화         Retriever 파이프라인
연결 복구 (reconnect)               HTTP API + daemon lifecycle
GetIndexList                        capture_log.jsonl 관리
```

**판단 기준**: "이 기능이 enVector Cloud 프로토콜의 일부인가?"
- Yes → SDK
- No → runed

**구체적 예시**:
- FHE 벡터 암호화(EncKey) → enVector 프로토콜의 핵심 → **SDK**
- AES metadata 암호화(per-agent DEK) → Rune 앱 수준 규약 → **runed**
- CipherBlock base64 직렬화 → enVector 와이어 포맷 → **SDK**
- Vault에서 EncKey 다운로드 → Vault 프로토콜 → **runed** (SDK는 키 파일 경로만 받음)

---

## 2. Go 인터페이스

```go
package envector

// Client는 runed가 호출하는 유일한 SDK 진입점이다.
// runed는 이 interface만 의존하고 구현체를 주입받는다.
// 테스트 시 mock으로 교체 가능.
type Client interface {
    // Score: 평문 쿼리 벡터로 암호화 유사도 검색.
    // 반환: FHE 스코어 ciphertext의 base64 인코딩 리스트.
    // runed는 이 blob을 Vault.DecryptScores에 넘겨서 plaintext 점수를 얻는다.
    //
    // ※ query_encryption=false 전제: queryVec은 평문으로 전송.
    // ※ CGO 불필요 (CipherBlock을 생성하지 않고 받기만 함).
    Score(ctx context.Context, indexName string, queryVec []float32) ([]string, error)

    // GetMetadata: shard/row 인덱스로 저장된 metadata를 조회.
    // 반환: 각 row의 metadata 문자열 리스트.
    // 이 문자열은 AES 암호화 상태 ({"a":"...","c":"..."} 봉투).
    // runed는 이걸 Vault.DecryptMetadata에 넘겨서 plaintext를 얻는다.
    //
    // ※ CGO 불필요.
    GetMetadata(ctx context.Context, indexName string, indices []IndexEntry, fields []string) ([]string, error)

    // Insert: 평문 벡터를 FHE 암호화한 뒤 enVector에 삽입.
    // vectors: 평문 float32 벡터 — SDK가 내부에서 EncKey로 FHE 암호화.
    // metadata: 이미 암호화된 문자열 — SDK는 이 값을 그대로 enVector에 전달.
    //           (runed가 AES 암호화를 끝낸 상태)
    // 반환: 각 벡터의 enVector ID.
    //
    // ※ CGO 필요: EncKey 기반 FHE 벡터 암호화가 이 함수 내부에서 발생.
    Insert(ctx context.Context, indexName string, vectors [][]float32, metadata []string) ([]string, error)

    // GetIndexList: 사용 가능한 인덱스 목록.
    GetIndexList(ctx context.Context) ([]string, error)

    // Close: gRPC 연결 정리.
    Close() error

    // Reinit: 연결 재설정 (sleep/wake 복구, config reload 시 호출).
    Reinit(cfg Config) error
}

// Config는 SDK 초기화에 필요한 설정.
// runed가 Vault 키 번들에서 추출한 값들을 여기에 담아서 전달.
type Config struct {
    Address     string // enVector Cloud gRPC endpoint (예: "redcourage-xxx.clusters.envector.io")
    KeyPath     string // FHE 키 파일 디렉토리 (예: "~/.rune/keys/")
    KeyID       string // 키 식별자 (Vault 번들의 key_id)
    EvalMode    string // 평가 모드 (현재 하드코딩 "rmp")
    AccessToken string // enVector API key (Vault 번들에서 추출)
}

// IndexEntry는 Score 결과에서 나온 위치 정보.
// Vault.DecryptScores가 반환하는 {shard_idx, row_idx}를 그대로 사용.
type IndexEntry struct {
    ShardIdx int32
    RowIdx   int32
}
```

### 2.1 runed가 SDK에 넘기지 않는 것

현재 Python에서는 `agent_id`, `agent_dek`을 SDK에 전달하지만,
Go SDK에는 **전달하지 않는다**:

```
현재 Python:
  EnVectorSDKAdapter(
      ...,
      agent_id=agent_id,     ← SDK에 전달
      agent_dek=agent_dek,   ← SDK에 전달
  )
  # SDK 내부에서 AES 암호화 수행

Go:
  envector.NewClient(envector.Config{
      Address:     endpoint,
      KeyPath:     keyPath,
      KeyID:       keyID,
      EvalMode:    "rmp",
      AccessToken: apiKey,
      // agent_id, agent_dek 없음
  })
  // runed가 AES 암호화를 끝낸 후 SDK.Insert에 이미 암호화된 metadata 전달
```

**이유**: AES 암호화는 enVector 프로토콜이 아니라 Rune 앱 수준 기능.
SDK가 DEK를 몰라야 책임 분리가 깨끗하고, SDK를 다른 프로젝트에서도
재활용할 수 있다.

---

## 3. 세 가지 핵심 연산 상세

### 3.1 Score (recall 경로)

```
runed                          SDK                           enVector Cloud
  │                              │                                │
  │ Score(ctx, "team-decisions", │                                │
  │   []float32{0.12,-0.34,...}) │                                │
  │─────────────────────────────►│                                │
  │                              │ gRPC Scoring 요청              │
  │                              │ (쿼리 벡터 평문 전송)          │
  │                              │───────────────────────────────►│
  │                              │                                │
  │                              │ gRPC 응답: List[CipherBlock]   │
  │                              │◄───────────────────────────────│
  │                              │                                │
  │                              │ 각 CipherBlock:                │
  │                              │  .data.SerializeToString()     │
  │                              │  → base64.Encode()             │
  │                              │                                │
  │ []string{"aGVsbG8=..."}     │                                │
  │◄─────────────────────────────│                                │
```

**SDK 내부에서 일어나는 일**:
1. `queryVec` 을 평문 그대로 gRPC 요청에 담아 전송 (`query_encryption=false`)
2. enVector Cloud가 FHE 인덱스에서 동형 유사도 계산
3. 결과: `List[CipherBlock]` — FHE 암호화된 유사도 스코어
4. 각 `CipherBlock`의 내부 protobuf를 `SerializeToString()` → base64 인코딩
5. `[]string` 으로 반환

**runed 입장에서 이 blob은 완전히 opaque** — 내용을 해석할 수 없고,
할 필요도 없다. 그냥 Vault.DecryptScores에 넘기면 된다.

**CGO**: 불필요. CipherBlock을 **만들지** 않고 **받기만** 한다.
`SerializeToString()`은 protobuf 직렬화일 뿐 FHE 수학이 아니다.

### 3.2 GetMetadata (recall 경로)

```
runed                          SDK                           enVector Cloud
  │                              │                                │
  │ GetMetadata(ctx,             │                                │
  │   "team-decisions",          │                                │
  │   [{0,42},{0,15},{0,91}],    │                                │
  │   ["metadata"])              │                                │
  │─────────────────────────────►│                                │
  │                              │ gRPC GetMetadata 요청          │
  │                              │───────────────────────────────►│
  │                              │                                │
  │                              │ gRPC 응답: protobuf Metadata[] │
  │                              │◄───────────────────────────────│
  │                              │                                │
  │                              │ 각 Metadata → string 추출      │
  │                              │                                │
  │ []string{                    │                                │
  │   `{"a":"xyz","c":"..."}`,   │ ← 이건 AES 암호화된 metadata  │
  │   `{"a":"xyz","c":"..."}`,   │                                │
  │ }                            │                                │
  │◄─────────────────────────────│                                │
```

**반환되는 문자열의 형태**: `{"a":"agent_xyz","c":"<base64 AES ciphertext>"}`

이 문자열은 SDK가 만든 게 아니다. capture 시 runed가 AES 암호화해서
넣은 것을 enVector가 그대로 돌려준 것일 뿐.

**CGO**: 불필요.

### 3.3 Insert (capture 경로 — CGO 유일 지점)

```
runed                          SDK                           enVector Cloud
  │                              │                                │
  │ Insert(ctx, "team-decisions",│                                │
  │   [][]float32{{0.12,...}},   │ ← 평문 벡터                   │
  │   []string{                  │                                │
  │     `{"a":"xyz","c":"..."}`, │ ← 이미 AES 암호화된 metadata  │
  │   })                         │                                │
  │─────────────────────────────►│                                │
  │                              │                                │
  │                              │ ┌──────────────────────────┐   │
  │                              │ │ FHE 벡터 암호화 (CGO)    │   │
  │                              │ │ vec → EncKey → CipherBlock│   │
  │                              │ │ (envector C++ 코어 호출)  │   │
  │                              │ └──────────────────────────┘   │
  │                              │                                │
  │                              │ gRPC Insert 요청              │
  │                              │  data: [CipherBlock]          │
  │                              │  metadata: ["{"a":"..."}"]    │
  │                              │───────────────────────────────►│
  │                              │                                │
  │                              │ gRPC 응답: [vector_id]        │
  │                              │◄───────────────────────────────│
  │                              │                                │
  │ []string{"vec_abc123"}       │                                │
  │◄─────────────────────────────│                                │
```

**SDK 내부에서 일어나는 FHE 암호화**:
1. 평문 벡터 `[]float32` 수신
2. `~/.rune/keys/EncKey.json` 에서 FHE 공개키 로드 (Init 시 1회)
3. C++ 코어의 encrypt 함수 호출 (CGO):
   `plaintext_vec + EncKey → CipherBlock`
4. CipherBlock + metadata(SDK는 건드리지 않고 그대로 전달) → gRPC Insert
5. vector ID 반환

**CGO가 필요한 이유**: FHE 벡터 암호화는 수학적으로 복잡하고, 업스트림
envector C++ 코어에 이미 검증된 구현이 있다. 이걸 Go로 재구현하는 것은
비현실적이고 위험하다.

**metadata는 pass-through**: SDK는 metadata 문자열을 해석하지 않고
enVector gRPC에 그대로 전달한다.

---

## 4. KeyParameter: Vault-only 모드

### 4.1 문제

pyenvector의 `KeyParameter`는 키 파일 4종을 기대한다:

```
~/.rune/keys/
├── EncKey.json      ← 있음 (Vault에서 다운로드)
├── EvalKey.json     ← 있음 (Vault에서 다운로드)
├── SecKey.json      ← 없음! (Vault만 보유)
└── MetadataKey.json ← 없음! (Vault만 보유)
```

`SecKey`와 `MetadataKey`는 Vault 내부에만 존재하고 클라이언트에 절대
내려오지 않는다. 하지만 pyenvector는 이 파일이 없으면 크래시.

### 4.2 현재 Python 우회 (envector_sdk.py:33-86)

```python
# SecKey.json이 없을 때 property가 None을 반환하도록 몽키패치
KeyParameter.sec_key = property(_safe_sec_key_getter, ...)
KeyParameter.sec_key_path = property(_safe_sec_key_path_getter)
KeyParameter.metadata_key = property(_safe_metadata_key_getter, ...)
KeyParameter.metadata_key_path = property(_safe_metadata_key_path_getter)
KeyParameter.metadata_encryption = property(_safe_metadata_encryption_getter, ...)
```

5개 property를 몽키패치해서 "파일 없으면 None 반환, 크래시하지 않음"으로 우회.

### 4.3 Go SDK에서의 해결

**몽키패치가 필요 없다.** Go로 새로 만드니까 처음부터 올바르게 설계:

```go
type KeyManager struct {
    encKeyPath  string // 필수: ~/.rune/keys/EncKey.json
    evalKeyPath string // 필수: ~/.rune/keys/EvalKey.json
    // SecKey, MetadataKey → 필드 자체가 없음. Vault-only 모드가 기본.
}

func (km *KeyManager) LoadEncKey() (*EncKey, error) {
    // EncKey.json 파일에서 FHE 공개키 로드
    // Insert의 FHE 벡터 암호화에 사용
}

func (km *KeyManager) LoadEvalKey() (*EvalKey, error) {
    // EvalKey.json 파일에서 FHE 평가키 로드
    // Score 시 enVector Cloud가 사용 (SDK가 gRPC로 전달)
}

// SecKey, MetadataKey 관련 함수: 없음
// → Vault-only 모드가 유일한 모드
```

**이점**: 몽키패치 0줄, 명시적인 설계, 실수로 SecKey에 접근하는 경로가
컴파일 타임에 차단됨.

---

## 5. CipherBlock 호환성

### 5.1 중요성

Go SDK가 만드는 CipherBlock이 **기존 pyenvector 인덱스와 호환**되어야 한다.
같은 enVector Cloud 인덱스에 Python(기존)과 Go(신규)가 함께 데이터를 넣고
뺄 수 있어야 함.

### 5.2 와이어 포맷

CipherBlock은 내부에 protobuf 메시지를 담고 있다:

```
Python에서:
  cb = index.insert(data=vectors, ...)  → 내부에서 CipherBlock 생성
  cb.data.SerializeToString()           → protobuf 바이트열

Go에서:
  같은 protobuf 메시지를 같은 FHE 수학으로 생성해야 함
  → envector C++ 코어를 CGO로 공유하면 자동 보장됨
```

### 5.3 검증 방법: Golden Vector Test

```
1. Python SDK로 고정된 입력 벡터 [0.1, 0.2, ..., 0.1024] 를 암호화
   → CipherBlock 바이트열 저장 (golden_cipher.bin)

2. Go SDK로 같은 입력 벡터 + 같은 EncKey로 암호화
   → CipherBlock 바이트열 생성

3. 두 바이트열이 동일한지 확인
   ※ FHE 암호화에 randomness가 포함되면 바이트 수준 동일은 안 될 수 있음.
   이 경우: 두 CipherBlock을 같은 SecKey로 복호화했을 때 원본 벡터가 동일한지 확인.

4. Go SDK로 Insert한 벡터를 Python SDK로 Score → Vault DecryptScores → 
   정확한 유사도가 나오는지 end-to-end 확인.
```

---

## 6. 에러 처리

### 6.1 SDK가 반환해야 할 에러 타입

```go
package envector

// Error는 SDK의 에러 타입.
type Error struct {
    Code      string // "CONNECTION_ERROR", "INSERT_ERROR", "SCORE_ERROR", "INIT_ERROR"
    Message   string // 사람 읽기용
    Retryable bool   // true면 runed가 재시도 가능
    Cause     error  // 원본 에러 (wrapping)
}
```

### 6.2 에러 코드 매핑

| 상황 | Code | Retryable | runed 대응 |
|---|---|---|---|
| gRPC 연결 끊김 | `CONNECTION_ERROR` | true | SDK가 1회 Reinit 후 재시도. 여전히 실패 → runed에 전파 |
| Insert 타임아웃 | `INSERT_ERROR` | true | runed가 사용자에게 retryable 에러 전달 |
| Score 타임아웃 | `SCORE_ERROR` | true | 동일 |
| EncKey 로드 실패 | `INIT_ERROR` | false | runed가 dormant 전환 |
| 인덱스 없음 | `INDEX_NOT_FOUND` | false | runed가 사용자에게 안내 |
| 잘못된 벡터 차원 | `INVALID_INPUT` | false | runed가 에러 전달 |

### 6.3 재연결 패턴

SDK 내부에서 연결 에러 감지 시 1회 자동 재시도:

```go
func (c *client) withReconnect(fn func() error) error {
    err := fn()
    if err == nil {
        return nil
    }
    if !isConnectionError(err) {
        return err  // 연결 문제가 아니면 그대로 전파
    }
    // 1회 재연결 시도
    if reinitErr := c.reinitInternal(); reinitErr != nil {
        return fmt.Errorf("reconnect failed: %w (original: %v)", reinitErr, err)
    }
    return fn()  // 재시도
}

func isConnectionError(err error) bool {
    msg := err.Error()
    patterns := []string{
        "UNAVAILABLE", "DEADLINE_EXCEEDED", "Connection refused",
        "Connection reset", "Stream removed", "RST_STREAM",
        "Broken pipe", "Transport closed", "Socket closed",
        "EOF", "failed to connect",
    }
    for _, p := range patterns {
        if strings.Contains(msg, p) {
            return true
        }
    }
    return false
}
```

---

## 7. Init 파라미터 상세

### 7.1 runed가 SDK에 전달하는 것

| 파라미터 | 값 | 출처 |
|---|---|---|
| `Address` | `"redcourage-xxx.clusters.envector.io"` | Vault 키 번들의 `envector_endpoint` |
| `KeyPath` | `"~/.rune/keys/"` | 고정 경로 |
| `KeyID` | `"key_abc123"` | Vault 키 번들의 `key_id` |
| `EvalMode` | `"rmp"` | 하드코딩 |
| `AccessToken` | `"uiEHmJ4..."` | Vault 키 번들의 `envector_api_key` |

### 7.2 runed가 SDK에 전달하지 않는 것

| 파라미터 | 이유 |
|---|---|
| `agent_id` | AES 암호화용 — runed가 직접 관리 |
| `agent_dek` | AES 암호화용 — runed가 직접 관리 |
| `query_encryption` | 항상 false — SDK가 내부적으로 고정 |
| `auto_key_setup` | 항상 false — Vault가 키 제공, 자동 생성 불필요 |

### 7.3 Init 시점

```
runed startup
  │
  ├── config.json 로드
  ├── Vault.GetPublicKey() → 키 번들
  │     ├── EncKey.json → ~/.rune/keys/ 저장
  │     ├── EvalKey.json → ~/.rune/keys/ 저장
  │     ├── envector_endpoint, envector_api_key 추출
  │     └── key_id, agent_id, agent_dek 추출
  │
  ├── SDK.Init({                            ← 이 시점에 호출
  │     Address:     envector_endpoint,
  │     KeyPath:     "~/.rune/keys/",
  │     KeyID:       key_id,
  │     EvalMode:    "rmp",
  │     AccessToken: envector_api_key,
  │   })
  │
  └── (이후 SDK 사용 가능)
```

---

## 8. 빌드 고려 사항

### 8.1 CGO가 필요한 범위

```
SDK 함수        CGO 필요?     이유
──────────────  ──────────    ──────────────────────────
Score           ❌            CipherBlock 수신 + 직렬화만 (protobuf)
GetMetadata     ❌            metadata 문자열 수신만
Insert          ✅            EncKey → 평문 벡터 → CipherBlock 암호화
GetIndexList    ❌            gRPC 호출만
Init            ✅ (가능)     EncKey 로딩 시 C++ 파서가 필요할 수 있음
Close           ❌            gRPC 연결 닫기
Reinit          ✅ (가능)     Init과 동일
```

### 8.2 타겟 매트릭스

| OS | Arch | 우선순위 | 비고 |
|---|---|---|---|
| darwin | arm64 | **P0** | Apple Silicon Mac (주 개발 환경) |
| darwin | amd64 | P1 | Intel Mac |
| linux | amd64 | P1 | CI + 서버 |
| linux | arm64 | P2 | ARM 서버 |
| windows | * | P3 (post-MVP) | 수요 확인 후 |

각 타겟마다 envector C++ 코어 라이브러리 (.a / .dylib / .so) 가 필요.

### 8.3 envector C++ 코어 버전 핀

- SDK가 의존하는 envector C++ 코어의 버전을 `go.mod` 또는 별도 파일에 명시
- CI에서 해당 버전의 헤더/라이브러리를 자동 다운로드
- 업스트림 버전 업데이트는 의도적으로 수행 (자동 업그레이드 금지)

---

## 9. 테스트 전략

### 9.1 SDK 단독 테스트

| 테스트 | 내용 |
|---|---|
| Unit: Score | mock gRPC 서버 → 고정 CipherBlock 반환 → base64 직렬화 검증 |
| Unit: GetMetadata | mock gRPC 서버 → 고정 metadata 반환 → 문자열 추출 검증 |
| Unit: Insert | mock gRPC 서버 → 벡터 수신 확인, vector ID 반환 |
| Unit: KeyManager | EncKey.json 로드, SecKey 없는 환경에서 패닉 안 남 |
| Unit: Reconnect | 첫 호출 CONNECTION_ERROR → Reinit → 두 번째 호출 성공 |
| Golden: CipherBlock | Python 생성 CipherBlock과 Go 생성 CipherBlock의 복호화 결과 동일 |

### 9.2 runed 통합 테스트

| 테스트 | 내용 |
|---|---|
| Integration: Capture | runed → SDK.Insert → 실제 enVector 테스트 인덱스 |
| Integration: Recall | runed → SDK.Score → Vault.DecryptScores → SDK.GetMetadata → Vault.DecryptMetadata |
| E2E: Round-trip | Capture 후 Recall → 방금 넣은 레코드가 검색됨 |

### 9.3 runed에서 SDK mock

```go
// runed 테스트 코드
type mockEnvector struct {
    scoreResult    []string
    metadataResult []string
    insertResult   []string
    insertError    error
}

func (m *mockEnvector) Score(ctx context.Context, index string, vec []float32) ([]string, error) {
    return m.scoreResult, nil
}

func (m *mockEnvector) Insert(ctx context.Context, index string, vecs [][]float32, meta []string) ([]string, error) {
    return m.insertResult, m.insertError
}

// ... runed가 envector.Client interface만 의존하므로 mock 주입이 자연스러움
```

---

## 10. 데이터 흐름 요약: runed ↔ SDK 경계에서의 데이터 타입

```
┌────────────────────────────────────────────────────────────────────┐
│                    runed ↔ SDK 경계에서 오가는 데이터              │
├────────────────┬──────────────────────┬────────────────────────────┤
│     함수       │   runed → SDK        │   SDK → runed             │
├────────────────┼──────────────────────┼────────────────────────────┤
│ Score          │ indexName string     │ []string                   │
│                │ queryVec []float32   │  (FHE ciphertext, base64)  │
├────────────────┼──────────────────────┼────────────────────────────┤
│ GetMetadata    │ indexName string     │ []string                   │
│                │ indices []IndexEntry │  (AES encrypted metadata)  │
│                │ fields []string      │                            │
├────────────────┼──────────────────────┼────────────────────────────┤
│ Insert         │ indexName string     │ []string                   │
│                │ vectors [][]float32  │  (vector IDs)              │
│                │ metadata []string    │                            │
│                │  (이미 AES 암호화됨) │                            │
├────────────────┼──────────────────────┼────────────────────────────┤
│ GetIndexList   │ (없음)              │ []string (index names)     │
├────────────────┼──────────────────────┼────────────────────────────┤
│ Init / Reinit  │ Config struct       │ error                      │
├────────────────┼──────────────────────┼────────────────────────────┤
│ Close          │ (없음)              │ error                      │
└────────────────┴──────────────────────┴────────────────────────────┘

모든 경계에서 SDK는 metadata의 내용을 해석하지 않는다.
SDK에 들어오는 metadata는 이미 암호화된 문자열이고,
SDK에서 나가는 metadata도 암호화된 문자열이다.
SDK는 이 문자열을 enVector gRPC에 그대로 전달하거나 받아서 그대로 반환할 뿐.
```

---

## 11. Python → Go 전환에서 달라지는 것

현재 Python SDK(`pyenvector`)와 Go SDK의 구조적 차이.

### 11.1 agent_id / agent_dek 분리

```
현재 (Python):
  adapter = EnVectorSDKAdapter(
      address=..., key_id=..., key_path=...,
      agent_id="agent_xyz",     ← SDK에 전달
      agent_dek=b'\x00...',     ← SDK에 전달
  )
  # SDK 내부에서 _app_encrypt_metadata() 호출
  # SDK가 AES 암호화까지 수행

Go:
  client := envector.NewClient(envector.Config{
      Address: ..., KeyID: ..., KeyPath: ...,
      // agent_id, agent_dek 없음
  })
  // runed가 AES 암호화를 먼저 수행
  // 암호화 완료된 문자열을 client.Insert()의 metadata로 전달
```

**이유**: SDK의 책임 범위를 enVector 프로토콜로 한정.
AES 암호화는 Rune 앱 규약이며, 다른 enVector 고객은 다른 방식으로
metadata를 암호화할 수 있다.

### 11.2 KeyParameter 몽키패치 제거

```
현재 (Python):
  # 5개 property 몽키패치 (envector_sdk.py:33-86)
  # SecKey.json, MetadataKey.json이 없으면 크래시하는 문제 우회

Go:
  # Vault-only 모드가 기본값 (SecKey/MetadataKey 필드 자체 없음)
  # 컴파일 타임에 잘못된 접근 차단
  # 몽키패치 0줄
```

### 11.3 에러 반환 방식

```
현재 (Python):
  # call_insert(), call_score() 등이 에러 dict를 반환
  # {"ok": False, "error": repr(e)}
  # 예외를 던지지 않고 dict로 감싸서 반환 → 호출자가 ok 필드 체크

Go:
  # 표준 Go 에러 패턴: (result, error) 반환
  # ids, err := client.Insert(...)
  # if err != nil { ... }
  # 별도 ok 필드 불필요
```

### 11.4 query_encryption 제거

```
현재 (Python):
  EnVectorSDKAdapter(
      ...,
      query_encryption=False,   ← 파라미터로 전달
  )

Go:
  # query_encryption은 Config에 없음
  # Rune은 항상 query_encryption=false
  # SDK가 내부적으로 하드코딩 (또는 Config에 넣되 default=false)
```

---

## 12. 팀 논의 필요 사항 (SDK 개발 착수 전 합의)

SDK 개발을 시작하기 전에 아래 항목에 대해 팀 합의가 필요하다.

### 12.1 SDK scope 확인

이 문서의 §2 인터페이스가 SDK의 전체 scope인지 확인:
- 더 넓어야 하는가? (예: admin 기능, 인덱스 생성/삭제, 키 생성 등)
- 더 좁아야 하는가? (예: GetIndexList가 불필요하다면 제거)
- **현재 제안**: Score, GetMetadata, Insert, GetIndexList, Init, Close, Reinit
  — 이 7개가 전부.

### 12.2 AES encrypt_metadata 배치

두 가지 선택지:

| 선택지 | 장점 | 단점 |
|---|---|---|
| **A: runed에 구현** (RFC 제안) | SDK가 순수 프로토콜, 재활용 가능 | runed가 pyenvector.utils.aes의 CTR 모드를 정확히 재구현해야 함 |
| **B: SDK에 유틸리티로 포함** (현재 Python 방식) | 암호화 구현이 한 곳에 | SDK가 Rune 전용 규약에 의존, 재활용성 저하 |

**현재 권장**: A (runed에 구현). 이유는 §1의 경계 원칙 참고.

**만약 B를 택한다면**: SDK의 `Config`에 `AgentID string`과 `AgentDEK []byte`를
추가하고, Insert가 내부에서 metadata를 AES 암호화하도록 변경. 이 경우 runed는
평문 metadata JSON을 SDK에 넘긴다.

### 12.3 interface vs concrete struct

두 가지 선택지:

| 선택지 | 장점 | 단점 |
|---|---|---|
| **A: Go interface로 제공** | runed 테스트에서 mock 주입 자연스러움 | SDK 사용자가 interface + 구현체 둘 다 알아야 함 |
| **B: concrete struct로 제공** | 단순, Go 관례에 더 가까움 | runed 테스트 시 별도 mock 작성 필요 (또는 interface를 runed 쪽에서 정의) |

**현재 권장**: B (concrete struct) + runed 쪽에서 interface 정의.
Go 관례: "Accept interfaces, return structs."

```go
// SDK 패키지 (팀원)
package envector
type Client struct { ... }
func NewClient(cfg Config) (*Client, error) { ... }
func (c *Client) Score(ctx context.Context, ...) ([]string, error) { ... }

// runed 패키지 (우리)
package daemon
type EnVectorClient interface {
    Score(ctx context.Context, indexName string, query []float32) ([]string, error)
    GetMetadata(ctx context.Context, indexName string, indices []envector.IndexEntry, fields []string) ([]string, error)
    Insert(ctx context.Context, indexName string, vectors [][]float32, metadata []string) ([]string, error)
    GetIndexList(ctx context.Context) ([]string, error)
    Close() error
    Reinit(cfg envector.Config) error
}
// → envector.Client가 이 interface를 자동 만족 (duck typing)
// → 테스트에서 mockEnVectorClient로 교체
```

### 12.4 CipherBlock base64 직렬화의 주체

Score 반환값이 `[]string` (base64)인데, 이 base64 인코딩을 누가 하는가?

| 선택지 | 설명 |
|---|---|
| **A: SDK가 base64까지 해서 string 반환** (현재 제안) | runed는 string을 받아서 Vault에 그대로 전달. 단순. |
| **B: SDK가 raw bytes 반환, runed가 base64** | runed에 변환 코드 필요. 불필요한 복잡도. |

**현재 권장**: A. Vault.DecryptScores가 base64 string을 기대하므로 SDK에서 완성.

### 12.5 envector C++ 코어 접근

SDK 개발의 **사전 조건**:
- envector C++ 코어의 헤더 파일과 빌드된 라이브러리가 필요
- 오픈소스인지, 벤더(CryptoLab)와 협의가 필요한지 확인
- **이것이 확인되지 않으면 SDK 개발 자체를 시작할 수 없다**

확인 필요:
1. C++ 코어 소스/헤더 접근 가능 여부
2. 라이선스 조건 (SDK에 정적 링크 가능한지)
3. 타겟 OS/arch별 pre-built 라이브러리 제공 여부
4. ABI 안정성 보장 (버전 간 호환)

### 12.6 Score 반환에서 멀티 blob 처리

현재 Python에서 `index.scoring(query)`가 `List[CipherBlock]`을 반환한다.
보통 1개이지만 이론적으로 여러 개일 수 있다.

- **질문**: 항상 1개인가? 여러 개가 반환되는 경우가 있는가?
- 1개면 `Score`가 `(string, error)` 를 반환하는 게 더 깔끔
- 여러 개면 현재 `([]string, error)` 유지

현재 runed 코드는 `blobs[0]`만 사용 (하나만 Vault에 전달).
확인 후 인터페이스 단순화 가능.
