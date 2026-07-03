from __future__ import annotations

from fastapi import APIRouter, HTTPException, Response

from src.consts.errors import INTERNAL_ERROR, NOT_FOUND, make_error
from src.database.repository import UserRepository, resolve_repository
from src.database.types import CreateUser, UpdateUser

db_router = APIRouter()


def _require_repo(database: str) -> UserRepository:
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))
    return repo


def _not_found(id: str) -> HTTPException:
    return HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"user with id {id} not found"))


@db_router.get("/{database}/health")
async def database_health(database: str):
    repo = resolve_repository(database)
    if repo is None:
        return Response(content="Service Unavailable", status_code=503, media_type="text/plain")

    healthy = await repo.health_check()
    if healthy:
        return Response(content="OK", media_type="text/plain")
    return Response(content="Service Unavailable", status_code=503, media_type="text/plain")


@db_router.post("/{database}/users", status_code=201)
async def create_user(database: str, data: CreateUser):
    repo = _require_repo(database)
    try:
        user = await repo.create(data)
        return user.model_dump(exclude_none=True)
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.get("/{database}/users/{id}")
async def get_user(database: str, id: str):
    repo = _require_repo(database)
    try:
        user = await repo.find_by_id(id)
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))
    if user is None:
        raise _not_found(id)
    return user.model_dump(exclude_none=True)


@db_router.patch("/{database}/users/{id}")
async def update_user(database: str, id: str, data: UpdateUser):
    repo = _require_repo(database)
    try:
        user = await repo.update(id, data)
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))
    if user is None:
        raise _not_found(id)
    return user.model_dump(exclude_none=True)


@db_router.delete("/{database}/users/{id}")
async def delete_user(database: str, id: str):
    repo = _require_repo(database)
    try:
        deleted = await repo.delete(id)
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))
    if not deleted:
        raise _not_found(id)
    return {"success": True}


@db_router.delete("/{database}/users")
async def delete_all_users(database: str):
    repo = _require_repo(database)
    try:
        await repo.delete_all()
        return {"success": True}
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.delete("/{database}/reset")
async def reset_database(database: str):
    repo = _require_repo(database)
    try:
        await repo.delete_all()
        return {"status": "ok"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))
