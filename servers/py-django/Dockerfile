FROM python:3.14.6-slim AS builder

WORKDIR /app/servers/py-django

RUN pip install --no-cache-dir uv

# The bench-shared path source (../../shared/python) must be present before
# `uv sync` so the local package builds; the repo tree is mirrored under /app.
COPY shared/python /app/shared/python

COPY servers/py-django/pyproject.toml servers/py-django/uv.lock ./
RUN uv sync --frozen --no-dev

COPY servers/py-django ./

FROM python:3.14.6-slim

# Match the builder's venv path exactly: uv writes absolute shebangs into
# .venv/bin/*, so relocating the venv to a different prefix would break them.
WORKDIR /app/servers/py-django

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=8080 \
    DJANGO_SETTINGS_MODULE=src.settings \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /app/servers/py-django/.venv ./.venv
COPY --from=builder /app/servers/py-django/src ./src
COPY --from=builder /app/servers/py-django/pyproject.toml ./

RUN addgroup --gid 1001 app-group && \
    adduser --disabled-password --gecos "" --uid 1001 --gid 1001 app-runner

USER app-runner

EXPOSE 8080

# ASGI via uvicorn, single worker + uvloop (fleet fairness canon). --no-access-log
# keeps the logger off in prod (fleet-wide env contract).
CMD [".venv/bin/uvicorn", "src.asgi:application", "--host", "0.0.0.0", "--port", "4002", "--loop", "uvloop", "--workers", "1", "--no-access-log"]
