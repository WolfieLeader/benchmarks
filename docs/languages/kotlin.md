# Kotlin best practices — kt-ktor & kt-spring-boot

For the implementer/reviewer agents building Phase 4's two Kotlin lanes
(`servers/kt-ktor`, `servers/kt-spring-boot`, shared module `shared/kotlin` →
Gradle `:shared`). Part 1 shipped the root Gradle skeleton, `shared/kotlin`, and
`kt-ktor`; `kt-spring-boot` joins later as a second `:kt-spring-boot` module
without restructuring. Versions below were verified live in early July 2026 —
**re-verify at implementation time**, especially anything marked UNVERIFIED.

Confirmed current versions: Kotlin **2.4.0** (pinned in the version catalog;
Gradle 9.6.1's _embedded_ Kotlin is 2.3.21, so the Kotlin Gradle plugin is pinned
forward to 2.4.0), Gradle **9.6.1**, Ktor **3.5.1**, Spring Boot **4.1.0** (Spring
Framework 7.0.8+, JDK 17–26 certified), kotlinx.coroutines (no official
virtual-thread dispatcher yet), detekt **1.23.8**, ktlint core **1.8.0** (no
first-party Gradle plugin — use `org.jlleitschuh.gradle.ktlint:14.2.0`).

---

## 1. Idioms

1. **Data classes are pure value holders; `copy()` is shallow.** A mutated
   nested object/List is shared between original and copy, not cloned. Only
   **primary-constructor** properties feed generated `equals`/`hashCode`/
   `toString`/`copy()` — body-declared properties are silently excluded (a real
   inheritance/equality bug source). `data class User(val id: UUID, val name: String, val email: String, val favoriteNumber: Int? = null)`
   — kotlinlang.org/docs/data-classes.html

2. **Model variants as `sealed class`/`sealed interface`, switch with `when`
   used as an _expression_.** Exhaustiveness is compiler-enforced only when the
   `when` result is assigned/returned — as a bare statement a missing branch
   compiles silently and no-ops. Always bind or return it so a new subtype
   breaks the build at every call site.

   ```kotlin
   fun DbError.toStatus(): Int = when (this) { is NotFound -> 404; is Conflict -> 409 }
   ```

   — kotlinlang.org/docs/sealed-classes.html, .../control-flow.html

3. **`!!` is a defect, not a null check.** It turns a recoverable null into an
   unhandled NPE with no diagnostic. Prefer `?.`/`?:`, or `requireNotNull(x) {
"why" }` / `checkNotNull(x) { "why" }` (`require*` → `IllegalArgumentException`
   for bad input, `check*` → `IllegalStateException` for bad state). —
   kotlinlang.org/docs/null-safety.html, .../exceptions.html

4. **Don't trust platform types (`T!`) at Java boundaries.** An unannotated
   Java return type suppresses Kotlin's null-checking, so you can dereference it
   and get an uncaught NPE with zero compiler warning. Mitigate with JSR-305
   annotations on the library, or Kotlin's stable, direct **JSpecify** support
   (`org.jspecify.annotations`: `@Nullable`/`@NonNull`/`@NullMarked`, default
   report level `strict`, tunable via `-Xjspecify-annotations=<strict|warn|ignore>`).
   For unannotated legacy Java (some Mongo/Cassandra driver surfaces), assert
   nullability once in a thin adapter at the boundary, not ad hoc at every call
   site. — kotlinlang.org/docs/java-interop.html

5. **Extension functions extend types you don't own — they're not a substitute
   for a small class.** Check stdlib first (`map`/`filter`/`fold`/`groupBy`/
   `filterNotNull`). Caveat: resolution is **static** on the receiver's
   _declared_ type, not runtime type — extensions aren't polymorphic like member
   functions, so relying on one where behavior should vary by subtype is a
   correctness bug, not a style nit. Member functions always win over
   same-signature extensions. — kotlinlang.org/docs/extensions.html

6. **Scope functions — pick by table**, don't default to the familiar one; the
   docs explicitly warn nesting/overusing them hurts readability.

   | Fn          | Ref    | Returns       | Use for                                      |
   | ----------- | ------ | ------------- | -------------------------------------------- |
   | `let`       | `it`   | lambda result | null-check chains, local scoping             |
   | `run`       | `this` | lambda result | configure-then-compute in one block          |
   | `with(x){}` | `this` | lambda result | grouping calls on an object you already have |
   | `apply`     | `this` | the object    | builder-style configuration                  |
   | `also`      | `it`   | the object    | side effects mid-chain (logging)             |

   — kotlinlang.org/docs/scope-functions.html

