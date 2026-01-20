FROM python:3.14-slim AS builder

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev

COPY . .

FROM python:3.14-slim

WORKDIR /app

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=4001

COPY --from=builder /app/.venv ./.venv
COPY --from=builder /app/src ./src
COPY --from=builder /app/pyproject.toml ./

RUN addgroup --gid 1001 app-group && \
    adduser --disabled-password --gecos "" --uid 1001 --gid 1001 app-runner

USER app-runner

EXPOSE 4001

CMD [".venv/bin/uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "4001"]
