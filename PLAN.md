# Benchmark Platform Expansion Plan

Status: **planning approved-in-progress** · Last updated: 2026-07-03

---

## 0. Locked decisions

| Topic                  | Decision                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Run scope              | **Selectable suites** — config defines endpoint suites + server groups; CLI picks. Full matrix possible, not default.                                                                                                                                                                                                                                                                                                              |
| TS sharing             | **Max sharing of infrastructure** — one shared DB layer / schemas / consts for all TS apps; handlers stay idiomatic per framework (see "Framework idioms" below).                                                                                                                                                                                                                                                                  |
| New servers            | **Full target roster, implemented incrementally** — Django, Flask, Zig, Kotlin (Ktor + Spring Boot), Rust (Axum + Actix), Go Echo. Each lands only after the contract gate exists.                                                                                                                                                                                                                                                 |
| Contract tests         | **First implementation slice** — build the conformance/contract gate against the current 10 servers before folder moves, shared extraction, driver swaps, or new servers. Every later server/refactor must pass it.                                                                                                                                                                                                                |
| "Playground"           | Folded into the `POST /validate` endpoint (heavy validation à la Zod in every language). Not a separate feature.                                                                                                                                                                                                                                                                                                                   |
| Framework idioms       | **Idiomatic code and ecosystem conventions everywhere** — each framework/language is written the way its community writes production code (Django ORM, NestJS DI + modules, Spring annotations, Cargo/Gradle layouts, etc.). **Sharing stops where idiom starts**: shared packages hold infrastructure (DB clients, schemas, validation rules, constants, config); routing/handlers/app structure are per-framework and idiomatic. |
| Zig                    | **One server** (http.zig), **all 4 databases**, no shared layer (single implementation).                                                                                                                                                                                                                                                                                                                                           |
| FastAPI workers        | Normalize to **single-process** like all other servers (fairness; current `--workers 4` is the biggest asymmetry).                                                                                                                                                                                                                                                                                                                 |
| TS runtimes            | **Hono is the single multi-runtime TS app** (Node 26 + Bun + Deno — the only framework officially first-class on all three, see §4). All other TS frameworks stay on their home runtime: express/fastify/nestjs → Node, elysia → Bun, oak → Deno.                                                                                                                                                                                  |
| Server layout          | **Flat `servers/` with prefixed folder names** (folder = entry = image; Node unprefixed for TS): ts-express, ts-bun-elysia, go-chi, py-fastapi, rs-axum, … — see §2.1 (revised 2026-07-03).                                                                                                                                                                                                                                        |
| TS postgres driver     | Switch `pg` → **`postgres` (postgres.js)** via `drizzle-orm/postgres-js`.                                                                                                                                                                                                                                                                                                                                                          |
| Go version             | **1.27rc1** (confirmed available) via `toolchain` directive, everywhere.                                                                                                                                                                                                                                                                                                                                                           |
| Task runner            | **just stays** (no Makefile). We need a command runner, not a build system — incremental builds belong to each language's toolchain. Note `just` install in README for contributors.                                                                                                                                                                                                                                               |
| Lint/format            | **Strict on correctness, default on style** — formatters at ecosystem defaults (they ARE the convention), linters strict and merge-gating via `just verify` / `just contract`. **One config per language** at the language root (no per-server copies). All lint/format tools pinned to **latest versions**.                                                                                                                       |
| Metrics stack          | **Switch InfluxDB → dedicated PostgreSQL** (metrics instance, separate from the benchmarked one); **keep Grafana**, upgrade to 13.x. Researched decision — see §9.1.                                                                                                                                                                                                                                                               |
| Client queue           | **No broker** (no Kafka/Rabbit/NATS/BullMQ) — in-process bounded channels; see §7.5.                                                                                                                                                                                                                                                                                                                                               |
| Client & orchestration | **Keep the custom Go client as both generator and orchestrator** (validation + sequences + lifecycle are the project's value); **no local K8s** (noise + complexity for zero benefit single-node); generator correctness guarded by a **cross-validation gate vs oha/k6**; see §7.6.                                                                                                                                               |
| Client flags           | **Minimal — flags select, config configures.** `config/config.json` is the single source of behavior, schema-validated at startup; see §7.4.                                                                                                                                                                                                                                                                                       |
| Git workflow           | **Feature branches + PRs, no direct pushes to `main`.** One PR per small phase-slice, reviewed (incl. a fresh-context reviewer for risky diffs); see §0.2.                                                                                                                                                                                                                                                                         |
| Toolchains             | Installed & pinned per §0.1 — notably **Go 1.27rc1 as a separate `go1.27rc1` binary** (stable Go untouched), Node via **fnm**, Rust via keg-only **brew rustup** (PATH quirk), Kotlin via **Gradle wrapper** (no system compiler).                                                                                                                                                                                                 |

---

## 0.1 Prerequisites & toolchain notes (installed 2026-07-03, macOS arm64)

All toolchains installed and verified. Operational specifics that affect how the repo is built — capture these in the scripts/justfile/README during Phase 0:

- **Go 1.27rc1 — separate binary, not a replacement.** Installed via `go install golang.org/dl/go1.27rc1@latest && go1.27rc1 download` → `~/go/bin/go1.27rc1`; stable `go` (1.26.4, Homebrew) is untouched. Every Go go.mod (chi/gin/fiber/benchmark) declares `go 1.27rc1` (`go mod tidy` normalizes away a same-version `toolchain` line), so plain `go` under the default `GOTOOLCHAIN=auto` auto-downloads and uses the rc; `scripts/lib.mts` additionally pins `GOTOOLCHAIN=go1.27rc1` for every spawned Go command, and the Docker builders use `golang:1.27rc1-alpine`. `go get -u ./... && go mod tidy` (what `just update` runs for Go) preserves the directive — the pin survives blanket updates (verified).
- **encoding/json/v2 is in Go 1.27rc1's default baseline** — `encoding/json/v2` and `encoding/json/jsontext` compile with **no `GOEXPERIMENT` set** (only `GOEXPERIMENT=none` excludes them; verified against the local sdk and the official `golang:1.27rc1-alpine` image). The repo intentionally carries **zero GOEXPERIMENT references**: if a later RC flipped the baseline, the `encoding/json/v2` imports fail loud at compile and the flag gets re-added deliberately. Adoption: chi uses `json.MarshalWrite`/`json.UnmarshalRead` directly; fiber wires v2 through its configurable `JSONEncoder`/`JSONDecoder`; **gin keeps `gin.Context` helpers out of the render path by design** — its render engine only swaps bundled encoders via build tags (sonic/go-json), so stdlib v2 is wired as thin handler-level helpers (`utils.WriteResponse`/`utils.BindJSON`) rather than forking gin internals. All three decode with `jsontext.AllowDuplicateNames(true)` so duplicate keys keep the last-wins JSON.parse semantics of the JS/Python stacks; v2's case-sensitive field matching is kept (v1 Go was the cross-server outlier, silently accepting case-mismatched keys).
- **golangci-lint rides the Go toolchain.** The brew bottle is built with go1.26 and refuses to load any module targeting Go 1.27 ("the Go language version used to build golangci-lint is lower than the targeted Go version"). A rebuild lives at `~/go/bin/golangci-lint` — `GOTOOLCHAIN=go1.27rc1 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2` (keep the version pinned to brew's) — and `scripts/lib.mts` prepends `~/go/bin` to PATH for spawned commands so it outranks the brew binary in all `just verify`/`just lint`/`just fmt` runs (the brew install stays linked for everything else). Restore brew-only management once Go 1.27 is stable and brew's bottle is rebuilt against it: `rm ~/go/bin/golangci-lint` (and if it was ever `brew unlink`ed, `brew link golangci-lint`). `just install` does **not** automate this rebuild — on a fresh machine the brew bottle lints first and fails loud ("the Go language version used to build golangci-lint is lower than the targeted Go version"); that failure is the signal to run the `go install` above.
- **Node 26.4.0 via fnm** (`fnm install 26 && fnm default 26`). Each TS app pins with a `.node-version` file (fnm auto-switches on `cd` if `--use-on-cd` is enabled). Node 26 is Current (LTS ~Oct 2026) — fine for a benchmark rig.
- **Rust 1.96.1 via Homebrew `rustup`** (the `rustup` formula, **not** `brew install rust`). Formula is **keg-only and no longer ships `rustup-init`**; bootstrap was `rustup default stable`. Proxies (`cargo`/`rustc`/`rustfmt`/`cargo-clippy`) live in **`/opt/homebrew/opt/rustup/bin`**, which must be on `PATH` (there is **no `~/.cargo/bin`**). README setup and scripts must export this path; CI should do the same if added later.
- **Zig 0.16.0 via `brew install zig`** — brew is exactly at 0.16.0 (no lag). Pulls llvm@21/lld@21 as deps.
- **Deno 2.9.1 via `brew install deno`** — but **upgrade with `deno upgrade`, not brew**.
- **Kotlin: no system compiler.** `brew install gradle` → **Gradle 9.6.1 bundles Kotlin 2.3.21**; projects build via the Gradle wrapper (`./gradlew`). ⚠️ Gradle's launcher JVM is now **openjdk 26** (brew dep), but Spring Boot's supported ceiling is lower — **Kotlin projects pin a supported JDK via Gradle's `toolchain` block** rather than inheriting 26. JDK 21 is also present.
- **Already correct**: Bun 1.3.14, uv 0.11.25, pnpm 10.29.1, just 1.54.0, Docker 29.3.0.

## 0.2 Git workflow

- **`main` is protected in practice**: no direct feature pushes. Only the planning docs already on `main` were pushed directly (pre-decision); from here, all changes land via PR.
- **One PR per reviewable slice**, smaller than the old phase buckets (e.g. `phase0/contract-current`, `phase0/scripts`, `phase0/restructure`, `phase0/shared-ts-extract`, `phase0/ts-postgres-driver`, `phase1/client-metrics-pg`). Branch naming: `phase<N>/<slug>`.
- **Gate is local-first**: run `just verify` (typecheck + format-check + lint) and — once it exists — `just contract <touched server>` before opening/merging a PR. Optional convenience: a local `pre-push` git hook that runs the same gates. A tiny CI can be added later if useful, but it should only run the same commands rather than inventing a second gate.
- **Risky/security-sensitive PRs get a fresh-context review** (correctness + requirement gaps), per the repo's working agreement.

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

1. **Duplication is extreme and literal**: DB/repository layer is byte-identical copy-paste — 3× in Go, ~6× in TS. This makes extraction a pure _move_ (verifiable by conformance suite), not a rewrite.
2. **Fairness asymmetries**: FastAPI runs 4 uvicorn workers (everyone else single-process); Python pg pool is 20+40 vs 50 elsewhere; Bun servers use native `RedisClient`/`randomUUIDv7`; Bun servers never stop `Bun.serve` on shutdown.
3. **Client gaps**: closed-loop fixed-VU only (coordinated omission → understated tail latency); **no RPS/throughput metric at all**; docker CLI shell-outs; host port hardcoded 8080 (no parallel servers); Influx URL/token hardcoded; **metrics silently dropped** if Influx is down; DB reset only once per server; no DB-container resource sampling.
4. **Grafana won't scale**: per-server colors/labels hardcoded in ~140 lines of JSON overrides; axis capped at `max: 11`; endpoint variable single-select. Queries themselves (`GROUP BY server`) scale fine.
5. **No workspaces anywhere**: root package.json is not a pnpm workspace; Go servers are 4 unrelated modules; the copy-paste follows from this.

---

## 2. Target architecture

### 2.1 Folder structure

**Flat `servers/` with prefixed names** (revised 2026-07-03; supersedes the earlier per-language subdir sketch). Every server is a self-contained island with its own toolchain, so language subdirs bought nothing: per-language lint/format configs anchor at the repo root (all the tools search upward), the workspace roots live at the repo root anyway, and discovery gets _simpler_ (one-level `servers/*/bench.json`). Flat + prefixes gives one identity everywhere — folder name = entry name = image name — and `ls servers/` reads as the roster.

```
servers/
  ts-express/  ts-fastify/  ts-nestjs/      # Node is the default TS runtime — unprefixed
  ts-bun-elysia/  ts-deno-oak/              # non-node runtimes named explicitly
  ts-honojs/  ts-bun-honojs/  ts-deno-honojs/   # 3 thin runtime entries; the Hono app itself lives in shared/ (§4)
  go-stdlib/  go-chi/  go-gin/  go-fiber/  go-echo/
  py-fastapi/  py-django/  py-flask/
  rs-axum/  rs-actix/
  kt-ktor/  kt-spring-boot/
  zig/                                      # single server: http.zig, all 4 DBs
benchmark/                      # the Go client (moved from benchmarks/)
shared/
  typescript/                   # @bench/shared — db ops, zod schemas, env, consts (+ the shared Hono app)
  go/                           # module: shared db/config/consts/validation
  python/                       # bench-shared: async + sync repository impls
  rust/                         # shared crate (workspace member)
  kotlin/                       # shared Gradle module
contract/                       # language-neutral API spec + conformance cases (JSON)
scripts/                        # typed orchestration scripts; justfile stays thin
config/  infra/  grafana/  test-files/  results/
```

Folder = entry = image (`servers/go-chi` ↔ entry `go-chi` ↔ image `bench/go-chi`). Per-language tool configs live at the repo root (`.golangci.json`, `ruff.toml`, `biome.json`, …) — still exactly one per language, discovered upward by each tool.

### 2.2 Workspace tooling — native per language, `just` as the umbrella

No Nx/Turborepo, and **explicitly not Bazel/Buck2/Pants** (considered and rejected): there is no build graph to optimize, just "N apps → 1 shared package" per language. The hermetic-build tools are rejected because — (1) the languages are **islands** (no Go→Rust cross-target deps), so Bazel's one-cross-language-graph superpower has nothing to bite on; (2) they **contradict the "idiomatic everywhere" decision** — replacing `cargo`/`bun`/`deno`/`gradle`/`zig build` with `BUILD` files makes each server _non-idiomatic_, the exact anti-pattern the repo avoids, and less representative as a benchmark; (3) **poor/no rules for the exotic members** (Zig especially; Bun/Deno fight rules_js/pnpm), meaning custom-rule maintenance for the hardest part; (4) **incrementality is already per-language** (build caches for Go/cargo/Gradle/tsc/Zig) and outputs are Docker images from idiomatic Dockerfiles; (5) it's a **hobby project** (CI was already cut as overkill — these are a far bigger tax). They'd only pay off at large-team scale with genuinely cross-language shared builds and remote-execution needs — none present.

| Language   | Mechanism                                    | Notes                                                                                                                                                                                                                                                                                                                                                             |
| ---------- | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| TypeScript | **pnpm workspace** (the only workspace)      | The one language where a workspace earns its keep: shared's runtime deps (drizzle/mongodb/ioredis/cassandra-driver) install once and resolve via `workspace:*`; `file:`/`link:` would re-create version-skew duplication. Bun consumes pnpm-installed `node_modules` fine. **Deno does not read `pnpm-workspace.yaml`** — root `deno.json` mirrors the workspace with `nodeModulesDir: "manual"` (spiked in 0B). **0D acceptance: shared TS code must be proven consumed from all three runtimes (node/bun/deno) via the contract gate before the lane closes.** |
| Go         | **`go.mod` `replace` per module**            | `replace <shared-module> => ../../shared/go` committed in each server + `benchmark/`; `go.work` stays optional local DX (gitignored), never load-bearing                                                                                                                                                                                                            |
| Python     | **uv path sources**                          | `[tool.uv.sources] bench-shared = { path = "../../shared/python" }` per server                                                                                                                                                                                                                                                                                      |
| Rust       | **Cargo path deps**                          | `shared = { path = "../../shared/rust" }` — fully idiomatic; no workspace needed (one shared lockfile/target dir is the only thing it would add)                                                                                                                                                                                                                    |
| Kotlin     | **Gradle multi-project**                     | `:shared`, `:ktor`, `:spring-boot` — Gradle has no idiomatic no-workspace alternative                                                                                                                                                                                                                                                                               |
| Zig        | none needed                                  | single self-contained app                                                                                                                                                                                                                                                                                                                                           |

(Revised 2026-07-03, supersedes "workspaces for all five": each language uses its most idiomatic sharing mechanism — a workspace only where it solves a real problem (TS). Docker root-context builds are unaffected: each Dockerfile COPYs `shared/<lang>` + its app and relative paths resolve identically in-image.)

### 2.3 Task scripts — thin justfile over typed `.mts` orchestrators

The current justfile hides big per-framework bash `case` blocks inside `install`/`update`/`verify`/`dev`/`images` — hard to read, hard to extend as the roster grows to 20+ servers. Move dispatch and orchestration into a **`scripts/` folder of typed `.mts` scripts** (pattern proven in the user's `lets-go` repo), keeping `just` as a thin command menu:

- Run via **Node 26 native TypeScript type-stripping** — `node scripts/verify.mts`, **no tsx, no build step**.
- Each script is a **declarative table** (e.g. `CHECKS` / `TARGETS`) that shells out to per-language/per-server commands, runs them **concurrently**, and prints **one grouped report** instead of bailing on first failure (`a && b && c` hides later errors).
- Adding a server/target = **one row**, not a new bash branch — mirrors the manifest-driven discovery in §7.4 (scripts can even read the same `bench.json` manifests, so the roster has one source of truth).
- Recipes become one-liners: `verify target='all': node scripts/verify.mts {{target}}`. Complex flags/logic live in typed TS, not brittle just/bash.
- Scope: many small scripts, not one giant dispatcher: `verify.mts`, `format.mts`, `lint.mts`, `install.mts`, `update.mts` (pin-aware, §10), `images.mts`, `dev.mts`, `contract.mts`, `check-config.mts`, and later report/dashboard helpers. Genuinely-simple recipes (`db-up`, `grafana-up`) stay inline in the justfile.
- `verify` must be non-mutating: it runs format **checks**, type/build checks, and linters. Write-formatting stays available as `just fmt <target>` / `scripts/format.mts`, but it is not part of the merge gate.
- **`scripts/contract.mts` is the server contract harness**: given an entry or manifest, it builds or finds the server image, starts the server in a container with the same env/DB dependencies used by benchmarks, waits for `/health`, runs the Go client's conformance command against the mapped port, streams a concise failure report, and tears the container down. `just contract <entry>` is the normal gate; `just conformance <entry>` can remain as an alias if desired.

### 2.4 Docker build contexts

Shared folders force build context above the app dir. Convention: **build from repo root**, `docker build -f servers/go-chi/Dockerfile .` — each Dockerfile copies `shared/<lang>` + its app. `just images` updated accordingly. `.dockerignore` at root keeps contexts small.

**Ignore files must grow with the new languages** (Phase 0C task). Root context = a root `.dockerignore` is load-bearing (a fat context slows every image build). Both `.gitignore` and root `.dockerignore` need the per-language artifacts:

| Source           | Artifacts to ignore                                                                                                                                                                        |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Rust             | `target/` (commit `Cargo.lock` for app crates)                                                                                                                                             |
| Zig              | `.zig-cache/`, `zig-out/`                                                                                                                                                                  |
| Kotlin/Gradle    | `.gradle/`, `build/`, `**/bin/` — but **keep `gradle/wrapper/gradle-wrapper.jar`** (`!` rule)                                                                                              |
| Python           | `__pycache__/`, `*.pyc`, `.venv/`, `.pytest_cache/`, `.ruff_cache/`, `.mypy_cache/`                                                                                                        |
| Go               | `bin/`, `tmp/` (air live-reload)                                                                                                                                                           |
| TS/Deno          | `node_modules/`, `dist/`, Deno-generated `node_modules/` under `--node-modules-dir=manual`                                                                                                 |
| Benchmark output | **split published vs scratch** — version curated `results/published/**/*.json`; ignore scratch/local run output (`results/runs/`, tmp dirs) so machine-specific runs do not churn the repo |

`.dockerignore` additionally excludes globally (independent of git): `.git/`, all of the above, `*.md`, `grafana/`, `infra/` volumes, and scratch `results/`. Do **not** rely on one root `.dockerignore` to exclude "other apps' source" differently for each Dockerfile — Docker ignore rules are context-wide, not per target. If context size becomes a real problem, use Dockerfile-specific ignore files (`servers/.../Dockerfile.dockerignore`) or BuildKit named contexts; otherwise prefer a simple root context that is correct over a clever one that can accidentally omit needed shared files.

---

## 3. Shared code strategy per language

**Guiding principle — share infrastructure, keep app code idiomatic.** The shared packages contain what is framework-independent by nature: DB clients + repositories, data types, validation schemas/rules, constants, env parsing. Everything the framework has an opinion about — routing, handlers, middleware wiring, DI, project layout — is written per-framework in that framework's canonical production style. If sharing a piece would force a framework out of its idiom (NestJS services/DI, Django ORM views, Spring controllers), it is not shared. Each language follows its ecosystem's conventions and tooling, under one policy — **formatters at defaults, linters strict, one config per language root, latest tool versions, all merge-gating through `just verify` + `just contract`**:

| Language   | Formatter (ecosystem defaults)    | Linter (strict)                                                                                       |
| ---------- | --------------------------------- | ----------------------------------------------------------------------------------------------------- |
| TypeScript | biome format                      | biome recommended + current extra rules (single root config — today's per-server configs consolidate) |
| Go         | golangci-lint fmt (gofmt/gofumpt) | golangci-lint, curated linter set in one shared `.golangci` config                                    |
| Python     | ruff format                       | ruff (wide rule selection) + pyright **strict mode**                                                  |
| Rust       | rustfmt (untouched)               | clippy with `-D warnings`                                                                             |
| Kotlin     | ktlint                            | detekt                                                                                                |
| Zig        | `zig fmt --check`                 | (compiler is the linter)                                                                              |

### TypeScript — `@bench/shared`

- **DB clients + repositories**: single implementation. Extract first with the existing behavior/driver under the contract gate; switch `pg` → `postgres` (postgres.js) through `drizzle-orm/postgres-js` in a separate PR so a driver swap is not hidden inside a move-only refactor. Mongo (`mongodb`), Redis (`ioredis`), Cassandra (`cassandra-driver`).
- **Runtime adapters**: portable impl is the default; Bun-native bits (`Bun.RedisClient`, `randomUUIDv7`) become injectable adapters chosen by the entrypoint, so Bun entries keep their native edge while sharing everything else.
- **Zod schemas, env parsing, consts/errors**: moved verbatim (already byte-identical).
- **Build split — servers use `tsc`, shared uses tsdown.** The **shared package** builds with **tsdown** (rolldown/oxc-based, tsup successor) → **ESM + `.d.ts`**, not consumed as raw source: Bun/Deno/tsx _could_ import source directly, but NestJS builds via tsc and wants real declaration files, and the TS 7 native `tsc` typecheck resolves cleaner against emitted `.d.ts` than deep source across workspace refs. One built artifact → every runtime consumes the shared layer identically (reinforces "same code everywhere"). The **server apps compile/typecheck with `tsc`** (TS 7) against that built `.d.ts` — NestJS emits to `dist/`, the others typecheck `--noEmit` and run via their runtime (tsx/Bun/Deno). Cost: a build step in shared (tsdown `--watch` in dev). tsdown pinned like the rest.

- **TypeScript 7.0 RC (native compiler) — adopt** (researched, July 2026). Install `typescript@rc` (= **`typescript@7.0.1-rc`**; `latest` is still 6.0.3, so **pin it** — a blanket update would clobber it). The native binary is now named **`tsc`** (the old `tsgo` name was dropped at RC). Adoption rules: (1) **use native `tsc --noEmit` as the typecheck gate across all TS projects** — safe, at parity, ~10× faster; (2) **NestJS emit**: native `tsc` now does full JS + `.d.ts` emit, so it _can_ build `dist/` — validate the declaration output once, else keep 6.x `tsc` for NestJS's emit step and native for `--noEmit`; (3) **do not point `ts-node` at TS 7** (no stable programmatic API until 7.1) — moot for us, we run via `tsx`; (4) **Bun/Deno runtimes are unaffected** — they transpile with their own toolchains and never read the `typescript` package (Deno's `deno check` uses its bundled TS), so TS 7 is purely an optional external typecheck there. Validate each project's typecheck/build once when flipping the switch (no official per-tool compat matrix exists).
- **Routing/handlers stay per-framework and idiomatic** (Express routers, Fastify plugins + its schema hooks, NestJS modules/controllers/services with DI, Hono/Elysia app chains, Oak router) — they call the shared repositories and Zod schemas.

### Go — `shared` module

Move `internal/database` (+ sqlc output), `config`, `consts`, and validation into one module. Framework dirs keep idiomatic router wiring, middleware, and handlers (stdlib/chi/gin/fiber/echo each in their canonical style). Add **Echo** and **stdlib** (`net/http` with the ≥1.22 pattern-matching `ServeMux` — the zero-dependency baseline every Go framework is measured against; router idiom is `mux.HandleFunc("METHOD /path/{param}", …)` + `r.PathValue`). `go.work` ties it together.

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

| Framework                                              | Node 26                                                      | Bun 1.3                                                   | Deno 2.9                                   | Ship?                              |
| ------------------------------------------------------ | ------------------------------------------------------------ | --------------------------------------------------------- | ------------------------------------------ | ---------------------------------- |
| Hono 4 (`@hono/node-server` v2)                        | ✅ official                                                  | ✅ official (`export default app`)                        | ✅ official (`Deno.serve(app.fetch)`)      | **node-hono, bun-hono, deno-hono** |
| Elysia 1.4 (`@elysiajs/node` 1.4.5)                    | ⚠️ official adapter, youngest — history of lockstep breakage | ✅ home runtime                                           | ❌ no adapter (web-standard mode untested) | **bun-elysia only**                |
| Express 5                                              | ✅ native                                                    | ✅ Bun claims full support                                | ✅ via `npm:express`                       | **node-express only**              |
| Fastify 5                                              | ✅ native                                                    | ✅ node:http fully implemented (not in Fastify CI)        | ⚠️ compat-only; Fastify won't support Deno | **node-fastify only**              |
| NestJS 11                                              | ✅ native                                                    | ⚠️ real regressions (e.g. bun#27526), no official support | ⚠️ community-only, "not production-ready"  | **node-nestjs only**               |
| Oak 17 (**JSR** `@oak/oak`; npm copy is stale at 14.1) | ⚠️ official but no TLS/`.send()`/WS                          | ⚠️ same as Node                                           | ✅ home runtime                            | **deno-oak only**                  |

**Decision: Hono is the single multi-runtime TS app** — it is the only framework with first-party support on all three runtimes, which makes it the clean runtime-vs-runtime comparison (same app, same code, three runtimes). Every other framework ships on its home runtime only, avoiding the ⚠️ compat-layer tier entirely. → **8 TS entries** (was 6): the 5 home-runtime apps + hono×3. Layout (per the flat scheme, §2.1): the Hono app itself lives in `shared/typescript/` and the three entries — `ts-honojs` (node), `ts-bun-honojs`, `ts-deno-honojs` — are thin per-runtime server folders consuming it.

**Driver caveat to smoke-test in conformance**: `cassandra-driver` on Bun (2023-era failures, current status unverified) and on Deno (never tested upstream) — relevant to bun-elysia, bun-hono, deno-hono, deno-oak. postgres.js / mongodb / ioredis are confirmed fine on all three.

---

## 5. New endpoints

| Endpoint          | Suite | What it exercises                                            | Contract sketch                                                                                                                                     |
| ----------------- | ----- | ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /html`       | `web` | server-rendered HTML template                                | `200 text/html`, small dynamic template (name + list + number interpolation)                                                                        |
| `GET /jwt/sign`   | `web` | HS256 sign                                                   | `200 {"token": "..."}` — fixed claims + exp, shared secret via env                                                                                  |
| `GET /jwt/verify` | `web` | HS256 verify + header parsing                                | `Authorization: Bearer <t>` → `200 {payload}` / `401 {"error":"invalid token"}`                                                                     |
| `POST /validate`  | `web` | heavy validation (Zod / Pydantic / validator / serde / etc.) | deep nested object (~4 levels, arrays, enums, email/uuid/range rules) → `200 {"valid":true}` / `400` with error count; pass **and** fail variations |
| `GET /compute?n=` | `web` | pure CPU (isolates runtime from I/O)                         | e.g. iterative SHA-256 chain of n rounds → `200 {"result": "<hex>"}` , n capped                                                                     |

Existing 16 routes are unchanged. Suites: `basic` (root/health), `params`, `web` (new), `db` (per-database CRUD).

---

## 6. Server roster & port convention

### Roster (final: 21 entries)

Entry names = folder names (flat `servers/` scheme, §2.1): Node is the default TS runtime and goes unprefixed; bun/deno are explicit.

| Language       | Entries                                                                                                     |
| -------------- | ----------------------------------------------------------------------------------------------------------- |
| TypeScript (8) | ts-express, ts-fastify, ts-nestjs, ts-deno-oak, ts-bun-elysia, **ts-honojs, ts-bun-honojs, ts-deno-honojs** |
| Go (5)         | **go-stdlib** (net/http), go-chi, go-gin, go-fiber, **go-echo**                                             |
| Python (3)     | py-fastapi, **py-django**, **py-flask**                                                                     |
| Rust (2)       | **rs-axum**, **rs-actix**                                                                                   |
| Kotlin (2)     | **kt-ktor**, **kt-spring-boot**                                                                             |
| Zig (1)        | **zig** (http.zig)                                                                                          |

### Zig server (researched, July 2026)

Zig **0.16.0 stable** (Apr 2026). Stack — all four databases, mixed pure-Zig/C:

| Piece     | Choice                                                                  | Status                                                           |
| --------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------- |
| HTTP      | **http.zig** (karlseguin)                                               | 0.16-native, actively maintained, the de-facto production server |
| Postgres  | **pg.zig** (same author)                                                | 0.16-native, has `pg.Pool` — solid                               |
| Redis     | **okredis** (verify 0.16 build) or hand-rolled RESP client (~150 lines) | easy either way                                                  |
| Cassandra | **zig-cassandra** (pure Zig, updated at 0.16 release — build-verify)    | fallback: apache cassandra-cpp-driver (C API) via `@cImport`     |
| MongoDB   | **libmongoc via C interop** — no living Zig driver exists               | the one hard item; drags a C toolchain into the Docker build     |

Blocking C clients are fine under http.zig's thread-per-worker model — no architectural blocker. Estimated 4–7 days.

### Port convention

Two rules replace the current ad-hoc list:

1. **Inside containers, every server listens on the same canonical port: `8080`** (via `PORT` env in the Dockerfile). The benchmark client maps a host port dynamically (frees us from hardcoded 8080 and enables parallel servers later). Config drops per-server `port`.
2. **All host ports live in the `2LRFF` block** (revised 2026-07-03 — the old 3011–8020 span collided with common dev tooling; 20000–29999 collides with nothing). Five digits, each position meaningful, derivable never looked up:

```
2 L R FF      2  = the bench block (everything this repo binds on the host)
              L  = language     0=infra/DBs · 1=Go · 2=TS · 3=Python · 4=Rust
                                5=Kotlin · 6=Zig · 7+=future (.NET, Swift, …)
              R  = runtime      TS: 0=node 1=bun 2=deno · others: 0
              FF = framework    stable per framework ACROSS runtimes (hono is
                                always 05, whatever the runtime)

Infra  = 20001 postgres · 20002 mongodb · 20003 redis · 20004 cassandra
         20090 grafana · 20091 metrics-postgres
Go     = 21001 go-stdlib · 21002 go-chi · 21003 go-gin · 21004 go-fiber · 21005 go-echo
TS     = node  22001 ts-express · 22002 ts-nestjs · 22003 ts-fastify · 22005 ts-honojs
         bun   22105 ts-bun-honojs · 22106 ts-bun-elysia
         deno  22204 ts-deno-oak · 22205 ts-deno-honojs
Python = 23001 py-fastapi · 23002 py-django · 23003 py-flask
Rust   = 24001 rs-axum · 24002 rs-actix
Kotlin = 25001 kt-ktor · 25002 kt-spring-boot
Zig    = 26001 zig
```

DB containers keep their canonical ports **inside** the docker network (server containers connect by service name:5432 etc. — unchanged); only the host-published mappings move to 2000X, so no other project's stack can ever collide. Local-dev connection strings (env defaults) follow the host ports. Dev-port and DB-host renumbering executes at 0E alongside the hono split; framework numbers are assigned once here and never reused.

Image naming: `bench/<entry>` — the folder name is the entry is the image (e.g. `bench/ts-bun-honojs`, `bench/go-echo`).

---

## 7. Benchmark client v2 — "perfect" requirements

### 7.1 Correctness of measurement

- **Throughput (RPS) reported everywhere** — the current latency-only ranking cannot distinguish fast-per-request from high-throughput. New `throughput` fields in summaries, the metrics DB, and dashboards.
- **Open-model mode** alongside the current closed loop: constant-arrival-rate scheduling with latency measured from _intended_ send time — fixes coordinated omission. Config: `mode: "closed" | "open"`, `rate`.
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

**Discovery — make the client folder-structure aware.** Today the roster lives hardcoded in `config/config.json` (`servers` list) and drifts from reality. Instead: each server app carries a small manifest (`bench.json`) next to its Dockerfile declaring `{name, language, runtime, image, databases, experimental, dev_port}`. The client **discovers the roster by scanning `servers/*/bench.json`** (one-level walk over the flat layout, §2.1) — adding a server = adding a folder, zero central edits. `config/config.json` keeps only benchmark parameters (suites, endpoints, load profiles, container limits); the schema validates both.

**Flags — deliberately minimal: flags select, config configures.** `config/config.json` is the single source of truth for all behavior (load mode, rates, profiles, durations, limits, output). Flags only scope _this run_ and never introduce a second place to configure something:

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
- Brokers solve durability and fan-out _across processes/machines_. We have one producer and one consumer in one process; Go channels give the same decoupling with nanosecond overhead and zero serialization.
- The "no silent drops" requirement (§7.2) is solved by bounded channels + accounting, not by a durable queue.
- BullMQ is a Node/Redis job queue — wrong ecosystem for a Go client entirely.

The only scenario where a broker (NATS would be the pick) earns its place is **distributed multi-machine load generation** — explicitly out of scope; revisit only if the generator ever moves off-host.

### 7.6 Build vs buy: custom client & orchestrator — and why not Kubernetes

**Custom client: keep it — it is the project.** The generic load-generation part could be bought (k6, wrk2, vegeta, oha), but everything that makes this repo valuable is custom logic no off-the-shelf tool provides together: full response-body validation on every request (correctness, not just status codes), CRUD sequences with capture/templated vars fanned out per database, per-server container lifecycle with resource sampling, the conformance mode, and one coherent results/metrics pipeline. Gluing k6 (JS scripting, its own per-VU overhead) or wrk2 (Lua, no sequences/validation) to a separate orchestrator, a separate validator, and a separate stats collector would be _more_ moving parts, not fewer. TechEmpower reached the same conclusion — custom harness. The known risk of DIY — subtle generator bugs — is handled by the **cross-validation gate** in §7.1 (calibrate against oha/k6, agree within ~5%).

**Orchestrator: the Go client stays the orchestrator; no Kubernetes.** What orchestration actually requires here: start one server container at a time, wait for readiness, apply resource limits, sample stats, tear down, next. That's testcontainers-go territory (§7.3). Local K8s (kind/k3d/minikube) would add:

- **measurement noise** — control plane + kubelet burning CPU on the same host that runs the SUT;
- **network distortion** — kube-proxy/CNI layers between client and server, versus Docker's direct port mapping;
- **complexity tax** — manifests, image loading into the cluster, slower iteration — for a _sequential, single-node_ workload that uses none of K8s's actual value (scheduling across nodes, self-healing, service discovery).

K8s becomes the right tool only if benchmarking ever goes distributed/multi-node (client on one machine, SUT on another, DBs on a third) — same trigger as the broker question, same answer: out of scope, revisit then. Plain docker compose alone was also considered and rejected for the per-server loop: it can't express "sequential lifecycle with readiness gates, stats attach, and cooldowns" — that logic needs a program, and we have one.

---

## 8. Contract conformance suite (first gate)

Build the contract gate **before** restructuring or extracting shared code. It should pass against the current 10 servers in the current layout first; after that it becomes the safety net for every move, driver swap, endpoint addition, and new server.

Two pieces work together:

- **Go client conformance command**: reuses the existing request builder + validator and runs the contract against a base URL.
- **`scripts/contract.mts` container harness**: builds or finds a server image, starts it with benchmark-equivalent env and DB dependencies, waits for readiness, invokes the Go conformance command against the mapped host port, reports failures, and tears the container down.

Behavior:

- Runs every endpoint + every variation **once, sequentially, strict full-body assertions**.
- Adds **negative cases** the benchmark never exercises: 400 invalid JSON / non-object body / bad form, 404 unknown user + unknown database, 413 oversized file, 415 wrong content type, malformed `favoriteNumber`, invalid email, JWT 401.
- Adds **security-behavior cases** — the contract's security properties, asserted per server: file upload must inspect actual content, not trust the `Content-Type` header (**anti-sniffing**: an image/binary sent with `Content-Type: text/plain` must be rejected 415 `"file does not look like plain text"` — already part of the contract, now explicitly tested with a binary fixture in `test-files/`); size caps enforced pre-read (413); JSON body type enforcement (no array/null smuggling); JWT signature + expiry actually verified (tampered/expired token → 401); path params handled safely (`/params/url/..%2f` style inputs return a normal 200/404, never traversal).
- Cases live in top-level `contract/` (language-neutral JSON), consumed by both benchmark and conformance modes.
- `just contract <entry>` is the normal gate; `just conformance <entry>` may remain as an alias. Exit code is CI-friendly even if the project stays local-first. **No server ships without passing.** Also the smoke test for risky runtime×driver combos (cassandra-driver × Bun/Deno).
- This is what makes "idiomatic everywhere" safe: implementations may differ in style as much as their frameworks demand, but the observable contract may not — the suite is the referee.

---

## 9. Metrics, storage & Grafana redesign

### 9.1 Storage decision: **switch InfluxDB → plain PostgreSQL** (researched, July 2026)

What we actually do is **not classic time-series**: we write event data once per run (per-request latencies, aggregates) tagged by `run_id`, then run OLAP-style queries (rank, percentile, group-by, cross-run compare). The current setup even fakes timestamps (`baseTime + index·µs`) to satisfy Influx's data model — a sign of mismatch. Research verdict:

- **InfluxDB 3 Core is disqualified** for the "compare runs across weeks" requirement: the ~72h limit is real and current (implemented as a 432-Parquet-file query limit — queries touching more files _error out_; raising `query-file-limit` degrades speed/RAM because **Core has no compactor**). Historical data is second-class by design; the fix is Enterprise's non-commercial license — a mismatch traded for a license dependency. ([config-options docs](https://docs.influxdata.com/influxdb3/core/reference/config-options/), [community thread](https://community.influxdata.com/t/influxdb-3-core-seems-to-ignore-the-72-hour-query-time-range-limit/57443))
- **Plain PostgreSQL wins** for this workload (≤ millions of rows per run): exact `percentile_cont` for p50/p95/p99/p99.9, rankings are plain `GROUP BY`/`ORDER BY`, durable history with ordinary backups, no query window, built-in core Grafana datasource with `$__timeFilter` macros — and it's already in the project. Batched inserts (COPY / multi-row) during runs.
- **ClickHouse** (runner-up): best-in-class quantiles, but the heaviest container in the compose (~0.5–1 GB idle untuned; docs assume big machines) and a small-insert anti-pattern to manage — overkill at this scale. **TimescaleDB**: optimizations we don't need at this scale. **VictoriaMetrics/Prometheus**: confirmed anti-pattern (per-run*id labels = high-cardinality churn, approximate quantiles only, no raw events). **DuckDB/Parquet**: great for \_static post-run reports*, weak as a live Grafana backend (unsigned plugin, single-writer).
- Also considered: **MongoDB** (already in the project, but percentile/ranking analytics in aggregation pipelines are clumsy vs SQL and Grafana's Mongo support is weaker); **SQLite/DuckDB as live store** (single-writer, no server for Grafana to query mid-run — fine for post-run reports only).
- Peer validation: TechEmpower keeps `results.json` + a custom viewer; sharkbench writes CSVs; k6 recommends Prometheus→Grafana live + HTML export post-run. Nobody at this scale runs a heavy analytics DB.

**Why Postgres specifically, in one paragraph**: the workload is _small-scale relational OLAP_ — millions of rows at most, queried by exact `run_id`/`server`/`endpoint` equality, needing exact percentiles and rankings. That is the textbook profile of a boring SQL database. Postgres does it _exactly_ (`percentile_cont`, window functions), is already operated in this repo, costs one small container, has the most battle-tested Grafana datasource in existence, and imposes zero data-model contortions (no tags-vs-fields, no cardinality budgets, no retention windows). Every alternative is either a specialized engine whose specialization we don't use (ClickHouse: columnar scale; Influx/VM: high-frequency ingest with recent-data bias) or fails a hard requirement (live queries, history, exact quantiles). When no requirement demands a specialized tool, the general boring one wins.

**Decisions**: (1) metrics go to a **dedicated `metrics-postgres` container** in the grafana compose stack — _never_ the benchmarked postgres instance, which must stay uncontaminated; (2) the writer swap is contained — `internal/influx` becomes `internal/metrics` with the same call sites; (3) schema: `runs`, `request_events` (sampled drilldown only), `endpoint_stats`, `sequence_stats`, `resource_samples` — tags become plain indexed columns, killing the fake-timestamp hack for free; (4) exact percentiles/rankings are computed from the full in-memory/full-run result set before any event sampling, then written to aggregate tables; sampled raw events are never the source of truth for p95/p99; (5) curated `results/published/**/*.json` stays the durable, versioned source for published results, while scratch run output stays ignored.

### 9.2 How we query & present results

- **Query contract, documented in the repo**: dashboards read only **aggregate tables** (`endpoint_stats`, `sequence_stats`, `resource_samples`, throughput) for official numbers; raw `request_events` is sampled drilldown-only. Canonical queries (per-run ranking, cross-run diff, saturation curve) live as `.sql` files in `grafana/queries/` so they're reviewable and reusable — dashboards reference them, not ad-hoc copies.
- **Real timestamps** on rows; `run_id` remains the primary selector everywhere (indexed column, not a tag).
- **Presentation layers** (in order of truth): 1) curated `results/published/**/*.json` — durable, versioned, source of truth for published numbers; 2) scratch `results/runs/<timestamp>/**` — local run artifacts, ignored by git unless intentionally promoted; 3) terminal summary tables (exists, gets RPS + capacity added); 4) Grafana dashboards (live exploration, §9.3); 5) _(Phase 5 nice-to-have)_ a generated static report — one HTML/PNG per run from the JSONs, for the README results section, so published numbers don't depend on a running Grafana.

### 9.3 Grafana dashboards

- **Remove all per-server hardcoded overrides** — palette-by-series-name gives new servers colors automatically; delete the `max: 11` axis cap.
- Variables: `run_id`, `suite`, `endpoint` (**multi-select**), `language`, `runtime`, `server` (multi), `experimental` filter — all powered by the new tags.
- Dashboard split:
  1. **Overview** — RPS + p50/p95/p99 ranking, filter by suite/language/runtime
  2. **Endpoint drilldown** — one endpoint across servers, latency distribution over run time
  3. **Resources** — CPU/mem of servers _and_ DB containers
  4. **Databases** — CRUD sequence timings per DB engine
  5. **Run compare** — two `run_id`s side by side (e.g. Go 1.26 vs 1.27rc, Node vs Bun vs Deno for the same framework)

---

## 10. Version targets (verified July 2026)

| Stack         | Target                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Node.js       | **26.4.x** (Current)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| Bun           | **1.3.14**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Deno          | **2.9.1**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| Go            | **1.27rc1** (go.mod `go` directive + `GOTOOLCHAIN` pin in scripts/justfile; stable is 1.26.4). **encoding/json/v2 adopted** — in the rc's default baseline, zero GOEXPERIMENT references (§0.1). golangci-lint must be rebuilt with the rc (§0.1). `just update` for Go (`go get -u ./... && go mod tidy`) preserves the directive — the pin is update-safe.                                                                                                                                                   |
| Python        | 3.14.x · FastAPI latest · Django 6.x · Flask 3.x                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| Rust          | latest stable · Axum + Actix Web latest                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| Zig           | **0.16.0**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Kotlin        | latest · Ktor 3.x · Spring Boot latest                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| TS libs       | **TypeScript `7.0.1-rc`** (`typescript@rc`, native `tsc`; `module/moduleResolution: nodenext` + explicit `.js` relative imports on Node-runtime servers — Bun/Deno keep their idiomatic resolution) · express 5.2 · fastify 5.9 · nestjs 11.1 · hono 4.12 (+`@hono/node-server` 2.x, needs Node ≥20) · elysia 1.4 (+`@elysiajs/node` in lockstep) · oak 17 (**from JSR**) · `postgres` 3.4 · **drizzle 1.0-rc** · drizzle-kit 1.0-rc · **tsdown** (latest) · mongodb 7.4 · ioredis 5.11 · cassandra-driver 4.9 |
| Tooling       | **Latest versions, pinned**: just, biome, prettier, golangci-lint, ruff, pyright, rustfmt/clippy (ride the Rust toolchain), ktlint, detekt, sqlc, drizzle-kit, uv, pnpm — checked/bumped in Phase 0 alongside runtime deps                                                                                                                                                                                                                                                                                     |
| `just update` | run across all stacks as part of Phase 0; extend it to also cover the lint/format tooling above. **Must be pin-aware**: blanket "update to latest" resolves the `latest` dist-tag and would **clobber deliberate prerelease pins** (`typescript@7.0.1-rc` — `latest` is still 6.0.3; Go 1.27rc1; drizzle + drizzle-kit 1.0-rc) and **lockstep sets** (elysia + `@elysiajs/node`). Pinned/prerelease deps are exempt from auto-bump and tracked in a short "pinned deps" list in the README.                    |

Benchmark credibility note: pre-release/current toolchains are allowed for exploration, but published results must label them clearly. If a pre-release runtime materially affects a headline comparison (Go 1.27rc1, TypeScript RC tooling, Node Current), either include the stable baseline in the published run or mark the run as "current/prerelease toolchain" so readers do not mistake it for the mainstream stable stack.

---

## 11. Execution phases

**Phase 0A — Safety gate first** 0. **Bootstrap workflow**: adopt `phase<N>/<slug>` branches + PRs, stop pushing to `main` directly; gates run locally (`just verify` / `just contract`, optional `pre-push` hook) per §0.2.

1. Add top-level `contract/` cases for the current API and implement the Go client conformance command against a base URL.
2. Add `scripts/contract.mts` and `just contract <entry>` to build/run one existing server in a container, wait for health, execute the conformance command, and tear down.
3. Run the contract gate against all current 10 entries before any restructure. This is the baseline that protects every later refactor.

**Phase 0B — Scripts and manifest foundation**

1. Create `scripts/` and move justfile logic into small typed `.mts` scripts: `verify`, `format`, `lint`, `install`, `update`, `images`, `dev`, `check-config`, `contract`.
2. Make `just verify` non-mutating: type/build checks + format-check + lint. Keep write-formatting in `just fmt`.
3. Add manifest discovery (`bench.json`) and config validation without changing server behavior yet. This makes later server additions one manifest + one script table row.
4. Prototype the risky TS workspace/runtime assumptions before the full restructure: Deno workspace + `--node-modules-dir=manual`, and Bun/Deno Cassandra client smoke tests.

**Phase 0C — Restructure only**

1. Move folders to flat `servers/` (prefixed names, §2.1), `benchmark/`, `shared/` scaffolding.
2. Docker root-context builds. (Sharing wiring — TS-only pnpm workspace + per-language path deps per §2.2 — lands in 0D when `shared/` has members; wiring an empty shared at 0C would bind nothing and force lockfile churn in a move-only slice.)
3. Update ignore files, Dockerfile paths, README setup, and scripts to the new layout.
4. Run `just verify` and `just contract` for all existing entries. This PR should be mostly moves and path updates, not driver swaps. **(Done 2026-07-03, PR #18.)**

**Phase 0D — Shared extraction, one language at a time**

1. TypeScript shared extraction with the current driver first; contract all affected entries.
2. Go shared extraction; contract all affected entries.
3. Python shared extraction; contract FastAPI.
4. Only after move-only extraction is stable, do behavior-affecting swaps as separate slices: TS `pg` → `postgres`, TS 7 RC typecheck gate, tsdown-built shared package, FastAPI worker/pool normalization, Bun shutdown fix.

**Phase 0E — Hono multi-runtime and fairness cleanup**

1. Convert Hono into one app with node/bun/deno entrypoints.
2. Apply the dev/container port convention.
3. Run contract gates for Hono on all runtimes and smoke-test risky runtime×driver combos.

**Phase 1 — Client v2**
testcontainers-go · RPS · open model + ramping + backpressure accounting · no-silent-drops rules · suites/groups selection · new tags · DB-container sampling · runtime config validation · cross-validation against oha/k6.

**Phase 2 — Metrics storage**
Influx → dedicated metrics Postgres · aggregate tables from full result sets · sampled raw event drilldown · Grafana datasource migration · keep old dashboards working until the new ones are ready.

**Phase 3 — New endpoints**
`web` suite (html/jwt/validate/compute) in all existing servers + config cases + contract cases. Add the contract cases first, then implement endpoint support per server.

**Phase 4 — New servers** (each gated by `just contract`)
Echo → Rust (axum, actix) → Python (django, flask) → Kotlin (ktor, spring-boot) → Zig (postgres+redis first, then mongo via libmongoc, cassandra via zig-cassandra/cpp-driver)

**Phase 5 — Grafana/reporting redesign**
5 dashboards, tag-driven, zero per-server hardcoding · published-results static report · README refresh (stack map, results)

---

## 11.1 Dependency graph & parallel execution

The phase list above reads linearly, but the real structure is a DAG with two serial choke points and several wide fan-outs. This section is the map for running work concurrently (multiple agents / worktrees, one PR per lane per §0.2) instead of phase-by-phase.

### The DAG

```
A  contract gate (0A) ── serial root, single-threaded
│      (contract/ cases + Go conformance cmd → then contract.mts harness)
│   B  scripts + manifest + config-validation + Deno/Cassandra spikes (0B)
│   │     ↑ overlaps A; feeds both tracks (manifest format, verify gate)
▼   ▼
C  RESTRUCTURE (0C) ── serial barrier: renames every path, nothing straddles it
│
├───────────────── SERVER TRACK ─────────────────┐   ├──── CLIENT TRACK ────┐
│  D_ts ─┐                                        │   │  P1 client v2        │
│  D_go ─┼─ 3 parallel worktrees (islands)        │   │   └─ P2 metrics PG   │
│  D_py ─┘   each: move-only → then D_swap        │   │        (needs P1)    │
│     │                                           │   └──────────┬───────────┘
│     ├─ E  hono multi-runtime (needs D_ts)       │              │
│     └─ P3 web endpoints (needs shared web utils │              │
│           per lang; per-server parallel)        │              │
│                                                 │              │
│  P4 NEW SERVERS ── widest fan-out ──────────────┤              │
│     zig    (no shared → starts right after C)   │              │
│     echo, stdlib (need D_go)                    │              │
│     rust   (builds own shared crate in-lane)    │              │
│     kotlin (builds own shared module in-lane)   │              │
│     django/flask (need bench-shared = D_py)     │              │
└─────────────────────────┬───────────────────────┘              │
                          P5 Grafana redesign ← needs P2 + P4 (join point)
```

### Two hard serialization points

1. **Contract gate (A) is the root.** One gate, built once, green on the current 10 before anything else — inherently single-threaded. Internal order: `contract/` JSON cases + Go conformance command first, then `contract.mts` (the harness _invokes_ the command). `scripts/` is created here for `contract.mts`; 0B fills out the rest. `contract.mts` may start with explicit entries and switch to manifest discovery once B lands.
2. **Restructure (C) is a stop-the-world barrier.** It renames every path, so no branch may straddle it — a pre-move branch that also edits a file whose directory is renamed produces conflicts git can't auto-resolve. Land C as its own small, fast-merged PR; rebase everything else on top. **All parallel lanes sit entirely after C.**

### The reordering that matters

The linear list implies all of `0D` finishes before Phase 1. It should not. **After C, fork two independent tracks that share no files** (`servers/**` + `shared/**` vs `benchmark/**`):

- **Server track**: D_ts / D_go / D_py (parallel) → D_swap → E → P3 → P4
- **Client track**: P1 → P2

Running them concurrently roughly halves the 0→2 wall clock. They rejoin only at **P5** (needs P2's tags/schema + P4's servers to prove scale).

### Parallel lanes

| Lane                              | Isolation   | Can start when                     | Fan-out                |
| --------------------------------- | ----------- | ---------------------------------- | ---------------------- |
| Contract gate (A)                 | —           | now                                | 1 (serial)             |
| Scripts/manifest (B)              | own PR      | overlaps A                         | 1                      |
| Restructure (C)                   | own PR      | A green                            | 1 (barrier)            |
| Shared extract D_ts / D_go / D_py | worktree ×3 | after C                            | **3**                  |
| Behaviour swaps (D_swap)          | per-lang    | after that lang's move-only        | folds into each D lane |
| Hono multi-runtime (E)            | worktree    | after D_ts                         | 1                      |
| Client v2 (P1)                    | worktree    | **after C — parallel to all of D** | 1                      |
| Metrics PG (P2)                   | worktree    | after P1 (schema design earlier)   | 1                      |
| Web endpoints (P3)                | per-server  | after shared web utils per lang    | per-server             |
| New servers (P4)                  | worktree ×5 | see below                          | **up to 5**            |
| Grafana (P5)                      | worktree    | after P2 **and** P4                | 1                      |

### New-server fan-out (Phase 4) — the widest lane

Each new server is a language island in its own worktree. Start conditions differ:

- **Zig** — needs **nothing shared**; only C + the web-suite contract cases. It's the **longest single task** (MongoDB via libmongoc C-interop) with the **fewest deps** → **start it earliest**, right after C.
- **Echo** — needs `shared/go` (D_go) done.
- **django / flask** — need `bench-shared` (D_py) done.
- **Rust (axum, actix)** and **Kotlin (ktor, spring-boot)** — their shared crate/module does **not** exist yet (0D only extracts TS/Go/Py); it's built **inside the lane**. So within each: `shared + first server` is serial, then the second server parallelizes. Across all five languages: fully parallel.
- New servers implement the **full** surface incl. the `web` suite, so they depend on the **endpoint contract cases** (P3's JSON, writable early) — _not_ on P3 being implemented in the old servers.

### Two laws for safe fan-out

1. **Freeze shared before fanning out its consumers.** A server lane is safe only because it _reads_ `shared/<lang>` and never mutates it. Two lanes both editing the same shared package recreate the copy-paste problem in reverse. So: _extract shared → freeze → fan out consumers._ This is exactly why servers can't parallelize before their language's `D` lane lands.
2. **Root sharing files are the hidden contention.** With §2.2's per-language path deps (go.mod `replace`, uv sources, cargo paths are all per-server files — no contention), the only shared root files left are `pnpm-workspace.yaml` + the mirrored `deno.json` (TS) and `settings.gradle` (Kotlin). Two lanes each appending a member line there conflict trivially. Serialize those one-line edits (or accept the trivial conflict) — they are not a reason to serialize the lanes themselves.

### Critical path / long poles

Two chains, both startable right after C, determine total wall clock:

- **Zig** (server track long pole — C toolchain in Docker, libmongoc).
- **P1 → P2 → P5** (client → metrics → Grafana).

Everything else — TS/Go/Py shared, echo, rust, kotlin, django/flask, endpoints in existing servers — fits in the shadow of those two if fanned out. **Front-load both**: kick Zig and P1 the moment restructure merges.

---

## 11.2 Working method — multi-agent execution & merge protocol

How the plan is actually executed (decided 2026-07-03, in effect from Phase 0A):

### Roles

- **Lead** (main session): owns the DAG — sequences slices, spawns agents, makes contract-level decisions when implementations disagree, reviews every slice, and is the only one who pushes/merges.
- **Implementers**: Opus subagents at **medium** effort — one per slice/lane, worktree-isolated when lanes run in parallel. They implement, self-verify, and commit locally; they never push.
- **Reviewers**: **fresh-context** Opus subagents at **high** effort — they see the diff cold (no implementer context), and critique correctness and requirement gaps, not style. Findings are ranked (blocker/major/minor) with concrete failure scenarios.

### Merge protocol — every PR, in order

1. Implementer **self-verifies**: build + vet + lint clean; the relevant gates green (conformance runs, guard checks).
2. **Rebase onto fresh `main` before review** — the branch is rebased (conflicts resolved deliberately, full rebased diff re-read, gates re-run if code moved) so the reviewer always diffs against current main. This is a hard rule born from two real incidents: a rebase auto-merge silently dropped an import, and a stale-base branch nearly reverted a roster decision merged to main mid-slice. Auto-merge output is never trusted unread.
3. **Fresh-context high-effort review** of the branch diff; all gating findings fixed by the implementer and re-verified.
4. Lead **critiques the final diff directly** — reads the actual code, not the agents' summaries.
5. Lead **independently re-runs every gate**, including the failure modes (non-zero exit on failure, vacuous-green guards firing) — agent claims are never trusted on their face; "looks correct" is not evidence.
6. Only then: push branch, open PR, merge. `main` stays green and correct at every commit.

### Verification rules

- Go to primary sources: re-run the suite, check the exit code, read the handler — never conclude from a report alone.
- Bugs are **reproduced before they are fixed**, and the fix is proven with the same reproduction (e.g. the null-body and multipart-form chi bugs: observed on the wire first, fixed, re-proven green across chi/gin/fiber).
- Findings carry their real conditions — a conclusion that holds only under a precondition is reported with it.
- If a fix cycles without converging, stop and present evidence + hypotheses instead of churning.

### Standing decisions

- **Popular production libraries over hand-rolled** primitives everywhere — this benchmark represents real production stacks. JWT: `golang-jwt/jwt` (Go), `jose` (TS), PyJWT/joserfc (Python), `jsonwebtoken` (Rust), the idiomatic pick per remaining language. UUID: `google/uuid` (Go), `uuid` (TS) — Bun-native `randomUUIDv7` stays an injectable adapter per §3.
- Commits are authored as the repo owner with plain messages — no AI attribution of any kind.
- Small diffs; no features/abstractions/defensive code beyond what the slice needs; validate at system boundaries.
- **Correctness over speed** (Sensei, 2026-07-03): parallelism is welcome only where it costs nothing in correctness, best practices, optimizations, or idioms. Lanes run in parallel when they are DAG-independent and worktree-isolated with the contract gate as the referee; anything touching global state (deps, toolchain, layout, the contract itself, scripts/) runs serially through the full protocol. Never trade a review or gate re-run for wall-clock.
- Implementer briefs carry an explicit decision boundary: what the agent may decide alone vs. what must be escalated (contract semantics, user-environment changes, scope crossing into another slice — always escalate; internal naming, mechanical structure — decide and report).

---

## 12. Risks & open items

- **Deno × pnpm workspace friction**: Deno needs a mirrored `deno.json` workspace list + `--node-modules-dir=manual`; verified in docs, not in this repo — prototype first in Phase 0B before the full restructure.
- **cassandra-driver on Bun/Deno unverified upstream** — the contract gate decides (affects bun-elysia, bun/deno-hono, deno-oak).
- **Zig MongoDB via libmongoc** is the single biggest new-server effort item (C toolchain in Docker build).
- **Bun/NestJS regressions** — NestJS stays Node-only for now; revisit when official support lands.
- **Metrics migration Influx→Postgres** happens in Phase 2 with the writer swap — until then dashboards keep working on Influx; curated `results/published/**/*.json` is the source of truth for published numbers and scratch `results/runs/**` remains local. ClickHouse remains the documented exit if event volume ever outgrows Postgres (§9.1).
- **Run-time budget**: 21 servers × 5 suites × 4 DBs is hours — selectable suites is the mitigation; a `quick` suite preset for development, full matrix for publish runs.
- **Benchmark fairness on one machine**: generator, DBs, and SUT share the host. Out of scope for this plan, but resource-limit the generator and document the caveat in README.
