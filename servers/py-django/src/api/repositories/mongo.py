from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any

from asgiref.sync import sync_to_async
from bson import ObjectId
from pymongo import MongoClient, ReturnDocument

from bench_shared.schemas import CreateUser, UpdateUser, User, build_user


def _parse_object_id(value: str) -> ObjectId | None:
    try:
        return ObjectId(value)
    except Exception:
        return None


def _to_user(doc: dict[str, Any]) -> User:
    return User(id=str(doc["_id"]), name=doc["name"], email=doc["email"], favoriteNumber=doc.get("favoriteNumber"))


class MongoRepository:
    """MongoDB via the sync pymongo driver (no first-party Django support),
    bridged onto the async views with sync_to_async (non-thread-sensitive, so
    calls run concurrently in the default executor rather than serializing)."""

    def __init__(self, url: str, db_name: str) -> None:
        self._url = url
        self._db_name = db_name
        self._client: MongoClient[dict[str, Any]] | None = None
        # Build the sync_to_async bridges once, not per request: each sync_to_async
        # call allocates a fresh SyncToAsync wrapper, so constructing them on the hot
        # path would tax every mongo request. thread_sensitive=False so calls run
        # concurrently in the default executor rather than serializing onto one thread.
        self._create_async: Callable[[CreateUser], Coroutine[Any, Any, User]] = sync_to_async(
            self._create, thread_sensitive=False
        )
        self._find_by_id_async: Callable[[str], Coroutine[Any, Any, User | None]] = sync_to_async(
            self._find_by_id, thread_sensitive=False
        )
        self._update_async: Callable[[str, UpdateUser], Coroutine[Any, Any, User | None]] = sync_to_async(
            self._update, thread_sensitive=False
        )
        self._delete_async: Callable[[str], Coroutine[Any, Any, bool]] = sync_to_async(
            self._delete, thread_sensitive=False
        )
        self._delete_all_async: Callable[[], Coroutine[Any, Any, None]] = sync_to_async(
            self._delete_all, thread_sensitive=False
        )
        self._health_check_async: Callable[[], Coroutine[Any, Any, bool]] = sync_to_async(
            self._health_check, thread_sensitive=False
        )

    def _collection(self):
        if self._client is None:
            self._client = MongoClient(self._url)
        return self._client[self._db_name]["users"]

    def _create(self, data: CreateUser) -> User:
        oid = ObjectId()
        doc: dict[str, Any] = {"_id": oid, "name": data.name, "email": data.email}
        if data.favoriteNumber is not None:
            doc["favoriteNumber"] = data.favoriteNumber
        self._collection().insert_one(doc)
        return build_user(str(oid), data)

    def _find_by_id(self, id: str) -> User | None:
        oid = _parse_object_id(id)
        if oid is None:
            return None
        doc = self._collection().find_one({"_id": oid})
        return _to_user(doc) if doc else None

    def _update(self, id: str, data: UpdateUser) -> User | None:
        oid = _parse_object_id(id)
        if oid is None:
            return None
        fields: dict[str, Any] = {}
        if data.name is not None:
            fields["name"] = data.name
        if data.email is not None:
            fields["email"] = data.email
        if data.favoriteNumber is not None:
            fields["favoriteNumber"] = data.favoriteNumber
        if not fields:
            return self._find_by_id(id)
        doc = self._collection().find_one_and_update(
            {"_id": oid}, {"$set": fields}, return_document=ReturnDocument.AFTER
        )
        return _to_user(doc) if doc else None

    def _delete(self, id: str) -> bool:
        oid = _parse_object_id(id)
        if oid is None:
            return False
        result = self._collection().delete_one({"_id": oid})
        return result.deleted_count > 0

    def _delete_all(self) -> None:
        self._collection().delete_many({})

    def _health_check(self) -> bool:
        try:
            if self._client is None:
                self._client = MongoClient(self._url)
            self._client.admin.command("ping")
        except Exception:
            return False
        return True

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
