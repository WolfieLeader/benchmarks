from __future__ import annotations

import uuid
from typing import Any

import redis.asyncio as aioredis

from src.database.types import CreateUser, UpdateUser, User, build_user


class RedisUserRepository:
    def __init__(self, connection_string: str):
        self._url = connection_string
        self._client: Any = None
        self._prefix = "user:"

    async def _connect(self) -> None:
        if self._client is not None:
            return
        self._client = aioredis.from_url(self._url)

    def _key(self, id: str) -> str:
        return f"{self._prefix}{id}"

    async def create(self, data: CreateUser) -> User:
        await self._connect()
        assert self._client is not None

        id = str(uuid.uuid7())
        fields: dict[str, str] = {"name": data.name, "email": data.email}
        if data.favoriteNumber is not None:
            fields["favoriteNumber"] = str(data.favoriteNumber)

        await self._client.hset(self._key(id), mapping=fields)
        return build_user(id, data)

    async def find_by_id(self, id: str) -> User | None:
        await self._connect()
        assert self._client is not None

        key = self._key(id)
        exists = await self._client.exists(key)
        if not exists:
            return None

        result = await self._client.hmget(key, ["name", "email", "favoriteNumber"])
        if result[0] is None or result[1] is None:
            return None

        name = result[0].decode() if isinstance(result[0], bytes) else result[0]
        email = result[1].decode() if isinstance(result[1], bytes) else result[1]

        favorite_number = None
        if result[2] is not None:
            fav_str = result[2].decode() if isinstance(result[2], bytes) else result[2]
            try:
                favorite_number = int(fav_str)
            except ValueError:
                return None

        return User(id=id, name=name, email=email, favoriteNumber=favorite_number)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        await self._connect()
        assert self._client is not None

        key = self._key(id)
        exists = await self._client.exists(key)
        if not exists:
            return None

        fields: dict[str, str] = {}
        if data.name is not None:
            fields["name"] = data.name
        if data.email is not None:
            fields["email"] = data.email
        if data.favoriteNumber is not None:
            fields["favoriteNumber"] = str(data.favoriteNumber)

        if fields:
            await self._client.hset(key, mapping=fields)

        return await self.find_by_id(id)

    async def delete(self, id: str) -> bool:
        await self._connect()
        assert self._client is not None

        deleted = await self._client.delete(self._key(id))
        return deleted > 0

    async def delete_all(self) -> None:
        await self._connect()
        assert self._client is not None

        keys = await self._client.keys(f"{self._prefix}*")
        if keys:
            await self._client.delete(*keys)

    async def health_check(self) -> bool:
        try:
            await self._connect()
            assert self._client is not None
            await self._client.ping()
            return True
        except Exception:
            return False

    async def disconnect(self) -> None:
        if self._client is not None:
            await self._client.close()
            self._client = None
