FROM node:25-alpine AS builder

WORKDIR /app

COPY package.json pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY . .

FROM node:25-alpine

WORKDIR /app

ENV NODE_ENV=production \
    ENV=prod \
    HOST=0.0.0.0 \
    PORT=3002

COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/src ./src
COPY --from=builder /app/package.json ./
COPY --from=builder /app/tsconfig.json ./

RUN addgroup -g 1001 -S app-group && \
    adduser -S app-runner -u 1001 -G app-group

USER app-runner

EXPOSE 3002

CMD ["node", "--enable-source-maps", "--loader", "tsx", "src/main.ts"]