7. **Default `val` + read-only collection types; `var`/`Mutable*` only where
   mutation is the point.** Kotlin's coding conventions: prefer `val` for
   anything not reassigned, and immutable interfaces (`List`/`Set`/`Map`) for
   anything not mutated in place. Read-only ≠ deep-immutable: a `List`
   reference into a live `MutableList` still reflects mutation through the
   other reference. `kotlinx.collections.immutable` (v0.5.1) is maintained but
   still "experimental" per its own README — don't adopt it here; stdlib
   read-only types suffice. — kotlinlang.org/docs/coding-conventions.html, .../constructing-collections.html

---

## 2. Concurrency — coroutines (deep) + virtual threads

**kt-ktor is coroutine-native end-to-end; kt-spring-boot is blocking MVC +
virtual threads.** Both are correct, current, idiomatic choices for their
framework — don't blur them (item 20).

### Structured concurrency

8. **Every coroutine must be owned by a scope tied to a real lifecycle**
   (request/connection/app). A child's `Job` nests under its launcher's `Job`;
   cancelling the scope recursively cancels every descendant. Never launch
   request-handling work from `GlobalScope` — it has no parent, nothing
   auto-cancels it on disconnect/shutdown, and it's `@DelicateCoroutinesApi`
   precisely because it routinely leaks. — kotlinlang.org/docs/coroutine-context-and-dispatchers.html

9. **`coroutineScope` (all-or-nothing) vs `supervisorScope` (isolated
   failures):** `coroutineScope{}` — one child failing cancels every sibling
   and rethrows; use when a partial result is meaningless (e.g. a step needing
   both Postgres and Redis). `supervisorScope{}` — a child failing does not
   cancel siblings; use for fan-out to independent downstream calls (e.g.
   parallel calls across independent `/db/*` backends).

   ```kotlin
   suspend fun readBoth(id: UUID) = coroutineScope {
       val pg = async { pgRepo.find(id) }; val cache = async { redis.get(id.toString()) }
       pg.await() to cache.await()
   }
   ```

   — kotlinlang.org/api/.../coroutine-scope.html, .../supervisor-scope.html

10. **Cancellation is cooperative, not preemptive** — noticed only at
    suspension points or explicit `isActive`/`ensureActive()` checks; a
    tight non-suspending loop in a cancelled coroutine runs to completion
    regardless. Suspending cleanup in `finally` must be wrapped
    `withContext(NonCancellable) { ... }`, or it throws `CancellationException`
    immediately instead of running. — kotlinlang.org/docs/cancellation-and-timeouts.html

