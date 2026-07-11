# Multi-stage, root-context build (PLAN §2.4): the context is the repo root, so the
# builder copies the whole Kotlin Gradle build — the root settings/build scripts, the
# committed wrapper, the version catalog, the shared module, and this server — and
# `:kt-spring-boot:bootJar` resolves the `:shared` project dependency in-image exactly
# as it does on disk. JDK 21 is the pinned compile toolchain (build.gradle.kts
# `jvmToolchain(21)`); a JRE 21 runs the assembled fat jar.
FROM eclipse-temurin:21-jdk-noble AS builder

WORKDIR /app

# Root Gradle build definition + the committed wrapper (bootstraps Gradle 9.6.1) and
# the version catalog. NOTE: no separate dependency-resolution step exists, so a source
# change re-runs the whole bootJar RUN (wrapper + deps + compile); only the base-image
# layers cache. A cache-mount/dep-priming stage is deliberate complexity we skip.
# config/detekt is the detekt plugin's configured config path (rootProject.file) —
# copied so project configuration never resolves a missing file.
COPY gradlew ./gradlew
COPY gradle ./gradle
COPY settings.gradle.kts build.gradle.kts gradle.properties ./
COPY config/detekt ./config/detekt

# The two Gradle modules that make up the :kt-spring-boot build.
COPY shared/kotlin ./shared/kotlin
COPY servers/kt-spring-boot ./servers/kt-spring-boot

# bootJar assembles the executable Spring Boot fat jar (build/libs/app.jar) — the
# canonical Spring Boot packaging. --no-daemon: one-shot build, no daemon in a layer.
RUN ./gradlew --no-daemon :kt-spring-boot:bootJar

FROM eclipse-temurin:21-jre-noble

WORKDIR /app

# Baked env contract (host = compose service name; the harness only injects JWT_SECRET
# at run time). ENV=prod activates the prod Spring profile (via main()) which silences
# logging; :shared's Env.load() reads the DB URLs below directly.
ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=8080 \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /app/servers/kt-spring-boot/build/libs/app.jar ./app.jar

RUN groupadd -g 1001 app-group \
    && useradd -u 1001 -g app-group -M -s /usr/sbin/nologin app-runner

USER app-runner

EXPOSE 8080

CMD ["java", "-jar", "app.jar"]
