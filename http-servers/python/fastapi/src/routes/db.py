from __future__ import annotations

from fastapi import APIRouter, HTTPException
from fastapi.responses import JSONResponse

from src.consts.errors import INTERNAL_ERROR, NOT_FOUND
from src.database.repository import resolve_repository
from src.database.types import CreateUser, UpdateUser

db_router = APIRouter()


@db_router.post("/{database}/users", status_code=201)
async def create_user(database: str, data: CreateUser):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        user = await repo.create(data)
        return user
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)


@db_router.get("/{database}/users/{id}")
async def get_user(database: str, id: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        user = await repo.find_by_id(id)
        if user is None:
            raise HTTPException(status_code=404, detail=NOT_FOUND)
        return user
    except HTTPException:
        raise
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)


@db_router.patch("/{database}/users/{id}")
async def update_user(database: str, id: str, data: UpdateUser):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        user = await repo.update(id, data)
        if user is None:
            raise HTTPException(status_code=404, detail=NOT_FOUND)
        return user
    except HTTPException:
        raise
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)


@db_router.delete("/{database}/users/{id}")
async def delete_user(database: str, id: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        deleted = await repo.delete(id)
        if not deleted:
            raise HTTPException(status_code=404, detail=NOT_FOUND)
        return {"success": True}
    except HTTPException:
        raise
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)


@db_router.delete("/{database}/users")
async def delete_all_users(database: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        await repo.delete_all()
        return {"success": True}
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)


@db_router.delete("/{database}/reset")
async def reset_database(database: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        await repo.delete_all()
        return {"status": "ok"}
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)


@db_router.get("/{database}/health")
async def health_check(database: str):
    repo = resolve_repository(database)
    if repo is None:
        raise HTTPException(status_code=404, detail=NOT_FOUND)

    try:
        healthy = await repo.health_check()
        if not healthy:
            return JSONResponse(status_code=503, content={"error": "database unavailable"})
        return {"status": "healthy"}
    except Exception:
        raise HTTPException(status_code=500, detail=INTERNAL_ERROR)
