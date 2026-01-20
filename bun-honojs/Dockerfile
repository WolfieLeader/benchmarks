FROM oven/bun:1.3-alpine AS builder

WORKDIR /app

COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile

COPY . .

FROM oven/bun:1.3-alpine

WORKDIR /app

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=3005

COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/src ./src
COPY --from=builder /app/package.json ./

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 3005

CMD ["bun", "run", "src/index.ts"]
