# HTTP Server Benchmark Suite

Benchmark harness and a set of minimal HTTP servers across Node.js, Bun, Python, and Go. The goal is to compare request/response performance under the same API surface. Database integrations (PostgreSQL, MongoDB, Redis) are planned and not wired yet.

## Current Status

- Reference implementation: `bun-honojs` (used as behavior baseline)
- Implemented APIs: core `/` and `/health` + `/params/*` endpoints
- Not implemented yet: auth and database endpoints (planned next)

## Technology Matrix

| Language / Runtime  | Framework            | Port | Status            | PostgreSQL (Driver/ORM) | MongoDB (Driver/ODM) | 
| ------------------- | -------------------- | ---- | ----------------- | ----------------------- | -------------------- | 
| **Node.js** v25.3.0 | **Express** v5.0.1   | 3001 | ✅ Done           | `pg` (Raw)              | `mongoose`           |
| **Node.js** v25.3.0 | **NestJS** v10.4.15  | 3002 | ✅ Done           | `TypeORM`               | `mongoose`           |
| **Node.js** v25.3.0 | **Fastify** v5.2.0   | 3003 | ✅ Done           | `prisma`                | `mongodb`            |
| **Deno** v2.6.5     | **Oak** v17.1.3      | 3004 | ✅ Done           | `postgres.js`           | `mongodb`            |
| **Bun** v1.3.6      | **Hono** v4.6.14     | 3005 | ✅ Done           | `drizzle-orm`           | `mongoose`           |
| **Bun** v1.3.6      | **Elysia** v1.1.26   | 3006 | ✅ Done           | `bun:sql`               | `mongoose`           |
| **Python** v3.14.2  | **FastAPI** v0.115.6 | 4001 | ✅ Done           | `SQLAlchemy` (Async)    | `Motor` + `Beanie`   |
| **Python** v3.14.2  | **Flask** v3.1.0     | 4002 | 🚧 In Progress    | `SQLAlchemy` (Core)     | `PyMongo`            |
| **Python** v3.14.2  | **Django** v5.1.4    | 4003 | 🚧 In Progress    | `Django ORM`            | `Djongo`             |
| **Go** v1.25.6      | **Chi** v5.2.0       | 5001 | ✅ Done           | `pgx` (Raw)             | `mongo-go-driver`    |
| **Go** v1.25.6      | **Gin** v1.10.0      | 5002 | ✅ Done           | `GORM`                  | `mongo-go-driver`    |
| **Go** v1.25.6      | **Fiber** v2.52.5    | 5003 | ✅ Done           | `sqlc`                  | `mongo-go-driver`    |
| **Go** v1.25.6      | **Echo** v4.13.3     | 5004 | 🚧 In Progress    | `bun`                   | `mongo-go-driver`    |

## Requirements

- **Node.js**: 25.3.0 + `pnpm` 10.28.0
- **Bun**: 1.3.6
- **Deno**: 2.6.5
- **Python**: 3.14.2 + `uv` 0.9.26
- **Go**: 1.25.6
- **Docker** (optional): for running DB containers later

## Project Layout

- `benchmark/` Go benchmark runner
- `bun-honojs/` reference server
- `node-express/`, `node-fastify/`, `node-nestjs/` Node servers
- `python-fastapi/` Python server
- `go-chi/`, `go-gin/`, `go-fiber/` Go servers

## API Surface (Current)

- `GET /` returns `{ "message": "Hello, World!" }`
- `GET /health` returns `OK`

### Params Endpoints

- `GET /params/search` query params `q`, `limit`
- `GET /params/url/{dynamic}` route param
- `GET /params/header` reads `X-Custom-Header`
- `POST /params/body` expects JSON object
- `GET /params/cookie` reads `foo`, sets `bar`
- `POST /params/form` accepts urlencoded or multipart
- `POST /params/file` accepts multipart text file

## Planned (Near Future)

- Auth endpoints:
  - `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`
  - `GET /auth/me`, `PATCH /auth/permissions`
  - `GET /auth/public`, `GET /auth/protected`, `GET /auth/admin`
- Database endpoints:
  - `/pg/users`, `/mongo/users`, `/redis/users`
- Docker Compose for `postgres`, `mongodb`, `redis`

## Environment Variables

Common variables (defaults shown):

- `ENV=dev` (must be `dev` or `prod`; non-prod enables logging)
- `HOST=0.0.0.0` (supports IP, URL, or `localhost`)
- `PORT` per server (see table above)

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

The benchmark currently targets `GET /` with a fixed response check. More scenarios will be added as the API grows.

## Docker (Optional)

Each server has a Dockerfile. Images are currently available for Go servers via:

```sh
make build-images
```

Database containers and server images will be expanded once DB endpoints are implemented.
