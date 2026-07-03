# Go Best-Practices Guide — benchmarks repo

Scope: the Go surface of this repo — server modules (go-chi, go-gin, go-fiber, soon
go-stdlib + go-echo) and the benchmark client (load generator, testcontainers-go,
bubbletea TUI). All modules are on **Go 1.27rc1 with `encoding/json/v2`**. This guide
steers implementer and reviewer agents plus human newcomers; wrong advice propagates,
so every rule below carries a source. Verify against current sources before overriding.

Source-numbering note: the "100 Go Mistakes" concurrency material lives in chapters
7 (Foundations, mistakes **#55–#60**) and 8 (Practice, **#61–#74**) — _not_ #55–#79.
HTTP client/server is #81 (Standard Library); false sharing #92 and allocation/`sync.Pool`
#96 (Optimizations). Numbers cited below were verified against https://100go.co (2026-07).

---

## 1. Idioms & style

1. **Wrap errors with `%w`, inspect with `errors.Is`/`errors.As`; never compare error
   strings.** `%w` preserves the chain so callers can match sentinels or extract typed
   errors; `%v` flattens it and breaks `Is`/`As`. Wrap only when you add context —
   otherwise return the error unchanged.

   ```go
   if err != nil { return fmt.Errorf("connect postgres: %w", err) }
   // caller: if errors.Is(err, pgx.ErrNoRows) { ... }
   ```

   Source: Go Code Review Comments (Errors); 100 Go Mistakes #48–#49 (error wrapping/`Is`/`As`).

2. **Wrap once, at the boundary where context is added; don't double-wrap the same fact.**
   Repeated wrapping produces noise like `connect: connect: dial: ...`. Add context the
   caller lacks (operation, key, id), not the callee's own words. Source: 100 Go
   Mistakes #48; Uber Go Style Guide (Error Wrapping).

3. **Sentinel errors are `var ErrX = errors.New(...)`; dynamic errors are custom types
   matched with `errors.As`.** Sentinels for "is this THE condition", typed errors when
   the caller needs fields (status code, field name). Source: Go Code Review Comments.

4. **Design types so the zero value is useful; avoid mandatory constructors where the
   zero value can work.** `sync.Mutex`, `bytes.Buffer` are usable at zero value. When a
   type genuinely needs setup (a pool, a client), a `New*` constructor returning a
   ready struct is idiomatic. Source: Effective Go; Google Go Style Guide.

5. **Accept interfaces, return concrete structs.** Callers get flexibility (any impl of
   the input interface); returning a struct keeps the concrete type's full API available
   and avoids premature interface lock-in. Source: Go Proverbs / Go Code Review Comments;
   Uber Go Style Guide.

6. **Do not create an interface until you have ≥2 real implementations or a real test
   seam.** Single-implementation interfaces are "interface pollution" — indirection with
   no payoff. Define the interface on the _consumer_ side, not the producer. Source:
   100 Go Mistakes #5 (Interface pollution); Go Proverbs ("the bigger the interface, the
   weaker the abstraction").

7. **Prefer a plain config struct for construction; reach for functional options only
   when there are many optional, defaultable knobs.** Options (`func(*Config)`) shine for
   evolving public APIs; for internal wiring a struct literal is clearer and cheaper.
   Source: Uber Go Style Guide (Functional Options); Rob Pike's options blog.

8. **Put non-exported code under `internal/`; it is compiler-enforced privacy.** Anything
   in `internal/` can only be imported by packages rooted at that `internal/`'s parent.
   This repo already leans on it (`internal/app`, `internal/routes`, `internal/database`).
   Source: Go command docs (Internal Directories).

9. **Name for the caller's readability: short receivers, no stutter, MixedCaps not
   underscores.** `chi.Router` not `chi.ChiRouter`; receiver `r *Repo` not `this`/`self`.
   Exported identifiers read as `package.Name`, so don't repeat the package. Source:
   Effective Go (Names); Go Code Review Comments.

10. **Return early; keep the happy path at minimum indentation.** Guard-clause the errors
    and `continue`/`return`, so the main flow reads top-to-bottom. Source: Go Code Review
    Comments (Indent Error Flow); Uber Go Style Guide.

---

## 2. Concurrency (deep — mandated)

> The single most load-bearing rule: **whoever starts a goroutine is responsible for
> ensuring it stops.** A goroutine with no defined stop condition is a leak in waiting.
> Source: 100 Go Mistakes #62 ("Starting a goroutine without knowing when to stop it").

11. **Every goroutine needs an owner and a stop signal before you write the `go`.** The
    stop signal is almost always a `context.Context` cancel or a closed channel. If you
    can't state who cancels it and when, don't start it. This repo's `Start()` funcs model
    it: the server goroutine is stopped by `Shutdown` after `ctx.Done()`. Source: 100 Go
    Mistakes #62; Go blog (Pipelines and cancellation).

12. **`context.Context` is the cancellation backbone — propagate it as the first param,
    never store it in a struct.** A request-scoped context threads deadline + cancellation
    - values through the call tree; stashing it in a struct outlives its scope and defeats
      cancellation. Source: Go Code Review Comments (Contexts); 100 Go Mistakes #60
      ("Misunderstanding Go contexts").

13. **Don't propagate a request context into work that must outlive the request.** When a
    handler kicks off fire-and-forget work, the request's context is cancelled at response
    time and kills it. Use `context.WithoutCancel(ctx)` (Go 1.21+) to keep values but drop
    cancellation — the repo does exactly this in container teardown
    (`ctr.Terminate(context.WithoutCancel(ctx))`). Source: 100 Go Mistakes #61
    ("Propagating an inappropriate context").

14. **Choose channels for ownership transfer / signaling / pipelines; choose mutexes for
    protecting shared in-memory state.** "Share memory by communicating" is the default,
    but a mutex around a map/counter is simpler and faster than a channel dance. Decision
    rule: passing data along → channel; guarding a field → mutex. Source: 100 Go Mistakes
    #57 ("Being puzzled about when to use channels or mutexes").

15. **Understand data race vs. race condition; the memory model gives you no ordering
    without synchronization.** A program can be race-free yet still logically wrong
    (race condition). Unsynchronized reads/writes are UB — there is no "benign" data race
    in Go. Source: 100 Go Mistakes #58 ("Not understanding race problems"); The Go Memory
    Model (go.dev/ref/mem).

16. **Run the race detector as a CI gate, not an afterthought: `go test -race ./...`.** It
    finds real, timing-dependent bugs that pass 1000 clean runs. It ~5–10×'s memory/CPU,
    so it's a test-time gate, never a production build flag. Source: Go blog (Introducing
    the Race Detector); 100 Go Mistakes #58.

