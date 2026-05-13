# v1.2.2 (rmp/flat) vs v1.4.3 (mm32/ivf_vct) Latency Comparison

> **[측정 환경]** 양 환경 모두 동일한 시나리오 셋 14개(T1–T14, `latency_bench_v1.4.3.py` 기반)를 `runs=10 / warmup=2`로 측정. **pyenvector 버전(1.2.2→1.4.3), eval_mode(rmp→mm32), index_type(flat→ivf_vct), envector/Vault 엔드포인트** 4 가지가 동시에 변경된 상태이며 측정은 모두 2026-05-12 같은 날 수행.
> v1.2.2 측정은 v1.4.3 runner를 SDK 호환으로 어댑팅한 버전(`benchmark/envector-latency-v1.2.2-extended` 브랜치)으로 진행. v1.2.2 SDK는 레벨에서 항상 single insert이므로 batch 모드는 v1.4.3 only.
>
> **[비교 주의]** 같은 시나리오의 latency 차이는 위 4가지 변화(SDK 버전 / eval_mode / index_type / infra)의 **복합 결과**. 단일 요인 해석 금지.

> **목적**: v1.4.3 latency runner의 14개 시나리오를 v1.2.2 환경에서도 동일하게 측정하여 두 환경의 phase별 latency를 비교한다.
>
> **소스 리포트**
> - v1.2.2: `benchmark/reports/latency_results_v1.2.2_rmpflat_single_2026-05-12.md` (이번 측정)
> - v1.4.3 single: `benchmark/reports/latency_results_v1.4.3_ivfvct_single_2026-05-12.md`
> - v1.4.3 batch: `benchmark/reports/latency_results_v1.4.3_ivfvct_batch_2026-05-12.md` (부속 참고)

---

## 환경 비교

| 항목 | v1.2.2 | v1.4.3 |
|------|--------|--------|
| pyenvector | 1.2.2 | 1.4.3 |
| eval_mode | **rmp** | **mm32** |
| index_type | **flat** | **ivf_vct** |
| insert_mode | single (SDK 레벨에 분기 없음) | single, batch |
| envector endpoint | `0511-1401-...envector.io` | `0512-1000-...envector.ai` |
| vault endpoint | `tcp://193.122.124.173:50051` | `tcp://161.118.149.143:50051` |
| network_rtt | unknown | unknown |
| runs / warmup | 10 / 2 | 10 / 2 |
| 측정 일자 | 2026-05-12 | 2026-05-12 |

> **[비교 주의]** pyenvector 버전, eval_mode, index_type, envector/vault 엔드포인트가 동시에 달라졌으므로 단일 요인 해석 금지.

---

## 핵심 한 줄 결론

v1.4.3는 v1.2.2 대비 **capture/recall에서 ~3배, vault_topk phase 단독으로는 ~22배** 빠르다. 가장 큰 차이는 `vault_topk` phase(328.8ms → 14.5ms)에서 발생하며, 이는 vault 인프라(엔드포인트/리전 또는 자체 처리) 변화 + eval_mode(rmp→mm32) 결과 decrypt payload 차이가 주된 후보. 단, **searchable / multi_capture의 수치 차이는 측정 메커니즘이 달라(polling vs server-push) 직접 비교 부적절**.

---

## 1. Total Latency 비교 (single insert, p50 ms)

| 시나리오 | Feature | v1.2.2 single total | v1.4.3 single total | v1.4.3 / v1.2.2 | 비고 |
|----------|---------|--------------|--------------|-----------------|------|
| T1 short_en | capture | 1153.7 | 341.3 | **0.30** | apples-to-apples |
| T2 long_en  | capture | 1176.0 | 359.2 | **0.31** | apples-to-apples |
| T3 korean   | capture | 1194.9 | 380.7 | **0.32** | apples-to-apples |
| T4 duplicate | capture | 1271.8 | 343.1 | **0.27** | apples-to-apples |
| T5 exact_match | recall | 839.7 | 244.0 | **0.29** | apples-to-apples |
| T6 cross_lang | recall | 860.0 | 249.8 | **0.29** | apples-to-apples |
| T7 topk_1 | recall | 804.8 | 250.0 | **0.31** | apples-to-apples |
| T7 topk_3 | recall | 826.3 | 235.0 | **0.28** | apples-to-apples |
| T7 topk_5 | recall | 837.2 | 244.2 | **0.29** | apples-to-apples |
| T7 topk_10 | recall | 840.6 | 249.5 | **0.30** | apples-to-apples |
| T9 vault_status | vault_status | 7.4 | 6.5 | 0.88 | apples-to-apples (양쪽 모두 vault health check 단독) |
| T10 short_en_searchable | searchable | 2200.5 | 89.5 | 0.04 | ⚠ **측정 메커니즘 차이** (아래 §3) |
| T11 long_en_searchable  | searchable | 2368.0 | 71.4 | 0.03 | ⚠ **측정 메커니즘 차이** |
| T12 korean_searchable   | searchable | 2494.6 | 86.8 | 0.03 | ⚠ **측정 메커니즘 차이** |
| T13 multi_2phase | multi_capture | 2013.0 | 88.4 | 0.04 | ⚠ **측정 메커니즘 차이** (insert phase 정의 다름) |
| T14 multi_5phase | multi_capture | 2556.6 | 144.3 | 0.06 | ⚠ **측정 메커니즘 차이** |

