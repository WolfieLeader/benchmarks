from __future__ import annotations

import json
import threading
import uuid
from typing import Any

import redis as _redis_module

from bench_shared.schemas import CreateUser, UpdateUser, User, build_user

# redis-py's stubs are incomplete under pyright strict (from_url / flushdb / ping
# are partially unknown, and Redis is not subscriptable), so the client library is
# pinned to Any at this seam — the §7.42 "explicit Any at the untyped boundary"
# pattern, done once here rather than as scattered per-call ignores.
redis: Any = _redis_module

_POOL_SIZE = 50
_PREFIX = "user:"


def _key(id: str) -> str:
    return f"{_PREFIX}{id}"


def _record_to_user(id: str, record: dict[str, Any]) -> User:
    return User(id=id, name=record["name"], email=record["email"], favoriteNumber=record.get("favoriteNumber"))


class RedisRepository:
    """Redis via the sync redis-py client. A user is a JSON string under
    ``user:<id>``; partial updates are a read-modify-write. delete_all is a
    FLUSHDB — the benchmark Redis is dedicated, so clearing the keyspace is
    equivalent to removing every user (matches the other servers' reset)."""

    def __init__(self, url: str) -> None:
        self._url = url
        self._client: Any = None
        self._lock = threading.Lock()

    def _get_client(self) -> Any:
        client = self._client
        if client is not None:
            return client
        with self._lock:
            if self._client is None:
                self._client = redis.Redis.from_url(self._url, max_connections=_POOL_SIZE, decode_responses=True)
            return self._client

    def create(self, data: CreateUser) -> User:
        id = str(uuid.uuid7())
        record: dict[str, Any] = {"name": data.name, "email": data.email, "favoriteNumber": data.favoriteNumber}
        self._get_client().set(_key(id), json.dumps(record))
        return build_user(id, data)

    def find_by_id(self, id: str) -> User | None:
        raw = self._get_client().get(_key(id))
        if raw is None:
            return None
        record: dict[str, Any] = json.loads(raw)
        return _record_to_user(id, record)

    def update(self, id: str, data: UpdateUser) -> User | None:
        client = self._get_client()
        raw = client.get(_key(id))
        if raw is None:
            return None
        record: dict[str, Any] = json.loads(raw)
        if data.name is not None:
            record["name"] = data.name
        if data.email is not None:
            record["email"] = data.email
        if data.favoriteNumber is not None:
            record["favoriteNumber"] = data.favoriteNumber
        client.set(_key(id), json.dumps(record))
        return _record_to_user(id, record)

    def delete(self, id: str) -> bool:
        return self._get_client().delete(_key(id)) > 0

    def delete_all(self) -> None:
        self._get_client().flushdb()

    def health_check(self) -> bool:
        try:
            self._get_client().ping()
        except Exception:
            return False
        return True

    def disconnect(self) -> None:
        if self._client is not None:
            self._client.close()
            self._client = None