17. **`errgroup.Group` for a fan-out of tasks that share a cancellation + first-error.**
    It bounds goroutine lifetime, propagates the first error, and cancels siblings via the
    derived context. Use `g.SetLimit(n)` to cap concurrency. Preferred over hand-rolled
    `sync.WaitGroup` + error channel. Source: 100 Go Mistakes #73 ("Not using errgroup");
    pkg.go.dev/golang.org/x/sync/errgroup.

    ```go
    g, ctx := errgroup.WithContext(ctx); g.SetLimit(8)
    for _, s := range servers { s := s; g.Go(func() error { return probe(ctx, s) }) }
    err := g.Wait()
    ```

18. **`sync.Once` for exactly-once init (lazy singletons); `sync.Pool` for reusable
    ephemeral allocations; `singleflight` to collapse duplicate concurrent work.** The
    repo uses `once.Do` to build the pgx pool once under concurrent first-hits. Source:
    pkg.go.dev/sync; golang.org/x/sync/singleflight.

19. **`sync.WaitGroup`: `Add` before you `go`, `Done` in a `defer` inside the goroutine,
    `Wait` in the owner.** Calling `Add` inside the goroutine races with `Wait`. Never copy
    a `WaitGroup` (or any `sync` type) after first use — pass a pointer. Source: 100 Go
    Mistakes #71 ("Misusing sync.WaitGroup"), #74 ("Copying a sync type").

