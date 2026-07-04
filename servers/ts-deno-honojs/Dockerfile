# Root-context build (PLAN §2.4, §2.2). A Node+pnpm builder installs the workspace
# and builds @bench/shared + @bench/hono-app (the shared Hono app, PLAN §4);
# `pnpm deploy` prunes to this server + the two injected @bench/* packages in
# node_modules. The Deno runtime consumes that pnpm node_modules with
# `nodeModulesDir: manual` (PLAN §2.2: pnpm installs, Deno reads) — hono resolves
# from node_modules (npm), not JSR.
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
COPY shared/typescript-hono ./shared/typescript-hono
COPY servers/ts-deno-honojs ./servers/ts-deno-honojs
RUN pnpm --filter @bench/shared run build
RUN pnpm --filter @bench/hono-app run build
RUN pnpm --filter ts-deno-honojs deploy --legacy /deploy

FROM denoland/deno:alpine-2.9.1
WORKDIR /app

ENV ENV=prod \
    HOST=0.0.0.0 \
    PORT=22205 \
    DENO_DIR=/deno-dir \
    POSTGRES_URL=postgres://postgres:postgres@postgres:5432/benchmarks \
    MONGODB_URL=mongodb://mongodb:27017 \
    MONGODB_DB=benchmarks \
    REDIS_URL=redis://redis:6379 \
    CASSANDRA_CONTACT_POINTS=cassandra \
    CASSANDRA_KEYSPACE=benchmarks \
    CASSANDRA_LOCAL_DATACENTER=datacenter1

COPY --from=builder /deploy ./

# Prime the module cache at build time (as root) so the container starts
# offline-clean. hono resolves from the deployed node_modules (nodeModulesDir:
# manual) — no network fetch. Then hand ownership to the non-root deno user.
RUN deno cache src/index.ts
RUN chown -R deno:deno /app /deno-dir

USER deno

EXPOSE 22205
CMD ["deno", "run", "--allow-all", "src/index.ts"]
