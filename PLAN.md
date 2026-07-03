# Benchmark Platform Expansion Plan

Status: **planning approved-in-progress** · Last updated: 2026-07-03

---

## 0. Locked decisions

| Topic | Decision |
| --- | --- |
| Run scope | **Selectable suites** — config defines endpoint suites + server groups; CLI picks. Full matrix possible, not default. |
| TS sharing | **Max sharing of infrastructure** — one shared DB layer / schemas / consts for all TS apps; handlers stay idiomatic per framework (see "Framework idioms" below). |
| New servers | **All in one wave** — Django, Flask, Zig, Kotlin (Ktor + Spring Boot), Rust (Axum + Actix), Go Echo. |
| Contract tests | **Before adding servers** — conformance suite is the gate for every new server. |
| "Playground" | Folded into the `POST /validate` endpoint (heavy validation à la Zod in every language). Not a separate feature. |
| Framework idioms | **Idiomatic code and ecosystem conventions everywhere** — each framework/language is written the way its community writes production code (Django ORM, NestJS DI + modules, Spring annotations, Cargo/Gradle layouts, etc.). **Sharing stops where idiom starts**: shared packages hold infrastructure (DB clients, schemas, validation rules, constants, config); routing/handlers/app structure are per-framework and idiomatic. |
| Zig | **One server** (http.zig), **all 4 databases**, no shared layer (single implementation). |
| FastAPI workers | Normalize to **single-process** like all other servers (fairness; current `--workers 4` is the biggest asymmetry). |
| TS runtimes | **Hono is the single multi-runtime TS app** (Node 26 + Bun + Deno — the only framework officially first-class on all three, see §4). All other TS frameworks stay on their home runtime: express/fastify/nestjs → Node, elysia → Bun, oak → Deno. |
| TS postgres driver | Switch `pg` → **`postgres` (postgres.js)** via `drizzle-orm/postgres-js`. |
| Go version | **1.27rc1** (confirmed available) via `toolchain` directive, everywhere. |
| Task runner | **just stays** (no Makefile). We need a command runner, not a build system — incremental builds belong to each language's toolchain. Note `just` install in README for contributors. |
| Lint/format | **Strict on correctness, default on style** — formatters at ecosystem defaults (they ARE the convention), linters strict and merge-gating via `just verify` + CI. **One config per language** at the language root (no per-server copies). All lint/format tools pinned to **latest versions**. |
| Metrics stack | **Switch InfluxDB → dedicated PostgreSQL** (metrics instance, separate from the benchmarked one); **keep Grafana**, upgrade to 13.x. Researched decision — see §9.1. |
| Client queue | **No broker** (no Kafka/Rabbit/NATS/BullMQ) — in-process bounded channels; see §7.5. |
| Client & orchestration | **Keep the custom Go client as both generator and orchestrator** (validation + sequences + lifecycle are the project's value); **no local K8s** (noise + complexity for zero benefit single-node); generator correctness guarded by a **cross-validation gate vs oha/k6**; see §7.6. |
| Client flags | **Minimal — flags select, config configures.** `config/config.json` is the single source of behavior, schema-validated at startup; see §7.4. |

---

## 1. Current state (audit summary)

### 1.1 The API contract today

All 10 servers expose the **exact same 16 routes** — verified consistent (status codes, JSON shapes, validation rules, error strings):

- `GET /` → `{"hello":"world"}` · `GET /health` → `OK` (text/plain)
- `/params/*` (7 routes): search query (trim + safe-int limit), URL param, header, JSON body (object-only), cookie (read `foo`, set `bar`), form (urlencoded/multipart), file upload (multipart, 1 MB cap, text/plain only → 413/415)
- `/db/:database/*` (7 routes × postgres/mongodb/redis/cassandra): health (`OK`/503), create (201), read/update/delete by id (200/404), delete-all, reset
- Data: `User {id, name, email, favoriteNumber?}` — UUIDv7 (ObjectId for Mongo); errors always `{"error", "details"?}`
- Shared behaviors: logger off when `ENV=prod`, validation (validator/v10, Pydantic, Zod), graceful shutdown on SIGINT/SIGTERM, same env-var contract, non-root multi-stage Dockerfiles

### 1.2 Key findings

1. **Duplication is extreme and literal**: DB/repository layer is byte-identical copy-paste — 3× in Go, ~6× in TS. This makes extraction a pure *move* (verifiable by conformance suite), not a rewrite.
2. **Fairness asymmetries**: FastAPI runs 4 uvicorn workers (everyone else single-process); Python pg pool is 20+40 vs 50 elsewhere; Bun servers use native `RedisClient`/`randomUUIDv7`; Bun servers never stop `Bun.serve` on shutdown.
3. **Client gaps**: closed-loop fixed-VU only (coordinated omission → understated tail latency); **no RPS/throughput metric at all**; docker CLI shell-outs; host port hardcoded 8080 (no parallel servers); Influx URL/token hardcoded; **metrics silently dropped** if Influx is down; DB reset only once per server; no DB-container resource sampling.
4. **Grafana won't scale**: per-server colors/labels hardcoded in ~140 lines of JSON overrides; axis capped at `max: 11`; endpoint variable single-select. Queries themselves (`GROUP BY server`) scale fine.
5. **No workspaces anywhere**: root package.json is not a pnpm workspace; Go servers are 4 unrelated modules; the copy-paste follows from this.

---

## 2. Target architecture

### 2.1 Folder structure

```
apps/
  benchmark/                    # the Go client (moved from benchmarks/)
  servers/
    typescript/
      express/  fastify/  nestjs/  oak/  elysia/    # one app each, home runtime
      hono/
        src/...                 # ONE app
        entry/node.ts entry/bun.ts entry/deno.ts    # 3 runtime entrypoints → 3 benchmark entries
    go/        chi/ gin/ fiber/ echo/
    python/    fastapi/ django/ flask/
    rust/      axum/ actix/
    zig/       server/          # single server: http.zig, all 4 DBs
    kotlin/    ktor/ spring-boot/
shared/
  typescript/                   # @bench/shared — db ops, zod schemas, env, consts
  go/                           # module: shared db/config/consts/validation
  python/                       # bench-shared: async + sync repository impls
  rust/                         # shared crate (workspace member)
  kotlin/                       # shared Gradle module
  contract/                     # language-neutral API spec + conformance cases (JSON)
config/  infra/  grafana/  test-files/  results/
```

### 2.2 Workspace tooling — native per language, `just` as the umbrella

No Nx/Turborepo/Bazel: there is no build graph to optimize, just "N apps → 1 shared package" per language.

| Language | Mechanism | Notes |
| --- | --- | --- |
| TypeScript | **pnpm workspace** | Bun consumes pnpm-installed `node_modules` fine. **Deno does not read `pnpm-workspace.yaml`** — add a root `deno.json` with a mirrored `workspace` list and run with `--node-modules-dir=manual` (verified against Deno docs; expect first-run friction — see §12). |
| Go | **`go.work`** | spans `shared/go` + each server + `apps/benchmark` |
| Python | **uv workspace** | `[tool.uv.workspace]`; fastapi/django/flask depend on `bench-shared` |
| Rust | **Cargo workspace** | shared crate + axum + actix |
| Kotlin | **Gradle multi-project** | `:shared`, `:ktor`, `:spring-boot` |
| Zig | none needed | single self-contained app |

### 2.3 Docker build contexts

Shared folders force build context above the app dir. Convention: **build from repo root**, `docker build -f apps/servers/go/chi/Dockerfile .` — each Dockerfile copies `shared/<lang>` + its app. `just images` updated accordingly. `.dockerignore` at root keeps contexts small.

---

## 3. Shared code strategy per language

**Guiding principle — share infrastructure, keep app code idiomatic.** The shared packages contain what is framework-independent by nature: DB clients + repositories, data types, validation schemas/rules, constants, env parsing. Everything the framework has an opinion about — routing, handlers, middleware wiring, DI, project layout — is written per-framework in that framework's canonical production style. If sharing a piece would force a framework out of its idiom (NestJS services/DI, Django ORM views, Spring controllers), it is not shared. Each language follows its ecosystem's conventions and tooling, under one policy — **formatters at defaults, linters strict, one config per language root, latest tool versions, all merge-gating through `just verify` + CI**:

| Language | Formatter (ecosystem defaults) | Linter (strict) |
| --- | --- | --- |
| TypeScript | biome format | biome recommended + current extra rules (single root config — today's per-server configs consolidate) |
| Go | golangci-lint fmt (gofmt/gofumpt) | golangci-lint, curated linter set in one shared `.golangci` config |
| Python | ruff format | ruff (wide rule selection) + pyright **strict mode** |
| Rust | rustfmt (untouched) | clippy with `-D warnings` |
| Kotlin | ktlint | detekt |
| Zig | `zig fmt --check` | (compiler is the linter) |

### TypeScript — `@bench/shared`

- **DB clients + repositories**: single implementation; `pg` → `postgres` (postgres.js) through `drizzle-orm/postgres-js` (postgres.js officially supports Node/Deno/Bun). Mongo (`mongodb`), Redis (`ioredis`), Cassandra (`cassandra-driver`).
- **Runtime adapters**: portable impl is the default; Bun-native bits (`Bun.RedisClient`, `randomUUIDv7`) become injectable adapters chosen by the entrypoint, so Bun entries keep their native edge while sharing everything else.
- **Zod schemas, env parsing, consts/errors**: moved verbatim (already byte-identical).
- **Routing/handlers stay per-framework and idiomatic** (Express routers, Fastify plugins + its schema hooks, NestJS modules/controllers/services with DI, Hono/Elysia app chains, Oak router) — they call the shared repositories and Zod schemas.

### Go — `shared` module

Move `internal/database` (+ sqlc output), `config`, `consts`, and validation into one module. Framework dirs keep idiomatic router wiring, middleware, and handlers (chi/gin/fiber/echo each in their canonical style). Add **Echo**. `go.work` ties it together.

### Python — `bench-shared`

Two repository implementations because runtimes differ:
- **async** (asyncpg/SQLAlchemy, motor, redis.asyncio, scylla-driver) → FastAPI, and Django's non-ORM DBs
- **sync** (psycopg3, pymongo, redis-py, cassandra-driver) → Flask (gunicorn, sync workers)
- **Django is batteries-included** (locked decision): Django ORM + its migrations for Postgres; Mongo/Redis/Cassandra via the shared layer (Django has no first-party support for them). Run under ASGI (uvicorn) with async views where idiomatic.
- Normalize FastAPI: 1 worker, pg pool 50.

### Rust / Kotlin / Zig

- **Rust**: shared crate (sqlx or deadpool-postgres, mongodb, redis-rs, scylla) used by Axum + Actix.
- **Kotlin**: shared Gradle module with DB ops; Ktor (Netty/CIO) + Spring Boot in Kotlin (MVC + virtual threads — the current idiomatic-modern setup).
- **Zig**: no shared layer — single server (see §6).

---

## 4. TypeScript runtime × framework matrix (researched, July 2026)

Latest: **Node 26.4.0** (Current; LTS is 24.x until Oct 2026) · **Bun 1.3.14** · **Deno 2.9.1**.

| Framework | Node 26 | Bun 1.3 | Deno 2.9 | Ship? |
| --- | --- | --- | --- | --- |
| Hono 4 (`@hono/node-server` v2) | ✅ official | ✅ official (`export default app`) | ✅ official (`Deno.serve(app.fetch)`) | **node-hono, bun-hono, deno-hono** |
| Elysia 1.4 (`@elysiajs/node` 1.4.5) | ⚠️ official adapter, youngest — history of lockstep breakage | ✅ home runtime | ❌ no adapter (web-standard mode untested) | **bun-elysia only** |
| Express 5 | ✅ native | ✅ Bun claims full support | ✅ via `npm:express` | **node-express only** |
| Fastify 5 | ✅ native | ✅ node:http fully implemented (not in Fastify CI) | ⚠️ compat-only; Fastify won't support Deno | **node-fastify only** |
| NestJS 11 | ✅ native | ⚠️ real regressions (e.g. bun#27526), no official support | ⚠️ community-only, "not production-ready" | **node-nestjs only** |
| Oak 17 (**JSR** `@oak/oak`; npm copy is stale at 14.1) | ⚠️ official but no TLS/`.send()`/WS | ⚠️ same as Node | ✅ home runtime | **deno-oak only** |

**Decision: Hono is the single multi-runtime TS app** — it is the only framework with first-party support on all three runtimes, which makes it the clean runtime-vs-runtime comparison (same app, same code, three runtimes). Every other framework ships on its home runtime only, avoiding the ⚠️ compat-layer tier entirely. → **8 TS entries** (was 6): the 5 home-runtime apps + hono×3.

**Driver caveat to smoke-test in conformance**: `cassandra-driver` on Bun (2023-era failures, current status unverified) and on Deno (never tested upstream) — relevant to bun-elysia, bun-hono, deno-hono, deno-oak. postgres.js / mongodb / ioredis are confirmed fine on all three.

---

## 5. New endpoints

| Endpoint | Suite | What it exercises | Contract sketch |
| --- | --- | --- | --- |
| `GET /html` | `web` | server-rendered HTML template | `200 text/html`, small dynamic template (name + list + number interpolation) |
| `GET /jwt/sign` | `web` | HS256 sign | `200 {"token": "..."}` — fixed claims + exp, shared secret via env |
| `GET /jwt/verify` | `web` | HS256 verify + header parsing | `Authorization: Bearer <t>` → `200 {payload}` / `401 {"error":"invalid token"}` |
| `POST /validate` | `web` | heavy validation (Zod / Pydantic / validator / serde / etc.) | deep nested object (~4 levels, arrays, enums, email/uuid/range rules) → `200 {"valid":true}` / `400` with error count; pass **and** fail variations |
| `GET /compute?n=` | `web` | pure CPU (isolates runtime from I/O) | e.g. iterative SHA-256 chain of n rounds → `200 {"result": "<hex>"}` , n capped |

Existing 16 routes are unchanged. Suites: `basic` (root/health), `params`, `web` (new), `db` (per-database CRUD).

---

## 6. Server roster & port convention

### Roster (final: 20 entries)

| Language | Entries |
| --- | --- |
| TypeScript (8) | node-express, node-fastify, node-nestjs, deno-oak, bun-elysia, **node-hono, bun-hono, deno-hono** |
| Go (4) | chi, gin, fiber, **echo** |
| Python (3) | fastapi, **django**, **flask** |
| Rust (2) | **axum**, **actix** |
| Kotlin (2) | **ktor**, **spring-boot** |
| Zig (1) | **zig** (http.zig) |

### Zig server (researched, July 2026)

Zig **0.16.0 stable** (Apr 2026). Stack — all four databases, mixed pure-Zig/C:

| Piece | Choice | Status |
| --- | --- | --- |
| HTTP | **http.zig** (karlseguin) | 0.16-native, actively maintained, the de-facto production server |
| Postgres | **pg.zig** (same author) | 0.16-native, has `pg.Pool` — solid |
| Redis | **okredis** (verify 0.16 build) or hand-rolled RESP client (~150 lines) | easy either way |
| Cassandra | **zig-cassandra** (pure Zig, updated at 0.16 release — build-verify) | fallback: apache cassandra-cpp-driver (C API) via `@cImport` |
| MongoDB | **libmongoc via C interop** — no living Zig driver exists | the one hard item; drags a C toolchain into the Docker build |

Blocking C clients are fine under http.zig's thread-per-worker model — no architectural blocker. Estimated 4–7 days.

### Port convention

Two rules replace the current ad-hoc list:

1. **Inside containers, every server listens on the same canonical port: `8080`** (via `PORT` env in the Dockerfile). The benchmark client maps a host port dynamically (frees us from hardcoded 8080 and enables parallel servers later). Config drops per-server `port`.
2. **Dev ports** (`just dev <entry>`) follow `<language-block> + <framework><runtime>` so they're derivable, never looked up:

```
TS    = 3000 + framework×10 + runtime      runtime: 1=node 2=bun 3=deno
        express=1x, nestjs=2x, fastify=3x, oak=4x, hono=5x, elysia=6x
        → node-express 3011 · node-nestjs 3021 · node-fastify 3031 · deno-oak 3043
          node-hono 3051 · bun-hono 3052 · deno-hono 3053 · bun-elysia 3062
Python= 4010 fastapi · 4020 django · 4030 flask
Go    = 5010 chi · 5020 gin · 5030 fiber · 5040 echo
Rust  = 6010 axum · 6020 actix
Zig   = 7010
Kotlin= 8010 ktor · 8020 spring-boot
```

(Host-side reserved: 3000 Grafana, 5433 metrics-postgres — no collisions with the scheme.)

Image naming: `bench/<language>-<entry>` (e.g. `bench/ts-bun-hono`, `bench/go-echo`).

---

## 7. Benchmark client v2 — "perfect" requirements

### 7.1 Correctness of measurement

- **Throughput (RPS) reported everywhere** — the current latency-only ranking cannot distinguish fast-per-request from high-throughput. New `throughput` fields in summaries, the metrics DB, and dashboards.
- **Open-model mode** alongside the current closed loop: constant-arrival-rate scheduling with latency measured from *intended* send time — fixes coordinated omission. Config: `mode: "closed" | "open"`, `rate`.
- **Load profiles / ramping**: k6-style stages — `[{target, duration}]` for ramp-up → hold → ramp-down, step-load, and spike shapes; per suite.
- **Backpressure is measured, not hidden**: in open mode, when the server can't absorb the target rate the client records schedule lag / late-starts / backlog depth explicitly — saturation becomes a first-class result instead of silently degrading into a closed loop.
- **Max-throughput search** (phase 2 of client work): stepped ramp finding the highest rate where error-rate and p99 stay within budget → a single "capacity" number per server.
- **Cross-validation gate**: writing a correct load generator is genuinely hard (that's why wrk2/k6 exist). Before trusting client v2's numbers, run an established tool (oha or k6) against 2–3 endpoints and require RPS/p50/p99 to agree within tolerance (~5%). Re-run this calibration whenever the generator's hot path changes. This keeps the custom client honest without giving up its advantages (§7.6).
- Percentiles: add p99.9, use interpolated quantiles; record real wall-clock timestamps on points (drop the synthetic `baseTime + index·µs` hack — keep offsets as fields).

### 7.2 No silent drops — hard rules

- Metrics DB unreachable → **fail the run** (or explicit `--no-metrics`); never the current warn-and-drop.
- Bounded internal channels/buffers with accounting: any dropped/sampled point is **counted and reported** (`points_written`, `points_dropped`, `points_sampled_out` in `run_meta`); post-run verification query confirms written counts match.
- Async metrics writes (batched COPY) get retry + final flush with deadline; a failed flush fails the run summary.
- Config validated against the JSON schema **at runtime** (it's editor-only today).
- Container/DB readiness failures, non-2xx during warmup, and mid-run container death are all distinct, loudly-reported failure modes.

### 7.3 Infrastructure

- **testcontainers-go** replaces docker-CLI shell-outs for DBs + servers-under-test: real wait strategies, ryuk auto-cleanup, per-container resource limits, stats via SDK (drop the raw unix-socket HTTP client). Grafana/metrics-postgres stay on compose — they outlive the run for dashboard viewing.
- Un-hardcode: metrics DB DSN, host port, required-DB list (derive from config `databases`).
- Per-server `databases` subset + `experimental` flag in config; `--suites=` and `--group=` (language/runtime) selection; DB state reset between endpoint groups, not once per server.
- Sample DB-container resources too (CPU/mem of postgres/mongo/redis/cassandra during each server's run).
- New tags on every measurement: `language`, `runtime`, `suite`, `experimental` — this is what makes Grafana scale.

### 7.4 CLI flags & server discovery (no more hardcoding)

**Discovery — make the client folder-structure aware.** Today the roster lives hardcoded in `config/config.json` (`servers` list) and drifts from reality. Instead: each server app carries a small manifest (`bench.json`) next to its Dockerfile declaring `{name, language, runtime, image, databases, experimental, dev_port}`. The client **discovers the roster by scanning `apps/servers/**/bench.json`** — adding a server = adding a folder, zero central edits. `config/config.json` keeps only benchmark parameters (suites, endpoints, load profiles, container limits); the schema validates both.

**Flags — deliberately minimal: flags select, config configures.** `config/config.json` is the single source of truth for all behavior (load mode, rates, profiles, durations, limits, output). Flags only scope *this run* and never introduce a second place to configure something:

```
--servers=a,b,c     select servers (default: interactive multi-select from discovery)
--suites=basic,db   select suites
--quick             preset: one small suite, short durations (dev loop)
--conformance       run the contract suite instead of the benchmark
--check             validate config against schema + resolve roster, then exit
--no-metrics        run without the metrics DB (results JSON still written)
```

That's the whole surface. Anything tempting as a flag (rate, concurrency, profile, output dir) goes in config — if two configs are needed often, that's a second config file (`config/quick.json`), not more flags.

**Config correctness is enforced**: the JSON schema (today editor-only) is validated at startup — unknown keys, bad durations, unknown suite/database references, and manifest/roster mismatches are startup errors with precise messages, not silent defaults.

### 7.5 Does the client need a queue / pub-sub (Kafka, RabbitMQ, NATS, BullMQ)?

**No — and adding one would actively hurt.** The client is a single Go process on the same host as the system under test:

- The work is already in-process: workers → bounded Go channels → aggregator → batched async metrics writer. That is the same architecture k6, vegeta, and wrk2 use; none of them use an external broker.
- A broker container (Kafka/Rabbit/NATS) would compete for CPU/RAM with the server being measured — the benchmark would contaminate its own numbers with infrastructure it doesn't need.
- Brokers solve durability and fan-out *across processes/machines*. We have one producer and one consumer in one process; Go channels give the same decoupling with nanosecond overhead and zero serialization.
- The "no silent drops" requirement (§7.2) is solved by bounded channels + accounting, not by a durable queue.
- BullMQ is a Node/Redis job queue — wrong ecosystem for a Go client entirely.

The only scenario where a broker (NATS would be the pick) earns its place is **distributed multi-machine load generation** — explicitly out of scope; revisit only if the generator ever moves off-host.

### 7.6 Build vs buy: custom client & orchestrator — and why not Kubernetes

**Custom client: keep it — it is the project.** The generic load-generation part could be bought (k6, wrk2, vegeta, oha), but everything that makes this repo valuable is custom logic no off-the-shelf tool provides together: full response-body validation on every request (correctness, not just status codes), CRUD sequences with capture/templated vars fanned out per database, per-server container lifecycle with resource sampling, the conformance mode, and one coherent results/metrics pipeline. Gluing k6 (JS scripting, its own per-VU overhead) or wrk2 (Lua, no sequences/validation) to a separate orchestrator, a separate validator, and a separate stats collector would be *more* moving parts, not fewer. TechEmpower reached the same conclusion — custom harness. The known risk of DIY — subtle generator bugs — is handled by the **cross-validation gate** in §7.1 (calibrate against oha/k6, agree within ~5%).

**Orchestrator: the Go client stays the orchestrator; no Kubernetes.** What orchestration actually requires here: start one server container at a time, wait for readiness, apply resource limits, sample stats, tear down, next. That's testcontainers-go territory (§7.3). Local K8s (kind/k3d/minikube) would add:
- **measurement noise** — control plane + kubelet burning CPU on the same host that runs the SUT;
- **network distortion** — kube-proxy/CNI layers between client and server, versus Docker's direct port mapping;
- **complexity tax** — manifests, image loading into the cluster, slower iteration — for a *sequential, single-node* workload that uses none of K8s's actual value (scheduling across nodes, self-healing, service discovery).

K8s becomes the right tool only if benchmarking ever goes distributed/multi-node (client on one machine, SUT on another, DBs on a third) — same trigger as the broker question, same answer: out of scope, revisit then. Plain docker compose alone was also considered and rejected for the per-server loop: it can't express "sequential lifecycle with readiness gates, stats attach, and cooldowns" — that logic needs a program, and we have one.

---

## 8. Contract conformance suite (gate for every server)

Extend the Go client with a **`conformance` command** (reuses the existing request builder + validator):

- Runs every endpoint + every variation **once, sequentially, strict full-body assertions**.
- Adds **negative cases** the benchmark never exercises: 400 invalid JSON / non-object body / bad form, 404 unknown user + unknown database, 413 oversized file, 415 wrong content type, malformed `favoriteNumber`, invalid email, JWT 401.
- Adds **security-behavior cases** — the contract's security properties, asserted per server: file upload must inspect actual content, not trust the `Content-Type` header (**anti-sniffing**: an image/binary sent with `Content-Type: text/plain` must be rejected 415 `"file does not look like plain text"` — already part of the contract, now explicitly tested with a binary fixture in `test-files/`); size caps enforced pre-read (413); JSON body type enforcement (no array/null smuggling); JWT signature + expiry actually verified (tampered/expired token → 401); path params handled safely (`/params/url/..%2f` style inputs return a normal 200/404, never traversal).
- Cases live in `shared/contract/` (language-neutral JSON), consumed by both benchmark and conformance modes.
- `just conformance <entry>` — CI-friendly exit code. **No server ships without passing.** Also the smoke test for risky runtime×driver combos (cassandra-driver × Bun/Deno).
- This is what makes "idiomatic everywhere" safe: implementations may differ in style as much as their frameworks demand, but the observable contract may not — the suite is the referee.

---

## 9. Metrics, storage & Grafana redesign

### 9.1 Storage decision: **switch InfluxDB → plain PostgreSQL** (researched, July 2026)

What we actually do is **not classic time-series**: we write event data once per run (per-request latencies, aggregates) tagged by `run_id`, then run OLAP-style queries (rank, percentile, group-by, cross-run compare). The current setup even fakes timestamps (`baseTime + index·µs`) to satisfy Influx's data model — a sign of mismatch. Research verdict:

- **InfluxDB 3 Core is disqualified** for the "compare runs across weeks" requirement: the ~72h limit is real and current (implemented as a 432-Parquet-file query limit — queries touching more files *error out*; raising `query-file-limit` degrades speed/RAM because **Core has no compactor**). Historical data is second-class by design; the fix is Enterprise's non-commercial license — a mismatch traded for a license dependency. ([config-options docs](https://docs.influxdata.com/influxdb3/core/reference/config-options/), [community thread](https://community.influxdata.com/t/influxdb-3-core-seems-to-ignore-the-72-hour-query-time-range-limit/57443))
- **Plain PostgreSQL wins** for this workload (≤ millions of rows per run): exact `percentile_cont` for p50/p95/p99/p99.9, rankings are plain `GROUP BY`/`ORDER BY`, durable history with ordinary backups, no query window, built-in core Grafana datasource with `$__timeFilter` macros — and it's already in the project. Batched inserts (COPY / multi-row) during runs.
- **ClickHouse** (runner-up): best-in-class quantiles, but the heaviest container in the compose (~0.5–1 GB idle untuned; docs assume big machines) and a small-insert anti-pattern to manage — overkill at this scale. **TimescaleDB**: optimizations we don't need at this scale. **VictoriaMetrics/Prometheus**: confirmed anti-pattern (per-run_id labels = high-cardinality churn, approximate quantiles only, no raw events). **DuckDB/Parquet**: great for *static post-run reports*, weak as a live Grafana backend (unsigned plugin, single-writer).
- Also considered: **MongoDB** (already in the project, but percentile/ranking analytics in aggregation pipelines are clumsy vs SQL and Grafana's Mongo support is weaker); **SQLite/DuckDB as live store** (single-writer, no server for Grafana to query mid-run — fine for post-run reports only).
- Peer validation: TechEmpower keeps `results.json` + a custom viewer; sharkbench writes CSVs; k6 recommends Prometheus→Grafana live + HTML export post-run. Nobody at this scale runs a heavy analytics DB.

**Why Postgres specifically, in one paragraph**: the workload is *small-scale relational OLAP* — millions of rows at most, queried by exact `run_id`/`server`/`endpoint` equality, needing exact percentiles and rankings. That is the textbook profile of a boring SQL database. Postgres does it *exactly* (`percentile_cont`, window functions), is already operated in this repo, costs one small container, has the most battle-tested Grafana datasource in existence, and imposes zero data-model contortions (no tags-vs-fields, no cardinality budgets, no retention windows). Every alternative is either a specialized engine whose specialization we don't use (ClickHouse: columnar scale; Influx/VM: high-frequency ingest with recent-data bias) or fails a hard requirement (live queries, history, exact quantiles). When no requirement demands a specialized tool, the general boring one wins.

**Decisions**: (1) metrics go to a **dedicated `metrics-postgres` container** in the grafana compose stack — *never* the benchmarked postgres instance, which must stay uncontaminated; (2) the writer swap is contained — `internal/influx` becomes `internal/metrics` with the same call sites; (3) schema: `runs`, `request_events` (sampled), `endpoint_stats`, `sequence_stats`, `resource_samples` — tags become plain indexed columns, killing the fake-timestamp hack for free; (4) `results/*.json` stays the durable, versioned source of truth.

### 9.2 How we query & present results

- **Query contract, documented in the repo**: dashboards read only **aggregate tables** (`endpoint_stats`, `sequence_stats`, `resource_samples`, throughput); raw `request_events` is drilldown-only. Canonical queries (per-run ranking, cross-run diff, saturation curve) live as `.sql` files in `grafana/queries/` so they're reviewable and reusable — dashboards reference them, not ad-hoc copies.
- **Real timestamps** on rows; `run_id` remains the primary selector everywhere (indexed column, not a tag).
- **Presentation layers** (in order of truth): 1) `results/<timestamp>/*.json` — durable, versioned, source of truth; 2) terminal summary tables (exists, gets RPS + capacity added); 3) Grafana dashboards (live exploration, §9.3); 4) *(Phase 4 nice-to-have)* a generated static report — one HTML/PNG per run from the JSONs, for the README results section, so published numbers don't depend on a running Grafana.

### 9.3 Grafana dashboards

- **Remove all per-server hardcoded overrides** — palette-by-series-name gives new servers colors automatically; delete the `max: 11` axis cap.
- Variables: `run_id`, `suite`, `endpoint` (**multi-select**), `language`, `runtime`, `server` (multi), `experimental` filter — all powered by the new tags.
- Dashboard split:
  1. **Overview** — RPS + p50/p95/p99 ranking, filter by suite/language/runtime
  2. **Endpoint drilldown** — one endpoint across servers, latency distribution over run time
  3. **Resources** — CPU/mem of servers *and* DB containers
  4. **Databases** — CRUD sequence timings per DB engine
  5. **Run compare** — two `run_id`s side by side (e.g. Go 1.26 vs 1.27rc, Node vs Bun vs Deno for the same framework)

---

## 10. Version targets (verified July 2026)

| Stack | Target |
| --- | --- |
| Node.js | **26.4.x** (Current) |
| Bun | **1.3.14** |
| Deno | **2.9.1** |
| Go | **1.27rc1** (toolchain directive; stable is 1.26.4) |
| Python | 3.14.x · FastAPI latest · Django 6.x · Flask 3.x |
| Rust | latest stable · Axum + Actix Web latest |
| Zig | **0.16.0** |
| Kotlin | latest · Ktor 3.x · Spring Boot latest |
| TS libs | express 5.2 · fastify 5.9 · nestjs 11.1 · hono 4.12 (+`@hono/node-server` 2.x, needs Node ≥20) · elysia 1.4 (+`@elysiajs/node` in lockstep) · oak 17 (**from JSR**) · `postgres` 3.4 · drizzle 0.45 (pin — 1.0 still rc) · mongodb 7.4 · ioredis 5.11 · cassandra-driver 4.9 |
| Tooling | **Latest versions, pinned**: just, biome, prettier, golangci-lint, ruff, pyright, rustfmt/clippy (ride the Rust toolchain), ktlint, detekt, sqlc, drizzle-kit, uv, pnpm — checked/bumped in Phase 0 alongside runtime deps |
| `just update` | run across all stacks as part of Phase 0; extend it to also cover the lint/format tooling above |

---

## 11. Execution phases

**Phase 0 — Foundation (~40% of effort)**
1. Folder restructure (`apps/`, `shared/`) + workspaces (pnpm/go.work/uv/cargo/gradle) + Docker root-context builds + justfile rework + lint/format consolidation (one strict config per language, latest tool versions)
2. Shared extraction: TS (`@bench/shared`, pg→postgres, runtime adapters), Go (shared module), Python (async+sync)
3. Hono multi-runtime entrypoints (node/bun/deno) + port convention + single-process FastAPI + pool normalization + Bun shutdown fix
4. **Conformance suite** + negative cases → run against all entries (validates the extraction was a pure move; smoke-tests risky runtime×driver combos)

**Phase 1 — Client v2**
testcontainers-go · RPS · open model + ramping + backpressure accounting · no-silent-drops rules · suites/groups selection · new tags · DB-container sampling · runtime config validation

**Phase 2 — New endpoints**
`web` suite (html/jwt/validate/compute) in all existing servers + config cases + conformance cases

**Phase 3 — New servers** (each gated by conformance)
Echo → Rust (axum, actix) → Python (django, flask) → Kotlin (ktor, spring-boot) → Zig (postgres+redis first, then mongo via libmongoc, cassandra via zig-cassandra/cpp-driver)

**Phase 4 — Grafana redesign**
5 dashboards, tag-driven, zero per-server hardcoding · README refresh (stack map, results)

---

## 12. Risks & open items

- **Deno × pnpm workspace friction**: Deno needs a mirrored `deno.json` workspace list + `--node-modules-dir=manual`; verified in docs, not in this repo — prototype first in Phase 0.
- **cassandra-driver on Bun/Deno unverified upstream** — conformance decides (affects bun-elysia, bun/deno-hono, deno-oak).
- **Zig MongoDB via libmongoc** is the single biggest new-server effort item (C toolchain in Docker build).
- **Bun/NestJS regressions** — NestJS stays Node-only for now; revisit when official support lands.
- **Metrics migration Influx→Postgres** happens in Phase 1 with the writer swap — until then dashboards keep working on Influx; `results/*.json` is the source of truth throughout. ClickHouse remains the documented exit if event volume ever outgrows Postgres (§9.1).
- **Run-time budget**: 20 servers × 5 suites × 4 DBs is hours — selectable suites is the mitigation; a `quick` suite preset for development, full matrix for publish runs.
- **Benchmark fairness on one machine**: generator, DBs, and SUT share the host. Out of scope for this plan, but resource-limit the generator and document the caveat in README.