11. **Dispatchers: `Default` for CPU-bound, `IO` for blocking calls,
    `.limitedParallelism(n)` to ring-fence a resource.** `Default`/`IO` share
    the same underlying pool machinery; `IO`'s elastic pool defaults to
    `max(64, cores)`, tunable via the `kotlinx.coroutines.io.parallelism`
    system property. **No official virtual-thread dispatcher exists yet**
    (tracking issue `kotlinx.coroutines#3606`, open since Jan 2023) — the only
    pattern is hand-rolled `Executors.newVirtualThreadPerTaskExecutor().asCoroutineDispatcher()`,
    treat as an experimental adapter (per PLAN §3's "injectable adapter, not a
    fork"), not a library feature. kt-ktor shouldn't need this at all —
    coroutines + `Dispatchers.IO` already scale without virtual threads. —
    kotlinlang.org/api/.../-dispatchers/-i-o.html, github.com/Kotlin/kotlinx.coroutines/issues/3606

12. **Reach for `Flow` only for streamed/multiple values over time** — a
    single async result stays a plain `suspend fun`. `Flow` is cold: the
    `flow{}` block runs once per `collect`, independently per collector. No
    streaming route exists in this repo today; model one as `Flow<T>` if added,
    not a pre-built `List<T>`. — kotlinlang.org/docs/coroutines-flow.html

13. **Never call `runBlocking` inside a coroutine or a Ktor handler** — the
    handler is already running on a coroutine; `runBlocking` there blocks the
    thread for nothing and can starve the dispatcher under load (docs call
    this "redundant... potentially leading to thread starvation"). Legitimate
    uses: `main()`, bridging sync tests/non-suspend callbacks. —
    kotlinlang.org/api/.../run-blocking.html

14. **Context elements combine/inherit via `+`; raw `ThreadLocal` does not
    survive a suspension's thread switch.** A child inherits its parent's
    full context and can override individual elements. For per-request
    ThreadLocal-style context, use `threadLocal.asContextElement(value)` — it
    restores around each resumption but doesn't track in-coroutine mutation;
    reissue via a fresh `withContext` to change it mid-flight. —
    kotlinlang.org/docs/coroutine-context-and-dispatchers.html, .../as-context-element.html

15. **Exception propagation, exact rule:** an uncaught non-`CancellationException`
    in a child cancels its parent — structural, not overridable by a handler on
    an intermediate coroutine. `CoroutineExceptionHandler` **only fires on a
    root coroutine** (no parent job); a handler on a regular child is a no-op
    since the exception already propagated up by then. Direct children of a
    `supervisorScope`/`SupervisorJob` act like roots — attach the handler to
    each `launch(handler){}` call, not the `supervisorScope{}` block. A
    `try/catch` directly around a child's suspending call prevents the
    exception reaching the parent at all — the usual way to isolate one
    fan-out branch's failure without restructuring to `supervisorScope`. —
    github.com/Kotlin/kotlinx.coroutines/blob/master/docs/topics/exception-handling.md

### Virtual threads (Loom) — the Spring MVC lane

16. **Virtual threads let blocking code scale without a reactive rewrite.**
    Cheap JDK-managed threads mount onto a small fixed carrier-thread pool and
    unmount whenever they block on I/O — a blocking JDBC call no longer pins a
    scarce OS thread. This is why the plan calls MVC + virtual threads "the
    current idiomatic-modern setup," not WebFlux. — openjdk.org/jeps/444

17. **Pinning: `synchronized` mostly fixed, native calls not.** JDK 21–23:
    `synchronized` pinned the virtual thread to its carrier for the block's
    duration. **JEP 491 fixed this in JDK 24** — Spring now "strongly
    recommends Java 24+" with virtual threads. Native/JNI/FFI calls **still
    pin** regardless. This repo pins JDK 21 (below the 24+ recommendation) —
    fine functionally; if pinning shows up in profiling (e.g. HikariCP's
    internal locking, still partly `synchronized` in some versions — its
    maintainers deferred to JEP 491 rather than rewriting, see item 26), the
    fix is bumping the toolchain to 24+, not avoiding `synchronized`. —
    openjdk.org/jeps/491, openjdk.org/jeps/444

18. **`Executors.newVirtualThreadPerTaskExecutor()`** — unbounded per-task
    virtual threads, no pooling; `close()` awaits everything submitted. Spring
    Boot wires this in for you via one property (item 31) — don't hand-roll an
    `Executor` unless deviating from that switch on purpose. —
    docs.oracle.com/.../Executors.html

19. **Prefer `ScopedValue` over `ThreadLocal` for per-request context, if/when
    the toolchain reaches JDK 25+** (`ScopedValue` finalized as JEP 506; JEP 444
    itself flags `ThreadLocal` as costly at virtual-thread scale). Not urgent
    at JDK 21; flag only, don't build for it speculatively. — openjdk.org/jeps/506

20. **Coroutines vs virtual threads is per-framework, follow the plan exactly.**
    Ktor's own stack (routing, I/O, plugins) is `suspend`-based — no reason to
    introduce virtual threads there. Spring MVC is historically blocking;
    virtual threads keep that model while removing the platform-thread-count
    ceiling. **Correction worth flagging**: Spring MVC controllers _can_
    technically declare `suspend fun`/`Deferred`/`Flow` (native Kotlin
    coroutine support, gated only on `kotlinx-coroutines-reactor` on the
    classpath) — "MVC needs WebFlux for coroutines" is not literally true.
    Still not idiomatic here: mixing `suspend` controllers into the lane whose
    whole point is exercising blocking+virtual-thread MVC adds a second
    concurrency model for no benefit, and MVC's execution is still
    thread-per-request regardless of handler signature — the non-blocking win
    only appears if the _entire_ chain including the DB client is reactive,
    which is out of scope (JDBC not R2DBC, item 34). Keep kt-spring-boot's
    controllers plain blocking functions. — docs.spring.io/spring-framework/reference/languages/kotlin/coroutines.html,
    docs.spring.io/spring-boot/reference/features/task-execution-and-scheduling.html

---

## 3. Ktor specifics (verified: 3.5.1)

21. **Engine: Netty.** CIO only exposes `connectionIdleTimeoutSeconds` — no
    worker-pool controls. Netty exposes `connectionGroupSize`/
    `workerGroupSize`/`callGroupSize`, needed to pin thread counts explicitly
    for fairness against this repo's other single-process servers. (Docs make
    no explicit "Netty is production-recommended" claim — cite the thread-pool
    control gap, not that framing.) — ktor.io/docs/server-engines.html

22. **Use `EngineMain`, not `embeddedServer`, for free config + shutdown.**
    Reads `application.yaml`/`.conf` + env overrides (matches this repo's
    uniform env-var contract) and — per Ktor's FAQ — handles SIGINT/SIGTERM
    automatically; `embeddedServer` needs a manual shutdown hook. Order DB
    teardown after drain: subscribe to `ApplicationStopped` and close
    postgres/mongo/redis/cassandra clients there, so pools outlive in-flight
    requests. — ktor.io/docs/server-create-and-configure.html, .../faq.html

