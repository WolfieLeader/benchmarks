from __future__ import annotations

from pydantic import BaseModel, EmailStr, Field


class User(BaseModel):
    id: str
    name: str
    email: EmailStr
    favoriteNumber: int | None = None


class CreateUser(BaseModel):
    name: str = Field(min_length=1)
    email: EmailStr
    favoriteNumber: int | None = Field(default=None, ge=0, le=100)


class UpdateUser(BaseModel):
    name: str | None = Field(default=None, min_length=1)
    email: EmailStr | None = None
    favoriteNumber: int | None = Field(default=None, ge=0, le=100)


def build_user(id: str, data: CreateUser) -> User:
    return User(id=id, name=data.name, email=data.email, favoriteNumber=data.favoriteNumber)
