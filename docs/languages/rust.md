# Rust Best-Practices Guide — benchmarks repo (Phase 4: rs-axum, rs-actix)

Target: Rust 1.96 stable, shared crate via Cargo path deps (no workspace), clippy `-D warnings`.
Planned stack: sqlx **or** deadpool-postgres, mongodb, redis-rs, scylla (Cassandra).
Audience: implementer agents writing the lanes, reviewers critiquing them, and human newcomers.

Every rule is: **imperative — why — (sketch when it disambiguates) — source.** Claims that could
not be pinned to a primary source at authoring time are tagged **UNVERIFIED** — the lane must
confirm them in the contract container before relying on them.

Version facts verified July 2026: async fn in traits stabilized Rust 1.75; axum current line 0.8
(`axum::serve` + `.with_graceful_shutdown`, `DefaultBodyLimit` default 2 MB); actix-web workers
default to `available_parallelism()`; `scylla` crate 1.7.0 (CQL protocol version unstated in driver docs);
mongodb Rust driver `max_pool_size` default 10; redis `ConnectionManager` is multiplexed, not pooled.

---

## 1. Ownership, borrowing & API design

1. **Accept borrowed, return owned.** Take `&str` / `&[T]` (or `impl AsRef<str>` / `impl AsRef<Path>`)
   for inputs you only read; return owned `String` / `Vec<T>` from constructors. This lets callers
   pass `&String`, `&str`, or literals without allocating, while you own what you hand back.
   Why: maximal caller flexibility, minimal copying. Source: Rust API Guidelines C-GENERIC, C-CALLER-CONTROL.

2. **Don't take `String` when `&str` will do; don't take `Vec<T>` when `&[T]` will do.** Taking an
   owned value forces the caller to give up (or clone) ownership for no reason. Only take ownership
   when you actually store the value. Source: Effective Rust Item 8; API Guidelines C-CALLER-CONTROL.

3. **Prefer `impl Trait` in argument position for "any iterable/readable," but name concrete types in
   return position of public handler helpers** unless the abstraction earns its keep. Over-generic
   signatures (`fn f<T: AsRef<str> + Into<String> + Display>(...)`) hurt readability and compile time.
   Source: API Guidelines C-GENERIC; Effective Rust Item 8.

4. **Model domain values with the newtype pattern instead of raw primitives.** `struct UserId(Uuid)`,
   `struct Email(String)` prevent mixing a user id with a post id at the type level, and give you a
   home for validation (`TryFrom`) and `Display`. Zero runtime cost. Source: Rust Book ch.19 (newtype);
   Effective Rust Item 6 ("newtypes").

5. **Provide conversions via `From` / `TryFrom`, never ad-hoc `to_x` methods, and never implement the
   reverse by hand.** Implement `From` for infallible, `TryFrom` for fallible; `Into`/`TryInto` come
   free via the blanket impl. Handler code then reads `let dto: UserDto = row.try_into()?;`.
   Source: API Guidelines C-CONV; std `convert` docs.

6. **Use the builder pattern only for genuinely optional/many-field construction** (e.g. client/config
   options). For 1–3 required fields a plain constructor or struct literal is clearer. Don't build a
   builder for a two-field DTO. Source: API Guidelines C-BUILDER; Rust Book ch.19.

7. **Reach for `Option`/`Result` combinators when they read as a pipeline, fall back to `match` when
   branches carry logic.** `opt.map(f).unwrap_or_default()` and `res.map_err(AppError::from)?` are
   clearer than a match; but a 3-arm match with side effects per arm beats a combinator chain nobody
   can parse. Readability is the tie-breaker. Source: Rust Book ch.6/ch.9; Effective Rust Item 3.

8. **Use `?` for propagation; reserve `unwrap`/`expect` for truly-impossible states, and then `expect`
   with a reason, never bare `unwrap`.** In a server binary tokio isolates a handler panic to just
   that task (it `catch_unwind`s per task) — but only under the default unwinding `panic` strategy;
   see rule 50 on why we must not set `panic = "abort"`. Either way, a panic signals a bug. Source:
   Rust Book ch.9; API Guidelines C-DEBUG.

---

## 2. Error handling (server binary + shared crate)

