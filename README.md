# HTTP Server Benchmark Suite

Benchmark harness and a set of minimal HTTP servers across Node.js, Bun, Deno, Python, and Go. Each server implements the same API surface for consistent benchmarking.

## Stack Map

| Folder           | Runtime       | Framework       | Port |
| ---------------- | ------------- | --------------- | ---- |
| `benchmark`      | Go 1.25.5     | -               | -    |
| `bun-elysia`     | Bun 1.3.4     | Elysia 1.4.22   | 3006 |
| `bun-honojs`     | Bun 1.3.4     | Hono 4.11.3     | 3005 |
| `deno-oak`       | Deno 2.6.5    | Oak 17.2.0      | 3004 |
| `go-chi`         | Go 1.25.5     | Chi 5.2.3       | 5001 |
| `go-gin`         | Go 1.25.5     | Gin 1.11.0      | 5002 |
| `go-fiber`       | Go 1.25.5     | Fiber 2.52.10   | 5003 |
| `node-express`   | Node >=24     | Express 5.2.1   | 3001 |
| `node-nestjs`    | Node >=24     | NestJS 11.1.12  | 3002 |
| `node-fastify`   | Node >=24     | Fastify 5.7.1   | 3003 |
| `python-fastapi` | Python >=3.14 | FastAPI >=0.128 | 4001 |

## Configuration

- ENV: `dev` (default, enables logger) or `prod`.
- HOST: `0.0.0.0` (default). Accepts IP or `localhost` (mapped to `0.0.0.0`).
- PORT: See Stack Map above.

Benchmark config is JSON-only and lives at `benchmark/config.json`.

## API Surface

### Base Routes

- `GET /` -> `OK` (Text)
- `GET /health` -> `{ "message": "Hello, World!" }` (JSON)

### Params Routes (`/params/*`)

- `GET /search`: Parse query `q` (trim, def `none`) and `limit` (safe int, def 10).
- `GET /url/:val`: Return `{ "dynamic": "<val>" }`.
- `GET /header`: Read `X-Custom-Header` (trim, def `none`).
- `POST /body`: Validate JSON object (no array/null). Return `{ "body": <parsed> }`.
- `GET /cookie`: Read cookie `foo` (trim, def `none`), set cookie `bar`.
- `POST /form`: Support `urlencoded`/`multipart`. Return `{ "name": "<trim>", "age": <int> }` (def: `none`, 0).
- `POST /file`: Multipart `file` (max 1MB, `text/plain` only). Return `{ "filename", "size", "content" }`.

## Error Responses

Return JSON `{ "error": "<message>" }`.

| Status | Messages                                                                                               |
| ------ | ------------------------------------------------------------------------------------------------------ |
| 400    | `invalid JSON body`, `invalid form data`, `invalid multipart form data`, `file not found in form data` |
| 404    | `not found`                                                                                            |
| 413    | `file size exceeds limit`                                                                              |
| 415    | `only text/plain files are allowed`, `file does not look like plain text`                              |
| 500    | `internal error`                                                                                       |

## Framework Notes

- Fastify: Return values directly from handlers; avoid `reply.send()`.

## Run Servers (Dev)

You can use the Makefile shortcuts:

```sh
make honojs
make express
make fastify
make nestjs
make fastapi
make chi
make gin
make fiber
```

Or run directly inside each folder (see each `package.json` or language-specific setup).

## Benchmark Runner

```sh
make benchmark
```
