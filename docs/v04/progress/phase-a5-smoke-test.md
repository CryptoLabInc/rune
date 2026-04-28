# Phase A.5 — smoke tests

> ✅ 통과 (2026-04-27) · 커밋 `c353926` · 브랜치 `yg/phase-a5-smoke-test` · PR #87
> 핵심 파일 2개: `internal/mcp/register_test.go` (137줄) · `cmd/rune-mcp/main_test.go` (33줄)

## 1. 한 줄 요약

Phase A 의 bash/jq cookbook (`phase-a-mcp-boot.md` §4.2 / §4.3) 을 Go test 로 정착. `go test ./...` 한 번이 handshake + tools/list + tools/call stub + shutdown 정상성 회귀를 모두 가드. 외부 의존 0 (SDK 의 `NewInMemoryTransports` 사용 — 네트워크/stdio 없음, ~1.5s).

## 2. 동작하는 것

| 테스트 | 위치 | 잡아주는 회귀 |
|---|---|---|
| `TestRegister_All8ToolsListed` | `internal/mcp/register_test.go` | tool 누락 / 오타 / 이름 변경 (예: `rune_caputre`), SDK 알파벳 정렬 회귀 |
| `TestRegister_SchemasInferred` | 동 | domain/service struct 가 InputSchema · OutputSchema 추론 불가 형태로 변경 |
| `TestRegister_StubReturnsIsError` (8 subtest) | 동 | stub 응답 포맷 변경, IsError 누락, "not yet implemented" 문구 깨짐, 잘못된 핸들러 매핑 |
| `TestIsNormalShutdown` (6 subtest) | `cmd/rune-mcp/main_test.go` | EOF 매칭 잘못 추가, ctx.Canceled 누락, 진짜 에러를 정상 종료로 오판 |

총 4 함수 / 16 케이스, ~1.5s.

## 3. 검증 — 30초 컷

### 3.1. 전체 (가장 흔한 사용)

```bash
go test ./...
```

기대: 두 패키지 `ok`, 나머지 `[no test files]`.

### 3.2. Phase A.5 만

```bash
go test ./internal/mcp/ ./cmd/rune-mcp/
```

### 3.3. verbose / 단일 테스트

```bash
go test -v ./internal/mcp/                              # 모든 subtest 출력
go test -v -run TestRegister_All8ToolsListed ./internal/mcp/  # 한 함수
go test -v -run 'TestRegister_StubReturnsIsError/rune_diagnostics' ./internal/mcp/  # 한 subtest
```

### 3.4. 캐시 무시 / 커버리지

```bash
go test -count=1 ./...                                  # 캐시 우회
go test -cover ./internal/mcp/ ./cmd/rune-mcp/         # 커버리지 한 줄
go test -coverprofile=coverage.out ./internal/mcp/ && go tool cover -html=coverage.out  # HTML
```

## 4. 새 tool 추가 시 절차

1. `internal/mcp/tools.go` `Register` 에 `sdkmcp.AddTool(...)` 한 줄 추가
2. `internal/mcp/register_test.go` 의 `expectedTools` 슬라이스에 새 이름 추가 (알파벳순 유지)
3. `go test ./internal/mcp/` 통과 확인 — 안 통과하면 보통 알파벳순 깨졌거나 이름 오타

## 5. 흔한 실패 → 원인

| 메시지 | 원인 |
|---|---|
| `tool count: got 7, want 8` | `tools.go` 에서 `AddTool` 빠뜨림 |
| `tool[N]: got "X", want "Y"` | SDK 알파벳 정렬 회귀, `expectedTools` 순서 미준수, 또는 이름 변경 |
| `InputSchema is nil` / `OutputSchema is nil` | args/result struct 가 schema inference 안 되는 타입 |
| `Content[0].Text does not contain stub marker` | `stubResult` 메시지 변경 — Phase 5 진입 전엔 실패해야 정상 |
| `isNormalShutdown(...) = true, want false` | `isNormalShutdown` 에 의도치 않은 매칭 추가 |

## 6. 다음 마일스톤

- **Phase B** — `rune_diagnostics` environment 섹션 stdlib 응답 (`runtime.GOOS` · `runtime.Version` · `os.Getwd`). 첫 진짜 응답 흐름. Phase A.5 의 `TestRegister_StubReturnsIsError/rune_diagnostics` 는 의도적으로 깨짐 — 새 메시지 expectations 로 갱신 필요
- **Phase 4 이후** — Vault/envector/embedder adapter 가 들어오면 in-memory mock 패턴 같이 도입 (Phase A.5 의 `NewInMemoryTransports` 패턴이 reference)
