# rune-embedder — 공유 임베딩 데몬

머신에 1개만 상주하며 Qwen3-Embedding-0.6B 모델을 메모리에 유지. HTTP+JSON over unix socket으로 임베딩 요청을 처리. 임베딩 모델의 메모리 복제를 제거하기 위해 도입된 **이 아키텍처 전환의 핵심 컴포넌트**.

## 존재 이유

- Python MCP가 세션당 모델을 중복 로드하던 문제를 제거한다 (공유 1개)
- 다른 프로젝트에서도 같은 HTTP endpoint로 재사용 가능하게 한다 (범용 embedding 서비스)
- rune-mcp 본체를 가볍게 유지한다

**책임이 아닌 것**:
- Vault·envector 통신 — rune-mcp 담당
- AES envelope — rune-mcp 담당
- 상태 관리·세션·권한 — rune-mcp 담당 (embedder는 stateless)

## 수명 관리

### macOS — launchd

설치 시 `~/Library/LaunchAgents/io.envector.rune-embedder.plist` 등록:
```xml
<plist><dict>
  <key>Label</key><string>io.envector.rune-embedder</string>
  <key>Program</key><string>/Users/USER/.rune/bin/rune-embedder</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>/Users/USER/.rune/logs/embedder.stdout.log</string>
  <key>StandardErrorPath</key><string>/Users/USER/.rune/logs/embedder.stderr.log</string>
</dict></plist>
```

등록/해제:
```bash
launchctl bootstrap gui/$UID ~/Library/LaunchAgents/io.envector.rune-embedder.plist   # modern, 10.11+
launchctl bootout  gui/$UID ~/Library/LaunchAgents/io.envector.rune-embedder.plist
```

(deprecated `load`/`unload` 사용 금지)

### Linux — systemd user unit

`~/.config/systemd/user/rune-embedder.service`:
```ini
[Unit]
Description=Rune embedding daemon
After=network.target

[Service]
ExecStart=%h/.rune/bin/rune-embedder
Restart=on-failure
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60
MemoryMax=1200M

[Install]
WantedBy=default.target
```

등록:
```bash
systemctl --user daemon-reload
systemctl --user enable --now rune-embedder
loginctl enable-linger $USER   # 로그아웃 후에도 유지
```

### Windows

**MVP에서 지원 안 함**. (Python rune·기존 결정 흐름 일관)

## HTTP API

### Transport

- **Unix socket**: `~/.rune/embedder.sock`
- **Permission**: 0600 (소유 유저만 접근)
- **Peer credential check**: `SO_PEERCRED` (Linux) · `LOCAL_PEERCRED`/`getpeereid` (macOS). 소켓 접속자의 uid가 파일 소유자와 동일한지 확인. 다르면 거절
- **Content-Type**: `application/json`
- **버전 prefix**: `/v1/*` (future-proof)

### Endpoints

#### GET /v1/health

```
GET /v1/health HTTP/1.1

HTTP/1.1 200 OK
{
  "ok": true,
  "model": "qwen3-embedding-0.6b",
  "dim": 1024,
  "backend": "llama-server",
  "uptime_seconds": 12345,
  "version": "0.4.0"
}
```

모델 로드 중일 때:
```
HTTP/1.1 503 Service Unavailable
{
  "ok": false,
  "status": "starting",
  "progress": "loading_model",
  "elapsed_seconds": 3.2
}
```

#### POST /v1/embed

**요청**:
```json
{
  "texts": ["PostgreSQL chosen over MongoDB", "Use Go 1.25 for runed"],
  "mode": "document"
}
```

- `texts`: 문자열 배열. 1-N개. 배치 처리
- `mode` (optional): `"document"` (기본) / `"query"` — 모델이 query/document 구분을 지원하는 경우만 의미. Qwen3-Embedding은 기본 처리

**응답 200**:
```json
{
  "vectors": [
    [0.0123, 0.0456, ...],   // 1024 floats (L2 normalized)
    [0.0789, 0.0321, ...]
  ],
  "dim": 1024,
  "model": "qwen3-embedding-0.6b"
}
```

