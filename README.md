<div align="center">
  <img src="./assets/banner.svg" alt="HTTP Benchmarks" />
  <p>
    A side-by-side HTTP performance comparison <br/> 
    focused on identical behavior, real database flows, <br/> 
    and metrics that enable fair cross-runtime evaluation.
  </p>

<p align="center">
  <img src="./assets/node.svg" alt="Node.js" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/bun.svg" alt="Bun" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/deno.svg" alt="Deno" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/go.svg" alt="Go" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/python.svg" alt="Python" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/typescript.svg" alt="TypeScript" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/postgresql.svg" alt="PostgreSQL" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/mongodb.svg" alt="MongoDB" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/redis.svg" alt="Redis" height="28" style="display:inline-block; vertical-align:middle;" />
  <img src="./assets/cassandra.svg" alt="Cassandra" height="28" style="display:inline-block; vertical-align:middle;" />
  <a href="./http-servers/typescript/node-express"><img src="./assets/express.svg" alt="Express" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/typescript/node-fastify"><img src="./assets/fastify.svg" alt="Fastify" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/typescript/node-nestjs"><img src="./assets/nestjs.svg" alt="NestJS" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/typescript/bun-honojs"><img src="./assets/honojs.svg" alt="Hono" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/typescript/bun-elysia"><img src="./assets/elysiajs.svg" alt="Elysia" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/typescript/deno-oak"><img src="./assets/oak.svg" alt="Oak" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/go/chi"><img src="./assets/chi.svg" alt="Chi" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/go/gin"><img src="./assets/gin.svg" alt="Gin" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/go/fiber"><img src="./assets/fiber.svg" alt="Fiber" height="28" style="display:inline-block; vertical-align:middle;" /></a>
  <a href="./http-servers/python/fastapi"><img src="./assets/fastapi.svg" alt="FastAPI" height="28" style="display:inline-block; vertical-align:middle;" /></a>
</p>
</div>

## Stack Map 📦

| Folder                                 | Runtime       | Framework       | Port |
| -------------------------------------- | ------------- | --------------- | ---- |
| `benchmarks`                           | Go 1.25.6     | -               | -    |
| `http-servers/typescript/node-express` | Node >=24     | Express 5.2.1   | 3001 |
| `http-servers/typescript/node-nestjs`  | Node >=24     | NestJS 11.1.12  | 3002 |
| `http-servers/typescript/node-fastify` | Node >=24     | Fastify 5.7.1   | 3003 |
| `http-servers/typescript/deno-oak`     | Deno 2.6.5    | Oak 17.2.0      | 3004 |
| `http-servers/typescript/bun-honojs`   | Bun 1.3.4     | Hono 4.11.3     | 3005 |
| `http-servers/typescript/bun-elysia`   | Bun 1.3.4     | Elysia 1.4.22   | 3006 |
| `http-servers/go/chi`                  | Go 1.25.6     | Chi 5.2.3       | 5001 |
| `http-servers/go/gin`                  | Go 1.25.6     | Gin 1.11.0      | 5002 |
| `http-servers/go/fiber`                | Go 1.25.6     | Fiber 2.52.10   | 5003 |
| `http-servers/python/fastapi`          | Python >=3.14 | FastAPI >=0.128 | 4001 |

## Quick Start 🚀

```sh
just benchmark                # Run benchmark (interactive mode)
just benchmark --servers=a,b  # Run benchmark for specific servers only
just dev honojs               # Start dev server (honojs, express, fastify, etc.)
```

## Configuration ⚙️

| Variable | Default       | Description                              |
| -------- | ------------- | ---------------------------------------- |
| `ENV`    | `dev`         | `dev` enables logger, `prod` disables it |
| `HOST`   | `0.0.0.0`     | IP or `localhost` (mapped to `0.0.0.0`)  |
| `PORT`   | See Stack Map | Server port                              |

Benchmark config is JSON-only and lives at `config/config.json`.

## API Surface 🌐

### Base Routes

| Method | Route     | Response              |
| ------ | --------- | --------------------- |
| GET    | `/`       | `{ "hello": "world"}` |
| GET    | `/health` | `OK` (text/plain)     |

### Params Routes (`/params/*`)

