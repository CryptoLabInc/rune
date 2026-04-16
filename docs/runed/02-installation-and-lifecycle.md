# 02. 설치와 라이프사이클

runed 데몬의 배포, 설치, 시작, 종료, 복구, 업그레이드 전체 과정.
구현할 때 이 문서의 순서와 경로를 그대로 따른다.

---

## 1. 배포 방식

### 1.1 바이너리 배포

Go 바이너리 두 개를 배포한다.

| 바이너리 | 역할 | 예상 크기 |
|----------|------|-----------|
| `rune` | CLI — capture, recall 등 사용자 명령 | ~15-30 MB |
| `runed` | 상주 데몬 — 임베딩, FHE 암호화, gRPC 통신 | ~30-50 MB (모델 별도) |

임베딩 모델은 바이너리에 포함하지 않는다. ~500 MB로 바이너리에 넣기엔 너무 크다.
첫 데몬 시작 시 별도 다운로드한다.

**빌드 타겟:**

| OS | Arch | 파일명 |
|----|------|--------|
| darwin | arm64 | `rune-darwin-arm64.tar.gz` |
| darwin | amd64 | `rune-darwin-amd64.tar.gz` |
| linux | amd64 | `rune-linux-amd64.tar.gz` |
| linux | arm64 | `rune-linux-arm64.tar.gz` |

**배포 채널:**

- Claude Code plugin 패키지
- Codex skill 패키지
- Gemini extension 패키지

세 채널 모두 동일한 tar.gz를 사용한다. 각 플러그인의 install hook이 바이너리를
`~/.rune/bin/`에 배치하는 방식만 다르다.

### 1.2 플러그인 install hook 동작

플러그인이 설치될 때 install hook이 실행하는 정확한 단계:

```
1. OS/arch 감지
   ├── runtime.GOOS → darwin | linux
   └── runtime.GOARCH → arm64 | amd64

2. GitHub release에서 rune-{os}-{arch}.tar.gz 다운로드
   └── URL: https://github.com/envector/rune/releases/latest/download/rune-{os}-{arch}.tar.gz

3. ~/.rune/bin/ 디렉토리 생성 (mkdir -p)

4. tar.gz 압축 해제 → ~/.rune/bin/rune, ~/.rune/bin/runed 배치

5. 실행 권한 부여
   ├── chmod +x ~/.rune/bin/rune
   └── chmod +x ~/.rune/bin/runed

6. 데몬 등록
   ├── macOS → launchd plist 생성 + launchctl load
   └── Linux → systemd user unit 생성 + systemctl --user enable

7. runed 데몬 시작
   ├── macOS → launchctl start io.envector.runed
   └── Linux → systemctl --user start runed

8. runed 첫 실행 시 자동 수행 (데몬 내부 로직)
   ├── Vault 키 번들 다운로드 (EncKey.json, EvalKey.json → ~/.rune/keys/)
   └── 임베딩 모델 다운로드 (~500MB → ~/.rune/models/)

9. 완료: 플러그인 사용 가능
```

install hook은 8단계까지 동기적으로 실행한다.
8단계(키 번들 + 모델 다운로드)는 데몬이 비동기로 처리하므로 hook 자체는 빠르게 리턴한다.
모델 다운로드가 완료되기 전에 들어온 요청에는 503 Service Unavailable을 응답한다.

### 1.3 uninstall hook 동작

```
1. runed 데몬 정지
   ├── macOS → launchctl stop io.envector.runed
   └── Linux → systemctl --user stop runed

2. 데몬 등록 해제
   ├── macOS → launchctl unload ~/Library/LaunchAgents/io.envector.runed.plist
   │          + plist 파일 삭제
   └── Linux → systemctl --user disable runed
              + unit 파일 삭제

3. ~/.rune/bin/ 삭제
   └── rm -rf ~/.rune/bin/

4. ~/.rune/sock 삭제
   └── rm -f ~/.rune/sock

5. (선택) 캐시 삭제 — 사용자 확인 후
   ├── ~/.rune/keys/ 삭제 (FHE 키 캐시 — Vault에서 재다운로드 가능)
   ├── ~/.rune/models/ 삭제 (임베딩 모델 — 재다운로드 가능)
   └── ~/.rune/logs/ 삭제

6. (선택) config 유지
   └── ~/.rune/config.json은 삭제하지 않음 (재설치 시 재사용 가능)
       사용자가 명시적으로 요청한 경우에만 삭제
```