**에러 500**:
```json
{
  "error": "tokenizer_failed",
  "message": "input text too long (max 8192 tokens)",
  "request_id": "uuid-..."
}
```

### 성능·크기 제한

- 요청 body 최대 크기: 10 MiB (text 수천 개도 수용)
- 배치 최대 텍스트 수: 32 (backend 메모리 여유에 따라 조정)
- per-request timeout: 30초
- 동시 요청: 최대 4개 (모델 inference가 CPU-heavy라 큐잉)

## 실행 엔진 (D29로 확정 — llama-server)

**결정**: llama.cpp의 `llama-server`를 공식 백엔드로 채택. 상세 근거는 `decisions.md` D29 참조.

### 구조

- `llama-server` 바이너리를 별도 프로세스로 supervise (rune-embedder의 자식)
- 모델: `qwen3-embedding-0.6b.gguf` (공식 GGUF export 존재, 필요 시 Q8 quant)
- llama-server가 OpenAI 호환 HTTP `/v1/embeddings` endpoint 제공
- rune-embedder는 **thin proxy**: unix socket 수신 → llama-server HTTP → 응답 변환 → rune 규격 응답
- 예상 Go 코드: ~150 LoC (process supervise + HTTP proxy + healthcheck)

### 배포

- `llama-server` 바이너리: `/opt/homebrew/bin/llama-server` (macOS) · 사용자가 `brew install llama.cpp` 또는 `apt install` 등으로 제공
- 모델 파일: `~/.rune/models/qwen3-embedding-0.6b.gguf`
- rune-embedder는 시작 시 바이너리 경로 + 모델 경로 검증. 미존재 시 명확한 에러 후 종료

### 폐기된 후보 — ONNX Runtime Go

다음 이유로 탈락 (D29 참조):
- CGO 의존성 → cross-build 복잡, 바이너리 배포 무거움
- Go 바인딩 성숙도가 Python 대비 낮음
- rune 전용 구현 → 다른 프로젝트에서 재사용 어려움

### 품질 기준 (구현 후 검증)

- Python sentence-transformers와 **cosine similarity ≥0.999** 달성
- 런타임 안정성: llama-server 크래시 시 rune-embedder가 자동 재기동 (throttle 10s)
- 레이턴시: localhost HTTP RTT 수 ms + forward pass 수십~수백 ms

## 모델 · 디스크

### 모델 위치

`~/.rune/models/qwen3-embedding-0.6b.{onnx,gguf}` (backend 확정 후 선택)

### 모델 배포

- `/rune:configure` 실행 시 GitHub Release 또는 HF Hub에서 다운로드
- SHA256 검증 필수 (무결성 + 손상 시 재다운로드)
- 크기: ONNX fp32 ~2GB, int8 ~500MB · GGUF Q4_K_M ~400MB, Q8 ~700MB (모두 추정치, 실측 필요)

### 모델 버전

- 현재 고정: `Qwen/Qwen3-Embedding-0.6B`
- 업그레이드 정책: 향후 결정. 모델 교체는 차원·임계값 재검증 필요

## ActivateKeys · Session isolation

**embedder는 stateless**. 세션 개념 없음.
- 모든 rune-mcp가 같은 embedder 공유
- embedding은 단순 `text → vector` 함수. session context 영향 없음
- 다만 모델 inference가 thread-safe하지 않을 수 있어 내부 뮤텍스 또는 worker pool로 직렬화 (backend에 따라 다름)

## 보안

### 소켓 보안

- 0600 퍼미션 + `SO_PEERCRED`/`getpeereid`로 uid 검증
- socket 경로: `~/.rune/embedder.sock` (사용자 홈 안, 같은 머신 다른 유저 접근 불가)

### 입력 검증

- 요청 body 크기 제한 (`http.MaxBytesReader`)
- JSON schema 검증 (texts 배열 유무·크기·문자열 타입)
- text 길이 token 수 기준 상한 (모델 context limit)

### 로그 안전

- 입력 텍스트는 **로그에 기록하지 않음**. 민감 정보 가능성. metric만 집계 (길이·개수·latency)
- 에러 시 request_id로 상관관계만

## 관측성

### Metric (Prometheus)

