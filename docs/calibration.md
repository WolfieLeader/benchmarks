# Client v2 calibration gate

Cross-validation of the custom Go load generator (`benchmark/`) against an
independent reference, per PLAN §7.1/§7.6: _"writing a correct load generator
is genuinely hard — before trusting client v2's numbers, run an established
tool against 2–3 endpoints and require RPS/p50/p99 to agree within tolerance."_

## How to run

```
just db-up          # the server needs the DB stack to boot
just calibrate      # full run against go-chi (~5 min)
just calibrate go-chi --quick   # harness smoke test only — NOT a valid calibration
```

**Re-run whenever `benchmark/internal/client`'s hot path changes** (loadgen
scheduling, latency stamping, percentile math, transport setup) and record the
new numbers below. Runs need a quiet machine — close heavy apps first.

## Reference tool

**oha** (`brew install oha`) — the only maintained generator that measures what
we measure: constant-arrival-rate load with latency taken from the _intended_
send time (`-q <rate> --latency-correction`, wrk2 semantics, verified in oha's
source). Surveyed alternatives (k6, vegeta, fortio, Hyperfoil, wrk2, hey,
bombardier, …): none combine maintained + open-model + intended-send-time
correction + JSON output. k6's `constant-arrival-rate` is the right _model_ but
deliberately does not latency-correct; wrk2 defined the semantics but is frozen
since 2019. Escalation options if oha and the client ever disagree: k6 as a
second opinion on RPS/drops, Hyperfoil as a heavyweight CO-free arbiter.

## Methodology

One go-chi container (same limits as a real run: 4 CPUs / 2 GB), both
generators hit the identical instance via the client's `--target` mode, on
`GET /health` — the dumbest contract endpoint: no input, no JSON, no DB.
Config: `config/calibration.json` (closed base; the script derives the open and
ceiling variants).

- **Experiment A — agreement (the gate).** Same closed shape (c=200, 30s) and
  same open shape (5,000 req/s, 30s, CO-corrected) from both tools.
  - _Open mode gates_ (exit 1 on drift): identical load shape + identical
    correction semantics ⇒ RPS within ±5%, p50/p99 within ±10% **or** 1 ms
    absolute (floor rationale below). Any dropped iteration at the
    calibration rate also fails.
  - _Closed mode is informational_: the client validates every response body
    (the project's purpose), so its closed loop is legitimately slower — the
    check is that the gap stays small and stable, not zero.
- **Experiment B — ceiling.** Step the client's open rate (5k → 160k, 10s
  each) until dropped iterations exceed 0.1% of attempts (the arrival clock
  never blocks, so `offered_rate` stays ~100% even under saturation — drops
  are the wall signal). Then fire oha at the failing rate to attribute the
  wall: if oha sustains it, the wall is the client's; if not, the server's.

### Why a 1 ms latency floor (three-instrument tie-break, 2026-07-04)

At the 5k calibration rate the server answers in ~0.1–0.5 ms, and there the
two tools disagree on latency by ~3x while agreeing perfectly on RPS. We broke
the tie with a third independent stack (Node `node:http`, sequential paced
requests) against the same container at the same rate class:

| instrument                                           | p50          |
| ---------------------------------------------------- | ------------ |
| client v2 (Go net/http), open 40k                    | 0.35 ms      |
| node:http, sequential paced                          | 0.32 ms      |
| oha, `-q 40000` (corrected **or not**, c=64/200/512) | 0.79–1.05 ms |

Two independent stacks agree with the client; **oha carries ~0.5–0.7 ms of
per-request overhead of its own in rate-limited mode** (additive, constant
across rates and connection counts). So sub-millisecond latency comparisons
against oha bound _oha's_ noise, not ours — the floor absorbs exactly that.
The meaningful latency cross-check is the closed-mode leg, where magnitudes
(~3–5 ms) dwarf generator overhead: there the tools agree within 3–9%. RPS
and drop accounting gate at all magnitudes.

## Calibration record

### 2026-07-04 — initial calibration (client v2 @ PR #33–#35)

Apple-silicon macOS host, Docker Desktop, go-chi @ 4 CPUs / 2 GB, oha 1.14.0,
30s legs. **PASSED.**

| leg          | metric | client v2 | oha     | Δ                               |
| ------------ | ------ | --------- | ------- | ------------------------------- |
| open 5k      | rps    | 5000      | 5000    | 0.0%                            |
| open 5k      | p50    | 0.14 ms   | 0.45 ms | floor (oha overhead, see above) |
| open 5k      | p99    | 0.31 ms   | 0.97 ms | floor (oha overhead, see above) |
| closed c=200 | rps    | 66,381    | 69,718  | −4.8% (validation tax)          |
| closed c=200 | p50    | 2.89 ms   | 2.80 ms | +3.2%                           |
| closed c=200 | p99    | 5.18 ms   | 4.75 ms | +9.1%                           |

Ceiling: the client sustained scheduling at every tested rate (offered =
target up to 80k). Drops appeared at 80k (12.1%) — **attributed to the
server**: oha at 80k also achieved only 74,870 rps (93.6%), i.e. go-chi
saturates at ~75k rps on this hardware, while the client's own generator
ceiling was not reached up to 80k arrivals/s.

**Operating guidance:** the closed-mode validation tax is ~5% of throughput
(uniform across servers, so rankings are unaffected). For open-mode benchmark
configs, keep the target rate within the calibrated regime — well below the
_server's_ saturation point (for go-chi-class servers here: ≲ 50k req/s) —
or expect and interpret `dropped_iterations` as the saturation result it is.
