FROM python:3.14.6-slim AS builder

WORKDIR /app/servers/py-flask

RUN pip install --no-cache-dir uv

# The bench-shared path source (../../shared/python) must be present before
# `uv sync` so the local package builds; the repo tree is mirrored under /app.
COPY shared/python /app/shared/python

COPY servers/py-flask/pyproject.toml servers/py-flask/uv.lock ./
RUN uv sync --frozen --no-dev

COPY servers/py-flask ./

FROM python:3.14.6-slim

# Match the builder's venv path exactly: uv writes absolute shebangs into
# .venv/bin/*, so relocating the venv to a different prefix would break them.
WORKDIR /app/servers/py-flask

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=23003 \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /app/servers/py-flask/.venv ./.venv
COPY --from=builder /app/servers/py-flask/src ./src
COPY --from=builder /app/servers/py-flask/gunicorn.conf.py ./
COPY --from=builder /app/servers/py-flask/pyproject.toml ./

RUN addgroup --gid 1001 app-group && \
    adduser --disabled-password --gecos "" --uid 1001 --gid 1001 app-runner

USER app-runner

EXPOSE 23003

# Sync WSGI via gunicorn, single worker + 50-thread pool (fleet fairness canon;
# see gunicorn.conf.py). Exec form so gunicorn is PID 1 and receives SIGTERM/
# SIGINT directly for graceful shutdown. The app-factory call builds the Flask
# app per worker (post-fork), so DB pools are never inherited across the fork.
CMD [".venv/bin/gunicorn", "--config", "gunicorn.conf.py", "src.app:create_app()"]
