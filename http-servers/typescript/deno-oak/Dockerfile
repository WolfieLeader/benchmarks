FROM denoland/deno:alpine-2.6.5 AS deps

WORKDIR /app

ENV DENO_DIR=/deno-dir

COPY deno.json ./
COPY src ./src

RUN deno install

FROM denoland/deno:alpine-2.6.5

WORKDIR /app

ENV DENO_DIR=/deno-dir \
    ENV=prod \
    HOST=0.0.0.0 \
    PORT=3004

COPY --from=deps /deno-dir /deno-dir
COPY --from=deps /app /app

USER deno

EXPOSE 3004

CMD ["deno", "run", "--allow-all", "src/index.ts"]
