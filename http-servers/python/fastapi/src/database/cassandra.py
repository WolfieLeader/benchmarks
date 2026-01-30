from __future__ import annotations

import asyncio
from concurrent.futures import ThreadPoolExecutor
import uuid
from uuid import UUID
from cassandra.cluster import Cluster  # type: ignore[import-untyped]
from cassandra.policies import DCAwareRoundRobinPolicy  # type: ignore[import-untyped]

from src.database.types import CreateUser, UpdateUser, User


class CassandraUserRepository:
    def __init__(self, contact_points: list[str], local_dc: str, keyspace: str):
        self._contact_points = contact_points
        self._local_dc = local_dc
        self._keyspace = keyspace
        self._cluster: Cluster | None = None
        self._session = None
        self._executor = ThreadPoolExecutor(max_workers=4)

    def _connect_sync(self) -> None:
        if self._session is not None:
            return
        self._cluster = Cluster(
            contact_points=self._contact_points,
            load_balancing_policy=DCAwareRoundRobinPolicy(local_dc=self._local_dc),
        )
        self._session = self._cluster.connect(self._keyspace)  # type: ignore[union-attr]

    async def _connect(self) -> None:
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(self._executor, self._connect_sync)

    async def _execute(self, query: str, params: tuple = ()) -> list:
        await self._connect()
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(self._executor, lambda: list(self._session.execute(query, params)))  # type: ignore[union-attr]

    async def _execute_one(self, query: str, params: tuple = ()):
        rows = await self._execute(query, params)
        return rows[0] if rows else None

    def _parse_uuid(self, id: str) -> UUID | None:
        try:
            return UUID(id)
        except ValueError:
            return None

    async def create(self, data: CreateUser) -> User:
        id = uuid.uuid7()

        if data.favoriteNumber is not None:
            query = "INSERT INTO users (id, name, email, favorite_number) VALUES (%s, %s, %s, %s)"
            params = (id, data.name, data.email, data.favoriteNumber)
        else:
            query = "INSERT INTO users (id, name, email) VALUES (%s, %s, %s)"
            params = (id, data.name, data.email)

        await self._execute(query, params)
        return User(id=str(id), name=data.name, email=data.email, favoriteNumber=data.favoriteNumber)

    async def find_by_id(self, id: str) -> User | None:
        uuid_id = self._parse_uuid(id)
        if uuid_id is None:
            return None

        query = "SELECT id, name, email, favorite_number FROM users WHERE id = %s"
        row = await self._execute_one(query, (uuid_id,))
        if row is None:
            return None
        return User(id=str(row.id), name=row.name, email=row.email, favoriteNumber=row.favorite_number)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        uuid_id = self._parse_uuid(id)
        if uuid_id is None:
            return None

        existing = await self.find_by_id(id)
        if existing is None:
            return None

        set_clauses = []
        params: list = []

        if data.name is not None:
            set_clauses.append("name = %s")
            params.append(data.name)
            existing.name = data.name
        if data.email is not None:
            set_clauses.append("email = %s")
            params.append(data.email)
            existing.email = data.email
        if data.favoriteNumber is not None:
            set_clauses.append("favorite_number = %s")
            params.append(data.favoriteNumber)
            existing.favoriteNumber = data.favoriteNumber

        if not set_clauses:
            return existing

        params.append(uuid_id)
        query = f"UPDATE users SET {', '.join(set_clauses)} WHERE id = %s"
        await self._execute(query, tuple(params))
        return existing

    async def delete(self, id: str) -> bool:
        uuid_id = self._parse_uuid(id)
        if uuid_id is None:
            return False

        existing = await self.find_by_id(id)
        if existing is None:
            return False

        query = "DELETE FROM users WHERE id = %s"
        await self._execute(query, (uuid_id,))
        return True

    async def delete_all(self) -> None:
        await self._execute("TRUNCATE users")

    async def health_check(self) -> bool:
        try:
            await self._execute("SELECT now() FROM system.local")
            return True
        except Exception:
            return False

    async def disconnect(self) -> None:
        if self._cluster is not None:
            self._cluster.shutdown()
            self._cluster = None
            self._session = None
        self._executor.shutdown(wait=False)