### Total p50 시각 비교 (apples-to-apples 시나리오, single insert)

> `(÷N)` = v1.4.3가 v1.2.2보다 N배 빠름. searchable/multi_capture는 측정 메커니즘이 달라 제외(§3 참조).

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

**한 눈에 보이는 패턴**: capture/recall 모두 v1.4.3가 v1.2.2의 ~1/3.2–1/3.7. **T9만 v1.2.2와 v1.4.3가 거의 동일** — 즉 vault 연결 자체의 비용은 같고, FHE/index 경로(score+vault_topk+insert)에서 차이가 발생.

---

## 2. Phase 분해 (T1 short_en capture, p50 ms)

| Phase | v1.2.2 (rmp/flat) | v1.4.3 (mm32/ivf_vct) | v1.4.3 / v1.2.2 | 해석 |
|-------|---|---|---|---|
| embed | 39.2 | 54.4 | 1.39 | v1.4.3가 살짝 느림 (로컬 embedding, 측정 잡음 범위) |
| score | 303.6 | 134.8 | 0.44 | eval_mode rmp→mm32 + index_type flat→ivf_vct의 영향 |
| **vault_topk** | **328.8** | **14.5** | **0.04** | **22배 차이 — 가장 큰 변동 요인** |
| insert | 462.1 | 134.0 | 0.29 | index_type 차이 + vault key 등록 차이 |
| total | 1153.7 | 341.3 | 0.30 | (Σ phase ≈ total) |

> **[해석]** vault_topk phase가 22배 차이로 가장 큰 영향. score/insert는 3배 이내. 즉 v1.4.3가 빠른 주된 이유는 **vault 자체 처리(엔드포인트/리전/decrypt payload 형식)** 변화이며, eval_mode/index_type은 부차적.

### Recall T5 phase 분해 참고

| Phase | v1.2.2 | v1.4.3 | 해석 |
|-------|---|---|---|
| embed | ~30 | ~30 | 동일 |
| score | ~290 | ~135 | flat brute-force가 ivf_vct nprobe 탐색보다 느림 (인덱스 크기 의존) |
| vault_topk | ~330 | ~13 | T1과 같은 24배 차이 |
| remind | ~38 | ~37 | 동일 |

`embed`/`remind`는 두 환경에서 거의 동일 → **FHE/vault 경로가 전체 latency 결정 요인**.

### Phase별 시각 비교 (T1 capture)

> 각 phase별로 기준을 다르게 잡아 비례 비교가 보이도록 함. `(÷N)` = v1.4.3가 v1.2.2보다 N배 빠름.

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

**한 눈에 보이는 패턴**: `embed`는 사실상 동일(로컬 연산), `score`는 ~2배, `insert`는 ~3배, **`vault_topk`만 22배**라는 비대칭. T1 총 latency 차이의 대부분이 vault_topk phase에서 만들어진다는 것을 시각적으로도 확인 가능.

---

## 3. ⚠ Searchable / Multi_capture — 직접 비교 부적절

두 시나리오 그룹은 측정 메커니즘 자체가 환경별로 다르다.

### Searchable (T10–T12)