---

## 2. 데몬 등록

### 2.1 macOS launchd

**plist 파일:** `~/Library/LaunchAgents/io.envector.runed.plist`

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.envector.runed</string>

    <key>ProgramArguments</key>
    <array>
        <string>~/.rune/bin/runed</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>~/.rune/logs/daemon.log</string>

    <key>StandardErrorPath</key>
    <string>~/.rune/logs/daemon.err</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>RUNE_CONFIG</key>
        <string>~/.rune/config.json</string>
    </dict>
</dict>
</plist>
```

**주요 속성:**

- `RunAtLoad: true` — 로그인 시 자동 시작
- `KeepAlive: true` — 크래시 시 launchd가 자동 재시작

**관리 명령어:**

```bash
# plist 등록 (install hook이 수행)
launchctl load ~/Library/LaunchAgents/io.envector.runed.plist

# plist 해제
launchctl unload ~/Library/LaunchAgents/io.envector.runed.plist

# 데몬 시작
launchctl start io.envector.runed

# 데몬 정지
launchctl stop io.envector.runed

# 상태 확인
launchctl list | grep io.envector.runed
```

**구현 시 주의:**

plist 내 `~`는 launchd가 자동 확장하지 않는 환경이 있다.
install hook에서 plist를 생성할 때 `~`를 실제 홈 디렉토리 절대 경로로 치환해서 써야 한다.

```go
home, _ := os.UserHomeDir()
plistContent = strings.ReplaceAll(plistTemplate, "~", home)
```

### 2.2 Linux systemd --user

**unit 파일:** `~/.config/systemd/user/runed.service`

```ini
[Unit]
Description=Rune Daemon
After=network.target

[Service]
ExecStart=%h/.rune/bin/runed
Environment=RUNE_CONFIG=%h/.rune/config.json
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

**주요 속성:**

- `%h` — systemd 특수 지시어, 사용자 홈 디렉토리로 확장
- `Restart=on-failure` — 비정상 종료 시 자동 재시작 (exit 0은 재시작 안 함)
- `RestartSec=5` — 재시작 전 5초 대기

**관리 명령어:**

```bash
# unit 등록 + 부팅 시 자동 시작
systemctl --user enable runed

# 데몬 시작
systemctl --user start runed

# 데몬 정지
systemctl --user stop runed

# 데몬 재시작
systemctl --user restart runed

# 상태 확인
systemctl --user status runed

# 로그 확인
journalctl --user -u runed -f
```

**구현 시 주의:**

systemd user 서비스는 로그인 세션이 없으면 실행되지 않는다.
서버 환경에서 세션 없이 실행하려면 `loginctl enable-linger $USER`가 필요하다.
install hook에서 linger를 자동으로 활성화할지는 선택사항이다.

---

## 3. Startup 시퀀스

runed 프로세스가 시작될 때 실행되는 정확한 순서:

```
runed 프로세스 시작
  │
  ├── 1. CLI 플래그 파싱
  │      ├── --config    설정 파일 경로 (기본값: ~/.rune/config.json)
  │      ├── --socket    소켓 경로 (기본값: ~/.rune/sock)
  │      └── --log-level 로그 레벨 (기본값: info)
  │
  ├── 2. 로그 초기화
  │      ├── log/slog 패키지 사용
  │      ├── 출력: ~/.rune/logs/daemon.log
  │      └── 로그 디렉토리 없으면 생성 (mkdir -p)
  │
  ├── 3. config.json 로딩
  │      ├── 파일 없음 → dormant 모드
  │      │     └── HTTP 서버만 기동, /health + /diagnostics만 응답
  │      ├── state != "active" → dormant 모드
  │      └── state == "active" → 4단계로 계속
  │
  ├── 4. Vault 키 번들 다운로드
  │      ├── GetPublicKey(token) 호출
  │      │     └── Vault gRPC → key_bundle_json 응답
  │      ├── key_bundle_json 파싱
  │      │     ├── EncKey.json → ~/.rune/keys/EncKey.json 캐시
  │      │     ├── EvalKey.json → ~/.rune/keys/EvalKey.json 캐시 (수십 MB)
  │      │     ├── agent_id 추출
  │      │     └── agent_dek 추출
  │      └── envector endpoint, api_key 추출 → config에 기록
  │
  ├── 5. enVector Go SDK 초기화
  │      ├── SDK.Init(address, keyPath, keyID, evalMode, accessToken)
  │      └── gRPC 채널 생성 + handshake
  │
  ├── 6. 임베딩 모델 로드 (가장 느린 단계)
  │      ├── 첫 실행: 모델 다운로드 (~500MB) → ~/.rune/models/
  │      │     └── 다운로드 중 /health 응답: {"status": "starting", "embed": "downloading"}
  │      ├── 이후: 디스크 캐시에서 로드 (~수 초)
  │      └── embed.Service.Ready() = true
  │
  ├── 7. Unix socket 생성
  │      ├── 기존 소켓 파일 존재 확인
  │      │     └── 있으면 삭제 (os.Remove)
  │      ├── net.Listen("unix", "~/.rune/sock")
  │      └── os.Chmod(sockPath, 0600)
  │           └── 소켓 소유자만 접근 가능
  │
  ├── 8. fsnotify watcher 시작
  │      └── ~/.rune/config.json 변경 감시
  │           └── 변경 감지 시 → config reload (5절 참조)
  │
  ├── 9. Signal handler 등록
  │      ├── SIGTERM, SIGINT → graceful shutdown (4절 참조)
  │      └── SIGHUP → config reload (5절 참조)
  │
  └── 10. HTTP 서버 시작
         ├── Unix socket 위에서 http.Serve()
         └── 요청 수신 대기 — 이 시점부터 클라이언트 요청 처리 가능
```

**dormant 모드 동작:**

config가 없거나 `state != "active"`이면 데몬은 dormant 모드로 기동한다.
이 모드에서는:

- 4-6단계를 건너뛴다 (Vault 연결, SDK 초기화, 모델 로드 안 함)
- 7-10단계는 정상 실행 (소켓 생성, HTTP 서버 기동)
- `/health`와 `/diagnostics`만 응답한다
- capture/recall 요청에는 503을 응답하며 `"runed is dormant"` 메시지를 포함한다
- fsnotify로 config 변경을 감시하다가 `state`가 `"active"`로 바뀌면 4단계부터 실행하여 active 모드로 전이한다

**타이밍 참고:**

| 단계 | 소요 시간 |
|------|-----------|
| 1-3 (파싱, 로그, config) | < 100 ms |
| 4 (키 번들 다운로드) | 1-3 초 (네트워크) |
| 5 (SDK 초기화) | < 1 초 |
| 6 (모델 로드 — 캐시) | 2-5 초 |
| 6 (모델 다운로드 — 첫 실행) | 30-120 초 (네트워크) |
| 7-10 (소켓, 서버) | < 100 ms |

---

## 4. Shutdown 시퀀스

SIGTERM 또는 SIGINT를 수신했을 때 실행되는 순서:

```
SIGTERM 또는 SIGINT 수신
  │
  ├── 1. 새 요청 수신 중단
  │      └── listener.Close() — 새 커넥션 거부
  │
  ├── 2. in-flight 요청 완료 대기
  │      ├── context.WithTimeout(ctx, 30*time.Second)
  │      ├── http.Server.Shutdown(ctx) 호출
  │      └── 30초 초과 시 강제 종료 (server.Close())
  │
  ├── 3. enVector SDK 연결 닫기
  │      └── sdk.Close()
  │
  ├── 4. Vault gRPC 채널 닫기
  │      └── vaultConn.Close()
  │
  ├── 5. 임베딩 모델 리소스 해제
  │      └── embed.Service.Close()
  │
  ├── 6. fsnotify watcher 중단
  │      └── watcher.Close()
  │
  ├── 7. 소켓 파일 삭제
  │      └── os.Remove("~/.rune/sock")
  │
  └── 8. 프로세스 종료
         └── os.Exit(0)
```

