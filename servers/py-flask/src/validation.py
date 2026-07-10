"""Deep-nested pydantic schema for POST /validate (web suite).

Kept in-server under the multi-consumer rule (docs/languages/python.md §10):
py-flask is the only web-suite implementer today, so this schema has a single
consumer. When py-fastapi/py-django gain `web: true`, extract the schema to the
shared pydantic schemas (PLAN §3: "pydantic schemas = fastapi+flask").

Contract (contract/web.json, ~4 levels):
    user{ id:uuid, email, profile{ age:0..120, role:admin|user|guest,
          preferences{ theme:light|dark, notifications:bool } } },
    items[]{ sku, quantity:1..100, tags:string[] },
    total:>=0
"""

from __future__ import annotations

from typing import Literal
from uuid import UUID

from pydantic import BaseModel, EmailStr, Field, ValidationError


class _Preferences(BaseModel):
    theme: Literal["light", "dark"]
    notifications: bool


class _Profile(BaseModel):
    age: int = Field(ge=0, le=120)
    role: Literal["admin", "user", "guest"]
    preferences: _Preferences


class _ValidateUser(BaseModel):
    id: UUID
    email: EmailStr
    profile: _Profile


class _Item(BaseModel):
    sku: str
    quantity: int = Field(ge=1, le=100)
    tags: list[str]


class _ValidatePayload(BaseModel):
    user: _ValidateUser
    items: list[_Item]
    total: float = Field(ge=0)


def validate_payload(raw: bytes) -> str | None:
    """Validate the raw request body against the schema.

    Returns None when the payload is valid, else a short per-framework error
    summary for the canonical `{"error": "validation failed", "details": ...}`
    response (the exact count is intentionally not asserted by the contract —
    validators count failures differently). One compiled pydantic-core pass
    parses JSON and validates together (model_validate_json).
    """
    try:
        _ValidatePayload.model_validate_json(raw)
    except ValidationError as e:
        count = e.error_count()
        return f"{count} validation error{'s' if count != 1 else ''}"
    return None
