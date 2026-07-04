# Python Best Practices — HTTP Benchmarks Repo

Scope: py-fastapi (shipped, Python 3.14, FastAPI + uvicorn single-process target, SQLAlchemy-async/asyncpg,
motor, redis.asyncio, cassandra-driver, pydantic v2, uv, ruff + pyright strict) and py-django / py-flask
(Phase 4, not yet implemented). Rules are numbered, imperative, with the "why" and a minimal sketch where
one clarifies more than prose. Claims that could not be checked against a primary source in this session are
marked **UNVERIFIED**.

---

## 1. Modern idioms

1. **Default to type hints everywhere — they are enforced, not decorative.** pyright runs in strict mode here
   (`servers/py-fastapi/pyproject.toml` → `dev` group has `pyright>=1.1.400`), so every function signature,
   every `Mapped[...]` column, every repository method needs a real annotation. Untyped `def create(self, data):`
   is a lint failure waiting to happen, not a style nit.
2. **Use PEP 695 generic syntax (3.12+), not `TypeVar` boilerplate.** `def first[T](xs: list[T]) -> T: ...` and
   `class Box[T]: ...` replace `T = TypeVar("T"); def first(xs: list[T]) -> T: ...`. Same for type aliases:
   `type UserId = str` instead of `UserId: TypeAlias = str`. Since target-version is `py314`, there is no reason
   to write the old form. (docs.python.org — What's New in 3.12, "Type Parameter Syntax")
3. **Pick TypedDict / Protocol / dataclass / pydantic by what the data does, not habit:**
   - **pydantic `BaseModel`** — data that crosses a validated boundary (HTTP body, query params, env vars). It
     parses and coerces; that's its whole job. `src/database/types.py` uses it for `User`/`CreateUser`/`UpdateUser`
     because FastAPI needs runtime validation of client input.
   - **`TypedDict`** — a plain dict shape with zero runtime validation cost, for internal structures you already
     trust (e.g. `ErrorResponse` in `src/consts/errors.py` — an error body you _construct_, never validate).
   - **`Protocol`** — structural typing for "anything with this shape," used for swappable implementations
     without inheritance. `src/database/repository.py`'s `UserRepository` Protocol is exactly this: four unrelated
     repository classes (SQLAlchemy, motor, redis, cassandra) satisfy it without a common base class.
   - **`@dataclass`** — plain in-process value objects with no validation and no external boundary. Cheapest
     option; use it when pydantic's validation overhead buys nothing.
   - Rule of thumb: **validation boundary → pydantic; internal shape → TypedDict/dataclass; interchangeable
     implementations → Protocol.**
4. **Use `pathlib.Path`, never raw string path-joining.** `Path(__file__).parent / "static"` over
   `os.path.join(os.path.dirname(__file__), "static")` — composable, cross-platform, and has `.exists()`,
   `.read_text()`, etc. Note: macOS paths are case-insensitive (repo-wide gotcha per `CLAUDE.md`) — don't rely on
   `Path` equality to catch a casing bug that only breaks in Linux containers.
5. **f-strings for all string formatting; no `%`-formatting or bare `.format()` in new code.** Ruff's `UP` rules
   (pyupgrade) flag the old forms. `logging.info(f"{request.method} {request.url.path} ...")` in
   `src/main.py:46` is the repo's own convention — keep matching it.
6. **`match` statements only for genuine multi-branch shape/value dispatch — not as an `if/elif` replacement.**
   Good fit: dispatching on a `Literal["postgres", "mongodb", "redis", "cassandra"]` or destructuring a tagged
   union/response shape. A `match` over 4 database names with structural patterns reads better than a chain of
   `==`s; a `match` used purely for `match x: case True: ... case False: ...` is worse than `if x`. This repo's
   `get_repository` in `src/database/repository.py:26-53` currently uses `if/elif` over `DatabaseType` — a
   reasonable `match` candidate if it grows more branches, not a bug as-is.
7. **EAFP over LBYL for anything touching external state (dict keys, attributes, parsing).** Python's exception
   cost is fine for the "rare failure" case and EAFP avoids TOCTOU races. The repo's own `_parse_uuid` helpers
   (`src/database/postgres.py:40-44`, `cassandra.py:61-65`) are canonical EAFP: `try: return UUID(id) except
ValueError: return None` rather than a pre-validation regex. LBYL is still fine for cheap, side-effect-free
   checks (`if data.name is not None:`) where "ask forgiveness" buys nothing.
8. **Every resource with a lifetime gets a context manager — don't rely on GC.** `async with self._session_maker()
as session:` (SQLAlchemy) closes the session/connection deterministically even on exception; the ad hoc
   "manual open, hope disconnect() is called" pattern is how connections leak. Write your own with
   `@contextmanager`/`@asynccontextmanager` when wrapping something that doesn't already provide one (see the
   FastAPI `lifespan` context manager in `src/main.py:27-31`, which is exactly this idiom at the app level).

---

## 2. Async — the deep section (this is where the fastapi stack actually lives)

9. **Blocking the event loop is the cardinal sin.** A single sync call inside an `async def` handler
   (a sync DB driver, `time.sleep`, CPU-bound work, sync file I/O) freezes _every_ concurrent request on that
   process, not just the caller — there is one event loop, one thread, and uvicorn here runs single-process
   (canon, once the Phase-4 worker-count fix lands — see §9 "In this repo"). Diagnose by asking: "does this call
   have `await` anywhere in its call stack down to the OS?" If no, it blocks.
10. **Bridge unavoidable sync work with `asyncio.to_thread` (3.9+) or `loop.run_in_executor`, never call it bare.**
    `to_thread` is the modern, simpler wrapper around `run_in_executor(None, ...)` for the default thread pool;
    reach for `run_in_executor` with an explicit `ThreadPoolExecutor` when you need a _dedicated, sized_ pool
    isolated from the default executor (e.g. so a slow driver can't starve unrelated background work). This
    repo's `CassandraUserRepository` does exactly the latter — its own `ThreadPoolExecutor(max_workers=50)`
    (`src/database/cassandra.py:36`) so blocking cassandra-driver calls never touch the loop's default executor.
    ```python
    async def _execute(self, query, params=()):
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(self._executor, lambda: list(self._session.execute(query, params)))
    ```
11. **Why cassandra-driver specifically needs bridging: it's callback/future-based, not asyncio-native.**
    `cassandra-driver`'s `Session.execute()` blocks the calling thread; its async counterpart,
    `execute_async()`, returns a driver-native `ResponseFuture` (not an `asyncio.Future`) that you attach
    `add_callbacks(on_success, on_error)` to — those callbacks fire on the _driver's own I/O thread_, not the
    event loop thread, so touching event-loop state from them without `loop.call_soon_threadsafe(...)` is a race.
    (DataStax docs: "callbacks... executed on the event loop thread [for `AsyncioConnection`]; the normal
    advice about minimizing cycles and avoiding blocking applies.") There are two correct bridge strategies:
    - **What this repo does**: run the _blocking_ `session.execute()` call inside `run_in_executor` on a
      dedicated thread pool (§10) — simplest, correct, no manual `Future` wiring, costs one thread-pool hop.
    - **The `ResponseFuture` route** (not used here, `AsyncioConnection` is experimental per DataStax docs):
      wrap `execute_async()` in an `asyncio.Future` via `loop.create_future()`, then in the driver's
      `add_callbacks` handlers do `loop.call_soon_threadsafe(future.set_result, rows)` /
      `loop.call_soon_threadsafe(future.set_exception, exc)` — never call `future.set_result` directly from the
      callback thread. Note the docs diverge here: DataStax's upstream `python-driver` docs (3.29) still label
      `AsyncioConnection` "experimental," but the driver actually pinned here — `scylla-driver>=3.26.6`
      (`pyproject.toml`) — has **dropped** the word "experimental" for the same class in its own docs. Check the
      pinned driver's docs, not DataStax's, before relying on it if a future Python cassandra lane considers switching.
12. **Prefer `asyncio.TaskGroup` (3.11+) over `asyncio.gather` for new concurrent-fan-out code.** `TaskGroup` is
    structured concurrency: the `async with` block doesn't exit until every child task is done or cancelled, and
    if one task raises, the group cancels its siblings and raises an `ExceptionGroup` — no silent partial
    results. `gather` without `return_exceptions=True` cancels siblings on first exception too, but loses the
    "wait for cancellation to actually finish" guarantee and doesn't aggregate multiple failures. Reach for
    `gather` only when you deliberately want `return_exceptions=True` (collect every result/error, keep going).
    ```python
    async def initialize_databases() -> None:
        async with asyncio.TaskGroup() as tg:
            for db in DATABASE_TYPES:
                tg.create_task(get_repository(db).health_check())
    ```
    This repo's `initialize_databases()` (`src/database/repository.py:62-63`) currently uses
    `asyncio.gather(*[...])` — fine today (fire-and-forget health checks at startup, no per-task error handling
    needed), but a `TaskGroup` is the more defensible default for anything added later that needs to _propagate_
    a failure. (Python docs, `asyncio-task.html`: "TaskGroup... provides stronger safety guarantees... will
    cancel the remaining scheduled tasks" vs gather.)
13. **Cancellation is cooperative and can land at any `await`.** A cancelled task gets `CancelledError` raised at
    its next suspension point — code that holds a resource across an `await` must clean up in `finally`, not
    "after the await returns," because it may never return normally. `asyncio.shield()` protects a specific
    awaitable from the _outer_ task's cancellation (e.g. "let this commit finish even if the request was
    cancelled") but does **not** protect against the shielded operation's own timeout/cancellation — don't reach
    for it as a general "make cancellation not happen" hammer; use it narrowly around the one critical inner
    await, and prefer structuring the code (e.g. `asyncio.timeout()` around the caller, not the callee) over
    sprinkling shields.
14. **Backpressure: bound your concurrency, don't fan out unbounded.** An `asyncio.Semaphore` around a batch of
    tasks, or a bounded `asyncio.Queue` between producer/consumer coroutines, keeps memory and downstream
    connection use flat as load grows — relevant here because pool sizes are fixed (50 connections; §9's
    fairness canon), so issuing more concurrent DB calls than the pool can serve just queues inside the driver
    instead of failing fast. Size any semaphore to (or below) the pool size it fronts.
15. **Async context managers for anything with async setup/teardown — pools chief among them.** SQLAlchemy's
    `async_sessionmaker` + `async with session_maker() as session:` is the shape to copy; don't hand-roll
    "call `.connect()`, remember to call `.close()`" when the library already ships an `__aenter__`/`__aexit__`.
    When you must write your own (a raw driver without one), an `@asynccontextmanager` generator function is the
    idiomatic way, exactly like `src/main.py`'s `lifespan`.
16. **uvicorn's worker model: one OS process per worker, no shared memory, no built-in shared pool.** `--workers
N` forks N independent processes each running their own event loop and — critically — their own DB
    connection pool; "pool size 50" only means 50 _per process_. This repo's locked target is **exactly one
    uvicorn worker** (see §9), which is what makes "pool size 50" a single, comparable number across the whole
    benchmark matrix instead of `50 × workers`. Separately, the repo runs uvicorn with `--loop uvloop`
    (`Dockerfile:38`) — a deliberate C-accelerated event-loop replacement (`uvloop>=0.21.0` is a pinned
    dependency), load-bearing rather than incidental; preserve the `--loop uvloop` flag through any
    uvicorn-invocation edit.
17. **Lifespan is the only correct place to open/close pools — not module import time, not first-request lazy
    init (mostly).** FastAPI's `lifespan` context manager (`src/main.py:27-31`) runs startup code before the
    app accepts traffic and shutdown code after uvicorn stops accepting new connections but before process exit
    — `await initialize_databases()` then `yield` then `await disconnect_databases()`. Graceful shutdown on
    SIGINT/SIGTERM (required by this repo's env-var contract) depends on that teardown actually running: uvicorn
    catches the signal, stops the accept loop, drains in-flight requests, then runs the lifespan shutdown phase.
    One documented footgun: if uvicorn is launched via a shell wrapper (`sh -c "uvicorn ..."`), the shell — not
    uvicorn — receives SIGTERM, and uvicorn's shutdown code never runs; run it directly (`uvicorn ...` as PID 1,
    which is what a non-root Dockerfile `CMD` without a shell form gives you). (uvicorn docs/GitHub discussions,
    see Sources.)
18. **Connection pool sizing is a fairness knob here, not a performance-tuning free choice.** The plan's locked
    canon (`PLAN.md:194`) is **pool size exactly 50** for every Python server, single process — matching the
    "50 elsewhere" convention used across the other language servers. Don't retune a pool size to make one
    server look faster; if a framework needs a different real-world default, that's a drift to report, not to
    silently work around. Footnote on scope: the "pool 50" canon is currently enforced for Postgres
    (`pool_size`/`max_overflow`) and Cassandra (`ThreadPoolExecutor(max_workers=50)`), but the motor (Mongo) and
    `redis.asyncio` (Redis) repositories set **no explicit pool size at all** — they run on their drivers'
    defaults. Treat that as a fairness-audit follow-up to normalize, not a silent exception to the rule.

---

## 3. Python 3.14 currency check (verified July 2026)

19. **Free-threading (no-GIL) is no longer experimental in 3.14, but it doesn't matter for this repo yet.**
    The free-threaded build removes the GIL via per-object locking + biased reference counting; pure Python code
    benefits from real multi-core parallelism without extension recompilation, at a measured cost of roughly
    1–8% single-thread overhead depending on platform. **This repo doesn't use it**: uvicorn here is single
    event-loop, single-process by design (fairness canon, §2.16), and free-threading only helps CPU-bound
    _threaded_ work — our concurrency is I/O-bound `asyncio`, which the GIL was never the bottleneck for. Adopting
    the free-threaded build would also require the free-threaded wheel/build of every C-extension dependency
    (asyncpg, motor's C accelerators, cassandra-driver, pydantic-core) — not verified as available/stable for all
    of them as of July 2026; do not switch builds without checking each dependency's free-threaded wheel status
    first. (docs.python.org — What's New in 3.14; docs.python.org/3/howto/free-threading-python.html)
20. **Template strings (t-strings, PEP 750) are new in 3.14 — not a fit for this repo's JSON/HTTP surface.**
    `t"..."` returns a `Template` object exposing the static and interpolated parts separately, letting a
    consumer (e.g. a safe-HTML renderer or a query builder) validate/escape each interpolation before assembling
    the final string — f-strings can't do this because they eagerly concatenate. Relevant mainly if a future
    `GET /html` route (per `PLAN.md`'s new-endpoints table) wants injection-safe server-rendered HTML without a
    templating engine; not needed for the current JSON-only routes, where f-strings remain correct.
    (docs.python.org — What's New in 3.14)
21. **Other 3.12/3.13 changes worth knowing land underneath what's already used here**: PEP 695 generic syntax
    (§1.2, stable since 3.12); improved error messages; `pathlib` gained more `Path` convenience methods across
    3.12–3.13. None of these are breaking for this stack; `requires-python = ">=3.14.3"` in
    `servers/py-fastapi/pyproject.toml` already assumes the newest baseline, so write to 3.14 idioms by default
    rather than hedging for older interpreters.

---

## 4. FastAPI specifics

22. **`async def` vs `def` route handlers is a real footgun, not a style choice.** FastAPI runs `async def`
    handlers directly on the event loop; a plain `def` handler is automatically dispatched to FastAPI's external
    threadpool via `run_in_threadpool` (Starlette's `request_response()` picks the path via `is_async_callable`).
    That means: **a `def` handler that does something actually async-unsafe is fine** (it's off the loop), but
    an `async def` handler that calls a blocking library (a sync DB driver, `requests`, `time.sleep`) blocks the
    _entire_ event loop for every other request. The rule: if the handler body is genuinely sync (CPU work, a
    sync-only library), declare it `def` and let FastAPI thread it; if it awaits async I/O, declare it
    `async def` and make sure every call inside is actually awaited, not a blocking call quietly wrapped.
    (FastAPI docs, `docs/en/docs/async.md`, `docs/en/docs/advanced/stream-data.md`)
23. **The same threadpool-offload rule applies to dependencies, including sub-dependencies.** A `def` dependency
    (no `async`) used with `Depends(...)` also runs in FastAPI's external thread pool; `async def` dependencies
    are awaited inline. Mixed sync/async dependency graphs are supported — but a sync dependency graph that fans
    out to many `def` dependencies uses up threadpool slots (shared with sync route handlers) that a busy app
    might exhaust. (FastAPI docs, `docs/en/docs/async.md`)
24. **Use `Depends()` for anything that needs setup/teardown or is shared across routes — don't hand-roll a
    global.** DI here doubles as resource lifetime management (a `yield`-style dependency runs cleanup after the
    response is sent) and as the natural seam for testing (override a dependency in tests instead of monkeypatching
    a module global). This repo's `_require_repo`/`resolve_repository` helpers in `src/routes/db.py` are a
    lighter hand-rolled version of the same idea — reasonable at this scale, but `Depends()` is the idiomatic
    FastAPI answer once a dependency needs its own cleanup or per-request state.
25. **pydantic v2 performance: prefer `model_validate` over manually building dicts, and reach for `TypeAdapter`
    for non-`BaseModel` types.** `model_validate(data)` runs the compiled Rust `pydantic-core` validator directly;
    manually validating field-by-field in Python and only using pydantic for the "shape" defeats the point.
    For validating a bare `list[int]` or other non-model type, `TypeAdapter(list[int]).validate_python(...)` gets
    the same compiled-validator speed without wrapping it in a throwaway `BaseModel`. A `TypeAdapter` must be
    **built once at module scope and reused** — constructing it compiles the validator, so rebuilding it per call
    inside a hot path throws away exactly the speed you reached for it to get. (pydantic docs, `docs/why.md`,
    `docs/concepts/strict_mode.md`)
26. **Strict mode is opt-in per-field, per-call, or per-model — pick the narrowest scope that solves the actual
    problem.** Default ("lax") mode coerces `"123"` → `123` for an `int` field; `strict=True` (via
    `model_validate(..., strict=True)`, `Field(strict=True)`, or `ConfigDict(strict=True)`) rejects the coercion.
    This repo's env parsing (`src/config/env.py`) deliberately uses _lax_ + custom `field_validator`s (parsing
    `PORT` from a string env var, normalizing `HOST`) rather than strict mode, because env vars are always
    strings and need coercion — strict mode there would reject every real input. Use strict mode for API
    boundaries where the client should _not_ get free type coercion (e.g. rejecting `"1"` for a boolean field),
    not for boundaries whose input format is a string by construction. (pydantic docs, `concepts/strict_mode.md`)
27. **`response_model` costs a second full validation pass — know when you're paying it twice.** Returning a
    pydantic model directly from a route with a matching `response_model` (or the return-type annotation, which
    FastAPI uses as an implicit `response_model`) re-validates and re-serializes the object even though it was
    already a valid model; for hot paths, either return a `dict` already known to conform to `response_model=None`
    and set the response class/annotation explicitly, or accept the cost as the "runtime-checked serialization
    contract" you're buying. This repo's routes call `user.model_dump(exclude_none=True)` and return plain dicts
    (`src/routes/db.py:40,54,66`) rather than declaring a `response_model` — that sidesteps the second validation
    pass, but it is **not free on the serialization side**: a returned plain dict goes through FastAPI's
    `jsonable_encoder` (the slow path), whereas a declared `response_model` lets `pydantic-core` serialize the
    object directly via its Rust core "without intermediate steps." So the dict-return trades away both the
    auto-generated OpenAPI schema _and_ the fast serialization path — a real, measurable choice in a benchmarking
    repo, not just an OpenAPI-docs cosmetic. (FastAPI docs, `custom-response.md`: "you are probably better off
    using a Response Model.")
28. **Exception handlers must produce this repo's exact `{"error": string, "details"?: string}` shape — no
    default FastAPI error body survives contact with the contract.** `src/handlers.py` overrides all four
    relevant handlers (`RequestValidationError`, Starlette's 404 `HTTPException`, FastAPI's `HTTPException`,
    and a catch-all `Exception`) to route every failure through `make_error()` (`src/consts/errors.py:22-26`),
    which omits `details` entirely when there's nothing to say rather than emitting `"details": null`. Any new
    Python server (Django, Flask) must reproduce this same shape — it's a contract-level invariant
    (`CLAUDE.md`: "Error responses are always `{"error": string, "details"?: string}`"), not a FastAPI-only
    convention.

---

## 5. Django (ahead of the Phase 4 lane — locked decisions per `PLAN.md`)

29. **Django 6's async story is real but has one hard gap: transactions.** Function-based views become async by
    declaring `async def`; class-based views declare `async def get()`/`post()`/etc. per-method. ORM methods
    that issue queries have `a`-prefixed async twins — `await Book.objects.acreate(...)`, `await book.asave()`,
    `await Book.objects.aget(...)`, `AsyncPaginator`/`AsyncPage`. **Transactions do not yet work in async mode**
    (Django 6.0 docs): any code that needs `atomic()` semantics must be written as a plain sync function and
    invoked via `sync_to_async(...)` from the async view — don't try to open an ORM transaction directly inside
    an `async def` view. (docs.djangoproject.com/en/6.0/topics/async/, /en/6.0/releases/6.0/)
30. **Use `aget`/`asave`/`acreate` directly in async views; reach for `sync_to_async` only for the sync-only
    remainder (transactions, signals, some third-party sync-only code).** Wrapping the _entire_ ORM call chain in
    `sync_to_async` when native async methods exist defeats the purpose — it still hops onto a thread and burns a
    thread-pool slot per call. Reserve `sync_to_async` for the specific sync-only piece, not the whole view body.
31. **The locked decision here is Django's _first-party_ Redis cache backend for Redis routes, not `django-redis`.**
    `django.core.cache.backends.redis.RedisCache` has shipped in Django core since 4.0 —
    `CACHES = {"default": {"BACKEND": "django.core.cache.backends.redis.RedisCache", "LOCATION": "redis://..."}}`.
    This is Django's own cache abstraction (`cache.set`/`get`/`delete` + a `RedisSerializer`), distinct from the
    popular third-party `django-redis` package, which has more features but isn't first-party — `PLAN.md:193`
    locks in the first-party one deliberately ("batteries-included... Django paying for its batteries, same as
    its ORM — representative, not unfair"). Map this repo's CRUD (create/read/update/delete/delete-all) onto
    `cache.set`/`cache.get`/`cache.delete` + read-modify-write for partial updates; if some case genuinely can't
    be expressed through the cache abstraction, that's an escalation (fallback = the shared sync redis-py client),
    not a silent workaround. (docs.djangoproject.com/en/6.0/topics/cache/; PLAN.md:193)
32. **N+1 discipline: `select_related`/`prefetch_related` are not optional once more than one route touches
    related objects.** This repo's `User` model is flat (no relations) so N+1 isn't yet a live risk, but any
    future route that joins across models must reach for these eagerly — Django's ORM will silently emit one
    query per related object otherwise, and that's invisible until someone runs a query-count assertion or watches
    the DB logs under load.
33. **Settings hygiene: one settings module per environment concern, secrets from env vars, never hardcoded.**
    This repo's env-var contract (`ENV`, `HOST`, `PORT`, `POSTGRES_URL`, etc. — same across every server) must be
    read the same way Django reads everything else: via `os.environ`/`django-environ`-style parsing at settings
    load time, not scattered `os.getenv()` calls inside views. Keep `DEBUG = env == "dev"` wired to the same
    `ENV` var every other server uses, matching the "logger off when `ENV=prod`" contract clause.
34. **Run under ASGI (uvicorn), matching the rest of the fleet — not `runserver`/WSGI/gunicorn+sync-workers.**
    `PLAN.md:193` locks this: "Run under ASGI (uvicorn) with async views where idiomatic." Keep pool sizing and
    single-process discipline (§2.16-18) identical to py-fastapi once this lane lands.

---

## 6. Flask (ahead of the Phase 4 lane)

35. **App factory pattern (`create_app()`), not a module-level `app = Flask(__name__)`.** A factory function that
    builds, configures, and returns the app avoids import-time side effects (§8 below) and makes multiple app
    instances (tests, multiple configs) possible without import hacks. Extensions get bound via their own
    `init_app(app)` inside the factory, not by importing an already-bound global extension instance —
    that's what keeps one extension object reusable across multiple app instances instead of hardwiring it to
    whichever app happened to be live at import time. (flask.palletsprojects.com/patterns/appfactories/)
36. **Blueprints for route grouping; use `current_app`/`g`, never a module-level `app` import, inside them.**
    A blueprint written to import a concrete `app` object breaks the moment there's more than one app instance
    (tests being the most common one); `current_app` is the factory-pattern-safe proxy.
37. **`teardown_appcontext` is where per-request resource cleanup belongs — not "connection.close() at the end of
    the view."** The canonical pattern: a `get_db()` helper stores the connection on `g` (`g.db = get_X()` if
    absent), and a function registered via `@app.teardown_appcontext` closes it — cleanup runs even if the view
    raised, which "close it at the end of the view function" does not guarantee.
38. **Flask here is sync WSGI, single-process — the mirror image of §2's async rules, same cardinal sin inverted.**
    Since Flask (per this repo's stack: psycopg3, pymongo, redis-py — all sync drivers) has no event loop to
    block, a slow request just occupies one worker thread/process; the risk moves from "one slow call freezes
    everyone" to "the process has a small, fixed pool of workers and a slow request holds one hostage." Same
    fairness discipline applies: single process, sized pool, no giving Flask an unfair worker-count advantage the
    other frameworks can't match (repo-wide fairness rule in `CLAUDE.md`).

---

## 7. Packaging & tooling

39. **`uv` is the single source of truth for the environment — don't hand-edit `.venv` or use `pip` directly.**
    `uv sync` installs from the lockfile; `uv add`/`uv remove` update `pyproject.toml` + lock atomically; `uv run
<cmd>` runs inside the managed venv without manual activation. The repo's `just install`/`just update`
    orchestrators (per `CLAUDE.md`) should be the only paths that touch dependencies — don't `pip install` inside
    the venv out-of-band, it desyncs the lockfile silently.
40. **`uv`'s pin-awareness matters for prerelease pins — check before bumping.** `CLAUDE.md`: "`just update` is
    pin-aware: prerelease pins... are exempt from blanket bumps." If a Python dependency here is ever pinned to a
    prerelease (mirroring the TS7-RC/drizzle-RC pattern elsewhere in the repo), don't let a routine `uv lock
--upgrade` silently float it past the intended version — check `PLAN.md` §10 first.
41. **Ruff: one config at the language root (`servers/py-fastapi/pyproject.toml`'s `[tool.ruff]`), strict on
    correctness rules, formatter left at defaults.** Ruff ships 900+ rules across many categories (pyflakes `F`,
    a pycodestyle subset `E`, plus `UP`/`B`/`RUF`/etc.); this repo's policy (`CLAUDE.md`/`PLAN.md:16`) is
    "linters strict... formatters at defaults" — don't disable a rule repo-wide to silence one false positive
    (`CLAUDE.md`: "Never... add a rule-wide lint disable... to get green"); fix the code or add a narrowly-scoped
    `# noqa: RULE123` with a reason if the rule is genuinely wrong for that one line. Ruff is a linter, not a type
    checker — it won't catch what pyright strict catches, and vice versa; both gates run (`just verify`).
42. **Pyright strict survival patterns**: annotate every public function signature (return type included, even
    `-> None`); avoid bare `Any` — prefer `object` if the type is genuinely unknown and narrow it before use;
    use `TYPE_CHECKING`-gated imports for types only needed for annotations (see `src/database/postgres.py:14-15`,
    `if TYPE_CHECKING: from sqlalchemy.ext.asyncio import AsyncEngine`) to avoid runtime import cost/cycles;
    reach for `# type: ignore[specific-code]` (never a bare `# type: ignore`) at genuine gaps in a third-party
    library's stubs — this repo already does this for `cassandra-driver`, which ships no type stubs
    (`src/database/cassandra.py:7-8,46,55`, `type: ignore[import-untyped]` / `[union-attr]`).

---

## 8. Common mistakes catalogue

43. **Mutable default arguments.** `def f(items: list = []):` shares one list across every call with no argument
    supplied — the classic trap. Default to `None` and construct inside: `def f(items: list | None = None): items
= items if items is not None else []`.
44. **Late-binding closures in loops.** `[lambda: i for i in range(3)]` produces three closures that all read the
    _final_ value of `i` (late binding), not the value at creation time. Fix by binding the loop variable as a
    default argument: `lambda i=i: i`, or avoid the closure-in-loop shape entirely (e.g. `functools.partial`).
45. **Bare `except:` (or `except Exception:` used to silently swallow everything).** Catches `SystemExit`/
    `KeyboardInterrupt` too if truly bare, and hides bugs. This repo's own health-check methods do use
    `except Exception: return False` (`src/database/*.py`, e.g. `postgres.py:106-107`) — that's a deliberate,
    narrow "treat any failure as unhealthy" contract for a health probe, not a general license to swallow
    exceptions; new code should catch the specific exception type it expects unless it's genuinely a
    last-resort boundary like a health check.
46. **Import-time side effects.** Opening a DB connection, reading a file, or hitting the network at module import
    time makes import order matter and breaks testability (importing the module _does the thing_). This repo's
    repositories correctly defer connection to first use (`self._client: AsyncIOMotorClient | None = None`,
    lazily created in `_collection()`/`_ensure_client()`) rather than connecting in `__init__` or at import time.
47. **`__init__.py` games**: don't re-export half the package's public surface through `__init__.py` star-imports
    just to shorten import paths — it obscures where a name actually lives and can create import cycles. An
    explicit `from src.database.types import User` beats a mystery `from src.database import User` that only
    works because of `__init__.py` re-export wiring.
48. **Misuse of async generators**: an `async def` function with a bare `return value` _and_ a `yield` in it is
    two different things smashed together — pick one. A more common real trap: forgetting that an async generator
    left un-exhausted (a consumer `break`s out of `async for` early) needs `aclose()` called on it eventually, or
    wrap consumption in `async with contextlib.aclosing(gen):` (3.10+) rather than assuming GC handles the
    cleanup deterministically — it doesn't, especially inside another coroutine's cancellation path.
49. **Time zones**: never use naive `datetime.now()`/`datetime.utcnow()` (the latter is deprecated since 3.12) for
    anything stored or compared across systems — use `datetime.now(timezone.utc)` (or `datetime.now(UTC)` with the
    3.11+ `UTC` constant) and store timezone-aware values. A naive datetime compared against an aware one raises
    `TypeError` at the worst possible moment (usually in production, in a code path with poor test coverage).

---

## 9. Testing

50. **pytest is the test runner; group async tests under `pytest-asyncio`.** Either set
    `asyncio_mode = "auto"` in config (every `async def test_*` is treated as a test automatically) or mark each
    async test explicitly with `@pytest.mark.asyncio` — pick one mode repo-wide, don't mix per-file.
51. **Use `httpx.AsyncClient` with `ASGITransport(app=app)` to test FastAPI in-process — never `TestClient` for
    async-flow tests.** `TestClient` wraps the app synchronously and works for simple cases, but blocks the
    async flow, defeating the purpose of testing an async app's concurrency behavior; `AsyncClient(transport=
ASGITransport(app=app), base_url="http://test")` calls the ASGI app directly with no real socket, and lets
    `await client.get(...)` run on the same event loop as the app under test. (fastapi.tiangolo.com/advanced/
    async-tests/)
52. **Match fixture `scope` and `loop_scope` deliberately, or hit "Future attached to a different loop."** A
    session/module-scoped async fixture (e.g. one shared `AsyncClient` for a whole test module, to avoid
    reconnecting per test) needs `@pytest.mark.asyncio(loop_scope="module")` on the tests that use it — a
    function-scoped default loop paired with a module-scoped async resource is the single most common
    pytest-asyncio error in practice.
53. **Test the actual contract shape, not just status codes.** Given this repo's `contract/` harness asserts full
    response bodies (`CLAUDE.md`: "contract cases assert strict full bodies"), unit/integration tests for a
    Python server should do the same — assert the complete JSON body (including absence of `details` when there's
    nothing to report), not just `assert response.status_code == 404`.

---

## 10. In this repo

- **Strictness ladder — applied 2026-07-04 (Stage 2).** Both `servers/py-fastapi/pyproject.toml` and
  `shared/python/pyproject.toml` now carry a byte-identical lint/type block (full-copy convention, same as the
  Go/TS servers):
  - **pyright `typeCheckingMode = "strict"` with ZERO subtractions.** Strict initially surfaced 80 errors, ~66 of
    them `reportUnknown{Member,Argument,Variable}Type` cascading out of the untyped `motor` and `cassandra-driver`
    stubs. Rather than disable those three rules repo-wide (which would also blind them to our own code), the
    library seams are pinned to explicit `Any`: `AsyncIOMotorClient[dict[str, Any]]` /
    `AsyncIOMotorCollection[dict[str, Any]]` in `mongodb.py`, `list[Any]`/`Any` returns on cassandra's
    `_execute`/`_execute_one`. An explicit `Any` annotation is "known" to pyright, so `reportUnknown*` never fires —
    strict then passes clean with **no `report*` flag excluded** (the §7.42 escape-hatch pattern, done at the seam
    instead of per-line). The remaining real fixes were our own code: middleware `call_next: RequestResponseEndpoint`
    - `-> Response` annotations (`main.py`), return-type/type-argument annotations, and one genuine typing bug —
      `mongodb.create` built `doc` as an inferred `dict[str, ObjectId | str]` then assigned an `int` favoriteNumber
      (`reportArgumentType`), fixed by annotating `doc: dict[str, Any]`.
  - **ruff `select = ["E", "F", "B", "UP", "SIM", "C4", "RET", "PERF", "RUF", "S", "ASYNC"]`** — the E/F default
    plus curated bug-catching families; `ASYNC` because this is an async server. Fixes made: 8× `B904` (chain
    wrapped exceptions with `raise … from e`/`from None` in `db.py`/`params.py`), `B008` handled the idiomatic way
    via `flake8-bugbear.extend-immutable-calls` whitelisting the FastAPI DI factories (`Body`/`File`/`Depends`/… —
    NOT a rule-wide B008 disable). Two narrow, rationale-carrying suppressions remain: `# noqa: S608` on cassandra's
    dynamic `UPDATE` (the interpolated fragments are static `"<col> = %s"` literals; all values are `%s`-bound) and
    `# noqa: S104` ×3 on the `HOST = "0.0.0.0"` bind-all default in `shared/python/env.py` (the deliberate container
    default every server shares — a config-level `ignore` there would be the prohibited rule-wide disable).
  - **No test-file overrides yet** — no `tests/` tree exists. A `[tool.ruff.lint.per-file-ignores]` for `tests/**`
    relaxing `S101` (assert) / `S105-6` (hardcoded test creds) returns when the first tests land (noted inline in
    both pyproject.toml files).
  - **`shared/python` is now a gated verify target** (`scripts/lib.mts` EXTRA_TARGETS row `shared-python`, eco
    `uv`) — closes the pre-existing gap flagged in PR #39 review where its twin `shared-typescript`/`shared-go`
    were gated but it was not. It gained its own `[dependency-groups] dev` (pyright + ruff) so the uv eco steps
    (`pyright src` · `ruff format --check .` · `ruff check .`) run standalone against it.
