from __future__ import annotations

import uuid
from typing import Any

from django.core.cache import cache as _cache

from bench_shared.schemas import CreateUser, UpdateUser, User, build_user

# django-types' cache stub is incomplete for Django's async cache API
# (adelete/aclear are absent), so the cache proxy is pinned to Any at this seam —
# the §7.42 "explicit Any at the untyped boundary" pattern, done once here rather
# than as scattered per-call ignores.
cache: Any = _cache

_PREFIX = "user:"


def _key(id: str) -> str:
    return f"{_PREFIX}{id}"


def _record_to_user(id: str, record: dict[str, Any]) -> User:
    return User(id=id, name=record["name"], email=record["email"], favoriteNumber=record.get("favoriteNumber"))


class RedisRepository:
    """Redis via Django's first-party cache backend (locked decision, PLAN §3).

    CRUD maps onto cache.aset/aget/adelete + aclear; partial updates are a
    read-modify-write through the same abstraction. delete_all uses aclear (a
    Redis FLUSHDB): the benchmark Redis is dedicated, so clearing the whole
    keyspace is equivalent to removing every user.
    """

    async def create(self, data: CreateUser) -> User:
        id = str(uuid.uuid7())
        record: dict[str, Any] = {"name": data.name, "email": data.email, "favoriteNumber": data.favoriteNumber}
        await cache.aset(_key(id), record)
        return build_user(id, data)

    async def find_by_id(self, id: str) -> User | None:
        record: dict[str, Any] | None = await cache.aget(_key(id))
        if record is None:
            return None
        return _record_to_user(id, record)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        record: dict[str, Any] | None = await cache.aget(_key(id))
        if record is None:
            return None
        if data.name is not None:
            record["name"] = data.name
        if data.email is not None:
            record["email"] = data.email
        if data.favoriteNumber is not None:
            record["favoriteNumber"] = data.favoriteNumber
        await cache.aset(_key(id), record)
        return _record_to_user(id, record)

    async def delete(self, id: str) -> bool:
        if await cache.aget(_key(id)) is None:
            return False
        await cache.adelete(_key(id))
        return True

    async def delete_all(self) -> None:
        await cache.aclear()

    async def health_check(self) -> bool:
        try:
            await cache.aget("__health__")
        except Exception:
            return False
        return True
