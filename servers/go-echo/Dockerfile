FROM golang:1.27rc1-alpine AS builder

WORKDIR /app

# Shared Go module is the `replace shared => ../../shared/go` target; copy it
# first (and preserve the repo-relative layout) so the module graph resolves.
COPY shared/go ./shared/go

COPY servers/go-echo/go.mod servers/go-echo/go.sum ./servers/go-echo/
WORKDIR /app/servers/go-echo
RUN go mod download

WORKDIR /app
COPY servers/go-echo ./servers/go-echo
WORKDIR /app/servers/go-echo

ENV CGO_ENABLED=0 \
    GOOS=linux

RUN go build -trimpath -ldflags="-s -w" -o bin/server ./cmd/main.go

FROM alpine:3.24

WORKDIR /app

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=5005 \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /app/servers/go-echo/bin/server .

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 5005

CMD ["./server"]