23. **Organize routes as per-resource `Route` extension functions**, called
    once from one `routing{}` block (community-standard, unchanged since Ktor
    2). Plugin install order (CallLogging → StatusPages → ContentNegotiation)
    is convention, not documented anywhere — verify empirically once a
    skeleton exists. — ktor.io/docs/server-routing-organization.html

24. **ContentNegotiation: `kotlinx.serialization`, not Jackson** — JetBrains'
    own library, the idiomatic pure-Kotlin choice (Jackson exists mainly for
    Java/Spring interop). `install(ContentNegotiation) { json() }`. Version:
    Ktor 3.5.1 pins **kotlinx.serialization 1.11.0** (confirmed in Ktor's own
    `gradle/libs.versions.toml` at tag 3.5.1). Its JSON decoder already yields
    **last-wins on duplicate keys by default** (no `ignoreDuplicateKeys` or other
    config needed), which matches the contract's duplicate-key rule — so don't add
    config for it. — ktor.io/docs/server-create-restful-apis.html

25. **Body limits: `RequestBodyLimit`, installed twice** — global 10MiB, then
    a 1MiB override scoped to `/params/file`. Confirmed via source read: it
    checks `Content-Length` up front (immediate reject, no read) _and_ wraps
    the streaming channel to cut off chunked/lying requests mid-read — real
    pre-read 413, not buffer-then-check. **No built-in content-type sniffing
    exists** — the file route's 415 anti-sniffing rule must be hand-rolled:
    after the size plugin bounds the read, inspect actual bytes (reject on any
    non-printable/control byte outside valid UTF-8 text).

    ```kotlin
    install(RequestBodyLimit) { bodyLimit { 10 * 1024 * 1024 } }
    route("/params/file") { install(RequestBodyLimit) { bodyLimit { 1 * 1024 * 1024 } }; post { /* then byte-sniff */ } }
    ```

    — github.com/ktorio/ktor `ktor-server-body-limit/.../RequestBodyLimit.kt` (read directly)

26. **StatusPages gotcha: `status{}` silently overwrites an explicit
    `call.respond()` for the same code.** Confirmed via source read — no
    committed/sent check before firing a matching `status(code){}` handler.
    Pick one lane per status code: throw typed exceptions and let
    `exception<T>{}` build every error body, or build JSON at the call site
    and reserve `status{}` only for codes never `call.respond()`'d explicitly
    (engine-generated 404/405). For the `/params/body` contract cases (malformed /
    non-object / array / string / number / bool / null top-level body → `400` with
    `{"error":"invalid JSON body","details":...}`), catch kotlinx.serialization's
    `SerializationException`/`MissingFieldException` in an `exception<…>{}` handler
    and emit exactly that shape — the plugin's default error body won't match the
    contract. — github.com/ktorio/ktor `ktor-server-status-pages/.../StatusPages.kt` (read directly)

---

## 4. Spring Boot specifics (verified: 4.1.0, Spring Framework 7.0.8+)

27. **Target 4.1.0; pin the toolchain at JDK 21** (inside certified 17–26,
    below the "24+ recommended" virtual-thread guidance — revisit only if
    profiling shows pinning, item 17). Don't target 3.5.x — its OSS support
    ended 2026-06-30.

    ```kotlin
    plugins { id("org.springframework.boot") version "4.1.0" }
    kotlin { jvmToolchain(21) }
    ```

    — docs.spring.io/spring-boot/system-requirements.html

28. **Constructor injection only** — no field/`@Autowired`, no `lateinit` for
    dependencies. A single-constructor class gets implicit injection (Spring
    4.3+); combined with primary-constructor `val`s this gives immutable,
    trivially-mockable dependencies with zero DI annotations in the common
    case. `@Service class UserService(private val repo: UserRepository, private val clock: Clock)`
    — docs.spring.io/.../autowired.html

29. **Typed config via `@ConfigurationProperties` on a data class** with
    constructor defaults, not scattered `@Value` — constructor binding is
    auto-detected from a single parameterized constructor since Boot 3.0, no
    `@ConstructorBinding` needed. `@ConfigurationProperties("app.pool") data class PoolProps(val size: Int = 50, val timeoutMs: Long = 2000)`
    — github.com/spring-projects/spring-boot/wiki/Spring-Boot-3.0-Migration-Guide

