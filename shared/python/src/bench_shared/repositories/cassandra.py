from __future__ import annotations

import threading
import uuid
from typing import Any
from uuid import UUID

from cassandra.cluster import Cluster  # type: ignore[import-untyped]
from cassandra.policies import AddressTranslator, DCAwareRoundRobinPolicy  # type: ignore[import-untyped]

from bench_shared.schemas import CreateUser, UpdateUser, User

_DEFAULT_PORT = 9042


def split_contact_points(contact_points: list[str]) -> tuple[list[str], int]:
    """Split ``host[:port]`` contact points into hosts plus the single driver port.

    This driver takes hosts and the port as separate arguments
    (``Cluster(contact_points=..., port=...)``); a raw ``host:port`` string is
    treated whole as a hostname and fails resolution. Mirrors shared/rust's
    ``resolve_addr`` and shared/kotlin's ``ContactPointTranslator``: the port
    comes from the first contact point that carries one (the driver supports
    only one port), defaulting to 9042. Non-numeric ports fall back to the
    default, like Kotlin's ``toIntOrNull() ?: DEFAULT_PORT``.
    """
    hosts: list[str] = []
    port = _DEFAULT_PORT
    port_seen = False
    for point in contact_points:
        host, sep, port_str = point.partition(":")
        hosts.append(host)
        if sep and not port_seen and port_str.isdigit():
            port = int(port_str)
            port_seen = True
    return hosts, port


class ContactPointAddressTranslator(AddressTranslator):
    """Pin every discovered node address to the configured contact point.

    Our single-node Cassandra advertises ``broadcast_rpc_address`` as 127.0.0.1;
    the driver would otherwise reconnect to that unreachable-from-another-container
    address. Routing discovered addresses back to the contact point keeps
    NAT/container topologies working (in-container -> ``cassandra``, host ->
    ``localhost``). (Load-bearing, docs/languages/python.md §10.)
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


class SyncCassandraRepository:
    """Cassandra user-repository core via the sync scylla-driver.

    Shared by the sync Python servers; async bridging (py-django's sync_to_async)
    stays in-server. The driver's cluster/session surface is only partially typed,
    so it is pinned to Any at this seam (§7.42) — the ORM-less driver calls stay
    clean under pyright strict without per-call ignores.
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
                hosts, port = split_contact_points(self._contact_points)
                self._cluster = Cluster(
                    contact_points=hosts,
                    port=port,
                    load_balancing_policy=DCAwareRoundRobinPolicy(local_dc=self._local_dc),
                    address_translator=ContactPointAddressTranslator(hosts[0]),
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