| Method | Route              | Description                                                                              |
| ------ | ------------------ | ---------------------------------------------------------------------------------------- |
| GET    | `/params/search`   | Query `q` (trim, default `none`) and `limit` (safe int, default 10)                      |
| GET    | `/params/url/:val` | Returns `{ "dynamic": "<val>" }`                                                         |
| GET    | `/params/header`   | Reads `X-Custom-Header` (trim, default `none`)                                           |
| POST   | `/params/body`     | Validates JSON object (no array/null), returns `{ "body": <parsed> }`                    |
| GET    | `/params/cookie`   | Reads cookie `foo` (trim, default `none`), sets cookie `bar`                             |
| POST   | `/params/form`     | Supports urlencoded/multipart, returns `{ "name": "<trim>", "age": <int> }`              |
| POST   | `/params/file`     | Multipart `file` (max 1MB, text/plain only), returns `{ "filename", "size", "content" }` |

### Database Routes (`/db/{database}/*`)

Supported databases: `postgres`, `mongodb`, `redis`, `cassandra`

| Method | Route                       | Description                                                |
| ------ | --------------------------- | ---------------------------------------------------------- |
| GET    | `/db/{database}/health`     | Database health check                                      |
| POST   | `/db/{database}/users`      | Create user `{ "name", "email", "favoriteNumber?" }` (201) |
| GET    | `/db/{database}/users/{id}` | Get user by ID (200 or 404)                                |
| PATCH  | `/db/{database}/users/{id}` | Update user fields (200 or 404)                            |
| DELETE | `/db/{database}/users/{id}` | Delete user by ID (200 or 404)                             |
| DELETE | `/db/{database}/users`      | Delete all users (200)                                     |
| DELETE | `/db/{database}/reset`      | Reset database (200)                                       |

## Error Responses ⚠️

All errors return JSON `{ "error": "<message>" }`.

| Status | Messages                                                                                               |
| ------ | ------------------------------------------------------------------------------------------------------ |
| 400    | `invalid JSON body`, `invalid form data`, `invalid multipart form data`, `file not found in form data` |
| 404    | `not found`                                                                                            |
| 413    | `file size exceeds limit`                                                                              |
| 415    | `only text/plain files are allowed`, `file does not look like plain text`                              |
| 500    | `internal error`                                                                                       |

## Databases 🗄️

All servers connect to all 4 databases with the same user schema.

| Database   | ID Type           |
| ---------- | ----------------- |
| PostgreSQL | UUID v7           |
| MongoDB    | ObjectId (native) |
| Redis      | UUID v7           |
| Cassandra  | UUID v7           |

**User schema:** `id`, `name`, `email`, `favoriteNumber` (optional)

## Grafana 📊

Metrics are exported to InfluxDB and visualized in Grafana during benchmarks.

| Service | URL                   | Username | Password  |
| ------- | --------------------- | -------- | --------- |
| Grafana | http://localhost:3000 | admin    | benchmark |

### Exported Metrics

| Measurement             | Fields                                                     | Tags                                        |
| ----------------------- | ---------------------------------------------------------- | ------------------------------------------- |
| `request_latency`       | latency_ns, server_offset_ms, endpoint_offset_ms           | run_id, server, endpoint, method            |
| `sequence_latency`      | total_duration_ns, server_offset_ms, sequence_offset_ms    | run_id, server, sequence_id, database       |
| `sequence_step_latency` | latency_ns, server_offset_ms                               | run_id, server, sequence_id, database, step |
| `resource_stats`        | memory_min/avg/max_bytes, cpu_min/avg/max_percent, samples | run_id, server                              |

## Development 🛠️

### Per-Target Commands

All verification commands accept an optional target (default: `all`).

```sh
just typecheck chi       # Type check only chi
just fmt honojs          # Format only honojs
just lint fastapi        # Lint only fastapi
just verify express      # Full verification for express
```

Valid targets: `honojs`, `elysia`, `oak`, `express`, `nestjs`, `fastify`, `chi`, `gin`, `fiber`, `fastapi`, `benchmark`, `root`

### Full Command Reference

```sh
just benchmark           # Run benchmark (interactive mode)
just install             # Install all dependencies
just typecheck           # Type/compile check all projects
just fmt                 # Format all code
just lint                # Lint all code
just verify              # Runs typecheck -> fmt -> lint
just images              # Build all Docker images
just clean               # Remove build artifacts and node_modules
just remove-images       # Remove Docker images
just grafana-up          # Start Grafana/InfluxDB stack
just grafana-down        # Stop Grafana/InfluxDB stack
just db-up               # Start database stack
just db-down             # Stop database stack
```
