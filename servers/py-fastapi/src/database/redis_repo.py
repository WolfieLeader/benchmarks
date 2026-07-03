from __future__ import annotations

import uuid

from typing import Any

import redis.asyncio as aioredis

from src.database.types import CreateUser, UpdateUser, User, build_user


def _decode(value: bytes | str) -> str:
    return value.decode() if isinstance(value, bytes) else value


class RedisUserRepository:
    def __init__(self, connection_string: str):
        self._url = connection_string
        self._client: Any = None
        self._prefix = "user:"

    async def _ensure_client(self) -> Any:
        if self._client is None:
            self._client = aioredis.from_url(self._url)
        return self._client

    def _key(self, id: str) -> str:
        return f"{self._prefix}{id}"

    async def create(self, data: CreateUser) -> User:
        client = await self._ensure_client()
        id = str(uuid.uuid7())
        fields: dict[str, str] = {"name": data.name, "email": data.email}
        if data.favoriteNumber is not None:
            fields["favoriteNumber"] = str(data.favoriteNumber)

        await client.hset(self._key(id), mapping=fields)
        return build_user(id, data)

    async def find_by_id(self, id: str) -> User | None:
        client = await self._ensure_client()
        key = self._key(id)
        if not await client.exists(key):
            return None

        result = await client.hmget(key, ["name", "email", "favoriteNumber"])
        if result[0] is None or result[1] is None:
            return None

        favorite_number = None
        if result[2] is not None:
            try:
                favorite_number = int(_decode(result[2]))
            except ValueError:
                return None

        return User(id=id, name=_decode(result[0]), email=_decode(result[1]), favoriteNumber=favorite_number)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        client = await self._ensure_client()
        key = self._key(id)
        if not await client.exists(key):
            return None

        fields: dict[str, str] = {}
        if data.name is not None:
            fields["name"] = data.name
        if data.email is not None:
            fields["email"] = data.email
        if data.favoriteNumber is not None:
            fields["favoriteNumber"] = str(data.favoriteNumber)

        if fields:
            await client.hset(key, mapping=fields)

        return await self.find_by_id(id)

    async def delete(self, id: str) -> bool:
        client = await self._ensure_client()
        deleted = await client.delete(self._key(id))
        return deleted > 0

    async def delete_all(self) -> None:
        client = await self._ensure_client()
        cursor = 0
        while True:
            cursor, keys = await client.scan(cursor, match=f"{self._prefix}*", count=100)
            if keys:
                await client.delete(*keys)
            if cursor == 0:
                break

    async def health_check(self) -> bool:
        try:
            client = await self._ensure_client()
            await client.ping()
            return True
        except Exception:
            return False

    async def disconnect(self) -> None:
        if self._client is not None:
            await self._client.close()
            self._client = None
