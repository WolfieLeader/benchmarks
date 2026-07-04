from __future__ import annotations

from urllib.parse import urlparse

from bench_shared.consts import MAX_REQUEST_BYTES
from bench_shared.env import env

# Logger off in prod (fleet-wide env contract): the request-logging middleware is
# only installed when running the dev profile.
DEBUG = env.ENV == "dev"

# Inert by design: no auth/sessions/CSRF/admin are enabled, so nothing signs or
# verifies with SECRET_KEY — Django only requires the setting to be present.
SECRET_KEY = "insecure-benchmark-key-not-a-secret"  # noqa: S105

ALLOWED_HOSTS = ["*"]

INSTALLED_APPS = ["src.api"]

# Stateless JSON API: the default security middleware (CSRF, sessions, auth)
# would 403 unauthenticated POSTs, so the stack is deliberately minimal. The
# request logger is dev-only, matching the fleet's "logger off in prod" clause.
MIDDLEWARE = ["src.middleware.LoggingMiddleware"] if DEBUG else []

ROOT_URLCONF = "src.urls"

_pg = urlparse(env.POSTGRES_URL)
DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.postgresql",
        "NAME": _pg.path.lstrip("/"),
        "USER": _pg.username or "",
        "PASSWORD": _pg.password or "",
        "HOST": _pg.hostname or "",
        "PORT": str(_pg.port) if _pg.port else "",
        # psycopg3 connection pool sized to the fleet fairness canon (50),
        # single process — mirrors py-fastapi's SQLAlchemy pool_size=50.
        "OPTIONS": {"pool": {"min_size": 1, "max_size": 50}},
    }
}

# First-party Redis cache backend (locked decision, PLAN §3): the Redis routes
# map CRUD onto this cache abstraction rather than a hand-rolled redis client.
CACHES = {
    "default": {
        "BACKEND": "django.core.cache.backends.redis.RedisCache",
        "LOCATION": env.REDIS_URL,
        # Datastore role: entries must not silently expire under the default 300s.
        "TIMEOUT": None,
    }
}

# Files are excluded from this cap; /params/file enforces its own 1MB limit.
# Set to the global request cap so oversized non-file bodies fail loud.
DATA_UPLOAD_MAX_MEMORY_SIZE = MAX_REQUEST_BYTES
FILE_UPLOAD_MAX_MEMORY_SIZE = MAX_REQUEST_BYTES

USE_TZ = True
USE_I18N = False
DEFAULT_AUTO_FIELD = "django.db.models.BigAutoField"
