# Multi-stage, root-context build (PLAN §2.4): the context is the repo root, so
# the builder copies both the shared crate and this server, and the Cargo path
# dependency `shared = { path = "../../shared/rust" }` resolves in-image exactly
# as it does on disk. All four DB drivers (sqlx, mongodb, redis, scylla) are pure
# Rust with rustls, so no system C libraries are needed.
FROM rust:1.96.1-slim-bookworm AS builder

WORKDIR /app

# Copy the shared crate first (its own path is the `../../shared/rust` target),
# preserving the repo-relative layout so the dependency graph resolves.
COPY shared/rust ./shared/rust
COPY servers/rs-axum ./servers/rs-axum

WORKDIR /app/servers/rs-axum
RUN cargo build --release --locked

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

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

COPY --from=builder /app/servers/rs-axum/target/release/server ./server

RUN groupadd -g 1001 app-group \
    && useradd -u 1001 -g app-group -M -s /usr/sbin/nologin app-runner

USER app-runner

EXPOSE 8080

CMD ["./server"]