- **Single-process, pool-of-50 is the fairness canon — applied 2026-07-04.** `PLAN.md:194` locks "Normalize
  FastAPI: 1 worker, pg pool 50"; py-fastapi now runs uvicorn with `--workers 1` (`Dockerfile:38`) and
  Postgres uses `pool_size=50, max_overflow=0` (`src/database/postgres.py:37`) — the old `--workers 4` was the
  audit's single "biggest asymmetry" (`PLAN.md:18`, `PLAN.md:72`), fixed by the `audit/production-fairness`
  merge. Any change to `main.py`'s uvicorn invocation or `postgres.py`'s engine construction must hold that
  canon — and must **preserve `--loop uvloop`** (`Dockerfile:38`; see §2.16).
- **Async repos deliberately live inside py-fastapi, not in a shared package — until a second async consumer
  exists.** `PLAN.md:186-192` locks a "multi-consumer rule": shared holds only what has ≥2 real consumers.
  asyncpg/SQLAlchemy-async, motor, redis.asyncio, and the cassandra bridge stay in `src/database/*.py` because
  FastAPI is currently the only async Python framework in the roster; extraction happens only if/when a second
  async framework (the plan names Sanic/Tornado as the shortlist trigger) lands. Sync repos (psycopg3, pymongo,
  redis-py, cassandra-driver) _will_ be shared once Django/Flask both need them (`PLAN.md:191`) — don't
  pre-emptively extract the async ones by analogy.