30. **Error shape: `@RestControllerAdvice` + `@ExceptionHandler`, not
    `ProblemDetail`.** `ProblemDetail` (RFC 7807/9457) is opt-in via
    `spring.mvc.problemdetails.enabled` (off by default) and produces
    `{type,title,status,detail,instance}` — not this repo's flat
    `{"error","details"?}`. A custom `ResponseEntity<ErrorBody>` from an
    `@ExceptionHandler` bypasses `BasicErrorController`/`ProblemDetail`
    entirely.

    ```kotlin
    data class ErrorBody(val error: String, val details: String? = null)
    @RestControllerAdvice class ApiExceptionHandler {
        @ExceptionHandler(NotFoundException::class)
        fun notFound(e: NotFoundException) = ResponseEntity.status(404).body(ErrorBody("not_found", e.message))
    }
    ```

    — docs.spring.io/.../mvc-ann-rest-exceptions.html

31. **Virtual threads: `spring.threads.virtual.enabled=true`.** Switches
    Tomcat's request executor to virtual threads and `@Async`/`@Scheduled` to
    `SimpleAsyncTaskExecutor`/`Scheduler` — does **not** touch Hikari pool
    sizing (independent, item 33). The customizer is
    `org.springframework.boot.tomcat.autoconfigure.TomcatVirtualThreadsWebServerFactoryCustomizer`
    (confirmed in spring-boot source). —
    docs.spring.io/spring-boot/reference/features/task-execution-and-scheduling.html

32. **Drop actuator entirely for benchmark fairness.** It auto-registers
    health indicators, metrics collection/export, and background
    instrumentation with no documented way to strip only that cost — exclude
    `spring-boot-starter-actuator`; no other roster framework carries an
    equivalent always-on tax. — docs.spring.io/spring-boot/reference/actuator/endpoints.html

33. **HikariCP fixed at exactly 50 — this repo's canon, not Hikari's own
    formula.** Hikari's wiki formula (`(core_count*2) + spindle_count`) sizes
    off the DB server's capacity; nothing in the wiki ties pool size to
    virtual threads — they raise app-side concurrency in flight, not what the
    DB can usefully serve. Don't "correct" the pool upward because virtual
    threads make more concurrent requests cheap.
    `spring.datasource.hikari.maximum-pool-size=50` —
    github.com/brettwooldridge/HikariCP/wiki/About-Pool-Sizing

34. **JDBC + HikariCP, not R2DBC, pairs correctly with MVC + virtual
    threads.** R2DBC's value is non-blocking I/O for thread-starved reactive
    stacks (WebFlux); under virtual threads a blocking JDBC call just parks
    the thread cheaply, so R2DBC buys nothing while adding a second, less
    mature stack. Postgres: `org.postgresql:postgresql` (pgjdbc), unchanged. —
    spring.io/blog/2022/10/11/embracing-virtual-threads/

---

## 5. DB clients

35. **Postgres**: `org.postgresql:postgresql` + HikariCP — Spring Boot still
    prefers Hikari whenever it's on the classpath.

36. **MongoDB**: `org.mongodb:mongodb-driver-kotlin-coroutine` — the official
    coroutine-native driver (`find()` returns `Flow`, `insertOne` is `suspend
fun`, not a wrapped reactive stream). **KMongo is a dead end** — its own
    README says don't use it for new projects; MongoDB ships an official
    KMongo-migration guide toward this driver. `suspend fun findByTitle(t: String) = collection.find(eq("title", t)).firstOrNull()`
    — mongodb.com/docs/drivers/kotlin/coroutine/current/, github.com/Litote/kmongo

37. **Redis**: **Lettuce**, not Jedis, for both servers. Spring Data Redis
    already defaults to Lettuce; for kt-ktor, Lettuce's async API
    (`conn.async().get(k).await()` via `kotlinx-coroutines-jdk8`, or its
    experimental native `.coroutines()` suspend surface) fits a coroutine
    server far better than Jedis, which is single-threaded-per-connection and
    needs `JedisPool`/`JedisPooled` for any concurrency. —
    docs.spring.io/spring-data/redis/reference/redis/drivers.html, redis.github.io/lettuce/user-guide/kotlin-api/

38. **Cassandra: driver moved to the ASF.** Use
    `org.apache.cassandra:java-driver-core` (`github.com/apache/cassandra-java-driver`,
    actively pushed), not the frozen `com.datastax.oss:java-driver-core`
    (stuck at 4.17.0). Package namespace unchanged
    (`com.datastax.oss.driver.api.core.*`) — mostly a group-ID swap. No
    maintained Kotlin coroutine wrapper — idiomatic path is
    `session.executeAsync(...)` (`CompletionStage`) bridged with `.await()`. —
    verified via the `pom.xml` groupId on the 4.19.3 tag

