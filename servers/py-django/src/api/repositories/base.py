from __future__ import annotations

from typing import Literal, Protocol

from bench_shared.env import env
from bench_shared.schemas import CreateUser, UpdateUser, User

DatabaseType = Literal["postgres", "mongodb", "redis", "cassandra"]
DATABASE_TYPES: tuple[DatabaseType, ...] = ("postgres", "mongodb", "redis", "cassandra")


class UserRepository(Protocol):
    async def create(self, data: CreateUser) -> User: ...
    async def find_by_id(self, id: str) -> User | None: ...
    async def update(self, id: str, data: UpdateUser) -> User | None: ...
    async def delete(self, id: str) -> bool: ...
    async def delete_all(self) -> None: ...
    async def health_check(self) -> bool: ...


_repositories: dict[str, UserRepository] = {}


def resolve_repository(database: str) -> UserRepository | None:
    if database not in DATABASE_TYPES:
        return None
    if database in _repositories:
        return _repositories[database]

    repo: UserRepository
    if database == "postgres":
        from src.api.repositories.postgres import PostgresRepository

        repo = PostgresRepository()
    elif database == "mongodb":
        from src.api.repositories.mongo import MongoRepository

        repo = MongoRepository(env.MONGODB_URL, env.MONGODB_DB)
    elif database == "redis":
        from src.api.repositories.redis_cache import RedisRepository

        repo = RedisRepository()
    else:
        from src.api.repositories.cassandra import CassandraRepository

        repo = CassandraRepository(env.CASSANDRA_CONTACT_POINTS, env.CASSANDRA_LOCAL_DATACENTER, env.CASSANDRA_KEYSPACE)

    _repositories[database] = repo
    return repo