- **The Cassandra `AddressTranslator` pattern is load-bearing and must be reused by any future Python Cassandra
  code.** `src/database/cassandra.py:13-26`'s `_ContactPointAddressTranslator` pins every node address the
  driver discovers back to the single configured contact point, because the single-node Cassandra container
  advertises `broadcast_rpc_address` as `127.0.0.1` — unreachable from inside another container. Without this,
  the driver reconnects to an address that only resolves on the Cassandra container itself. Fixed by commit
  `15e7834` ("route discovered Cassandra addresses to contact point"); any new Python Cassandra client
  (shared sync repo for Django/Flask, per the item above) needs the same translator, not a copy-pasted
  reimplementation — extract it once there's a second Python Cassandra consumer, per the multi-consumer rule.
- **uvicorn lifespan teardown order**: `initialize_databases()` (health-checks all four DBs via `asyncio.gather`)
  runs before `yield`; `disconnect_databases()` runs after, iterating `_repositories.values()` and calling each
  repo's `disconnect()` before clearing the dict (`src/database/repository.py:62-69`). Any new repository type
  must implement `disconnect()` cleanly (idempotent, safe to call once) — `main.py`'s `lifespan` context manager
  is the only place this runs, so a repository that never got constructed (never touched via `get_repository`)
  correctly never gets a disconnect call either.
