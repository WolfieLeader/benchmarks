<!-- Keep under ~100 lines. Every line must change Claude's behavior; prune when it stops earning its place. -->

# HTTP Benchmarks

Same 16-route API implemented across many frameworks/languages (11 servers today → 21 per plan), benchmarked by a custom Go client (`benchmark/`) that is also the orchestrator and the conformance runner. Currently mid-expansion — **PLAN.md is the authoritative roadmap** (locked decisions §0, working method §11.2, execution DAG §11.1).

## Sources of truth

- `PLAN.md` — the plan; §11.2 is the merge protocol every PR follows.
- `contract/` — the API contract as executable JSON cases (`contract/README.md` = case format). The contract outranks any single server's behavior: when servers disagree, the lead decides canon; **never weaken a case to make a server pass** — report the drift instead.
- `config/config.json` (+ schema) — benchmark parameters. The server roster now lives in per-server `servers/*/bench.json` manifests (PR #24), discovered by both the scripts (`scripts/lib.mts`) and the Go client (`benchmark/internal/roster`).

## Commands

DBs first: `just db-up` (postgres/mongodb/redis/cassandra via compose; the contract harness auto-detects this stack's network and fails loud if it's missing or ambiguous).

```
just contract <entry|all>   # THE gate: container + full conformance run; no server ships without it
just verify <target|all>    # non-mutating: typecheck → format-check → lint (never writes)
just fmt / just lint        # the writing counterparts
just install|update|images|dev <target>
just benchmark              # the actual benchmark TUI/run
```

- All dispatch logic lives in typed `scripts/*.mts` (Node 26 native type-stripping — no tsx, no build step; `scripts/contract.mts` stays npm-dependency-free). Adding a server = one roster row / one `bench.json` manifest — never a new bash branch.
- `just update` is pin-aware: prerelease pins (TS RC, drizzle RC, …) are exempt from blanket bumps — check PLAN §10 before bumping anything pinned.

## Working method (agents)

- **Never push or commit to `main`.** Branch `phase<N>/<slug>`, one PR per reviewable slice; plain commit messages, repo-default author, **no AI attribution of any kind**.
- Merge protocol (PLAN §11.2): implementer self-verifies → **rebase onto fresh main (re-read the rebased diff — auto-merge is never trusted unread)** → fresh-context high-effort review → fixes → lead re-verifies independently → PR → merge. Gates: `just verify` green + `just contract` green for every touched server.
- Correctness over speed: parallel lanes only when DAG-independent + worktree-isolated; global-state slices (deps, toolchain, layout, contract, scripts/) go serial. Never skip a review or gate re-run for wall-clock.
- Implementer subagents run Opus @ medium effort; reviewer subagents Opus @ high effort, fresh context, correctness over style.
- Verification is primary-source: re-run the suite, check exit codes, read the handler. Reproduce bugs before fixing; prove fixes with the same repro.

## Conventions

- **Idiomatic everywhere; sharing stops where idiom starts** (PLAN §3): shared packages hold infrastructure (DB clients, schemas, validation rules, consts, env); routing/handlers/app structure stay per-framework in that framework's canonical production style.
- **Popular production libraries over hand-rolled primitives** — JWT: `golang-jwt` (Go) / `jose` (TS) / PyJWT (Python); UUID: `google/uuid` (Go) / `uuid` (npm). Bun-native speedups (e.g. `randomUUIDv7`) are injectable adapters, not forks.
- Error responses are always `{"error": string, "details"?: string}`; contract cases assert strict full bodies (matcher tokens documented in `contract/README.md`).
- Every server: same env-var contract, canonical container port, graceful shutdown on SIGINT/SIGTERM, logger off when `ENV=prod`, non-root multi-stage Dockerfile built from repo-root context.
- Fairness: single-process servers, identical pool sizes — never give one framework an advantage the others can't have idiomatically.

## Toolchain quirks (PLAN §0.1 has the full list)

- Go 1.27rc1 is a **separate binary** `~/go/bin/go1.27rc1` (stable `go` untouched); Rust proxies live in `/opt/homebrew/opt/rustup/bin` (keg-only, no `~/.cargo/bin`); Deno upgrades via `deno upgrade`, not brew; Kotlin builds via Gradle wrapper with a pinned JDK toolchain (launcher JVM is newer than Spring's ceiling).
- pnpm workspace + Deno: root `deno.json` mirrors the workspace with `nodeModulesDir: "manual"`; **`pnpm install` must run before any Deno command**.
- macOS: no `timeout` (use `gtimeout`/`curl --max-time`); paths are case-insensitive (compare accordingly).

## Never

- Never weaken a contract case, add a rule-wide lint disable, or hide a failing target to get green.
- Never benchmark against the metrics/infra containers' resources (metrics-postgres is separate from the benchmarked postgres — keep it that way).
- Never edit generated output (sqlc, lockfiles) by hand.
