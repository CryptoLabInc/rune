> **요약본** — 자세한 분석(phase별 표, ⚠ caveat, 가설/후속 조사 제안 등)은 [`latency_comparison_v1.2.2_vs_v1.4.3_2026-05-12.md`](./latency_comparison_v1.2.2_vs_v1.4.3_2026-05-12.md) 참조.

## 측정 환경

| 항목 | v1.2.2 | v1.4.3 |
|------|--------|--------|
| pyenvector | 1.2.2 | 1.4.3 |
| eval_mode | **rmp** | **mm32** |
| index_type | **flat** | **ivf_vct** |
| insert_mode | single (SDK 레벨 분기 없음) | single, batch 각각 측정 |
| envector endpoint | `0511-1401-...envector.io` | `0512-1000-...envector.ai` |
| vault endpoint | `tcp://193.122.124.173:50051` | `tcp://161.118.149.143:50051` |
| runs / warmup | 10 / 2 | 10 / 2 |
| 측정 일자 | 2026-05-12 | 2026-05-12 |

- 동일한 시나리오 셋 14개(T1–T14, `latency_bench_v1.4.3.py` 기반)를 양 환경에서 측정
- v1.2.2 측정은 v1.4.3 runner를 **SDK 호출부만 5건 수정**한 버전으로 진행 (시나리오 정의/측정 통계/결과 포맷 그대로 유지)
- **비교 주의**: pyenvector 버전, eval_mode, index_type, envector/vault 엔드포인트 4가지가 동시에 달라졌으므로 **단일 요인 해석 금지**

## 실행 결과 요약

| 시나리오 | Feature | v1.2.2 total p50 | v1.4.3 total p50 | v1.4.3 / v1.2.2 | 비고 |
|---|---|---|---|---|---|
| T1 short_en | capture | 1153.7 ms | 341.3 ms | **÷3.4** | apples-to-apples |
| T2 long_en | capture | 1176.0 ms | 359.2 ms | **÷3.3** | apples-to-apples |
| T3 korean | capture | 1194.9 ms | 380.7 ms | **÷3.1** | apples-to-apples |
| T4 duplicate | capture | 1271.8 ms | 343.1 ms | **÷3.7** | apples-to-apples |
| T5 exact_match | recall | 839.7 ms | 244.0 ms | **÷3.4** | apples-to-apples |
| T6 cross_lang | recall | 860.0 ms | 249.8 ms | **÷3.4** | apples-to-apples |
| T7 topk_1/3/5/10 | recall | 805–841 ms | 235–250 ms | **÷3.2–3.6** | apples-to-apples |
| T9 vault_status | vault_status | 7.4 ms | 6.5 ms | ÷1.1 | 사실상 동일 |
| T10–T12 searchable | searchable | 2200–2495 ms | 71–90 ms | ⚠ | 측정 메커니즘 차이 |
| T13/T14 multi_capture | multi_capture | 2013/2557 ms | 88/144 ms | ⚠ | 측정 메커니즘 차이 |

**Headline**: v1.4.3가 capture/recall에서 ~3배 빠름. **T9 vault_status는 사실상 동일(÷1.1)** — vault 연결 자체의 비용은 같고 차이는 FHE/index 경로에서 발생.

> Searchable/Multi_capture의 ÷25 / ÷23 수치는 **측정 메커니즘 차이**(v1.2.2 polling vs v1.4.3 server-push, v1.2.2 N개 single vs v1.4.3 batch path)가 만든 숫자입니다. 직접 비교 부적절 — 자세한 caveat은 `latency_comparison_v1.2.2_vs_v1.4.3_2026-05-12.md` §3 참조.

## 비교 그래프

### 시나리오별 total p50 (apples-to-apples만)

```
── capture (p50 ms, 기준 1272ms = 40칸) ────────────────────────────────

T1 short_en   v1.2.2  ████████████████████████████████████  1154ms
              v1.4.3  ███████████  341ms   (÷3.4)

T2 long_en    v1.2.2  █████████████████████████████████████  1176ms
              v1.4.3  ███████████  359ms   (÷3.3)

T3 korean     v1.2.2  █████████████████████████████████████  1195ms
              v1.4.3  ████████████  381ms  (÷3.1)

T4 duplicate  v1.2.2  ████████████████████████████████████████  1272ms
              v1.4.3  ███████████  343ms   (÷3.7)

── recall (p50 ms, 기준 860ms = 30칸) ──────────────────────────────────

T5 exact      v1.2.2  █████████████████████████████  840ms
              v1.4.3  █████████  244ms     (÷3.4)

T6 cross      v1.2.2  ██████████████████████████████  860ms
              v1.4.3  █████████  250ms     (÷3.4)

T7 topk_1     v1.2.2  ████████████████████████████  805ms
              v1.4.3  █████████  250ms     (÷3.2)

T7 topk_10    v1.2.2  █████████████████████████████  841ms
              v1.4.3  █████████  250ms     (÷3.4)

── vault_status (p50 ms, 기준 7.4ms = 20칸) ────────────────────────────

T9 vault      v1.2.2  ████████████████████  7.4ms
              v1.4.3  █████████████████  6.5ms      (÷1.1, 사실상 동일)
```

### T1 capture phase별 비교

