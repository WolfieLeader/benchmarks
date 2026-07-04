from __future__ import annotations

import uuid
from collections.abc import Callable, Coroutine
from typing import Any
from uuid import UUID

from asgiref.sync import sync_to_async
from cassandra.cluster import Cluster  # type: ignore[import-untyped]
from cassandra.policies import AddressTranslator, DCAwareRoundRobinPolicy  # type: ignore[import-untyped]

from bench_shared.schemas import CreateUser, UpdateUser, User


class _ContactPointAddressTranslator(AddressTranslator):
    """Pin every discovered node address to the configured contact point.

    Our single-node Cassandra advertises ``broadcast_rpc_address`` as 127.0.0.1;
    the driver would reconnect to that unreachable-from-another-container address.
    Routing discovered addresses back to the contact point keeps NAT/container
    topologies working (in-container -> ``cassandra``, host -> ``localhost``).
    """

    def __init__(self, host: str) -> None:
        self._host = host

    def translate(self, addr: str) -> str:
        return self._host


def _parse_uuid(value: str) -> UUID | None:
    try:
        return UUID(value)
    except ValueError:
        return None


class CassandraRepository:
    """Cassandra via the sync scylla-driver (no first-party Django support),
    bridged onto the async views with sync_to_async (non-thread-sensitive)."""

    def __init__(self, contact_points: list[str], local_dc: str, keyspace: str) -> None:
        self._contact_points = contact_points
        self._local_dc = local_dc
        self._keyspace = keyspace
        # scylla-driver's cluster/session surface is only partially typed; pin it
        # to Any at this seam (§7.42) so the ORM-less driver calls stay clean under
        # pyright strict without per-call ignores.
        self._cluster: Any = None
        self._session: Any = None
        # Build the sync_to_async bridges once, not per request: each sync_to_async
        # call allocates a fresh SyncToAsync wrapper, so constructing them on the hot
        # path would tax every cassandra request. thread_sensitive=False so calls run
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

    def _connect(self) -> None:
        if self._session is not None:
            return
        self._cluster = Cluster(
            contact_points=self._contact_points,
            load_balancing_policy=DCAwareRoundRobinPolicy(local_dc=self._local_dc),
            address_translator=_ContactPointAddressTranslator(self._contact_points[0]),
        )
        self._session = self._cluster.connect(self._keyspace)

    def _execute(self, query: str, params: tuple[Any, ...] = ()) -> list[Any]:
        self._connect()
        return list(self._session.execute(query, params))

    def _create(self, data: CreateUser) -> User:
        uid = uuid.uuid7()
        if data.favoriteNumber is not None:
            query = "INSERT INTO users (id, name, email, favorite_number) VALUES (%s, %s, %s, %s)"
            params: tuple[Any, ...] = (uid, data.name, data.email, data.favoriteNumber)
        else:
            query = "INSERT INTO users (id, name, email) VALUES (%s, %s, %s)"
            params = (uid, data.name, data.email)
        self._execute(query, params)
        return User(id=str(uid), name=data.name, email=data.email, favoriteNumber=data.favoriteNumber)

    def _find_by_id(self, id: str) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        rows = self._execute("SELECT id, name, email, favorite_number FROM users WHERE id = %s", (uid,))
        if not rows:
            return None
        row = rows[0]
        return User(id=str(row.id), name=row.name, email=row.email, favoriteNumber=row.favorite_number)

    def _update(self, id: str, data: UpdateUser) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        existing = self._find_by_id(id)
        if existing is None:
            return None

        set_clauses: list[str] = []
        params: list[Any] = []
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

        params.append(uid)
        # S608 suppressed: set_clauses holds only static "<col> = %s" literals built
        # above; all values are %s-bound params, never interpolated into the text.
        query = f"UPDATE users SET {', '.join(set_clauses)} WHERE id = %s"  # noqa: S608
        self._execute(query, tuple(params))
        return existing

    def _delete(self, id: str) -> bool:
        uid = _parse_uuid(id)
        if uid is None:
            return False
        if self._find_by_id(id) is None:
            return False
        self._execute("DELETE FROM users WHERE id = %s", (uid,))
        return True

    def _delete_all(self) -> None:
        self._execute("TRUNCATE users")

    def _health_check(self) -> bool:
        try:
            self._connect()
            self._execute("SELECT now() FROM system.local")
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