- **Error shape is a hard contract, not a FastAPI convention.** `{"error": string, "details"?: string}` via
  `make_error()` (`src/consts/errors.py`) — `details` is omitted, never `null`, when there's nothing to add.
  Django/Flask must reproduce this exact shape; the `contract/` harness will catch drift (`just contract`).
  Never weaken a contract case to make a server's natural error format pass instead (`CLAUDE.md`, "Never").
- **The `UserRepository` Protocol (`src/database/repository.py:13-20`) is the shape every DB backend satisfies** —
  `create`/`find_by_id`/`update`/`delete`/`delete_all`/`health_check`/`disconnect`, all async. Any new async
  Python repository (unlikely per the multi-consumer rule above, but relevant if a second async framework lands)
  should satisfy the same Protocol rather than inventing a parallel shape.
- **pydantic v2 is used for both the wire contract (`User`/`CreateUser`/`UpdateUser` in `src/database/types.py`)
  and env parsing (`Env` in `src/config/env.py`)** — the latter uses lax mode + custom `field_validator`s
  precisely because env vars arrive as strings that need coercion (§4.26); don't "fix" env parsing by turning on
  strict mode.
- **Logger-off-in-prod is wired through the `ENV` var and a plain `if` in middleware**
  (`src/main.py:37-47`, `logging_middleware`) — checked once per request, not via a logging-library
  environment-driven handler swap. Keep this convention consistent if Django/Flask add equivalent request logging.
