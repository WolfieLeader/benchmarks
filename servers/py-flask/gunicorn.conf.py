"""Gunicorn config for the Flask WSGI server (production / benchmark entrypoint).

Fleet fairness canon (PLAN.md:194, docs/languages/python.md §2.16-18, §6.38):
ONE serving process, matching py-fastapi's single uvicorn worker and py-django's
single ASGI process. Flask is sync WSGI with sync DB drivers (psycopg3, pymongo,
redis-py, scylla-driver), so its concurrency comes from a bounded thread pool —
the sync mirror of the async servers' event loop. The pool is sized to the
pool-of-50 DB canon so a burst of concurrent requests never has more in-flight
work than the DB connection pools (also 50) can serve.

Run directly as the container CMD (exec form, PID 1) so SIGTERM/SIGINT reach the
gunicorn master, which stops accepting, drains in-flight requests within
`graceful_timeout`, then exits — the env contract's graceful shutdown.
"""

from bench_shared.env import env

bind = f"{env.HOST}:{env.PORT}"

# Single serving process (fairness canon) — never raise this without a matching
# fleet-wide decision; it is the number every other server is normalized to.
workers = 1

# Thread pool = concurrency for blocking sync DB I/O (the GIL is released during
# socket waits). Sized to the 50-connection DB pool canon.
worker_class = "gthread"
threads = 50

# Logger off in prod (fleet-wide env contract): no access log. Errors still go to
# stderr so a crash is never silent.
accesslog = None
errorlog = "-"

# Graceful drain window for in-flight requests on SIGTERM/SIGINT.
graceful_timeout = 30
# A SHA-256 chain up to the 1e6 cap (/compute) is well under a second; keep a
# generous request ceiling so a slow DB never trips a false worker timeout.
timeout = 60