| 측정 메커니즘 | v1.2.2 | v1.4.3 |
|---|---|---|
| 정의 | capture 후 recall이 가능해지는 시점 | (동일) |
| 검출 방법 | **클라이언트 폴링** — score → vault decrypt 반복하여 top-1 cosine ≥ 0.999가 될 때까지 대기 | **서버 push** — `insert(await_searchable=True)`가 서버 내부 상태 `MERGED_SAVED`(insert request의 모든 vector가 임시(raw) shard에서 정식(non-raw) shard로 전부 이동 완료됐지만, 아직 정식 publish(`LoadIndex`)는 거치지 않은 상태)에 도달하면 RPC 반환. (proto enum에 `MERGED_SAVED=6`과 `SEARCHABLE=7`이 별도로 존재 — `envector-msa-1.4.3/proto/v2/common/index-operation-message.proto`) |
| 최소 정밀도 | poll_interval(200ms) + per-cycle RPC latency | RPC RTT |
| 측정값에 포함된 것 | insert RPC + 폴링 1+ 사이클의 score + vault decrypt | insert RPC + 서버 인덱싱 wait |

**→** v1.2.2의 ~2200ms는 폴링 오버헤드(insert ~470ms + 폴링 사이클 1회 ~score 300ms + vault 330ms ≈ ~1100ms + α) 합산이며, v1.4.3의 ~89ms와 같은 잣대가 아니다. **"두 환경의 queryable 검출 메커니즘 자체가 다르다"는 정성적 결론**으로만 사용.

### Multi_capture (T13–T14)

| 측정 메커니즘 | v1.2.2 | v1.4.3 |
|---|---|---|
| insert phase | `insert(vectors=[v1,...,vN])` — SDK가 내부에서 처리(v1.2.2는 batch path 없음, 사실상 N번 single insert 등가) | `insert(vectors=..., use_row_insert=False)` — 서버 batch insert path |
| 비교 의미 | application-level "N개 vector 동시 처리"에 걸리는 wall-clock | (동일) |

**→** v1.4.3가 ~23배 빠른 것은 batch insert path와 vault_topk phase 차이의 합산. 의미 있는 비교를 하려면 multi_capture의 단계별(embed_batch / score / vault_topk / insert_batch) phase를 분리해 봐야 함 (양쪽 리포트에 분해 데이터 있음).

---

## 4. v1.4.3 batch insert — 부속 참고 (v1.4.3 only)

v1.2.2 SDK는 batch insert path가 없어 직접 비교 불가. v1.4.3 batch 결과만 단독 기록.

| 시나리오 | v1.4.3 single total | v1.4.3 batch total | 비고 |
|----------|---|---|---|
| T1 short_en | 341.3 | **11836.4** | ⚠ 서버 batch path 초기화 outlier (소스 리포트에 명시) |
| T2 long_en  | 359.2 | 115.5 | batch가 single보다 빠름 |
| T3 korean   | 380.7 | 124.7 | batch가 single보다 빠름 |
| T4 duplicate | 343.1 | 108.1 | batch가 single보다 빠름 |
| T5–T7 recall | ~240 | ~44 | v1.4.3 batch가 single보다 ~5배 빠름 |

T1 outlier 제외하면 v1.4.3 batch가 single보다 빠른 경향. 다만 batch 실행이 single 직후 같은 인덱스/세션에서 진행되어 인덱스 상태/캐시 효과가 섞였음(소스 리포트의 "측정 순서 영향" 노트 참조).

---

## 5. 주목할 점

### vault_topk phase에서 latency 차이의 대부분이 만들어진다

T1 phase 분해 기준 `vault_topk`가 **÷22.7로 가장 큰 gap**. `score`(÷2.3)와 `insert`(÷3.4)는 eval_mode/index_type 변화로 자연스럽게 설명되지만, vault_topk의 22배는 **vault 자체 처리 + decrypt payload 형식 + 네트워크 경로** 중 어디서 오는지 여전히 unresolved. 후속 분해 조사가 가장 임팩트가 크다 (§6.1 참조).

### T9 vault_health는 거의 동일 — vault TLS handshake 자체는 안정 영역

v1.2.2 **7.4ms** vs v1.4.3 **6.5ms** (÷1.1). 두 가지 동시 증거를 제공:
- **vault 서버 연결 자체는 빠르고 안정**: 두 환경 모두 원격 vault TLS handshake는 ~7ms 수준에서 수렴.
- **vault_topk 22배 차이가 health 단독이 아니라 decrypt 비용에서 발생**: health check는 RPC RTT만, vault_topk는 그 위에 FHE 결과 decrypt가 얹힘. 두 수치의 비대칭이 그 신호.

### embed/remind는 환경에 무관 — 차이는 FHE/vault 경로에 집중

