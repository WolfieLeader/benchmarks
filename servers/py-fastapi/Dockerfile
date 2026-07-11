FROM python:3.14.6-slim AS builder

WORKDIR /app/servers/py-fastapi

RUN pip install --no-cache-dir uv

# The bench-shared path source (../../shared/python) must be present before
# `uv sync` so the local package builds; the repo tree is mirrored under /app.
COPY shared/python /app/shared/python

COPY servers/py-fastapi/pyproject.toml servers/py-fastapi/uv.lock ./
RUN uv sync --frozen --no-dev

COPY servers/py-fastapi ./

FROM python:3.14.6-slim

# Match the builder's venv path exactly: uv writes absolute shebangs into
# .venv/bin/*, so relocating the venv to a different prefix would break them.
WORKDIR /app/servers/py-fastapi

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

COPY --from=builder /app/servers/py-fastapi/.venv ./.venv
COPY --from=builder /app/servers/py-fastapi/src ./src
COPY --from=builder /app/servers/py-fastapi/pyproject.toml ./

RUN addgroup --gid 1001 app-group && \
    adduser --disabled-password --gecos "" --uid 1001 --gid 1001 app-runner

USER app-runner

EXPOSE 8080

CMD [".venv/bin/uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080", "--loop", "uvloop", "--workers", "1"]
