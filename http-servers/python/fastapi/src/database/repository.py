from __future__ import annotations

from typing import Literal, Protocol

from src.config.env import env
from src.database.types import CreateUser, UpdateUser, User

DatabaseType = Literal["postgres", "mongodb", "redis", "cassandra"]
DATABASE_TYPES: list[DatabaseType] = ["postgres", "mongodb", "redis", "cassandra"]


class UserRepository(Protocol):
    async def create(self, data: CreateUser) -> User: ...
    async def find_by_id(self, id: str) -> User | None: ...
    async def update(self, id: str, data: UpdateUser) -> User | None: ...
    async def delete(self, id: str) -> bool: ...
    async def delete_all(self) -> None: ...
    async def health_check(self) -> bool: ...
    async def disconnect(self) -> None: ...


_repositories: dict[DatabaseType, UserRepository] = {}


def is_database_type(value: str) -> bool:
    return value in DATABASE_TYPES


def get_repository(database: DatabaseType) -> UserRepository:
    if database in _repositories:
        return _repositories[database]

    repo: UserRepository
    if database == "postgres":
        from src.database.postgres import PostgresUserRepository

        repo = PostgresUserRepository(env.POSTGRES_URL)
    elif database == "mongodb":
        from src.database.mongodb import MongoUserRepository

        repo = MongoUserRepository(env.MONGODB_URL, env.MONGODB_DB)
    elif database == "redis":
        from src.database.redis_repo import RedisUserRepository

        repo = RedisUserRepository(env.REDIS_URL)
    elif database == "cassandra":
        try:
            from src.database.cassandra import CassandraUserRepository

            repo = CassandraUserRepository(
                env.CASSANDRA_CONTACT_POINTS, env.CASSANDRA_LOCAL_DATACENTER, env.CASSANDRA_KEYSPACE
            )
        except ImportError as e:
            raise RuntimeError(f"Cassandra driver not available: {e}") from e
    else:
        raise ValueError(f"Unknown database type: {database}")

    _repositories[database] = repo
    return repo


def resolve_repository(database: str) -> UserRepository | None:
    if not is_database_type(database):
        return None
    return get_repository(database)  # type: ignore[arg-type]
