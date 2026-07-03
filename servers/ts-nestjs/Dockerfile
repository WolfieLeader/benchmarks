FROM node:26.4.0-alpine AS builder
WORKDIR /app

RUN npm i -g pnpm@10.29.1

COPY servers/ts-nestjs/package.json servers/ts-nestjs/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY servers/ts-nestjs/tsconfig.json servers/ts-nestjs/tsconfig.build.json ./
COPY servers/ts-nestjs/src ./src

RUN pnpm build

FROM node:26.4.0-alpine

WORKDIR /app

ENV NODE_ENV=production \
    ENV=prod \
    HOST=0.0.0.0 \
    PORT=3002 \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package.json ./

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 3002

CMD ["node", "dist/main.js"]