rune-embedder는 metric의 자연스러운 수집 지점 (모든 세션 공유):
- `rune_embedder_requests_total{endpoint,status}`
- `rune_embedder_request_duration_seconds{endpoint}` (histogram)
- `rune_embedder_batch_size` (histogram)
- `rune_embedder_model_loaded` (gauge 0/1)
- `rune_embedder_active_requests` (gauge)
- `rune_embedder_memory_bytes` (gauge)

opt-in: config로 `/metrics` 활성화 + 동일 unix socket 또는 별도 TCP (localhost 한정).

### Logging

- slog structured JSON to stderr (launchd/systemd가 파일로 수집)
- 각 요청에 request_id
- ERROR: 모델 로드 실패 · tokenize 실패 · inference 에러
- INFO: 기동 · 모델 로드 완료 · shutdown
- DEBUG (옵션): 요청별 latency 분해

### Tracing

Go 1.25 `runtime/trace.FlightRecorder` 상시 on (low-overhead ring buffer). panic/OOM/latency 초과 시 직전 trace를 `~/.rune/logs/flight-<ts>.trace`로 덤프 → `go tool trace`로 사후 분석.

## 버전 호환

- HTTP API는 `/v1/*` 네임스페이스
- response에 `"model": "qwen3-embedding-0.6b"`, `"version": "0.4.0"` 포함
- rune-mcp가 embedder version mismatch 감지하면 경고 로그 (강제 거부는 안 함 — minor ver 호환 허용)
- breaking change 시 `/v2/*` 신설 + `/v1/*` 일정 기간 유지

## 설치·배포

### `/rune:configure` 설치 단계 (embedder 부분)

```
1. 바이너리 다운로드: rune-embedder → ~/.rune/bin/
2. 모델 다운로드 + SHA256 검증: .onnx 또는 .gguf → ~/.rune/models/
3. launchd plist / systemd unit 파일 작성
4. 등록 (launchctl bootstrap / systemctl --user enable --now)
5. /v1/health 폴링 (최대 30초) → 모델 로드 완료 확인
```

### `/rune:reset` 해제 단계

```
1. 프로세스 stop (launchctl bootout / systemctl --user stop)
2. plist/unit 파일 제거
3. ~/.rune/bin/rune-embedder 바이너리 삭제
4. ~/.rune/models/ 삭제 (옵션)
5. ~/.rune/embedder.sock 파일 정리
```

## 업그레이드

- rune-embedder 바이너리 교체 → launchd/systemd에 SIGHUP 또는 재시작
- 모델 교체 → 새 모델 파일 배치 + embedder 재시작
- 재시작 중 rune-mcp 요청은 `embedder_unreachable` 수신 → exp backoff retry

## 패키지 레이아웃 (rune-embedder 한정)

```
cmd/rune-embedder/main.go
internal/
  ├── embedder/
  │   ├── server.go            # HTTP /v1/health /v1/embed
  │   ├── router.go            # net/http ServeMux (Go 1.22)
  │   ├── middleware.go        # recover · request_id · peer cred · size limit
  │   └── types.go             # EmbedRequest · EmbedResponse
  ├── backend/
  │   ├── interface.go         # type Backend { Embed(ctx, []string) ([][]float32, error) }
  │   └── llama.go             # llama-server supervisor (D29)
  ├── model/
  │   └── loader.go            # 모델 경로 검증 (llama-server가 로드 담당)
  └── obs/
      ├── slog.go
      └── metrics.go
```

## 테스트 전략

- **Unit**: types 직렬화 · 검증 로직 · tokenizer wrapper · backend mock
- **Integration**: 실 모델 로드해서 `/v1/embed` 호출. 결과와 Python sentence-transformers reference 비교 (cosine ≥0.999)
- **Performance**: 배치 크기별 latency · 메모리 사용량 측정
- **Fault injection**: 모델 파일 없음 · 권한 오류 · 모델 로드 중 재시작

## 제약 · 미결

- backend 선택 (Q2) — POC 후 확정
- 버전 호환 정책 상세 (Q6)
- socket 보안 최종 스펙 (Q7)
- 모델 크기·성능 실측은 `benchmark/` 계획 후