- `embed` (로컬 Qwen 임베딩): v1.2.2 39ms vs v1.4.3 54ms → 측정 잡음 범위
- `remind` (메타데이터 fetch): v1.2.2 ~38ms vs v1.4.3 ~37ms → 사실상 동일

latency 차이의 결정 요인은 **`score` + `vault_topk` + `insert`** 의 FHE/vault 경로 3단계에 집중. application 레벨 wrapping이나 클라이언트 코드는 영향 없음 → SDK upgrade만으로는 latency 개선 안 됨, vault 인프라 + eval_mode/index_type 변경이 함께 가야 효과가 나온다.

### Searchable/Multi_capture 수치 차이를 단순 비교에 인용하면 안 된다

§1 표의 ÷25 / ÷23 같은 수치는 **측정 메커니즘 차이가 만든 숫자**(v1.2.2 polling vs v1.4.3 server-push, v1.2.2 N개 single vs v1.4.3 batch insert path). ASCII 차트에서 일부러 제외했고, §3에 caveat을 둠. 외부에서 본 표를 인용할 때는 *"측정 메커니즘이 달라 직접 비교 부적절"* 을 함께 명시해야 함.

### v1.4.3 batch는 single 대비 ~5배 빠르지만 outlier 동반

§4 참조. T1만 ~12s 폭증(서버 batch path 초기화), T2 이후 안정. 실측 효율은 분명하나 production 적용 시 **첫 호출 outlier 처리 전략(warmup / cold-start hedge)** 이 별도로 필요.

---

## 6. 가설 / 후속 조사 제안

### 6.1 vault_topk 22배 차이의 분해
- 같은 vault 코드 베이스인데 v1.2.2 vault(`193.122.124.173`) vs v1.4.3 vault(`161.118.149.143`)에서 22배 차이는 인프라 단독으로 설명하기 어려움. 가능 후보:
  - **decrypt payload 형식**: mm32 결과가 rmp 결과보다 가벼울 수 있음
  - **vault 서버 부하/머신 스펙**: 운영 환경 차이
  - **네트워크 RTT**: ping 실패로 미측정. 다음 측정 시 `--network-rtt` 명시적 측정 추가 권장

### 6.2 v1.2.2 측정값이 5월 11일 측정 대비 더 느려진 이유
- 5월 11일 v1.2.2: T1 capture p50 **664ms**, T5 recall **528ms**, vault 7.6ms
- 5월 12일 v1.2.2(이번): T1 capture **1153ms**, T5 recall **840ms**, vault 7.4ms
- 가설: 한 달 사이 v1.2.2 인덱스에 다른 벤치마크/MCP 호출로 vector가 누적되어 score(flat brute-force) cost가 N에 선형 증가
- 확인 방법: `~/.rune/keys/<key_id>/`에서 인덱스 크기 메타데이터 확인, 또는 score phase의 vector 개수 의존성 측정
- 참고: `benchmark/reports/latency_results_v1.2.2_rmpflat_single_2026-05-12.md`의 "주목할 점" 섹션에 단독 분석이 있음 (T9 control + T3 한국어 score-vs-영문 차이 소실)

### 6.3 Searchable 측정의 표준화
- v1.2.2 polling과 v1.4.3 server-push를 같은 표에 올리지 않거나, 양쪽에 polling 측정을 추가해 같은 잣대로 재측정하는 게 정직.

---

## 부록: 재측정 절차

```bash
# Worktree
git worktree add -b benchmark/envector-latency-v1.2.2-extended \
  ../rune-v1.2.2-bench benchmark/envector-latency-v1.2.2

# v1.4.3 runner/plan 가져오기
cd ../rune-v1.2.2-bench
git checkout benchmark/envector-latency-v1.4.3 -- \
  benchmark/runners/latency_bench_v1.4.3.py \
  benchmark/plans/latency_bench_plan_envector_v1.4.3.md

# (runner의 v1.2.2 호환 어댑팅 — 본 리포트 작성 시 5개 위치 수정: eval/index 상수, EnVectorClient 인자 정리, insert 호출 인자 정리, searchable polling 우회)

# 본 측정
/Users/heeyeon/Desktop/Projects/rune/.venv/bin/python \
  benchmark/runners/latency_bench_v1.4.3.py \
  --insert-mode single --runs 10 --warmup 2 \
  --report benchmark/reports/latency_results_v1.2.2_rmpflat_single_$(date +%Y-%m-%d).md
```