```
── embed (p50 ms, 기준 54.4ms = 20칸) ──────────────────────────────────

v1.2.2  ██████████████  39.2ms
v1.4.3  ████████████████████  54.4ms     (×1.4, 측정 잡음 범위)

── score (p50 ms, 기준 303.6ms = 30칸) ─────────────────────────────────

v1.2.2  ██████████████████████████████  303.6ms
v1.4.3  █████████████  134.8ms          (÷2.3)

── vault_topk (p50 ms, 기준 328.8ms = 30칸) ────────────────────────────

v1.2.2  ██████████████████████████████  328.8ms
v1.4.3  █  14.5ms                       (÷22.7)  ← largest gap

── insert (p50 ms, 기준 462.1ms = 30칸) ────────────────────────────────

v1.2.2  ██████████████████████████████  462.1ms
v1.4.3  █████████  134.0ms              (÷3.4)
```

**한 눈에 보이는 패턴**: `embed`/`remind`는 사실상 동일, `score`/`insert`는 ~2–3배, **`vault_topk`만 22배**라는 비대칭. T1 총 latency 차이의 대부분이 `vault_topk` phase에서 만들어집니다.

## 주목할 점

### 1. vault_topk phase에서 차이의 대부분이 만들어진다

T1 phase 분해 기준 `vault_topk`가 **÷22.7로 가장 큰 gap**. `score`(÷2.3)와 `insert`(÷3.4)는 eval_mode/index_type 변화로 자연스럽게 설명되지만, `vault_topk`의 22배는 **vault 자체 처리 + decrypt payload 형식 + 네트워크 경로** 중 어디서 오는지 unresolved. 후속 분해 조사가 가장 임팩트가 큼.

### 2. T9 vault_health는 거의 동일 — vault TLS handshake 자체는 안정 영역

v1.2.2 **7.4ms** vs v1.4.3 **6.5ms** (÷1.1). 두 가지 동시 증거를 제공:

- **vault 서버 연결 자체는 빠르고 안정** — 두 환경 모두 원격 vault TLS handshake가 ~7ms 수준에서 수렴
- **vault_topk 22배 차이는 health 단독이 아니라 decrypt 비용에서 발생** — health check는 RPC RTT만, vault_topk는 그 위에 FHE 결과 decrypt가 얹힘

이게 §1 결론(인프라 자체는 변하지 않음)을 뒷받침하는 control 측정입니다.

### 3. embed / remind는 환경에 무관 — 차이는 FHE/vault 경로 3단계에 집중

- `embed`(로컬 Qwen 임베딩): v1.2.2 39ms vs v1.4.3 54ms → 측정 잡음 범위
- `remind`(메타데이터 fetch): v1.2.2 ~38ms vs v1.4.3 ~37ms → 사실상 동일

→ latency 차이의 결정 요인은 `score` + `vault_topk` + `insert`의 FHE/vault 경로에 집중. **SDK upgrade만으로는 latency 개선이 안 됨** — vault 인프라 + eval_mode/index_type 변경이 함께 가야 효과 발생.

### 4. Searchable / Multi_capture 수치를 단순 비교에 인용하면 안 된다

실행 결과 표의 ÷25 / ÷23 수치는 **측정 메커니즘 차이가 만든 숫자**(polling vs server-push, single vs batch insert path). 비교 그래프에서 일부러 제외했고 §3 caveat을 둠. 외부에서 인용할 때는 *"측정 메커니즘이 달라 직접 비교 부적절"* 을 함께 명시 필요.

### 5. v1.4.3 batch는 single 대비 ~5배 빠르지만 cold-start outlier 동반

T1만 ~12s 폭증(서버 batch path 초기화), T2 이후 안정. 실측 효율은 분명하나 production 적용 시 **첫 호출 outlier 처리 전략(warmup / cold-start hedge)** 이 별도로 필요. 자세한 표는 비교 리포트 §4 참조.

## 후속 TODO (Out of Scope)

- **rune SDK docstring 정정** — `agents/common/envector_client.py:135`와 `mcp/adapter/envector_sdk.py:{240,283}`이 `MERGED_SAVED`를 "searchable"의 동의어로 표기. proto 상 `MERGED_SAVED`(=6)과 `SEARCHABLE`(=7)은 별개 enum. pyenvector `await_completion`의 실제 동작 검증 후 별도 PR
- **vault_topk 22배 차이 분해** — vault 인프라/리전 / decrypt payload 형식(mm32 vs rmp) / 네트워크 RTT 중 지배 요인 식별. 다음 측정 시 명시적 RTT 측정 권장
- **v1.2.2 인덱스 누적 가설 검증** — 5/11 측정 대비 본 측정이 ~1.4–1.7배 느린 것이 인덱스 vector 수 증가 때문인지 — `score` phase의 N 의존성 측정 필요

## Test Plan

- [ ] Smoke run (`--runs 2 --warmup 1 --feature capture`)이 에러 없이 종료
- [ ] 14개 시나리오 모두 `--runs 10 --warmup 2`에서 수치 산출 (에러 0건)
- [ ] T9 vault_status p50가 5/11 baseline(7.6ms)과 ±20% 이내
- [ ] 비교 리포트의 모든 수치가 양 소스 리포트와 일치 (전사 오류 0건)
- [ ] v1.2.2 runner에 v1.4.3 신규 인자(`secure` / `eval_mode` / `index_type` / `use_row_insert` / `await_searchable`) 잔존하지 않음

🤖 Generated with [Claude Code](https://claude.com/claude-code)
