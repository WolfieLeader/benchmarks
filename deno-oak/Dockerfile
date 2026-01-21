FROM denoland/deno:2.6.5-alpine AS deps

WORKDIR /app

ENV DENO_DIR=/deno-dir

COPY deno.json ./
COPY src ./src

RUN deno cache --allow-net --allow-env src/index.ts

FROM denoland/deno:2.6.5-alpine

WORKDIR /app

ENV DENO_DIR=/deno-dir \
    ENV=prod \
    HOST=0.0.0.0 \
    PORT=3004

COPY --from=deps /deno-dir /deno-dir
COPY --from=deps /app /app

USER deno

EXPOSE 3004

CMD ["deno", "run", "--allow-net", "--allow-env", "src/index.ts"]
