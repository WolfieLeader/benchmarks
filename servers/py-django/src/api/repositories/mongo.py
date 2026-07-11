from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any

from asgiref.sync import sync_to_async

from bench_shared.repositories.mongo import SyncMongoRepository
from bench_shared.schemas import CreateUser, UpdateUser, User


class MongoRepository:
    """MongoDB via the shared sync pymongo core (no first-party Django support),
    bridged onto the async views with sync_to_async (non-thread-sensitive, so
    calls run concurrently in the default executor rather than serializing)."""

    def __init__(self, url: str, db_name: str) -> None:
        # No explicit max_pool_size — the driver default is this server's
        # pre-extraction behavior (pool config stays per-server).
        core = SyncMongoRepository(url, db_name)
        # Build the sync_to_async bridges once, not per request: each sync_to_async
        # call allocates a fresh SyncToAsync wrapper, so constructing them on the hot
        # path would tax every mongo request. thread_sensitive=False so calls run
        # concurrently in the default executor rather than serializing onto one thread.
        self._create_async: Callable[[CreateUser], Coroutine[Any, Any, User]] = sync_to_async(
            core.create, thread_sensitive=False
        )
        self._find_by_id_async: Callable[[str], Coroutine[Any, Any, User | None]] = sync_to_async(
            core.find_by_id, thread_sensitive=False
        )
        self._update_async: Callable[[str, UpdateUser], Coroutine[Any, Any, User | None]] = sync_to_async(
            core.update, thread_sensitive=False
        )
        self._delete_async: Callable[[str], Coroutine[Any, Any, bool]] = sync_to_async(
            core.delete, thread_sensitive=False
        )
        self._delete_all_async: Callable[[], Coroutine[Any, Any, None]] = sync_to_async(
            core.delete_all, thread_sensitive=False
        )
        self._health_check_async: Callable[[], Coroutine[Any, Any, bool]] = sync_to_async(
            core.health_check, thread_sensitive=False
        )

    async def create(self, data: CreateUser) -> User:
        return await self._create_async(data)

    async def find_by_id(self, id: str) -> User | None:
        return await self._find_by_id_async(id)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        return await self._update_async(id, data)

    async def delete(self, id: str) -> bool:
        return await self._delete_async(id)

    async def delete_all(self) -> None:
        await self._delete_all_async()

    async def health_check(self) -> bool:
        return await self._health_check_async()
