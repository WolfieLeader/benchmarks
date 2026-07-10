# Multi-stage, root-context build (PLAN §2.4): the context is the repo root, so
# the builder copies the whole Kotlin Gradle build — the root settings/build
# scripts, the committed wrapper, the version catalog, the shared module, and this
# server — and `:kt-ktor:installDist` resolves the `:shared` project dependency
# in-image exactly as it does on disk. JDK 21 is the pinned compile toolchain
# (build.gradle.kts `jvmToolchain(21)`); a JRE 21 runs the assembled distribution.
FROM eclipse-temurin:21-jdk-noble AS builder

WORKDIR /app

# Root Gradle build definition + the committed wrapper (bootstraps Gradle 9.6.1)
# and the version catalog. Copied before the sources so a source-only change can
# reuse the dependency-resolution layer. config/detekt is the detekt plugin's
# configured config path (rootProject.file) — copied so project configuration
# never resolves a missing file.
COPY gradlew ./gradlew
COPY gradle ./gradle
COPY settings.gradle.kts build.gradle.kts gradle.properties ./
COPY config/detekt ./config/detekt

# The two Gradle modules that make up the :kt-ktor build.
COPY shared/kotlin ./shared/kotlin
COPY servers/kt-ktor ./servers/kt-ktor

# installDist assembles a self-contained distribution (bin/ start script + lib/
# jars) under build/install/kt-ktor — the application-plugin canonical packaging,
# no fat-jar shading. --no-daemon: one-shot build, no daemon lingering in a layer.
RUN ./gradlew --no-daemon :kt-ktor:installDist

FROM eclipse-temurin:21-jre-noble

WORKDIR /app

# Baked env contract (host = compose service name; the harness only injects
# JWT_SECRET at run time). ENV=prod silences the request logger; Ktor's EngineMain
# reads HOST/PORT from application.yaml's $HOST/$PORT overrides.
ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=25001 \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

# The assembled distribution's contents (bin/kt-ktor + lib/*.jar) land in /app.
COPY --from=builder /app/servers/kt-ktor/build/install/kt-ktor ./

RUN groupadd -g 1001 app-group \
    && useradd -u 1001 -g app-group -M -s /usr/sbin/nologin app-runner

USER app-runner

EXPOSE 25001

CMD ["./bin/kt-ktor"]