**구현 패턴:**

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

go func() {
    <-sigCh
    slog.Info("shutdown signal received")

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // 1-2. HTTP 서버 graceful shutdown
    if err := server.Shutdown(ctx); err != nil {
        slog.Error("forced shutdown", "error", err)
        server.Close()
    }

    // 3-6. 리소스 해제
    sdk.Close()
    vaultConn.Close()
    embedService.Close()
    watcher.Close()

    // 7. 소켓 파일 삭제
    os.Remove(sockPath)

    os.Exit(0)
}()
```

---

## 5. Config Reload

### 5.1 트리거

config reload이 발생하는 세 가지 경로:

| 트리거 | 발생 조건 |
|--------|-----------|
| fsnotify | `~/.rune/config.json` 파일 변경 감지 (WRITE 이벤트) |
| HTTP | `POST /reload` 요청 |
| Signal | SIGHUP 수신 |

세 경로 모두 동일한 `reloadConfig()` 함수를 호출한다.

### 5.2 Reload 단계

```
config reload 트리거
  │
  ├── 1. 새 config.json 파싱
  │      ├── 파싱 실패 → 경고 로그 출력, 기존 config 유지, reload 중단
  │      └── 파싱 성공 → 계속
  │
  ├── 2. 변경된 필드 감지 (diff)
  │      ├── vault.endpoint 또는 vault.token 변경
  │      │     └── Vault gRPC 재연결 + 키 번들 재다운로드
  │      ├── envector.endpoint 또는 envector.api_key 변경
  │      │     └── enVector SDK 재초기화
  │      ├── embedding.mode 또는 embedding.model 변경
  │      │     └── 모델 재로드 (느림 — 수 초 소요)
  │      └── retriever.topk, retriever.threshold 등
  │           └── 즉시 반영 (구조체 필드 교체)
  │
  ├── 3. 상태 전이 평가
  │      ├── dormant → active
  │      │     ├── Vault 연결 시도
  │      │     ├── 성공 → startup 시퀀스 4단계부터 실행
  │      │     └── 실패 → dormant 유지, 에러 로그
  │      └── active → dormant
  │           ├── 사용자가 deactivate한 경우 (state를 "dormant"로 변경)
  │           └── 리소스 해제: SDK 연결 닫기, 모델 언로드
  │
  └── 4. 완료 로그
         └── slog.Info("config reloaded", "changed_fields", [...])
```

### 5.3 fsnotify debounce

파일 에디터에 따라 단일 저장에 여러 fsnotify 이벤트가 발생할 수 있다.
(vim의 경우 RENAME + CREATE + WRITE 등)

100ms debounce를 적용한다:

```go
var debounceTimer *time.Timer

