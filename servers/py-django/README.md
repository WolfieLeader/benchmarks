# py-django

Django 6 benchmark server, run under ASGI (uvicorn) with async views.

Batteries-included per PLAN §3:

- **Postgres** — Django ORM (psycopg3, connection pool sized to 50).
- **Redis** — Django's first-party cache backend
  (`django.core.cache.backends.redis.RedisCache`), CRUD mapped onto
  `cache.set`/`get`/`delete` + `clear`.
- **MongoDB / Cassandra** — no first-party Django support, so these use the sync
  drivers (`pymongo`, `scylla-driver`) bridged onto the async views via
  `asgiref.sync.sync_to_async`.

Validation rules, error strings, consts, env parsing, and the pydantic schemas
come from `bench-shared`.