20. **Kill the classic leaks:** (a) **blocked send on an unbuffered/full channel** whose
    receiver already left — size the channel or use `select { case ch<-v: case <-ctx.Done() }`;
    the repo's `errCh := make(chan error, 1)` is buffered precisely so the server goroutine
    can't block sending after the select moved on. (b) **forgotten receiver** — a producer
    that never gets drained. (c) **`time.After` in a `for`/`select` loop** allocates a new
    timer each iteration that isn't GC'd until it fires; use a reset `time.Timer`/`time.Ticker`.
    Source: 100 Go Mistakes #62, #67 ("channel size"); Go docs on `time.After`.

21. **`select` behavior is nondeterministic across ready cases, and a nil channel blocks
    forever — use that on purpose.** Don't assume case ordering; set a channel to `nil` to
    disable a `select` arm dynamically. Source: 100 Go Mistakes #64 ("Expecting a
    deterministic behavior using select and channels"), #66 ("Not using nil channels").

22. **Concurrency is not parallelism, and more goroutines is not always faster.** Small
    workloads lose to goroutine/scheduling/synchronization overhead; benchmark before
    parallelizing. Know your workload: CPU-bound wants ~`GOMAXPROCS` workers, I/O-bound can
    over-subscribe. Source: 100 Go Mistakes #55 ("Mixing up concurrency and parallelism"),
    #56 ("Thinking concurrency is always faster"), #59 ("concurrency impacts of a workload
    type").

23. **Know the scheduler realities: `GOMAXPROCS` defaults to the CPU count (Go 1.25+ is
    container-cgroup-aware), goroutines are M:N-multiplexed, and blocking syscalls park an
    OS thread.** Don't hand-set `GOMAXPROCS` in servers unless a benchmark proves it; the
    container CPU limit already shapes it here. Source: Go runtime docs; Go 1.25 release
    notes (container-aware `GOMAXPROCS`).

24. **Avoid false sharing in hot parallel counters: pad or shard per-CPU state so
    independent variables don't share a cache line.** Two goroutines writing adjacent fields
    of one struct thrash each other's caches. Relevant to the benchmark client's per-worker
    counters. Source: 100 Go Mistakes #92 ("Writing concurrent code that leads to false
    sharing").

25. **Watch `append` and map/slice mutation under concurrency — they are not safe.**
    Concurrent `append` to a shared slice, or a mutex that guards the slice header but not
    the backing array, both race. Source: 100 Go Mistakes #69 ("Creating data races with
    append"), #70 ("Using mutexes inaccurately with slices and maps").

---

## 3. HTTP server specifics

