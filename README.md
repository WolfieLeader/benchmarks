# HTTP Server & Database Benchmark Project

Comprehensive benchmark comparing different HTTP server implementations across various technology stacks, each connecting to PostgreSQL, MongoDB, and Redis simultaneously.

## Technology Stacks

| Language / Runtime  | Framework            | Port | Status           | PostgreSQL (Driver/ORM) | MongoDB (Driver/ODM) | Redis Client     | Validation                |
| ------------------- | -------------------- | ---- | ---------------- | ----------------------- | -------------------- | ---------------- | ------------------------- |
| **Node.js** v25.3.0 | **Express** v5.0.1   | 3001 | 🚫 Pending (A)   | `pg` (Raw)              | `mongoose`           | `redis`          | `zod`                     |
| **Node.js** v25.3.0 | **NestJS** v10.4.15  | 3002 | 🚫 Pending (A)   | `TypeORM`               | `mongoose`           | `ioredis`        | `class-validator`         |
| **Node.js** v25.3.0 | **Fastify** v5.2.0   | 3003 | 🚫 Pending (B)   | `prisma`                | `mongodb`            | `@fastify/redis` | `typebox`                 |
| **Deno** v2.6.5     | **Oak** v17.1.3      | 3004 | 🚫 Pending (C)   | `postgres.js`           | `mongodb`            | `redis`          | `zod`                     |
| **Bun** v1.3.6      | **Hono** v4.6.14     | 3005 | ✅ Reference (A) | `drizzle-orm`           | `mongoose`           | `ioredis`        | `arktype`                 |
| **Bun** v1.3.6      | **Elysia** v1.1.26   | 3006 | 🚫 Pending (C)   | `bun:sql`               | `mongoose`           | `ioredis`        | `typebox` (Built-in)      |
| **Python** v3.14.2  | **FastAPI** v0.115.6 | 4001 | 🚫 Pending (A)   | `SQLAlchemy` (Async)    | `Motor` + `Beanie`   | `redis-py`       | `Pydantic` v2             |
| **Python** v3.14.2  | **Flask** v3.1.0     | 4002 | 🚫 Pending (B)   | `SQLAlchemy` (Core)     | `PyMongo`            | `redis-py`       | `Marshmallow`             |
| **Python** v3.14.2  | **Django** v5.1.4    | 4003 | 🚫 Pending (B)   | `Django ORM`            | `Djongo`             | `django-redis`   | `Django Forms`            |
| **Go** v1.25.6      | **Chi** v5.2.0       | 5001 | 🚫 Pending (A)   | `pgx` (Raw)             | `mongo-go-driver`    | `go-redis`       | `go-playground/validator` |
| **Go** v1.25.6      | **Gin** v1.10.0      | 5002 | 🚫 Pending (A)   | `GORM`                  | `mongo-go-driver`    | `go-redis`       | `go-playground/validator` |
| **Go** v1.25.6      | **Fiber** v2.52.5    | 5003 | 🚫 Pending (A)   | `sqlc`                  | `mongo-go-driver`    | `go-redis`       | `go-playground/validator` |
| **Go** v1.25.6      | **Echo** v4.13.3     | 5004 | 🚫 Pending (C)   | `bun`                   | `mongo-go-driver`    | `go-redis`       | `go-playground/validator` |

### Requirements

- **TypeScript** and **JavaScript** Ecosystem
  - **Node.js**: 25.3.0 and `pnpm` package manager 10.28.0
  - **Bun**: v1.3.6
  - **Deno**: 2.6.5
- **Python**: 3.14.2 and `uv` package manager 0.9.26
- **Go**: 1.25.6
- **Docker**: For running databases and servers
  - **Dockerfile**: Each server has its own Dockerfile
  - **Docker Compose**: 5.0.1 For `postgres`, `mongodb`, and `redis`
    - **PostgreSQL**: 18.1
    - **MongoDB**: 8.2
    - **Redis**: 8.2
- **Makefile**: Easy commands for setup and running

## Quick Start

## Architecture

### Middleware Stack (All Servers)

1. **Request ID**: Generate UUID v4, set X-Request-ID header
2. **Logger**: JSON format logging
3. **CORS**: Allow all origins
4. **Security Headers**: CSP, HSTS, X-Frame-Options, etc.
5. **Body Parser**: JSON (1MB limit)
6. **Rate Limiter**: Redis sliding window
7. **Auth**: JWT validation
8. **Validation**: Framework-specific validators
9. **Error Handler**: Standardized error responses
10. **Not Found**: 404 handler

### API Endpoints

- `GET /health` - Health check

- `GET /params/search` - Query parameters
- `GET /params/:dynamic` - Dynamic route parameter
- `POST /params/body` - JSON body
- `GET /params/header` - Custom header
- `GET /params/cookie` - Cookie parameter
- `POST /params/form` - Form data
- `POST /params/file` - File upload

- `POST /auth/register` - Register user
- `POST /auth/login` - Login
- `POST /auth/refresh` - Refresh token
- `POST /auth/logout` - Logout
- `GET /auth/me` - Get current user
- `PATCH /auth/permissions` - Update permissions

- `GET /auth/public` - Public endpoint
- `GET /auth/protected` - Protected endpoint
- `GET /auth/admin` - Admin endpoint

- `/pg/users` - PostgreSQL CRUD
- `/mongo/users` - MongoDB CRUD
- `/redis/users` - Redis CRUD
