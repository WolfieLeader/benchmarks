from __future__ import annotations

import threading
import uuid
from typing import Any
from uuid import UUID

from cassandra.cluster import Cluster  # type: ignore[import-untyped]
from cassandra.policies import AddressTranslator, DCAwareRoundRobinPolicy  # type: ignore[import-untyped]

from bench_shared.schemas import CreateUser, UpdateUser, User


class _ContactPointAddressTranslator(AddressTranslator):
    """Pin every discovered node address to the configured contact point.

    Our single-node Cassandra advertises ``broadcast_rpc_address`` as 127.0.0.1;
    the driver would otherwise reconnect to that unreachable-from-another-container
    address. (Load-bearing, docs/languages/python.md §10.)

    NOTE (multi-consumer trigger): this is a verbatim copy of py-django's
    translator — py-flask is now the SECOND Python Cassandra consumer, so the
    rule says extract it (and the sync repo cores) into shared. Doing that here
    would mean refactoring py-django too (out of this slice's blast radius), so it
    is duplicated for now and flagged for a dedicated extraction lane.
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
    """Cassandra via the sync scylla-driver. The driver's cluster/session surface
    is only partially typed, so it is pinned to Any at this seam (§7.42) — the
    ORM-less driver calls stay clean under pyright strict without per-call ignores.
    """

    def __init__(self, contact_points: list[str], local_dc: str, keyspace: str) -> None:
        self._contact_points = contact_points
        self._local_dc = local_dc
        self._keyspace = keyspace
        self._cluster: Any = None
        self._session: Any = None
        self._lock = threading.Lock()

    def _get_session(self) -> Any:
        session = self._session
        if session is not None:
            return session
        with self._lock:
            if self._session is None:
                self._cluster = Cluster(
                    contact_points=self._contact_points,
                    load_balancing_policy=DCAwareRoundRobinPolicy(local_dc=self._local_dc),
                    address_translator=_ContactPointAddressTranslator(self._contact_points[0]),
                )
                self._session = self._cluster.connect(self._keyspace)
            return self._session

    def _execute(self, query: str, params: tuple[Any, ...] = ()) -> list[Any]:
        return list(self._get_session().execute(query, params))

    def create(self, data: CreateUser) -> User:
        uid = uuid.uuid7()
        if data.favoriteNumber is not None:
            query = "INSERT INTO users (id, name, email, favorite_number) VALUES (%s, %s, %s, %s)"
            params: tuple[Any, ...] = (uid, data.name, data.email, data.favoriteNumber)
        else:
            query = "INSERT INTO users (id, name, email) VALUES (%s, %s, %s)"
            params = (uid, data.name, data.email)
        self._execute(query, params)
        return User(id=str(uid), name=data.name, email=data.email, favoriteNumber=data.favoriteNumber)

    def find_by_id(self, id: str) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        rows = self._execute("SELECT id, name, email, favorite_number FROM users WHERE id = %s", (uid,))
        if not rows:
            return None
        row = rows[0]
        return User(id=str(row.id), name=row.name, email=row.email, favoriteNumber=row.favorite_number)

    def update(self, id: str, data: UpdateUser) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        existing = self.find_by_id(id)
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
        # S608: set_clauses holds only static "<col> = %s" literals built above; all
        # values are %s-bound params, never interpolated into the query text.
        query = f"UPDATE users SET {', '.join(set_clauses)} WHERE id = %s"  # noqa: S608
        self._execute(query, tuple(params))
        return existing

    def delete(self, id: str) -> bool:
        uid = _parse_uuid(id)
        if uid is None:
            return False
        if self.find_by_id(id) is None:
            return False
        self._execute("DELETE FROM users WHERE id = %s", (uid,))
        return True

    def delete_all(self) -> None:
        self._execute("TRUNCATE users")

    def health_check(self) -> bool:
        try:
            self._execute("SELECT now() FROM system.local")
        except Exception:
            return False
        return True

    def disconnect(self) -> None:
        if self._cluster is not None:
            self._cluster.shutdown()
            self._cluster = None
            self._session = None