9. **Shared crate → `thiserror`; binary/handler glue → `anyhow` is acceptable, but a typed app error
   is better here.** Libraries expose concrete, matchable error enums (`thiserror` derives `Display` +
   `Error` + `From` with no boilerplate); applications that only bubble-up-and-report can use `anyhow`.
   Our shared crate is a _library_ consumed by two binaries, so its public errors MUST be a typed
   `thiserror` enum — not `anyhow::Error` — so both servers can map variants to HTTP status codes.
   Source: thiserror vs anyhow (dtolnay); Effective Rust Item 4 ("errors").

10. **Define one app-level error enum per server that carries the HTTP mapping**, built by `From` on the
    shared-crate error and driver errors:

    ```rust
    #[derive(thiserror::Error, Debug)]
    pub enum ApiError {
        #[error("not found")]            NotFound,
        #[error("validation: {0}")]      Validation(String),
        #[error(transparent)]            Db(#[from] sqlx::Error),
        #[error(transparent)]            Shared(#[from] shared::Error),
    }
    ```

    axum: `impl IntoResponse for ApiError`; actix: `impl ResponseError for ApiError`. One place decides
    status + body. Why: keeps handlers `?`-clean and the error shape uniform. Source: axum error-handling
    docs; actix-web `ResponseError` docs.

11. **Never let a driver error string leak into the response `details`.** Map internal errors to a
    generic message; only echo caller-caused validation detail. The contract asserts strict bodies —
    a leaked `sqlx` string fails conformance. Source: repo contract/README; API Guidelines C-GOOD-ERR.

12. **No stringly-typed errors as an interface.** `Err("bad input".to_string())` cannot be matched,
    mapped to a status, or tested. Use enum variants. Source: Effective Rust Item 4; API Guidelines C-GOOD-ERR.