for event := range watcher.Events {
    if event.Op&fsnotify.Write == 0 && event.Op&fsnotify.Create == 0 {
        continue
    }
    if debounceTimer != nil {
        debounceTimer.Stop()
    }
    debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
        reloadConfig()
    })
}
```

---

## 6. Sleep/Wake 복구 (macOS)

macOS에서 노트북 덮개를 닫았다 열면 네트워크 연결이 끊긴다.
launchd `KeepAlive: true` 덕분에 프로세스 자체는 살아있지만, gRPC 채널이 죽어있다.

### 6.1 감지 방식

OS wake 이벤트를 직접 감지하지 않는다. 대신:

1. **요청 시점 감지** — 클라이언트 요청을 처리하다가 gRPC 호출이 실패하면 복구 시도
2. **주기적 health check** — 60초 간격으로 Vault/enVector 연결 상태 확인

### 6.2 복구 시퀀스

```
gRPC 에러 감지 (또는 주기적 health check 실패)
  │
  ├── 1. Vault gRPC health check
  │      ├── vaultClient.HealthCheck(ctx) 호출
  │      ├── 성공 → 채널 살아있음, 다음 단계
  │      └── 실패 → 채널 닫기 + 재생성
  │           ├── vaultConn.Close()
  │           ├── grpc.Dial(vaultEndpoint, ...) 으로 새 연결
  │           ├── 재연결 성공 → 계속
  │           └── 재연결 실패 → 에러 로그, 30초 후 재시도
  │
  ├── 2. enVector SDK 연결 확인
  │      ├── sdk.Ping(ctx) 호출
  │      ├── 성공 → 연결 살아있음
  │      └── 실패 → SDK.Reinit() 호출
  │           ├── 기존 gRPC 채널 닫기 + 새 채널 생성 + handshake
  │           ├── 재연결 성공 → 계속
  │           └── 재연결 실패 → 에러 로그, 30초 후 재시도
  │
  └── 3. health check 결과 로그
         └── slog.Info("connection recovery", "vault", ok/fail, "envector", ok/fail)
```

### 6.3 요청 시점 복구의 동작

capture/recall 요청 처리 중 gRPC 에러가 발생하면:

1. 복구 시퀀스를 동기적으로 실행한다 (요청 컨텍스트 내에서)
2. 복구 성공 시 원래 요청을 재시도한다 (최대 1회)
3. 복구 실패 시 503 응답 + 에러 메시지 리턴

```go
result, err := sdk.Score(ctx, query)
if isConnectionError(err) {
    if recoverErr := recoverConnections(ctx); recoverErr != nil {
        return nil, fmt.Errorf("connection recovery failed: %w", recoverErr)
    }
    // 재시도 1회
    result, err = sdk.Score(ctx, query)
}
```

---

## 7. 업그레이드

### 7.1 업그레이드 플로우

```
v0.4.0 → v0.4.1 업그레이드 시:
  │
  ├── 1. 새 바이너리 다운로드
  │      ├── 임시 파일로 먼저 다운로드: ~/.rune/bin/runed.new
  │      ├── 무결성 검증 (checksum 확인)
  │      └── os.Rename("runed.new", "runed") — 원자적 교체
  │
  ├── 2. 데몬 재시작
  │      ├── macOS → launchctl stop io.envector.runed
  │      │          + launchctl start io.envector.runed
  │      └── Linux → systemctl --user restart runed
  │
  ├── 3. 새 데몬 startup
  │      ├── 기존 데몬 graceful shutdown (4절 참조)
  │      └── 새 바이너리로 startup 시퀀스 실행 (3절 참조)
  │
  ├── 4. warm state 소실
  │      ├── 임베딩 모델 재로드 (디스크 캐시 → 수 초)
  │      ├── gRPC 재연결 (Vault + enVector)
  │      └── 첫 요청 latency 증가 예상
  │
  └── 5. config.json 유지
         └── 설정 파일은 변경하지 않음 (backward compatible)
```

### 7.2 원자적 바이너리 교체

실행 중인 바이너리를 직접 덮어쓰면 안 된다. 반드시 임시 파일에 쓰고 rename한다:

```go
tmpPath := filepath.Join(binDir, "runed.new")
f, _ := os.Create(tmpPath)
io.Copy(f, downloadReader)
f.Close()

os.Chmod(tmpPath, 0755)
os.Rename(tmpPath, filepath.Join(binDir, "runed"))
```

`os.Rename`은 같은 파일시스템 내에서 원자적이다.
다운로드 중 실패해도 기존 바이너리는 손상되지 않는다.

---

## 8. rune daemon 서브커맨드

`rune` CLI에서 데몬을 관리하는 서브커맨드:

```
rune daemon start      # 데몬 시작 (launchctl/systemctl 사용)
rune daemon stop       # 데몬 정지 (SIGTERM 전송)
rune daemon restart    # 정지 + 시작
rune daemon status     # PID, uptime, socket 상태 출력
rune daemon logs       # ~/.rune/logs/daemon.log tail -f
rune daemon health     # GET /health → JSON 출력
```

### 8.1 각 서브커맨드 동작

**`rune daemon start`**

```
1. 이미 실행 중인지 확인 (소켓 파일 존재 + /health 응답 확인)
   ├── 실행 중 → "runed is already running (PID: XXXX)" 출력, exit 0
   └── 미실행 → 계속