39. **UUIDv7**: `java.util.UUID` cannot generate v7 — this repo's canon
    requires it for every non-Mongo `User.id`. Use
    **`com.fasterxml.uuid:java-uuid-generator`** (JUG), not
    `com.github.f4b6a3:uuid-creator` — both are correct/RFC-9562-compliant
    one-liners, but uuid-creator has been dormant over a year while JUG has
    commits within days of this research; maintenance recency decided it, not
    ergonomics. `val id: UUID = Generators.timeBasedEpochGenerator().generate()`
    — github.com/cowtowncoder/java-uuid-generator, github.com/f4b6a3/uuid-creator (release history)

---

## 6. Gradle

40. **Use the version catalog** (`gradle/libs.versions.toml`, auto-imported,
    `alias(libs.plugins.x)` / `libs.x.y`) as the single place pinning Kotlin,
    Ktor, Spring Boot, kotlinx.coroutines, detekt/ktlint versions once across
    `:shared`/`:ktor`/`:spring-boot`. — docs.gradle.org/current/userguide/version_catalogs.html

41. **Pin JDK via `kotlin { jvmToolchain(21) }`, not a bare `java { toolchain
{ } }`** — Kotlin's docs note this single declaration also configures Java
    compile tasks. Add `org.gradle.toolchains.foojay-resolver-convention` in
    `settings.gradle.kts` for auto-provisioning. This directly answers the
    repo's quirk: **Gradle's launcher JVM (26) running newer than the pinned
    toolchain (21) is explicitly documented, supported behavior**, not a
    workaround. — kotlinlang.org/docs/gradle-configure-project.html

42. **Share config via a precompiled convention plugin in an included build
    (`build-logic`), not `buildSrc`** — any `buildSrc` change invalidates the
    whole build's configuration phase; an included build doesn't. Apply per
    subproject: `plugins { id("kotlin-common-conventions") }`. Community idiom
    on documented mechanics, not doc-verbatim — verify the wiring once the
    module tree exists. — docs.gradle.org (sharing build logic)

43. **Configuration cache is opt-in in Gradle 9.x, not default yet** —
    `org.gradle.configuration-cache=true` in `gradle.properties` (default-on
    targeted for Gradle 10). Watch: `Task.getProject()` at execution time is
    deprecated, hard error in Gradle 10 — a risk for any hand-rolled Gradle
    glue touching `project` at execution time. — docs.gradle.org/current/userguide/configuration_cache.html

44. **ktlint + detekt, both merge-gating via `just verify`.** ktlint has no
    first-party Gradle plugin — use `org.jlleitschuh.gradle.ktlint:14.2.0`;
    rules configure via `.editorconfig` (`ktlint_*` keys — ktlint migrated off
    its own config file). The ktlint-gradle plugin **auto-attaches `ktlintCheck`
    to `check`** (confirmed: its `TaskCreation` wires `check` to `dependsOn` the
    generated-reports task whenever the base `check` task exists) — no manual
    `dependsOn` wiring needed. detekt (`io.gitlab.arturbosch.detekt`, stable
    1.23.8) **also** doc-confirms auto-wiring into `check`; stay off the
    `dev.detekt` 2.0 alpha. `detekt { config.setFrom("$rootDir/config/detekt/detekt.yml"); buildUponDefaultConfig = true }`
    — github.com/pinterest/ktlint, github.com/jlleitschuh/ktlint-gradle, github.com/detekt/detekt

---

## 7. Common mistakes

45. **Companion-object overuse as a poor-man's static/singleton bag** — can't
    be swapped like a constructor-injected dependency (MockK needs
    `mockkObject` specifically because this is common — treat that as a
    smell signal). Prefer top-level functions/constants or DI.

46. **`GlobalScope` for anything request-scoped** — see item 8; repeated here
    as the single most common structured-concurrency violation.

47. **`lateinit` as a substitute for constructor injection** — reserved for
    genuine framework-mandated late init, not "didn't want to write a
    constructor param." Every dependency here should be a constructor `val`
    (item 28).

48. **Exceptions as flow control for expected outcomes** (e.g. throwing for
    routine "not found" instead of a sealed result/nullable) — exceptions
    capture a stack trace on construction, real cost on a hot path. Reserve
    `throw`/`@ExceptionHandler` for genuine 4xx/5xx mapping (item 30), not
    internal branching.

49. **Ignoring detekt complexity signals** (`LongMethod`, `ComplexCondition`,
    `NestedBlockDepth`) instead of refactoring, or suppressing narrowly with
    `@Suppress("RuleId")` + a comment when a rule is genuinely wrong for one
    case. Per this repo's Never-list: no rule-wide disables to get green.

50. **Java-style explicit getters/setters** instead of Kotlin properties
    (`val`/`var` already generate JVM accessors) — writing them out is dead
    code and a signal the file was transliterated from Java.

