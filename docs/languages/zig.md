# Zig Best Practices (targeting Zig 0.16.0)

Scope: the one Zig server in this repo (`servers/zig`) — http.zig (karlseguin,
thread-per-worker + thread pool), pg.zig, a hand-rolled RESP2 redis client with a
blocking connection pool, libmongoc + Cassandra cpp-driver via `@cImport`, and
hand-parsed multipart.

**Read this first, because Zig 0.16 is a moving target.** 0.16 landed the single
biggest std reshuffle in years: the new `std.Io` interface, `Io.Writer`/`Io.Reader`
replacing the old writer/reader, threading primitives moved from `std.Thread.*` to
`std.Io.*`, `ArrayList` gone fully unmanaged, and async/await returning in a new
form. Most training data and most blog posts predate this. When in doubt, check the
[0.16.0 release notes](https://ziglang.org/download/0.16.0/release-notes.html) and
read the actual `lib/std` source for the compiler you have installed —
"I remember it working like X" is not evidence on this version.

Rules are imperative + rationale + (sketch where it disambiguates) + source. Anything
I could not pin to a current source is marked **UNVERIFIED**.

---

## 1. Memory & allocators

Zig has no hidden allocations. Every allocation is explicit and every allocator is a
value you hand around. This is the discipline the whole language is organized around.

1.1. **Take `std.mem.Allocator` as an explicit parameter; never reach for a global.**
A function that allocates advertises it in its signature. This is why this repo
threads `allocator` (long-lived) and `arena` (per-request) into every DB method:
`create(self, arena, data)`. Rationale: the caller owns the memory strategy, so the
same code works under GPA, arena, or a test allocator.
Source: [zig.guide/standard-library/allocators](https://zig.guide/standard-library/allocators/).

1.2. **Pick the allocator to match the lifetime, not out of habit.**

- _Arena_ (`std.heap.ArenaAllocator`): many allocations, one bulk free. Perfect for
  a request: allocate freely, never individually free, drop it all at the end. The
  whole repo leans on http.zig's `res.arena` for exactly this.
- _GPA_ (`std.heap.GeneralPurposeAllocator` / `DebugAllocator`): general long-lived
  objects; in debug builds it detects leaks, double-frees, and use-after-free.
- _FixedBufferAllocator_: a stack/`[]u8` buffer, zero heap. Great for bounded, known
  work — but **it fails hard when you exceed the buffer**, which is a real source of
  truncation bugs (see §10.4).
- _c_allocator_ (`std.heap.c_allocator`): requires `link_libc`; this server uses it
  (`servers/zig/src/main.zig`) because it already links libc for the C drivers.
  Source: [release notes, allocator section](https://ziglang.org/download/0.16.0/release-notes.html);
  [zig.guide allocators](https://zig.guide/standard-library/allocators/).

1.3. **0.16: `ThreadSafeAllocator` is gone; `ArenaAllocator` is now lock-free and
thread-safe.** The rationale in the release notes: a thread-safe wrapper can only be
a mutex, which now needs an `Io`, and is slow. Make the underlying allocator
thread-safe instead. `c_allocator` (glibc/musl malloc) is already thread-safe, which
is why sharing it across http.zig workers here is fine.
Source: [0.16 release notes](https://ziglang.org/download/0.16.0/release-notes.html).

1.4. **Pair every acquire with `defer`, and every _partially-built_ resource with
`errdefer`.** `defer` runs on all exits; `errdefer` runs only on error return. Use
`errdefer` to unwind a resource you have created but not yet successfully handed to
its owner.

```zig
const conn = try allocator.create(Conn);
errdefer allocator.destroy(conn);      // frees only if a later step fails
conn.* = .{ .stream = try self.dial() }; // if dial() errors, errdefer fires
```

Rationale: without the `errdefer`, an error after `create` but before the value is
stored leaks. Note `defer`/`errdefer` fire in **reverse** order of registration.
Source: [Zig langref — defer/errdefer](https://ziglang.org/documentation/0.16.0/#defer).

1.5. **State the ownership contract in the signature and a comment: who frees, and with
which allocator.** Zig can't express ownership in the type system, so convention
carries it. This repo's rule: repository methods return values whose slices are
**arena-owned** (the `arena` passed in), so handlers never free field-by-field. Any
function that `dupe`s into a caller's allocator (e.g. `rowToUser`) must document that
the result lives as long as that allocator.
Source: repo convention (`src/db/postgres.zig:72`, `src/db/redis.zig:335`).

1.6. **Return the memory to the allocator it came from — never mix.** Freeing arena
memory with the GPA (or vice versa) is UB. Because http.zig hands you `res.arena`,
allocate response bodies there and never call `free` on them; the arena reset does it.

1.7. **Use sentinel-terminated slices (`[:0]const u8` / `[*:0]const u8`) at the C
boundary.** C wants NUL-terminated `char*`; Zig slices carry a length and are _not_
NUL-terminated. Use `allocator.dupeZ(u8, s)` to get a `[:0]u8`, or `std.mem.span` to
turn a C `[*:0]` back into a slice. This repo does both: `allocator.dupeZ` for contact
points (`src/db/cassandra.zig:27`) and a fixed `[37]u8` NUL-terminated stamp for a
36-char UUID (`zterm`, `src/db/cassandra.zig:215`). Passing a plain `[]const u8`
`.ptr` to a C function that expects termination reads past the end.
Source: [langref — Sentinel-Terminated Pointers](https://ziglang.org/documentation/0.16.0/#Sentinel-Terminated-Pointers).

1.8. **Prefer the length-taking C variants when the driver offers them.** e.g.
`cass_statement_bind_string_n(stmt, i, s.ptr, s.len)` avoids needing a NUL terminator
at all (`src/db/cassandra.zig:93`). Fewer copies, no termination footgun.

---

## 2. Error handling

2.1. **Model failure with error sets + error unions (`!T`), not sentinels or out-params.**
`fn find(...) !?User` says "may fail, may be absent" precisely. Let the compiler infer
the error set (`!T`) for leaf functions; name an explicit set (`error{Invalid}`) when
it's part of your API contract, as `user.zig` does with `ValidationError`.
Source: [langref — Errors](https://ziglang.org/documentation/0.16.0/#Errors).

2.2. **`try` to propagate, `catch` to handle, and handle at the layer that has context.**
The DB layer returns errors; the _handler_ maps them to HTTP status. See
`routes_db.zig`: `backend.create(...) catch { writeError(res, 500, ...); return; }`.
Don't smear HTTP concerns into the DB layer or DB concerns into the router.

2.3. **`catch |e|` to inspect; `@errorName(e)` to render.** This repo surfaces parse
failures to the client via `@errorName(e)` in the `details` field
(`routes_db.zig:27`). Cheap, allocation-free diagnostics.

2.4. **Crash only on true programmer errors; propagate everything environmental.**
`unreachable` and `@panic` assert _impossible_ states (and `unreachable` is UB in
ReleaseFast). Network down, malformed input, missing row → these are `error` values,
not panics. A server must not `@panic` on a bad request. Use `std.debug.assert` for
invariants you control; return errors for anything a client or peer can cause.
Source: [langref — unreachable](https://ziglang.org/documentation/0.16.0/#unreachable).

2.5. **Never silently discard an error union.** `_ = mightFail();` throws away a
possible error and the compiler won't stop you if you assign to `_`. Be deliberate:
`catch {}` (with a comment on why ignoring is safe), `catch |e| log`, or `try`. The
repo uses `catch {}` intentionally on best-effort reconnect (`redis.zig:86`) and on
`row.deinit() catch {}` — each is a conscious choice, not an oversight.

2.6. **`errdefer` is your diagnostic-safe cleanup.** When building something across
several `try` steps, `errdefer` guarantees partial state is unwound no matter which
step fails — you don't need a manual failure ladder. (See §1.4.)

---

## 3. Concurrency (deep — this is where 0.16 changed most)

**Verify the state of the world before trusting any memory of it.** async/await in Zig
has a _history_: it existed pre-0.10, was **removed** because the stackless-coroutine
design couldn't serve every execution model, and **returned in 0.16** in a completely
different form built on the new `std.Io` interface. Blog posts and LLM training from
2021–2024 describe the old, dead design. The following is the 0.16 model.
Sources: [Andrew Kelley — Zig's New Async I/O](https://andrewkelley.me/post/zig-new-async-io-text-version.html);
[Loris Cro — Zig's New Async I/O](https://kristoff.it/blog/zig-new-async-io/);
[0.16 release notes](https://ziglang.org/download/0.16.0/release-notes.html).

3.1. **`Io` is an interface you pass around, like an allocator.** You get one from
`std.process.Init` in `main` (`init.io`) and thread it into everything that does I/O.
This repo does exactly that: `pub fn main(init: std.process.Init)` → `const io = init.io`
→ `Redis.init(io, ...)`, `Postgres.init(io, ...)`. Backends implemented against `Io`
can be swapped between a threaded backend and (eventually) an evented one without
touching call sites.
Source: `src/main.zig:20-33`; [release notes](https://ziglang.org/download/0.16.0/release-notes.html).

3.2. **Threading primitives moved from `std.Thread.*` to `std.Io.*` and take an `io`.**
The renames that bite: `std.Thread.Mutex` → `std.Io.Mutex`, `.Condition` →
`std.Io.Condition`, `.Semaphore` → `std.Io.Semaphore`, `.RwLock` → `std.Io.RwLock`,
`ResetEvent` → `std.Io.Event`, `WaitGroup` → `std.Io.Group`, `Futex` → `std.Io.Futex`.
`std.Thread.Mutex.Recursive` and `std.once` were **removed**. Lock/unlock/wait now
take the `io`: `mutex.lock(io)`, `cond.wait(io, &mutex)`.
Source: [0.16 release notes — std.Thread → std.Io](https://ziglang.org/download/0.16.0/release-notes.html).

3.3. **`io.async` decouples call from return; `io.concurrent` demands real parallelism.**

- `io.async(fn, args)` returns a `Future(T)`. It _may_ run concurrently, or may run
  when you `await`. Do not assume overlap.
- `io.concurrent(fn, args)` guarantees simultaneous progress or fails **immediately**
  with `error.ConcurrencyUnavailable` (e.g. `builtin.single_threaded`) instead of
  deadlocking. Use it when correctness _needs_ two things running at once (classic
  producer/consumer on a bounded queue).
  Kelley's worked examples: `async` can succeed by luck with a big thread pool yet
  **deadlock at pool size 1**; `concurrent` makes the requirement explicit and fails
  loud. Prefer `concurrent` whenever a deadlock would otherwise be possible.

```zig
var f = io.async(compute, .{x});   // Future(T); may or may not overlap
const result = f.await(io);        // idempotent with cancel()
```

Source: [Andrew Kelley — Zig's New Async I/O](https://andrewkelley.me/post/zig-new-async-io-text-version.html).

3.4. **`await(io)` and `cancel(io)` both return the future's result; use `cancel` in
`defer` to unwind un-awaited work.** They're idempotent with each other. The
leak-proof pattern for a future that allocates:

```zig
defer if (future.cancel(io)) |resource| free(resource) else |_| {};
```

The model does _not_ auto-cancel dropped futures — leaking an un-awaited async task is
your bug to prevent.
Source: [Kelley](https://andrewkelley.me/post/zig-new-async-io-text-version.html).

3.5. **Know your backend. `Io.Threaded` is the only production-ready one in 0.16;
evented is experimental.** `Io.Threaded` is a thread pool spawning OS threads — fine
for a bounded request-handler workload, but it does not scale to tens of thousands of
concurrent in-flight async tasks (practical ceiling ~10k before hitting OS thread
limits). `Io.Evented` / `Io.Uring` / `Io.Kqueue` / `Io.Dispatch` exist but are
proof-of-concept / **may not even compile** in 0.16. Don't design around evented I/O
yet.
Source: [release notes](https://ziglang.org/download/0.16.0/release-notes.html);
[Lukáš Lalinský — Async I/O in Zig 0.16, today](https://lalinsky.com/2026/05/11/async-io-in-zig-016-today.html).

3.6. **The `*Uncancelable` lock/wait variants opt out of cancellation.** This repo's
redis pool uses `mutex.lockUncancelable(io)` and `cond.waitUncancelable(io, &mutex)`
(`src/db/redis.zig:72-74`). Rationale: in the 0.16 model an ordinary blocking op can
be _cancelled_ (returns `error.Canceled`); the `Uncancelable` forms promise the call
won't be interrupted, so pool bookkeeping (decrement `idle_count`, hand out a conn)
can't be torn apart mid-critical-section. Use them for short, must-complete critical
sections where handling a cancellation would only complicate correctness.
Source: repo (`src/db/redis.zig`); primitive naming per
[release notes](https://ziglang.org/download/0.16.0/release-notes.html). Exact
cancellation contract of each variant is **UNVERIFIED against a single canonical doc**
— confirm in `lib/std/Io.zig` for your compiler before relying on edge behavior.

3.7. **Mutex + condition = the blocking-pool pattern. Follow the acquire/wait/signal
shape exactly.** The redis client is the reference:

```zig
fn acquire(self: *Redis) *Conn {
    self.mutex.lockUncancelable(self.io);
    defer self.mutex.unlock(self.io);
    while (self.idle_count == 0) self.cond.waitUncancelable(self.io, &self.mutex);
    self.idle_count -= 1;
    return self.idle[self.idle_count];
}
// release: lock, push conn back, idle_count += 1, cond.signal(io), unlock.
```

Non-negotiables: (a) **`while`, not `if`**, around the wait — spurious wakeups and
multiple waiters are real; re-check the predicate. (b) `wait` atomically releases the
mutex and re-acquires on wake. (c) `defer unlock` so every path unlocks.
Source: repo (`src/db/redis.zig:71-93`); classic condition-variable discipline.

3.8. **Atomics need an explicit memory ordering; pick the weakest that's correct.**
Use `@atomicLoad`/`@atomicStore`/`@atomicRmw`/`@cmpxchg*` with an `AtomicOrder`
(`.monotonic`, `.acquire`, `.release`, `.acq_rel`, `.seq_cst`). `.monotonic` for a
plain counter; `.acquire`/`.release` to publish/consume data across threads;
`.seq_cst` when unsure (correct but slowest). A non-atomic read racing a write is UB.
Source: [langref — Atomics / @atomicRmw](https://ziglang.org/documentation/0.16.0/#atomicRmw).

3.9. **Zig has no data-race detector — shared mutable state is on you.** There is no
`-race` equivalent. Any memory touched by more than one thread must be guarded by a
mutex or accessed atomically. In this server, the CassSession is documented
internally thread-safe (`src/db/cassandra.zig:12`) so it's shared unguarded; the
redis pool is guarded; lazy connect is behind a mutex (`ensureConnected`). Match that
rigor: if you add shared state, say in a comment _why_ it's safe.

3.10. **http.zig dispatch model: handlers run on a thread pool; your `App` is shared
concurrently.** Worker threads accept + parse; a separate `thread_pool` (whose size is
a `Config` knob — spot-check http.zig's current default against your version rather than
trusting a fixed number) runs your handler functions. The `*App` you pass to `Server(*App).init` is **one
instance shared across all concurrent requests** — karlseguin's docs say so
explicitly. Implication: everything reachable from `*App` (the four DB clients here)
must be thread-safe or pooled. Per-request mutable state belongs in `req.arena` /
`res.arena` or a `RequestContext`, never in fields on `App`.
Source: [http.zig readme — Handler / Per-Request Context / Configuration](https://github.com/karlseguin/http.zig);
repo `src/app.zig:68`.

---

## 4. comptime

4.1. **Reach for comptime when it removes runtime cost or duplication — generics,
dispatch tables, config baked at build time.** The `Backend` union here uses
`inline else => |repo| repo.health()` so one line generates the switch arm for every
backend with zero runtime tag cost on the common path (`src/app.zig:23`). Compile-time
string building (`"UPDATE users SET " ++ column ++ " = ? ..."` in
`cassandra.zig:153`, with `column: comptime []const u8`) yields a literal, not a
runtime concat.
Source: [langref — comptime](https://ziglang.org/documentation/0.16.0/#comptime).

4.2. **Don't comptime-golf. Prefer a plain runtime function when the metaprogramming
doesn't buy correctness or real speed.** Heavy comptime is hard to read, produces
worse error messages, and bloats compile times. If a runtime `if` chain (like
`App.resolve`) is clear and cheap, leave it runtime.

4.3. **`comptime` parameters are a contract: the arg must be known at compile time.**
`setColumn(comptime column: []const u8, ...)` guarantees the SQL is a literal —
which is also an **injection-safety** property here (§9). If you can't provide the
value at comptime, you can't call it; that's the point.

---

## 5. C interop (@cImport)

5.1. **Keep one `@cImport` per C library, near the code that uses it, and let Zig
translate the header.** `cassandra.zig` and `mongo.zig` each do a focused
`@cImport({ @cInclude("cassandra.h"); })`. Zig translate-c turns the header into a
Zig module (`c.cass_*`). Don't hand-transcribe C signatures. **Caveat: in-source
`@cImport` is deprecated in 0.16** (now backed by arocc, not libclang; issue
[ziglang/zig#20630](https://github.com/ziglang/zig/issues/20630)) — the repo still uses
it and it compiles, but for _new_ C modules prefer wiring `b.addTranslateC` in `build.zig`
and `@import("c")`.
Source: [langref — @cImport / Import from C](https://ziglang.org/documentation/0.16.0/#cImport);
[0.16 release notes — translate-c / @cImport deprecation](https://ziglang.org/download/0.16.0/release-notes.html).

5.2. **C returns error _codes_, not error unions — translate at the boundary.** Check
the code and convert to a Zig error immediately, so the rest of your code speaks Zig
errors: `if (c.cass_future_error_code(future) != c.CASS_OK) return error.CassandraQuery;`
(`cassandra.zig:77`). Never let a raw C status leak upward.

5.3. **When C owns the memory, respect its lifetime and free with its API — pair
`defer` with the matching C destructor.** The Cassandra code is a model:
`defer c.cass_statement_free(stmt)`, `defer c.cass_future_free(future)`,
`defer c.cass_result_free(result)`. And **copy C-owned bytes into your arena before
the C object is freed** — `getString` does `arena.dupe(u8, ptr[0..len])` because
`cass_value_get_string` points into the result set, which `cass_result_free`
invalidates (`cassandra.zig:198`). A slice into freed C memory is a use-after-free.

5.4. **Guard nullable C returns.** C `NULL` becomes an optional-ish `?*T` /
`[*c]` pointer; check it. `c.cass_result_first_row(result) orelse return null`
(`cassandra.zig:121`) turns "no row" into a clean absence.

5.5. **Linking C libs must survive both Homebrew (macOS dev) and Alpine (Docker) — probe
pkg-config, don't hardcode.** `build.zig` here picks `mongoc2` vs the legacy
`libmongoc-1.0`/`libbson-1.0` by testing `pkg-config --exists`, and only adds the
Homebrew include/lib paths for Cassandra when pkg-config _doesn't_ know it and the
target is macOS. Rationale: Alpine ships a `cassandra` .pc file; Homebrew doesn't and
puts headers under `/opt/homebrew`. Hardcoding either path breaks the other.
Source: repo `servers/zig/build.zig:26-44`.

5.6. **`link_libc` / `link_libcpp` go on the module.** Cassandra's cpp-driver needs the
C++ runtime even though it exposes a C API, so this build sets both
(`build.zig:16-18`). Missing `link_libcpp` yields link errors for C++ symbols.

---

## 6. std.json on 0.16

6.1. **Use `parseFromSliceLeaky` with an arena you own; use `parseFromSlice` when you
want a self-contained `Parsed(T)` to `deinit`.** `parseFromSlice` bundles its own
`ArenaAllocator` in `Parsed(T)` — value and arena are one unit; you must
`parsed.deinit()`. `parseFromSliceLeaky` takes _your_ arena and returns `T` directly,
no per-value free. This repo uses the leaky form with `res.arena`
(`user.zig:35`) — correct, because the request arena already owns the lifetime.
Sources: [openmymind — parseFromSlice / Parsed(T)](https://www.openmymind.net/Zigs-json-parseFromSlice/);
repo `src/user.zig`.

6.2. **Set `ParseOptions` deliberately — the defaults are strict.**
`duplicate_field_behavior` defaults to `.@"error"` (values `.use_first`, `.use_last`);
`ignore_unknown_fields` defaults to `false` (unknown key → `error.UnknownField`).
This repo intentionally sets `.duplicate_field_behavior = .use_last` (JS/Python
last-wins) and leaves `ignore_unknown_fields = false` so PascalCase/unknown keys are
_rejected_ — that strictness is the contract, not an accident (`user.zig:32`).
Source: [ParseOptions docs](https://ziglang.org/documentation/master/std/#std.json.ParseOptions);
repo `src/user.zig:29-32`.

6.3. **Mind string lifetimes: parsed strings may point _into_ the input buffer.** The
default `allocate = .alloc_if_needed` references the source JSON rather than copying.
If the parsed value outlives the input `[]u8`, that's a dangling pointer — pass
`.allocate = .alloc_always` to force duplication. In this repo it's a non-issue
because both the request body and the parse target live in the same request arena, but
the moment you parse into a longer-lived struct, force `alloc_always`.
Source: [openmymind — parseFromSlice string lifetime](https://www.openmymind.net/Zigs-json-parseFromSlice/).

6.4. **Make required fields have no default and optional fields default to `null`.**
Absent-required → parse error; absent-optional → `null`. `CreateUser` gives `name`
and `email` no default (so a body missing them fails), while `favoriteNumber` is
`?i32 = null` (`user.zig:14`). This is how you express "required vs optional" without
a separate validation pass for presence.

---

## 7. HTTP-server specifics under http.zig

7.1. **Allocate per-request in `res.arena` / `req.arena`; never on the global allocator
inside a handler.** These are fast thread-local buffers backed by an arena, reset
after each request — allocation-heavy but free-free. Every handler here does
`std.fmt.allocPrint(res.arena, ...)`. Anything you put in `res.body` must outlive the
handler return, so arena-allocate it (or use a string literal).
Source: [http.zig — Memory and Arenas](https://github.com/karlseguin/http.zig);
repo `src/routes_db.zig`.

7.2. **Return, don't hold: set `res.body` / `res.status` and return; http.zig flushes
after the handler.** Only call `res.write()` explicitly when you must release a
resource _after_ the bytes are on the wire (e.g. a refcounted cache entry).

7.3. **Configure body/form limits explicitly; enforce your real cap in the handler.**
This server sets `max_body_size` and `max_form_count`/`max_multiform_count` in
`Server.init` (`main.zig:45-51`). Note the deliberate design in the file-upload param
handler (`src/routes_params.zig:122`): it accepts slightly over the limit so the
_handler_ can return a clean `413` rather than http.zig dropping the connection (this
413-on-slight-overage behavior is the upload path, not general DB-body sizing).
**Current code caps at 2 MiB** (`2 * 1024 * 1024`); the
suite is unifying body caps across servers, so treat the exact number as in-flux and
check `config/` + PLAN before assuming a value.
Source: repo `src/main.zig:43-52`; [http.zig — Configuration](https://github.com/karlseguin/http.zig).

7.4. **Use the `res.writer()` → `.interface` bridge for streaming writers (0.15+ API).**
`res.writer()` returns a wrapper whose `.interface` field is the `*std.Io.Writer`;
pass an empty buffer (`&.{}`) unless a std API forces one, since http.zig buffers
itself. The only shared idiom with the redis client is the `.interface` field access —
note its writer is a _different_ shape: `redis.zig:104` passes a real 64-byte buffer
(`&wbuf`) to a raw `conn.stream.writer`, not `&.{}` to `res.writer()`.
Source: [http.zig — io.Writer / Memory and Arenas](https://github.com/karlseguin/http.zig).

7.5. **http.zig's multipart parser drops the per-part Content-Type; hand-parse if you
need it.** That's exactly why `src/multipart.zig` exists — it re-parses the raw body
to recover `filename` + declared `content_type`. If you write a parser like this,
the boundary handling (`--` prefix, closing `--`, the CRLF that precedes each
boundary) is fiddly; copy the tested shape rather than reinventing it.
Source: repo `src/multipart.zig`; [http.zig — Multi Part Form Data](https://github.com/karlseguin/http.zig).

7.6. **Graceful shutdown: signal handler flips a flag and calls `server.stop()`; tear
down after `listen()` returns.** `main.zig` installs SIGINT/SIGTERM handlers that
call `server.stop()`, which unblocks `listen()`; only _then_ does it deinit the DB
clients — by which point all workers have stopped and every pooled connection is idle
(safe to close). Keep signal handlers tiny (`callconv(.c)`, just set state / call
stop); do real work on the main thread.
Source: repo `src/main.zig:75-116`.

---

## 8. Testing

8.1. **Write tests inline with `test "name" {}` blocks; run with `zig build test` /
`zig test`.** Tests live next to the code they exercise.
Source: [langref — Zig Test](https://ziglang.org/documentation/0.16.0/#Zig-Test).

8.2. **Use `std.testing.allocator` — it fails the test on any leak.** This is the
cheapest, highest-value correctness check in Zig: it detects leaks, double-frees, and
use-after-free for anything allocated through it. Prefer it over `c_allocator` in
tests even though the server runs on `c_allocator`.
Source: [zig.guide — testing / allocator](https://zig.guide/standard-library/allocators/).

8.3. **Assert with `std.testing.expect*`.** `expect`, `expectEqual`, `expectError`,
`expectEqualStrings`, `expectEqualSlices`. Use `expectError` to pin down _which_
error a failing path returns (e.g. that a bad body yields `error.UnknownField`).

8.4. **Test handlers with `httpz.testing`.** It builds a `*Request`/`*Response` pair and
offers `expectStatus` / `expectJson` / `expectBody` — no socket needed. Ideal for the
route handlers here.
Source: [http.zig — Testing](https://github.com/karlseguin/http.zig).

8.5. **Fuzzing (`std.testing.fuzz`) exists but is early — treat as opportunistic.** The
signature is confirmed on 0.16: `fuzz(context: anytype, comptime testOne: fn(@TypeOf(context), []const u8) anyerror!void, options)` — a test calls
`std.testing.fuzz(context, oneInput, .{})` where `oneInput` takes the context plus an
input `[]const u8`; run under `zig build test --fuzz`. Great fit for the hand-parsers
here (multipart, RESP, URL parse).
Source: [langref — fuzzing](https://ziglang.org/documentation/0.16.0/#Fuzzing) (evolving).

---

## 9. Injection & framing safety (this repo cares)

9.1. **Redis: keep the RESP framing length-prefixed; never build commands by string
concatenation.** `sendCommand` writes `*<argc>\r\n` then, per arg, `$<len>\r\n<bytes>\r\n`
(`redis.zig:300`). Because every argument is length-delimited, a value containing
`\r\n`, spaces, or `*`/`$` is _data_, not protocol — this is the injection defense.
If anyone "simplifies" this into `"HSET " ++ key ++ " " ++ value`, they reintroduce a
command-injection hole. Do not.
Source: repo `src/db/redis.zig:300-308`; RESP2 spec.

9.2. **SQL/CQL: always bind parameters; never interpolate user data into the query
string.** pg.zig uses `$1..$n` (`postgres.zig`), Cassandra uses `?` placeholders with
`cass_statement_bind_*`. The only strings concatenated into CQL here are
`comptime`-known column names (§4.3), which cannot carry user data.

---

## 10. Common-mistake catalogue

10.1. **Slices vs arrays vs pointers.** An array `[N]T` has a comptime-known length and
is a value (copied on assign); a slice `[]T` is a `{ptr,len}` view into memory it
doesn't own; `*T` / `[*]T` are single/many pointers with no length. `&arr` gives
`*[N]T`; `arr[0..]` gives a slice. Returning a slice into a stack array that goes out
of scope is a dangling-pointer bug. Know which one you hold.
Source: [langref — Slices / Pointers](https://ziglang.org/documentation/0.16.0/#Slices).

10.2. **Pointers into an `ArrayList` are invalidated when it grows.** Appending can
reallocate the backing buffer, so any pointer/slice into `list.items` taken _before_
the append may dangle after. Take indices, or re-slice after you're done appending.
0.16 note: `ArrayList` is **unmanaged** — you pass the allocator to each mutating
call: `var list: std.ArrayList([]const u8) = .empty; try list.append(arena, x);`
exactly as `deleteAll` does (`redis.zig:260-266`). The old managed `ArrayList(T)` with
a stored allocator is gone.
Source: [langref / release notes — unmanaged containers](https://ziglang.org/download/0.16.0/release-notes.html);
repo `src/db/redis.zig`.

10.3. **Don't size a buffer to a protocol _hint_ and then truncate.** Redis `SCAN
   COUNT` is only a hint — a single batch can return more keys than any fixed array. The
correct pattern (and the one in this repo now) is to collect the whole batch into an
arena-backed `ArrayList` so no key is ever silently dropped:

```zig
var del_argv: std.ArrayList([]const u8) = .empty;
try del_argv.append(arena, "DEL");
while (i < key_count) : (i += 1)
    try del_argv.append(arena, (try readBulk(r, arena)) orelse return error.Protocol);
```

A prior version that read SCAN results into a `FixedBufferAllocator`/fixed array
truncated under large batches — the canonical "sized to a hint" bug. Unbounded input
→ unbounded (arena-backed) collection.
Source: repo `src/db/redis.zig:257-274`.

10.4. **FixedBufferAllocator over-run doesn't grow — it errors (or truncates if you
ignore the error).** If you use FBA for convenience, either the allocation returns
`error.OutOfMemory` (handle it) or your logic silently produces short output. For
anything whose size you don't strictly control, use an arena. (This is the root cause
class behind 10.3.)

10.5. **Integer overflow is _illegal behavior_ by default; opt into wrapping/saturating
explicitly.** `a + b` panics (safe builds) / is UB (ReleaseFast) on overflow. Use
`+%`/`-%`/`*%` for wraparound and `+|`/`-|`/`*|` for saturation when you _want_ that.
Don't reach for `+%` to "shut up the panic" — reaching for it should mean you
genuinely want modular arithmetic. Cast widths deliberately (`@intCast`, `@truncate`).
Source: [langref — Integer Overflow / Wrapping Operations](https://ziglang.org/documentation/0.16.0/#Integer-Overflow).

10.6. **Off-by-one with sentinels.** A `[:0]const u8` of "content length" N occupies
N+1 bytes; the `.len` is N (excludes the terminator) but the C side reads through the
`\0`. When you build a NUL-terminated buffer by hand (like `zterm`'s `[37]u8` for 36
chars), size for length+1 and set the terminator. Off-by-one here reads past the end
in C.
Source: repo `src/db/cassandra.zig:215`; [langref — sentinel pointers](https://ziglang.org/documentation/0.16.0/#Sentinel-Terminated-Pointers).

10.7. **Ignoring an error union.** Covered in §2.5 — reiterated because it's the single
most common Zig bug: `_ = fallibleThing();` compiles and hides failure. Be explicit.

10.8. **`orelse` / `.?` on optionals.** `.?` panics on null; `orelse` provides a
fallback or control flow. In a request handler, never `.?` on something a client
controls — use `orelse ""` / `orelse return` (the repo's `req.param("id") orelse ""`).

---

## In this repo (`servers/zig`)

- **Deps are commit-pinned into `zig-pkg/`** (hashed dirs like
  `httpz-0.0.0-PNVzr…`, `pg-0.0.0-Wp_7g…`). Treat them as vendored + immutable — bump
  via `build.zig.zon` + `zig fetch`, never by editing the hashed dirs. httpz's own
  readme calls the 0.16 build "experimental," so pin deliberately.
- **`main` is the new 0.16 entrypoint:** `pub fn main(init: std.process.Init) !void`,
  with `init.io` (the `Io`) and `std.heap.c_allocator` threaded into every subsystem
  (`src/main.zig:20-33`).
- **`App` is shared across all http.zig thread-pool workers** (`src/app.zig:68`);
  everything on it (4 DB clients) is pooled or documented thread-safe. Per-request state
  lives in `res.arena`. Don't add mutable fields to `App` without a guard.
- **`Backend` is a tagged union with `inline else` dispatch** (`src/app.zig:17`) — one
  set of handlers serves postgres/redis/mongo/cassandra. Idiomatic comptime that earns
  its keep.
- **Redis client = hand-rolled RESP2 + blocking pool** guarded by `Io.Mutex` +
  `Io.Condition`, `*Uncancelable` variants, `while`-loop wait (`src/db/redis.zig:22-93`).
  okredis doesn't compile on 0.16 stable, hence the hand-roll.
- **RESP framing is length-prefixed and MUST stay that way** — injection safety lives in
  `sendCommand` (`redis.zig:300`). String-concat "simplifications" are a security
  regression; reject them in review.
- **SCAN bulk-delete collects into an arena-backed `ArrayList`** (`redis.zig:260`) —
  the fix for the earlier fixed-buffer truncation bug. `COUNT` is a hint; never size to
  it.
- **Postgres via pg.zig native pool (size 50)**, `Io`-aware
  (`pg.Pool.initUri(io, ...)`, `src/db/postgres.zig:14`); slices from rows are
  `arena.dupe`'d before `row.deinit()` (`postgres.zig:72`) because they point into
  driver-owned memory.
- **Cassandra + Mongo via `@cImport`:** C-owned memory copied into the arena before the
  C object is freed (`cassandra.zig:198`); every C resource has a `defer *_free`; lazy
  `ensureConnected` behind a mutex so a not-yet-created keyspace resolves on a later poll
  instead of crashing (`cassandra.zig:49`).
- **`build.zig` handles Alpine-vs-Homebrew pkg-config divergence** — `mongoc2` vs
  `libmongoc-1.0`/`libbson-1.0`, and Homebrew include/lib paths for Cassandra only when
  pkg-config is silent on macOS (`build.zig:26-44`). Keep both paths working.
- **`link_libc` + `link_libcpp` on the module** (`build.zig:16-18`) — libc for the C
  allocator/mongoc, libcpp for the Cassandra cpp-driver runtime.
- **JSON decode is strict on purpose:** `parseFromSliceLeaky` into `res.arena`,
  `duplicate_field_behavior = .use_last`, unknown fields rejected (`user.zig:32`) — this
  encodes the cross-server contract; don't loosen it to make a payload pass.
- **Body cap is 2 MiB in code today** (`main.zig:48`), intentionally accepting
  slightly-over so the handler returns a clean 413; the suite is unifying caps, so
  confirm the target value in `config/` + PLAN before changing it.
- **Graceful shutdown deinits DB clients in reverse init order** after `listen()`
  returns (`main.zig:84-88`): cassandra → mongo → redis → postgres, the mirror of the
  init order in the `App` literal.

---

## Sources

- [Zig 0.16.0 Release Notes](https://ziglang.org/download/0.16.0/release-notes.html) — std.Io, threading renames, unmanaged containers, allocator changes, Writer/Reader.
- [Zig 0.16 langref](https://ziglang.org/documentation/0.16.0/) — errors, comptime, sentinels, slices/pointers, atomics, integer overflow, testing.
- [std docs (master/0.16)](https://ziglang.org/documentation/master/std/) — `std.json.ParseOptions` and friends.
- [Andrew Kelley — Zig's New Async I/O (text version)](https://andrewkelley.me/post/zig-new-async-io-text-version.html) — `io.async` vs `io.concurrent`, futures, cancellation.
- [Loris Cro — Zig's New Async I/O](https://kristoff.it/blog/zig-new-async-io/) — rationale for the Io interface / no function coloring.
- [Lukáš Lalinský — Async I/O in Zig 0.16, today](https://lalinsky.com/2026/05/11/async-io-in-zig-016-today.html) — practical threaded-vs-evented backend status.
- [karlseguin — http.zig README](https://github.com/karlseguin/http.zig) — dispatch model, arenas, config, testing, multipart, io.Writer bridge.
- [openmymind — Zig's json.parseFromSlice and Parsed(T)](https://www.openmymind.net/Zigs-json-parseFromSlice/) — leaky vs non-leaky, string lifetime.
- [zig.guide — allocators/testing](https://zig.guide/) — allocator selection, testing allocator.
- Repo primary source: `servers/zig/src/{main,app,user}.zig`, `src/db/{redis,postgres,cassandra}.zig`, `src/multipart.zig`, `build.zig`.
