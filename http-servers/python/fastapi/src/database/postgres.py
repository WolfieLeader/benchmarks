from __future__ import annotations

import uuid as uuid_module
from typing import TYPE_CHECKING
from uuid import UUID

from sqlalchemy import String, delete, select, text
from sqlalchemy.dialects.postgresql import UUID as PgUUID
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column

from src.database.types import CreateUser, UpdateUser, User

if TYPE_CHECKING:
    from sqlalchemy.ext.asyncio import AsyncEngine


class Base(DeclarativeBase):
    pass


class UserModel(Base):
    __tablename__ = "users"

    id: Mapped[UUID] = mapped_column(PgUUID(as_uuid=True), primary_key=True)
    name: Mapped[str] = mapped_column(String, nullable=False)
    email: Mapped[str] = mapped_column(String, nullable=False)
    favorite_number: Mapped[int | None] = mapped_column(nullable=True)

    def to_user(self) -> User:
        return User(id=str(self.id), name=self.name, email=self.email, favoriteNumber=self.favorite_number)


class PostgresUserRepository:
    def __init__(self, connection_string: str):
        url = connection_string.replace("postgres://", "postgresql+asyncpg://", 1)
        self._engine: AsyncEngine = create_async_engine(url, pool_size=10, max_overflow=20)
        self._session_maker = async_sessionmaker(self._engine, expire_on_commit=False)

    def _parse_uuid(self, id: str) -> UUID | None:
        try:
            return UUID(id)
        except ValueError:
            return None

    async def create(self, data: CreateUser) -> User:
        async with self._session_maker() as session:
            user = UserModel(
                id=uuid_module.uuid7(),
                name=data.name,
                email=data.email,
                favorite_number=data.favoriteNumber,
            )
            session.add(user)
            await session.commit()
            return user.to_user()

    async def find_by_id(self, id: str) -> User | None:
        uuid_id = self._parse_uuid(id)
        if uuid_id is None:
            return None
        async with self._session_maker() as session:
            result = await session.execute(select(UserModel).where(UserModel.id == uuid_id))
            user = result.scalar_one_or_none()
            return user.to_user() if user else None

    async def update(self, id: str, data: UpdateUser) -> User | None:
        uuid_id = self._parse_uuid(id)
        if uuid_id is None:
            return None
        async with self._session_maker() as session:
            result = await session.execute(select(UserModel).where(UserModel.id == uuid_id))
            user = result.scalar_one_or_none()
            if user is None:
                return None

            if data.name is not None:
                user.name = data.name
            if data.email is not None:
                user.email = data.email
            if data.favoriteNumber is not None:
                user.favorite_number = data.favoriteNumber

            await session.commit()
            return user.to_user()

    async def delete(self, id: str) -> bool:
        uuid_id = self._parse_uuid(id)
        if uuid_id is None:
            return False
        async with self._session_maker() as session:
            result = await session.execute(delete(UserModel).where(UserModel.id == uuid_id))
            await session.commit()
            return result.rowcount == 1

    async def delete_all(self) -> None:
        async with self._session_maker() as session:
            await session.execute(delete(UserModel))
            await session.commit()

    async def health_check(self) -> bool:
        try:
            async with self._session_maker() as session:
                await session.execute(text("SELECT 1"))
                return True
        except Exception:
            return False

    async def disconnect(self) -> None:
        await self._engine.dispose()