- **pyright strict + `type: ignore[specific-code]` for untyped third-party libraries is the established pattern**
  for `cassandra-driver` (no stubs) — reuse the same narrow-code-ignore convention rather than a blanket
  `# type: ignore` when Django/Flask bring in their own untyped sync drivers.

---

## Sources

- Python 3.14: [What's New in Python 3.14](https://docs.python.org/3/whatsnew/3.14.html); [Free-threading HOWTO](https://docs.python.org/3/howto/free-threading-python.html); [Coroutines and Tasks — asyncio](https://docs.python.org/3/library/asyncio-task.html)
- FastAPI docs: [Async — Technical Details](https://github.com/fastapi/fastapi/blob/master/docs/en/docs/async.md) (via Context7 `/fastapi/fastapi`); [Streaming — blocking file ops](https://github.com/fastapi/fastapi/blob/master/docs/en/docs/advanced/stream-data.md); [Async Tests](https://fastapi.tiangolo.com/advanced/async-tests/); [Graceful shutdown discussion #6912](https://github.com/fastapi/fastapi/discussions/6912)
- Pydantic v2 docs (via Context7 `/pydantic/pydantic`): [Strict Mode](https://github.com/pydantic/pydantic/blob/main/docs/concepts/strict_mode.md); [Why Pydantic](https://github.com/pydantic/pydantic/blob/main/docs/why.md); [JSON parsing](https://github.com/pydantic/pydantic/blob/main/docs/concepts/json.md)
- Django 6.0: [Django 6.0 release notes](https://docs.djangoproject.com/en/6.0/releases/6.0/); [Asynchronous support](https://docs.djangoproject.com/en/6.0/topics/async/); [Django's cache framework](https://docs.djangoproject.com/en/6.0/topics/cache/)
- Flask 3: [Application Factories](https://flask.palletsprojects.com/en/stable/patterns/appfactories/); [The Application Context](https://flask.palletsprojects.com/en/stable/appcontext/); [Application Structure and Lifecycle](https://flask.palletsprojects.com/en/stable/lifecycle/)
- uvicorn: [Settings](https://www.uvicorn.org/settings/); [Graceful shutdown PR #853](https://github.com/encode/uvicorn/pull/853); [Kludex/uvicorn discussion #2257](https://github.com/Kludex/uvicorn/discussions/2257)
- Ruff: [Ruff Linter docs](https://docs.astral.sh/ruff/linter/); [Ruff Rules](https://docs.astral.sh/ruff/rules/)
- cassandra-driver: [DataStax Python Driver — Performance Notes](https://docs.datastax.com/en/developer/python-driver/3.29/performance/index.html); [cassandra.cluster API](https://docs.datastax.com/en/developer/python-driver/3.25/api/cassandra/cluster/); [ScyllaDB python-driver cassandra.cluster](https://python-driver.docs.scylladb.com/stable/api/cassandra/cluster.html)
- asyncio structured concurrency: [Why Taskgroup and Timeout Are so Crucial in Python 3.11 Asyncio](https://www.dataleadsfuture.com/why-taskgroup-and-timeout-are-so-crucial-in-python-3-11-asyncio/)
- Repo primary sources: `servers/py-fastapi/src/**` (read directly), `servers/py-fastapi/pyproject.toml`, `PLAN.md` (§0, §2 Python section, §11), `CLAUDE.md`
