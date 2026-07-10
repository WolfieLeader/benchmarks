# py-flask

Flask 3 benchmark server, sync WSGI. Implements the full contract surface,
including the `web` suite (`GET /html`, `/jwt/sign`, `/jwt/verify`,
`POST /validate`, `GET /compute`).

## Server model

Sync WSGI under **gunicorn**, the idiomatic production Flask server. Fairness
canon (PLAN §3, `docs/languages/python.md` §6.38): **one serving process** (like
py-fastapi's single uvicorn worker), concurrency from a **50-thread pool**
(`gthread`) — the sync mirror of the async servers' event loop, sized to the
50-connection DB pool canon so no request starves for a connection. See
`gunicorn.conf.py`. `python -m src.main` runs the Werkzeug dev server instead
(dev only).

App structure is idiomatic Flask: an application factory (`create_app`) plus
blueprints (`basic`, `params`, `db`, `web`).

## Databases (sync drivers, in-server)

- **Postgres** — `psycopg3` + `psycopg_pool.ConnectionPool` (max_size 50).
- **MongoDB** — `pymongo` (maxPoolSize 50).
- **Redis** — `redis-py` (a user is a JSON string under `user:<id>`; reset = FLUSHDB).
- **Cassandra** — `scylla-driver` (+ the contact-point `AddressTranslator`).

The repositories live in-server (not `shared/python`): under the multi-consumer
rule they have a single plain-sync consumer today — py-django's Mongo/Cassandra
repos are `sync_to_async`-bridged (a different, async shape). See the PR report
for the recommended shared-sync-repo extraction lane.

Validation rules, error strings, consts, env parsing (incl. `JWT_SECRET`), and
the base pydantic schemas come from `bench-shared`. Web-suite pieces (JWT via
`PyJWT`, the `/validate` schema, the SHA-256 chain, and the web error strings)
are in-server — py-flask is the first web-suite implementer.
