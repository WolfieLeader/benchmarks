<div align="center">
  <img src="./assets/banner.svg" alt="HTTP Benchmarks" />
  <p>
    A benchmark harness comparing 10 HTTP frameworks<br/>
    across Node.js, Bun, Deno, Go, and Python<br/>
    with real database operations and metrics visualization.
  </p>

[![Express](https://img.shields.io/badge/Express-000000?style=for-the-badge&logo=express&logoColor=white)](./http-servers/typescript/node-express)
[![Fastify](https://img.shields.io/badge/Fastify-000000?style=for-the-badge&logo=fastify&logoColor=white)](./http-servers/typescript/node-fastify)
[![NestJS](https://img.shields.io/badge/NestJS-E0234E?style=for-the-badge&logo=nestjs&logoColor=white)](./http-servers/typescript/node-nestjs)
[![Hono](https://img.shields.io/badge/Hono-E36002?style=for-the-badge&logo=hono&logoColor=white)](./http-servers/typescript/bun-honojs)
[![Elysia](https://img.shields.io/badge/Elysia-14151A?style=for-the-badge&logo=bun&logoColor=white)](./http-servers/typescript/bun-elysia)
[![Oak](https://img.shields.io/badge/Oak-70FFAF?style=for-the-badge&logo=deno&logoColor=black)](./http-servers/typescript/deno-oak)
[![Chi](https://img.shields.io/badge/Chi-00ADD8?style=for-the-badge&logo=go&logoColor=white)](./http-servers/go/chi)
[![Gin](https://img.shields.io/badge/Gin-00ADD8?style=for-the-badge&logo=go&logoColor=white)](./http-servers/go/gin)
[![Fiber](https://img.shields.io/badge/Fiber-00ADD8?style=for-the-badge&logo=go&logoColor=white)](./http-servers/go/fiber)
[![FastAPI](https://img.shields.io/badge/FastAPI-009688?style=for-the-badge&logo=fastapi&logoColor=white)](./http-servers/python/fastapi)

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
