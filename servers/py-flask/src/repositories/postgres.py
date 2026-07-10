from __future__ import annotations

import threading
import uuid
from typing import Any
from uuid import UUID

from psycopg import Connection, sql
from psycopg.rows import TupleRow, dict_row
from psycopg_pool import ConnectionPool

from bench_shared.schemas import CreateUser, UpdateUser, User

# Fleet fairness canon (PLAN.md:194): a single-process pool of 50, matching
# py-fastapi's SQLAlchemy pool_size=50 and py-django's psycopg pool max_size=50.
_POOL_SIZE = 50


def _parse_uuid(value: str) -> UUID | None:
    try:
        return UUID(value)
    except ValueError:
        return None


def _row_to_user(row: dict[str, Any]) -> User:
    return User(id=str(row["id"]), name=row["name"], email=row["email"], favoriteNumber=row["favorite_number"])


class PostgresRepository:
    """Postgres via the sync psycopg3 driver + a psycopg_pool connection pool.

    The `users` table is provisioned by the shared infra migration
    (infra/databases/postgres/init.sql) — identical schema for every server — so
    this repo never issues DDL, only CRUD.
    """

    def __init__(self, dsn: str) -> None:
        self._dsn = dsn
        self._pool: ConnectionPool[Connection[TupleRow]] | None = None
        self._lock = threading.Lock()

    def _get_pool(self) -> ConnectionPool[Connection[TupleRow]]:
        pool = self._pool
        if pool is not None:
            return pool
        with self._lock:
            if self._pool is None:
                self._pool = ConnectionPool(self._dsn, min_size=1, max_size=_POOL_SIZE, open=True)
            return self._pool

    def create(self, data: CreateUser) -> User:
        uid = uuid.uuid7()
        with self._get_pool().connection() as conn:
            conn.execute(
                "INSERT INTO users (id, name, email, favorite_number) VALUES (%s, %s, %s, %s)",
                (uid, data.name, data.email, data.favoriteNumber),
            )
        return User(id=str(uid), name=data.name, email=data.email, favoriteNumber=data.favoriteNumber)

    def find_by_id(self, id: str) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        with self._get_pool().connection() as conn, conn.cursor(row_factory=dict_row) as cur:
            cur.execute("SELECT id, name, email, favorite_number FROM users WHERE id = %s", (uid,))
            row = cur.fetchone()
        return _row_to_user(row) if row is not None else None

    def update(self, id: str, data: UpdateUser) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None

        assignments: list[sql.Composable] = []
        params: list[Any] = []
        if data.name is not None:
            assignments.append(sql.SQL("name = %s"))
            params.append(data.name)
        if data.email is not None:
            assignments.append(sql.SQL("email = %s"))
            params.append(data.email)
        if data.favoriteNumber is not None:
            assignments.append(sql.SQL("favorite_number = %s"))
            params.append(data.favoriteNumber)

        # No valid fields (e.g. a PATCH carrying only a wrongly-cased key): a no-op
        # that returns the unchanged row (contract: patch_ignores_unknown_field_noop).
        if not assignments:
            return self.find_by_id(id)

        params.append(uid)
        # psycopg.sql composition (not an f-string) keeps this injection-safe and
        # LiteralString-typed: only static "<col> = %s" fragments are composed, all
        # values are %s-bound params.
        query = sql.SQL("UPDATE users SET {} WHERE id = %s RETURNING id, name, email, favorite_number").format(
            sql.SQL(", ").join(assignments)
        )
        with self._get_pool().connection() as conn, conn.cursor(row_factory=dict_row) as cur:
            cur.execute(query, params)
            row = cur.fetchone()
        return _row_to_user(row) if row is not None else None

    def delete(self, id: str) -> bool:
        uid = _parse_uuid(id)
        if uid is None:
            return False
        with self._get_pool().connection() as conn:
            cur = conn.execute("DELETE FROM users WHERE id = %s", (uid,))
            return cur.rowcount > 0

    def delete_all(self) -> None:
        with self._get_pool().connection() as conn:
            conn.execute("DELETE FROM users")

    def health_check(self) -> bool:
        try:
            with self._get_pool().connection() as conn:
                conn.execute("SELECT 1")
        except Exception:
            return False
        return True

    def disconnect(self) -> None:
        if self._pool is not None:
            self._pool.close()
            self._pool = None
