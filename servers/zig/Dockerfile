# Alpine base: it ships both the MongoDB C driver and the Cassandra cpp-driver
# in its repositories (main + community), which Debian/Ubuntu do not.
FROM alpine:3.22 AS builder

ARG ZIG_VERSION=0.16.0

# Build deps: pkgconf (resolves the mongoc/bson include+lib flags), the two C
# database drivers' headers, and musl/C toolchain headers for @cImport.
RUN apk add --no-cache curl xz musl-dev pkgconf \
    mongo-c-driver-dev cassandra-cpp-driver-dev

# Install the official Zig 0.16.0 release (statically linked, runs on musl).
RUN ARCH="$(uname -m)" && \
    curl -fsSL "https://ziglang.org/download/${ZIG_VERSION}/zig-${ARCH}-linux-${ZIG_VERSION}.tar.xz" \
      | tar -xJ -C /opt && \
    ln -s "/opt/zig-${ARCH}-linux-${ZIG_VERSION}/zig" /usr/local/bin/zig

WORKDIR /app

# Resolve dependencies first so the fetch is cached independently of source edits.
COPY servers/zig/build.zig servers/zig/build.zig.zon ./
RUN zig build --fetch

COPY servers/zig/src ./src
RUN zig build -Doptimize=ReleaseFast

FROM alpine:3.22

# Runtime shared libraries for the two C drivers (+ their C++/gcc runtime).
RUN apk add --no-cache mongo-c-driver cassandra-cpp-driver libstdc++ libgcc

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

COPY --from=builder /app/zig-out/bin/server ./server

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 8080

CMD ["./server"]