26. **Never use `http.Server{}` with default (zero) timeouts on a public listener — set
    `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and `ReadHeaderTimeout`.** Zero means "no
    timeout", so a slow-loris client ties up a connection forever. The repo's chi server
    sets Read/Write=15s, Idle=60s (see §In this repo for the `ReadHeaderTimeout` gap).
    Source: 100 Go Mistakes #81 ("Using the default HTTP client and server"); Cloudflare
    "The complete guide to Go net/http timeouts".

27. **Bound request bodies with `http.MaxBytesReader` (or `r.Body = http.MaxBytesReader(...)`).**
    Without it a client can stream an unbounded body into memory. It caps the read and
    signals the client with a 413-appropriate error. Source: net/http docs;
    100 Go Mistakes #81.

28. **`server.Shutdown(ctx)` drains _in-flight_ requests and stops accepting new ones — it
    does NOT force-close hijacked, WebSocket, or streaming connections.** Give it a bounded
    context (the repo uses 15s). Long-lived/hijacked conns must be tracked and closed
    yourself via `RegisterOnShutdown` or your own signal. `Shutdown` returns
    `ctx.Err()` if the deadline hits first. Source: net/http `Server.Shutdown` docs.

29. **Treat `http.ErrServerClosed` from `ListenAndServe` as a clean stop, not an error.**
    It's the sentinel returned after `Shutdown`/`Close`. The repo filters it with
    `errors.Is(err, http.ErrServerClosed)`. Source: net/http docs.

30. **Use `r.Context()` for per-request cancellation and deadlines; derive DB/RPC contexts
    from it.** When the client disconnects, the request context cancels and your downstream
    pgx/mongo calls should abort. Never background-detach unless the work must outlive the
    request (then `context.WithoutCancel`). Source: Go Code Review Comments (Contexts);
    net/http docs.

31. **When load-testing, remember the client side reuses connections — a leaked response
    body or `DisableKeepAlives` skews results.** Always `io.Copy(io.Discard, resp.Body)` +
    `resp.Body.Close()` so the connection returns to the pool; otherwise you measure
    connection setup, not the server. Tune `Transport.MaxIdleConnsPerHost` for the load
    generator. Source: 100 Go Mistakes #81; net/http `Transport` docs.

---

## 4. Performance

32. **Know the escape-analysis basics: values that outlive their stack frame (returned
    pointers, interface boxing, closures capturing by reference) escape to the heap.**
    Check with `go build -gcflags='-m'`. Reducing escapes reduces GC pressure. Don't guess —
    measure. Source: 100 Go Mistakes #95–#96; Go compiler docs.

33. **Pre-size slices and maps when the length is known: `make([]T, 0, n)` / `make(map[K]V, n)`.**
    Growth reallocates and rehashes; pre-sizing is often the cheapest win. Source: 100 Go
    Mistakes #21 ("Inefficient slice initialization"), #27 (map allocation).

34. **`sync.Pool` only pays off for large, frequently-allocated, short-lived objects with a
    clear reset — and it is a cache, not a guarantee.** Pooled objects can be GC'd at any
    time; pooling tiny objects loses to the allocator. Always reset before `Put`. Source:
    100 Go Mistakes #96 ("Not knowing how to reduce allocations"); sync.Pool docs.

35. **Avoid needless `string`↔`[]byte` conversions in hot paths; each copies.** The `bytes`
    package mirrors `strings`, so operate on the type you already have. Go 1.27's json/v2
    `MarshalWrite`/`jsontext.Encoder` lets you skip the intermediate `[]byte` entirely (the
    repo's gin `WriteResponse` marshals straight to the `ResponseWriter`). Source: 100 Go
    Mistakes #40 ("Useless string conversions").

36. **pprof-first: profile before optimizing, always.** Wire `net/http/pprof` (guarded, not
    on the benchmarked port) or use `runtime/pprof` in the client. Optimize what the CPU/heap
    profile shows, not what you suspect. Source: Go blog (Profiling Go Programs); Go Proverbs.

37. **Benchmark hygiene: use `testing.B`, call `b.ReportAllocs()`, keep the result escaping
    a package-level sink to defeat dead-code elimination, and prefer `b.Loop()` (Go 1.24+)
    over `for i := 0; i < b.N; i++`.** `b.Loop()` correctly scopes setup/teardown and keeps
    args alive. Source: testing package docs; Go 1.24 release notes (`B.Loop`).
    ```go
    func BenchmarkMarshal(b *testing.B) {
        b.ReportAllocs()
        for b.Loop() { sink, _ = json.Marshal(u) }
    }
    ```

---

## 5. encoding/json/v2 (Go 1.27 — NEW, verify, don't trust stale memory)

38. **In Go 1.27, `encoding/json/v2` and `encoding/json/jsontext` are real stdlib packages
    and the v2 engine backs `encoding/json` v1 by default — no `GOEXPERIMENT=jsonv2`
    needed.** The opt-out is `GOEXPERIMENT=nojsonv2` (expected to be removed later). This
    repo imports `encoding/json/v2` and `encoding/json/jsontext` directly. Source: Go 1.27
    release notes (go.dev/doc/go1.27); go.dev/blog/jsonv2-exp.

39. **v2 defaults differ from v1 — internalize these five, they change behavior:**

    | Behavior                                                     | v1                   | v2 (default)                                 |
    | ------------------------------------------------------------ | -------------------- | -------------------------------------------- |
    | Field-name match                                             | case-**insensitive** | case-**sensitive**                           |
    | Duplicate object keys                                        | last wins (silent)   | **error**                                    |
    | `nil` slice / map marshal                                    | `null`               | `[]` / `{}`                                  |
    | `omitempty`                                                  | omits Go zero values | omits **empty JSON** (`null`,`""`,`[]`,`{}`) |
    | Invalid UTF-8 / `time.Duration`                              | lenient              | **error** (opt back in via options)          |
    | Source: go.dev/blog/jsonv2-exp; pkg.go.dev/encoding/json/v2. |

40. **To accept v1-style duplicate keys (last-wins), pass `jsontext.AllowDuplicateNames(true)`
    at the decode site.** In this suite that matches JS/Python `JSON.parse` semantics and is
    **contract canon** for request/response decoding. It's an explicit per-call option, not
    a global. Source: pkg.go.dev/encoding/json/jsontext; repo `internal/routes/params.go`.

    ```go
    var decodeOpts = jsontext.AllowDuplicateNames(true)
    err := json.Unmarshal(body, &out, decodeOpts)      // or json.UnmarshalRead(r.Body, &out, decodeOpts)
    ```

41. **To re-enable v1 case-insensitive matching, use `json.MatchCaseInsensitiveNames(true)`
    — but prefer exact `json:"name"` tags and leave the strict default on.** Case-sensitive
    matching is faster and closes a real class of security bug. Source: pkg.go.dev/encoding/json/v2.

42. **Stream with `jsontext.Encoder`/`Decoder` for large or incremental payloads; use
    `MarshalWrite`/`UnmarshalRead` to avoid a `[]byte` round-trip.** `jsontext` is the
    syntactic layer (tokens/values, no reflection); `json/v2` is the semantic layer. The
    repo reads docker-stats streams token-by-token off a `jsontext.NewDecoder`. Source:
    go.dev/blog/jsonv2-exp; repo `internal/container/stats.go`.

43. **For custom marshaling in hot paths, implement the streaming `MarshalerTo` /
    `UnmarshalerFrom` (`MarshalJSONTo(*jsontext.Encoder)` / `UnmarshalJSONFrom(*jsontext.Decoder)`)
    rather than the allocation-heavy v1 `MarshalJSON`/`UnmarshalJSON`.** v1 interfaces still
    work but force a buffer round-trip. Source: pkg.go.dev/encoding/json/v2.

44. **Unmarshal is markedly faster under v2; Marshal is ~parity.** Don't cite a fixed
    multiplier as gospel — benchmark your payloads. Source: go.dev/blog/jsonv2-exp
    (measured on Go's suites; UNVERIFIED for this repo's exact shapes).

---

## 6. Common mistakes catalogue (server-relevant, non-concurrency)

45. **Variable shadowing with `:=` inside a block silently creates a new variable — the
    outer one keeps its old value.** Classic with `err` re-declared in an `if`. `go vet`
    and golangci-lint's `govet`/`shadow` catch many; watch nested scopes. Source: 100 Go
    Mistakes #1 ("Unintended variable shadowing").

46. **Know nil slice vs. empty slice vs. `null` in JSON.** `var s []T` is nil; `[]T{}` is
    empty-non-nil. Both `len==0`. Under json/v2 a **nil slice now marshals as `[]`, not
    `null`** (rule 39) — this is a behavior change reviewers must check against contract
    cases that assert `[]` vs `null`. Source: 100 Go Mistakes #22 ("nil vs. empty slice");
    json/v2 blog.

47. **Don't `defer` inside a loop when the resource must be released each iteration —
    defers stack until function return.** Wrap the body in a closure/function, or release
    explicitly. A per-iteration `defer rows.Close()` holds every connection until the
    function exits. Source: 100 Go Mistakes #35 ("Using defer inside a loop").

48. **Handle time correctly: compare with `time.Since`, store UTC, use monotonic-clock-aware
    `time.Time` for durations, and never `==`-compare `time.Time` (use `.Equal`).** Wall-clock
    equality breaks across locations/monotonic readings. Source: 100 Go Mistakes (Time
    section); time package docs.

49. **Avoid `init()` for anything non-trivial — it's implicit, unordered across files, and
    untestable.** Prefer explicit constructors called from `main`. Reserve `init` for
    registering with a required global (rare). Source: 100 Go Mistakes #3 ("Misusing init
    functions"); Google Go Style Guide.

50. **Don't over-abstract with interfaces (interface pollution, rule 6 restated for the
    catalogue).** Also avoid empty-`interface{}`/`any` where a concrete type or generic
    would do. Source: 100 Go Mistakes #5.

51. **Check every error; blank-assigning `_ = f()` on a call that can fail is a review
    red flag unless justified with a comment** (the repo does this deliberately for
    best-effort `Terminate` on an already-failing path — and comments why). Source: Go Code
    Review Comments (Handle Errors).

---

## 7. Testing

52. **Table-driven tests with subtests: `for _, tc := range cases { t.Run(tc.name, ...) }`.**
    One readable case list, isolated failures, targeted `-run`. Source: Go blog (Using
    Subtests and Sub-benchmarks); Go wiki (TableDrivenTests).

53. **`t.Parallel()` for independent tests, and gate CI with `-race`.** Parallel + race is
    where data races surface. Capture the range variable (`tc := tc`) before `t.Parallel`
    on Go <1.22; 1.22+ per-iteration loop vars make this unnecessary but harmless. Source:
    testing docs; 100 Go Mistakes (Testing chapter).

54. **Use `net/http/httptest` for handler tests: `httptest.NewRecorder()` for unit-level,
    `httptest.NewServer()` for full round-trips.** No real ports, deterministic, fast — the
    right layer for routing/handler assertions. Source: net/http/httptest docs.

55. **testcontainers-go for integration against real Postgres/Mongo/Redis/Cassandra;
    always pass a `context.Context`, wait on a readiness `wait.Strategy`, and terminate in
    cleanup.** The repo's `container.Start` composes `wait.ForHTTP` strategies via
    `wait.ForAll(...).WithStartupTimeoutDefault(...)` and terminates partial containers so
    nothing leaks; Ryuk is the backstop, not the primary teardown. Source:
    golang.testcontainers.org; repo `internal/container/lifecycle.go`.

56. **Reproduce a bug with a failing test first, then fix, then prove green with the same
    test (repo CLAUDE.md mandate).** Applies to conformance drift too — a contract case is
    the reproduction. Source: repo CLAUDE.md / PLAN §11.2.

---

## In this repo

- **All Go modules pin `go 1.27rc1`** and `scripts/lib.mts` sets `GOTOOLCHAIN=go1.27rc1`
  (belt-and-suspenders over the `go` directive). Don't add `GOEXPERIMENT` — json/v2 is the
  1.27 default; `GOEXPERIMENT=nojsonv2` would _break_ the direct `encoding/json/v2` imports.
- **json/v2 is used directly, not via v1 shim.** `encoding/json/v2` + `encoding/json/jsontext`
  are imported across servers and the client (`response.go`, `params.go`, `stats.go`,
  `config/loader.go`, `roster.go`, ...).
- **Duplicate-keys = last-wins is contract canon at every _request/response_ decode site**
  via `jsontext.AllowDuplicateNames(true)` — this mirrors JS/Python `JSON.parse` so all
  frameworks agree. See go-chi `routes/params.go`, go-gin/go-fiber `utils/response.go`,
  client `client/response.go`.
- **Config loading deliberately does NOT allow duplicate keys** (`config/loader.go`): a
  duplicated config key is an operator mistake the strict v2 default should surface. Two
  intentional policies — don't "unify" them.
- **go-fiber's `StructValidator` is deliberately nil** (`go-fiber/internal/app/app.go`):
  keeps `c.Bind().Body()` in manual decode-only mode so bind errors return to the handler
  and validation stays in go-playground handlers like every other server. Setting one would
  change error status/shape. Don't add it.
- **fiber wraps json/v2 in non-variadic `JSONEncoder`/`JSONDecoder` funcs** because v2's
  variadic-options signature doesn't satisfy fiber's func types directly — expected, not a
  smell.
- **Server graceful shutdown pattern is uniform** (`internal/app/start.go` per framework):
  `signal.NotifyContext(SIGINT,SIGTERM)` → run listener in a goroutine sending to a
  **buffered** `errCh` (size 1, so it can't block after the select) → on `ctx.Done()` call
  `Shutdown`/`ShutdownWithContext` with a 15s timeout. chi/gin filter
  `http.ErrServerClosed`; fiber's `Listen` returns nil on clean close.
- **Reviewer note — `http.Server` timeouts:** chi/gin/stdlib set Read/Write=15s, Idle=60s
  but **not `ReadHeaderTimeout`** (rule 26). Flag if a slow-header defense is wanted; keep
  it identical across frameworks for fairness (repo CLAUDE.md: never give one framework an
  advantage the others can't have idiomatically).
- **Pools are pinned equal for fairness:** pgxpool `MaxConns=50 / MinConns=10`, built once
  under `sync.Once` (`internal/database/postgres.go`). Never bump one framework's pool
  without matching the rest (repo fairness rule).
- **`context.WithoutCancel(ctx)` on teardown** (`container/lifecycle.go`) is the correct
  "must outlive the cancelled parent" idiom (rule 13) — terminate must run even when the
  run's context is already cancelled.
- **Race detector + `just verify`/`just contract` are the gates** (PLAN §11.2). Run
  `go test -race` for any concurrency-touching change; reproduce bugs with a contract case
  before fixing.
- **`scripts/contract.mts` stays npm-dependency-free** and `scripts/*.mts` run under Node 26
  native type-stripping — Go changes never add a bash branch; a new server is one roster
  row / `bench.json` manifest.
- **Idiomatic-per-framework, shared-infra-only** (PLAN §3): DB clients/schemas/validation
  rules/consts/env may be shared; routing/handlers/app structure stay per-framework in that
  framework's canonical style. Don't hoist a handler into a shared package to cut duplication.

---

## Sources (verify current before overriding)

- **100 Go Mistakes** — https://100go.co (structure verified 2026-07: concurrency = ch.7
  Foundations #55–#60, ch.8 Practice #61–#74; #81 HTTP; #92 false sharing; #96 allocations;
  #1/#3/#5/#20–22/#35/#40 general). Book by Teiva Harsanyi.
- **encoding/json/v2 blog** — https://go.dev/blog/jsonv2-exp
- **Go 1.27 release notes** — https://go.dev/doc/go1.27 (json/v2 default; `GOEXPERIMENT=nojsonv2` opt-out)
- **pkg docs** — https://pkg.go.dev/encoding/json/v2 , https://pkg.go.dev/encoding/json/jsontext
- **Effective Go** — https://go.dev/doc/effective_go
- **Go Code Review Comments** — https://go.dev/wiki/CodeReviewComments
- **Google Go Style Guide** — https://google.github.io/styleguide/go/
- **Uber Go Style Guide** — https://github.com/uber-go/guide/blob/master/style.md
- **The Go Memory Model** — https://go.dev/ref/mem
- **Race Detector / Profiling / Pipelines blogs** — https://go.dev/blog/
- **testcontainers-go** — https://golang.testcontainers.org/
- Repo primary sources: `servers/go-*/internal/app/start.go`, `servers/go-fiber/internal/app/app.go`,
  `servers/go-chi/internal/routes/params.go`, `servers/go-chi/internal/database/postgres.go`,
  `benchmark/internal/container/lifecycle.go`, `benchmark/internal/config/loader.go`, `scripts/lib.mts`.

_Marked UNVERIFIED where noted (json/v2 exact speedup on this repo's payloads). Style-guide
rules drawn from stable canonical guides; library-specific API names checked against Go 1.27
pkg docs and the repo's own usage._