13. **Wire serde attributes to the contract's strict-body rules — conformance turns on them.** Request
    DTOs need `#[serde(deny_unknown_fields)]` (unknown-field rejection), object-only bodies (a top-level
    array/string/number/bool/null must **400**, not deserialize), case-sensitive field names, and a
    typed `favoriteNumber` (a string where a number is required must fail, not coerce). Duplicate keys
    are last-wins by `serde_json` default — that matches the contract, so no extra config. The contract's
    strict-body cases (#14) assert exactly these. Source: serde `container-attrs` docs; `serde_json`
    (duplicate-key last-wins); repo `contract/` strict-body cases.

14. **Remap extractor rejections to the `{"error","details"}` shape — the framework defaults don't
    match.** axum's `JsonRejection`/`PathRejection`/multipart rejections and actix's default extractor
    errors emit their own body + status, so the contract's "invalid JSON / non-object → 400", bad-form,
    and 415 cases fail unless you intercept them. In axum, wrap extractors (custom `FromRequest`, or
    `axum_extra`'s `WithRejection`) to render `ApiError`; in actix, set `.error_handler(...)` on
    `JsonConfig`/`PathConfig`/`PayloadConfig`. Rules 10–11 only cover handler-returned `ApiError`; the
    extractor layer is separate. Source: axum `rejection` docs; actix `JsonConfig::error_handler` docs;
    repo contract.

---

## 3. Concurrency & async — the deep section

### 3.1 Send/Sync mental model

15. **Internalize the two auto-traits before touching tokio.** `Send` = safe to _move_ to another
    thread; `Sync` = `&T` is `Send`, i.e. safe to _share_ a reference across threads. `Arc<T>` is
    `Send + Sync` only if `T: Send + Sync`. `Rc`, `RefCell`, and raw pointers are `!Send`/`!Sync`.
    Source: Rustonomicon "Send and Sync"; Rust Book ch.16.

16. **A future is `Send` only if everything held _across an await point_ is `Send`.** This is the single
    rule behind "future is not Send" errors on the multithreaded runtime: hold a `!Send` guard (e.g.
    `MutexGuard` from `std::sync`, or an `Rc`) across `.await` and the whole task stops being spawnable.
    Source: tokio tutorial "Shared state"; async-book.

### 3.2 Choosing a sharing primitive

17. **Default to `Arc<T>` with no lock for immutable shared state** (pools, config, prepared handles).
    Clients here (`sqlx::Pool`, `mongodb::Client`, redis `ConnectionManager`) are already internally
    synchronized and cheaply `Clone` — wrap once in your `AppState`, clone freely. Don't wrap a pool in
    a `Mutex`. Source: mongodb Rust `Client` docs (Arc internally); sqlx `Pool` docs.

18. **`Arc<Mutex<T>>` for short critical sections that mutate; `Arc<RwLock<T>>` only when reads vastly
    dominate writes and the guard is held long enough to matter.** `RwLock` has higher overhead and
    writer-starvation risk; for a `HashMap` touched briefly, a `Mutex` is usually faster and simpler.
    Use **`tokio::sync::Mutex`/`RwLock` only if you must hold the guard across an await** (see rule 20);
    otherwise prefer `std::sync` / `parking_lot` — they're faster and don't need `.await` to lock.
    Source: tokio `sync` docs ("which kind of mutex"); tokio tutorial.

19. **For producer/consumer or fan-out work, prefer channels (`tokio::sync::mpsc`, `oneshot`,
    `broadcast`, `watch`) over shared mutable state.** Message passing sidesteps lock-ordering and
    held-guard bugs. An "actor" (a task owning state behind an `mpsc` receiver) is the idiomatic way to
    serialize access without a lock. Note: our 16 routes are stateless request/response — you likely
    need _none_ of this; don't invent an actor where a pool clone suffices. Source: tokio tutorial
    "Channels"; Actors with Tokio (Alice Ryhl).

### 3.3 The lock-across-await footgun

20. **Never hold a `std::sync::Mutex`/`RwLock` guard across `.await`.** The guard is `!Send`, so the task
    won't spawn on the multithreaded runtime; worse, even single-threaded it can deadlock the reactor.
    Fix by scoping the lock to drop before the await, or clone the needed value out:

    ```rust
    // BAD: guard alive across await
    let mut g = state.map.lock().unwrap();
    let v = fetch(g.get(&k)).await;   // ❌ guard held across .await
    // GOOD: copy out, drop guard, then await
    let id = { state.map.lock().unwrap().get(&k).copied() };  // guard dropped here
    let v = fetch(id).await;
    ```

    If you genuinely must hold state across an await, use `tokio::sync::Mutex` deliberately.
    Source: tokio tutorial "Shared state — holding a MutexGuard across an .await"; clippy
    `await_holding_lock` lint.

### 3.4 tokio runtime & task placement

21. **Use the multithreaded runtime via `#[tokio::main]` (default `flavor = "multi_thread"`), and let it
    size its worker threads to the machine.** These are async _reactor_ threads, not per-request threads
    — this does not violate our single-process fairness canon (see rule 33). Source: tokio `runtime`
    docs; `#[tokio::main]` docs.

22. **CPU-bound work and blocking/synchronous DB drivers MUST NOT run on the async reactor — move them
    to `spawn_blocking`.** A blocking call on a reactor thread stalls every other task that thread was
    driving (tail-latency collapse). Our chosen drivers (sqlx, mongodb, redis-rs, scylla) are all
    _async_ and belong directly on the reactor — do **not** wrap their calls in `spawn_blocking`. Reserve
    `spawn_blocking` for genuinely synchronous work (e.g. a CPU-heavy hash, a sync crate). Source: tokio
    `spawn_blocking` docs; tokio blog "Reducing tail latencies with cooperative yielding."

23. **Know cooperative scheduling / the task budget.** Each task gets an operation budget; tokio
    resources (sockets, channels, timers) return `Pending` when it's exhausted so the task yields and
    others progress. This is automatic — but a tight `loop` doing only in-memory work (no `.await` on a
    budgeted resource) can still starve peers; insert `tokio::task::yield_now().await` in such loops.
    We have none of those in 16 CRUD routes, but reviewers should flag any unbounded compute loop.
    Source: tokio blog "Reducing tail latencies with automatic cooperative task yielding" (2020-04).

24. **`spawn_blocking` tasks cannot be cancelled once started** — `abort()` only helps if the task
    hasn't begun. Don't rely on aborting blocking work during shutdown; bound it with your own timeout
    or let the drain deadline cover it. Source: tokio `spawn_blocking` / `JoinHandle::abort` docs.

### 3.5 Cancellation & structured concurrency

25. **Understand cancel-safety before using `select!`.** When one `select!` branch completes, the others'
    futures are **dropped mid-flight**. A branch is _cancel-safe_ only if being dropped at an await
    point loses no data. `mpsc::Receiver::recv`, `tokio::time::sleep`, and `Notify::notified` are
    cancel-safe; a multi-step read that has buffered half a message is **not**. Put not-cancel-safe work
    behind a `tokio::spawn` (or hold its state outside the `select!`) rather than inlining it as a
    branch. Source: tokio `select!` docs ("cancellation safety"); tokio tutorial "Select."

26. **For "run N tasks, collect results," use `JoinSet`, not a `Vec<JoinHandle>` + `select!`.** `JoinSet`
    yields tasks as they finish and cancels all remaining on drop (structured concurrency). `select!` is
    for watching a _small fixed_ set of distinct events. Source: tokio `JoinSet` docs.

27. **`tokio::spawn`'d tasks are detached and outlive their spawner unless you join them.** A spawned
    task's future must be `'static + Send`. Don't spawn per-request background work that must finish
    before the response — just `.await` it. Source: tokio `spawn` docs.

### 3.6 async fn in traits — current status (verified)

28. **Native `async fn` in traits is stable since Rust 1.75 — use it for private/internal traits.** You
    can write `trait Repo { async fn get(&self, id: Uuid) -> Result<Row>; }` with no `async-trait` crate.
    Source: Rust Blog "Announcing async fn and RPIT in traits" (2023-12-21).

29. **The Send-bound problem still bites public traits used on the multithreaded runtime.** Native AFIT
    gives you no way to say "the returned future is `Send`" at the _use_ site, so a generic bound like
    `where R: Repo, R::…: Send` isn't expressible directly. If you need a `Send`-bounded async trait,
    add `#[trait_variant::make(Repo: Send)]` (the maintained escape hatch) or fall back to the
    `async-trait` macro (boxes the future; tiny alloc per call). Return-Type-Notation (RFC 3654) was the
    intended long-term fix, but its stabilization PR (rust-lang/rust#138424) was **closed unmerged on
    2025-12-27** — RTN is on **no** stable Rust, including 1.96, so don't plan around it; the
    `trait_variant`/`async-trait` recommendation stands. Source: Rust Blog (above); `trait_variant`
    crate; rust-lang/rust #103854; rust-lang/rust#138424 (closed 2025-12-27).

30. **You almost certainly don't need an async trait at all here.** axum handlers are plain `async fn`s;
    actix handlers are plain `async fn`s. Concrete repo structs with inherent `async fn` methods avoid
    the whole Send-bound question. Prefer concrete types; reach for a trait only if you're abstracting
    over multiple implementations (we aren't). Source: axum/actix handler docs; Effective Rust Item 8.

### 3.7 Graceful shutdown & pool sizing

31. **Implement graceful shutdown: catch SIGINT _and_ SIGTERM, stop accepting, drain in-flight, then
    tear down DB pools — in that order.** Containers send SIGTERM; a Ctrl-C-only handler ignores it.

    ```rust
    async fn shutdown_signal() {
        let ctrl_c = async { tokio::signal::ctrl_c().await.unwrap() };
        #[cfg(unix)]
        let term = async {
            tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
                .unwrap().recv().await;
        };
        tokio::select! { _ = ctrl_c => {}, _ = term => {} }
    }
    ```

    Source: tokio `signal` docs; axum `graceful-shutdown` example.

32. **Drain before DB teardown, not after.** Await the server's graceful-shutdown completion (in-flight
    requests finish), _then_ close pools (`pool.close().await`, `client` drop). Closing pools first
    fails the requests you were trying to drain. Source: axum `with_graceful_shutdown` docs; repo canon.

33. **Pin every pool to exactly 50 connections — max = min = 50 — to match the cross-language fairness
    canon; never leave driver defaults.** Defaults differ wildly (sqlx max 10, mongodb `max_pool_size`
    10, redis `ConnectionManager` = one multiplexed connection). Set both max and min so the pool is
    pre-warmed and identical to peers. **Escalation point:** "50 connections" is unambiguous for
    Postgres, but redis-rs's recommended path is a _single multiplexed_ connection (no pool), and
    Cassandra/Mongo pool _per node_. The lane MUST escalate how "pool = 50" maps onto multiplexed/
    per-node drivers rather than silently choosing (see rules 43–46). Source: repo CLAUDE.md fairness
    canon; sqlx `PoolOptions`, mongodb `ClientOptions`, redis docs.

34. **Gate logging on `ENV=prod` — structured `tracing`, off in prod.** Initialize `tracing-subscriber`
    with an `EnvFilter` and add request logging via tower-http's `TraceLayer` (axum) or
    `tracing-actix-web` / `middleware::Logger` (actix). The repo mandate is **logger off when
    `ENV=prod`** — set the filter to `OFF` (or omit the layer) in that mode so prod runs carry no log
    overhead. Source: `tracing-subscriber` `EnvFilter` docs; tower-http `TraceLayer` docs; repo CLAUDE.md
    (logger off when `ENV=prod`).

---

## 4. axum-specific (rs-axum, axum 0.8)

35. **Compose the app from typed extractors and share state with `State`, not globals.** `State(pool):
State<AppState>` is the idiomatic injection; derive `FromRef` if you split state into sub-states.
    Body/JSON/Path/Query all come from extractors — order matters (body-consuming extractor last).
    Source: axum `extract` docs; axum `State` docs.

36. **Cross-cutting concerns are tower `Layer`s via `.layer(...)`; per-route error handling is
    `impl IntoResponse` on your error type.** Reuse `tower_http` (`TraceLayer`, `TimeoutLayer`,
    `RequestBodyLimitLayer`) instead of hand-rolling middleware. Return `Result<Json<T>, ApiError>`
    from handlers and let `IntoResponse` render both arms. Source: axum middleware docs; tower-http docs.

37. **Set the body limit explicitly — axum's `DefaultBodyLimit` default is 2 MB, not our 10 MB.**
    Apply `DefaultBodyLimit::max(10 * 1024 * 1024)` globally, and enforce the 1 MB file-route cap with a
    tighter limit on that route (`RequestBodyLimitLayer::new(1 MiB)` or a route-scoped `DefaultBodyLimit`).
    Over-limit must surface as **413 Payload Too Large** (and the contract's 415 for wrong content-type).
    Sourcing to keep straight: only the 1 MiB file-route cap (413) and the 415 are **contract** canon;
    the **10 MiB global cap is a pending fairness slice** (`audit/production-fairness`, commit `fad60b1`
    "unified 10 MiB global request-body cap"), not yet on main — it's about-to-be-canon, so build to it.
    Source: axum `DefaultBodyLimit` docs (default 2 MB); repo contract (1 MiB file cap / 415); fairness
    slice `fad60b1` (10 MiB global).

38. **Serve with `axum::serve(listener, app).with_graceful_shutdown(shutdown_signal())` — the old
    `axum::Server::bind(...).serve(...)` builder was removed in 0.7.** Bind a `tokio::net::TcpListener`
    yourself and hand it to `axum::serve`. (Web snippets showing `axum::Server::bind` are stale ≤0.6 —
    do not copy them.)

    ```rust
    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).with_graceful_shutdown(shutdown_signal()).await?;
    ```

    Source: axum 0.8 `serve` docs; axum `graceful-shutdown` example.

---

## 5. actix-web-specific (rs-actix)

39. **Set `.workers(1)` — actix defaults to `available_parallelism()` (one Arbiter/worker per logical
    CPU), which breaks our single-process fairness canon.** Each worker is a separate thread with its own
    App instance and its own copy of any non-`Arc` state; N workers ≈ N event loops. To stay parity with
    the other single-process servers, either force one worker **or** escalate a documented parity
    decision to the lead — do not ship default multi-worker silently. **This is a mandated escalation
    point.** Source: actix-web `HttpServer::workers` docs (default = `available_parallelism()`); repo
    fairness canon.

    ```rust
    HttpServer::new(move || App::new().app_data(state.clone()) /* … */)
        .workers(1)
        .bind(addr)?.run().await
    ```

40. **The `App` factory is a closure run _per worker_ — share state as `web::Data<T>` (an `Arc`) built
    once outside and `.clone()`d in.** Build the pool/clients before `HttpServer::new` and move a clone
    into the factory; never construct a pool inside the factory (that would create one pool _per worker_).
    Extract with `data: web::Data<AppState>`. Source: actix-web `web::Data` / application-state docs.

41. **Configure body limits with `PayloadConfig` / `JsonConfig::limit`, and map errors via
    `ResponseError`.** actix's default payload limits are small — 2 MB for `web::Json` (`JsonConfig`,
    verified on docs.rs) and 256 KB for `web::Payload` (`PayloadConfig`, still **UNVERIFIED** against a
    primary source — confirm); set the 10 MB global cap
    and the 1 MB file-route cap explicitly, returning 413/415 to match the contract. Implement
    `impl actix_web::ResponseError for ApiError` so `?` renders the uniform error body. Source:
    actix-web `PayloadConfig`/`JsonConfig` docs; actix-web error-handling docs.

42. **actix runs on `actix-rt`/`actix_web::main` (a tokio-based single-threaded runtime per worker), not
    `#[tokio::main]`.** Use `#[actix_web::main]`. If you pull in a tokio-multithread-only crate, be aware
    of the runtime mismatch. Graceful shutdown is built in (`HttpServer` handles SIGINT/SIGTERM and
    drains within a shutdown timeout — set it with `.shutdown_timeout(secs)`). Source: actix-web
    `HttpServer` docs; `#[actix_web::main]` docs.

---

## 6. Database clients

43. **Recommendation: use `sqlx` for Postgres (compile-time-checked queries), not deadpool-postgres —
    unless the lane wants raw `tokio-postgres` control.** `sqlx::query!`/`query_as!` verify SQL against a
    live/offline schema at compile time (catches typos, type mismatches) and ship their own async pool;
    that safety is worth more here than deadpool's manager flexibility. deadpool-postgres wraps
    `tokio-postgres` (no compile-time checking) and is the pick only if you deliberately want prepared-
    statement control or to avoid sqlx's macro/compile-time DB dependency. Either way: **pin the pool to
    50** (`PgPoolOptions::new().max_connections(50).min_connections(50)`). If using sqlx's offline mode,
    commit `.sqlx/` (`cargo sqlx prepare`) so builds don't need a live DB — the contract container won't
    have the dev DB at compile time. Source: sqlx README/`Pool` docs; deadpool-postgres docs.

44. **mongodb: one `Client`, `max_pool_size(50)` + `min_pool_size(50)`, cloned into state.** The driver's
    `Client` is `Arc` internally (cheap clone, shared pool) and defaults to `max_pool_size = 10` — set it
    to 50 explicitly. Clone the `Client`; do not create one per handler/worker. Source: mongodb Rust
    driver `Client` / `ClientOptions` docs (performance page).

45. **redis-rs: `ConnectionManager` (multiplexed, auto-reconnect) is the idiomatic high-perf async path —
    but it is ONE multiplexed connection, not a pool of 50.** For non-blocking commands a multiplexed
    connection saturates throughput without a pool. To honor the "pool = 50" mandate literally you'd use
    `deadpool-redis` (an actual pool of 50 `MultiplexedConnection`s). **Escalate** which model the fair-
    ness canon requires for Redis — multiplexed-single vs deadpool-50 — rather than choosing unilaterally
    (ties to rule 33). Do **not** use blocking commands (`BLPOP` etc.) on a multiplexed connection.
    Source: redis-rs docs ("pooling isn't necessary unless blocking commands are used"); deadpool-redis.

46. **cassandra: use the `scylla` crate (1.7.0) — it is a generic CQL driver that is compatible with
    Apache Cassandra, not Scylla-only.** The exact CQL binary protocol version the driver negotiates is
    **UNVERIFIED** (the driver README/docs don't state it); Cassandra 5.0 still accepts the older CQL
    binary protocols, so the driver connects — but the README does not _explicitly_ list
    Cassandra 5.0, so treat "works against plain Cassandra 5.0" as **UNVERIFIED until the contract
    container proves the handshake**. Smoke-test connect + a round-trip against the repo's Cassandra 5
    container before building handlers. Configure the session's per-host connection pool to the mandated
    50 (per-shard/host semantics differ from Postgres — another rule-33 escalation). Source:
    scylladb/scylla-rust-driver README ("compatible with Apache Cassandra"); crate 1.7.0 (CQL protocol
    version unstated in driver docs).

---

## 7. Performance

47. **Don't allocate to satisfy the borrow checker — treat a `.clone()` added "to make it compile" as a
    smell to investigate, not a fix.** Often the real fix is borrowing longer, restructuring, or moving.
    A clone of an `Arc`/pool handle is fine (it's a refcount bump); a clone of a `String`/`Vec` on the
    hot path is the thing to question. Source: Effective Rust Item 15; clippy `redundant_clone`.

48. **Use `Cow<str>` when a function _usually_ returns its input unchanged but _sometimes_ must own a
    modified copy** (e.g. escaping). Avoids allocating in the common pass-through case. Don't reach for
    `Cow` when you always own or always borrow — it just adds noise. Source: std `Cow` docs; API
    Guidelines.

49. **Prefer iterator adapters over manual index loops.** `iter().filter().map().collect()` is bounds-
    check-friendly (LLVM elides checks), expresses intent, and avoids off-by-one/`clone` traps that index
    loops invite. Source: Rust Book ch.13; Rust Performance Book "Iterators."

50. **Set a release profile tuned for throughput** in the binary's `Cargo.toml`. Fat LTO + one codegen
    unit trade compile time for runtime speed — appropriate for a benchmark artifact:

    ```toml
    [profile.release]
    lto = "fat"
    codegen-units = 1
    # opt-level = 3 is already the release default
    # Do NOT set `panic = "abort"`: it removes unwinding, so tokio's per-task
    # catch_unwind can no longer isolate a handler panic — one panicking request
    # would abort the whole server process mid-run. Keep the default `unwind`.
    ```

    Keep the setting identical across rs-axum and rs-actix for fairness. Source: Cargo profile docs;
    Rust Performance Book "Build Configuration"; tokio per-task panic isolation (rule 8).

51. **`unsafe` is not a performance tool here.** Nothing in 16 CRUD routes justifies `unsafe`; the safe
    async drivers and `serde` are already zero-copy where it matters. Reaching for `unsafe` to shave an
    allocation is a review red flag — the answer is a better safe abstraction. clippy `-D warnings` plus
    `#![forbid(unsafe_code)]` at the crate root enforces this. Source: Rustonomicon intro ("don't");
    repo clippy gate.

---

## 8. Common mistakes (reviewer checklist)

52. **`unwrap()`/`expect()` culture in handler/DB paths.** Any `unwrap` on a `Result` that can fail at
    runtime (DB call, parse, header) is a latent 500-via-panic. Propagate with `?` into `ApiError`.
    Grep the diff for `.unwrap()`/`.expect()` outside `main`/tests/const-init. Source: rules 8–12.

53. **Blocking in async.** Synchronous file I/O, `std::thread::sleep`, sync DB clients, or a long CPU
    loop on a reactor thread. Use tokio's async equivalents or `spawn_blocking`. clippy has
    `await_holding_lock`; there is no lint for all blocking — read the handler. Source: rules 20, 22–23.

54. **`.clone()` scattered to appease borrowck.** See rule 47 — distinguish cheap `Arc` clones from
    hot-path deep clones.

55. **Over-generic public APIs** (`impl Into<String> + AsRef<str> + …`) and premature trait abstraction.
    See rules 3, 30. Concrete types until a second implementation actually exists.

56. **Stringly-typed errors / stringly-typed ids.** Enum errors (rule 12) and newtype ids (rule 4).

57. **Constructing pools/clients inside the actix App factory or per request.** Build once, share via
    `Arc`/`web::Data`/`State`. See rules 17, 40, 43–46.

---

## 9. Testing

58. **Unit-test async code with `#[tokio::test]`.** It spins a runtime per test; use
    `#[tokio::test(flavor = "multi_thread")]` if the test needs real parallelism. Source: tokio
    `#[tokio::test]` docs.

59. **Put black-box HTTP tests in `tests/` (integration test crate), driving the real router/app.** For
    axum, exercise the `Router` via `tower::ServiceExt::oneshot` (no socket needed) or bind an ephemeral
    port; for actix use `actix_web::test`. Keep DB-touching tests behind the same compose stack the
    contract runner uses. Note: the repo's authoritative correctness gate is `just contract` (full
    conformance in a container) — in-crate tests supplement, they don't replace it. Source: axum
    testing docs; actix-web `test` module docs; repo CLAUDE.md.

60. **Reach for `proptest` only where input space is large and rules are invariant-shaped** (e.g. a
    validation/parse function in the shared crate: "any valid email round-trips"). Don't property-test
    handler wiring. Source: proptest book.

---

## In this repo (canon the Rust lanes MUST honor)

- **16-route contract is the spec, not any server.** Never weaken a contract case to make Rust pass —
  report drift to the lead (repo CLAUDE.md; `contract/README.md`).
- **Error body is always `{"error": string, "details"?: string}`.** One `IntoResponse`/`ResponseError`
  impl per server produces it; internal error strings never leak into `details` (rules 10–11).
- **UUIDv7 via the `uuid` crate** (`uuid = { features = ["v7"] }`, `Uuid::now_v7()`) — the repo mandates
  the popular `uuid` crate, not a hand-rolled generator. Wrap ids in newtypes (rule 4).
- **Single process, pool pinned to exactly 50** — tokio multi-thread reactor is fine (rule 21); actix
  MUST run `.workers(1)` or escalate a documented parity decision (rule 39). Pin max=min=50 on every
  pool; escalate the redis multiplexed / Cassandra per-host mapping (rules 33, 45, 46).
- **Body limits: 10 MiB global cap, 1 MiB on the file route**, over-limit → **413**, wrong media type →
  **415**. axum default is 2 MiB — override it (rules 37, 41).
- **Graceful drain then DB teardown** on SIGINT _and_ SIGTERM (rules 31–32); logger off when
  `ENV=prod` (rule 34).
- **Non-root, multi-stage Dockerfile built from the repo root context**, canonical container port, same
  env-var contract as every other server.
- **Shared crate holds infrastructure only** (DB clients, schemas, validation rules, consts, env, the
  `thiserror` error type) — routing/handlers/app structure stay per-framework in each framework's
  canonical production style (repo CLAUDE.md §3 "sharing stops where idiom starts").
- **`clippy -D warnings` is a hard gate**; add `#![forbid(unsafe_code)]` (rule 51). `just verify`
  (typecheck → format-check → lint) and `just contract` must both be green for every touched server
  before review.

---

### Sources

- Rust API Guidelines — https://rust-lang.github.io/api-guidelines/ (C-CALLER-CONTROL, C-CONV, C-BUILDER, C-GENERIC, C-GOOD-ERR, C-DEBUG)
- The Rust Programming Language (Book) — ch.6, 9, 13, 16, 19 — https://doc.rust-lang.org/book/
- The Rustonomicon — "Send and Sync," intro — https://doc.rust-lang.org/nomicon/
- Effective Rust, David Drysdale — Items 3, 4, 6, 8, 15 — https://www.lurklurk.org/effective-rust/
- Rust Blog, "Announcing `async fn` and return-position `impl Trait` in traits" (2023-12-21) — https://blog.rust-lang.org/2023/12/21/async-fn-rpit-in-traits/
- rust-lang/rust #103854 (Send bounds / async_fn_in_trait); `trait_variant` crate; RFC 3654 (Return Type Notation)
- Tokio docs — `spawn`, `spawn_blocking`, `task::JoinSet`, `select!`, `sync` (Mutex/RwLock/mpsc), `signal`, `runtime`, `#[tokio::test]` — https://docs.rs/tokio/latest/tokio/
- Tokio tutorial — Spawning, Shared state, Channels, Select — https://tokio.rs/tokio/tutorial/
- Tokio blog, "Reducing tail latencies with automatic cooperative task yielding" (2020-04) — https://tokio.rs/blog/2020-04-preemption
- axum docs & examples (0.8) — `serve`, `DefaultBodyLimit`, `State`, extractors, middleware, `graceful-shutdown` example — https://docs.rs/axum/latest/axum/ , https://github.com/tokio-rs/axum/tree/main/examples/graceful-shutdown
- tower-http docs — `TraceLayer`, `TimeoutLayer`, `RequestBodyLimitLayer` — https://docs.rs/tower-http/
- actix-web docs — `HttpServer::workers`, `web::Data`, `PayloadConfig`/`JsonConfig`, `ResponseError`, server guide — https://actix.rs/docs/server/ , https://docs.rs/actix-web/latest/actix_web/
- sqlx — README, `Pool`/`PgPoolOptions`, offline mode — https://github.com/launchbadge/sqlx , https://docs.rs/sqlx/
- deadpool-postgres / deadpool-redis — https://docs.rs/deadpool-postgres/ , https://docs.rs/deadpool-redis/
- mongodb Rust driver — `Client`, `ClientOptions`, Performance Considerations — https://docs.rs/mongodb/ , https://www.mongodb.com/docs/drivers/rust/current/fundamentals/performance/
- redis-rs — README, `aio::ConnectionManager`, `MultiplexedConnection` — https://github.com/redis-rs/redis-rs , https://docs.rs/redis/
- scylladb/scylla-rust-driver (crate 1.7.0, CQL protocol v4, Cassandra-compatible) — https://github.com/scylladb/scylla-rust-driver , https://rust-driver.docs.scylladb.com/stable/
- Rust Performance Book — Iterators, Build Configuration — https://nnethercote.github.io/perf-book/
- Cargo profiles reference — https://doc.rust-lang.org/cargo/reference/profiles.html
- proptest book — https://proptest-rs.github.io/proptest/
