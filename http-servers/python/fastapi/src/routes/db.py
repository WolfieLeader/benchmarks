from __future__ import annotations

from fastapi import APIRouter, HTTPException

from src.consts.errors import INTERNAL_ERROR, NOT_FOUND, make_error
from src.database.repository import resolve_repository
from src.database.types import CreateUser, UpdateUser

db_router = APIRouter()


@db_router.post("/{database}/users", status_code=201)
async def create_user(database: str, data: CreateUser):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))

    try:
        user = await repo.create(data)
        return user
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.get("/{database}/users/{id}")
async def get_user(database: str, id: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))

    try:
        user = await repo.find_by_id(id)
        if user is None:
            raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"user with id {id} not found"))
        return user
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.patch("/{database}/users/{id}")
async def update_user(database: str, id: str, data: UpdateUser):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))

    try:
        user = await repo.update(id, data)
        if user is None:
            raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"user with id {id} not found"))
        return user
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.delete("/{database}/users/{id}")
async def delete_user(database: str, id: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))

    try:
        deleted = await repo.delete(id)
        if not deleted:
            raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"user with id {id} not found"))
        return {"success": True}
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.delete("/{database}/users")
async def delete_all_users(database: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))

    try:
        await repo.delete_all()
        return {"success": True}
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))


@db_router.delete("/{database}/reset")
async def reset_database(database: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=make_error(NOT_FOUND, f"unknown database type: {database}"))

    try:
        await repo.delete_all()
        return {"status": "ok"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=make_error(INTERNAL_ERROR, e))
