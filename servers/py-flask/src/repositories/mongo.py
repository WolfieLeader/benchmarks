from __future__ import annotations

import threading
from typing import Any

from bson import ObjectId
from pymongo import MongoClient, ReturnDocument
from pymongo.collection import Collection

from bench_shared.schemas import CreateUser, UpdateUser, User, build_user

_POOL_SIZE = 50


def _parse_object_id(value: str) -> ObjectId | None:
    try:
        return ObjectId(value)
    except Exception:
        return None


def _to_user(doc: dict[str, Any]) -> User:
    return User(id=str(doc["_id"]), name=doc["name"], email=doc["email"], favoriteNumber=doc.get("favoriteNumber"))


class MongoRepository:
    """MongoDB via the sync pymongo driver (thread-safe, connection-pooled)."""

    def __init__(self, url: str, db_name: str) -> None:
        self._url = url
        self._db_name = db_name
        self._client: MongoClient[dict[str, Any]] | None = None
        self._lock = threading.Lock()

    def _get_client(self) -> MongoClient[dict[str, Any]]:
        client = self._client
        if client is not None:
            return client
        with self._lock:
            if self._client is None:
                self._client = MongoClient(self._url, maxPoolSize=_POOL_SIZE)
            return self._client

    def _collection(self) -> Collection[dict[str, Any]]:
        return self._get_client()[self._db_name]["users"]

    def create(self, data: CreateUser) -> User:
        oid = ObjectId()
        doc: dict[str, Any] = {"_id": oid, "name": data.name, "email": data.email}
        if data.favoriteNumber is not None:
            doc["favoriteNumber"] = data.favoriteNumber
        self._collection().insert_one(doc)
        return build_user(str(oid), data)

    def find_by_id(self, id: str) -> User | None:
        oid = _parse_object_id(id)
        if oid is None:
            return None
        doc = self._collection().find_one({"_id": oid})
        return _to_user(doc) if doc else None

    def update(self, id: str, data: UpdateUser) -> User | None:
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
            return self.find_by_id(id)
        doc = self._collection().find_one_and_update(
            {"_id": oid}, {"$set": fields}, return_document=ReturnDocument.AFTER
        )
        return _to_user(doc) if doc else None

    def delete(self, id: str) -> bool:
        oid = _parse_object_id(id)
        if oid is None:
            return False
        result = self._collection().delete_one({"_id": oid})
        return result.deleted_count > 0

    def delete_all(self) -> None:
        self._collection().delete_many({})

    def health_check(self) -> bool:
        try:
            self._get_client().admin.command("ping")
        except Exception:
            return False
        return True

    def disconnect(self) -> None:
        if self._client is not None:
            self._client.close()
            self._client = None
