# Root-context build (PLAN §2.4). pnpm workspace: the frozen install needs every
# member's package.json to resolve the single root lockfile; `pnpm deploy` then
# prunes to a self-contained single-server image (only this server + the built
# @bench/shared, injected — no other server's deps, no workspace symlink graph).
FROM node:26.4.0-alpine AS builder
WORKDIR /app
ENV CI=true
RUN npm i -g pnpm@11.9.0

COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./
COPY shared/typescript/package.json ./shared/typescript/
COPY shared/typescript-hono/package.json ./shared/typescript-hono/
COPY servers/ts-express/package.json ./servers/ts-express/
COPY servers/ts-fastify/package.json ./servers/ts-fastify/
COPY servers/ts-nestjs/package.json ./servers/ts-nestjs/
COPY servers/ts-bun-elysia/package.json ./servers/ts-bun-elysia/
COPY servers/ts-honojs/package.json ./servers/ts-honojs/
COPY servers/ts-bun-honojs/package.json ./servers/ts-bun-honojs/
COPY servers/ts-deno-honojs/package.json ./servers/ts-deno-honojs/
COPY servers/ts-deno-oak/package.json ./servers/ts-deno-oak/
RUN pnpm install --frozen-lockfile

COPY shared/typescript ./shared/typescript
COPY servers/ts-express ./servers/ts-express
RUN pnpm --filter @bench/shared run build
RUN pnpm --filter server-express deploy --legacy --prod=false /deploy

FROM node:26.4.0-alpine
WORKDIR /app

ENV NODE_ENV=production \
    ENV=prod \
    HOST=0.0.0.0 \
    PORT=8080 \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /deploy ./

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 8080
CMD ["node", "--enable-source-maps", "--import", "tsx", "src/index.ts"]
