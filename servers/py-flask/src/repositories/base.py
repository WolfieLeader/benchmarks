from __future__ import annotations

import contextlib
import threading
from typing import Literal, Protocol

from bench_shared.env import env
from bench_shared.schemas import CreateUser, UpdateUser, User

DatabaseType = Literal["postgres", "mongodb", "redis", "cassandra"]
DATABASE_TYPES: tuple[DatabaseType, ...] = ("postgres", "mongodb", "redis", "cassandra")


class UserRepository(Protocol):
    """The shape every sync DB backend satisfies (mirrors py-fastapi's async
    Protocol, sans async). Four unrelated driver classes (psycopg3, pymongo,
    redis-py, scylla-driver) implement it structurally — mongo/cassandra via
    the shared bench_shared.repositories cores."""

    def create(self, data: CreateUser) -> User: ...
    def find_by_id(self, id: str) -> User | None: ...
    def update(self, id: str, data: UpdateUser) -> User | None: ...
    def delete(self, id: str) -> bool: ...
    def delete_all(self) -> None: ...
    def health_check(self) -> bool: ...
    def disconnect(self) -> None: ...


_repositories: dict[str, UserRepository] = {}
_lock = threading.Lock()

# Canon pool bound (PLAN §3), same 50 that postgres/redis carry in their modules.
# Pool config stays per-server — the shared mongo core takes it as a parameter.
_MONGO_POOL_SIZE = 50


def _build(database: str) -> UserRepository:
    # Deferred imports: a driver a given DB never uses is never imported (nor its
    # module-level cost paid) unless that DB is exercised. `database` is already
    # known to be one of DATABASE_TYPES (resolve_repository guards it).
    if database == "postgres":
        from src.repositories.postgres import PostgresRepository

        return PostgresRepository(env.POSTGRES_URL)
    if database == "mongodb":
        from bench_shared.repositories.mongo import SyncMongoRepository

        return SyncMongoRepository(env.MONGODB_URL, env.MONGODB_DB, max_pool_size=_MONGO_POOL_SIZE)
    if database == "redis":
        from src.repositories.redis_repo import RedisRepository

        return RedisRepository(env.REDIS_URL)
    from bench_shared.repositories.cassandra import SyncCassandraRepository

    return SyncCassandraRepository(env.CASSANDRA_CONTACT_POINTS, env.CASSANDRA_LOCAL_DATACENTER, env.CASSANDRA_KEYSPACE)


def resolve_repository(database: str) -> UserRepository | None:
    """Return the singleton repository for `database`, or None if unknown.

    Thread-safe lazy construction: the fast path is a lock-free dict read (atomic
    under the GIL); only the one-time build per DB takes the lock (double-checked).
    Construction only stores config — each repo connects lazily on first use — so
    the lock is never held across network I/O.
    """
    if database not in DATABASE_TYPES:
        return None
    repo = _repositories.get(database)
    if repo is not None:
        return repo
    with _lock:
        repo = _repositories.get(database)
        if repo is None:
            repo = _build(database)
            _repositories[database] = repo
        return repo


def close_all() -> None:
    """Disconnect every constructed repository — registered via atexit so pools
    close on graceful worker shutdown (SIGTERM/SIGINT drained by gunicorn)."""
    with _lock:
        for repo in _repositories.values():
            with contextlib.suppress(Exception):
                repo.disconnect()
        _repositories.clear()