2. OS별 시작
   ├── macOS → launchctl start io.envector.runed
   └── Linux → systemctl --user start runed

3. 시작 확인 (최대 10초 대기)
   ├── 소켓 파일 생성 대기
   ├── /health 요청 → 200 응답 확인
   └── 타임아웃 → "runed failed to start. Check ~/.rune/logs/daemon.log" 출력
```

**`rune daemon stop`**

```
1. 실행 중인지 확인
   ├── 미실행 → "runed is not running" 출력, exit 0
   └── 실행 중 → 계속

2. OS별 정지
   ├── macOS → launchctl stop io.envector.runed
   └── Linux → systemctl --user stop runed

3. 정지 확인 (최대 35초 대기 — graceful shutdown 30초 + 여유 5초)
```

**`rune daemon status`**

```
출력 예시:

runed status: running
  PID:      12345
  Uptime:   2h 34m
  Socket:   ~/.rune/sock (active)
  State:    active
  Vault:    connected
  enVector: connected
  Embed:    ready (Qwen3-Embedding-0.6B)
  Version:  0.4.1
```

이 정보는 `GET /health`와 `GET /diagnostics` 응답을 조합해서 표시한다.

**`rune daemon health`**

```
GET /health → JSON 그대로 출력

{
  "status": "ok",
  "uptime_seconds": 9240,
  "vault": "connected",
  "envector": "connected",
  "embed": "ready",
  "version": "0.4.1"
}
```

---

## 9. 파일 시스템 레이아웃

```
~/.rune/
├── bin/
│   ├── rune                    # CLI 바이너리 (~15-30 MB)
│   └── runed                   # 데몬 바이너리 (~30-50 MB)
│
├── config.json                 # 메인 설정 파일
│                               #   state, vault 정보, envector 정보,
│                               #   embedding 설정, retriever 설정 포함
│
├── sock                        # Unix domain socket
│                               #   데몬이 생성, 퍼미션 0600
│                               #   데몬 종료 시 삭제
│
├── keys/
│   ├── EncKey.json              # FHE 공개키 (Vault에서 다운로드, 캐시)
│   └── EvalKey.json             # FHE 평가키 (Vault에서 다운로드, 캐시)
│                                #   크기: 수십 MB
│
├── models/
│   └── Qwen3-Embedding-0.6B/   # 임베딩 모델 파일
│       ├── config.json          #   첫 실행 시 다운로드 (~500 MB)
│       ├── model.onnx           #   이후 디스크 캐시에서 로드
│       └── tokenizer.json
│
├── logs/
│   ├── daemon.log               # 데몬 stdout 로그 (rotating)
│   └── daemon.err               # 데몬 stderr 로그
│
├── capture_log.jsonl            # 캡처 감사 로그
│                                #   capture 성공/실패 기록
│
├── certs/
│   └── ca.pem                   # self-signed CA 인증서 (선택)
│                                #   on-premise Vault 사용 시 필요
│
└── review_queue.json            # legacy (건드리지 않음)
```

**퍼미션 요약:**

| 경로 | 퍼미션 | 이유 |
|------|--------|------|
| `~/.rune/` | 0700 | 사용자 전용 디렉토리 |
| `~/.rune/sock` | 0600 | 소켓 소유자만 접근 |
| `~/.rune/bin/*` | 0755 | 실행 가능 |
| `~/.rune/keys/*` | 0600 | FHE 키는 민감 데이터 |
| `~/.rune/config.json` | 0600 | 토큰 등 민감 정보 포함 |