---

## 8. Testing

51. **JUnit5 as the runner for both servers; add `kotest-assertions-core`
    for `shouldBe` only — skip full Kotest specs.** Kotest is actively
    maintained (releases within the last month), and its
    assertions/property-testing modules are explicitly documented as usable
    standalone under plain JUnit. Neither Spring Boot's nor Ktor's testing
    docs mention Kotest specs (`spring-boot-starter-test` bundles JUnit
    Jupiter + AssertJ; Ktor's own docs use plain JUnit + `testApplication{}`)
    — running two discovery mechanisms buys nothing here. —
    kotest.io/docs/framework/testing-styles.html, docs.spring.io/spring-boot/reference/testing/index.html, ktor.io/docs/server-testing.html

52. **MockK, not Mockito, for both servers.** Mocks Kotlin's default-final
    classes with zero config, has native `coEvery`/`coVerify` for suspend
    functions, and `mockkObject(...)` mocks `object`/companions (no Mockito
    equivalent).

    ```kotlin
    val repo = mockk<UserRepository>(); coEvery { repo.findById(id) } returns user
    coVerify { repo.findById(id) }
    ```

    — mockk.io

53. **`@SpringBootTest` is the expensive, last-resort option.** Prefer slices
    first — `@WebMvcTest` (controller layer, mocked service deps),
    `@JdbcTest`/`@DataJpaTest` (persistence) — full `@SpringBootTest` loads the
    entire context, reserve for genuine end-to-end (`webEnvironment =
RANDOM_PORT`). Spring reuses a cached context only when the full cache key
    matches exactly (profiles, mocked beans, properties) — converge test
    classes on identical setup to benefit. — docs.spring.io/spring-boot/reference/testing/spring-boot-applications.html,
    .../testcontext-framework/ctx-management/caching.html

54. **Testcontainers: plain Java API from Kotlin, no dedicated module
    needed** — `org.testcontainers:postgresql|mongodb|cassandra` are official
    core modules (3 of this repo's 4 DBs). Redis is **not** core — it's
    community `com.redis:testcontainers-redis`, pin separately. For
    containers shared across test classes (to cooperate with Spring's
    context-cache reuse, item 53), use the documented **singleton-container
    pattern** (start in a static initializer on a shared abstract base class)
    rather than `@Testcontainers`/`@Container`, which tears down at end of
    class. — java.testcontainers.org/modules/databases/, testcontainers.com/guides/testcontainers-container-lifecycle/, testcontainers.com/modules/redis/

---

## 9. The `web` suite (Phase 3 contract → Phase 4 build) — library choices

Phase 3 adds a `web` suite to the contract — `GET /html` (server-rendered
template), `GET /jwt/sign` + `GET /jwt/verify` (HS256), `POST /validate` (deep
nested validation, ~4 levels), `GET /compute?n=` (iterative SHA-256 CPU chain)
(`PLAN.md:223-233`) — and the server-track DAG lands P3 (web endpoints) _before_
P4 (new servers) (`PLAN.md:545,562-563`), so both Kotlin lanes must satisfy these
from day one. CLAUDE.md's JWT/UUID library table names Go/TS/Python but not
Kotlin; the picks below fill that gap.

55. **JWT (HS256): a maintained JOSE library, not hand-rolled HMAC.** Prefer
    `com.nimbusds:nimbus-jose-jwt` (full JOSE/JWK surface, actively maintained)
    or, if only classic JWT is needed, `com.auth0:java-jwt`. Spring Security's
    OAuth2 resource-server JWT support already wraps Nimbus, so Nimbus keeps
    kt-spring-boot consistent with Spring's own stack. Match the HS256 algorithm
    and the exact claim set the contract asserts — don't invent a header/claim
    shape. — connect2id.com/products/nimbus-jose-jwt
56. **HTML templating: framework-native, not a third engine.** kt-ktor → the
    FreeMarker or Thymeleaf Ktor plugin (`io.ktor:ktor-server-freemarker` /
    `ktor-server-thymeleaf`, `install(FreeMarker|Thymeleaf)` then
    `call.respondTemplate(...)`). kt-spring-boot → Spring Boot's first-party
    Thymeleaf starter (`spring-boot-starter-thymeleaf`, auto-configured). Pick
    one engine per lane and keep the rendered output matching the contract's
    expected HTML exactly. —
    ktor.io/docs/server-freemarker.html, docs.spring.io/spring-boot/reference/web/servlet.html
57. **Validation (`POST /validate`, deep nested): Spring has
    `jakarta.validation`; Ktor has no official deep-validation library.**
    kt-spring-boot → `spring-boot-starter-validation` (Hibernate Validator) with
    `@Valid` + JSR-380 constraint annotations on nested data classes, mapped to
    the repo error shape via `@ExceptionHandler` (item 30). kt-ktor → there is
    **no first-party nested-validation plugin**; either hand-write the nested
    checks (throwing typed exceptions that `StatusPages` maps, item 26) or use
    Ktor's basic `RequestValidation` plugin for shallow cases and hand-roll the
    nested ones. Don't drag a Spring-only annotation model into the Ktor lane. —
    jakarta.ee/specifications/bean-validation/, ktor.io/docs/server-request-validation.html
58. **`GET /compute` (CPU chain): dispatch onto `Dispatchers.Default`, don't
    block the request thread.** In kt-ktor run the CPU work inside
    `withContext(Dispatchers.Default)` (item 11) so it doesn't hog an I/O
    dispatcher thread; in kt-spring-boot the blocking compute runs on its
    (virtual) request thread, which parks cheaply — no offload needed, but keep
    the algorithm byte-for-byte identical across both lanes for fairness. —
    item 11.

---

## In this repo

- **16-route contract + `{"error", "details"?}` error shape**: kt-ktor via
  `StatusPages` (item 26, mind the overwrite gotcha), kt-spring-boot via
  `@RestControllerAdvice` (item 30, not `ProblemDetail`).
- **UUIDv7** for `User.id` on postgres/redis/cassandra (Mongo keeps `ObjectId`
  per PLAN §1.1) — `com.fasterxml.uuid:java-uuid-generator` (item 39), never
  `java.util.UUID` (no v7) or `uuid-creator` (dormant).
- **Single-process + pool-50 fairness**: no extra Tomcat/Netty worker
  processes; `HikariCP maximum-pool-size=50` fixed regardless of virtual
  threads (item 33); size Ktor's Netty thread pools explicitly (item 21)
  rather than leaving engine defaults.
- **10MiB global body cap + 1MiB file-route 413/415**: Ktor via two
  `RequestBodyLimit` installs plus a hand-rolled byte-sniff for 415 (item 25 —
  no built-in sniffing exists). Spring Boot has **no built-in property that caps
  an arbitrary JSON POST body**: `server.tomcat.max-http-form-post-size` only
  bounds `application/x-www-form-urlencoded` bodies, and `max-swallow-size` is
  not a size cap at all (it only limits how many bytes Tomcat drains from an
  _already-rejected_ upload). kt-spring-boot must therefore **hand-roll a
  `Filter`** (e.g. `ContentCachingRequestWrapper` / a `Content-Length` +
  streaming check) to enforce the 10MiB/1MiB caps, mirroring Ktor's approach —
  plus the same hand-rolled content-type byte-check for the 415.
- **Graceful drain then DB teardown order**: Ktor — `ApplicationStopped` after
  `EngineMain`'s automatic SIGINT/SIGTERM (item 22). Spring Boot — Tomcat's
  graceful shutdown (`server.shutdown=graceful`) completes in-flight requests
  before `@PreDestroy`/`DisposableBean` hooks close Hikari/Mongo/Redis/
  Cassandra. This ordering is **confirmed**: Spring's `SmartLifecycle` runs the
  graceful web-server stop in the earliest shutdown phase, ahead of other beans'
  `@PreDestroy`/`DisposableBean` callbacks — so the pools outlive the drain by design.
- **Non-root multi-stage Dockerfile from repo root**: mirror
  `servers/go-chi/Dockerfile`'s pattern (builder stage compiles, runtime stage
  adds a dedicated non-root `app-runner`/`app-group` before `USER
app-runner`). For Kotlin: builder runs `./gradlew :kt-ktor:installDist` (or
  `:spring-boot:bootJar`) on a JDK 21 base image; runtime stage copies only the
  built distribution/jar onto a slim JRE 21 base, non-root user, `EXPOSE`
  matching the assigned port (kt-ktor **25001**, kt-spring-boot **25002** per
  PLAN's port table).
- **Gradle multi-project layout**: `:shared` (DB clients/repositories, `User`
  data class, UUIDv7 gen, env parsing — framework-agnostic per PLAN §3),
  `:ktor`, `:spring-boot` as subprojects in one root `settings.gradle.kts` —
  the one Kotlin-specific contention point in PLAN §11.1: two lanes each
  appending a subproject line conflict trivially — serialize that one-line
  edit, don't serialize the whole lanes over it.
- **ktlint/detekt merge-gating**: wired into all three subprojects via the
  shared convention plugin (item 42), both must pass under `just verify` for
  every Kotlin PR like every other language here — no rule-wide disables to
  get green (repo Never-list), detekt config at one repo-root path per PLAN's
  "one config per language" rule.
