from __future__ import annotations

import uuid
from uuid import UUID

from bench_shared.schemas import CreateUser, UpdateUser, User
from src.api.models import UserModel


def _to_user(obj: UserModel) -> User:
    return User(id=str(obj.id), name=obj.name, email=obj.email, favoriteNumber=obj.favorite_number)


def _parse_uuid(value: str) -> UUID | None:
    try:
        return UUID(value)
    except ValueError:
        return None


class PostgresRepository:
    """Postgres via the Django ORM (batteries-included, PLAN §3), async methods."""

    async def create(self, data: CreateUser) -> User:
        obj = await UserModel.objects.acreate(
            id=uuid.uuid7(),
            name=data.name,
            email=data.email,
            favorite_number=data.favoriteNumber,
        )
        return _to_user(obj)

    async def find_by_id(self, id: str) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        try:
            obj = await UserModel.objects.aget(id=uid)
        except UserModel.DoesNotExist:
            return None
        return _to_user(obj)

    async def update(self, id: str, data: UpdateUser) -> User | None:
        uid = _parse_uuid(id)
        if uid is None:
            return None
        try:
            obj = await UserModel.objects.aget(id=uid)
        except UserModel.DoesNotExist:
            return None

        fields: list[str] = []
        if data.name is not None:
            obj.name = data.name
            fields.append("name")
        if data.email is not None:
            obj.email = data.email
            fields.append("email")
        if data.favoriteNumber is not None:
            obj.favorite_number = data.favoriteNumber
            fields.append("favorite_number")

        if fields:
            await obj.asave(update_fields=fields)
        return _to_user(obj)

    async def delete(self, id: str) -> bool:
        uid = _parse_uuid(id)
        if uid is None:
            return False
        deleted, _ = await UserModel.objects.filter(id=uid).adelete()
        return deleted > 0

    async def delete_all(self) -> None:
        await UserModel.objects.all().adelete()

    async def health_check(self) -> bool:
        try:
            await UserModel.objects.all().aexists()
        except Exception:
            return False
        return True
